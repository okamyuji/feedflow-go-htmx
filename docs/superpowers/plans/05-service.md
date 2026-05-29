# Phase4サービス層 実装計画

> For agentic workers: REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

Goal: 設計書のセクション3と4と5に基づき、internal/serviceに購読管理、記事操作、保持ポリシー適用、ミュートフィルタ適用、OPML入出力、設定の取得と更新を実装し、port.SubscriptionServiceとport.ItemServiceとport.RetentionServiceとport.MuteServiceとport.OPMLServiceとport.SettingsServiceをすべて満たします。port.Repositoryとport.Fetcherとport.FeedParserとport.Clockとport.IDGenをフェイク注入してテーブル駆動テストで検証します。

Architecture: クリーンアーキテクチャの業務ロジック層を作ります。internal/serviceの各サービスはinternal/portのインターフェースにのみ依存し、具象型に直接依存しません。依存はコンストラクタ注入で渡します。サービスはdomainの純粋関数(ShouldRetain、Matches、HasUserAction、Settings.Valid)を組み合わせて副作用のあるロジックを構成します。外部I/Oは永続化(port.Repository)、HTTP取得(port.Fetcher)、パース(port.FeedParser)、時刻(port.Clock)、ID生成(port.IDGen)のインターフェース経由でのみ触れます。テストではこれらのフェイクを注入し、ネットワークやファイルに触れずに検証します。port.PollServiceはPhase5のinternal/pollerが担うためこのフェーズでは扱いません。SubscribeFromSiteで使うサイトHTMLからのfeed link検出は、追加I/Oポートを作らずport.Fetcherで取得したHTMLをサービス層がgolang.org/x/net/htmlで解析して行います。

Tech Stack: Go 1.25(標準ライブラリ中心)。OPMLの入出力はencoding/xmlを使います。サイトHTMLのfeed link検出はgolang.org/x/net/htmlを使います。golang.org/x/net自体はPhase3で`go get`済みの前提です。テストは標準のtestingでテーブル駆動とし、-raceで通る前提で書きます。

前提: 作業ディレクトリは`/Users/yujiokamoto/devs/golang/feedflow-go-htmx`です。Phase1とPhase2とPhase3が完了し`bash scripts/quality-gate.sh`が緑であることを確認してから始めます。module pathは`github.com/okamyuji/feedflow-go-htmx`です。internal/domainとinternal/portの型とインターフェースは確定済みで、シグネチャを変更しません。

このフェーズで追加する補助型は次の3つです。いずれもPhase1の確定定義と矛盾しません。

- `service.fakeRepo`とそのほかのフェイクは各テストファイル内のテスト専用型です。Phase1のport_test.goのフェイクと同型ですが、サービステストは別パッケージのためテスト側で再定義します
- OPML入出力のためのXMLマッピング型`opmlDocument`、`opmlHead`、`opmlBody`、`opmlOutline`をinternal/service/opml.goに定義します。これらはencoding/xmlのタグを持つ非公開の中間表現で、domainやportの型には影響しません
- サイトHTMLからのfeed link検出に使う非公開関数`discoverFeedURL`をinternal/service/subscription.goに定義します。port.Fetcherで取得したHTMLバイト列を入力に取り、最初に見つかったRSSまたはAtomのfeed link URLを返します。新しいインターフェースは追加しません

---

## Task 1: サービス共通の依存とSubscriptionServiceの骨格

Files:
- Create: `/Users/yujiokamoto/devs/golang/feedflow-go-htmx/internal/service/service.go`

各サービスが共有する依存(port.Repository、port.Fetcher、port.FeedParser、port.Clock、port.IDGen)をまとめたDeps構造体を定義し、コンストラクタ注入の入口を一本化します。これにより各サービスのコンストラクタが冗長にならず、テストでもフェイク一式を一度に渡せます。

- [ ] Step 1: 共通の依存型を作成する

Create `internal/service/service.go`:
```go
// Package service feedflowの業務ロジックを提供します。
// 各サービスはinternal/portのインターフェースにのみ依存し、具象型に直接依存しません。
// 依存はコンストラクタ注入で受け取ります。設計書のセクション5.2に対応します。
package service

import "github.com/okamyuji/feedflow-go-htmx/internal/port"

// Deps 各サービスが共有する依存をまとめた束です。
// 外部I/Oはすべてこのインターフェース群経由で行い、テストではフェイクを注入します。
type Deps struct {
	Repo   port.Repository // 永続化境界です
	Fetch  port.Fetcher    // HTTP取得境界です
	Parse  port.FeedParser // フィードのパース境界です
	Clock  port.Clock      // 時刻取得境界です
	IDs    port.IDGen      // ID生成境界です
}
```

- [ ] Step 2: コンパイルが通ることを確認する

Run:
```bash
cd /Users/yujiokamoto/devs/golang/feedflow-go-htmx && go build ./internal/service/
```
Expected: エラーなく完了します。

- [ ] Step 3: gofmtを適用する

Run:
```bash
cd /Users/yujiokamoto/devs/golang/feedflow-go-htmx && gofmt -w internal/service/service.go
```
Expected: エラーなく完了します。

- [ ] Step 4: コミットする

```bash
cd /Users/yujiokamoto/devs/golang/feedflow-go-htmx && git add internal/service/service.go && git commit -m "feat: サービス共通の依存束Depsを追加する"
```

---

## Task 2: サービステスト共通のフェイク実装

Files:
- Create: `/Users/yujiokamoto/devs/golang/feedflow-go-htmx/internal/service/fakes_test.go`

サービステストはservice_testパッケージから公開APIを叩く形にし、port.Repositoryとport.Fetcherとport.FeedParserとport.Clockとport.IDGenのフェイクをテスト専用に定義します。Phase1のport_test.goと同型ですが、エラー注入と呼び出し記録ができるよう拡張します。これによりI/Oに触れずにサービスの分岐とエラー伝播を検証できます。

- [ ] Step 1: フェイク一式を作成する

Create `internal/service/fakes_test.go`:
```go
package service_test

import (
	"context"
	"errors"
	"strconv"
	"time"

	"github.com/okamyuji/feedflow-go-htmx/internal/domain"
	"github.com/okamyuji/feedflow-go-htmx/internal/port"
	"github.com/okamyuji/feedflow-go-htmx/internal/service"
)

// errNotFound フェイクリポジトリが対象を見つけられなかったときに返すエラーです。
var errNotFound = errors.New("not found")

// fakeClock 固定時刻を返すClockのフェイクです。
type fakeClock struct{ now time.Time }

func (c fakeClock) Now() time.Time { return c.now }

// fakeIDGen 決定的な連番IDを返すIDGenのフェイクです。
type fakeIDGen struct{ n int }

func (g *fakeIDGen) NewID() string {
	g.n++
	return "id-" + strconv.Itoa(g.n)
}

// fakeFetcher URLごとに固定の結果を返すFetcherのフェイクです。
// results に登録のないURLが来たときはerrNotFoundを返します。
type fakeFetcher struct {
	results map[string]port.FetchResult
	err     error
	calls   []port.FetchRequest
}

func newFakeFetcher() *fakeFetcher {
	return &fakeFetcher{results: map[string]port.FetchResult{}}
}

func (f *fakeFetcher) Fetch(_ context.Context, req port.FetchRequest) (port.FetchResult, error) {
	f.calls = append(f.calls, req)
	if f.err != nil {
		return port.FetchResult{}, f.err
	}
	res, ok := f.results[req.URL]
	if !ok {
		return port.FetchResult{}, errNotFound
	}
	return res, nil
}

// fakeParser バイト列ごとに固定の結果を返すFeedParserのフェイクです。
// parsed をそのまま返し、errが設定されていればエラーを返します。
type fakeParser struct {
	parsed port.ParsedFeed
	err    error
}

func (p fakeParser) Parse(_ []byte) (port.ParsedFeed, error) {
	if p.err != nil {
		return port.ParsedFeed{}, p.err
	}
	return p.parsed, nil
}

// fakeRepo メモリ上で全エンティティを保持するRepositoryのフェイクです。
// 任意のメソッドにエラーを差し込めるようfailOnを持ちます。
type fakeRepo struct {
	feeds      map[string]domain.Feed
	feedOrder  []string
	categories map[string]domain.Category
	items      map[string][]domain.Item
	boards     map[string]domain.Board
	filters    map[string]domain.MuteFilter
	settings   domain.Settings
	user       domain.User
	failOn     map[string]error
}

func newFakeRepo() *fakeRepo {
	return &fakeRepo{
		feeds:      map[string]domain.Feed{},
		categories: map[string]domain.Category{},
		items:      map[string][]domain.Item{},
		boards:     map[string]domain.Board{},
		filters:    map[string]domain.MuteFilter{},
		settings:   domain.DefaultSettings(),
		failOn:     map[string]error{},
	}
}

func (r *fakeRepo) fail(method string) error { return r.failOn[method] }

func (r *fakeRepo) Feeds() ([]domain.Feed, error) {
	if err := r.fail("Feeds"); err != nil {
		return nil, err
	}
	out := make([]domain.Feed, 0, len(r.feeds))
	for _, id := range r.feedOrder {
		out = append(out, r.feeds[id])
	}
	return out, nil
}

func (r *fakeRepo) Feed(id string) (domain.Feed, error) {
	if err := r.fail("Feed"); err != nil {
		return domain.Feed{}, err
	}
	f, ok := r.feeds[id]
	if !ok {
		return domain.Feed{}, errNotFound
	}
	return f, nil
}

func (r *fakeRepo) SaveFeed(feed domain.Feed) error {
	if err := r.fail("SaveFeed"); err != nil {
		return err
	}
	if _, exists := r.feeds[feed.ID]; !exists {
		r.feedOrder = append(r.feedOrder, feed.ID)
	}
	r.feeds[feed.ID] = feed
	return nil
}

func (r *fakeRepo) DeleteFeed(id string) error {
	if err := r.fail("DeleteFeed"); err != nil {
		return err
	}
	delete(r.feeds, id)
	delete(r.items, id)
	for i, fid := range r.feedOrder {
		if fid == id {
			r.feedOrder = append(r.feedOrder[:i], r.feedOrder[i+1:]...)
			break
		}
	}
	return nil
}

func (r *fakeRepo) Categories() ([]domain.Category, error) {
	if err := r.fail("Categories"); err != nil {
		return nil, err
	}
	out := make([]domain.Category, 0, len(r.categories))
	for _, c := range r.categories {
		out = append(out, c)
	}
	return out, nil
}

func (r *fakeRepo) SaveCategory(category domain.Category) error {
	if err := r.fail("SaveCategory"); err != nil {
		return err
	}
	r.categories[category.ID] = category
	return nil
}

func (r *fakeRepo) DeleteCategory(id string) error {
	if err := r.fail("DeleteCategory"); err != nil {
		return err
	}
	delete(r.categories, id)
	return nil
}

func (r *fakeRepo) Items(feedID string) ([]domain.Item, error) {
	if err := r.fail("Items"); err != nil {
		return nil, err
	}
	return r.items[feedID], nil
}

func (r *fakeRepo) SaveItems(feedID string, items []domain.Item) error {
	if err := r.fail("SaveItems"); err != nil {
		return err
	}
	r.items[feedID] = items
	return nil
}

func (r *fakeRepo) Boards() ([]domain.Board, error) {
	if err := r.fail("Boards"); err != nil {
		return nil, err
	}
	out := make([]domain.Board, 0, len(r.boards))
	for _, b := range r.boards {
		out = append(out, b)
	}
	return out, nil
}

func (r *fakeRepo) SaveBoard(board domain.Board) error {
	if err := r.fail("SaveBoard"); err != nil {
		return err
	}
	r.boards[board.ID] = board
	return nil
}

func (r *fakeRepo) DeleteBoard(id string) error {
	if err := r.fail("DeleteBoard"); err != nil {
		return err
	}
	delete(r.boards, id)
	return nil
}

func (r *fakeRepo) Filters() ([]domain.MuteFilter, error) {
	if err := r.fail("Filters"); err != nil {
		return nil, err
	}
	out := make([]domain.MuteFilter, 0, len(r.filters))
	for _, f := range r.filters {
		out = append(out, f)
	}
	return out, nil
}

func (r *fakeRepo) SaveFilter(filter domain.MuteFilter) error {
	if err := r.fail("SaveFilter"); err != nil {
		return err
	}
	r.filters[filter.ID] = filter
	return nil
}

func (r *fakeRepo) DeleteFilter(id string) error {
	if err := r.fail("DeleteFilter"); err != nil {
		return err
	}
	delete(r.filters, id)
	return nil
}

func (r *fakeRepo) Settings() (domain.Settings, error) {
	if err := r.fail("Settings"); err != nil {
		return domain.Settings{}, err
	}
	return r.settings, nil
}

func (r *fakeRepo) SaveSettings(settings domain.Settings) error {
	if err := r.fail("SaveSettings"); err != nil {
		return err
	}
	r.settings = settings
	return nil
}

func (r *fakeRepo) User() (domain.User, error) {
	if err := r.fail("User"); err != nil {
		return domain.User{}, err
	}
	return r.user, nil
}

func (r *fakeRepo) SaveUser(user domain.User) error {
	if err := r.fail("SaveUser"); err != nil {
		return err
	}
	r.user = user
	return nil
}

// newDeps テスト用にフェイク一式を組んだDepsを返します。
func newDeps(repo *fakeRepo, fetch *fakeFetcher, parse fakeParser, now time.Time, ids *fakeIDGen) service.Deps {
	return service.Deps{
		Repo:  repo,
		Fetch: fetch,
		Parse: parse,
		Clock: fakeClock{now: now},
		IDs:   ids,
	}
}

// インターフェース充足をコンパイル時に検証します。
var (
	_ port.Clock      = fakeClock{}
	_ port.IDGen      = (*fakeIDGen)(nil)
	_ port.Fetcher    = (*fakeFetcher)(nil)
	_ port.FeedParser = fakeParser{}
	_ port.Repository = (*fakeRepo)(nil)
)
```

