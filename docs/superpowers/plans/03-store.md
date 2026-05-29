# Phase2永続化エンジン 実装計画

> For agentic workers: REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

Goal: Phase1で定義したport.Repositoryを、全データをメモリ常駐させアトミックJSON永続化で満たすinternal/storeとして実装します。起動時に全件をロードし、sync.RWMutexで保護し、書き込みはtempファイルへ出力したのちos.Renameで原子的に反映し、エンティティ別のJSONファイル(data/feeds.json、data/categories.json、data/boards.json、data/filters.json、data/settings.json、data/user.json、data/items/フィードID.json)に分割保存します。

Architecture: internal/storeはport.Repositoryのただ1つの実装です。Storeはロード済みの全エンティティをメモリ上のマップとスライスで保持し、sync.RWMutexで読み書きを保護します。読み取りメソッドはメモリから値のコピーを返し、書き込みメソッドはメモリを更新したうえで対応するJSONファイルをアトミックに書き出します。ファイルへの書き込みは、同一ディレクトリ内のtempファイルへ書いてからos.Renameで置き換える方式で、書き込み途中のクラッシュによる破損を避けます。記事はFeedID単位でdata/items/配下に分割保存し、DeleteFeedはフィード本体と対応する記事ファイルの両方を削除します。defer Closeとファイル書き込みのエラーはinternal/obsのログ付きヘルパCloseAndLogとWriteAndLogを経由して握り潰さずに記録します。テストはフェイクではなく実ファイルを使い、t.TempDirの一時ディレクトリをデータディレクトリに指定してI/O経路を含めて検証します。

Tech Stack: Goの標準ライブラリ(encoding/json、os、path/filepath、sync、io、log/slog)、port.Repositoryインターフェース、internal/domainのエンティティ型。

前提: Phase1(02-domain-port.md)が完了し、internal/domainの全エンティティ型とinternal/portの全インターフェースが存在します。作業ディレクトリは`/Users/yujiokamoto/devs/golang/feedflow-go-htmx`です。`bash scripts/quality-gate.sh`がPhase1完了時点で緑であることを確認してから始めます。

設計上の確定事項です。後続タスクはこれに厳密に従います。

- Storeはコンストラクタstore.New(dataDir string) (*Store, error)で生成し、生成時に全件ロードを行います。dataDirと配下のitemsディレクトリが無ければ作成します。
- port.RepositoryのFeedとUser以外の取得メソッドは、メモリ上のスライスを安定した順序で返します。返すスライスは内部状態を共有しない新規スライスにします。
- Feed(id)は見つからないときsentinel errorのErrNotFoundをラップして返します。User()は未登録のときゼロ値のdomain.Userとnilを返します(設計書セクション5.2およびPhase1のRepositoryコメントに準拠)。
- Settings()はsettings.jsonが無いときdomain.DefaultSettings()を返します。
- 書き込みメソッドはメモリ更新と当該エンティティのJSONファイル書き出しを同一ロック区間内で行い、ファイル書き出しに失敗したらメモリ更新を巻き戻してエラーを返します。
- itemsはFeedID単位でdata/items/フィードID.jsonに保存します。SaveItemsは既存ファイルを置き換えます。DeleteFeedはfeeds.jsonの更新に加えて該当する記事ファイルを削除します。
- エラーはfmt.Errorfで文脈付きにラップし、握り潰しません。defer Closeはobs.CloseAndLog経由で記録します。

---

## Task 1: obsログ付きヘルパ

Files:
- Test: `/Users/yujiokamoto/devs/golang/feedflow-go-htmx/internal/obs/obs_test.go`
- Create: `/Users/yujiokamoto/devs/golang/feedflow-go-htmx/internal/obs/obs.go`

internal/storeはdefer Closeとファイル書き込みでエラーを握り潰さない方針です。設計書セクション5.1とオーバービューのファイル構造マップのとおり、internal/obsにログ付きのCloseAndLogとWriteAndLogを用意し、storeとそれ以降の層が共通利用します。obsはstoreより先に必要になるためPhase2の最初に作成します。

- [ ] Step 1: 失敗するテストを書く

Create `internal/obs/obs_test.go`:
```go
package obs_test

import (
	"bytes"
	"errors"
	"log/slog"
	"strings"
	"testing"

	"github.com/okamyuji/feedflow-go-htmx/internal/obs"
)

// errCloser 任意のエラーを返すフェイクio.Closerです。
type errCloser struct {
	err error
}

func (c errCloser) Close() error { return c.err }

func TestCloseAndLog(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		closeErr  error
		wantLog   bool
		wantLevel string
	}{
		{name: "no error logs nothing", closeErr: nil, wantLog: false},
		{name: "error is logged at warn", closeErr: errors.New("boom"), wantLog: true, wantLevel: "WARN"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			logger := slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug}))

			obs.CloseAndLog(logger, errCloser{err: tt.closeErr}, "closing feeds file")

			got := buf.String()
			if tt.wantLog {
				if !strings.Contains(got, "closing feeds file") {
					t.Fatalf("log got %q want it to contain context message", got)
				}
				if !strings.Contains(got, tt.wantLevel) {
					t.Fatalf("log got %q want level %q", got, tt.wantLevel)
				}
				if !strings.Contains(got, "boom") {
					t.Fatalf("log got %q want it to contain the close error", got)
				}
			} else if got != "" {
				t.Fatalf("log got %q want empty", got)
			}
		})
	}
}

func TestWriteAndLog(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug}))

	var sink bytes.Buffer
	n, err := obs.WriteAndLog(logger, &sink, []byte("payload"), "writing response")
	if err != nil {
		t.Fatalf("WriteAndLog returned error: %v", err)
	}
	if n != len("payload") {
		t.Fatalf("written got %d want %d", n, len("payload"))
	}
	if sink.String() != "payload" {
		t.Fatalf("sink got %q want %q", sink.String(), "payload")
	}
	if buf.String() != "" {
		t.Fatalf("log got %q want empty on success", buf.String())
	}
}

// shortWriter 要求より少ないバイト数しか書けないフェイクio.Writerです。
type shortWriter struct{}

func (shortWriter) Write(p []byte) (int, error) {
	return 0, errors.New("disk full")
}

func TestWriteAndLogError(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug}))

	_, err := obs.WriteAndLog(logger, shortWriter{}, []byte("payload"), "writing response")
	if err == nil {
		t.Fatal("WriteAndLog got nil error want non-nil")
	}
	if !strings.Contains(buf.String(), "writing response") {
		t.Fatalf("log got %q want it to contain context message", buf.String())
	}
	if !strings.Contains(buf.String(), "ERROR") {
		t.Fatalf("log got %q want ERROR level", buf.String())
	}
}
```

- [ ] Step 2: テストが失敗することを確認する

Run:
```bash
cd /Users/yujiokamoto/devs/golang/feedflow-go-htmx && go test ./internal/obs/ -run TestCloseAndLog -v
```
Expected: コンパイルエラーで失敗します。`undefined: obs.CloseAndLog` などと表示されます。

- [ ] Step 3: 最小実装を書く

Create `internal/obs/obs.go`:
```go
// Package obs ログ付きのリソース解放と書き込みのヘルパを提供します。
// エラーを握り潰さずloggerに記録するための共通ユーティリティです。
package obs

import (
	"fmt"
	"io"
	"log/slog"
)

// CloseAndLog cを閉じ、閉じる際のエラーがあればloggerにwarnとして記録します。
// defer obs.CloseAndLog(logger, f, "closing feeds file")の形で使い、deferでのClose漏れと
// エラー握り潰しを同時に避けます。loggerがnilの場合はslog.Defaultを用います。
func CloseAndLog(logger *slog.Logger, c io.Closer, context string) {
	if c == nil {
		return
	}
	if err := c.Close(); err != nil {
		if logger == nil {
			logger = slog.Default()
		}
		logger.Warn(context, slog.String("error", err.Error()))
	}
}

// WriteAndLog wにpを書き込み、書き込みのエラーがあればloggerにerrorとして記録して
// 文脈付きでラップしたエラーを返します。書き込んだバイト数も返します。
// loggerがnilの場合はslog.Defaultを用います。
func WriteAndLog(logger *slog.Logger, w io.Writer, p []byte, context string) (int, error) {
	n, err := w.Write(p)
	if err != nil {
		if logger == nil {
			logger = slog.Default()
		}
		logger.Error(context, slog.String("error", err.Error()))
		return n, fmt.Errorf("%s: %w", context, err)
	}
	return n, nil
}
```

