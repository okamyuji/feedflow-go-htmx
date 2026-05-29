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
