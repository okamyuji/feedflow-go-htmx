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
		// 事前にキャンセル済みなら新規取得を始めない(決定的な早期打ち切り)。
		if err := ctx.Err(); err != nil {
			break
		}
		// ワーカー枠の確保待ち中もキャンセルを尊重する。全枠が埋まっている間に
		// キャンセルされたら新規取得を始めず、開始済みの完了だけを待つ。
		// 枠を確保できたフィードだけをprocessedに数える。
		select {
		case <-ctx.Done():
			// 確保待ち中にキャンセルされた。
		case sem <- struct{}{}:
			processed.Add(1)
			wg.Add(1)
			go func(feedID string) {
				defer wg.Done()
				defer func() { <-sem }()
				pollOne(ctx, feedID)
			}(id)
			continue
		}
		break
	}
	wg.Wait()
	return int(processed.Load())
}