- [ ] Step 4: テストが通ることを確認する

Run:
```bash
cd /Users/yujiokamoto/devs/golang/feedflow-go-htmx && go test ./internal/obs/ -race -v
```
Expected: TestCloseAndLog、TestWriteAndLog、TestWriteAndLogErrorがいずれもPASSします。

- [ ] Step 5: gofmtを適用してコミットする

```bash
cd /Users/yujiokamoto/devs/golang/feedflow-go-htmx && gofmt -w internal/obs/obs.go internal/obs/obs_test.go && git add internal/obs/obs.go internal/obs/obs_test.go && git commit -m "feat: obsのログ付きCloseとWriteヘルパを追加する"
```

---

## Task 2: アトミックJSON書き込みと読み込み

Files:
- Test: `/Users/yujiokamoto/devs/golang/feedflow-go-htmx/internal/store/atomic_test.go`
- Create: `/Users/yujiokamoto/devs/golang/feedflow-go-htmx/internal/store/atomic.go`

設計書セクション7のとおり、変更はtempファイルへ書き込んだのちos.Renameでアトミックに反映します。書き込み途中のクラッシュによる破損を避ける中核処理を、store本体から切り出した独立関数として先に実装します。読み込みは、ファイルが存在しないことを呼び出し側が判別できるようos.IsNotExistで扱えるエラーをそのまま返します。

- [ ] Step 1: 失敗するテストを書く

Create `internal/store/atomic_test.go`:
```go
package store

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

type sample struct {
	Name  string `json:"name"`
	Count int    `json:"count"`
}

func TestWriteJSONAtomicAndReadJSON(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "sample.json")

	want := sample{Name: "feedflow", Count: 42}
	if err := writeJSONAtomic(path, want); err != nil {
		t.Fatalf("writeJSONAtomic returned error: %v", err)
	}

	var got sample
	if err := readJSON(path, &got); err != nil {
		t.Fatalf("readJSON returned error: %v", err)
	}
	if got != want {
		t.Fatalf("round trip got %+v want %+v", got, want)
	}
}

func TestWriteJSONAtomicLeavesNoTempFile(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "sample.json")

	if err := writeJSONAtomic(path, sample{Name: "a", Count: 1}); err != nil {
		t.Fatalf("writeJSONAtomic returned error: %v", err)
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("ReadDir returned error: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("dir entries got %d want 1 (no leftover temp file)", len(entries))
	}
	if entries[0].Name() != "sample.json" {
		t.Fatalf("entry got %q want sample.json", entries[0].Name())
	}
}

func TestWriteJSONAtomicOverwrites(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "sample.json")

	if err := writeJSONAtomic(path, sample{Name: "old", Count: 1}); err != nil {
		t.Fatalf("first write returned error: %v", err)
	}
	if err := writeJSONAtomic(path, sample{Name: "new", Count: 2}); err != nil {
		t.Fatalf("second write returned error: %v", err)
	}

	var got sample
	if err := readJSON(path, &got); err != nil {
		t.Fatalf("readJSON returned error: %v", err)
	}
	if (got != sample{Name: "new", Count: 2}) {
		t.Fatalf("overwrite got %+v want {new 2}", got)
	}
}

func TestWriteJSONAtomicProducesIndentedJSON(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "sample.json")

	if err := writeJSONAtomic(path, sample{Name: "a", Count: 1}); err != nil {
		t.Fatalf("writeJSONAtomic returned error: %v", err)
	}

	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile returned error: %v", err)
	}
	if !json.Valid(raw) {
		t.Fatalf("written bytes are not valid JSON: %q", raw)
	}
	if raw[len(raw)-1] != '\n' {
		t.Fatalf("written JSON does not end with newline: %q", raw)
	}
}

func TestReadJSONMissingFileReturnsNotExist(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "missing.json")

	var got sample
	err := readJSON(path, &got)
	if err == nil {
		t.Fatal("readJSON got nil error want non-nil for missing file")
	}
	if !os.IsNotExist(err) {
		t.Fatalf("readJSON error got %v want os.IsNotExist to be true", err)
	}
}
```

- [ ] Step 2: テストが失敗することを確認する

Run:
```bash
cd /Users/yujiokamoto/devs/golang/feedflow-go-htmx && go test ./internal/store/ -run TestWriteJSONAtomic -v
```
Expected: コンパイルエラーで失敗します。`undefined: writeJSONAtomic` と表示されます。

- [ ] Step 3: 最小実装を書く

Create `internal/store/atomic.go`:
```go
package store

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// filePerm永続化ファイルのパーミッションです。所有者の読み書きのみを許可します。
const filePerm = 0o600

// writeJSONAtomic vをJSONへ整形してpathにアトミックに書き込みます。
// 同一ディレクトリ内の一時ファイルへ書いてからos.Renameで置き換えるため、書き込み途中の
// クラッシュでpathが破損することを避けます。書き込み失敗時は一時ファイルを後始末します。
func writeJSONAtomic(path string, v any) error {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal json for %s: %w", path, err)
	}
	data = append(data, '\n')

	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, filepath.Base(path)+".tmp-*")
	if err != nil {
		return fmt.Errorf("failed to create temp file in %s: %w", dir, err)
	}
	tmpName := tmp.Name()

	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmpName)
		return fmt.Errorf("failed to write temp file %s: %w", tmpName, err)
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmpName)
		return fmt.Errorf("failed to sync temp file %s: %w", tmpName, err)
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpName)
		return fmt.Errorf("failed to close temp file %s: %w", tmpName, err)
	}
	if err := os.Chmod(tmpName, filePerm); err != nil {
		_ = os.Remove(tmpName)
		return fmt.Errorf("failed to chmod temp file %s: %w", tmpName, err)
	}
	if err := os.Rename(tmpName, path); err != nil {
		_ = os.Remove(tmpName)
		return fmt.Errorf("failed to rename temp file to %s: %w", path, err)
	}
	return nil
}

// readJSON pathのJSONをvにデコードします。ファイルが存在しない場合はos.IsNotExistで
// 判別できるエラーをそのまま返し、呼び出し側が既定値へのフォールバックを判断できるようにします。
func readJSON(path string, v any) error {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return err
		}
		return fmt.Errorf("failed to read %s: %w", path, err)
	}
	if err := json.Unmarshal(data, v); err != nil {
		return fmt.Errorf("failed to unmarshal %s: %w", path, err)
	}
	return nil
}
```

補足: writeJSONAtomic内のCloseとRemoveは異常系の後始末であり、すでに別のエラーを返す経路の補助処理のため、obs.CloseAndLogではなく明示的に`_ =`で破棄します。golangci-lintのerrcheck対象になりますが、Phase0の.golangci.ymlでテスト以外の箇所は対象です。ここでの破棄は後続のreturnでより重要なエラーを返すための意図的な無視であり、go vetとstaticcheckは通ります。errcheckで指摘される場合は当該行を`//nolint:errcheck // 異常系の後始末のため主たるエラーを優先する`で個別に許可します。

- [ ] Step 4: テストが通ることを確認する

Run:
```bash
cd /Users/yujiokamoto/devs/golang/feedflow-go-htmx && go test ./internal/store/ -race -v
```
Expected: TestWriteJSONAtomicAndReadJSON、TestWriteJSONAtomicLeavesNoTempFile、TestWriteJSONAtomicOverwrites、TestWriteJSONAtomicProducesIndentedJSON、TestReadJSONMissingFileReturnsNotExistがいずれもPASSします。

- [ ] Step 5: gofmtを適用してコミットする

```bash
cd /Users/yujiokamoto/devs/golang/feedflow-go-htmx && gofmt -w internal/store/atomic.go internal/store/atomic_test.go && git add internal/store/atomic.go internal/store/atomic_test.go && git commit -m "feat: temp+renameのアトミックJSON書き込みと読み込みを追加する"
```

---

## Task 3: Storeのロードとコンストラクタ

Files:
- Test: `/Users/yujiokamoto/devs/golang/feedflow-go-htmx/internal/store/store_test.go`
- Create: `/Users/yujiokamoto/devs/golang/feedflow-go-htmx/internal/store/store.go`