- [ ] Step 2: フェイクがコンパイルとビルドを通ることを確認する

Run:
```bash
cd /Users/yujiokamoto/devs/golang/feedflow-go-htmx && go vet ./internal/service/
```
Expected: エラーなく完了します。テストファイルのみの追加なので本体ビルドには影響しません。

- [ ] Step 3: gofmtを適用してコミットする

```bash
cd /Users/yujiokamoto/devs/golang/feedflow-go-htmx && gofmt -w internal/service/fakes_test.go && git add internal/service/fakes_test.go && git commit -m "test: サービステスト共通のフェイク実装を追加する"
```

---

## Task 3: SettingsServiceの取得と更新

Files:
- Create: `/Users/yujiokamoto/devs/golang/feedflow-go-htmx/internal/service/settings.go`
- Test: `/Users/yujiokamoto/devs/golang/feedflow-go-htmx/internal/service/settings_test.go`

設計書セクション4のとおり、設定の取得と更新を担います。更新時はdomain.Settings.Validで妥当性を検証し、不正値は保存せずエラーを返します。port.SettingsServiceを満たします。最も依存の少ないサービスから着手します。

- [ ] Step 1: 失敗するテストを書く

Create `internal/service/settings_test.go`:
```go
package service_test

import (
	"testing"
	"time"

	"github.com/okamyuji/feedflow-go-htmx/internal/domain"
	"github.com/okamyuji/feedflow-go-htmx/internal/port"
	"github.com/okamyuji/feedflow-go-htmx/internal/service"
)

var _ port.SettingsService = (*service.SettingsService)(nil)

func TestSettingsServiceGet(t *testing.T) {
	t.Parallel()
	repo := newFakeRepo()
	want := domain.DefaultSettings()
	want.MaxItems = 123
	repo.settings = want
	svc := service.NewSettingsService(newDeps(repo, newFakeFetcher(), fakeParser{}, time.Now(), &fakeIDGen{}))

	got, err := svc.Get()
	if err != nil {
		t.Fatalf("Get returned error: %v", err)
	}
	if got.MaxItems != 123 {
		t.Fatalf("MaxItems got %d want 123", got.MaxItems)
	}
}

func TestSettingsServiceGetRepoError(t *testing.T) {
	t.Parallel()
	repo := newFakeRepo()
	repo.failOn["Settings"] = errNotFound
	svc := service.NewSettingsService(newDeps(repo, newFakeFetcher(), fakeParser{}, time.Now(), &fakeIDGen{}))

	if _, err := svc.Get(); err == nil {
		t.Fatalf("Get must return error when repo fails")
	}
}

func TestSettingsServiceUpdate(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name      string
		settings  domain.Settings
		wantErr   bool
		wantSaved bool
	}{
		{
			name:      "妥当な設定は保存する",
			settings:  domain.DefaultSettings(),
			wantErr:   false,
			wantSaved: true,
		},
		{
			name: "件数0は不正で保存しない",
			settings: func() domain.Settings {
				s := domain.DefaultSettings()
				s.MaxItems = 0
				return s
			}(),
			wantErr:   true,
			wantSaved: false,
		},
		{
			name: "テーマ 不正値は保存しない",
			settings: func() domain.Settings {
				s := domain.DefaultSettings()
				s.Theme = "neon"
				return s
			}(),
			wantErr:   true,
			wantSaved: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			repo := newFakeRepo()
			repo.settings = domain.Settings{} // 保存有無を見分けるためゼロ値から始めます
			svc := service.NewSettingsService(newDeps(repo, newFakeFetcher(), fakeParser{}, time.Now(), &fakeIDGen{}))

			err := svc.Update(tt.settings)
			if tt.wantErr && err == nil {
				t.Fatalf("Update must return error")
			}
			if !tt.wantErr && err != nil {
				t.Fatalf("Update returned error: %v", err)
			}
			saved := repo.settings != domain.Settings{}
			if saved != tt.wantSaved {
				t.Fatalf("saved got %v want %v", saved, tt.wantSaved)
			}
		})
	}
}
```

- [ ] Step 2: テストが失敗することを確認する

Run:
```bash
cd /Users/yujiokamoto/devs/golang/feedflow-go-htmx && go test ./internal/service/ -run TestSettingsService -v
```
Expected: コンパイルエラーで失敗します。`undefined: service.SettingsService` や `undefined: service.NewSettingsService` と表示されます。

- [ ] Step 3: SettingsServiceの最小実装を書く

Create `internal/service/settings.go`:
```go
package service

import (
	"errors"
	"fmt"

	"github.com/okamyuji/feedflow-go-htmx/internal/domain"
)

// ErrInvalidSettings 設定値が妥当でないときに返すエラーです。
var ErrInvalidSettings = errors.New("invalid settings")

// SettingsService 設定の取得と更新を担います。port.SettingsServiceを満たします。
type SettingsService struct {
	deps Deps
}

// NewSettingsService 依存束を受け取りSettingsServiceを構築します。
func NewSettingsService(deps Deps) *SettingsService {
	return &SettingsService{deps: deps}
}

// Get 現在の設定を返します。
func (s *SettingsService) Get() (domain.Settings, error) {
	settings, err := s.deps.Repo.Settings()
	if err != nil {
		return domain.Settings{}, fmt.Errorf("failed to load settings: %w", err)
	}
	return settings, nil
}

// Update 設定を検証してから保存します。妥当でない場合は保存せずエラーを返します。
func (s *SettingsService) Update(settings domain.Settings) error {
	if !settings.Valid() {
		return ErrInvalidSettings
	}
	if err := s.deps.Repo.SaveSettings(settings); err != nil {
		return fmt.Errorf("failed to save settings: %w", err)
	}
	return nil
}
```

- [ ] Step 4: テストが通ることを確認する

Run:
```bash
cd /Users/yujiokamoto/devs/golang/feedflow-go-htmx && go test ./internal/service/ -run TestSettingsService -v
```
Expected: `TestSettingsServiceGet`と`TestSettingsServiceGetRepoError`と`TestSettingsServiceUpdate`がいずれもPASSします。

- [ ] Step 5: gofmtを適用してコミットする

```bash
cd /Users/yujiokamoto/devs/golang/feedflow-go-htmx && gofmt -w internal/service/settings.go internal/service/settings_test.go && git add internal/service/settings.go internal/service/settings_test.go && git commit -m "feat: SettingsServiceの取得と更新を追加する"
```

---

## Task 4: MuteServiceのフィルタ管理と適用

Files:
- Create: `/Users/yujiokamoto/devs/golang/feedflow-go-htmx/internal/service/mute.go`
- Test: `/Users/yujiokamoto/devs/golang/feedflow-go-htmx/internal/service/mute_test.go`

設計書セクション3.1のミュートフィルタを担います。フィルタの一覧と追加と削除に加え、記事群からミュート対象を除外するFilterを実装します。Filterはdomain.MuteFilter.Matchesを使い、いずれかのフィルタにタイトルが一致した記事を除外します。追加時はport.IDGenでIDを採番し、空キーワードは追加を拒否します。port.MuteServiceを満たします。

- [ ] Step 1: 失敗するテストを書く

Create `internal/service/mute_test.go`:
```go
package service_test

import (
	"testing"
	"time"

	"github.com/okamyuji/feedflow-go-htmx/internal/domain"
	"github.com/okamyuji/feedflow-go-htmx/internal/port"
	"github.com/okamyuji/feedflow-go-htmx/internal/service"
)

var _ port.MuteService = (*service.MuteService)(nil)

func TestMuteServiceAddFilter(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		keyword string
		scope   domain.MuteScope
		feedID  string
		wantErr bool
	}{
		{name: "全体フィルタを追加する", keyword: "広告", scope: domain.MuteScopeGlobal, feedID: "", wantErr: false},
		{name: "フィード限定フィルタを追加する", keyword: "PR", scope: domain.MuteScopeFeed, feedID: "f1", wantErr: false},
		{name: "空キーワードは拒否する", keyword: "", scope: domain.MuteScopeGlobal, feedID: "", wantErr: true},
		{name: "不正な対象範囲は拒否する", keyword: "x", scope: domain.MuteScope("other"), feedID: "", wantErr: true},
		{name: "フィード限定でFeedID空は拒否する", keyword: "x", scope: domain.MuteScopeFeed, feedID: "", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			repo := newFakeRepo()
			svc := service.NewMuteService(newDeps(repo, newFakeFetcher(), fakeParser{}, time.Now(), &fakeIDGen{}))

			f, err := svc.AddFilter(tt.keyword, tt.scope, tt.feedID)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("AddFilter must return error")
				}
				if len(repo.filters) != 0 {
					t.Fatalf("no filter must be saved on error")
				}
				return
			}
			if err != nil {
				t.Fatalf("AddFilter returned error: %v", err)
			}
			if f.ID == "" {
				t.Fatalf("AddFilter must assign an ID")
			}
			if _, ok := repo.filters[f.ID]; !ok {
				t.Fatalf("AddFilter must persist the filter")
			}
		})
	}
}

func TestMuteServiceListFilters(t *testing.T) {
	t.Parallel()
	repo := newFakeRepo()
	repo.filters["x1"] = domain.MuteFilter{ID: "x1", Keyword: "広告", Scope: domain.MuteScopeGlobal}
	svc := service.NewMuteService(newDeps(repo, newFakeFetcher(), fakeParser{}, time.Now(), &fakeIDGen{}))

	got, err := svc.ListFilters()
	if err != nil {
		t.Fatalf("ListFilters returned error: %v", err)
	}
	if len(got) != 1 || got[0].ID != "x1" {
		t.Fatalf("ListFilters got %+v", got)
	}
}

func TestMuteServiceDeleteFilter(t *testing.T) {
	t.Parallel()
	repo := newFakeRepo()
	repo.filters["x1"] = domain.MuteFilter{ID: "x1", Keyword: "広告", Scope: domain.MuteScopeGlobal}
	svc := service.NewMuteService(newDeps(repo, newFakeFetcher(), fakeParser{}, time.Now(), &fakeIDGen{}))

	if err := svc.DeleteFilter("x1"); err != nil {
		t.Fatalf("DeleteFilter returned error: %v", err)
	}
	if _, ok := repo.filters["x1"]; ok {
		t.Fatalf("DeleteFilter must remove the filter")
	}
}

func TestMuteServiceFilter(t *testing.T) {
	t.Parallel()
	repo := newFakeRepo()
	repo.filters["g1"] = domain.MuteFilter{ID: "g1", Keyword: "広告", Scope: domain.MuteScopeGlobal}
	repo.filters["f1"] = domain.MuteFilter{ID: "f1", Keyword: "PR", Scope: domain.MuteScopeFeed, FeedID: "feedA"}
	svc := service.NewMuteService(newDeps(repo, newFakeFetcher(), fakeParser{}, time.Now(), &fakeIDGen{}))

	items := []domain.Item{
		{ID: "i1", FeedID: "feedA", Title: "本日の広告まとめ"}, // 全体フィルタで除外
		{ID: "i2", FeedID: "feedA", Title: "これはPRです"},    // フィード限定で除外
		{ID: "i3", FeedID: "feedB", Title: "これはPRです"},    // 対象外フィードなので残る
		{ID: "i4", FeedID: "feedB", Title: "通常の技術記事"},   // どのフィルタにも一致せず残る
	}
	got, err := svc.Filter(items)
	if err != nil {
		t.Fatalf("Filter returned error: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("Filter kept %d items, want 2: %+v", len(got), got)
	}
	if got[0].ID != "i3" || got[1].ID != "i4" {
		t.Fatalf("Filter kept wrong items: %+v", got)
	}
}

func TestMuteServiceFilterEmptyFilters(t *testing.T) {
	t.Parallel()
	repo := newFakeRepo()
	svc := service.NewMuteService(newDeps(repo, newFakeFetcher(), fakeParser{}, time.Now(), &fakeIDGen{}))

	items := []domain.Item{{ID: "i1", FeedID: "f", Title: "なんでも"}}
	got, err := svc.Filter(items)
	if err != nil {
		t.Fatalf("Filter returned error: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("Filter with no filters must keep all items, got %d", len(got))
	}
}
```

