# Phase5 ポーラー 実装計画

> For agentic workers: REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

Goal: 設計書のセクション4.2と8に基づき、internal/pollerにフィードの取得反映ロジックと定期ポーリングのバックグラウンドスケジューラを実装します。port.PollServiceを満たすServiceがフィード単位と全体の取得反映を担い、Runnerがgoroutineとtime.Tickerで定期巡回します。フィードごとの間隔上書き、ジッタ、並行取得数の制限、手動更新での即時取得、contextでのグレースフル停止をすべてフェイク注入のテーブル駆動テストで検証します。

Architecture: クリーンアーキテクチャの一部としてinternal/pollerを置きます。pollerはinternal/portのインターフェース(Repository、Fetcher、FeedParser、Clock、IDGen、MuteService)にコンストラクタ注入で依存し、具象型には依存しません。Serviceはport.PollServiceを実装し、1フィードを取得してparseし新着記事をdomain.Itemへ写してETagとLast-Modifiedと連続エラー数を更新し保存します。Runnerはバックグラウンドのスケジューラで、time.Tickerの定期チェックごとに期限の来たフィードを並行数制限つきで取得します。手動更新はServiceのPollFeedを直接呼びます。停止はcontextのキャンセルで全goroutineを片付けてから戻ります。時刻はport.Clockを通して扱い、テストでは固定クロックとフェイクfetcherとフェイクparserとフェイクrepoを注入してI/Oと非決定性に触れずに検証します。

Tech Stack: Go 1.25(標準ライブラリのみ)。pollerはsync、time、context、math/rand/v2を使います。外部依存はありません。テストは標準のtestingでテーブル駆動とし、-raceで通る前提で書きます。

前提: 作業ディレクトリは`/Users/yujiokamoto/devs/golang/feedflow-go-htmx`です。Phase4までが完了し`bash scripts/quality-gate.sh`が緑であることを確認してから始めます。module pathは`github.com/okamyuji/feedflow-go-htmx`です。port.PollServiceのシグネチャはPhase1の`internal/port/service.go`で確定済みで、`PollFeed(ctx context.Context, feedID string) (int, error)`と`PollAll(ctx context.Context) (int, error)`です。port.MuteServiceはPhase4で実装済みで、ポーラーは新着記事の保存前にミュートを適用するために利用します。

このフェーズで追加する補助型は次のとおりです。いずれもPhase1で確定した型と矛盾しません。

- `poller.Config` Runnerの設定値を持つ構造体です。既定間隔、最小巡回間隔、並行取得数、ジッタ割合を保持します
- `poller.Runner` バックグラウンドのスケジューラ本体を表す構造体です
- `poller.Service` port.PollServiceの実装を表す構造体です
- `poller.jitterFunc` ジッタ計算を差し替え可能にする関数型です。テストでジッタを固定するために注入します

---

## Task 1: ポーラーのエラー定義と設定型

Files:
- Create: `/Users/yujiokamoto/devs/golang/feedflow-go-htmx/internal/poller/poller.go`

ポーラー全体で使うエラー値と、Runnerの設定型Configを先に定義します。Configはバックグラウンドスケジューラの巡回間隔と並行取得数とジッタ割合を保持します。手動更新や全体ポーリングで参照する定数もここに置きます。

- [ ] Step 1: 失敗するテストを書く

Create `internal/poller/config_test.go`:
```go
package poller

import "testing"

func TestDefaultConfig(t *testing.T) {
	t.Parallel()
	c := DefaultConfig()
	if c.TickInterval <= 0 {
		t.Fatalf("TickInterval got %v want positive", c.TickInterval)
	}
	if c.MaxConcurrent <= 0 {
		t.Fatalf("MaxConcurrent got %d want positive", c.MaxConcurrent)
	}
	if c.JitterRatio < 0 || c.JitterRatio >= 1 {
		t.Fatalf("JitterRatio got %v want in [0,1)", c.JitterRatio)
	}
}

func TestConfigNormalize(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name             string
		in               Config
		wantTick         bool // TickIntervalが正かどうか
		wantConcurrentGE int  // MaxConcurrentが下限以上か
	}{
		{
			name:             "ゼロ値は既定で補完する",
			in:               Config{},
			wantTick:         true,
			wantConcurrentGE: 1,
		},
		{
			name:             "負の並行数は1へ補正する",
			in:               Config{MaxConcurrent: -3},
			wantTick:         true,
			wantConcurrentGE: 1,
		},
		{
			name:             "正の値はそのまま保つ",
			in:               Config{MaxConcurrent: 8},
			wantTick:         true,
			wantConcurrentGE: 8,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := tt.in.normalize()
			if tt.wantTick && got.TickInterval <= 0 {
				t.Fatalf("TickInterval got %v want positive", got.TickInterval)
			}
			if got.MaxConcurrent < tt.wantConcurrentGE {
				t.Fatalf("MaxConcurrent got %d want >= %d", got.MaxConcurrent, tt.wantConcurrentGE)
			}
			if got.JitterRatio < 0 || got.JitterRatio >= 1 {
				t.Fatalf("JitterRatio got %v want in [0,1)", got.JitterRatio)
			}
		})
	}
}
```

- [ ] Step 2: テストが失敗することを確認する

Run:
```bash
cd /Users/yujiokamoto/devs/golang/feedflow-go-htmx && go test ./internal/poller/ -run 'TestDefaultConfig|TestConfigNormalize' -v
```
Expected: コンパイルエラーで失敗します。`undefined: DefaultConfig` や `undefined: Config` と表示されます。

- [ ] Step 3: 最小実装を書く

Create `internal/poller/poller.go`:
```go
// Package poller feedflowのフィード取得反映と定期ポーリングを提供します。
// このパッケージはinternal/portのインターフェースにコンストラクタ注入で依存し、
// 具体的な実装には依存しません。設計書のセクション4.2と8に対応します。
package poller

import (
	"errors"
	"time"
)

// errFeedNotFound 指定IDのフィードが見つからないことを表します。
var errFeedNotFound = errors.New("poller: feed not found")

// ポーラーの設定の既定値です。設計書のセクション4.2に対応します。
const (
	// defaultTickInterval Runnerが期限到来フィードを走査する間隔です。
	// 最短のフィード上書き間隔が15分のため、1分ごとの走査で十分に細かく検知できます。
	defaultTickInterval = time.Minute
	// defaultMaxConcurrent 同時に取得するフィード数の既定の上限です。
	defaultMaxConcurrent = 4
	// defaultJitterRatio 巡回判定に乗せるジッタの割合です。間隔の最大10パーセントを散らします。
	defaultJitterRatio = 0.1
)

// Config Runnerのバックグラウンド巡回の設定を保持します。
type Config struct {
	TickInterval  time.Duration // 期限到来フィードを走査する間隔です
	MaxConcurrent int           // 同時取得するフィード数の上限です
	JitterRatio   float64       // 取得判定に乗せるジッタの割合です。0以上1未満です
}

// DefaultConfig 既定値で初期化した設定を返します。
func DefaultConfig() Config {
	return Config{
		TickInterval:  defaultTickInterval,
		MaxConcurrent: defaultMaxConcurrent,
		JitterRatio:   defaultJitterRatio,
	}
}

// normalize 不正値や未設定値を既定値へ補正した新しい設定を返します。
// 受け取った値は変更せず、補正後の新しいConfigを返します。
func (c Config) normalize() Config {
	out := c
	if out.TickInterval <= 0 {
		out.TickInterval = defaultTickInterval
	}
	if out.MaxConcurrent < 1 {
		out.MaxConcurrent = defaultMaxConcurrent
	}
	if out.JitterRatio < 0 || out.JitterRatio >= 1 {
		out.JitterRatio = defaultJitterRatio
	}
	return out
}
```

- [ ] Step 4: テストが通ることを確認する

Run:
```bash
cd /Users/yujiokamoto/devs/golang/feedflow-go-htmx && go test ./internal/poller/ -run 'TestDefaultConfig|TestConfigNormalize' -v
```
Expected: `TestDefaultConfig` と `TestConfigNormalize` がいずれも PASS します。

- [ ] Step 5: gofmtを適用してコミットする

