package poller

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/okamyuji/feedflow-go-htmx/internal/domain"
)

func TestRunnerPollDueRespectsConcurrencyLimit(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 5, 29, 12, 0, 0, 0, time.UTC)
	repo := newFakeRepo()
	_ = repo.SaveSettings(domain.Settings{
		PollInterval:      domain.Poll15Min,
		MaxItems:          200,
		ReadRetentionDays: 30,
		Theme:             domain.ThemeDark,
		DefaultView:       domain.ViewCard,
	})
	const feedCount = 10
	for i := range feedCount {
		id := "f" + string(rune('a'+i))
		_ = repo.SaveFeed(domain.Feed{ID: id, FeedURL: "https://" + id + ".example/feed", PollInterval: domain.Poll15Min})
	}

	svc := NewService(repo, newFakeFetcher(), fakeParser{}, newFakeClock(now), &fakeIDGen{}, passthroughMute{})
	svc.jitter = func(time.Duration) time.Duration { return 0 }

	cfg := DefaultConfig()
	cfg.MaxConcurrent = 3
	runner := NewRunner(svc, repo, newFakeClock(now), cfg)

	var inFlight atomic.Int32
	var peak int32
	var mu sync.Mutex
	// 取得関数を差し替え、同時実行のピークを観測する
	runner.pollOne = func(_ context.Context, _ string) {
		cur := inFlight.Add(1)
		mu.Lock()
		if cur > peak {
			peak = cur
		}
		mu.Unlock()
		time.Sleep(5 * time.Millisecond)
		inFlight.Add(-1)
	}

	processed := runner.pollDue(context.Background())
	if processed != feedCount {
		t.Fatalf("processed got %d want %d", processed, feedCount)
	}
	if peak > int32(cfg.MaxConcurrent) {
		t.Fatalf("concurrency peak got %d want <= %d", peak, cfg.MaxConcurrent)
	}
	if peak == 0 {
		t.Fatalf("concurrency peak got 0 want > 0")
	}
}

func TestRunnerPollDueCanceledBeforeStart(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 5, 29, 12, 0, 0, 0, time.UTC)
	repo := newFakeRepo()
	_ = repo.SaveSettings(domain.DefaultSettings())
	_ = repo.SaveFeed(domain.Feed{ID: "f1", FeedURL: "https://f1.example/feed", PollInterval: domain.Poll15Min})

	svc := NewService(repo, newFakeFetcher(), fakeParser{}, newFakeClock(now), &fakeIDGen{}, passthroughMute{})
	svc.jitter = func(time.Duration) time.Duration { return 0 }
	runner := NewRunner(svc, repo, newFakeClock(now), DefaultConfig())

	var calls int32
	runner.pollOne = func(_ context.Context, _ string) { atomic.AddInt32(&calls, 1) }

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	processed := runner.pollDue(ctx)
	if processed != 0 {
		t.Fatalf("processed got %d want 0 on canceled context", processed)
	}
	if atomic.LoadInt32(&calls) != 0 {
		t.Fatalf("pollOne calls got %d want 0", calls)
	}
}