- [ ] Step 2: テストが失敗することを確認する

Run:
```bash
cd /Users/yujiokamoto/devs/golang/feedflow-go-htmx && go test ./internal/service/ -run TestMuteService -v
```
Expected: コンパイルエラーで失敗します。`undefined: service.MuteService` や `undefined: service.NewMuteService` と表示されます。

- [ ] Step 3: MuteServiceの最小実装を書く

Create `internal/service/mute.go`:
```go
package service

import (
	"errors"
	"fmt"

	"github.com/okamyuji/feedflow-go-htmx/internal/domain"
)

// ErrEmptyKeyword ミュートフィルタのキーワードが空のときに返すエラーです。
var ErrEmptyKeyword = errors.New("mute filter keyword must not be empty")

// ErrInvalidMuteScope ミュートフィルタの対象範囲が不正なときに返すエラーです。
var ErrInvalidMuteScope = errors.New("invalid mute scope")

// ErrMissingFeedID フィード限定フィルタで対象フィードIDが空のときに返すエラーです。
var ErrMissingFeedID = errors.New("feed-scoped filter requires a feed id")

// MuteService ミュートフィルタの管理と適用を担います。port.MuteServiceを満たします。
type MuteService struct {
	deps Deps
}

// NewMuteService 依存束を受け取りMuteServiceを構築します。
func NewMuteService(deps Deps) *MuteService {
	return &MuteService{deps: deps}
}

// ListFilters 全ミュートフィルタを返します。
func (s *MuteService) ListFilters() ([]domain.MuteFilter, error) {
	filters, err := s.deps.Repo.Filters()
	if err != nil {
		return nil, fmt.Errorf("failed to load filters: %w", err)
	}
	return filters, nil
}

// AddFilter ミュートフィルタを検証してから採番し保存します。追加後のフィルタを返します。
func (s *MuteService) AddFilter(keyword string, scope domain.MuteScope, feedID string) (domain.MuteFilter, error) {
	if keyword == "" {
		return domain.MuteFilter{}, ErrEmptyKeyword
	}
	if !scope.Valid() {
		return domain.MuteFilter{}, ErrInvalidMuteScope
	}
	if scope == domain.MuteScopeFeed && feedID == "" {
		return domain.MuteFilter{}, ErrMissingFeedID
	}
	filter := domain.MuteFilter{
		ID:      s.deps.IDs.NewID(),
		Keyword: keyword,
		Scope:   scope,
		FeedID:  feedID,
	}
	if err := s.deps.Repo.SaveFilter(filter); err != nil {
		return domain.MuteFilter{}, fmt.Errorf("failed to save filter: %w", err)
	}
	return filter, nil
}

// DeleteFilter 指定IDのミュートフィルタを削除します。
func (s *MuteService) DeleteFilter(id string) error {
	if err := s.deps.Repo.DeleteFilter(id); err != nil {
		return fmt.Errorf("failed to delete filter: %w", err)
	}
	return nil
}

// Filter 与えた記事群からミュート対象を除いた記事群を返します。
// いずれかのフィルタにタイトルが一致した記事を除外します。
func (s *MuteService) Filter(items []domain.Item) ([]domain.Item, error) {
	filters, err := s.deps.Repo.Filters()
	if err != nil {
		return nil, fmt.Errorf("failed to load filters: %w", err)
	}
	if len(filters) == 0 {
		return items, nil
	}
	out := make([]domain.Item, 0, len(items))
	for _, item := range items {
		if muted(filters, item) {
			continue
		}
		out = append(out, item)
	}
	return out, nil
}

// muted いずれかのフィルタが記事に一致するかどうかを返します。
func muted(filters []domain.MuteFilter, item domain.Item) bool {
	for _, f := range filters {
		if f.Matches(item.Title, item.FeedID) {
			return true
		}
	}
	return false
}
```

- [ ] Step 4: テストが通ることを確認する

Run:
```bash
cd /Users/yujiokamoto/devs/golang/feedflow-go-htmx && go test ./internal/service/ -run TestMuteService -v
```
Expected: `TestMuteServiceAddFilter`と`TestMuteServiceListFilters`と`TestMuteServiceDeleteFilter`と`TestMuteServiceFilter`と`TestMuteServiceFilterEmptyFilters`がいずれもPASSします。

- [ ] Step 5: gofmtを適用してコミットする

```bash
cd /Users/yujiokamoto/devs/golang/feedflow-go-htmx && gofmt -w internal/service/mute.go internal/service/mute_test.go && git add internal/service/mute.go internal/service/mute_test.go && git commit -m "feat: MuteServiceのフィルタ管理と適用を追加する"
```

---

## Task 5: ItemServiceの検索ヘルパと既読操作

Files:
- Create: `/Users/yujiokamoto/devs/golang/feedflow-go-htmx/internal/service/item.go`
- Test: `/Users/yujiokamoto/devs/golang/feedflow-go-htmx/internal/service/item_read_test.go`

設計書セクション3.1の記事操作のうち、まずListItemsとMarkReadとMarkAllReadを実装します。記事の更新は対象FeedIDの記事群を読み込み、対象itemIDの記事を不変更新で差し替えて保存します。feedIDが空のListItemsとMarkAllReadは全フィード横断で処理します。port.ItemServiceの一部をこのタスクで満たし、残りを次のタスクで満たします。

ListItemsはport.ItemService.ListItemsの契約どおりミュート適用済みの記事を返します。ミュート適用の責務をサービス層へ一本化するため、ItemServiceはMuteServiceを依存として受け取り、ListItems内でMuteService.Filterを通します。これにより呼び出し側でミュートを再適用する必要がなくなり、二重適用や未適用を防ぎます。MuteServiceはTask4で実装済みのため、ここで注入できます。

- [ ] Step 1: 失敗するテストを書く

Create `internal/service/item_read_test.go`:
```go
package service_test

import (
	"testing"
	"time"

	"github.com/okamyuji/feedflow-go-htmx/internal/domain"
	"github.com/okamyuji/feedflow-go-htmx/internal/service"
)

// newItemSvc テスト用にフェイク一式を組み、ミュートサービスを注入したItemServiceを返します。
// ListItems がミュート適用済みの記事を返す契約に合わせてMuteServiceを渡します。
func newItemSvc(repo *fakeRepo) *service.ItemService {
	deps := newDeps(repo, newFakeFetcher(), fakeParser{}, time.Now(), &fakeIDGen{})
	return service.NewItemService(deps, service.NewMuteService(deps))
}

func TestItemServiceListItems(t *testing.T) {
	t.Parallel()
	repo := newFakeRepo()
	_ = repo.SaveFeed(domain.Feed{ID: "fA"})
	_ = repo.SaveFeed(domain.Feed{ID: "fB"})
	_ = repo.SaveItems("fA", []domain.Item{{ID: "a1", FeedID: "fA", Title: "A1"}})
	_ = repo.SaveItems("fB", []domain.Item{{ID: "b1", FeedID: "fB", Title: "B1"}})
	svc := newItemSvc(repo)

	t.Run("単一フィードを返す", func(t *testing.T) {
		got, err := svc.ListItems("fA")
		if err != nil {
			t.Fatalf("ListItems returned error: %v", err)
		}
		if len(got) != 1 || got[0].ID != "a1" {
			t.Fatalf("ListItems(fA) got %+v", got)
		}
	})

	t.Run("空指定で全フィードを横断する", func(t *testing.T) {
		got, err := svc.ListItems("")
		if err != nil {
			t.Fatalf("ListItems returned error: %v", err)
		}
		if len(got) != 2 {
			t.Fatalf("ListItems(all) got %d items want 2", len(got))
		}
	})
}

func TestItemServiceMarkRead(t *testing.T) {
	t.Parallel()
	repo := newFakeRepo()
	_ = repo.SaveItems("fA", []domain.Item{
		{ID: "a1", FeedID: "fA", Read: false},
		{ID: "a2", FeedID: "fA", Read: false},
	})
	svc := newItemSvc(repo)

	if err := svc.MarkRead("fA", "a1", true); err != nil {
		t.Fatalf("MarkRead returned error: %v", err)
	}
	items, _ := repo.Items("fA")
	if !items[0].Read {
		t.Fatalf("a1 must be read")
	}
	if items[1].Read {
		t.Fatalf("a2 must stay unread")
	}
}

func TestItemServiceMarkReadNotFound(t *testing.T) {
	t.Parallel()
	repo := newFakeRepo()
	_ = repo.SaveItems("fA", []domain.Item{{ID: "a1", FeedID: "fA"}})
	svc := newItemSvc(repo)

	if err := svc.MarkRead("fA", "missing", true); err == nil {
		t.Fatalf("MarkRead must return error for missing item")
	}
}

func TestItemServiceMarkAllRead(t *testing.T) {
	t.Parallel()
	repo := newFakeRepo()
	_ = repo.SaveFeed(domain.Feed{ID: "fA"})
	_ = repo.SaveFeed(domain.Feed{ID: "fB"})
	_ = repo.SaveItems("fA", []domain.Item{{ID: "a1", FeedID: "fA"}, {ID: "a2", FeedID: "fA"}})
	_ = repo.SaveItems("fB", []domain.Item{{ID: "b1", FeedID: "fB"}})
	svc := newItemSvc(repo)

	t.Run("単一フィードを全既読にする", func(t *testing.T) {
		if err := svc.MarkAllRead("fA"); err != nil {
			t.Fatalf("MarkAllRead returned error: %v", err)
		}
		items, _ := repo.Items("fA")
		for _, it := range items {
			if !it.Read {
				t.Fatalf("item %s must be read", it.ID)
			}
		}
		items, _ = repo.Items("fB")
		if items[0].Read {
			t.Fatalf("fB must stay unread")
		}
	})

	t.Run("空指定で全フィードを全既読にする", func(t *testing.T) {
		if err := svc.MarkAllRead(""); err != nil {
			t.Fatalf("MarkAllRead returned error: %v", err)
		}
		items, _ := repo.Items("fB")
		if !items[0].Read {
			t.Fatalf("fB must be read after all-feeds mark")
		}
	})
}
```

- [ ] Step 2: テストが失敗することを確認する

Run:
```bash
cd /Users/yujiokamoto/devs/golang/feedflow-go-htmx && go test ./internal/service/ -run TestItemService -v
```
Expected: コンパイルエラーで失敗します。`undefined: service.ItemService` や `undefined: service.NewItemService` と表示されます。

- [ ] Step 3: ItemServiceの最小実装と既読操作を書く

