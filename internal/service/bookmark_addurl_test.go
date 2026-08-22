package service_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/okamyuji/feedflow-go-htmx/internal/domain"
	"github.com/okamyuji/feedflow-go-htmx/internal/port"
	"github.com/okamyuji/feedflow-go-htmx/internal/service"
)

// pageHTML タイトル抽出の検証に使う保存対象ページのHTMLです。
const pageHTML = `<html><head><title>記事タイトル</title></head><body></body></html>`

// newAddURLSvc AddURLの検証用にサービスとフェイク一式を組んで返します。
func newAddURLSvc() (*service.BookmarkService, *fakeRepo, *fakeFetcher) {
	repo := newFakeRepo()
	fetch := newFakeFetcher()
	deps := newDeps(repo, fetch, fakeParser{}, time.Date(2026, 8, 22, 9, 0, 0, 0, time.UTC), &fakeIDGen{})
	items := service.NewItemService(deps, service.NewMuteService(deps))
	return service.NewBookmarkService(deps, items), repo, fetch
}

// serveHTML フェイクフェッチャが指定URLでHTMLを返すように仕込みます。
func serveHTML(f *fakeFetcher, url, body string) {
	f.results[url] = port.FetchResult{
		StatusCode:  200,
		Body:        []byte(body),
		ContentType: "text/html; charset=utf-8",
	}
}

func TestAddURLCreatesSavedPagesFeedAndItem(t *testing.T) {
	t.Parallel()
	svc, repo, fetch := newAddURLSvc()
	serveHTML(fetch, "https://example.com/a", pageHTML)

	it, err := svc.AddURL(context.Background(), "https://example.com/a/", "")
	if err != nil {
		t.Fatalf("AddURL returned error: %v", err)
	}
	if it.FeedID != domain.SavedPagesFeedID {
		t.Errorf("FeedID = %q, want %q", it.FeedID, domain.SavedPagesFeedID)
	}
	if it.Title != "記事タイトル" {
		t.Errorf("Title = %q, want %q", it.Title, "記事タイトル")
	}
	if it.Link != "https://example.com/a" {
		t.Errorf("Link = %q, want the normalized url", it.Link)
	}
	if it.GUID != "https://example.com/a" {
		t.Errorf("GUID = %q, want the normalized url", it.GUID)
	}
	if !it.Bookmarked {
		t.Error("Bookmarked = false, want true")
	}
	if len(it.BookmarkIDs) != 0 {
		t.Errorf("BookmarkIDs = %v, want empty", it.BookmarkIDs)
	}

	f, err := repo.Feed(domain.SavedPagesFeedID)
	if err != nil {
		t.Fatalf("saved pages feed was not created: %v", err)
	}
	if f.Title != domain.SavedPagesFeedTitle {
		t.Errorf("feed title = %q, want %q", f.Title, domain.SavedPagesFeedTitle)
	}
	if f.PollInterval != domain.PollManualOnly {
		t.Errorf("feed poll interval = %v, want PollManualOnly", f.PollInterval)
	}
	if f.FeedURL != "" {
		t.Errorf("feed url = %q, want empty", f.FeedURL)
	}
}

func TestAddURLAssignsBookmarkLabel(t *testing.T) {
	t.Parallel()
	svc, repo, fetch := newAddURLSvc()
	if err := repo.SaveBookmark(domain.Bookmark{ID: "b1", Name: "あとで"}); err != nil {
		t.Fatalf("SaveBookmark returned error: %v", err)
	}
	serveHTML(fetch, "https://example.com/a", pageHTML)

	it, err := svc.AddURL(context.Background(), "https://example.com/a", "b1")
	if err != nil {
		t.Fatalf("AddURL returned error: %v", err)
	}
	if len(it.BookmarkIDs) != 1 || it.BookmarkIDs[0] != "b1" {
		t.Errorf("BookmarkIDs = %v, want [b1]", it.BookmarkIDs)
	}
}

