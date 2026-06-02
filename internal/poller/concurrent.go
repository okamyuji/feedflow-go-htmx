package poller

import (
	"context"
	"sync"
	"sync/atomic"
)

// pollFeedsConcurrently 指定フィードID群を最大limit並列でpollOneに渡して取得し、取得を開始したフィード数を返します。
// limitが1未満のときは1に補正します。
// contextがキャンセルされたら新規の取得を開始せず、開始済みの取得の完了を待ってから戻ります。
func pollFeedsConcurrently(ctx context.Context, ids []string, limit int, pollOne func(context.Context, string)) int {
	if limit < 1 {
		limit = 1
	}
	sem := make(chan struct{}, limit)
	var wg sync.WaitGroup
	var processed atomic.Int64
	for _, id := range ids {
		if err := ctx.Err(); err != nil {
			break
		}
		processed.Add(1)
		wg.Add(1)
		sem <- struct{}{}
		go func(feedID string) {
			defer wg.Done()
			defer func() { <-sem }()
			pollOne(ctx, feedID)
		}(id)
	}
	wg.Wait()
	return int(processed.Load())
}