Create `internal/service/item.go`:
```go
package service

import (
	"errors"
	"fmt"

	"github.com/okamyuji/feedflow-go-htmx/internal/domain"
)

// ErrItemNotFound 指定IDの記事が見つからないときに返すエラーです。
var ErrItemNotFound = errors.New("item not found")

// ItemService 記事の既読やスターやあとで読むやタグやボードやメモの操作を担います。
// port.ItemService を満たします。
type ItemService struct {
	deps Deps
	mute *MuteService
}

// NewItemService 依存束とミュートサービスを受け取りItemServiceを構築します。
// ListItems がミュート適用済みの記事を返すためMuteServiceを必須の依存とします。
func NewItemService(deps Deps, mute *MuteService) *ItemService {
	return &ItemService{deps: deps, mute: mute}
}

// ListItems 指定フィードの記事をミュート適用済みで返します。feedIDが空なら全フィード横断で返します。
// port.ItemService.ListItems の契約どおりMuteService.Filterを通してミュート対象を除外します。
func (s *ItemService) ListItems(feedID string) ([]domain.Item, error) {
	var raw []domain.Item
	if feedID != "" {
		items, err := s.deps.Repo.Items(feedID)
		if err != nil {
			return nil, fmt.Errorf("failed to load items for feed %s: %w", feedID, err)
		}
		raw = items
	} else {
		feeds, err := s.deps.Repo.Feeds()
		if err != nil {
			return nil, fmt.Errorf("failed to load feeds: %w", err)
		}
		for _, f := range feeds {
			items, err := s.deps.Repo.Items(f.ID)
			if err != nil {
				return nil, fmt.Errorf("failed to load items for feed %s: %w", f.ID, err)
			}
			raw = append(raw, items...)
		}
	}
	filtered, err := s.mute.Filter(raw)
	if err != nil {
		return nil, fmt.Errorf("failed to apply mute filters: %w", err)
	}
	return filtered, nil
}

// MarkRead 指定記事の既読状態を設定します。
func (s *ItemService) MarkRead(feedID, itemID string, read bool) error {
	return s.mutateItem(feedID, itemID, func(item domain.Item) domain.Item {
		item.Read = read
		return item
	})
}

// MarkAllRead 指定フィードの全記事を既読にします。feedIDが空なら全フィードを対象にします。
func (s *ItemService) MarkAllRead(feedID string) error {
	if feedID != "" {
		return s.markAllReadOne(feedID)
	}
	feeds, err := s.deps.Repo.Feeds()
	if err != nil {
		return fmt.Errorf("failed to load feeds: %w", err)
	}
	for _, f := range feeds {
		if err := s.markAllReadOne(f.ID); err != nil {
			return err
		}
	}
	return nil
}

// markAllReadOne 単一フィードの全記事を既読にします。
func (s *ItemService) markAllReadOne(feedID string) error {
	items, err := s.deps.Repo.Items(feedID)
	if err != nil {
		return fmt.Errorf("failed to load items for feed %s: %w", feedID, err)
	}
	updated := make([]domain.Item, len(items))
	for i, item := range items {
		item.Read = true
		updated[i] = item
	}
	if err := s.deps.Repo.SaveItems(feedID, updated); err != nil {
		return fmt.Errorf("failed to save items for feed %s: %w", feedID, err)
	}
	return nil
}

// mutateItem 指定フィード内の指定記事を不変更新で差し替えて保存します。
// 対象が見つからない場合はErrItemNotFoundを返します。
func (s *ItemService) mutateItem(feedID, itemID string, fn func(domain.Item) domain.Item) error {
	items, err := s.deps.Repo.Items(feedID)
	if err != nil {
		return fmt.Errorf("failed to load items for feed %s: %w", feedID, err)
	}
	updated := make([]domain.Item, len(items))
	copy(updated, items)
	found := false
	for i, item := range updated {
		if item.ID == itemID {
			updated[i] = fn(item)
			found = true
			break
		}
	}
	if !found {
		return ErrItemNotFound
	}
	if err := s.deps.Repo.SaveItems(feedID, updated); err != nil {
		return fmt.Errorf("failed to save items for feed %s: %w", feedID, err)
	}
	return nil
}
```

- [ ] Step 4: テストが通ることを確認する

Run:
```bash
cd /Users/yujiokamoto/devs/golang/feedflow-go-htmx && go test ./internal/service/ -run TestItemService -v
```
Expected: `TestItemServiceListItems`と`TestItemServiceMarkRead`と`TestItemServiceMarkReadNotFound`と`TestItemServiceMarkAllRead`がいずれもPASSします。

- [ ] Step 5: gofmtを適用してコミットする

```bash
cd /Users/yujiokamoto/devs/golang/feedflow-go-htmx && gofmt -w internal/service/item.go internal/service/item_read_test.go && git add internal/service/item.go internal/service/item_read_test.go && git commit -m "feat: ItemServiceの一覧と既読操作を追加する"
```

---

## Task 6: ItemServiceのスターとあとで読むとタグとボードとメモとハイライト

Files:
- Edit: `/Users/yujiokamoto/devs/golang/feedflow-go-htmx/internal/service/item.go`
- Test: `/Users/yujiokamoto/devs/golang/feedflow-go-htmx/internal/service/item_action_test.go`

設計書セクション3.1の残りの記事操作を実装します。Star、ReadLater、SetTags、SetBoards、SetNote、AddHighlightをmutateItemヘルパで実装します。タグとボードは指定した内容で置き換え、ハイライトは末尾へ追記します。これでport.ItemServiceを完全に満たします。

- [ ] Step 1: 失敗するテストを書く

Create `internal/service/item_action_test.go`:
```go
package service_test

import (
	"testing"
	"time"

	"github.com/okamyuji/feedflow-go-htmx/internal/domain"
	"github.com/okamyuji/feedflow-go-htmx/internal/port"
	"github.com/okamyuji/feedflow-go-htmx/internal/service"
)

var _ port.ItemService = (*service.ItemService)(nil)

func newItemSvcWith(item domain.Item) (*service.ItemService, *fakeRepo) {
	repo := newFakeRepo()
	_ = repo.SaveItems(item.FeedID, []domain.Item{item})
	deps := newDeps(repo, newFakeFetcher(), fakeParser{}, time.Now(), &fakeIDGen{})
	svc := service.NewItemService(deps, service.NewMuteService(deps))
	return svc, repo
}

func first(repo *fakeRepo, feedID string) domain.Item {
	items, _ := repo.Items(feedID)
	return items[0]
}

func TestItemServiceStar(t *testing.T) {
	t.Parallel()
	svc, repo := newItemSvcWith(domain.Item{ID: "a1", FeedID: "fA"})
	if err := svc.Star("fA", "a1", true); err != nil {
		t.Fatalf("Star returned error: %v", err)
	}
	if !first(repo, "fA").Starred {
		t.Fatalf("item must be starred")
	}
}

func TestItemServiceReadLater(t *testing.T) {
	t.Parallel()
	svc, repo := newItemSvcWith(domain.Item{ID: "a1", FeedID: "fA"})
	if err := svc.ReadLater("fA", "a1", true); err != nil {
		t.Fatalf("ReadLater returned error: %v", err)
	}
	if !first(repo, "fA").ReadLater {
		t.Fatalf("item must be read-later")
	}
}

func TestItemServiceSetTags(t *testing.T) {
	t.Parallel()
	svc, repo := newItemSvcWith(domain.Item{ID: "a1", FeedID: "fA", Tags: []string{"old"}})
	if err := svc.SetTags("fA", "a1", []string{"go", "rss"}); err != nil {
		t.Fatalf("SetTags returned error: %v", err)
	}
	got := first(repo, "fA").Tags
	if len(got) != 2 || got[0] != "go" || got[1] != "rss" {
		t.Fatalf("SetTags got %+v want [go rss]", got)
	}
}

func TestItemServiceSetBoards(t *testing.T) {
	t.Parallel()
	svc, repo := newItemSvcWith(domain.Item{ID: "a1", FeedID: "fA"})
	if err := svc.SetBoards("fA", "a1", []string{"b1"}); err != nil {
		t.Fatalf("SetBoards returned error: %v", err)
	}
	got := first(repo, "fA").BoardIDs
	if len(got) != 1 || got[0] != "b1" {
		t.Fatalf("SetBoards got %+v want [b1]", got)
	}
}

func TestItemServiceSetNote(t *testing.T) {
	t.Parallel()
	svc, repo := newItemSvcWith(domain.Item{ID: "a1", FeedID: "fA"})
	if err := svc.SetNote("fA", "a1", "あとで確認します"); err != nil {
		t.Fatalf("SetNote returned error: %v", err)
	}
	if first(repo, "fA").Note != "あとで確認します" {
		t.Fatalf("SetNote did not persist note")
	}
}

func TestItemServiceAddHighlight(t *testing.T) {
	t.Parallel()
	svc, repo := newItemSvcWith(domain.Item{ID: "a1", FeedID: "fA", Highlights: []string{"一文目"}})
	if err := svc.AddHighlight("fA", "a1", "二文目"); err != nil {
		t.Fatalf("AddHighlight returned error: %v", err)
	}
	got := first(repo, "fA").Highlights
	if len(got) != 2 || got[1] != "二文目" {
		t.Fatalf("AddHighlight got %+v", got)
	}
}

func TestItemServiceActionNotFound(t *testing.T) {
	t.Parallel()
	svc, _ := newItemSvcWith(domain.Item{ID: "a1", FeedID: "fA"})
	if err := svc.Star("fA", "missing", true); err == nil {
		t.Fatalf("Star must return error for missing item")
	}
}
```

- [ ] Step 2: テストが失敗することを確認する

Run:
```bash
cd /Users/yujiokamoto/devs/golang/feedflow-go-htmx && go test ./internal/service/ -run 'TestItemServiceStar|TestItemServiceReadLater|TestItemServiceSetTags|TestItemServiceSetBoards|TestItemServiceSetNote|TestItemServiceAddHighlight|TestItemServiceActionNotFound' -v
```
Expected: コンパイルエラーで失敗します。`svc.Star undefined`などと表示されます。

- [ ] Step 3: 残りの記事操作を実装する

`internal/service/item.go`の末尾、`mutateItem`メソッドの後ろに次のメソッド群を追記します。

Append to `internal/service/item.go`:
```go

// Star 指定記事のスター状態を設定します。
func (s *ItemService) Star(feedID, itemID string, starred bool) error {
	return s.mutateItem(feedID, itemID, func(item domain.Item) domain.Item {
		item.Starred = starred
		return item
	})
}

// ReadLater 指定記事のあとで読む状態を設定します。
func (s *ItemService) ReadLater(feedID, itemID string, readLater bool) error {
	return s.mutateItem(feedID, itemID, func(item domain.Item) domain.Item {
		item.ReadLater = readLater
		return item
	})
}

// SetTags 指定記事のタグを与えた内容で置き換えます。
func (s *ItemService) SetTags(feedID, itemID string, tags []string) error {
	return s.mutateItem(feedID, itemID, func(item domain.Item) domain.Item {
		next := make([]string, len(tags))
		copy(next, tags)
		item.Tags = next
		return item
	})
}

// SetBoards 指定記事の保存先ボードを与えた内容で置き換えます。
func (s *ItemService) SetBoards(feedID, itemID string, boardIDs []string) error {
	return s.mutateItem(feedID, itemID, func(item domain.Item) domain.Item {
		next := make([]string, len(boardIDs))
		copy(next, boardIDs)
		item.BoardIDs = next
		return item
	})
}

// SetNote 指定記事のメモを更新します。
func (s *ItemService) SetNote(feedID, itemID, note string) error {
	return s.mutateItem(feedID, itemID, func(item domain.Item) domain.Item {
		item.Note = note
		return item
	})
}

// AddHighlight 指定記事のハイライト群へ末尾追記します。
func (s *ItemService) AddHighlight(feedID, itemID, highlight string) error {
	return s.mutateItem(feedID, itemID, func(item domain.Item) domain.Item {
		next := make([]string, 0, len(item.Highlights)+1)
		next = append(next, item.Highlights...)
		next = append(next, highlight)
		item.Highlights = next
		return item
	})
}
```

- [ ] Step 4: テストが通ることを確認する

Run:
```bash
cd /Users/yujiokamoto/devs/golang/feedflow-go-htmx && go test ./internal/service/ -run TestItemService -v
```
Expected: ItemServiceの全テストがPASSします。`var _ port.ItemService = (*service.ItemService)(nil)`によりインターフェース充足もコンパイル時に検証されます。

- [ ] Step 5: gofmtを適用してコミットする

```bash
cd /Users/yujiokamoto/devs/golang/feedflow-go-htmx && gofmt -w internal/service/item.go internal/service/item_action_test.go && git add internal/service/item.go internal/service/item_action_test.go && git commit -m "feat: ItemServiceのスターとタグとボードとメモとハイライトを追加する"
```

---

## Task 7: RetentionServiceの保持ポリシー適用

Files:
- Create: `/Users/yujiokamoto/devs/golang/feedflow-go-htmx/internal/service/retention.go`
- Test: `/Users/yujiokamoto/devs/golang/feedflow-go-htmx/internal/service/retention_test.go`

設計書セクション4.1の核心です。フィードごとに記事を公開日時の新しい順へ並べ、domain.Item.ShouldRetainで保持判定します。引数はnowにport.Clock.Now、rankIndexに新しい順での0始まりの順位、maxItemsとretainDaysに設定値を渡します。アクション済みの記事はN件とM日に関わらず常に保持します。削除した記事の総数を返します。port.RetentionServiceを満たします。

公開日時の新しい順は、PublishedAtが等しい場合にFetchedAtの新しい順を二次キーにします。これは安定した順位付けのための補助規則で、Phase1の定義と矛盾しません。

- [ ] Step 1: 失敗するテストを書く