func TestAddURLStacksNewestFirst(t *testing.T) {
	t.Parallel()
	svc, repo, fetch := newAddURLSvc()
	serveHTML(fetch, "https://example.com/a", pageHTML)
	serveHTML(fetch, "https://example.com/b", pageHTML)

	if _, err := svc.AddURL(context.Background(), "https://example.com/a", ""); err != nil {
		t.Fatalf("first AddURL returned error: %v", err)
	}
	if _, err := svc.AddURL(context.Background(), "https://example.com/b", ""); err != nil {
		t.Fatalf("second AddURL returned error: %v", err)
	}

	items, err := repo.Items(domain.SavedPagesFeedID)
	if err != nil {
		t.Fatalf("Items returned error: %v", err)
	}
	if len(items) != 2 {
		t.Fatalf("item count = %d, want 2", len(items))
	}
	if items[0].Link != "https://example.com/b" {
		t.Errorf("first item link = %q, want the newest url", items[0].Link)
	}
}

func TestAddURLSecondTimeReusesExistingItem(t *testing.T) {
	t.Parallel()
	svc, repo, fetch := newAddURLSvc()
	if err := repo.SaveBookmark(domain.Bookmark{ID: "b1", Name: "あとで"}); err != nil {
		t.Fatalf("SaveBookmark returned error: %v", err)
	}
	serveHTML(fetch, "https://example.com/a", pageHTML)

	if _, err := svc.AddURL(context.Background(), "https://example.com/a", ""); err != nil {
		t.Fatalf("first AddURL returned error: %v", err)
	}
	it, err := svc.AddURL(context.Background(), "https://example.com/a/", "b1")
	if err != nil {
		t.Fatalf("second AddURL returned error: %v", err)
	}

	items, err := repo.Items(domain.SavedPagesFeedID)
	if err != nil {
		t.Fatalf("Items returned error: %v", err)
	}
	if len(items) != 1 {
		t.Errorf("item count = %d, want 1", len(items))
	}
	if len(it.BookmarkIDs) != 1 || it.BookmarkIDs[0] != "b1" {
		t.Errorf("BookmarkIDs = %v, want [b1]", it.BookmarkIDs)
	}
}

func TestAddURLReusesExistingSubscribedItem(t *testing.T) {
	t.Parallel()
	svc, repo, fetch := newAddURLSvc()
	if err := repo.SaveFeed(domain.Feed{ID: "f1", FeedURL: "https://example.com/feed"}); err != nil {
		t.Fatalf("SaveFeed returned error: %v", err)
	}
	if err := repo.SaveItems("f1", []domain.Item{{ID: "i1", FeedID: "f1", Link: "https://example.com/a"}}); err != nil {
		t.Fatalf("SaveItems returned error: %v", err)
	}
	if err := repo.SaveBookmark(domain.Bookmark{ID: "b1", Name: "あとで"}); err != nil {
		t.Fatalf("SaveBookmark returned error: %v", err)
	}

	it, err := svc.AddURL(context.Background(), "https://example.com/a", "b1")
	if err != nil {
		t.Fatalf("AddURL returned error: %v", err)
	}
	if it.FeedID != "f1" || it.ID != "i1" {
		t.Errorf("returned item = %+v, want the existing f1/i1", it)
	}
	if !it.Bookmarked {
		t.Error("the existing item was not marked bookmarked")
	}
	if len(it.BookmarkIDs) != 1 || it.BookmarkIDs[0] != "b1" {
		t.Errorf("BookmarkIDs = %v, want [b1]", it.BookmarkIDs)
	}
	if _, err := repo.Feed(domain.SavedPagesFeedID); err == nil {
		t.Error("the saved pages feed was created even though an existing item matched")
	}
	if len(fetch.calls) != 0 {
		t.Errorf("fetcher was called %d times, want 0 for an existing item", len(fetch.calls))
	}
}