設計書セクション7のとおり、起動時に全件をロードしてメモリに常駐させsync.RWMutexで保護するStoreの骨格を作ります。dataDir配下のitemsディレクトリを作成し、各エンティティのJSONファイルを読み込みます。ファイルが無いときは空のコレクションまたは既定値で初期化します。port.Repositoryの実装はこの後のタスクで段階的に足します。

- [ ] Step 1: 失敗するテストを書く

Create `internal/store/store_test.go`:
```go
package store_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/okamyuji/feedflow-go-htmx/internal/domain"
	"github.com/okamyuji/feedflow-go-htmx/internal/port"
	"github.com/okamyuji/feedflow-go-htmx/internal/store"
)

// assertRepository Storeがport.Repositoryを満たすことをコンパイル時に確認します。
var _ port.Repository = (*store.Store)(nil)

func TestNewCreatesDataDirAndItemsDir(t *testing.T) {
	t.Parallel()

	root := filepath.Join(t.TempDir(), "data")
	s, err := store.New(root)
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}
	if s == nil {
		t.Fatal("New returned nil Store")
	}

	if info, err := os.Stat(root); err != nil || !info.IsDir() {
		t.Fatalf("data dir not created: stat err %v", err)
	}
	if info, err := os.Stat(filepath.Join(root, "items")); err != nil || !info.IsDir() {
		t.Fatalf("items dir not created: stat err %v", err)
	}
}

func TestNewOnEmptyDirReturnsDefaults(t *testing.T) {
	t.Parallel()

	s, err := store.New(filepath.Join(t.TempDir(), "data"))
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}

	feeds, err := s.Feeds()
	if err != nil {
		t.Fatalf("Feeds returned error: %v", err)
	}
	if len(feeds) != 0 {
		t.Fatalf("feeds got %d want 0 on empty dir", len(feeds))
	}

	settings, err := s.Settings()
	if err != nil {
		t.Fatalf("Settings returned error: %v", err)
	}
	if settings != domain.DefaultSettings() {
		t.Fatalf("settings got %+v want DefaultSettings on empty dir", settings)
	}

	user, err := s.User()
	if err != nil {
		t.Fatalf("User returned error: %v", err)
	}
	if user.IsRegistered() {
		t.Fatalf("user got registered %+v want zero value on empty dir", user)
	}
}

func TestNewLoadsExistingFeeds(t *testing.T) {
	t.Parallel()

	root := filepath.Join(t.TempDir(), "data")

	// 1つ目のStoreで1件保存し、2つ目のStoreでロードできることを確認します。
	s1, err := store.New(root)
	if err != nil {
		t.Fatalf("first New returned error: %v", err)
	}
	want := domain.Feed{ID: "f1", FeedURL: "https://example.com/rss", Title: "Example"}
	if err := s1.SaveFeed(want); err != nil {
		t.Fatalf("SaveFeed returned error: %v", err)
	}

	s2, err := store.New(root)
	if err != nil {
		t.Fatalf("second New returned error: %v", err)
	}
	got, err := s2.Feed("f1")
	if err != nil {
		t.Fatalf("Feed returned error: %v", err)
	}
	if got.FeedURL != want.FeedURL || got.Title != want.Title {
		t.Fatalf("loaded feed got %+v want %+v", got, want)
	}
}
```

- [ ] Step 2: テストが失敗することを確認する

Run:
```bash
cd /Users/yujiokamoto/devs/golang/feedflow-go-htmx && go test ./internal/store/ -run TestNew -v
```
Expected: コンパイルエラーで失敗します。`undefined: store.New` や `undefined: store.Store` と表示されます。

- [ ] Step 3: 最小実装を書く

Create `internal/store/store.go`:
```go
// Package store メモリ常駐とアトミックJSON永続化でport.Repositoryを実装します。
// 起動時にdataディレクトリの全JSONをロードし、sync.RWMutexで保護したメモリ状態を保ちます。
// 変更はtempファイルへ書いてからos.Renameで原子的に反映します。設計書のセクション7に対応します。
package store

import (
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sync"

	"github.com/okamyuji/feedflow-go-htmx/internal/domain"
)

// ErrNotFound 指定IDのエンティティが見つからないことを表すsentinel errorです。
// 呼び出し側はerrors.Is(err, store.ErrNotFound)で判別できます。
var ErrNotFound = errors.New("store: entity not found")

// dirPerm永続化ディレクトリのパーミッションです。所有者のみアクセスを許可します。
const dirPerm = 0o700

// ファイル名の定数です。dataディレクトリ直下に配置します。
const (
	feedsFile      = "feeds.json"
	categoriesFile = "categories.json"
	boardsFile     = "boards.json"
	filtersFile    = "filters.json"
	settingsFile   = "settings.json"
	userFile       = "user.json"
	itemsDir       = "items"
)

// Store メモリ常駐の永続化集約です。全エンティティをメモリに保持しsync.RWMutexで保護します。
// port.Repositoryを実装します。
type Store struct {
	dataDir string
	logger  *slog.Logger

	mu         sync.RWMutex
	feeds      []domain.Feed
	categories []domain.Category
	boards     []domain.Board
	filters    []domain.MuteFilter
	items      map[string][]domain.Item
	settings   domain.Settings
	user       domain.User
}

// New dataDirを永続化ディレクトリとするStoreを生成し、全件をロードします。
// dataDirと配下のitemsディレクトリが無ければ作成します。
func New(dataDir string) (*Store, error) {
	s := &Store{
		dataDir:  dataDir,
		logger:   slog.Default(),
		items:    make(map[string][]domain.Item),
		settings: domain.DefaultSettings(),
	}

	if err := os.MkdirAll(filepath.Join(dataDir, itemsDir), dirPerm); err != nil {
		return nil, fmt.Errorf("failed to create data directories under %s: %w", dataDir, err)
	}
	if err := s.load(); err != nil {
		return nil, fmt.Errorf("failed to load store from %s: %w", dataDir, err)
	}
	return s, nil
}

// path dataディレクトリ直下のファイルへの絶対パスを返します。
func (s *Store) path(name string) string {
	return filepath.Join(s.dataDir, name)
}

// itemsPath指定フィードの記事ファイルへのパスを返します。
func (s *Store) itemsPath(feedID string) string {
	return filepath.Join(s.dataDir, itemsDir, feedID+".json")
}

// load 全エンティティのJSONをメモリに読み込みます。ファイルが無い項目は空または既定値のままにします。
func (s *Store) load() error {
	if err := s.loadSlice(feedsFile, &s.feeds); err != nil {
		return err
	}
	if err := s.loadSlice(categoriesFile, &s.categories); err != nil {
		return err
	}
	if err := s.loadSlice(boardsFile, &s.boards); err != nil {
		return err
	}
	if err := s.loadSlice(filtersFile, &s.filters); err != nil {
		return err
	}
	if err := s.loadSettings(); err != nil {
		return err
	}
	if err := s.loadUser(); err != nil {
		return err
	}
	return s.loadItems()
}

// loadSlice nameのファイルをdstにデコードします。ファイルが無い場合は何もしません。
func (s *Store) loadSlice(name string, dst any) error {
	err := readJSON(s.path(name), dst)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to load %s: %w", name, err)
	}
	return nil
}

// loadSettings settings.jsonを読み込みます。ファイルが無い場合は既定値を保ちます。
func (s *Store) loadSettings() error {
	var loaded domain.Settings
	err := readJSON(s.path(settingsFile), &loaded)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("failed to load %s: %w", settingsFile, err)
	}
	s.settings = loaded
	return nil
}

// loadUser user.jsonを読み込みます。ファイルが無い場合はゼロ値のままにします。
func (s *Store) loadUser() error {
	var loaded domain.User
	err := readJSON(s.path(userFile), &loaded)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("failed to load %s: %w", userFile, err)
	}
	s.user = loaded
	return nil
}

// loadItems itemsディレクトリ配下の全JSONを読み込み、ファイル名のフィードIDをキーに保持します。
func (s *Store) loadItems() error {
	dir := filepath.Join(s.dataDir, itemsDir)
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("failed to read items dir %s: %w", dir, err)
	}
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || filepath.Ext(name) != ".json" {
			continue
		}
		feedID := name[:len(name)-len(".json")]
		var items []domain.Item
		if err := readJSON(filepath.Join(dir, name), &items); err != nil {
			return fmt.Errorf("failed to load items file %s: %w", name, err)
		}
		s.items[feedID] = items
	}
	return nil
}
```