Create `internal/service/retention_test.go`:
```go
package service_test

import (
	"testing"
	"time"

	"github.com/okamyuji/feedflow-go-htmx/internal/domain"
	"github.com/okamyuji/feedflow-go-htmx/internal/port"
	"github.com/okamyuji/feedflow-go-htmx/internal/service"
)

var _ port.RetentionService = (*service.RetentionService)(nil)

func TestRetentionServiceApplyFeed(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 5, 29, 12, 0, 0, 0, time.UTC)
	old := now.AddDate(0, 0, -40)  // M=30を超過します
	recent := now.AddDate(0, 0, -5)

	repo := newFakeRepo()
	settings := domain.DefaultSettings()
	settings.MaxItems = 3
	settings.ReadRetentionDays = 30
	repo.settings = settings
	_ = repo.SaveFeed(domain.Feed{ID: "fA"})

	// 公開日時の新しい順に並ぶよう日時をずらします
	items := []domain.Item{
		{ID: "keep-recent-unread", FeedID: "fA", Read: false, PublishedAt: now.Add(-1 * time.Hour), FetchedAt: recent},
		{ID: "keep-recent-read", FeedID: "fA", Read: true, PublishedAt: now.Add(-2 * time.Hour), FetchedAt: recent},
		{ID: "drop-old-read", FeedID: "fA", Read: true, PublishedAt: now.Add(-3 * time.Hour), FetchedAt: old},
		{ID: "drop-over-limit", FeedID: "fA", Read: false, PublishedAt: now.Add(-4 * time.Hour), FetchedAt: recent},
		{ID: "keep-actioned", FeedID: "fA", Read: true, Starred: true, PublishedAt: now.Add(-5 * time.Hour), FetchedAt: old},
	}
	_ = repo.SaveItems("fA", items)

	svc := service.NewRetentionService(newDeps(repo, newFakeFetcher(), fakeParser{}, now, &fakeIDGen{}))

	removed, err := svc.ApplyFeed("fA")
	if err != nil {
		t.Fatalf("ApplyFeed returned error: %v", err)
	}
	// drop-old-readは既読かつM日超過で削除、drop-over-limitは順位3でN=3上限超過で削除
	if removed != 2 {
		t.Fatalf("removed got %d want 2", removed)
	}
	got, _ := repo.Items("fA")
	kept := map[string]bool{}
	for _, it := range got {
		kept[it.ID] = true
	}
	for _, id := range []string{"keep-recent-unread", "keep-recent-read", "keep-actioned"} {
		if !kept[id] {
			t.Fatalf("item %s must be kept", id)
		}
	}
	for _, id := range []string{"drop-old-read", "drop-over-limit"} {
		if kept[id] {
			t.Fatalf("item %s must be removed", id)
		}
	}
}

func TestRetentionServiceApplyAll(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 5, 29, 12, 0, 0, 0, time.UTC)
	old := now.AddDate(0, 0, -40)

	repo := newFakeRepo()
	settings := domain.DefaultSettings()
	settings.MaxItems = 100
	settings.ReadRetentionDays = 30
	repo.settings = settings
	_ = repo.SaveFeed(domain.Feed{ID: "fA"})
	_ = repo.SaveFeed(domain.Feed{ID: "fB"})
	_ = repo.SaveItems("fA", []domain.Item{
		{ID: "a-old", FeedID: "fA", Read: true, PublishedAt: now, FetchedAt: old},
	})
	_ = repo.SaveItems("fB", []domain.Item{
		{ID: "b-old", FeedID: "fB", Read: true, PublishedAt: now, FetchedAt: old},
		{ID: "b-keep", FeedID: "fB", Read: false, PublishedAt: now, FetchedAt: now},
	})

	svc := service.NewRetentionService(newDeps(repo, newFakeFetcher(), fakeParser{}, now, &fakeIDGen{}))

	removed, err := svc.Apply()
	if err != nil {
		t.Fatalf("Apply returned error: %v", err)
	}
	if removed != 2 {
		t.Fatalf("Apply removed got %d want 2", removed)
	}
	a, _ := repo.Items("fA")
	if len(a) != 0 {
		t.Fatalf("fA must be empty, got %d", len(a))
	}
	b, _ := repo.Items("fB")
	if len(b) != 1 || b[0].ID != "b-keep" {
		t.Fatalf("fB must keep only b-keep, got %+v", b)
	}
}

func TestRetentionServiceApplyFeedNoDelete(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 5, 29, 12, 0, 0, 0, time.UTC)
	repo := newFakeRepo()
	settings := domain.DefaultSettings()
	repo.settings = settings
	_ = repo.SaveFeed(domain.Feed{ID: "fA"})
	_ = repo.SaveItems("fA", []domain.Item{
		{ID: "a1", FeedID: "fA", Read: false, PublishedAt: now, FetchedAt: now},
	})
	svc := service.NewRetentionService(newDeps(repo, newFakeFetcher(), fakeParser{}, now, &fakeIDGen{}))

	removed, err := svc.ApplyFeed("fA")
	if err != nil {
		t.Fatalf("ApplyFeed returned error: %v", err)
	}
	if removed != 0 {
		t.Fatalf("removed got %d want 0", removed)
	}
}
```

- [ ] Step 2: テストが失敗することを確認する

Run:
```bash
cd /Users/yujiokamoto/devs/golang/feedflow-go-htmx && go test ./internal/service/ -run TestRetentionService -v
```
Expected: コンパイルエラーで失敗します。`undefined: service.RetentionService` や `undefined: service.NewRetentionService` と表示されます。

- [ ] Step 3: RetentionServiceの最小実装を書く

Create `internal/service/retention.go`:
```go
package service

import (
	"fmt"
	"sort"

	"github.com/okamyuji/feedflow-go-htmx/internal/domain"
)

// RetentionService 保持ポリシーの適用を担います。port.RetentionServiceを満たします。
type RetentionService struct {
	deps Deps
}

// NewRetentionService 依存束を受け取りRetentionServiceを構築します。
func NewRetentionService(deps Deps) *RetentionService {
	return &RetentionService{deps: deps}
}

// Apply 全フィードに保持ポリシーを適用し、削除した記事の総数を返します。
func (s *RetentionService) Apply() (int, error) {
	feeds, err := s.deps.Repo.Feeds()
	if err != nil {
		return 0, fmt.Errorf("failed to load feeds: %w", err)
	}
	total := 0
	for _, f := range feeds {
		removed, err := s.ApplyFeed(f.ID)
		if err != nil {
			return total, err
		}
		total += removed
	}
	return total, nil
}

// ApplyFeed 指定フィードに保持ポリシーを適用し、削除した記事数を返します。
// 記事を公開日時の新しい順へ並べ、順位とともにdomain.Item.ShouldRetainで保持判定します。
func (s *RetentionService) ApplyFeed(feedID string) (int, error) {
	settings, err := s.deps.Repo.Settings()
	if err != nil {
		return 0, fmt.Errorf("failed to load settings: %w", err)
	}
	items, err := s.deps.Repo.Items(feedID)
	if err != nil {
		return 0, fmt.Errorf("failed to load items for feed %s: %w", feedID, err)
	}

	ordered := make([]domain.Item, len(items))
	copy(ordered, items)
	sortByRecency(ordered)

	now := s.deps.Clock.Now()
	kept := make([]domain.Item, 0, len(ordered))
	removed := 0
	for rank, item := range ordered {
		if item.ShouldRetain(now, rank, settings.MaxItems, settings.ReadRetentionDays) {
			kept = append(kept, item)
			continue
		}
		removed++
	}
	if removed == 0 {
		return 0, nil
	}
	if err := s.deps.Repo.SaveItems(feedID, kept); err != nil {
		return 0, fmt.Errorf("failed to save items for feed %s: %w", feedID, err)
	}
	return removed, nil
}

// sortByRecency 記事を公開日時の新しい順へ並べ替えます。
// 公開日時が等しいときは取得日時の新しい順を二次キーにします。
func sortByRecency(items []domain.Item) {
	sort.SliceStable(items, func(i, j int) bool {
		if items[i].PublishedAt.Equal(items[j].PublishedAt) {
			return items[i].FetchedAt.After(items[j].FetchedAt)
		}
		return items[i].PublishedAt.After(items[j].PublishedAt)
	})
}
```

- [ ] Step 4: テストが通ることを確認する

Run:
```bash
cd /Users/yujiokamoto/devs/golang/feedflow-go-htmx && go test ./internal/service/ -run TestRetentionService -v
```
Expected: `TestRetentionServiceApplyFeed`と`TestRetentionServiceApplyAll`と`TestRetentionServiceApplyFeedNoDelete`がいずれもPASSします。

- [ ] Step 5: gofmtを適用してコミットする

```bash
cd /Users/yujiokamoto/devs/golang/feedflow-go-htmx && gofmt -w internal/service/retention.go internal/service/retention_test.go && git add internal/service/retention.go internal/service/retention_test.go && git commit -m "feat: RetentionServiceの保持ポリシー適用を追加する"
```

---

## Task 8: SubscriptionServiceの購読追加と削除と一覧

Files:
- Create: `/Users/yujiokamoto/devs/golang/feedflow-go-htmx/internal/service/subscription.go`
- Test: `/Users/yujiokamoto/devs/golang/feedflow-go-htmx/internal/service/subscription_test.go`

設計書セクション3.1の購読管理の中核です。SubscribeはフィードURLをport.Fetcherで取得し、port.FeedParserでパースし、port.IDGenでフィードIDを採番し、パース結果の記事へFeedIDとIDを付与してまとめて保存します。Unsubscribeはフィードと記事を削除します。ListFeedsはリポジトリのフィード一覧を返します。port.SubscriptionServiceを完全に満たすため、SubscribeFromSiteとReorderとSetFeedCategoriesもこのタスクで実装します。これによりこのタスク単体でテストが緑になり、各タスクが失敗テスト先行と最小実装とテスト通過確認とコミットで完結します。SubscribeFromSiteのフィード自動検出に対するテーブル駆動テストはTask9で追加します。

- [ ] Step 1: 失敗するテストを書く

Create `internal/service/subscription_test.go`:
```go
package service_test

import (
	"context"
	"testing"
	"time"

	"github.com/okamyuji/feedflow-go-htmx/internal/domain"
	"github.com/okamyuji/feedflow-go-htmx/internal/port"
	"github.com/okamyuji/feedflow-go-htmx/internal/service"
)

var _ port.SubscriptionService = (*service.SubscriptionService)(nil)

func TestSubscriptionServiceSubscribe(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 5, 29, 12, 0, 0, 0, time.UTC)
	repo := newFakeRepo()
	fetch := newFakeFetcher()
	fetch.results["https://example.com/feed.xml"] = port.FetchResult{
		StatusCode:   200,
		Body:         []byte("<rss></rss>"),
		ETag:         "etag-1",
		LastModified: "Wed, 28 May 2026 00:00:00 GMT",
	}
	parse := fakeParser{parsed: port.ParsedFeed{
		Format:  port.FormatRSS2,
		Title:   "Example Feed",
		SiteURL: "https://example.com",
		Items: []port.ParsedItem{
			{GUID: "g1", Title: "記事1", Link: "https://example.com/1", PublishedAt: now.Add(-time.Hour)},
			{GUID: "g2", Title: "記事2", Link: "https://example.com/2", PublishedAt: now.Add(-2 * time.Hour)},
		},
	}}
	svc := service.NewSubscriptionService(newDeps(repo, fetch, parse, now, &fakeIDGen{}))

	feed, err := svc.Subscribe(context.Background(), "https://example.com/feed.xml", []string{"cat1"})
	if err != nil {
		t.Fatalf("Subscribe returned error: %v", err)
	}
	if feed.ID == "" {
		t.Fatalf("Subscribe must assign a feed ID")
	}
	if feed.FeedURL != "https://example.com/feed.xml" {
		t.Fatalf("FeedURL got %q", feed.FeedURL)
	}
	if feed.Title != "Example Feed" || feed.SiteURL != "https://example.com" {
		t.Fatalf("feed metadata not applied: %+v", feed)
	}
	if feed.ETag != "etag-1" || feed.LastModified == "" {
		t.Fatalf("conditional headers not stored: %+v", feed)
	}
	if len(feed.CategoryIDs) != 1 || feed.CategoryIDs[0] != "cat1" {
		t.Fatalf("categories not applied: %+v", feed.CategoryIDs)
	}
	if !feed.LastFetchedAt.Equal(now) {
		t.Fatalf("LastFetchedAt got %v want %v", feed.LastFetchedAt, now)
	}
	saved, _ := repo.Feed(feed.ID)
	if saved.ID != feed.ID {
		t.Fatalf("feed must be persisted")
	}
	items, _ := repo.Items(feed.ID)
	if len(items) != 2 {
		t.Fatalf("items got %d want 2", len(items))
	}
	for _, it := range items {
		if it.ID == "" {
			t.Fatalf("each item must have an ID")
		}
		if it.FeedID != feed.ID {
			t.Fatalf("item FeedID got %q want %q", it.FeedID, feed.ID)
		}
		if it.FetchedAt.IsZero() {
			t.Fatalf("item FetchedAt must be set")
		}
	}
}

func TestSubscriptionServiceSubscribeDuplicate(t *testing.T) {
	t.Parallel()
	now := time.Now()
	repo := newFakeRepo()
	_ = repo.SaveFeed(domain.Feed{ID: "f0", FeedURL: "https://example.com/feed.xml"})
	fetch := newFakeFetcher()
	svc := service.NewSubscriptionService(newDeps(repo, fetch, fakeParser{}, now, &fakeIDGen{}))

	if _, err := svc.Subscribe(context.Background(), "https://example.com/feed.xml", nil); err == nil {
		t.Fatalf("Subscribe must reject a duplicate feed URL")
	}
}

func TestSubscriptionServiceSubscribeFetchError(t *testing.T) {
	t.Parallel()
	now := time.Now()
	repo := newFakeRepo()
	fetch := newFakeFetcher() // 何も登録しないのでerrNotFoundを返します
	svc := service.NewSubscriptionService(newDeps(repo, fetch, fakeParser{}, now, &fakeIDGen{}))

	if _, err := svc.Subscribe(context.Background(), "https://missing.example/feed.xml", nil); err == nil {
		t.Fatalf("Subscribe must propagate fetch error")
	}
	if len(repo.feeds) != 0 {
		t.Fatalf("no feed must be saved when fetch fails")
	}
}

func TestSubscriptionServiceUnsubscribe(t *testing.T) {
	t.Parallel()
	now := time.Now()
	repo := newFakeRepo()
	_ = repo.SaveFeed(domain.Feed{ID: "f1"})
	_ = repo.SaveItems("f1", []domain.Item{{ID: "i1", FeedID: "f1"}})
	svc := service.NewSubscriptionService(newDeps(repo, newFakeFetcher(), fakeParser{}, now, &fakeIDGen{}))

	if err := svc.Unsubscribe("f1"); err != nil {
		t.Fatalf("Unsubscribe returned error: %v", err)
	}
	if _, ok := repo.feeds["f1"]; ok {
		t.Fatalf("feed must be removed")
	}
	if _, ok := repo.items["f1"]; ok {
		t.Fatalf("items must be removed")
	}
}

func TestSubscriptionServiceListFeeds(t *testing.T) {
	t.Parallel()
	now := time.Now()
	repo := newFakeRepo()
	_ = repo.SaveFeed(domain.Feed{ID: "f1", Title: "A"})
	_ = repo.SaveFeed(domain.Feed{ID: "f2", Title: "B"})
	svc := service.NewSubscriptionService(newDeps(repo, newFakeFetcher(), fakeParser{}, now, &fakeIDGen{}))

	got, err := svc.ListFeeds()
	if err != nil {
		t.Fatalf("ListFeeds returned error: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("ListFeeds got %d want 2", len(got))
	}
}
```

