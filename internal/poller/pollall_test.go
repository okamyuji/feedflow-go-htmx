package poller

import (
	"context"
	"testing"
	"time"

	"github.com/okamyuji/feedflow-go-htmx/internal/domain"
	"github.com/okamyuji/feedflow-go-htmx/internal/port"
)

func TestServicePollAllSelectsDueFeeds(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 5, 29, 12, 0, 0, 0, time.UTC)
	repo := newFakeRepo()
	_ = repo.SaveSettings(domain.Settings{
		PollInterval:      domain.Poll30Min,
		MaxItems:          200,
		ReadRetentionDays: 30,
		Theme:             domain.ThemeDark,
		DefaultView:       domain.ViewCard,
	})
	// 期限経過で対象
	_ = repo.SaveFeed(domain.Feed{ID: "due", FeedURL: "https://a.example/feed", PollInterval: domain.Poll30Min, LastFetchedAt: now.Add(-time.Hour)})
	// 間隔未経過で対象外
	_ = repo.SaveFeed(domain.Feed{ID: "fresh", FeedURL: "https://b.example/feed", PollInterval: domain.Poll30Min, LastFetchedAt: now.Add(-time.Minute)})
	// 手動のみで対象外
	_ = repo.SaveFeed(domain.Feed{ID: "manual", FeedURL: "https://c.example/feed", PollInterval: domain.PollManualOnly, LastFetchedAt: now.Add(-100 * time.Hour)})
	// 未取得で対象
	_ = repo.SaveFeed(domain.Feed{ID: "never", FeedURL: "https://d.example/feed", PollInterval: domain.Poll1Hour})

	fetcher := newFakeFetcher()
	fetcher.results["https://a.example/feed"] = port.FetchResult{StatusCode: 200, Body: []byte("<rss></rss>")}
	fetcher.results["https://d.example/feed"] = port.FetchResult{StatusCode: 200, Body: []byte("<rss></rss>")}
	parser := fakeParser{parsed: port.ParsedFeed{}}

	// ジッタを0に固定して決定的に判定する
	svc := NewService(repo, fetcher, parser, newFakeClock(now), &fakeIDGen{}, passthroughMute{})
	svc.jitter = func(time.Duration) time.Duration { return 0 }

	processed, err := svc.PollAll(context.Background())
	if err != nil {
		t.Fatalf("PollAll returned error: %v", err)
	}
	if processed != 2 {
		t.Fatalf("processed got %d want 2", processed)
	}
	if fetcher.calls("https://a.example/feed") != 1 {
		t.Fatalf("due feed must be fetched once, got %d", fetcher.calls("https://a.example/feed"))
	}
	if fetcher.calls("https://d.example/feed") != 1 {
		t.Fatalf("never-fetched feed must be fetched once, got %d", fetcher.calls("https://d.example/feed"))
	}
	if fetcher.calls("https://b.example/feed") != 0 {
		t.Fatalf("fresh feed must not be fetched, got %d", fetcher.calls("https://b.example/feed"))
	}
	if fetcher.calls("https://c.example/feed") != 0 {
		t.Fatalf("manual feed must not be fetched, got %d", fetcher.calls("https://c.example/feed"))
	}
}

func TestServicePollAllNowIgnoresDueSettings(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 5, 29, 12, 0, 0, 0, time.UTC)
	repo := newFakeRepo()
	_ = repo.SaveSettings(domain.Settings{
		PollInterval:      domain.PollManualOnly,
		MaxItems:          200,
		ReadRetentionDays: 30,
		Theme:             domain.ThemeDark,
		DefaultView:       domain.ViewCard,
	})
	_ = repo.SaveFeed(domain.Feed{ID: "manual", FeedURL: "https://manual.example/feed", PollInterval: domain.PollManualOnly, LastFetchedAt: now})
	_ = repo.SaveFeed(domain.Feed{ID: "fresh", FeedURL: "https://fresh.example/feed", PollInterval: domain.Poll30Min, LastFetchedAt: now})

	fetcher := newFakeFetcher()
	fetcher.results["https://manual.example/feed"] = port.FetchResult{StatusCode: 200, Body: []byte("<rss></rss>")}
	fetcher.results["https://fresh.example/feed"] = port.FetchResult{StatusCode: 200, Body: []byte("<rss></rss>")}
	parser := fakeParser{parsed: port.ParsedFeed{}}
	svc := NewService(repo, fetcher, parser, newFakeClock(now), &fakeIDGen{}, passthroughMute{})

	processed, err := svc.PollAllNow(context.Background())
	if err != nil {
		t.Fatalf("PollAllNow returned error: %v", err)
	}
	if processed != 2 {
		t.Fatalf("processed got %d want 2", processed)
	}
	if fetcher.calls("https://manual.example/feed") != 1 {
		t.Fatalf("manual feed must be fetched on explicit poll, got %d", fetcher.calls("https://manual.example/feed"))
	}
	if fetcher.calls("https://fresh.example/feed") != 1 {
		t.Fatalf("fresh feed must be fetched on explicit poll, got %d", fetcher.calls("https://fresh.example/feed"))
	}
}

func TestServicePollAllContinuesOnError(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 5, 29, 12, 0, 0, 0, time.UTC)
	repo := newFakeRepo()
	_ = repo.SaveSettings(domain.DefaultSettings())
	_ = repo.SaveFeed(domain.Feed{ID: "bad", FeedURL: "https://bad.example/feed", PollInterval: domain.Poll15Min})
	_ = repo.SaveFeed(domain.Feed{ID: "good", FeedURL: "https://good.example/feed", PollInterval: domain.Poll15Min})

	fetcher := newFakeFetcher()
	fetcher.errs["https://bad.example/feed"] = context.DeadlineExceeded
	fetcher.results["https://good.example/feed"] = port.FetchResult{StatusCode: 200, Body: []byte("<rss></rss>")}
	parser := fakeParser{parsed: port.ParsedFeed{}}

	svc := NewService(repo, fetcher, parser, newFakeClock(now), &fakeIDGen{}, passthroughMute{})
	svc.jitter = func(time.Duration) time.Duration { return 0 }

	processed, err := svc.PollAll(context.Background())
	if err != nil {
		t.Fatalf("PollAll returned error: %v", err)
	}
	if processed != 2 {
		t.Fatalf("processed got %d want 2 (both attempted)", processed)
	}
	if fetcher.calls("https://good.example/feed") != 1 {
		t.Fatalf("good feed must still be fetched after bad feed error, got %d", fetcher.calls("https://good.example/feed"))
	}
}

func TestServicePollAllCanceledContext(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 5, 29, 12, 0, 0, 0, time.UTC)
	repo := newFakeRepo()
	_ = repo.SaveSettings(domain.DefaultSettings())
	_ = repo.SaveFeed(domain.Feed{ID: "f1", FeedURL: "https://a.example/feed", PollInterval: domain.Poll15Min})

	svc := NewService(repo, newFakeFetcher(), fakeParser{}, newFakeClock(now), &fakeIDGen{}, passthroughMute{})
	svc.jitter = func(time.Duration) time.Duration { return 0 }

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	processed, err := svc.PollAll(ctx)
	if err == nil {
		t.Fatalf("PollAll must return error for canceled context")
	}
	if processed != 0 {
		t.Fatalf("processed got %d want 0 on immediate cancel", processed)
	}
}