- [ ] Step 4: テストが通ることを確認する

Run:
```bash
cd /Users/yujiokamoto/devs/golang/feedflow-go-htmx && go test ./internal/store/ -race -run TestNew -v
```
Expected: TestNewCreatesDataDirAndItemsDir、TestNewOnEmptyDirReturnsDefaults、TestNewLoadsExistingFeedsがいずれもPASSします。なおこの時点ではSaveFeedやFeedやFeedsやSettingsやUserがまだ未実装のため、コンパイルが通らずテスト全体が失敗します。次のタスクでこれらを実装します。

補足: 上記テストはSaveFeed、Feed、Feeds、Settings、Userを参照します。コンパイルを通すため、本タスクのコミットは次のTask4とTask5まで分割せず、Task5完了時にまとめて1回コミットする方針も取れますが、本計画では各タスクで対象メソッドを足しながら進めます。Task3単独ではTestNewOnEmptyDirReturnsDefaultsとTestNewLoadsExistingFeedsはコンパイルできないため、Task3のStep4ではビルドのみ確認し、テスト実行はTask5まで持ち越します。Step4を次のRunに置き換えます。

Run:
```bash
cd /Users/yujiokamoto/devs/golang/feedflow-go-htmx && go build ./internal/store/
```
Expected: store.goのビルドはエラーなく完了します。store_test.goは未実装メソッドを参照するためテスト実行はTask5で行います。

- [ ] Step 5: gofmtを適用してコミットする

```bash
cd /Users/yujiokamoto/devs/golang/feedflow-go-htmx && gofmt -w internal/store/store.go internal/store/store_test.go && git add internal/store/store.go internal/store/store_test.go && git commit -m "feat: Storeのロードとコンストラクタを追加する"
```

---

## Task 4: フィードとカテゴリの取得と保存と削除

Files:
- Create: `/Users/yujiokamoto/devs/golang/feedflow-go-htmx/internal/store/feed.go`
- Edit: `/Users/yujiokamoto/devs/golang/feedflow-go-htmx/internal/store/store_test.go`

port.RepositoryのFeeds、Feed、SaveFeed、DeleteFeed、Categories、SaveCategory、DeleteCategoryを実装します。SaveFeedとSaveCategoryは同一IDがあれば更新し、無ければ追加します。DeleteFeedはフィード本体に加えて該当する記事ファイルとメモリ上の記事も削除します。読み取りは内部状態を共有しない新規スライスのコピーを返します。書き込みはメモリ更新とJSONファイル書き出しを同一ロック区間で行い、ファイル書き出しに失敗したらメモリを巻き戻します。

- [ ] Step 1: 失敗するテストをstore_test.goへ追記する

Add to `internal/store/store_test.go`:
```go

func TestSaveFeedAddsAndUpdates(t *testing.T) {
	t.Parallel()

	s, err := store.New(filepath.Join(t.TempDir(), "data"))
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}

	if err := s.SaveFeed(domain.Feed{ID: "f1", Title: "First"}); err != nil {
		t.Fatalf("SaveFeed add returned error: %v", err)
	}
	if err := s.SaveFeed(domain.Feed{ID: "f2", Title: "Second"}); err != nil {
		t.Fatalf("SaveFeed add returned error: %v", err)
	}
	if err := s.SaveFeed(domain.Feed{ID: "f1", Title: "First Updated"}); err != nil {
		t.Fatalf("SaveFeed update returned error: %v", err)
	}

	feeds, err := s.Feeds()
	if err != nil {
		t.Fatalf("Feeds returned error: %v", err)
	}
	if len(feeds) != 2 {
		t.Fatalf("feeds got %d want 2 (update must not duplicate)", len(feeds))
	}

	got, err := s.Feed("f1")
	if err != nil {
		t.Fatalf("Feed returned error: %v", err)
	}
	if got.Title != "First Updated" {
		t.Fatalf("feed title got %q want First Updated", got.Title)
	}
}

func TestFeedNotFound(t *testing.T) {
	t.Parallel()

	s, err := store.New(filepath.Join(t.TempDir(), "data"))
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}

	_, err = s.Feed("missing")
	if !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("Feed error got %v want errors.Is ErrNotFound", err)
	}
}

func TestFeedsReturnsCopy(t *testing.T) {
	t.Parallel()

	s, err := store.New(filepath.Join(t.TempDir(), "data"))
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}
	if err := s.SaveFeed(domain.Feed{ID: "f1", Title: "Original"}); err != nil {
		t.Fatalf("SaveFeed returned error: %v", err)
	}

	feeds, err := s.Feeds()
	if err != nil {
		t.Fatalf("Feeds returned error: %v", err)
	}
	feeds[0].Title = "Mutated"

	got, err := s.Feed("f1")
	if err != nil {
		t.Fatalf("Feed returned error: %v", err)
	}
	if got.Title != "Original" {
		t.Fatalf("internal state mutated via returned slice: got %q want Original", got.Title)
	}
}

func TestDeleteFeedRemovesFeedAndItems(t *testing.T) {
	t.Parallel()

	root := filepath.Join(t.TempDir(), "data")
	s, err := store.New(root)
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}
	if err := s.SaveFeed(domain.Feed{ID: "f1", Title: "First"}); err != nil {
		t.Fatalf("SaveFeed returned error: %v", err)
	}
	if err := s.SaveItems("f1", []domain.Item{{ID: "i1", FeedID: "f1", Title: "Article"}}); err != nil {
		t.Fatalf("SaveItems returned error: %v", err)
	}

	if err := s.DeleteFeed("f1"); err != nil {
		t.Fatalf("DeleteFeed returned error: %v", err)
	}

	if _, err := s.Feed("f1"); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("Feed after delete error got %v want ErrNotFound", err)
	}
	items, err := s.Items("f1")
	if err != nil {
		t.Fatalf("Items returned error: %v", err)
	}
	if len(items) != 0 {
		t.Fatalf("items after delete got %d want 0", len(items))
	}
	if _, statErr := os.Stat(filepath.Join(root, "items", "f1.json")); !os.IsNotExist(statErr) {
		t.Fatalf("items file still present after DeleteFeed: stat err %v", statErr)
	}
}

func TestDeleteFeedMissingReturnsNotFound(t *testing.T) {
	t.Parallel()

	s, err := store.New(filepath.Join(t.TempDir(), "data"))
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}
	if err := s.DeleteFeed("missing"); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("DeleteFeed error got %v want ErrNotFound", err)
	}
}

func TestSaveAndDeleteCategory(t *testing.T) {
	t.Parallel()

	s, err := store.New(filepath.Join(t.TempDir(), "data"))
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}
	if err := s.SaveCategory(domain.Category{ID: "c1", Name: "Tech", Order: 1}); err != nil {
		t.Fatalf("SaveCategory returned error: %v", err)
	}
	if err := s.SaveCategory(domain.Category{ID: "c1", Name: "Technology", Order: 1}); err != nil {
		t.Fatalf("SaveCategory update returned error: %v", err)
	}

	cats, err := s.Categories()
	if err != nil {
		t.Fatalf("Categories returned error: %v", err)
	}
	if len(cats) != 1 || cats[0].Name != "Technology" {
		t.Fatalf("categories got %+v want one Technology", cats)
	}

	if err := s.DeleteCategory("c1"); err != nil {
		t.Fatalf("DeleteCategory returned error: %v", err)
	}
	cats, err = s.Categories()
	if err != nil {
		t.Fatalf("Categories returned error: %v", err)
	}
	if len(cats) != 0 {
		t.Fatalf("categories after delete got %d want 0", len(cats))
	}
}

func TestDeleteCategoryMissingReturnsNotFound(t *testing.T) {
	t.Parallel()

	s, err := store.New(filepath.Join(t.TempDir(), "data"))
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}
	if err := s.DeleteCategory("missing"); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("DeleteCategory error got %v want ErrNotFound", err)
	}
}
```

import節に`"errors"`を追加する必要があります。store_test.goの先頭importへ`"errors"`を加えます。

