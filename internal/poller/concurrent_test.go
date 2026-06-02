package poller

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// TestPollFeedsConcurrentlyRespectsLimit 同時実行数が上限を超えず、かつ並列に走ることを確認します。
func TestPollFeedsConcurrentlyRespectsLimit(t *testing.T) {
	t.Parallel()
	const total = 12
	const limit = 4
	ids := make([]string, total)
	for i := range ids {
		ids[i] = "f" + string(rune('a'+i))
	}

	var inFlight atomic.Int32
	var peak atomic.Int32
	pollOne := func(_ context.Context, _ string) {
		cur := inFlight.Add(1)
		for {
			old := peak.Load()
			if cur <= old || peak.CompareAndSwap(old, cur) {
				break
			}
		}
		time.Sleep(5 * time.Millisecond)
		inFlight.Add(-1)
	}

	processed := pollFeedsConcurrently(context.Background(), ids, limit, pollOne)
	if processed != total {
		t.Fatalf("processed got %d want %d", processed, total)
	}
	if got := peak.Load(); got > limit {
		t.Fatalf("concurrency peak got %d want <= %d", got, limit)
	}
	if got := peak.Load(); got < 2 {
		t.Fatalf("concurrency peak got %d want >= 2 (must run in parallel)", got)
	}
}

// TestPollFeedsConcurrentlyProcessesAll 全フィードがちょうど1回ずつ取得されることを確認します。
func TestPollFeedsConcurrentlyProcessesAll(t *testing.T) {
	t.Parallel()
	ids := []string{"a", "b", "c", "d", "e"}
	var mu sync.Mutex
	seen := map[string]int{}
	pollOne := func(_ context.Context, id string) {
		mu.Lock()
		seen[id]++
		mu.Unlock()
	}

	processed := pollFeedsConcurrently(context.Background(), ids, 8, pollOne)
	if processed != len(ids) {
		t.Fatalf("processed got %d want %d", processed, len(ids))
	}
	for _, id := range ids {
		if seen[id] != 1 {
			t.Fatalf("feed %q polled %d times want 1", id, seen[id])
		}
	}
}

// TestPollFeedsConcurrentlyCanceledBeforeStart 事前キャンセル時は一切起動せず0を返すことを確認します。
func TestPollFeedsConcurrentlyCanceledBeforeStart(t *testing.T) {
	t.Parallel()
	var calls int32
	pollOne := func(_ context.Context, _ string) { atomic.AddInt32(&calls, 1) }

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	processed := pollFeedsConcurrently(ctx, []string{"a", "b", "c"}, 4, pollOne)
	if processed != 0 {
		t.Fatalf("processed got %d want 0 on canceled context", processed)
	}
	if atomic.LoadInt32(&calls) != 0 {
		t.Fatalf("pollOne calls got %d want 0", calls)
	}
}

// TestPollFeedsConcurrentlyNormalizesLimit limitが1未満でも1として動作することを確認します。
func TestPollFeedsConcurrentlyNormalizesLimit(t *testing.T) {
	t.Parallel()
	var calls int32
	pollOne := func(_ context.Context, _ string) { atomic.AddInt32(&calls, 1) }

	processed := pollFeedsConcurrently(context.Background(), []string{"a", "b"}, 0, pollOne)
	if processed != 2 {
		t.Fatalf("processed got %d want 2", processed)
	}
	if atomic.LoadInt32(&calls) != 2 {
		t.Fatalf("pollOne calls got %d want 2", calls)
	}
}

// TestPollFeedsConcurrentlyEmpty 空入力では何も起動せず0を返すことを確認します。
func TestPollFeedsConcurrentlyEmpty(t *testing.T) {
	t.Parallel()
	var calls int32
	pollOne := func(_ context.Context, _ string) { atomic.AddInt32(&calls, 1) }
	processed := pollFeedsConcurrently(context.Background(), nil, 4, pollOne)
	if processed != 0 || atomic.LoadInt32(&calls) != 0 {
		t.Fatalf("processed=%d calls=%d want 0/0", processed, calls)
	}
}
