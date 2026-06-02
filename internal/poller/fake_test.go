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
// delaysにURLごとの遅延を設定すると、並列取得の検証のためにその時間だけ待ってから応答します。
type fakeFetcher struct {
	mu        sync.Mutex
	results   map[string]port.FetchResult
	errs      map[string]error
	delays    map[string]time.Duration
	callCount map[string]int
}

func newFakeFetcher() *fakeFetcher {
	return &fakeFetcher{
		results:   map[string]port.FetchResult{},
		errs:      map[string]error{},
		delays:    map[string]time.Duration{},
		callCount: map[string]int{},
	}
}

func (f *fakeFetcher) Fetch(ctx context.Context, req port.FetchRequest) (port.FetchResult, error) {
	if err := ctx.Err(); err != nil {
		return port.FetchResult{}, err
	}
	// 遅延中に他goroutineの取得をブロックしないよう、mutexは状態読み取りの間だけ保持します。
	f.mu.Lock()
	f.callCount[req.URL]++
	err := f.errs[req.URL]
	res, ok := f.results[req.URL]
	delay := f.delays[req.URL]
	f.mu.Unlock()

	if delay > 0 {
		select {
		case <-ctx.Done():
			return port.FetchResult{}, ctx.Err()
		case <-time.After(delay):
		}
	}

	if err != nil {
		return port.FetchResult{}, err
	}
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

func (r *fakeRepo) Categories() ([]domain.Category, error) { return nil, nil }
func (r *fakeRepo) SaveCategory(_ domain.Category) error   { return nil }
func (r *fakeRepo) DeleteCategory(_ string) error          { return nil }

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

func (r *fakeRepo) Bookmarks() ([]domain.Bookmark, error) { return nil, nil }
func (r *fakeRepo) SaveBookmark(_ domain.Bookmark) error  { return nil }
func (r *fakeRepo) DeleteBookmark(_ string) error         { return nil }
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

func (r *fakeRepo) User() (domain.User, error)   { return domain.User{}, nil }
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