- [ ] Step 2: テストが失敗することを確認する

Run:
```bash
cd /Users/yujiokamoto/devs/golang/feedflow-go-htmx && go test ./internal/service/ -run TestSubscriptionService -v
```
Expected: コンパイルエラーで失敗します。`undefined: service.SubscriptionService` や `undefined: service.NewSubscriptionService` と表示されます。

- [ ] Step 3: SubscriptionServiceの最小実装を書く

このタスクでport.SubscriptionServiceの全6メソッドを実装します。購読追加と削除と一覧に加え、SubscribeFromSiteとReorderとSetFeedCategoriesの確定実装をここで書きます。これによりこのタスク単体で`var _ port.SubscriptionService`の充足検証がコンパイルを通り、Task8のテストが緑になります。SubscribeFromSiteはサイトHTMLをport.Fetcherで取得し、golang.org/x/net/htmlでlink要素のtype属性がapplication/rss+xmlまたはapplication/atom+xmlのhref相対URLを基準URLで絶対化し、見つけたフィードURLでSubscribeを呼びます。

Create `internal/service/subscription.go`:
```go
package service

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net/url"
	"strings"

	"golang.org/x/net/html"

	"github.com/okamyuji/feedflow-go-htmx/internal/domain"
	"github.com/okamyuji/feedflow-go-htmx/internal/port"
)

// ErrDuplicateFeed 同じフィードURLがすでに購読済みのときに返すエラーです。
var ErrDuplicateFeed = errors.New("feed url already subscribed")

// ErrFeedNotDiscovered サイトURLからフィードを検出できなかったときに返すエラーです。
var ErrFeedNotDiscovered = errors.New("no feed link discovered on site")

// errNotFoundCategory Reorderで未知のカテゴリIDを受け取ったときの基底エラーです。
var errNotFoundCategory = errors.New("category not found")

// SubscriptionService 購読の追加と削除と一覧と整理を担います。port.SubscriptionServiceを満たします。
type SubscriptionService struct {
	deps Deps
}

// NewSubscriptionService 依存束を受け取りSubscriptionServiceを構築します。
func NewSubscriptionService(deps Deps) *SubscriptionService {
	return &SubscriptionService{deps: deps}
}

// ListFeeds 購読中の全フィードを返します。
func (s *SubscriptionService) ListFeeds() ([]domain.Feed, error) {
	feeds, err := s.deps.Repo.Feeds()
	if err != nil {
		return nil, fmt.Errorf("failed to load feeds: %w", err)
	}
	return feeds, nil
}

// Subscribe フィードURLを取得しパースして購読に追加します。追加後のフィードを返します。
// 既存の購読URLと重複する場合は追加せずエラーを返します。
func (s *SubscriptionService) Subscribe(ctx context.Context, feedURL string, categoryIDs []string) (domain.Feed, error) {
	duplicate, err := s.feedURLExists(feedURL)
	if err != nil {
		return domain.Feed{}, err
	}
	if duplicate {
		return domain.Feed{}, ErrDuplicateFeed
	}

	res, err := s.deps.Fetch.Fetch(ctx, port.FetchRequest{URL: feedURL})
	if err != nil {
		return domain.Feed{}, fmt.Errorf("failed to fetch feed %s: %w", feedURL, err)
	}
	parsed, err := s.deps.Parse.Parse(res.Body)
	if err != nil {
		return domain.Feed{}, fmt.Errorf("failed to parse feed %s: %w", feedURL, err)
	}

	return s.createFeed(feedURL, categoryIDs, res, parsed)
}

// createFeed パース結果からフィードと記事を構築して保存し、保存したフィードを返します。
func (s *SubscriptionService) createFeed(feedURL string, categoryIDs []string, res port.FetchResult, parsed port.ParsedFeed) (domain.Feed, error) {
	now := s.deps.Clock.Now()
	cats := make([]string, len(categoryIDs))
	copy(cats, categoryIDs)

	feed := domain.Feed{
		ID:            s.deps.IDs.NewID(),
		FeedURL:       feedURL,
		SiteURL:       parsed.SiteURL,
		Title:         parsed.Title,
		CategoryIDs:   cats,
		PollInterval:  domain.PollDefault,
		ETag:          res.ETag,
		LastModified:  res.LastModified,
		LastFetchedAt: now,
	}
	if err := s.deps.Repo.SaveFeed(feed); err != nil {
		return domain.Feed{}, fmt.Errorf("failed to save feed: %w", err)
	}

	items := make([]domain.Item, 0, len(parsed.Items))
	for _, p := range parsed.Items {
		items = append(items, domain.Item{
			ID:          s.deps.IDs.NewID(),
			FeedID:      feed.ID,
			GUID:        p.GUID,
			Title:       p.Title,
			Link:        p.Link,
			Content:     p.Content,
			Summary:     p.Summary,
			Author:      p.Author,
			PublishedAt: p.PublishedAt,
			FetchedAt:   now,
		})
	}
	if err := s.deps.Repo.SaveItems(feed.ID, items); err != nil {
		return domain.Feed{}, fmt.Errorf("failed to save items for feed %s: %w", feed.ID, err)
	}
	return feed, nil
}

// feedURLExists 指定URLがすでに購読済みかどうかを返します。
func (s *SubscriptionService) feedURLExists(feedURL string) (bool, error) {
	feeds, err := s.deps.Repo.Feeds()
	if err != nil {
		return false, fmt.Errorf("failed to load feeds: %w", err)
	}
	for _, f := range feeds {
		if f.FeedURL == feedURL {
			return true, nil
		}
	}
	return false, nil
}

// Unsubscribe 指定フィードの購読を解除し、属する記事も削除します。
func (s *SubscriptionService) Unsubscribe(feedID string) error {
	if err := s.deps.Repo.DeleteFeed(feedID); err != nil {
		return fmt.Errorf("failed to delete feed %s: %w", feedID, err)
	}
	return nil
}

// SubscribeFromSite サイトURLを取得しHTMLからフィードリンクを検出して購読に追加します。
// 検出した最初のRSSまたはAtomのフィードURLでSubscribeを呼びます。
func (s *SubscriptionService) SubscribeFromSite(ctx context.Context, siteURL string, categoryIDs []string) (domain.Feed, error) {
	res, err := s.deps.Fetch.Fetch(ctx, port.FetchRequest{URL: siteURL})
	if err != nil {
		return domain.Feed{}, fmt.Errorf("failed to fetch site %s: %w", siteURL, err)
	}
	feedURL, err := discoverFeedURL(siteURL, res.Body)
	if err != nil {
		return domain.Feed{}, err
	}
	return s.Subscribe(ctx, feedURL, categoryIDs)
}

// Reorder カテゴリの並び順を指定したID順に更新します。
// 指定IDの先頭から0, 1, 2とOrderを振り直します。未知のIDがあればエラーを返します。
func (s *SubscriptionService) Reorder(categoryIDs []string) error {
	categories, err := s.deps.Repo.Categories()
	if err != nil {
		return fmt.Errorf("failed to load categories: %w", err)
	}
	byID := make(map[string]domain.Category, len(categories))
	for _, c := range categories {
		byID[c.ID] = c
	}
	for order, id := range categoryIDs {
		c, ok := byID[id]
		if !ok {
			return fmt.Errorf("unknown category id %s: %w", id, errNotFoundCategory)
		}
		c.Order = order
		if err := s.deps.Repo.SaveCategory(c); err != nil {
			return fmt.Errorf("failed to save category %s: %w", id, err)
		}
	}
	return nil
}

// SetFeedCategories 指定フィードの所属カテゴリを与えた内容で置き換えます。
func (s *SubscriptionService) SetFeedCategories(feedID string, categoryIDs []string) error {
	feed, err := s.deps.Repo.Feed(feedID)
	if err != nil {
		return fmt.Errorf("failed to load feed %s: %w", feedID, err)
	}
	next := make([]string, len(categoryIDs))
	copy(next, categoryIDs)
	feed.CategoryIDs = next
	if err := s.deps.Repo.SaveFeed(feed); err != nil {
		return fmt.Errorf("failed to save feed %s: %w", feedID, err)
	}
	return nil
}

// discoverFeedURL HTMLのバイト列から最初のフィードリンクの絶対URLを返します。
// link 要素のrelがalternateでtypeがRSSまたはAtomのhrefを対象にします。
// href が相対URLの場合は基準URLで絶対化します。
func discoverFeedURL(baseURL string, htmlBytes []byte) (string, error) {
	base, err := url.Parse(baseURL)
	if err != nil {
		return "", fmt.Errorf("failed to parse base url %s: %w", baseURL, err)
	}
	doc, err := html.Parse(bytes.NewReader(htmlBytes))
	if err != nil {
		return "", fmt.Errorf("failed to parse site html: %w", err)
	}
	href := findFeedHref(doc)
	if href == "" {
		return "", ErrFeedNotDiscovered
	}
	ref, err := url.Parse(href)
	if err != nil {
		return "", fmt.Errorf("failed to parse feed href %s: %w", href, err)
	}
	return base.ResolveReference(ref).String(), nil
}

// findFeedHref ノードを深さ優先で走査し、最初のフィードリンクのhrefを返します。
func findFeedHref(n *html.Node) string {
	if n.Type == html.ElementNode && n.Data == "link" {
		if href, ok := feedLinkHref(n); ok {
			return href
		}
	}
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		if href := findFeedHref(c); href != "" {
			return href
		}
	}
	return ""
}

// feedLinkHref link要素がフィードリンクならhrefを返します。
// type 属性がapplication/rss+xmlまたはapplication/atom+xmlのときに該当とみなします。
func feedLinkHref(n *html.Node) (string, bool) {
	var typ, href string
	for _, a := range n.Attr {
		switch strings.ToLower(a.Key) {
		case "type":
			typ = strings.ToLower(strings.TrimSpace(a.Val))
		case "href":
			href = strings.TrimSpace(a.Val)
		}
	}
	if href == "" {
		return "", false
	}
	if typ == "application/rss+xml" || typ == "application/atom+xml" {
		return href, true
	}
	return "", false
}
```