func TestAddURLMatchesExistingItemAcrossURLVariants(t *testing.T) {
	t.Parallel()
	svc, repo, _ := newAddURLSvc()
	if err := repo.SaveFeed(domain.Feed{ID: "f1", FeedURL: "https://example.com/feed"}); err != nil {
		t.Fatalf("SaveFeed returned error: %v", err)
	}
	if err := repo.SaveItems("f1", []domain.Item{{ID: "i1", FeedID: "f1", Link: "https://EXAMPLE.com/a/#top"}}); err != nil {
		t.Fatalf("SaveItems returned error: %v", err)
	}

	it, err := svc.AddURL(context.Background(), "https://example.com/a", "")
	if err != nil {
		t.Fatalf("AddURL returned error: %v", err)
	}
	if it.ID != "i1" {
		t.Errorf("returned item id = %q, want i1", it.ID)
	}
}

func TestAddURLIgnoresItemsWithUnparsableLink(t *testing.T) {
	t.Parallel()
	svc, repo, fetch := newAddURLSvc()
	if err := repo.SaveFeed(domain.Feed{ID: "f1", FeedURL: "https://example.com/feed"}); err != nil {
		t.Fatalf("SaveFeed returned error: %v", err)
	}
	if err := repo.SaveItems("f1", []domain.Item{{ID: "i1", FeedID: "f1", Link: ""}}); err != nil {
		t.Fatalf("SaveItems returned error: %v", err)
	}
	serveHTML(fetch, "https://example.com/a", pageHTML)

	it, err := svc.AddURL(context.Background(), "https://example.com/a", "")
	if err != nil {
		t.Fatalf("AddURL returned error: %v", err)
	}
	if it.FeedID != domain.SavedPagesFeedID {
		t.Errorf("FeedID = %q, want a newly created saved page", it.FeedID)
	}
}

func TestAddURLFallsBackToURLWhenTitleUnavailable(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name  string
		setup func(*fakeFetcher)
	}{
		{
			name:  "取得に失敗",
			setup: func(f *fakeFetcher) { f.err = errors.New("network down") },
		},
		{
			name: "HTML以外",
			setup: func(f *fakeFetcher) {
				f.results["https://example.com/a"] = port.FetchResult{
					StatusCode: 200, Body: []byte("%PDF-1.7"), ContentType: "application/pdf",
				}
			},
		},
		{
			name: "Content-Typeが空",
			setup: func(f *fakeFetcher) {
				f.results["https://example.com/a"] = port.FetchResult{StatusCode: 200, Body: []byte(pageHTML)}
			},
		},
		{
			name: "タイトルが空",
			setup: func(f *fakeFetcher) {
				serveHTML(f, "https://example.com/a", `<html><head></head><body>x</body></html>`)
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			svc, _, fetch := newAddURLSvc()
			tt.setup(fetch)
			it, err := svc.AddURL(context.Background(), "https://example.com/a", "")
			if err != nil {
				t.Fatalf("AddURL returned error: %v", err)
			}
			if it.Title != "https://example.com/a" {
				t.Errorf("Title = %q, want the input url", it.Title)
			}
		})
	}
}

func TestAddURLRejectsInvalidInput(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name       string
		rawURL     string
		bookmarkID string
		wantErr    error
	}{
		{name: "javascriptスキーム", rawURL: "javascript:alert(1)", wantErr: service.ErrInvalidURL},
		{name: "fileスキーム", rawURL: "file:///etc/passwd", wantErr: service.ErrInvalidURL},
		{name: "空文字", rawURL: "", wantErr: service.ErrInvalidURL},
		{name: "host無し", rawURL: "http:///a", wantErr: service.ErrInvalidURL},
		{name: "存在しないラベル", rawURL: "https://example.com/a", bookmarkID: "nope", wantErr: service.ErrBookmarkNotFound},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			svc, repo, _ := newAddURLSvc()
			if _, err := svc.AddURL(context.Background(), tt.rawURL, tt.bookmarkID); !errors.Is(err, tt.wantErr) {
				t.Errorf("AddURL error = %v, want %v", err, tt.wantErr)
			}
			items, err := repo.Items(domain.SavedPagesFeedID)
			if err != nil {
				t.Fatalf("Items returned error: %v", err)
			}
			if len(items) != 0 {
				t.Error("an item was created despite the rejection")
			}
		})
	}
}

