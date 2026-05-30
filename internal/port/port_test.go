package port_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/okamyuji/feedflow-go-htmx/internal/domain"
	"github.com/okamyuji/feedflow-go-htmx/internal/port"
)

// fakeClock 固定時刻を返すClockのフェイクです。
type fakeClock struct{ now time.Time }

func (c fakeClock) Now() time.Time { return c.now }

// fakeIDGen 決定的な連番IDを返すIDGenのフェイクです。
type fakeIDGen struct{ n int }

func (g *fakeIDGen) NewID() string {
	g.n++
	return "id-" + time.Duration(g.n).String()
}

// fakeFetcher 固定の結果を返すFetcherのフェイクです。
type fakeFetcher struct {
	result port.FetchResult
	err    error
}

func (f fakeFetcher) Fetch(_ context.Context, _ port.FetchRequest) (port.FetchResult, error) {
	return f.result, f.err
}

// fakeParser 固定の結果を返すFeedParserのフェイクです。
type fakeParser struct {
	parsed port.ParsedFeed
	err    error
}

func (p fakeParser) Parse(_ []byte) (port.ParsedFeed, error) {
	return p.parsed, p.err
}

// fakeRepo メモリ上で全エンティティを保持するRepositoryのフェイクです。
type fakeRepo struct {
	feeds      map[string]domain.Feed
	categories map[string]domain.Category
	items      map[string][]domain.Item
	bookmarks  map[string]domain.Bookmark
	filters    map[string]domain.MuteFilter
	settings   domain.Settings
	user       domain.User
}

func newFakeRepo() *fakeRepo {
	return &fakeRepo{
		feeds:      map[string]domain.Feed{},
		categories: map[string]domain.Category{},
		items:      map[string][]domain.Item{},
		bookmarks:  map[string]domain.Bookmark{},
		filters:    map[string]domain.MuteFilter{},
		settings:   domain.DefaultSettings(),
	}
}

var errNotFound = errors.New("not found")

func (r *fakeRepo) Feeds() ([]domain.Feed, error) {
	out := make([]domain.Feed, 0, len(r.feeds))
	for _, f := range r.feeds {
		out = append(out, f)
	}
	return out, nil
}

func (r *fakeRepo) Feed(id string) (domain.Feed, error) {
	f, ok := r.feeds[id]
	if !ok {
		return domain.Feed{}, errNotFound
	}
	return f, nil
}

func (r *fakeRepo) SaveFeed(feed domain.Feed) error {
	r.feeds[feed.ID] = feed
	return nil
}

func (r *fakeRepo) DeleteFeed(id string) error {
	delete(r.feeds, id)
	delete(r.items, id)
	return nil
}

func (r *fakeRepo) Categories() ([]domain.Category, error) {
	out := make([]domain.Category, 0, len(r.categories))
	for _, c := range r.categories {
		out = append(out, c)
	}
	return out, nil
}

func (r *fakeRepo) SaveCategory(category domain.Category) error {
	r.categories[category.ID] = category
	return nil
}

func (r *fakeRepo) DeleteCategory(id string) error {
	delete(r.categories, id)
	return nil
}

func (r *fakeRepo) Items(feedID string) ([]domain.Item, error) {
	return r.items[feedID], nil
}

func (r *fakeRepo) SaveItems(feedID string, items []domain.Item) error {
	r.items[feedID] = items
	return nil
}

func (r *fakeRepo) Bookmarks() ([]domain.Bookmark, error) {
	out := make([]domain.Bookmark, 0, len(r.bookmarks))
	for _, b := range r.bookmarks {
		out = append(out, b)
	}
	return out, nil
}

func (r *fakeRepo) SaveBookmark(bookmark domain.Bookmark) error {
	r.bookmarks[bookmark.ID] = bookmark
	return nil
}

func (r *fakeRepo) DeleteBookmark(id string) error {
	delete(r.bookmarks, id)
	return nil
}

func (r *fakeRepo) Filters() ([]domain.MuteFilter, error) {
	out := make([]domain.MuteFilter, 0, len(r.filters))
	for _, f := range r.filters {
		out = append(out, f)
	}
	return out, nil
}

func (r *fakeRepo) SaveFilter(filter domain.MuteFilter) error {
	r.filters[filter.ID] = filter
	return nil
}

func (r *fakeRepo) DeleteFilter(id string) error {
	delete(r.filters, id)
	return nil
}

func (r *fakeRepo) Settings() (domain.Settings, error) {
	return r.settings, nil
}

func (r *fakeRepo) SaveSettings(settings domain.Settings) error {
	r.settings = settings
	return nil
}

func (r *fakeRepo) User() (domain.User, error) {
	return r.user, nil
}

func (r *fakeRepo) SaveUser(user domain.User) error {
	r.user = user
	return nil
}

// インターフェース充足をコンパイル時に検証します。
var (
	_ port.Clock      = fakeClock{}
	_ port.IDGen      = (*fakeIDGen)(nil)
	_ port.Fetcher    = fakeFetcher{}
	_ port.FeedParser = fakeParser{}
	_ port.Repository = (*fakeRepo)(nil)
)

func TestFakesSatisfyPorts(t *testing.T) {
	t.Parallel()
	repo := newFakeRepo()
	if err := repo.SaveFeed(domain.Feed{ID: "f1", Title: "feed"}); err != nil {
		t.Fatalf("SaveFeed returned error: %v", err)
	}
	got, err := repo.Feed("f1")
	if err != nil {
		t.Fatalf("Feed returned error: %v", err)
	}
	if got.Title != "feed" {
		t.Fatalf("Feed title got %q want %q", got.Title, "feed")
	}

	clk := fakeClock{now: time.Date(2026, 5, 29, 0, 0, 0, 0, time.UTC)}
	if clk.Now().Year() != 2026 {
		t.Fatalf("fakeClock year got %d want 2026", clk.Now().Year())
	}

	gen := &fakeIDGen{}
	first := gen.NewID()
	second := gen.NewID()
	if first == second {
		t.Fatalf("fakeIDGen must return distinct ids got %q and %q", first, second)
	}

	f := fakeFetcher{result: port.FetchResult{StatusCode: 200, Body: []byte("ok")}}
	res, err := f.Fetch(context.Background(), port.FetchRequest{URL: "https://example.com"})
	if err != nil {
		t.Fatalf("Fetch returned error: %v", err)
	}
	if res.StatusCode != 200 {
		t.Fatalf("Fetch status got %d want 200", res.StatusCode)
	}

	p := fakeParser{parsed: port.ParsedFeed{Format: port.FormatRSS2, Title: "t"}}
	parsed, err := p.Parse([]byte("<rss></rss>"))
	if err != nil {
		t.Fatalf("Parse returned error: %v", err)
	}
	if parsed.Format != port.FormatRSS2 {
		t.Fatalf("Parse format got %q want %q", parsed.Format, port.FormatRSS2)
	}
}