補足: このタスクでport.SubscriptionServiceの全6メソッドを実装したため、`var _ port.SubscriptionService = (*service.SubscriptionService)(nil)`の充足検証がコンパイルを通り、Task8のテストが緑になります。golang.org/x/netはPhase3で取得済みの前提です。未取得の環境では`go mod tidy`が取得します。

- [ ] Step 4: Task 8のテストが通ることを確認する

Run:
```bash
cd /Users/yujiokamoto/devs/golang/feedflow-go-htmx && go test ./internal/service/ -run 'TestSubscriptionServiceSubscribe|TestSubscriptionServiceSubscribeDuplicate|TestSubscriptionServiceSubscribeFetchError|TestSubscriptionServiceUnsubscribe|TestSubscriptionServiceListFeeds' -v
```
Expected: `TestSubscriptionServiceSubscribe`と`TestSubscriptionServiceSubscribeDuplicate`と`TestSubscriptionServiceSubscribeFetchError`と`TestSubscriptionServiceUnsubscribe`と`TestSubscriptionServiceListFeeds`がいずれもPASSします。`var _ port.SubscriptionService = (*service.SubscriptionService)(nil)`の充足検証もコンパイル時に通ります。

- [ ] Step 5: golang.org/x/netがgo.modに含まれることを確認する

Run:
```bash
cd /Users/yujiokamoto/devs/golang/feedflow-go-htmx && go mod tidy && go build ./...
```
Expected: エラーなく完了します。golang.org/x/netはPhase3で追加済みのため新規取得は発生しません。未取得の環境では`go mod tidy`が取得します。

- [ ] Step 6: gofmtを適用してコミットする

```bash
cd /Users/yujiokamoto/devs/golang/feedflow-go-htmx && gofmt -w internal/service/subscription.go internal/service/subscription_test.go && git add internal/service/subscription.go internal/service/subscription_test.go go.mod go.sum && git commit -m "feat: SubscriptionServiceの購読管理と自動検出と整理を追加する"
```

---

## Task 9: SubscribeFromSiteと並べ替えとカテゴリ設定のテスト

Files:
- Test: `/Users/yujiokamoto/devs/golang/feedflow-go-htmx/internal/service/subscription_site_test.go`

設計書セクション3.1のサイトURLからのフィード自動検出と、購読の並べ替えと整理に対するテーブル駆動テストを追加します。SubscribeFromSiteとReorderとSetFeedCategoriesの実装はTask8で完了済みのため、このタスクではテストのみを追加して各分岐とエラー伝播を検証します。SubscribeFromSiteはサイトHTMLをport.Fetcherで取得し、golang.org/x/net/htmlでlink要素のtype属性がapplication/rss+xmlまたはapplication/atom+xmlのhref相対URLを基準URLで絶対化し、見つけたフィードURLでSubscribeを呼びます。Reorderはカテゴリの並び順を指定ID順に更新します。SetFeedCategoriesは指定フィードの所属カテゴリを更新します。

- [ ] Step 1: 追加の検証テストを書く

SubscribeFromSiteとReorderとSetFeedCategoriesの実装はTask8で完成済みです。ここではその振る舞いを検証するテストを追加します。

Create `internal/service/subscription_site_test.go`:
```go
package service_test

import (
	"context"
	"testing"
	"time"

	"github.com/okamyuji/feedflow-go-htmx/internal/domain"
	"github.com/okamyuji/feedflow-go-htmx/internal/port"
	"github.com/okamyuji/feedflow-go-htmx/internal/service"
)

func TestSubscriptionServiceSubscribeFromSite(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 5, 29, 12, 0, 0, 0, time.UTC)
	repo := newFakeRepo()
	fetch := newFakeFetcher()
	siteHTML := `<!doctype html><html><head>` +
		`<link rel="alternate" type="application/rss+xml" href="/feed.xml">` +
		`</head><body>hello</body></html>`
	fetch.results["https://example.com/"] = port.FetchResult{StatusCode: 200, Body: []byte(siteHTML), ContentType: "text/html"}
	fetch.results["https://example.com/feed.xml"] = port.FetchResult{StatusCode: 200, Body: []byte("<rss></rss>")}
	parse := fakeParser{parsed: port.ParsedFeed{Format: port.FormatRSS2, Title: "Example", SiteURL: "https://example.com"}}
	svc := service.NewSubscriptionService(newDeps(repo, fetch, parse, now, &fakeIDGen{}))

	feed, err := svc.SubscribeFromSite(context.Background(), "https://example.com/", []string{"c1"})
	if err != nil {
		t.Fatalf("SubscribeFromSite returned error: %v", err)
	}
	if feed.FeedURL != "https://example.com/feed.xml" {
		t.Fatalf("discovered feed url got %q want https://example.com/feed.xml", feed.FeedURL)
	}
	if feed.Title != "Example" {
		t.Fatalf("feed title got %q", feed.Title)
	}
}

func TestSubscriptionServiceSubscribeFromSiteAtom(t *testing.T) {
	t.Parallel()
	now := time.Now()
	repo := newFakeRepo()
	fetch := newFakeFetcher()
	siteHTML := `<html><head>` +
		`<link rel="alternate" type="application/atom+xml" href="https://blog.example.org/atom">` +
		`</head></html>`
	fetch.results["https://blog.example.org"] = port.FetchResult{StatusCode: 200, Body: []byte(siteHTML)}
	fetch.results["https://blog.example.org/atom"] = port.FetchResult{StatusCode: 200, Body: []byte("<feed></feed>")}
	parse := fakeParser{parsed: port.ParsedFeed{Format: port.FormatAtom, Title: "Blog"}}
	svc := service.NewSubscriptionService(newDeps(repo, fetch, parse, now, &fakeIDGen{}))

	feed, err := svc.SubscribeFromSite(context.Background(), "https://blog.example.org", nil)
	if err != nil {
		t.Fatalf("SubscribeFromSite returned error: %v", err)
	}
	if feed.FeedURL != "https://blog.example.org/atom" {
		t.Fatalf("discovered feed url got %q", feed.FeedURL)
	}
}

func TestSubscriptionServiceSubscribeFromSiteNotFound(t *testing.T) {
	t.Parallel()
	now := time.Now()
	repo := newFakeRepo()
	fetch := newFakeFetcher()
	fetch.results["https://nofeed.example/"] = port.FetchResult{
		StatusCode: 200,
		Body:       []byte(`<html><head><title>no feed here</title></head></html>`),
	}
	svc := service.NewSubscriptionService(newDeps(repo, fetch, fakeParser{}, now, &fakeIDGen{}))

	if _, err := svc.SubscribeFromSite(context.Background(), "https://nofeed.example/", nil); err == nil {
		t.Fatalf("SubscribeFromSite must return error when no feed link is found")
	}
}

func TestSubscriptionServiceReorder(t *testing.T) {
	t.Parallel()
	now := time.Now()
	repo := newFakeRepo()
	_ = repo.SaveCategory(domain.Category{ID: "c1", Name: "A", Order: 0})
	_ = repo.SaveCategory(domain.Category{ID: "c2", Name: "B", Order: 0})
	_ = repo.SaveCategory(domain.Category{ID: "c3", Name: "C", Order: 0})
	svc := service.NewSubscriptionService(newDeps(repo, newFakeFetcher(), fakeParser{}, now, &fakeIDGen{}))

	if err := svc.Reorder([]string{"c3", "c1", "c2"}); err != nil {
		t.Fatalf("Reorder returned error: %v", err)
	}
	if repo.categories["c3"].Order != 0 {
		t.Fatalf("c3 order got %d want 0", repo.categories["c3"].Order)
	}
	if repo.categories["c1"].Order != 1 {
		t.Fatalf("c1 order got %d want 1", repo.categories["c1"].Order)
	}
	if repo.categories["c2"].Order != 2 {
		t.Fatalf("c2 order got %d want 2", repo.categories["c2"].Order)
	}
}

func TestSubscriptionServiceReorderUnknownCategory(t *testing.T) {
	t.Parallel()
	now := time.Now()
	repo := newFakeRepo()
	_ = repo.SaveCategory(domain.Category{ID: "c1", Name: "A"})
	svc := service.NewSubscriptionService(newDeps(repo, newFakeFetcher(), fakeParser{}, now, &fakeIDGen{}))

	if err := svc.Reorder([]string{"c1", "missing"}); err == nil {
		t.Fatalf("Reorder must return error for unknown category")
	}
}

func TestSubscriptionServiceSetFeedCategories(t *testing.T) {
	t.Parallel()
	now := time.Now()
	repo := newFakeRepo()
	_ = repo.SaveFeed(domain.Feed{ID: "f1", CategoryIDs: []string{"old"}})
	svc := service.NewSubscriptionService(newDeps(repo, newFakeFetcher(), fakeParser{}, now, &fakeIDGen{}))

	if err := svc.SetFeedCategories("f1", []string{"c1", "c2"}); err != nil {
		t.Fatalf("SetFeedCategories returned error: %v", err)
	}
	got := repo.feeds["f1"].CategoryIDs
	if len(got) != 2 || got[0] != "c1" || got[1] != "c2" {
		t.Fatalf("categories got %+v want [c1 c2]", got)
	}
}

func TestSubscriptionServiceSetFeedCategoriesNotFound(t *testing.T) {
	t.Parallel()
	now := time.Now()
	repo := newFakeRepo()
	svc := service.NewSubscriptionService(newDeps(repo, newFakeFetcher(), fakeParser{}, now, &fakeIDGen{}))

	if err := svc.SetFeedCategories("missing", []string{"c1"}); err == nil {
		t.Fatalf("SetFeedCategories must return error for missing feed")
	}
}
```

- [ ] Step 2: テストが通ることを確認する

Run:
```bash
cd /Users/yujiokamoto/devs/golang/feedflow-go-htmx && go test ./internal/service/ -run TestSubscriptionService -v
```
Expected: SubscriptionServiceの全テストがPASSします。実装はTask8で完成済みのため、追加したSubscribeFromSiteとReorderとSetFeedCategoriesのテストもそのまま緑になります。`var _ port.SubscriptionService = (*service.SubscriptionService)(nil)`によりインターフェース充足もコンパイル時に検証されます。

- [ ] Step 3: gofmtを適用してコミットする

```bash
cd /Users/yujiokamoto/devs/golang/feedflow-go-htmx && gofmt -w internal/service/subscription_site_test.go && git add internal/service/subscription_site_test.go && git commit -m "test: SubscribeFromSiteと並べ替えとカテゴリ設定のテストを追加する"
```

---

## Task 10: OPMLServiceの入出力

Files:
- Create: `/Users/yujiokamoto/devs/golang/feedflow-go-htmx/internal/service/opml.go`
- Test: `/Users/yujiokamoto/devs/golang/feedflow-go-htmx/internal/service/opml_test.go`

設計書セクション3.1のOPMLのインポートとエクスポートを実装します。Importはencoding/xmlでOPMLをパースし、各outlineのxmlUrlをフィードURLとしてSubscribeを呼び、新規購読数を返します。すでに購読済みのURLは重複として数えずスキップします。Exportは現在の購読をOPMLのバイト列として返します。OPMLのXMLマッピングはこのファイル内の非公開型で表します。port.OPMLServiceを満たします。

- [ ] Step 1: 失敗するテストを書く

Create `internal/service/opml_test.go`:
```go
package service_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/okamyuji/feedflow-go-htmx/internal/domain"
	"github.com/okamyuji/feedflow-go-htmx/internal/port"
	"github.com/okamyuji/feedflow-go-htmx/internal/service"
)

var _ port.OPMLService = (*service.OPMLService)(nil)

func newOPMLDeps(repo *fakeRepo, fetch *fakeFetcher, parse fakeParser, now time.Time) service.Deps {
	return newDeps(repo, fetch, parse, now, &fakeIDGen{})
}

func TestOPMLServiceImport(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 5, 29, 12, 0, 0, 0, time.UTC)
	repo := newFakeRepo()
	fetch := newFakeFetcher()
	fetch.results["https://a.example/feed"] = port.FetchResult{StatusCode: 200, Body: []byte("<rss></rss>")}
	fetch.results["https://b.example/feed"] = port.FetchResult{StatusCode: 200, Body: []byte("<rss></rss>")}
	parse := fakeParser{parsed: port.ParsedFeed{Format: port.FormatRSS2, Title: "imported"}}
	deps := newOPMLDeps(repo, fetch, parse, now)

	sub := service.NewSubscriptionService(deps)
	svc := service.NewOPMLService(deps, sub)

	opml := `<?xml version="1.0" encoding="UTF-8"?>
<opml version="2.0">
  <head><title>subs</title></head>
  <body>
    <outline text="cat1">
      <outline type="rss" text="A" xmlUrl="https://a.example/feed"/>
      <outline type="rss" text="B" xmlUrl="https://b.example/feed"/>
    </outline>
  </body>