```bash
cd /Users/yujiokamoto/devs/golang/feedflow-go-htmx && gofmt -w internal/poller/poller.go internal/poller/config_test.go && git add internal/poller/poller.go internal/poller/config_test.go && git commit -m "feat: ポーラーのエラー定義と設定型を追加する"
```

---

## Task 2: pollerのテスト用フェイクとヘルパ

Files:
- Create: `/Users/yujiokamoto/devs/golang/feedflow-go-htmx/internal/poller/fake_test.go`

後続のServiceとRunnerのテストで共通利用するフェイク群を先に用意します。固定クロック、連番IDGen、呼び出し記録つきフェイクfetcher、フェイクparser、メモリ常駐フェイクrepo、素通しと固定除外を選べるフェイクMuteServiceを定義します。これによりポーラーを外部I/Oと非決定性に触れずに検証できます。

- [ ] Step 1: テスト用フェイクを書く

Create `internal/poller/fake_test.go`:
```go
package poller

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/okamyuji/feedflow-go-htmx/internal/domain"
	"github.com/okamyuji/feedflow-go-htmx/internal/port"
)

// fakeClock 制御可能な時刻を返すClockのフェイクです。
type fakeClock struct {
	mu  sync.Mutex
	now time.Time
}

func newFakeClock(t time.Time) *fakeClock { return &fakeClock{now: t} }

func (c *fakeClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

// advance 現在時刻を指定だけ進めます。テストで間隔到来を再現するために使います。
func (c *fakeClock) advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.now = c.now.Add(d)
}

// fakeIDGen 決定的な連番IDを返すIDGenのフェイクです。
type fakeIDGen struct {
	mu sync.Mutex
	n  int
}

func (g *fakeIDGen) NewID() string {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.n++
	return fmt.Sprintf("id-%d", g.n)
}

// fakeFetcher URLごとの固定結果を返し、呼び出し回数を記録するFetcherのフェイクです。
type fakeFetcher struct {
	mu        sync.Mutex
	results   map[string]port.FetchResult
	errs      map[string]error
	callCount map[string]int
}

func newFakeFetcher() *fakeFetcher {
	return &fakeFetcher{
		results:   map[string]port.FetchResult{},
		errs:      map[string]error{},
		callCount: map[string]int{},
	}
}

func (f *fakeFetcher) Fetch(ctx context.Context, req port.FetchRequest) (port.FetchResult, error) {
	if err := ctx.Err(); err != nil {
		return port.FetchResult{}, err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	f.callCount[req.URL]++
	if err, ok := f.errs[req.URL]; ok {
		return port.FetchResult{}, err
	}
	res, ok := f.results[req.URL]
	if !ok {
		return port.FetchResult{StatusCode: 200}, nil
	}
	return res, nil
}

func (f *fakeFetcher) calls(url string) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.callCount[url]
}

// fakeParser 固定の結果を返すFeedParserのフェイクです。
type fakeParser struct {
	parsed port.ParsedFeed
	err    error
}

func (p fakeParser) Parse(_ []byte) (port.ParsedFeed, error) {
	return p.parsed, p.err
}

// passthroughMute 何も除外しないMuteServiceのフェイクです。
type passthroughMute struct{}

func (passthroughMute) ListFilters() ([]domain.MuteFilter, error) { return nil, nil }

func (passthroughMute) AddFilter(_ string, _ domain.MuteScope, _ string) (domain.MuteFilter, error) {
	return domain.MuteFilter{}, nil
}

func (passthroughMute) DeleteFilter(_ string) error { return nil }

func (passthroughMute) Filter(items []domain.Item) ([]domain.Item, error) {
	return items, nil
}

// titleMute 指定キーワードをタイトルに含む記事を除外するMuteServiceのフェイクです。
type titleMute struct{ keyword string }

func (titleMute) ListFilters() ([]domain.MuteFilter, error) { return nil, nil }

func (titleMute) AddFilter(_ string, _ domain.MuteScope, _ string) (domain.MuteFilter, error) {
	return domain.MuteFilter{}, nil
}

func (titleMute) DeleteFilter(_ string) error { return nil }

func (m titleMute) Filter(items []domain.Item) ([]domain.Item, error) {
	out := make([]domain.Item, 0, len(items))
	for _, it := range items {
		if it.Title == m.keyword {
			continue
		}
		out = append(out, it)
	}
	return out, nil
}

// fakeRepo メモリ上で全エンティティを保持するRepositoryのフェイクです。
// 並行アクセスに耐えるためsync.Mutexで保護します。
type fakeRepo struct {
	mu       sync.Mutex
	feeds    map[string]domain.Feed
	items    map[string][]domain.Item
	settings domain.Settings
}

func newFakeRepo() *fakeRepo {
	return &fakeRepo{
		feeds:    map[string]domain.Feed{},
		items:    map[string][]domain.Item{},
		settings: domain.DefaultSettings(),
	}
}

func (r *fakeRepo) Feeds() ([]domain.Feed, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]domain.Feed, 0, len(r.feeds))
	for _, f := range r.feeds {
		out = append(out, f)
	}
	return out, nil
}

func (r *fakeRepo) Feed(id string) (domain.Feed, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	f, ok := r.feeds[id]
	if !ok {
		return domain.Feed{}, errFeedNotFound
	}
	return f, nil
}

func (r *fakeRepo) SaveFeed(feed domain.Feed) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.feeds[feed.ID] = feed
	return nil
}

func (r *fakeRepo) DeleteFeed(id string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.feeds, id)
	delete(r.items, id)
	return nil
}

func (r *fakeRepo) Categories() ([]domain.Category, error)   { return nil, nil }
func (r *fakeRepo) SaveCategory(_ domain.Category) error     { return nil }
func (r *fakeRepo) DeleteCategory(_ string) error            { return nil }

func (r *fakeRepo) Items(feedID string) ([]domain.Item, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	src := r.items[feedID]
	out := make([]domain.Item, len(src))
	copy(out, src)
	return out, nil
}

func (r *fakeRepo) SaveItems(feedID string, items []domain.Item) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	stored := make([]domain.Item, len(items))
	copy(stored, items)
	r.items[feedID] = stored
	return nil
}

func (r *fakeRepo) Boards() ([]domain.Board, error)      { return nil, nil }
func (r *fakeRepo) SaveBoard(_ domain.Board) error       { return nil }
func (r *fakeRepo) DeleteBoard(_ string) error           { return nil }
func (r *fakeRepo) Filters() ([]domain.MuteFilter, error) { return nil, nil }
func (r *fakeRepo) SaveFilter(_ domain.MuteFilter) error  { return nil }
func (r *fakeRepo) DeleteFilter(_ string) error           { return nil }

func (r *fakeRepo) Settings() (domain.Settings, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.settings, nil
}

func (r *fakeRepo) SaveSettings(s domain.Settings) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.settings = s
	return nil
}

func (r *fakeRepo) User() (domain.User, error)  { return domain.User{}, nil }
func (r *fakeRepo) SaveUser(_ domain.User) error { return nil }

// インターフェース充足をコンパイル時に検証します。
var (
	_ port.Clock       = (*fakeClock)(nil)
	_ port.IDGen       = (*fakeIDGen)(nil)
	_ port.Fetcher     = (*fakeFetcher)(nil)
	_ port.FeedParser  = fakeParser{}
	_ port.Repository  = (*fakeRepo)(nil)
	_ port.MuteService = passthroughMute{}
	_ port.MuteService = titleMute{}
)
```

- [ ] Step 2: フェイクがコンパイルとインターフェース充足を満たすことを確認する

Run:
```bash
cd /Users/yujiokamoto/devs/golang/feedflow-go-htmx && go vet ./internal/poller/
```
Expected: エラーなく完了します。テストファイルのみのためビルド対象は空ですが、go vetがテストファイルもコンパイルして型チェックを通します。

- [ ] Step 3: gofmtを適用してコミットする

```bash
cd /Users/yujiokamoto/devs/golang/feedflow-go-htmx && gofmt -w internal/poller/fake_test.go && git add internal/poller/fake_test.go && git commit -m "test: ポーラーのテスト用フェイクとヘルパを追加する"
```

---

## Task 3: Service の生成とPollFeedの新着反映(TDD)

Files:
- Create: `/Users/yujiokamoto/devs/golang/feedflow-go-htmx/internal/poller/service.go`

