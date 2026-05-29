package poller

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/okamyuji/feedflow-go-htmx/internal/domain"
	"github.com/okamyuji/feedflow-go-htmx/internal/port"
)

func TestRunnerRunStopsOnContextCancel(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 5, 29, 12, 0, 0, 0, time.UTC)
	repo := newFakeRepo()
	_ = repo.SaveSettings(domain.DefaultSettings())
	_ = repo.SaveFeed(domain.Feed{ID: "f1", FeedURL: "https://f1.example/feed", PollInterval: domain.Poll15Min})

	svc := NewService(repo, newFakeFetcher(), fakeParser{}, newFakeClock(now), &fakeIDGen{}, passthroughMute{})
	svc.jitter = func(time.Duration) time.Duration { return 0 }

	cfg := DefaultConfig()
	cfg.TickInterval = 5 * time.Millisecond
	runner := NewRunner(svc, repo, newFakeClock(now), cfg)

	var ticks int32
	runner.pollOne = func(_ context.Context, _ string) { atomic.AddInt32(&ticks, 1) }

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		runner.Run(ctx)
		close(done)
	}()

	// 何度か巡回するのを待ってからキャンセルする
	time.Sleep(40 * time.Millisecond)
	cancel()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatalf("Run did not return within timeout after cancel")
	}

	if atomic.LoadInt32(&ticks) == 0 {
		t.Fatalf("Run must have polled at least once before cancel")
	}
}

func TestRunnerRunReturnsImmediatelyIfPreCanceled(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 5, 29, 12, 0, 0, 0, time.UTC)
	repo := newFakeRepo()
	_ = repo.SaveSettings(domain.DefaultSettings())

	svc := NewService(repo, newFakeFetcher(), fakeParser{}, newFakeClock(now), &fakeIDGen{}, passthroughMute{})
	runner := NewRunner(svc, repo, newFakeClock(now), DefaultConfig())

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	done := make(chan struct{})
	go func() {
		runner.Run(ctx)
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatalf("Run did not return promptly for pre-canceled context")
	}
}

func TestRunnerPollNow(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 5, 29, 12, 0, 0, 0, time.UTC)
	repo := newFakeRepo()
	_ = repo.SaveFeed(domain.Feed{ID: "f1", FeedURL: "https://f1.example/feed"})

	fetcher := newFakeFetcher()
	fetcher.results["https://f1.example/feed"] = port.FetchResult{StatusCode: 200, Body: []byte("<rss></rss>")}
	parser := fakeParser{parsed: port.ParsedFeed{Items: []port.ParsedItem{{GUID: "g1", Title: "新着"}}}}

	svc := NewService(repo, fetcher, parser, newFakeClock(now), &fakeIDGen{}, passthroughMute{})
	runner := NewRunner(svc, repo, newFakeClock(now), DefaultConfig())

	n, err := runner.PollNow(context.Background(), "f1")
	if err != nil {
		t.Fatalf("PollNow returned error: %v", err)
	}
	if n != 1 {
		t.Fatalf("PollNow new item count got %d want 1", n)
	}
	if fetcher.calls("https://f1.example/feed") != 1 {
		t.Fatalf("PollNow must fetch the feed immediately, got %d", fetcher.calls("https://f1.example/feed"))
	}
}