- [ ] Step 2: テストが失敗することを確認する

Run:
```bash
cd /Users/yujiokamoto/devs/golang/feedflow-go-htmx && go test ./internal/store/ -run "TestSaveFeed|TestFeed|TestDeleteFeed|TestCategory|TestSaveAndDelete" -v
```
Expected: コンパイルエラーで失敗します。`s.SaveFeed undefined` などと表示されます。

- [ ] Step 3: 最小実装を書く

Create `internal/store/feed.go`:
```go
package store

import (
	"fmt"
	"os"
	"slices"

	"github.com/okamyuji/feedflow-go-htmx/internal/domain"
)

// Feeds登録済みの全フィードを内部状態と共有しないコピーで返します。
func (s *Store) Feeds() ([]domain.Feed, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return slices.Clone(s.feeds), nil
}

// Feed 指定IDのフィードを返します。見つからない場合はErrNotFoundをラップして返します。
func (s *Store) Feed(id string) (domain.Feed, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, f := range s.feeds {
		if f.ID == id {
			return f, nil
		}
	}
	return domain.Feed{}, fmt.Errorf("feed %q: %w", id, ErrNotFound)
}

// SaveFeed フィードを新規追加または更新し、feeds.jsonをアトミックに書き出します。
func (s *Store) SaveFeed(feed domain.Feed) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	prev := slices.Clone(s.feeds)
	idx := slices.IndexFunc(s.feeds, func(f domain.Feed) bool { return f.ID == feed.ID })
	if idx >= 0 {
		s.feeds[idx] = feed
	} else {
		s.feeds = append(s.feeds, feed)
	}

	if err := writeJSONAtomic(s.path(feedsFile), s.feeds); err != nil {
		s.feeds = prev
		return fmt.Errorf("failed to save feed %q: %w", feed.ID, err)
	}
	return nil
}

// DeleteFeed 指定IDのフィードと、それに属する全記事を削除します。
func (s *Store) DeleteFeed(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	idx := slices.IndexFunc(s.feeds, func(f domain.Feed) bool { return f.ID == id })
	if idx < 0 {
		return fmt.Errorf("feed %q: %w", id, ErrNotFound)
	}

	prev := slices.Clone(s.feeds)
	s.feeds = slices.Delete(s.feeds, idx, idx+1)
	if err := writeJSONAtomic(s.path(feedsFile), s.feeds); err != nil {
		s.feeds = prev
		return fmt.Errorf("failed to delete feed %q: %w", id, err)
	}

	// 記事ファイルとメモリ上の記事も削除します。ファイルが無い場合は許容します。
	if err := os.Remove(s.itemsPath(id)); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to remove items file for feed %q: %w", id, err)
	}
	delete(s.items, id)
	return nil
}

// Categories全カテゴリを内部状態と共有しないコピーで返します。
func (s *Store) Categories() ([]domain.Category, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return slices.Clone(s.categories), nil
}

// SaveCategory カテゴリを新規追加または更新し、categories.jsonをアトミックに書き出します。
func (s *Store) SaveCategory(category domain.Category) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	prev := slices.Clone(s.categories)
	idx := slices.IndexFunc(s.categories, func(c domain.Category) bool { return c.ID == category.ID })
	if idx >= 0 {
		s.categories[idx] = category
	} else {
		s.categories = append(s.categories, category)
	}

	if err := writeJSONAtomic(s.path(categoriesFile), s.categories); err != nil {
		s.categories = prev
		return fmt.Errorf("failed to save category %q: %w", category.ID, err)
	}
	return nil
}

// DeleteCategory 指定IDのカテゴリを削除し、categories.jsonをアトミックに書き出します。
func (s *Store) DeleteCategory(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	idx := slices.IndexFunc(s.categories, func(c domain.Category) bool { return c.ID == id })
	if idx < 0 {
		return fmt.Errorf("category %q: %w", id, ErrNotFound)
	}

	prev := slices.Clone(s.categories)
	s.categories = slices.Delete(s.categories, idx, idx+1)
	if err := writeJSONAtomic(s.path(categoriesFile), s.categories); err != nil {
		s.categories = prev
		return fmt.Errorf("failed to delete category %q: %w", id, err)
	}
	return nil
}
```

補足: DeleteFeedでfeeds.jsonの書き出し成功後にos.Removeが失敗した場合、フィード本体は削除済みで記事ファイルだけが残る不整合になり得ます。記事ファイルはItemsの読み取り時にメモリ側のdeleteで参照されなくなるため画面上は消え、次回起動時に孤児ファイルが残るのみです。孤児ファイルの掃除はPhase4の保持ポリシー適用とは別軸のため、ここではエラーを返して呼び出し側に通知するに留めます。この設計は設計書セクション7のアトミック書き込みの範囲内であり、SQLを使わない単一ユーザー運用で許容できます。

- [ ] Step 4: テストが通ることを確認する

Run:
```bash
cd /Users/yujiokamoto/devs/golang/feedflow-go-htmx && go test ./internal/store/ -race -run "TestSaveFeed|TestFeed|TestDeleteFeed|TestCategory|TestSaveAndDelete" -v
```
Expected: TestSaveFeedAddsAndUpdates、TestFeedNotFound、TestFeedsReturnsCopy、TestDeleteFeedRemovesFeedAndItems、TestDeleteFeedMissingReturnsNotFound、TestSaveAndDeleteCategory、TestDeleteCategoryMissingReturnsNotFoundがいずれもPASSします。SaveItemsとItemsはまだ未実装のため、TestDeleteFeedRemovesFeedAndItemsはコンパイルできず失敗する可能性があります。その場合は次のTask5まで該当テストの実行を持ち越し、本タスクではフィードとカテゴリのテストのみ確認します。

Run(SaveItems未実装でコンパイルが通らない場合):
```bash
cd /Users/yujiokamoto/devs/golang/feedflow-go-htmx && go test ./internal/store/ -race -run "TestSaveFeedAddsAndUpdates|TestFeedNotFound|TestFeedsReturnsCopy|TestSaveAndDeleteCategory|TestDeleteCategoryMissingReturnsNotFound" -v
```
Expected: 記載した5件のフィードとカテゴリのテストがPASSします。記事関連はTask5で確認します。

- [ ] Step 5: gofmtを適用してコミットする

```bash
cd /Users/yujiokamoto/devs/golang/feedflow-go-htmx && gofmt -w internal/store/feed.go internal/store/store_test.go && git add internal/store/feed.go internal/store/store_test.go && git commit -m "feat: フィードとカテゴリの取得と保存と削除をStoreに追加する"
```

---

## Task 5: 記事の取得と保存

Files:
- Create: `/Users/yujiokamoto/devs/golang/feedflow-go-htmx/internal/store/item.go`
- Edit: `/Users/yujiokamoto/devs/golang/feedflow-go-htmx/internal/store/store_test.go`

port.RepositoryのItemsとSaveItemsを実装します。記事はFeedID単位でdata/items/フィードID.jsonに分割保存します。SaveItemsは指定フィードの記事群を丸ごと置き換え、対応する1ファイルだけをアトミックに書き出します。Itemsは内部状態と共有しないコピーを返し、未登録フィードには空スライスを返します。

- [ ] Step 1: 失敗するテストをstore_test.goへ追記する

