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

const brokenFeedURL = "http://old.example.com/atom.xml"

// newBrokenFeedFixture 旧URLで取得エラーが続くフィードと記事を1件ずつ持つリポジトリを返します。
func newBrokenFeedFixture() *fakeRepo {
	repo := newFakeRepo()
	_ = repo.SaveFeed(domain.Feed{
		ID: "f1", FeedURL: brokenFeedURL, Title: "Old", CategoryIDs: []string{"c1"},
		ETag: "old-etag", LastModified: "old-lm", ConsecutiveErrors: 4525, LastError: "HTTP 500",
	})
	_ = repo.SaveItems("f1", []domain.Item{{ID: "i1", FeedID: "f1", GUID: "g1", Read: true}})
	return repo
}

func TestUpdateFeedURLReplacesURLAndClearsError(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 10, 30, 8, 0, 0, 0, time.UTC)
	repo := newBrokenFeedFixture()
	fetch := newFakeFetcher()
	fetch.results["https://new.example.com/index.xml"] = port.FetchResult{
		StatusCode: 200, Body: []byte("<rss></rss>"), ETag: "new-etag", LastModified: "new-lm",
	}
	parse := fakeParser{parsed: port.ParsedFeed{Title: "New", SiteURL: "https://new.example.com/"}}
	svc := service.NewSubscriptionService(newDeps(repo, fetch, parse, now, &fakeIDGen{}))

	got, err := svc.UpdateFeedURL(context.Background(), "f1", " https://new.example.com/index.xml ")
	if err != nil {
		t.Fatalf("UpdateFeedURL returned error: %v", err)
	}
	stored, _ := repo.Feed("f1")
	if stored.FeedURL != "https://new.example.com/index.xml" || got.FeedURL != stored.FeedURL {
		t.Fatalf("FeedURL got stored=%q returned=%q", stored.FeedURL, got.FeedURL)
	}
	if stored.ConsecutiveErrors != 0 || stored.LastError != "" || stored.HasError() {
		t.Fatalf("error state must be cleared, got errors=%d LastError=%q", stored.ConsecutiveErrors, stored.LastError)
	}
	if stored.ETag != "" || stored.LastModified != "" {
		t.Fatalf("validators got etag=%q lm=%q want both empty so the next poll imports items", stored.ETag, stored.LastModified)
	}
	if stored.Title != "New" || stored.SiteURL != "https://new.example.com/" || !stored.LastFetchedAt.Equal(now) {
		t.Fatalf("feed meta got %+v", stored)
	}
	if len(stored.CategoryIDs) != 1 || stored.CategoryIDs[0] != "c1" {
		t.Fatalf("categories must be kept, got %v", stored.CategoryIDs)
	}
	items, _ := repo.Items("f1")
	if len(items) != 1 || !items[0].Read {
		t.Fatalf("existing items and read state must be kept, got %+v", items)
	}
}

func TestUpdateFeedURLDiscoversFeedFromSiteURL(t *testing.T) {
	t.Parallel()
	repo := newBrokenFeedFixture()
	fetch := newFakeFetcher()
	fetch.results["https://new.example.com/"] = port.FetchResult{
		StatusCode: 200,
		Body:       []byte(`<html><head><link rel="alternate" type="application/rss+xml" href="/index.xml"></head></html>`),
	}
	fetch.results["https://new.example.com/index.xml"] = port.FetchResult{StatusCode: 200, Body: []byte("<rss></rss>")}
	parse := fakeParser{parsed: port.ParsedFeed{Title: "New"}, rejectHTML: true}
	svc := service.NewSubscriptionService(newDeps(repo, fetch, parse, time.Now(), &fakeIDGen{}))

	got, err := svc.UpdateFeedURL(context.Background(), "f1", "https://new.example.com/")
	if err != nil {
		t.Fatalf("UpdateFeedURL returned error: %v", err)
	}
	if got.FeedURL != "https://new.example.com/index.xml" {
		t.Fatalf("FeedURL got %q want the discovered feed", got.FeedURL)
	}
}

