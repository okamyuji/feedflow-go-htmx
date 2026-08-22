package poller

import (
	"context"
	"testing"
	"time"

	"github.com/okamyuji/feedflow-go-htmx/internal/domain"
	"github.com/okamyuji/feedflow-go-htmx/internal/port"
)

// newSavedSkipService 合成フィードの除外を検証するためのServiceとフェイクを組んで返します。
func newSavedSkipService(t *testing.T, now time.Time) (*Service, *fakeRepo, *fakeFetcher) {
	t.Helper()
	repo := newFakeRepo()
	if err := repo.SaveSettings(domain.DefaultSettings()); err != nil {
		t.Fatalf("SaveSettings returned error: %v", err)
	}
	fetcher := newFakeFetcher()
	svc := NewService(repo, fetcher, fakeParser{parsed: port.ParsedFeed{}}, newFakeClock(now), &fakeIDGen{}, passthroughMute{})
	svc.jitter = func(time.Duration) time.Duration { return 0 }
	return svc, repo, fetcher
}

func TestPollFeedSkipsSavedPagesFeed(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 8, 22, 12, 0, 0, 0, time.UTC)
	svc, repo, fetcher := newSavedSkipService(t, now)
	if err := repo.SaveFeed(domain.Feed{
		ID:           domain.SavedPagesFeedID,
		Title:        domain.SavedPagesFeedTitle,
		PollInterval: domain.PollManualOnly,
	}); err != nil {
		t.Fatalf("SaveFeed returned error: %v", err)
	}

	added, err := svc.PollFeed(context.Background(), domain.SavedPagesFeedID)
	if err != nil {
		t.Fatalf("PollFeed returned error: %v", err)
	}
	if added != 0 {
		t.Errorf("added = %d, want 0", added)
	}
	if got := fetcher.calls(""); got != 0 {
		t.Errorf("fetcher was called %d times with an empty url, want 0", got)
	}
	f, err := repo.Feed(domain.SavedPagesFeedID)
	if err != nil {
		t.Fatalf("Feed returned error: %v", err)
	}
	if f.ConsecutiveErrors != 0 {
		t.Errorf("ConsecutiveErrors = %d, want 0", f.ConsecutiveErrors)
	}
	if !f.LastFetchedAt.IsZero() {
		t.Errorf("LastFetchedAt = %v, want the zero value", f.LastFetchedAt)
	}
}

func TestPollAllSkipsSavedPagesFeed(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 8, 22, 12, 0, 0, 0, time.UTC)
	svc, repo, fetcher := newSavedSkipService(t, now)
	if err := repo.SaveFeed(domain.Feed{ID: domain.SavedPagesFeedID, Title: domain.SavedPagesFeedTitle}); err != nil {
		t.Fatalf("SaveFeed returned error: %v", err)
	}
	if err := repo.SaveFeed(domain.Feed{ID: "f1", FeedURL: "https://a.example/feed", PollInterval: domain.Poll30Min}); err != nil {
		t.Fatalf("SaveFeed returned error: %v", err)
	}
	fetcher.results["https://a.example/feed"] = port.FetchResult{StatusCode: 200, Body: []byte("<rss></rss>")}

	processed, err := svc.PollAll(context.Background())
	if err != nil {
		t.Fatalf("PollAll returned error: %v", err)
	}
	if processed != 1 {
		t.Errorf("processed = %d, want 1", processed)
	}
	if got := fetcher.calls(""); got != 0 {
		t.Errorf("fetcher was called %d times with an empty url, want 0", got)
	}
}

func TestPollAllNowSkipsSavedPagesFeed(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 8, 22, 12, 0, 0, 0, time.UTC)
	svc, repo, fetcher := newSavedSkipService(t, now)
	if err := repo.SaveFeed(domain.Feed{ID: domain.SavedPagesFeedID, Title: domain.SavedPagesFeedTitle}); err != nil {
		t.Fatalf("SaveFeed returned error: %v", err)
	}
	if err := repo.SaveFeed(domain.Feed{ID: "f1", FeedURL: "https://a.example/feed"}); err != nil {
		t.Fatalf("SaveFeed returned error: %v", err)
	}
	fetcher.results["https://a.example/feed"] = port.FetchResult{StatusCode: 200, Body: []byte("<rss></rss>")}

	processed, err := svc.PollAllNow(context.Background())
	if err != nil {
		t.Fatalf("PollAllNow returned error: %v", err)
	}
	if processed != 1 {
		t.Errorf("processed = %d, want 1", processed)
	}
	if got := fetcher.calls(""); got != 0 {
		t.Errorf("fetcher was called %d times with an empty url, want 0", got)
	}
	f, err := repo.Feed(domain.SavedPagesFeedID)
	if err != nil {
		t.Fatalf("Feed returned error: %v", err)
	}
	if f.ConsecutiveErrors != 0 {
		t.Errorf("ConsecutiveErrors = %d, want 0", f.ConsecutiveErrors)
	}
}

func TestRunnerDueFeedIDsSkipsSavedPagesFeed(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 8, 22, 12, 0, 0, 0, time.UTC)
	svc, repo, _ := newSavedSkipService(t, now)
	// 合成フィードは未取得のため、除外が無ければ期限判定で必ず対象になります。
	if err := repo.SaveFeed(domain.Feed{ID: domain.SavedPagesFeedID, Title: domain.SavedPagesFeedTitle}); err != nil {
		t.Fatalf("SaveFeed returned error: %v", err)
	}
	if err := repo.SaveFeed(domain.Feed{ID: "f1", FeedURL: "https://a.example/feed"}); err != nil {
		t.Fatalf("SaveFeed returned error: %v", err)
	}
	runner := NewRunner(svc, repo, newFakeClock(now), DefaultConfig())

	ids, err := runner.dueFeedIDs()
	if err != nil {
		t.Fatalf("dueFeedIDs returned error: %v", err)
	}
	for _, id := range ids {
		if domain.IsSavedPagesFeed(id) {
			t.Fatalf("dueFeedIDs returned the saved pages feed: %v", ids)
		}
	}
	if len(ids) != 1 || ids[0] != "f1" {
		t.Errorf("dueFeedIDs = %v, want [f1]", ids)
	}
}