port.PollServiceを満たすServiceを実装します。NewServiceでrepo、fetcher、parser、clock、ids、muteを注入します。PollFeedは1フィードを取得してparseし、既存記事のGUID集合と突き合わせて新着だけをdomain.Itemへ写してID付与とFeedID紐付けを行い、ミュートを適用してから既存記事の前に積んで保存します。取得成功時はETagとLast-Modifiedと最終取得時刻を更新し連続エラー数を0へ戻します。NotModifiedのときは記事を増やさず最終取得時刻だけ更新します。

- [ ] Step 1: 失敗するテストを書く

Create `internal/poller/service_test.go`:
```go
package poller

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/okamyuji/feedflow-go-htmx/internal/domain"
	"github.com/okamyuji/feedflow-go-htmx/internal/port"
)

func TestServicePollFeedNewItems(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 5, 29, 12, 0, 0, 0, time.UTC)
	repo := newFakeRepo()
	_ = repo.SaveFeed(domain.Feed{ID: "f1", FeedURL: "https://example.com/feed", Title: "old"})
	_ = repo.SaveItems("f1", []domain.Item{
		{ID: "x1", FeedID: "f1", GUID: "g-existing", Title: "既存", FetchedAt: now.Add(-time.Hour)},
	})

	fetcher := newFakeFetcher()
	fetcher.results["https://example.com/feed"] = port.FetchResult{
		StatusCode:   200,
		Body:         []byte("<rss></rss>"),
		ETag:         "etag-new",
		LastModified: "Thu, 29 May 2026 11:00:00 GMT",
	}
	parser := fakeParser{parsed: port.ParsedFeed{
		Format:  port.FormatRSS2,
		Title:   "new title",
		SiteURL: "https://example.com",
		Items: []port.ParsedItem{
			{GUID: "g-new1", Title: "新着1", Link: "https://example.com/1", PublishedAt: now.Add(-30 * time.Minute)},
			{GUID: "g-existing", Title: "既存", Link: "https://example.com/0"},
		},
	}}

	svc := NewService(repo, fetcher, parser, newFakeClock(now), &fakeIDGen{}, passthroughMute{})

	n, err := svc.PollFeed(context.Background(), "f1")
	if err != nil {
		t.Fatalf("PollFeed returned error: %v", err)
	}
	if n != 1 {
		t.Fatalf("new item count got %d want 1", n)
	}

	items, _ := repo.Items("f1")
	if len(items) != 2 {
		t.Fatalf("stored item count got %d want 2", len(items))
	}
	if items[0].GUID != "g-new1" {
		t.Fatalf("newest item GUID got %q want %q", items[0].GUID, "g-new1")
	}
	if items[0].ID == "" || items[0].FeedID != "f1" {
		t.Fatalf("new item must get ID and FeedID, got ID=%q FeedID=%q", items[0].ID, items[0].FeedID)
	}
	if !items[0].FetchedAt.Equal(now) {
		t.Fatalf("new item FetchedAt got %v want %v", items[0].FetchedAt, now)
	}

	feed, _ := repo.Feed("f1")
	if feed.ETag != "etag-new" {
		t.Fatalf("feed ETag got %q want %q", feed.ETag, "etag-new")
	}
	if feed.Title != "new title" {
		t.Fatalf("feed Title got %q want %q", feed.Title, "new title")
	}
	if !feed.LastFetchedAt.Equal(now) {
		t.Fatalf("feed LastFetchedAt got %v want %v", feed.LastFetchedAt, now)
	}
	if feed.ConsecutiveErrors != 0 {
		t.Fatalf("feed ConsecutiveErrors got %d want 0", feed.ConsecutiveErrors)
	}
}

func TestServicePollFeedNotModified(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 5, 29, 12, 0, 0, 0, time.UTC)
	repo := newFakeRepo()
	_ = repo.SaveFeed(domain.Feed{ID: "f1", FeedURL: "https://example.com/feed", ETag: "etag-old"})
	_ = repo.SaveItems("f1", []domain.Item{{ID: "x1", FeedID: "f1", GUID: "g0", Title: "既存"}})

	fetcher := newFakeFetcher()
	fetcher.results["https://example.com/feed"] = port.FetchResult{StatusCode: 304, NotModified: true}
	parser := fakeParser{err: errors.New("parser must not be called on 304")}

	svc := NewService(repo, fetcher, parser, newFakeClock(now), &fakeIDGen{}, passthroughMute{})

	n, err := svc.PollFeed(context.Background(), "f1")
	if err != nil {
		t.Fatalf("PollFeed returned error: %v", err)
	}
	if n != 0 {
		t.Fatalf("new item count got %d want 0", n)
	}
	items, _ := repo.Items("f1")
	if len(items) != 1 {
		t.Fatalf("item count got %d want 1 (unchanged)", len(items))
	}
	feed, _ := repo.Feed("f1")
	if !feed.LastFetchedAt.Equal(now) {
		t.Fatalf("feed LastFetchedAt got %v want %v", feed.LastFetchedAt, now)
	}
	if feed.ConsecutiveErrors != 0 {
		t.Fatalf("feed ConsecutiveErrors got %d want 0", feed.ConsecutiveErrors)
	}
}

func TestServicePollFeedAppliesMute(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 5, 29, 12, 0, 0, 0, time.UTC)
	repo := newFakeRepo()
	_ = repo.SaveFeed(domain.Feed{ID: "f1", FeedURL: "https://example.com/feed"})

	fetcher := newFakeFetcher()
	fetcher.results["https://example.com/feed"] = port.FetchResult{StatusCode: 200, Body: []byte("<rss></rss>")}
	parser := fakeParser{parsed: port.ParsedFeed{Items: []port.ParsedItem{
		{GUID: "g1", Title: "通す記事"},
		{GUID: "g2", Title: "広告"},
	}}}

	svc := NewService(repo, fetcher, parser, newFakeClock(now), &fakeIDGen{}, titleMute{keyword: "広告"})

	n, err := svc.PollFeed(context.Background(), "f1")
	if err != nil {
		t.Fatalf("PollFeed returned error: %v", err)
	}
	if n != 1 {
		t.Fatalf("new item count got %d want 1 (muted one excluded)", n)
	}
	items, _ := repo.Items("f1")
	if len(items) != 1 || items[0].Title != "通す記事" {
		t.Fatalf("stored items got %+v want only 通す記事", items)
	}
}

func TestServicePollFeedError(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 5, 29, 12, 0, 0, 0, time.UTC)
	repo := newFakeRepo()
	_ = repo.SaveFeed(domain.Feed{ID: "f1", FeedURL: "https://example.com/feed", ConsecutiveErrors: 2})

	fetcher := newFakeFetcher()
	fetcher.errs["https://example.com/feed"] = errors.New("network down")
	parser := fakeParser{}

	svc := NewService(repo, fetcher, parser, newFakeClock(now), &fakeIDGen{}, passthroughMute{})

	_, err := svc.PollFeed(context.Background(), "f1")
	if err == nil {
		t.Fatalf("PollFeed must return error on fetch failure")
	}
	feed, _ := repo.Feed("f1")
	if feed.ConsecutiveErrors != 3 {
		t.Fatalf("feed ConsecutiveErrors got %d want 3", feed.ConsecutiveErrors)
	}
}

func TestServicePollFeedNotFound(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 5, 29, 12, 0, 0, 0, time.UTC)
	repo := newFakeRepo()
	svc := NewService(repo, newFakeFetcher(), fakeParser{}, newFakeClock(now), &fakeIDGen{}, passthroughMute{})

	_, err := svc.PollFeed(context.Background(), "missing")
	if err == nil {
		t.Fatalf("PollFeed must return error for unknown feed")
	}
}

// インターフェース充足をコンパイル時に検証します。
var _ port.PollService = (*Service)(nil)
```

- [ ] Step 2: テストが失敗することを確認する

Run:
```bash
cd /Users/yujiokamoto/devs/golang/feedflow-go-htmx && go test ./internal/poller/ -run 'TestServicePollFeed' -v
```
Expected: コンパイルエラーで失敗します。`undefined: NewService` や `undefined: Service` と表示されます。

- [ ] Step 3: Service の実装を書く