func TestUpdateFeedURLKeepsFeedOnFailure(t *testing.T) {
	t.Parallel()
	siteWithoutFeed := port.FetchResult{StatusCode: 200, Body: []byte(`<html><head></head></html>`)}
	tests := []struct {
		name    string
		feedID  string
		rawURL  string
		setup   func(*fakeRepo, *fakeFetcher)
		wantErr error
	}{
		{name: "URLの形式が不正", feedID: "f1", rawURL: "ftp://example.com/feed", wantErr: service.ErrInvalidURL},
		{name: "空のURL", feedID: "f1", rawURL: "  ", wantErr: service.ErrInvalidURL},
		{
			name: "取得に失敗", feedID: "f1", rawURL: "https://down.example.com/feed",
			setup: func(_ *fakeRepo, f *fakeFetcher) { f.err = errors.New("HTTP 500") },
		},
		{
			name: "サイトにフィードのリンクが無い", feedID: "f1", rawURL: "https://plain.example.com/",
			setup:   func(_ *fakeRepo, f *fakeFetcher) { f.results["https://plain.example.com/"] = siteWithoutFeed },
			wantErr: service.ErrFeedNotDiscovered,
		},
		{
			name: "他のフィードと重複", feedID: "f1", rawURL: "https://other.example.com/feed",
			setup: func(r *fakeRepo, f *fakeFetcher) {
				_ = r.SaveFeed(domain.Feed{ID: "f2", FeedURL: "https://other.example.com/feed"})
				f.results["https://other.example.com/feed"] = port.FetchResult{StatusCode: 200, Body: []byte("<rss/>")}
			},
			wantErr: service.ErrDuplicateFeed,
		},
		{name: "存在しないフィード", feedID: "missing", rawURL: "https://new.example.com/feed"},
		{name: "合成フィード", feedID: domain.SavedPagesFeedID, rawURL: "https://new.example.com/feed", wantErr: service.ErrFeedNotEditable},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			repo := newBrokenFeedFixture()
			fetch := newFakeFetcher()
			if tt.setup != nil {
				tt.setup(repo, fetch)
			}
			svc := service.NewSubscriptionService(newDeps(repo, fetch, fakeParser{rejectHTML: true}, time.Now(), &fakeIDGen{}))

			_, err := svc.UpdateFeedURL(context.Background(), tt.feedID, tt.rawURL)
			if err == nil {
				t.Fatalf("UpdateFeedURL must fail")
			}
			if tt.wantErr != nil && !errors.Is(err, tt.wantErr) {
				t.Fatalf("error got %v want %v", err, tt.wantErr)
			}
			stored, _ := repo.Feed("f1")
			if stored.FeedURL != brokenFeedURL || stored.ConsecutiveErrors != 4525 || stored.ETag != "old-etag" {
				t.Fatalf("feed must stay unchanged on failure, got %+v", stored)
			}
		})
	}
}

func TestUpdateFeedURLAcceptsSameURLForRetry(t *testing.T) {
	t.Parallel()
	repo := newBrokenFeedFixture()
	fetch := newFakeFetcher()
	fetch.results[brokenFeedURL] = port.FetchResult{StatusCode: 200, Body: []byte("<rss/>")}
	svc := service.NewSubscriptionService(newDeps(repo, fetch, fakeParser{}, time.Now(), &fakeIDGen{}))

	if _, err := svc.UpdateFeedURL(context.Background(), "f1", brokenFeedURL); err != nil {
		t.Fatalf("re-entering the current url must not be a duplicate: %v", err)
	}
	stored, _ := repo.Feed("f1")
	if stored.HasError() {
		t.Fatalf("error state must be cleared, got %d", stored.ConsecutiveErrors)
	}
}

func TestUpdateFeedURLReportsSaveFailure(t *testing.T) {
	t.Parallel()
	repo := newBrokenFeedFixture()
	repo.failOn["SaveFeed"] = errors.New("disk full")
	fetch := newFakeFetcher()
	fetch.results["https://new.example.com/feed"] = port.FetchResult{StatusCode: 200, Body: []byte("<rss/>")}
	svc := service.NewSubscriptionService(newDeps(repo, fetch, fakeParser{}, time.Now(), &fakeIDGen{}))

	if _, err := svc.UpdateFeedURL(context.Background(), "f1", "https://new.example.com/feed"); err == nil {
		t.Fatalf("UpdateFeedURL must report the save failure")
	}
}

func TestUpdateFeedURLFetchesURLAsTyped(t *testing.T) {
	t.Parallel()
	repo := newBrokenFeedFixture()
	fetch := newFakeFetcher()
	fetch.results["https://new.example.com/blog/"] = port.FetchResult{
		StatusCode: 200,
		Body:       []byte(`<html><head><link rel="alternate" type="application/atom+xml" href="feed.atom"></head></html>`),
	}
	fetch.results["https://new.example.com/blog/feed.atom"] = port.FetchResult{StatusCode: 200, Body: []byte("<feed/>")}
	svc := service.NewSubscriptionService(newDeps(repo, fetch, fakeParser{rejectHTML: true}, time.Now(), &fakeIDGen{}))

	got, err := svc.UpdateFeedURL(context.Background(), "f1", "https://new.example.com/blog/")
	if err != nil {
		t.Fatalf("a trailing slash must be kept because servers treat /blog and /blog/ differently: %v", err)
	}
	if got.FeedURL != "https://new.example.com/blog/feed.atom" {
		t.Fatalf("FeedURL got %q", got.FeedURL)
	}
}