func TestAddURLPropagatesRepositoryErrors(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		method string
	}{
		{name: "フィード一覧の失敗", method: "Feeds"},
		{name: "フィード保存の失敗", method: "SaveFeed"},
		{name: "記事保存の失敗", method: "SaveItems"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			svc, repo, fetch := newAddURLSvc()
			serveHTML(fetch, "https://example.com/a", pageHTML)
			boom := errors.New("boom")
			repo.failOn[tt.method] = boom

			if _, err := svc.AddURL(context.Background(), "https://example.com/a", ""); !errors.Is(err, boom) {
				t.Errorf("AddURL error = %v, want the injected error", err)
			}
		})
	}
}

func TestAddURLPropagatesBookmarkListError(t *testing.T) {
	t.Parallel()
	svc, repo, _ := newAddURLSvc()
	boom := errors.New("boom")
	repo.failOn["Bookmarks"] = boom

	if _, err := svc.AddURL(context.Background(), "https://example.com/a", "b1"); !errors.Is(err, boom) {
		t.Errorf("AddURL error = %v, want the injected error", err)
	}
}

func TestAddURLPropagatesErrorWhenMarkingExistingItem(t *testing.T) {
	t.Parallel()
	svc, repo, _ := newAddURLSvc()
	if err := repo.SaveFeed(domain.Feed{ID: "f1", FeedURL: "https://example.com/feed"}); err != nil {
		t.Fatalf("SaveFeed returned error: %v", err)
	}
	if err := repo.SaveItems("f1", []domain.Item{{ID: "i1", FeedID: "f1", Link: "https://example.com/a"}}); err != nil {
		t.Fatalf("SaveItems returned error: %v", err)
	}
	boom := errors.New("boom")
	repo.failOn["SaveItems"] = boom

	if _, err := svc.AddURL(context.Background(), "https://example.com/a", ""); !errors.Is(err, boom) {
		t.Errorf("AddURL error = %v, want the injected error", err)
	}
}

func TestAddURLPropagatesErrorWhenAttachingLabelToExistingItem(t *testing.T) {
	t.Parallel()
	svc, repo, _ := newAddURLSvc()
	if err := repo.SaveFeed(domain.Feed{ID: "f1", FeedURL: "https://example.com/feed"}); err != nil {
		t.Fatalf("SaveFeed returned error: %v", err)
	}
	if err := repo.SaveItems("f1", []domain.Item{{ID: "i1", FeedID: "f1", Link: "https://example.com/a"}}); err != nil {
		t.Fatalf("SaveItems returned error: %v", err)
	}
	if err := repo.SaveBookmark(domain.Bookmark{ID: "b1", Name: "あとで"}); err != nil {
		t.Fatalf("SaveBookmark returned error: %v", err)
	}
	// 記事が見つからない状態にして、ラベル付与の失敗を再現します。
	if err := repo.SaveItems("f1", []domain.Item{{ID: "i1", FeedID: "f1", Link: "https://example.com/a"}}); err != nil {
		t.Fatalf("SaveItems returned error: %v", err)
	}
	boom := errors.New("boom")
	repo.failOn["SaveItems"] = boom

	if _, err := svc.AddURL(context.Background(), "https://example.com/a", "b1"); !errors.Is(err, boom) {
		t.Errorf("AddURL error = %v, want the injected error", err)
	}
}

func TestAddURLPropagatesErrorWhenRereadingUpdatedItem(t *testing.T) {
	t.Parallel()
	svc, repo, _ := newAddURLSvc()
	if err := repo.SaveFeed(domain.Feed{ID: "f1", FeedURL: "https://example.com/feed"}); err != nil {
		t.Fatalf("SaveFeed returned error: %v", err)
	}
	if err := repo.SaveItems("f1", []domain.Item{{ID: "i1", FeedID: "f1", Link: "https://example.com/a"}}); err != nil {
		t.Fatalf("SaveItems returned error: %v", err)
	}
	// 探索と更新で2回読んだあと、更新後の読み直しだけを失敗させます。
	repo.callCount["Items"] = 0
	repo.failAfter["Items"] = 2

	if _, err := svc.AddURL(context.Background(), "https://example.com/a", ""); err == nil {
		t.Error("AddURL returned nil, want the re-read error")
	}
}