Add to `internal/store/store_test.go`:
```go

func TestSaveItemsAndItems(t *testing.T) {
	t.Parallel()

	root := filepath.Join(t.TempDir(), "data")
	s, err := store.New(root)
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}

	want := []domain.Item{
		{ID: "i1", FeedID: "f1", Title: "First"},
		{ID: "i2", FeedID: "f1", Title: "Second"},
	}
	if err := s.SaveItems("f1", want); err != nil {
		t.Fatalf("SaveItems returned error: %v", err)
	}

	got, err := s.Items("f1")
	if err != nil {
		t.Fatalf("Items returned error: %v", err)
	}
	if len(got) != 2 || got[0].ID != "i1" || got[1].ID != "i2" {
		t.Fatalf("items got %+v want i1,i2", got)
	}

	// ファイルがitemsディレクトリに作られていることを確認します。
	if _, statErr := os.Stat(filepath.Join(root, "items", "f1.json")); statErr != nil {
		t.Fatalf("items file not created: %v", statErr)
	}
}

func TestSaveItemsReplaces(t *testing.T) {
	t.Parallel()

	s, err := store.New(filepath.Join(t.TempDir(), "data"))
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}
	if err := s.SaveItems("f1", []domain.Item{{ID: "i1", FeedID: "f1"}, {ID: "i2", FeedID: "f1"}}); err != nil {
		t.Fatalf("first SaveItems returned error: %v", err)
	}
	if err := s.SaveItems("f1", []domain.Item{{ID: "i3", FeedID: "f1"}}); err != nil {
		t.Fatalf("second SaveItems returned error: %v", err)
	}

	got, err := s.Items("f1")
	if err != nil {
		t.Fatalf("Items returned error: %v", err)
	}
	if len(got) != 1 || got[0].ID != "i3" {
		t.Fatalf("items after replace got %+v want only i3", got)
	}
}

func TestItemsUnknownFeedReturnsEmpty(t *testing.T) {
	t.Parallel()

	s, err := store.New(filepath.Join(t.TempDir(), "data"))
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}
	got, err := s.Items("nope")
	if err != nil {
		t.Fatalf("Items returned error: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("items got %d want 0 for unknown feed", len(got))
	}
}

func TestItemsReturnsCopy(t *testing.T) {
	t.Parallel()

	s, err := store.New(filepath.Join(t.TempDir(), "data"))
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}
	if err := s.SaveItems("f1", []domain.Item{{ID: "i1", FeedID: "f1", Title: "Original"}}); err != nil {
		t.Fatalf("SaveItems returned error: %v", err)
	}

	got, err := s.Items("f1")
	if err != nil {
		t.Fatalf("Items returned error: %v", err)
	}
	got[0].Title = "Mutated"

	again, err := s.Items("f1")
	if err != nil {
		t.Fatalf("Items returned error: %v", err)
	}
	if again[0].Title != "Original" {
		t.Fatalf("internal state mutated via returned slice: got %q want Original", again[0].Title)
	}
}

func TestItemsPersistAcrossReload(t *testing.T) {
	t.Parallel()

	root := filepath.Join(t.TempDir(), "data")
	s1, err := store.New(root)
	if err != nil {
		t.Fatalf("first New returned error: %v", err)
	}
	if err := s1.SaveItems("f1", []domain.Item{{ID: "i1", FeedID: "f1", Title: "Persisted"}}); err != nil {
		t.Fatalf("SaveItems returned error: %v", err)
	}

	s2, err := store.New(root)
	if err != nil {
		t.Fatalf("second New returned error: %v", err)
	}
	got, err := s2.Items("f1")
	if err != nil {
		t.Fatalf("Items returned error: %v", err)
	}
	if len(got) != 1 || got[0].Title != "Persisted" {
		t.Fatalf("reloaded items got %+v want one Persisted", got)
	}
}
```

- [ ] Step 2: テストが失敗することを確認する

Run:
```bash
cd /Users/yujiokamoto/devs/golang/feedflow-go-htmx && go test ./internal/store/ -run "TestSaveItems|TestItems" -v
```
Expected: コンパイルエラーで失敗します。`s.SaveItems undefined` と表示されます。

- [ ] Step 3: 最小実装を書く

Create `internal/store/item.go`:
```go
package store

import (
	"fmt"
	"slices"

	"github.com/okamyuji/feedflow-go-htmx/internal/domain"
)

// Items指定フィードの全記事を内部状態と共有しないコピーで返します。
// 未登録のフィードには空スライスを返します。
func (s *Store) Items(feedID string) ([]domain.Item, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	items, ok := s.items[feedID]
	if !ok {
		return []domain.Item{}, nil
	}
	return slices.Clone(items), nil
}

// SaveItems指定フィードの記事群をまとめて保存し、既存の記事群を置き換えます。
// 対応するitems/フィードID.jsonをアトミックに書き出します。
func (s *Store) SaveItems(feedID string, items []domain.Item) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	prev, had := s.items[feedID]
	s.items[feedID] = slices.Clone(items)

	if err := writeJSONAtomic(s.itemsPath(feedID), s.items[feedID]); err != nil {
		if had {
			s.items[feedID] = prev
		} else {
			delete(s.items, feedID)
		}
		return fmt.Errorf("failed to save items for feed %q: %w", feedID, err)
	}
	return nil
}
```

- [ ] Step 4: テストが通ることを確認する

Run:
```bash
cd /Users/yujiokamoto/devs/golang/feedflow-go-htmx && go test ./internal/store/ -race -v
```
Expected: Task3からTask5までの全テスト(TestNew系、TestSaveFeed系、TestDeleteFeed系、TestCategory系、TestSaveItems系、TestItems系)がいずれもPASSします。Task4で持ち越したTestDeleteFeedRemovesFeedAndItemsもここでPASSします。

- [ ] Step 5: gofmtを適用してコミットする

```bash
cd /Users/yujiokamoto/devs/golang/feedflow-go-htmx && gofmt -w internal/store/item.go internal/store/store_test.go && git add internal/store/item.go internal/store/store_test.go && git commit -m "feat: 記事の取得と保存をStoreに追加する"
```

---

## Task 6: ボードとフィルタの取得と保存と削除

Files:
- Create: `/Users/yujiokamoto/devs/golang/feedflow-go-htmx/internal/store/board.go`
- Edit: `/Users/yujiokamoto/devs/golang/feedflow-go-htmx/internal/store/store_test.go`

port.RepositoryのBoards、SaveBoard、DeleteBoard、Filters、SaveFilter、DeleteFilterを実装します。フィードやカテゴリと同じ追加更新の規約に従い、それぞれboards.jsonとfilters.jsonをアトミックに書き出します。

- [ ] Step 1: 失敗するテストをstore_test.goへ追記する

Add to `internal/store/store_test.go`:
```go

func TestSaveAndDeleteBoard(t *testing.T) {
	t.Parallel()

	s, err := store.New(filepath.Join(t.TempDir(), "data"))
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}
	if err := s.SaveBoard(domain.Board{ID: "b1", Name: "Reading", Description: "later"}); err != nil {
		t.Fatalf("SaveBoard returned error: %v", err)
	}
	if err := s.SaveBoard(domain.Board{ID: "b1", Name: "Reading List", Description: "later"}); err != nil {
		t.Fatalf("SaveBoard update returned error: %v", err)
	}

	boards, err := s.Boards()
	if err != nil {
		t.Fatalf("Boards returned error: %v", err)
	}
	if len(boards) != 1 || boards[0].Name != "Reading List" {
		t.Fatalf("boards got %+v want one Reading List", boards)
	}

	if err := s.DeleteBoard("b1"); err != nil {
		t.Fatalf("DeleteBoard returned error: %v", err)
	}
	boards, err = s.Boards()
	if err != nil {
		t.Fatalf("Boards returned error: %v", err)
	}
	if len(boards) != 0 {
		t.Fatalf("boards after delete got %d want 0", len(boards))
	}
}

func TestDeleteBoardMissingReturnsNotFound(t *testing.T) {
	t.Parallel()

	s, err := store.New(filepath.Join(t.TempDir(), "data"))
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}
	if err := s.DeleteBoard("missing"); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("DeleteBoard error got %v want ErrNotFound", err)
	}
}

func TestSaveAndDeleteFilter(t *testing.T) {
	t.Parallel()

	s, err := store.New(filepath.Join(t.TempDir(), "data"))
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}
	if err := s.SaveFilter(domain.MuteFilter{ID: "m1", Keyword: "spam", Scope: domain.MuteScopeGlobal}); err != nil {
		t.Fatalf("SaveFilter returned error: %v", err)
	}
	if err := s.SaveFilter(domain.MuteFilter{ID: "m1", Keyword: "promo", Scope: domain.MuteScopeGlobal}); err != nil {
		t.Fatalf("SaveFilter update returned error: %v", err)
	}

	filters, err := s.Filters()
	if err != nil {
		t.Fatalf("Filters returned error: %v", err)
	}
	if len(filters) != 1 || filters[0].Keyword != "promo" {
		t.Fatalf("filters got %+v want one promo", filters)
	}

	if err := s.DeleteFilter("m1"); err != nil {
		t.Fatalf("DeleteFilter returned error: %v", err)
	}
	filters, err = s.Filters()
	if err != nil {
		t.Fatalf("Filters returned error: %v", err)
	}
	if len(filters) != 0 {
		t.Fatalf("filters after delete got %d want 0", len(filters))
	}
}

func TestDeleteFilterMissingReturnsNotFound(t *testing.T) {
	t.Parallel()

	s, err := store.New(filepath.Join(t.TempDir(), "data"))
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}
	if err := s.DeleteFilter("missing"); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("DeleteFilter error got %v want ErrNotFound", err)
	}
}
```

