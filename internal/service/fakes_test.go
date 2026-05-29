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