Create `internal/poller/service.go`:
```go
package poller

import (
	"context"
	"fmt"
	"time"

	"github.com/okamyuji/feedflow-go-htmx/internal/domain"
	"github.com/okamyuji/feedflow-go-htmx/internal/port"
)

// Service フィードの取得反映を担いport.PollServiceを満たします。
// repoとfetcherとparserとclockとidsとmuteとjitterをコンストラクタ注入で受け取ります。
type Service struct {
	repo    port.Repository
	fetcher port.Fetcher
	parser  port.FeedParser
	clock   port.Clock
	ids     port.IDGen
	mute    port.MuteService
	jitter  jitterFunc
}

// NewService 依存を注入してServiceを生成します。
// ジッタは既定の割合ジッタを用います。PollAllの期限判定で取得時刻を散らします。
func NewService(
	repo port.Repository,
	fetcher port.Fetcher,
	parser port.FeedParser,
	clock port.Clock,
	ids port.IDGen,
	mute port.MuteService,
) *Service {
	return &Service{
		repo:    repo,
		fetcher: fetcher,
		parser:  parser,
		clock:   clock,
		ids:     ids,
		mute:    mute,
		jitter:  ratioJitter(defaultJitterRatio),
	}
}

// PollFeed 指定フィードを取得し新着記事を反映して新着件数を返します。
// 取得に失敗した場合は連続エラー数を1増やして保存し、エラーを返します。
// サーバが未更新を示した場合は記事を増やさず最終取得時刻だけ更新します。
func (s *Service) PollFeed(ctx context.Context, feedID string) (int, error) {
	feed, err := s.repo.Feed(feedID)
	if err != nil {
		return 0, fmt.Errorf("failed to load feed %q: %w", feedID, err)
	}

	res, err := s.fetcher.Fetch(ctx, port.FetchRequest{
		URL:          feed.FeedURL,
		ETag:         feed.ETag,
		LastModified: feed.LastModified,
	})
	if err != nil {
		return 0, s.recordFetchError(feed, err)
	}

	now := s.clock.Now()
	if res.NotModified {
		return 0, s.recordNotModified(feed, now)
	}

	parsed, err := s.parser.Parse(res.Body)
	if err != nil {
		return 0, s.recordFetchError(feed, fmt.Errorf("failed to parse feed %q: %w", feedID, err))
	}

	added, err := s.applyParsed(feed, parsed, res, now)
	if err != nil {
		return 0, err
	}
	return added, nil
}

// PollAll 期限の来た全フィードを取得して反映し、処理したフィード数を返します。
// 期限判定は最終取得時刻と間隔から行い、手動のみのフィードは対象外とします。
// 個々のフィードの取得失敗は処理を止めず、処理を試みたフィード数を数えます。
func (s *Service) PollAll(ctx context.Context) (int, error) {
	feeds, err := s.repo.Feeds()
	if err != nil {
		return 0, fmt.Errorf("failed to load feeds: %w", err)
	}
	settings, err := s.repo.Settings()
	if err != nil {
		return 0, fmt.Errorf("failed to load settings: %w", err)
	}

	now := s.clock.Now()
	processed := 0
	for _, feed := range feeds {
		if err := ctx.Err(); err != nil {
			return processed, err
		}
		if !dueForPollWithJitter(feed, settings, now, s.jitter) {
			continue
		}
		processed++
		if _, err := s.PollFeed(ctx, feed.ID); err != nil {
			continue
		}
	}
	return processed, nil
}

// applyParsed パース結果を既存記事と突き合わせ新着を反映してフィードを更新します。
// 既存のGUID集合に無い記事だけを新着としてdomain.Itemへ写し、ID付与とFeedID紐付けを行います。
// 新着にミュートを適用してから既存記事の前に積んで保存します。
// あわせてフィードのタイトルとサイトURLとETagとLast-Modifiedと最終取得時刻を更新し、
// 連続エラー数を0へ戻します。戻り値は保存した新着の件数です。
func (s *Service) applyParsed(feed domain.Feed, parsed port.ParsedFeed, res port.FetchResult, now time.Time) (int, error) {
	existing, err := s.repo.Items(feed.ID)
	if err != nil {
		return 0, fmt.Errorf("failed to load items for feed %q: %w", feed.ID, err)
	}

	seen := make(map[string]struct{}, len(existing))
	for _, it := range existing {
		seen[it.GUID] = struct{}{}
	}

	fresh := make([]domain.Item, 0, len(parsed.Items))
	for _, pi := range parsed.Items {
		if _, ok := seen[pi.GUID]; ok {
			continue
		}
		seen[pi.GUID] = struct{}{}
		fresh = append(fresh, domain.Item{
			ID:          s.ids.NewID(),
			FeedID:      feed.ID,
			GUID:        pi.GUID,
			Title:       pi.Title,
			Link:        pi.Link,
			Content:     pi.Content,
			Summary:     pi.Summary,
			Author:      pi.Author,
			PublishedAt: pi.PublishedAt,
			FetchedAt:   now,
		})
	}

	fresh, err = s.mute.Filter(fresh)
	if err != nil {
		return 0, fmt.Errorf("failed to apply mute for feed %q: %w", feed.ID, err)
	}

	if len(fresh) > 0 {
		merged := make([]domain.Item, 0, len(fresh)+len(existing))
		merged = append(merged, fresh...)
		merged = append(merged, existing...)
		if err := s.repo.SaveItems(feed.ID, merged); err != nil {
			return 0, fmt.Errorf("failed to save items for feed %q: %w", feed.ID, err)
		}
	}

	updated := feed
	if parsed.Title != "" {
		updated.Title = parsed.Title
	}
	if parsed.SiteURL != "" {
		updated.SiteURL = parsed.SiteURL
	}
	updated.ETag = res.ETag
	updated.LastModified = res.LastModified
	updated.LastFetchedAt = now
	updated.ConsecutiveErrors = 0
	if err := s.repo.SaveFeed(updated); err != nil {
		return 0, fmt.Errorf("failed to save feed %q: %w", feed.ID, err)
	}

	return len(fresh), nil
}

// recordFetchError 取得や解析の失敗を連続エラー数へ反映してエラーを返します。
func (s *Service) recordFetchError(feed domain.Feed, cause error) error {
	updated := feed
	updated.ConsecutiveErrors = feed.ConsecutiveErrors + 1
	if saveErr := s.repo.SaveFeed(updated); saveErr != nil {
		return fmt.Errorf("failed to save feed after fetch error (%v): %w", cause, saveErr)
	}
	return fmt.Errorf("failed to poll feed %q: %w", feed.ID, cause)
}

// recordNotModified 未更新応答時に最終取得時刻を更新し連続エラー数を0へ戻します。
func (s *Service) recordNotModified(feed domain.Feed, now time.Time) error {
	updated := feed
	updated.LastFetchedAt = now
	updated.ConsecutiveErrors = 0
	if err := s.repo.SaveFeed(updated); err != nil {
		return fmt.Errorf("failed to save feed %q after not-modified: %w", feed.ID, err)
	}
	return nil
}
```
補足: PollAllが参照するdueForPollWithJitterとeffectiveIntervalとratioJitterは次のStep4でschedule.goに確定実装として作成します。このStepのservice.goは確定形で、後続の差し替えはありません。

- [ ] Step 4: スケジュール判定の確定実装を書く

