package poller

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/okamyuji/feedflow-go-htmx/internal/domain"
	"github.com/okamyuji/feedflow-go-htmx/internal/port"
)

func TestServicePollFeedNewItems(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 5, 29, 12, 0, 0, 0, time.UTC)
	repo := newFakeRepo()
	_ = repo.SaveFeed(domain.Feed{ID: "f1", FeedURL: "https://example.com/feed", Title: "old", ConsecutiveErrors: 6, LastError: "HTTP 403"})
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
	if feed.ConsecutiveErrors != 0 || feed.LastError != "" {
		t.Fatalf("feed got errors=%d LastError=%q want both cleared", feed.ConsecutiveErrors, feed.LastError)
	}
}

func TestServicePollFeedNotModified(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 5, 29, 12, 0, 0, 0, time.UTC)
	repo := newFakeRepo()
	_ = repo.SaveFeed(domain.Feed{ID: "f1", FeedURL: "https://example.com/feed", ETag: "etag-old", ConsecutiveErrors: 6, LastError: "HTTP 403"})
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
	if feed.ConsecutiveErrors != 0 || feed.LastError != "" {
		t.Fatalf("feed got errors=%d LastError=%q want both cleared", feed.ConsecutiveErrors, feed.LastError)
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
	if !strings.Contains(feed.LastError, "network down") {
		t.Fatalf("feed LastError got %q want it to contain the cause", feed.LastError)
	}
	// 失敗時も最終取得時刻を進めないと、期限切れのまま毎tick取り直して相手サーバを叩き続けます。
	if !feed.LastFetchedAt.Equal(now) {
		t.Fatalf("feed LastFetchedAt got %v want %v so the next retry waits for the interval", feed.LastFetchedAt, now)
	}
	if dueForPollWithJitter(feed, domain.DefaultSettings(), now.Add(time.Minute), func(time.Duration) time.Duration { return 0 }) {
		t.Fatalf("a failed feed must not be due again one minute later")
	}
}

func TestServicePollFeedReportsSaveFailureAfterFetchError(t *testing.T) {
	t.Parallel()
	repo := newFakeRepo()
	_ = repo.SaveFeed(domain.Feed{ID: "f1", FeedURL: "https://example.com/feed"})
	saveErr := errors.New("disk full")
	repo.saveFeedErr = saveErr
	fetcher := newFakeFetcher()
	fetcher.errs["https://example.com/feed"] = errors.New("network down")
	svc := NewService(repo, fetcher, fakeParser{}, newFakeClock(time.Now()), &fakeIDGen{}, passthroughMute{})

	_, err := svc.PollFeed(context.Background(), "f1")
	if !errors.Is(err, saveErr) || !strings.Contains(err.Error(), "network down") {
		t.Fatalf("error got %v want the save failure with the fetch cause", err)
	}
}

func TestServicePollFeedParseErrorRecordsLastError(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 5, 29, 12, 0, 0, 0, time.UTC)
	repo := newFakeRepo()
	_ = repo.SaveFeed(domain.Feed{ID: "f1", FeedURL: "https://example.com/feed"})
	fetcher := newFakeFetcher()
	fetcher.results["https://example.com/feed"] = port.FetchResult{StatusCode: 200, Body: []byte("<html>")}
	svc := NewService(repo, fetcher, fakeParser{err: errors.New("not a feed")}, newFakeClock(now), &fakeIDGen{}, passthroughMute{})

	if _, err := svc.PollFeed(context.Background(), "f1"); err == nil {
		t.Fatalf("PollFeed must return error on parse failure")
	}
	feed, _ := repo.Feed("f1")
	if feed.ConsecutiveErrors != 1 || !strings.Contains(feed.LastError, "not a feed") {
		t.Fatalf("feed got errors=%d LastError=%q want 1 and the parse cause", feed.ConsecutiveErrors, feed.LastError)
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
