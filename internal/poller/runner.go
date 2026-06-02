package poller

import (
	"context"
	"fmt"
	"time"

	"github.com/okamyuji/feedflow-go-htmx/internal/port"
)

// Runner バックグラウンドで期限の来たフィードを定期取得するスケジューラです。
// svcとrepoとclockをコンストラクタ注入で受け取り、設定に従って巡回します。
type Runner struct {
	svc   port.PollService
	repo  port.Repository
	clock port.Clock
	cfg   Config

	// pollOne 1フィードを取得する関数です。既定ではsvc.PollFeedを呼びます。
	// テストでは同時実行の観測のために差し替えます。
	pollOne func(ctx context.Context, feedID string)
}

// NewRunner 依存と設定を注入してRunnerを生成します。設定はゼロ値や不正値を既定へ補正します。
func NewRunner(svc port.PollService, repo port.Repository, clock port.Clock, cfg Config) *Runner {
	r := &Runner{
		svc:   svc,
		repo:  repo,
		clock: clock,
		cfg:   cfg.normalize(),
	}
	r.pollOne = func(ctx context.Context, feedID string) {
		_, _ = r.svc.PollFeed(ctx, feedID)
	}
	return r
}

// dueFeedIDs 現時点で取得対象のフィードID群を返します。
// 期限判定はServiceが用いるのと同じeffectiveIntervalとLastFetchedAtの規則に従います。
// ジッタはRunnerでは掛けず、間隔ちょうどの経過で対象にします。
func (r *Runner) dueFeedIDs() ([]string, error) {
	feeds, err := r.repo.Feeds()
	if err != nil {
		return nil, err
	}
	settings, err := r.repo.Settings()
	if err != nil {
		return nil, err
	}
	now := r.clock.Now()
	ids := make([]string, 0, len(feeds))
	zeroJitter := func(_ time.Duration) time.Duration { return 0 }
	for _, f := range feeds {
		if dueForPollWithJitter(f, settings, now, zeroJitter) {
			ids = append(ids, f.ID)
		}
	}
	return ids, nil
}

// pollDue 期限の来た全フィードを並行数制限つきで取得し、処理したフィード数を返します。
// context がキャンセルされたら新規の取得を開始せず、開始済みの取得の完了を待ってから戻ります。
func (r *Runner) pollDue(ctx context.Context) int {
	if err := ctx.Err(); err != nil {
		return 0
	}
	ids, err := r.dueFeedIDs()
	if err != nil {
		return 0
	}

	return pollFeedsConcurrently(ctx, ids, r.cfg.MaxConcurrent, r.pollOne)
}

// Run contextがキャンセルされるまで設定間隔ごとに期限フィードを取得し続けます。
// 起動直後に一度巡回し、その後はTickerの刻みごとに巡回します。
// contextのキャンセルでTickerを止め、進行中の取得の完了を待ってから戻ります。
func (r *Runner) Run(ctx context.Context) {
	if err := ctx.Err(); err != nil {
		return
	}
	r.pollDue(ctx)

	ticker := time.NewTicker(r.cfg.TickInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			r.pollDue(ctx)
		}
	}
}

// PollNow 手動更新で指定フィードを即時取得し、新着件数を返します。
// 定期巡回の期限判定を経由せず、その場で取得反映を行います。
func (r *Runner) PollNow(ctx context.Context, feedID string) (int, error) {
	n, err := r.svc.PollFeed(ctx, feedID)
	if err != nil {
		return 0, fmt.Errorf("failed to poll feed now %q: %w", feedID, err)
	}
	return n, nil
}