Create `internal/poller/schedule.go`:
```go
package poller

import (
	"math/rand/v2"
	"time"

	"github.com/okamyuji/feedflow-go-htmx/internal/domain"
)

// jitterFunc 取得判定に乗せるジッタを返す関数型です。
// 引数は対象フィードのポーリング間隔で、戻り値は前倒しを許す時間幅です。
// テストではジッタを固定するためにこの関数を差し替えます。
type jitterFunc func(interval time.Duration) time.Duration

// ratioJitter 間隔に対する割合で0以上その割合分以下のジッタを返す関数を生成します。
// 割合が0以下のときは常に0を返し、ジッタを無効にします。
func ratioJitter(ratio float64) jitterFunc {
	return func(interval time.Duration) time.Duration {
		if ratio <= 0 || interval <= 0 {
			return 0
		}
		maxJitter := time.Duration(float64(interval) * ratio)
		if maxJitter <= 0 {
			return 0
		}
		return time.Duration(rand.Int64N(int64(maxJitter) + 1))
	}
}

// effectiveInterval 指定フィードに適用するポーリング間隔を返します。
// フィードの上書きがdefaultまたは空のときは全体設定の間隔を使います。
// manualのときと不正値のときはゼロを返し、定期取得の対象外とします。
func effectiveInterval(feed domain.Feed, settings domain.Settings) time.Duration {
	pi := feed.PollInterval
	if pi == "" || pi == domain.PollDefault {
		pi = settings.PollInterval
	}
	d, ok := pi.Duration()
	if !ok {
		return 0
	}
	return d
}

// dueForPollWithJitter 指定フィードが現時点で取得対象かどうかをジッタ込みで返します。
// 間隔がゼロのフィード(手動のみや不正値)は常に対象外です。
// 最終取得が未設定のフィードは常に対象です。
// それ以外は最終取得からの経過が、間隔からジッタ分を引いた値以上のとき対象とします。
func dueForPollWithJitter(feed domain.Feed, settings domain.Settings, now time.Time, jitter jitterFunc) bool {
	interval := effectiveInterval(feed, settings)
	if interval <= 0 {
		return false
	}
	if feed.LastFetchedAt.IsZero() {
		return true
	}
	threshold := interval - jitter(interval)
	if threshold < 0 {
		threshold = 0
	}
	return now.Sub(feed.LastFetchedAt) >= threshold
}
```
補足: schedule.goはこのStepで確定形を置きます。Task4ではこのファイルに対するテストを追加するだけで、実装の差し替えはありません。これでパッケージ全体がコンパイルされ、PollFeedのテストが通ります。

- [ ] Step 5: テストが通ることを確認する

Run:
```bash
cd /Users/yujiokamoto/devs/golang/feedflow-go-htmx && go test ./internal/poller/ -run 'TestServicePollFeed' -v
```
Expected: `TestServicePollFeedNewItems` と `TestServicePollFeedNotModified` と `TestServicePollFeedAppliesMute` と `TestServicePollFeedError` と `TestServicePollFeedNotFound` がいずれも PASS します。

- [ ] Step 6: gofmtを適用してコミットする

```bash
cd /Users/yujiokamoto/devs/golang/feedflow-go-htmx && gofmt -w internal/poller/service.go internal/poller/schedule.go internal/poller/service_test.go && git add internal/poller/service.go internal/poller/schedule.go internal/poller/service_test.go && git commit -m "feat: PollService の PollFeed で新着反映と未更新と失敗を扱う"
```

---

## Task 4: 間隔上書きとジッタのテスト(TDD)

Files:
- Create: `/Users/yujiokamoto/devs/golang/feedflow-go-htmx/internal/poller/schedule_test.go`

Task3のStep4でschedule.goに確定実装したeffectiveIntervalとdueForPollWithJitterとratioJitterの振る舞いをテーブル駆動テストで検証します。effectiveIntervalはフィードのPollIntervalがdefaultまたは空のとき全体設定の間隔を使い、manualのときと不正値のときはゼロを返し対象外にします。dueForPollWithJitterはジッタを引数の関数で受け取り、最終取得時刻からの経過が間隔からジッタ分を引いた値以上なら取得対象とします。これにより全フィードが同時刻に集中しないよう散らします。実装はTask3で完成済みのため、このTaskは振る舞いの検証に集中します。

- [ ] Step 1: スケジュール判定のテストを書く

Create `internal/poller/schedule_test.go`:
```go
package poller

import (
	"testing"
	"time"

	"github.com/okamyuji/feedflow-go-htmx/internal/domain"
)

func TestEffectiveInterval(t *testing.T) {
	t.Parallel()
	settings := domain.Settings{PollInterval: domain.Poll1Hour}
	tests := []struct {
		name string
		feed domain.Feed
		want time.Duration
	}{
		{name: "上書きなしは全体設定に従う", feed: domain.Feed{PollInterval: domain.PollDefault}, want: time.Hour},
		{name: "空も全体設定に従う", feed: domain.Feed{PollInterval: ""}, want: time.Hour},
		{name: "15分上書き", feed: domain.Feed{PollInterval: domain.Poll15Min}, want: 15 * time.Minute},
		{name: "6時間上書き", feed: domain.Feed{PollInterval: domain.Poll6Hour}, want: 6 * time.Hour},
		{name: "手動のみは対象外でゼロ", feed: domain.Feed{PollInterval: domain.PollManualOnly}, want: 0},
		{name: "不正値はゼロ", feed: domain.Feed{PollInterval: "weekly"}, want: 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := effectiveInterval(tt.feed, settings); got != tt.want {
				t.Fatalf("effectiveInterval() got %v want %v", got, tt.want)
			}
		})
	}
}

func TestDueForPoll(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 5, 29, 12, 0, 0, 0, time.UTC)
	settings := domain.Settings{PollInterval: domain.Poll30Min}
	noJitter := func(time.Duration) time.Duration { return 0 }
	tests := []struct {
		name   string
		feed   domain.Feed
		jitter jitterFunc
		want   bool
	}{
		{
			name:   "未取得は常に対象",
			feed:   domain.Feed{PollInterval: domain.Poll30Min},
			jitter: noJitter,
			want:   true,
		},
		{
			name:   "間隔未経過は対象外",
			feed:   domain.Feed{PollInterval: domain.Poll30Min, LastFetchedAt: now.Add(-10 * time.Minute)},
			jitter: noJitter,
			want:   false,
		},
		{
			name:   "間隔経過は対象",
			feed:   domain.Feed{PollInterval: domain.Poll30Min, LastFetchedAt: now.Add(-31 * time.Minute)},
			jitter: noJitter,
			want:   true,
		},
		{
			name:   "手動のみは経過しても対象外",
			feed:   domain.Feed{PollInterval: domain.PollManualOnly, LastFetchedAt: now.Add(-10 * time.Hour)},
			jitter: noJitter,
			want:   false,
		},
		{
			name:   "ジッタで前倒し取得を許す",
			feed:   domain.Feed{PollInterval: domain.Poll30Min, LastFetchedAt: now.Add(-26 * time.Minute)},
			jitter: func(time.Duration) time.Duration { return 5 * time.Minute },
			want:   true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := dueForPollWithJitter(tt.feed, settings, now, tt.jitter)
			if got != tt.want {
				t.Fatalf("dueForPollWithJitter() got %v want %v", got, tt.want)
			}
		})
	}
}

func TestRatioJitterBounds(t *testing.T) {
	t.Parallel()
	j := ratioJitter(0.1)
	interval := 30 * time.Minute
	for i := 0; i < 1000; i++ {
		got := j(interval)
		if got < 0 || got > interval/10 {
			t.Fatalf("ratioJitter out of bounds: got %v want in [0, %v]", got, interval/10)
		}
	}
}

func TestRatioJitterZeroRatio(t *testing.T) {
	t.Parallel()
	j := ratioJitter(0)
	if got := j(30 * time.Minute); got != 0 {
		t.Fatalf("ratioJitter(0) got %v want 0", got)
	}
}
```

- [ ] Step 2: テストが通ることを確認する

Run:
```bash
cd /Users/yujiokamoto/devs/golang/feedflow-go-htmx && go test ./internal/poller/ -run 'TestEffectiveInterval|TestDueForPoll|TestRatioJitter' -v
```
Expected: `TestEffectiveInterval` と `TestDueForPoll` と `TestRatioJitterBounds` と `TestRatioJitterZeroRatio` がいずれも PASS します。schedule.goの実装はTask3のStep4で完成済みのため、テスト追加だけで通ります。

- [ ] Step 3: ポーラー全体のテストが通ることを確認する

Run:
```bash
cd /Users/yujiokamoto/devs/golang/feedflow-go-htmx && go test -race ./internal/poller/
```
Expected: `ok  github.com/okamyuji/feedflow-go-htmx/internal/poller` と表示されます。Task3とTask4のテストが両方通ります。

- [ ] Step 4: gofmtを適用してコミットする

```bash
cd /Users/yujiokamoto/devs/golang/feedflow-go-htmx && gofmt -w internal/poller/schedule_test.go && git add internal/poller/schedule_test.go && git commit -m "test: 間隔上書きとジッタの振る舞いを検証する"
```

---

## Task 5: PollAllの期限選別と全体取得(TDD)

Files:
- Create: `/Users/yujiokamoto/devs/golang/feedflow-go-htmx/internal/poller/pollall_test.go`

