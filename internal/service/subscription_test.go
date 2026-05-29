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