- [ ] Step 2: テストが失敗することを確認する

Run:
```bash
cd /Users/yujiokamoto/devs/golang/feedflow-go-htmx && go test ./internal/store/ -run "TestSaveAndDeleteBoard|TestDeleteBoard|TestSaveAndDeleteFilter|TestDeleteFilter" -v
```
Expected: コンパイルエラーで失敗します。`s.SaveBoard undefined` と表示されます。

- [ ] Step 3: 最小実装を書く

Create `internal/store/board.go`:
```go
package store

import (
	"fmt"
	"slices"

	"github.com/okamyuji/feedflow-go-htmx/internal/domain"
)

// Boards全ボードを内部状態と共有しないコピーで返します。
func (s *Store) Boards() ([]domain.Board, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return slices.Clone(s.boards), nil
}

// SaveBoard ボードを新規追加または更新し、boards.jsonをアトミックに書き出します。
func (s *Store) SaveBoard(board domain.Board) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	prev := slices.Clone(s.boards)
	idx := slices.IndexFunc(s.boards, func(b domain.Board) bool { return b.ID == board.ID })
	if idx >= 0 {
		s.boards[idx] = board
	} else {
		s.boards = append(s.boards, board)
	}

	if err := writeJSONAtomic(s.path(boardsFile), s.boards); err != nil {
		s.boards = prev
		return fmt.Errorf("failed to save board %q: %w", board.ID, err)
	}
	return nil
}

// DeleteBoard 指定IDのボードを削除し、boards.jsonをアトミックに書き出します。
func (s *Store) DeleteBoard(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	idx := slices.IndexFunc(s.boards, func(b domain.Board) bool { return b.ID == id })
	if idx < 0 {
		return fmt.Errorf("board %q: %w", id, ErrNotFound)
	}

	prev := slices.Clone(s.boards)
	s.boards = slices.Delete(s.boards, idx, idx+1)
	if err := writeJSONAtomic(s.path(boardsFile), s.boards); err != nil {
		s.boards = prev
		return fmt.Errorf("failed to delete board %q: %w", id, err)
	}
	return nil
}

// Filters全ミュートフィルタを内部状態と共有しないコピーで返します。
func (s *Store) Filters() ([]domain.MuteFilter, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return slices.Clone(s.filters), nil
}

// SaveFilter ミュートフィルタを新規追加または更新し、filters.jsonをアトミックに書き出します。
func (s *Store) SaveFilter(filter domain.MuteFilter) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	prev := slices.Clone(s.filters)
	idx := slices.IndexFunc(s.filters, func(m domain.MuteFilter) bool { return m.ID == filter.ID })
	if idx >= 0 {
		s.filters[idx] = filter
	} else {
		s.filters = append(s.filters, filter)
	}

	if err := writeJSONAtomic(s.path(filtersFile), s.filters); err != nil {
		s.filters = prev
		return fmt.Errorf("failed to save filter %q: %w", filter.ID, err)
	}
	return nil
}

// DeleteFilter 指定IDのミュートフィルタを削除し、filters.jsonをアトミックに書き出します。
func (s *Store) DeleteFilter(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	idx := slices.IndexFunc(s.filters, func(m domain.MuteFilter) bool { return m.ID == id })
	if idx < 0 {
		return fmt.Errorf("filter %q: %w", id, ErrNotFound)
	}

	prev := slices.Clone(s.filters)
	s.filters = slices.Delete(s.filters, idx, idx+1)
	if err := writeJSONAtomic(s.path(filtersFile), s.filters); err != nil {
		s.filters = prev
		return fmt.Errorf("failed to delete filter %q: %w", id, err)
	}
	return nil
}
```

- [ ] Step 4: テストが通ることを確認する

Run:
```bash
cd /Users/yujiokamoto/devs/golang/feedflow-go-htmx && go test ./internal/store/ -race -run "TestSaveAndDeleteBoard|TestDeleteBoard|TestSaveAndDeleteFilter|TestDeleteFilter" -v
```
Expected: TestSaveAndDeleteBoard、TestDeleteBoardMissingReturnsNotFound、TestSaveAndDeleteFilter、TestDeleteFilterMissingReturnsNotFoundがいずれもPASSします。

- [ ] Step 5: gofmtを適用してコミットする

```bash
cd /Users/yujiokamoto/devs/golang/feedflow-go-htmx && gofmt -w internal/store/board.go internal/store/store_test.go && git add internal/store/board.go internal/store/store_test.go && git commit -m "feat: ボードとフィルタの取得と保存と削除をStoreに追加する"
```

---

## Task 7: 設定とユーザーの取得と保存

Files:
- Create: `/Users/yujiokamoto/devs/golang/feedflow-go-htmx/internal/store/settings.go`
- Edit: `/Users/yujiokamoto/devs/golang/feedflow-go-htmx/internal/store/store_test.go`

port.RepositoryのSettings、SaveSettings、User、SaveUserを実装します。Settingsはsettings.jsonをアトミックに書き出し、ロード未済の初期状態ではdomain.DefaultSettingsを返します。Userは単一レコードのためuser.jsonに保存し、未登録時はゼロ値を返します。

- [ ] Step 1: 失敗するテストをstore_test.goへ追記する

Add to `internal/store/store_test.go`:
```go

func TestSaveAndGetSettings(t *testing.T) {
	t.Parallel()

	root := filepath.Join(t.TempDir(), "data")
	s, err := store.New(root)
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}

	want := domain.Settings{
		PollInterval:      domain.Poll1Hour,
		MaxItems:          500,
		ReadRetentionDays: 60,
		Theme:             domain.ThemeLight,
		DefaultView:       domain.ViewMagazine,
	}
	if err := s.SaveSettings(want); err != nil {
		t.Fatalf("SaveSettings returned error: %v", err)
	}

	got, err := s.Settings()
	if err != nil {
		t.Fatalf("Settings returned error: %v", err)
	}
	if got != want {
		t.Fatalf("settings got %+v want %+v", got, want)
	}

	// 再ロードして永続化を確認します。
	s2, err := store.New(root)
	if err != nil {
		t.Fatalf("reload New returned error: %v", err)
	}
	reloaded, err := s2.Settings()
	if err != nil {
		t.Fatalf("reload Settings returned error: %v", err)
	}
	if reloaded != want {
		t.Fatalf("reloaded settings got %+v want %+v", reloaded, want)
	}
}

func TestSaveAndGetUser(t *testing.T) {
	t.Parallel()

	root := filepath.Join(t.TempDir(), "data")
	s, err := store.New(root)
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}

	want := domain.User{Username: "owner", PasswordHash: "scrypt$hash"}
	if err := s.SaveUser(want); err != nil {
		t.Fatalf("SaveUser returned error: %v", err)
	}

	got, err := s.User()
	if err != nil {
		t.Fatalf("User returned error: %v", err)
	}
	if got != want {
		t.Fatalf("user got %+v want %+v", got, want)
	}
	if !got.IsRegistered() {
		t.Fatal("user got not registered want registered")
	}

	s2, err := store.New(root)
	if err != nil {
		t.Fatalf("reload New returned error: %v", err)
	}
	reloaded, err := s2.User()
	if err != nil {
		t.Fatalf("reload User returned error: %v", err)
	}
	if reloaded != want {
		t.Fatalf("reloaded user got %+v want %+v", reloaded, want)
	}
}
```

- [ ] Step 2: テストが失敗することを確認する

Run:
```bash
cd /Users/yujiokamoto/devs/golang/feedflow-go-htmx && go test ./internal/store/ -run "TestSaveAndGetSettings|TestSaveAndGetUser" -v
```
Expected: コンパイルエラーで失敗します。`s.SaveSettings undefined` と表示されます。

- [ ] Step 3: 最小実装を書く