PollAllが期限の来たフィードだけを取得対象に選び、手動のみのフィードを除外し、未取得や間隔経過のフィードを処理することを検証します。Task3でPollAll本体は実装済みのため、このTaskは振る舞いの検証に集中します。contextが途中でキャンセルされたら処理済み件数を返して止まることも確認します。

- [ ] Step 1: 失敗するテストを書く

Create `internal/poller/pollall_test.go`:
```go
package poller

import (
	"context"
	"testing"
	"time"

	"github.com/okamyuji/feedflow-go-htmx/internal/domain"
	"github.com/okamyuji/feedflow-go-htmx/internal/port"
)

func TestServicePollAllSelectsDueFeeds(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 5, 29, 12, 0, 0, 0, time.UTC)
	repo := newFakeRepo()
	_ = repo.SaveSettings(domain.Settings{
		PollInterval:      domain.Poll30Min,
		MaxItems:          200,
		ReadRetentionDays: 30,
		Theme:             domain.ThemeDark,
		DefaultView:       domain.ViewCard,
	})
	// 期限経過で対象
	_ = repo.SaveFeed(domain.Feed{ID: "due", FeedURL: "https://a.example/feed", PollInterval: domain.Poll30Min, LastFetchedAt: now.Add(-time.Hour)})
	// 間隔未経過で対象外
	_ = repo.SaveFeed(domain.Feed{ID: "fresh", FeedURL: "https://b.example/feed", PollInterval: domain.Poll30Min, LastFetchedAt: now.Add(-time.Minute)})
	// 手動のみで対象外
	_ = repo.SaveFeed(domain.Feed{ID: "manual", FeedURL: "https://c.example/feed", PollInterval: domain.PollManualOnly, LastFetchedAt: now.Add(-100 * time.Hour)})
	// 未取得で対象
	_ = repo.SaveFeed(domain.Feed{ID: "never", FeedURL: "https://d.example/feed", PollInterval: domain.Poll1Hour})

	fetcher := newFakeFetcher()
	fetcher.results["https://a.example/feed"] = port.FetchResult{StatusCode: 200, Body: []byte("<rss></rss>")}
	fetcher.results["https://d.example/feed"] = port.FetchResult{StatusCode: 200, Body: []byte("<rss></rss>")}
	parser := fakeParser{parsed: port.ParsedFeed{}}

	// ジッタを0に固定して決定的に判定する
	svc := NewService(repo, fetcher, parser, newFakeClock(now), &fakeIDGen{}, passthroughMute{})
	svc.jitter = func(time.Duration) time.Duration { return 0 }

	processed, err := svc.PollAll(context.Background())
	if err != nil {
		t.Fatalf("PollAll returned error: %v", err)
	}
	if processed != 2 {
		t.Fatalf("processed got %d want 2", processed)
	}
	if fetcher.calls("https://a.example/feed") != 1 {
		t.Fatalf("due feed must be fetched once, got %d", fetcher.calls("https://a.example/feed"))
	}
	if fetcher.calls("https://d.example/feed") != 1 {
		t.Fatalf("never-fetched feed must be fetched once, got %d", fetcher.calls("https://d.example/feed"))
	}
	if fetcher.calls("https://b.example/feed") != 0 {
		t.Fatalf("fresh feed must not be fetched, got %d", fetcher.calls("https://b.example/feed"))
	}
	if fetcher.calls("https://c.example/feed") != 0 {
		t.Fatalf("manual feed must not be fetched, got %d", fetcher.calls("https://c.example/feed"))
	}
}

func TestServicePollAllContinuesOnError(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 5, 29, 12, 0, 0, 0, time.UTC)
	repo := newFakeRepo()
	_ = repo.SaveSettings(domain.DefaultSettings())
	_ = repo.SaveFeed(domain.Feed{ID: "bad", FeedURL: "https://bad.example/feed", PollInterval: domain.Poll15Min})
	_ = repo.SaveFeed(domain.Feed{ID: "good", FeedURL: "https://good.example/feed", PollInterval: domain.Poll15Min})

	fetcher := newFakeFetcher()
	fetcher.errs["https://bad.example/feed"] = context.DeadlineExceeded
	fetcher.results["https://good.example/feed"] = port.FetchResult{StatusCode: 200, Body: []byte("<rss></rss>")}
	parser := fakeParser{parsed: port.ParsedFeed{}}

	svc := NewService(repo, fetcher, parser, newFakeClock(now), &fakeIDGen{}, passthroughMute{})
	svc.jitter = func(time.Duration) time.Duration { return 0 }

	processed, err := svc.PollAll(context.Background())
	if err != nil {
		t.Fatalf("PollAll returned error: %v", err)
	}
	if processed != 2 {
		t.Fatalf("processed got %d want 2 (both attempted)", processed)
	}
	if fetcher.calls("https://good.example/feed") != 1 {
		t.Fatalf("good feed must still be fetched after bad feed error, got %d", fetcher.calls("https://good.example/feed"))
	}
}

func TestServicePollAllCanceledContext(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 5, 29, 12, 0, 0, 0, time.UTC)
	repo := newFakeRepo()
	_ = repo.SaveSettings(domain.DefaultSettings())
	_ = repo.SaveFeed(domain.Feed{ID: "f1", FeedURL: "https://a.example/feed", PollInterval: domain.Poll15Min})

	svc := NewService(repo, newFakeFetcher(), fakeParser{}, newFakeClock(now), &fakeIDGen{}, passthroughMute{})
	svc.jitter = func(time.Duration) time.Duration { return 0 }

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	processed, err := svc.PollAll(ctx)
	if err == nil {
		t.Fatalf("PollAll must return error for canceled context")
	}
	if processed != 0 {
		t.Fatalf("processed got %d want 0 on immediate cancel", processed)
	}
}
```

- [ ] Step 2: テストが通ることを確認する

Run:
```bash
cd /Users/yujiokamoto/devs/golang/feedflow-go-htmx && go test ./internal/poller/ -run 'TestServicePollAll' -v
```
Expected: `TestServicePollAllSelectsDueFeeds` と `TestServicePollAllContinuesOnError` と `TestServicePollAllCanceledContext` がいずれも PASS します。PollAll本体はTask3で実装済みのため新規実装は不要で、振る舞いがそろっていることを確認します。

- [ ] Step 3: gofmtを適用してコミットする

```bash
cd /Users/yujiokamoto/devs/golang/feedflow-go-htmx && gofmt -w internal/poller/pollall_test.go && git add internal/poller/pollall_test.go && git commit -m "test: PollAll の期限選別と継続と中断を検証する"
```

---

## Task 6: 並行取得数の制限つき取得(TDD)

Files:
- Create: `/Users/yujiokamoto/devs/golang/feedflow-go-htmx/internal/poller/runner.go`

バックグラウンドスケジューラRunnerの骨格と、並行取得数を制限して複数フィードを取得するpollDueメソッドを実装します。pollDueは期限の来たフィードを集め、重み付きセマフォとしてバッファ付きチャネルで同時実行数を上限以下に抑えながらgoroutineで取得します。観測可能性のため、同時実行のピークを記録できるよう取得関数を差し替え可能にします。

- [ ] Step 1: 失敗するテストを書く