</opml>`

	count, err := svc.Import(context.Background(), []byte(opml))
	if err != nil {
		t.Fatalf("Import returned error: %v", err)
	}
	if count != 2 {
		t.Fatalf("imported count got %d want 2", count)
	}
	feeds, _ := repo.Feeds()
	if len(feeds) != 2 {
		t.Fatalf("repo feeds got %d want 2", len(feeds))
	}
}

func TestOPMLServiceImportSkipsDuplicates(t *testing.T) {
	t.Parallel()
	now := time.Now()
	repo := newFakeRepo()
	_ = repo.SaveFeed(domain.Feed{ID: "f0", FeedURL: "https://a.example/feed"})
	fetch := newFakeFetcher()
	fetch.results["https://b.example/feed"] = port.FetchResult{StatusCode: 200, Body: []byte("<rss></rss>")}
	parse := fakeParser{parsed: port.ParsedFeed{Format: port.FormatRSS2, Title: "imported"}}
	deps := newOPMLDeps(repo, fetch, parse, now)

	sub := service.NewSubscriptionService(deps)
	svc := service.NewOPMLService(deps, sub)

	opml := `<opml version="2.0"><body>
    <outline type="rss" xmlUrl="https://a.example/feed"/>
    <outline type="rss" xmlUrl="https://b.example/feed"/>
  </body></opml>`

	count, err := svc.Import(context.Background(), []byte(opml))
	if err != nil {
		t.Fatalf("Import returned error: %v", err)
	}
	if count != 1 {
		t.Fatalf("imported count got %d want 1 (duplicate skipped)", count)
	}
}

func TestOPMLServiceImportInvalidXML(t *testing.T) {
	t.Parallel()
	now := time.Now()
	repo := newFakeRepo()
	deps := newOPMLDeps(repo, newFakeFetcher(), fakeParser{}, now)
	sub := service.NewSubscriptionService(deps)
	svc := service.NewOPMLService(deps, sub)

	if _, err := svc.Import(context.Background(), []byte("not xml at all <<<")); err == nil {
		t.Fatalf("Import must return error for invalid xml")
	}
}

func TestOPMLServiceExport(t *testing.T) {
	t.Parallel()
	now := time.Now()
	repo := newFakeRepo()
	_ = repo.SaveFeed(domain.Feed{ID: "f1", Title: "Feed One", FeedURL: "https://a.example/feed", SiteURL: "https://a.example"})
	_ = repo.SaveFeed(domain.Feed{ID: "f2", Title: "Feed Two", FeedURL: "https://b.example/feed", SiteURL: "https://b.example"})
	deps := newOPMLDeps(repo, newFakeFetcher(), fakeParser{}, now)
	sub := service.NewSubscriptionService(deps)
	svc := service.NewOPMLService(deps, sub)

	data, err := svc.Export()
	if err != nil {
		t.Fatalf("Export returned error: %v", err)
	}
	out := string(data)
	if !strings.Contains(out, `xmlUrl="https://a.example/feed"`) {
		t.Fatalf("export missing feed A: %s", out)
	}
	if !strings.Contains(out, `xmlUrl="https://b.example/feed"`) {
		t.Fatalf("export missing feed B: %s", out)
	}
	if !strings.Contains(out, `text="Feed One"`) {
		t.Fatalf("export missing title: %s", out)
	}
	if !strings.HasPrefix(out, "<?xml") {
		t.Fatalf("export must start with xml declaration: %s", out)
	}
}

func TestOPMLServiceExportImportRoundTrip(t *testing.T) {
	t.Parallel()
	now := time.Now()

	srcRepo := newFakeRepo()
	_ = srcRepo.SaveFeed(domain.Feed{ID: "f1", Title: "Feed One", FeedURL: "https://a.example/feed"})
	srcDeps := newOPMLDeps(srcRepo, newFakeFetcher(), fakeParser{}, now)
	srcSvc := service.NewOPMLService(srcDeps, service.NewSubscriptionService(srcDeps))
	data, err := srcSvc.Export()
	if err != nil {
		t.Fatalf("Export returned error: %v", err)
	}

	dstRepo := newFakeRepo()
	fetch := newFakeFetcher()
	fetch.results["https://a.example/feed"] = port.FetchResult{StatusCode: 200, Body: []byte("<rss></rss>")}
	dstDeps := newOPMLDeps(dstRepo, fetch, fakeParser{parsed: port.ParsedFeed{Title: "Feed One"}}, now)
	dstSvc := service.NewOPMLService(dstDeps, service.NewSubscriptionService(dstDeps))

	count, err := dstSvc.Import(context.Background(), data)
	if err != nil {
		t.Fatalf("Import returned error: %v", err)
	}
	if count != 1 {
		t.Fatalf("round-trip import count got %d want 1", count)
	}
}
```

- [ ] Step 2: テストが失敗することを確認する

Run:
```bash
cd /Users/yujiokamoto/devs/golang/feedflow-go-htmx && go test ./internal/service/ -run TestOPMLService -v
```
Expected: コンパイルエラーで失敗します。`undefined: service.OPMLService` や `undefined: service.NewOPMLService` と表示されます。

- [ ] Step 3: OPMLServiceの最小実装を書く

OPMLServiceは購読追加をSubscriptionServiceに委譲します。重複判定はSubscribeが返すErrDuplicateFeedで識別し、その場合はカウントせずスキップします。outlineは入れ子になりうるため再帰的にxmlUrlを集めます。

Create `internal/service/opml.go`:
```go
package service

import (
	"context"
	"encoding/xml"
	"errors"
	"fmt"

	"github.com/okamyuji/feedflow-go-htmx/internal/port"
)

// opmlDocument OPML文書全体のXMLマッピングです。
type opmlDocument struct {
	XMLName xml.Name  `xml:"opml"`
	Version string    `xml:"version,attr"`
	Head    opmlHead  `xml:"head"`
	Body    opmlBody  `xml:"body"`
}

// opmlHead OPMLのヘッダ部です。
type opmlHead struct {
	Title string `xml:"title"`
}

// opmlBody OPMLの本体部です。outlineを入れ子で持ちます。
type opmlBody struct {
	Outlines []opmlOutline `xml:"outline"`
}

// opmlOutline OPMLのoutline要素です。カテゴリ表現として入れ子になりえます。
type opmlOutline struct {
	Text     string        `xml:"text,attr"`
	Title    string        `xml:"title,attr"`
	Type     string        `xml:"type,attr"`
	XMLURL   string        `xml:"xmlUrl,attr"`
	HTMLURL  string        `xml:"htmlUrl,attr"`
	Outlines []opmlOutline `xml:"outline"`
}

// OPMLService OPMLの入出力を担います。購読追加はSubscriptionServiceに委譲します。
// port.OPMLService を満たします。
type OPMLService struct {
	deps Deps
	subs port.SubscriptionService
}

// NewOPMLService 依存束と購読サービスを受け取りOPMLServiceを構築します。
func NewOPMLService(deps Deps, subs port.SubscriptionService) *OPMLService {
	return &OPMLService{deps: deps, subs: subs}
}

// Import OPMLのバイト列を読み込み、各outlineのxmlUrlを購読に追加します。
// 新規に購読したフィード数を返します。すでに購読済みのURLはスキップしてカウントしません。
func (s *OPMLService) Import(ctx context.Context, data []byte) (int, error) {
	var doc opmlDocument
	if err := xml.Unmarshal(data, &doc); err != nil {
		return 0, fmt.Errorf("failed to parse opml: %w", err)
	}
	urls := collectFeedURLs(doc.Body.Outlines)
	count := 0
	for _, u := range urls {
		_, err := s.subs.Subscribe(ctx, u, nil)
		if err != nil {
			if errors.Is(err, ErrDuplicateFeed) {
				continue
			}
			return count, fmt.Errorf("failed to subscribe %s during opml import: %w", u, err)
		}
		count++
	}
	return count, nil
}

// collectFeedURLs outlineを再帰的に走査し、空でないxmlUrlを順序を保って集めます。
func collectFeedURLs(outlines []opmlOutline) []string {
	var urls []string
	for _, o := range outlines {
		if o.XMLURL != "" {
			urls = append(urls, o.XMLURL)
		}
		urls = append(urls, collectFeedURLs(o.Outlines)...)
	}
	return urls
}

// Export 現在の購読をOPMLのバイト列として返します。
func (s *OPMLService) Export() ([]byte, error) {
	feeds, err := s.deps.Repo.Feeds()
	if err != nil {
		return nil, fmt.Errorf("failed to load feeds: %w", err)
	}
	outlines := make([]opmlOutline, 0, len(feeds))
	for _, f := range feeds {
		outlines = append(outlines, opmlOutline{
			Text:    f.Title,
			Title:   f.Title,
			Type:    "rss",
			XMLURL:  f.FeedURL,
			HTMLURL: f.SiteURL,
		})
	}
	doc := opmlDocument{
		Version: "2.0",
		Head:    opmlHead{Title: "feedflow subscriptions"},
		Body:    opmlBody{Outlines: outlines},
	}
	body, err := xml.MarshalIndent(doc, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("failed to marshal opml: %w", err)
	}
	out := append([]byte(xml.Header), body...)
	return out, nil
}
```

- [ ] Step 4: テストが通ることを確認する

Run:
```bash
cd /Users/yujiokamoto/devs/golang/feedflow-go-htmx && go test ./internal/service/ -run TestOPMLService -v
```
Expected: `TestOPMLServiceImport`と`TestOPMLServiceImportSkipsDuplicates`と`TestOPMLServiceImportInvalidXML`と`TestOPMLServiceExport`と`TestOPMLServiceExportImportRoundTrip`がいずれもPASSします。

- [ ] Step 5: gofmtを適用してコミットする

```bash
cd /Users/yujiokamoto/devs/golang/feedflow-go-htmx && gofmt -w internal/service/opml.go internal/service/opml_test.go && git add internal/service/opml.go internal/service/opml_test.go && git commit -m "feat: OPMLServiceの入出力を追加する"
```

---

## Task 11: フェーズ全体のテストと品質ゲート

Files:
- 変更なし

- [ ] Step 1: サービス層の全テストをraceで実行する

Run:
```bash
cd /Users/yujiokamoto/devs/golang/feedflow-go-htmx && go test -race -count=1 ./internal/service/...
```
Expected: `ok  github.com/okamyuji/feedflow-go-htmx/internal/service` と表示されます。

- [ ] Step 2: サービス層のカバレッジを確認する

Run:
```bash
cd /Users/yujiokamoto/devs/golang/feedflow-go-htmx && go test -coverprofile=coverage.out ./internal/service/... && go tool cover -func=coverage.out | tail -n 1
```
Expected: 合計カバレッジが80パーセント前後以上になります。目安であり厳密な合否基準ではありません。

- [ ] Step 3: 全パッケージのテストを通す

Run:
```bash
cd /Users/yujiokamoto/devs/golang/feedflow-go-htmx && go test -race -count=1 ./...
```
Expected: すべてのパッケージで `ok` または `no test files` と表示されます。

- [ ] Step 4: 品質ゲートを実行する

Run:
```bash
cd /Users/yujiokamoto/devs/golang/feedflow-go-htmx && bash scripts/quality-gate.sh
```
Expected: `all quality checks passed` で終わります。lintやvetの指摘が出たら修正してから再実行します。

- [ ] Step 5: 品質ゲート緑のままコミットする

```bash
cd /Users/yujiokamoto/devs/golang/feedflow-go-htmx && git add -A && git commit -m "chore: Phase4のサービス層で品質ゲートを緑化する"
```
Expected: コミット時にquality-gateが走り、緑のままコミットされます。差分が無ければこのコミットは省略できます。

---

## Phase4完了条件

- [ ] `go test -race ./internal/service/...` が通る
- [ ] port.SubscriptionServiceとport.ItemServiceとport.RetentionServiceとport.MuteServiceとport.OPMLServiceとport.SettingsServiceをそれぞれサービスの具象型が満たし、`var _ port.X = (*service.Y)(nil)` でコンパイル時に検証されている
- [ ] 購読の追加と削除と一覧と整理、記事の既読とスターとあとで読むとタグとボードとメモとハイライト、保持ポリシー適用(最新N件と既読M日、アクション済みは永久保持)、ミュートフィルタ適用、OPMLの入出力、設定の取得と更新が、いずれもフェイク注入のテーブル駆動テストで検証されている
- [ ] 各サービスはportのインターフェースにコンストラクタ注入で依存し、具象型に直接依存していない
- [ ] エラーは握り潰さずfmt.Errorfで文脈付きにラップしている
- [ ] `bash scripts/quality-gate.sh` が `all quality checks passed` で終わる
- [ ] コミットが規約に沿って積まれている