Create `internal/store/settings.go`:
```go
package store

import (
	"fmt"

	"github.com/okamyuji/feedflow-go-htmx/internal/domain"
)

// Settings 現在の設定を返します。settings.jsonが未保存のときは既定値を返します。
func (s *Store) Settings() (domain.Settings, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.settings, nil
}

// SaveSettings 設定を保存し、settings.jsonをアトミックに書き出します。
func (s *Store) SaveSettings(settings domain.Settings) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	prev := s.settings
	s.settings = settings
	if err := writeJSONAtomic(s.path(settingsFile), s.settings); err != nil {
		s.settings = prev
		return fmt.Errorf("failed to save settings: %w", err)
	}
	return nil
}

// User 所有者ユーザーを返します。未登録の場合はゼロ値のUserを返します。
func (s *Store) User() (domain.User, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.user, nil
}

// SaveUser 所有者ユーザーを保存し、user.jsonをアトミックに書き出します。
func (s *Store) SaveUser(user domain.User) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	prev := s.user
	s.user = user
	if err := writeJSONAtomic(s.path(userFile), s.user); err != nil {
		s.user = prev
		return fmt.Errorf("failed to save user: %w", err)
	}
	return nil
}
```

- [ ] Step 4: テストが通ることを確認する

Run:
```bash
cd /Users/yujiokamoto/devs/golang/feedflow-go-htmx && go test ./internal/store/ -race -run "TestSaveAndGetSettings|TestSaveAndGetUser" -v
```
Expected: TestSaveAndGetSettings、TestSaveAndGetUserがいずれもPASSします。

- [ ] Step 5: gofmtを適用してコミットする

```bash
cd /Users/yujiokamoto/devs/golang/feedflow-go-htmx && gofmt -w internal/store/settings.go internal/store/store_test.go && git add internal/store/settings.go internal/store/store_test.go && git commit -m "feat: 設定とユーザーの取得と保存をStoreに追加する"
```

---

## Task 8: 並行アクセスの検証と破損ファイルのロード

Files:
- Edit: `/Users/yujiokamoto/devs/golang/feedflow-go-htmx/internal/store/store_test.go`

設計書セクション4.3と7のとおりStoreはメモリ常駐で並行読み書きに耐えます。-raceでのデータ競合検出と、破損JSONをロードしたときに明確なエラーを返すことを検証して、永続化エンジンの堅牢性を固めます。新規実装は不要で、既存コードがこの2つの性質を満たすことをテストで確認します。

- [ ] Step 1: 失敗または検証のためのテストをstore_test.goへ追記する

Add to `internal/store/store_test.go`:
```go

func TestConcurrentSaveAndRead(t *testing.T) {
	t.Parallel()

	s, err := store.New(filepath.Join(t.TempDir(), "data"))
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}

	const workers = 8
	const perWorker = 20

	var wg sync.WaitGroup
	wg.Add(workers * 2)

	for w := 0; w < workers; w++ {
		go func(w int) {
			defer wg.Done()
			for i := 0; i < perWorker; i++ {
				id := fmt.Sprintf("f-%d-%d", w, i)
				if err := s.SaveFeed(domain.Feed{ID: id, Title: id}); err != nil {
					t.Errorf("SaveFeed returned error: %v", err)
					return
				}
			}
		}(w)
		go func() {
			defer wg.Done()
			for i := 0; i < perWorker; i++ {
				if _, err := s.Feeds(); err != nil {
					t.Errorf("Feeds returned error: %v", err)
					return
				}
			}
		}()
	}
	wg.Wait()

	feeds, err := s.Feeds()
	if err != nil {
		t.Fatalf("Feeds returned error: %v", err)
	}
	if len(feeds) != workers*perWorker {
		t.Fatalf("feeds got %d want %d", len(feeds), workers*perWorker)
	}
}

func TestNewWithCorruptFeedsFileReturnsError(t *testing.T) {
	t.Parallel()

	root := filepath.Join(t.TempDir(), "data")
	if err := os.MkdirAll(filepath.Join(root, "items"), 0o700); err != nil {
		t.Fatalf("MkdirAll returned error: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "feeds.json"), []byte("{not json"), 0o600); err != nil {
		t.Fatalf("WriteFile returned error: %v", err)
	}

	_, err := store.New(root)
	if err == nil {
		t.Fatal("New got nil error want error for corrupt feeds.json")
	}
	if !strings.Contains(err.Error(), "feeds.json") {
		t.Fatalf("error got %v want it to mention feeds.json", err)
	}
}

func TestNewWithCorruptItemsFileReturnsError(t *testing.T) {
	t.Parallel()

	root := filepath.Join(t.TempDir(), "data")
	if err := os.MkdirAll(filepath.Join(root, "items"), 0o700); err != nil {
		t.Fatalf("MkdirAll returned error: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "items", "f1.json"), []byte("[broken"), 0o600); err != nil {
		t.Fatalf("WriteFile returned error: %v", err)
	}

	_, err := store.New(root)
	if err == nil {
		t.Fatal("New got nil error want error for corrupt items file")
	}
	if !strings.Contains(err.Error(), "f1.json") {
		t.Fatalf("error got %v want it to mention f1.json", err)
	}
}
```

import節に`"strings"`と`"sync"`を追加する必要があります。store_test.goの先頭importへ`"strings"`と`"sync"`を加えます。

- [ ] Step 2: テストを実行する

Run:
```bash
cd /Users/yujiokamoto/devs/golang/feedflow-go-htmx && go test ./internal/store/ -race -run "TestConcurrent|TestNewWithCorrupt" -v
```
Expected: TestConcurrentSaveAndRead、TestNewWithCorruptFeedsFileReturnsError、TestNewWithCorruptItemsFileReturnsErrorがいずれもPASSします。-raceでデータ競合が検出されないことを確認します。万一competing書き込みでrace警告が出る場合は、SaveFeedのロック範囲にwriteJSONAtomicが含まれていることを再確認します。

- [ ] Step 3: gofmtを適用してコミットする

```bash
cd /Users/yujiokamoto/devs/golang/feedflow-go-htmx && gofmt -w internal/store/store_test.go && git add internal/store/store_test.go && git commit -m "test: 並行アクセスと破損ファイルロードの検証を追加する"
```

---

## Task 9: フェーズ全体の品質ゲート

Files:
- なし(検証のみ)

このフェーズで追加したinternal/obsとinternal/storeを含めて品質ゲートを緑にします。

- [ ] Step 1: storeパッケージのカバレッジを確認する

Run:
```bash
cd /Users/yujiokamoto/devs/golang/feedflow-go-htmx && go test ./internal/store/ ./internal/obs/ -race -coverprofile=coverage.out -covermode=atomic && go tool cover -func=coverage.out | tail -n 1
```
Expected: storeとobsのテストがPASSし、合計カバレッジが80パーセント前後に達します。目標は80パーセントで、未達でもfailにはしません。

- [ ] Step 2: 品質ゲートを実行する

Run:
```bash
cd /Users/yujiokamoto/devs/golang/feedflow-go-htmx && bash scripts/quality-gate.sh
```
Expected: `all quality checks passed` で終わります。golangci-lintでwriteJSONAtomicの`_ =`破棄がerrcheckに指摘された場合は、Task2の補足のとおり該当行に`//nolint:errcheck // 異常系の後始末のため主たるエラーを優先する`を付けて再実行します。

- [ ] Step 3: 必要なら修正をコミットする

```bash
cd /Users/yujiokamoto/devs/golang/feedflow-go-htmx && git add -A && git commit -m "chore: Phase2の品質ゲートを緑化する"
```
補足: 修正が無ければこのコミットは省略します。

---

## Phase2完了条件

- [ ] `go test ./internal/store/ ./internal/obs/ -race` がPASSする
- [ ] `*store.Store` が `port.Repository` を満たす(`var _ port.Repository = (*store.Store)(nil)` がコンパイルされる)
- [ ] dataディレクトリとitemsサブディレクトリが起動時に作成される
- [ ]書き込みがtemp+renameでアトミックに行われ、一時ファイルが残らない
- [ ] DeleteFeedがフィードと記事ファイルの両方を削除する
- [ ]破損JSONのロードで明確なエラーを返す
- [ ] `bash scripts/quality-gate.sh` が `all quality checks passed` で終わる
- [ ]コミットが規約に沿って積まれている