Create `internal/poller/runner_concurrency_test.go`:
```go
package poller

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/okamyuji/feedflow-go-htmx/internal/domain"
)

func TestRunnerPollDueRespectsConcurrencyLimit(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 5, 29, 12, 0, 0, 0, time.UTC)
	repo := newFakeRepo()
	_ = repo.SaveSettings(domain.Settings{
		PollInterval:      domain.Poll15Min,
		MaxItems:          200,
		ReadRetentionDays: 30,
		Theme:             domain.ThemeDark,
		DefaultView:       domain.ViewCard,
	})
	const feedCount = 10
	for i := 0; i < feedCount; i++ {
		id := "f" + string(rune('a'+i))
		_ = repo.SaveFeed(domain.Feed{ID: id, FeedURL: "https://" + id + ".example/feed", PollInterval: domain.Poll15Min})
	}

	svc := NewService(repo, newFakeFetcher(), fakeParser{}, newFakeClock(now), &fakeIDGen{}, passthroughMute{})
	svc.jitter = func(time.Duration) time.Duration { return 0 }

	cfg := DefaultConfig()
	cfg.MaxConcurrent = 3
	runner := NewRunner(svc, repo, newFakeClock(now), cfg)

	var inFlight int32
	var peak int32
	var mu sync.Mutex
	// 取得関数を差し替え、同時実行のピークを観測する
	runner.pollOne = func(_ context.Context, _ string) {
		cur := atomic.AddInt32(&inFlight, 1)
		mu.Lock()
		if cur > peak {
			peak = cur
		}
		mu.Unlock()
		time.Sleep(5 * time.Millisecond)
		atomic.AddInt32(&inFlight, -1)
	}

	processed := runner.pollDue(context.Background())
	if processed != feedCount {
		t.Fatalf("processed got %d want %d", processed, feedCount)
	}
	if peak > int32(cfg.MaxConcurrent) {
		t.Fatalf("concurrency peak got %d want <= %d", peak, cfg.MaxConcurrent)
	}
	if peak == 0 {
		t.Fatalf("concurrency peak got 0 want > 0")
	}
}

func TestRunnerPollDueCanceledBeforeStart(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 5, 29, 12, 0, 0, 0, time.UTC)
	repo := newFakeRepo()
	_ = repo.SaveSettings(domain.DefaultSettings())
	_ = repo.SaveFeed(domain.Feed{ID: "f1", FeedURL: "https://f1.example/feed", PollInterval: domain.Poll15Min})

	svc := NewService(repo, newFakeFetcher(), fakeParser{}, newFakeClock(now), &fakeIDGen{}, passthroughMute{})
	svc.jitter = func(time.Duration) time.Duration { return 0 }
	runner := NewRunner(svc, repo, newFakeClock(now), DefaultConfig())

	var calls int32
	runner.pollOne = func(_ context.Context, _ string) { atomic.AddInt32(&calls, 1) }

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	processed := runner.pollDue(ctx)
	if processed != 0 {
		t.Fatalf("processed got %d want 0 on canceled context", processed)
	}
	if atomic.LoadInt32(&calls) != 0 {
		t.Fatalf("pollOne calls got %d want 0", calls)
	}
}
```

- [ ] Step 2: テストが失敗することを確認する

Run:
```bash
cd /Users/yujiokamoto/devs/golang/feedflow-go-htmx && go test ./internal/poller/ -run 'TestRunnerPollDue' -v
```
Expected: コンパイルエラーで失敗します。`undefined: NewRunner` や `runner.pollOne undefined` と表示されます。

- [ ] Step 3: Runner の実装を書く

Create `internal/poller/runner.go`:
```go
package poller

import (
	"context"
	"sync"

	"github.com/okamyuji/feedflow-go-htmx/internal/port"
)

// Runner バックグラウンドで期限の来たフィードを定期取得するスケジューラです。
// svcとrepoとclockをコンストラクタ注入で受け取り、設定に従って巡回します。
type Runner struct {
	svc   port.PollService
	repo  port.Repository
	clock port.Clock
	cfg   Config

	// pollOne 1フィードを取得する関数です。既定ではsvc.PollFeedを呼びます。
	// テストでは同時実行の観測のために差し替えます。
	pollOne func(ctx context.Context, feedID string)
}

// NewRunner 依存と設定を注入してRunnerを生成します。設定はゼロ値や不正値を既定へ補正します。
func NewRunner(svc port.PollService, repo port.Repository, clock port.Clock, cfg Config) *Runner {
	r := &Runner{
		svc:   svc,
		repo:  repo,
		clock: clock,
		cfg:   cfg.normalize(),
	}
	r.pollOne = func(ctx context.Context, feedID string) {
		_, _ = r.svc.PollFeed(ctx, feedID)
	}
	return r
}

// dueFeedIDs 現時点で取得対象のフィードID群を返します。
// 期限判定はServiceが用いるのと同じeffectiveIntervalとLastFetchedAtの規則に従います。
// ジッタはRunnerでは掛けず、間隔ちょうどの経過で対象にします。
func (r *Runner) dueFeedIDs() ([]string, error) {
	feeds, err := r.repo.Feeds()
	if err != nil {
		return nil, err
	}
	settings, err := r.repo.Settings()
	if err != nil {
		return nil, err
	}
	now := r.clock.Now()
	ids := make([]string, 0, len(feeds))
	for _, f := range feeds {
		if dueForPollWithJitter(f, settings, now, func(d ...interface{}) {}) {
			ids = append(ids, f.ID)
		}
	}
	return ids, nil
}

// pollDue 期限の来た全フィードを並行数制限つきで取得し、処理したフィード数を返します。
// context がキャンセルされたら新規の取得を開始せず、開始済みの取得の完了を待ってから戻ります。
func (r *Runner) pollDue(ctx context.Context) int {
	if err := ctx.Err(); err != nil {
		return 0
	}
	ids, err := r.dueFeedIDs()
	if err != nil {
		return 0
	}

	sem := make(chan struct{}, r.cfg.MaxConcurrent)
	var wg sync.WaitGroup
	processed := 0
	for _, id := range ids {
		if err := ctx.Err(); err != nil {
			break
		}
		processed++
		wg.Add(1)
		sem <- struct{}{}
		go func(feedID string) {
			defer wg.Done()
			defer func() { <-sem }()
			r.pollOne(ctx, feedID)
		}(id)
	}
	wg.Wait()
	return processed
}
```
補足: このStep3のdueFeedIDsは`dueForPollWithJitter`へ不正な引数を渡しており型として通りません。次のStep4で正しい呼び出しへ差し替えます。

- [ ] Step 4: dueFeedIDsのジッタ引数を正しい関数へ差し替える

`internal/poller/runner.go` の `dueFeedIDs` 内のループを次へ置き換えます。Runnerは間隔ちょうどで判定するため、ジッタを常に0返す関数を渡します。

```go
	zeroJitter := func(_ time.Duration) time.Duration { return 0 }
	for _, f := range feeds {
		if dueForPollWithJitter(f, settings, now, zeroJitter) {
			ids = append(ids, f.ID)
		}
	}
```

あわせてファイル先頭の import に `time` を追加します。import 句を次へ置き換えます。
```go
import (
	"context"
	"sync"
	"time"

	"github.com/okamyuji/feedflow-go-htmx/internal/port"
)
```

- [ ] Step 5: テストが通ることを確認する

Run:
```bash
cd /Users/yujiokamoto/devs/golang/feedflow-go-htmx && go test -race ./internal/poller/ -run 'TestRunnerPollDue' -v
```
Expected: `TestRunnerPollDueRespectsConcurrencyLimit` と `TestRunnerPollDueCanceledBeforeStart` がいずれも PASS します。-raceでデータ競合が報告されないことも確認します。

- [ ] Step 6: gofmtを適用してコミットする

```bash
cd /Users/yujiokamoto/devs/golang/feedflow-go-htmx && gofmt -w internal/poller/runner.go internal/poller/runner_concurrency_test.go && git add internal/poller/runner.go internal/poller/runner_concurrency_test.go && git commit -m "feat: 並行取得数の制限つき取得を Runner に追加する"
```

---

## Task 7: Runnerの定期巡回とグレースフル停止(TDD)

Files:
- Modify: `/Users/yujiokamoto/devs/golang/feedflow-go-htmx/internal/poller/runner.go`

time.Tickerで設定間隔ごとにpollDueを呼ぶRunメソッドと、手動更新で1フィードを即時取得するPollNowメソッドを実装します。Runは渡されたcontextがキャンセルされるまで巡回を続け、キャンセル時はTickerを止めて進行中の取得の完了を待ってから戻ります。これによりプロセス終了時にgoroutineを残さず安全に停止します。

- [ ] Step 1: 失敗するテストを書く

Create `internal/poller/runner_run_test.go`:
```go
package poller

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/okamyuji/feedflow-go-htmx/internal/domain"
)

func TestRunnerRunStopsOnContextCancel(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 5, 29, 12, 0, 0, 0, time.UTC)
	repo := newFakeRepo()
	_ = repo.SaveSettings(domain.DefaultSettings())
	_ = repo.SaveFeed(domain.Feed{ID: "f1", FeedURL: "https://f1.example/feed", PollInterval: domain.Poll15Min})

	svc := NewService(repo, newFakeFetcher(), fakeParser{}, newFakeClock(now), &fakeIDGen{}, passthroughMute{})
	svc.jitter = func(time.Duration) time.Duration { return 0 }

	cfg := DefaultConfig()
	cfg.TickInterval = 5 * time.Millisecond
	runner := NewRunner(svc, repo, newFakeClock(now), cfg)

	var ticks int32
	runner.pollOne = func(_ context.Context, _ string) { atomic.AddInt32(&ticks, 1) }

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		runner.Run(ctx)
		close(done)
	}()

	// 何度か巡回するのを待ってからキャンセルする
	time.Sleep(40 * time.Millisecond)
	cancel()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatalf("Run did not return within timeout after cancel")
	}

	if atomic.LoadInt32(&ticks) == 0 {
		t.Fatalf("Run must have polled at least once before cancel")
	}
}

func TestRunnerRunReturnsImmediatelyIfPreCanceled(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 5, 29, 12, 0, 0, 0, time.UTC)
	repo := newFakeRepo()
	_ = repo.SaveSettings(domain.DefaultSettings())

	svc := NewService(repo, newFakeFetcher(), fakeParser{}, newFakeClock(now), &fakeIDGen{}, passthroughMute{})
	runner := NewRunner(svc, repo, newFakeClock(now), DefaultConfig())

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	done := make(chan struct{})
	go func() {
		runner.Run(ctx)
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatalf("Run did not return promptly for pre-canceled context")
	}
}

func TestRunnerPollNow(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 5, 29, 12, 0, 0, 0, time.UTC)
	repo := newFakeRepo()
	_ = repo.SaveFeed(domain.Feed{ID: "f1", FeedURL: "https://f1.example/feed"})

	fetcher := newFakeFetcher()
	fetcher.results["https://f1.example/feed"] = port.FetchResult{StatusCode: 200, Body: []byte("<rss></rss>")}
	parser := fakeParser{parsed: port.ParsedFeed{Items: []port.ParsedItem{{GUID: "g1", Title: "新着"}}}}

	svc := NewService(repo, fetcher, parser, newFakeClock(now), &fakeIDGen{}, passthroughMute{})
	runner := NewRunner(svc, repo, newFakeClock(now), DefaultConfig())

	n, err := runner.PollNow(context.Background(), "f1")
	if err != nil {
		t.Fatalf("PollNow returned error: %v", err)
	}
	if n != 1 {
		t.Fatalf("PollNow new item count got %d want 1", n)
	}
	if fetcher.calls("https://f1.example/feed") != 1 {
		t.Fatalf("PollNow must fetch the feed immediately, got %d", fetcher.calls("https://f1.example/feed"))
	}
}
```
補足: このテストファイルは `port` を参照するため、import に `"github.com/okamyuji/feedflow-go-htmx/internal/port"` を含めます。次のStepで提示する実装ではテストにこのimportを加えます。

- [ ] Step 2: テストのimportを補う

`internal/poller/runner_run_test.go` のimport句を次へ置き換えます。`port` パッケージを参照するためです。
```go
import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/okamyuji/feedflow-go-htmx/internal/domain"
	"github.com/okamyuji/feedflow-go-htmx/internal/port"
)
```

- [ ] Step 3: テストが失敗することを確認する

Run:
```bash
cd /Users/yujiokamoto/devs/golang/feedflow-go-htmx && go test ./internal/poller/ -run 'TestRunnerRun|TestRunnerPollNow' -v
```
Expected: コンパイルエラーで失敗します。`runner.Run undefined` や `runner.PollNow undefined` と表示されます。

- [ ] Step 4: RunとPollNowを実装する

`internal/poller/runner.go` の末尾へ次の2メソッドを追記します。

```go
// Run contextがキャンセルされるまで設定間隔ごとに期限フィードを取得し続けます。
// 起動直後に一度巡回し、その後はTickerの刻みごとに巡回します。
// contextのキャンセルでTickerを止め、進行中の取得の完了を待ってから戻ります。
func (r *Runner) Run(ctx context.Context) {
	if err := ctx.Err(); err != nil {
		return
	}
	r.pollDue(ctx)

	ticker := time.NewTicker(r.cfg.TickInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			r.pollDue(ctx)
		}
	}
}

// PollNow 手動更新で指定フィードを即時取得し、新着件数を返します。
// 定期巡回の期限判定を経由せず、その場で取得反映を行います。
func (r *Runner) PollNow(ctx context.Context, feedID string) (int, error) {
	n, err := r.svc.PollFeed(ctx, feedID)
	if err != nil {
		return 0, fmt.Errorf("failed to poll feed now %q: %w", feedID, err)
	}
	return n, nil
}
```

あわせてファイル先頭の import に `fmt` を追加します。import 句を次へ置き換えます。
```go
import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/okamyuji/feedflow-go-htmx/internal/port"
)
```

- [ ] Step 5: テストが通ることを確認する

Run:
```bash
cd /Users/yujiokamoto/devs/golang/feedflow-go-htmx && go test -race ./internal/poller/ -run 'TestRunnerRun|TestRunnerPollNow' -v
```
Expected: `TestRunnerRunStopsOnContextCancel` と `TestRunnerRunReturnsImmediatelyIfPreCanceled` と `TestRunnerPollNow` がいずれも PASS します。-raceでデータ競合が報告されないことも確認します。

- [ ] Step 6: gofmtを適用してコミットする

```bash
cd /Users/yujiokamoto/devs/golang/feedflow-go-htmx && gofmt -w internal/poller/runner.go internal/poller/runner_run_test.go && git add internal/poller/runner.go internal/poller/runner_run_test.go && git commit -m "feat: Runner の定期巡回とグレースフル停止と手動更新を追加する"
```

---

## Task 8: フェーズ全体のテストとカバレッジと品質ゲート

Files:
- 変更なし

- [ ] Step 1: ポーラーの全テストを race で実行する

Run:
```bash
cd /Users/yujiokamoto/devs/golang/feedflow-go-htmx && go test -race -count=1 -shuffle=on ./internal/poller/...
```
Expected: `ok  github.com/okamyuji/feedflow-go-htmx/internal/poller` と表示されます。shuffleとraceでも安定して通ります。

- [ ] Step 2: カバレッジを確認する

Run:
```bash
cd /Users/yujiokamoto/devs/golang/feedflow-go-htmx && go test -coverprofile=coverage.out ./internal/poller/... && go tool cover -func=coverage.out | tail -n 1
```
Expected: pollerパッケージの主要関数(PollFeed、PollAll、applyParsed、effectiveInterval、dueForPollWithJitter、ratioJitter、pollDue、Run、PollNow)を網羅し、合計カバレッジが80パーセント前後以上になります。目安であり厳密な合否基準ではありません。

- [ ] Step 3: 品質ゲートを実行する

Run:
```bash
cd /Users/yujiokamoto/devs/golang/feedflow-go-htmx && bash scripts/quality-gate.sh
```
Expected: `all quality checks passed` で終わります。golangci-lintのcontextcheckやnoctxの指摘が出た場合は、Fetchへ渡すcontextが呼び出し元から伝播されているかを確認して修正します。

- [ ] Step 4: 品質ゲート緑のままコミットする

```bash
cd /Users/yujiokamoto/devs/golang/feedflow-go-htmx && git add -A && git commit -m "chore: Phase5 のポーラーで品質ゲートを緑化する"
```
Expected: コミット時に quality-gate が走り、緑のままコミットされます。差分が無ければこのコミットは省略できます。

---

## Phase5 完了条件

- [ ] `go test -race -shuffle=on ./internal/poller/...` が通る
- [ ] poller.Serviceがport.PollServiceを満たし、PollFeedが新着反映と未更新と取得失敗をそれぞれ正しく扱う
- [ ] フィードごとのポーリング間隔上書き(default、15m、30m、1h、6h、manual)がeffectiveIntervalで反映される
- [ ] ジッタがratioJitterで間隔の割合内に収まり、dueForPollWithJitterで前倒し取得を許す
- [ ] Runner.pollDueが並行取得数を上限以下に抑える(-raceで競合なし)
- [ ] Runner.Runがcontextキャンセルでグレースフルに停止し、PollNowで手動更新の即時取得ができる
- [ ] Clockとフェイクfetcherとフェイクparserとフェイクrepoの注入だけでユニットテストが完結する
- [ ] `bash scripts/quality-gate.sh` が `all quality checks passed` で終わる
- [ ] コミットが規約に沿って積まれている
