// Package poller feedflowのフィード取得反映と定期ポーリングを提供します。
// このパッケージはinternal/portのインターフェースにコンストラクタ注入で依存し、
// 具体的な実装には依存しません。設計書のセクション4.2と8に対応します。
package poller

import (
	"errors"
	"time"
)

// errFeedNotFound 指定IDのフィードが見つからないことを表します。
var errFeedNotFound = errors.New("poller: feed not found")

// ポーラーの設定の既定値です。設計書のセクション4.2に対応します。
const (
	// defaultTickInterval Runnerが期限到来フィードを走査する間隔です。
	// 最短のフィード上書き間隔が15分のため、1分ごとの走査で十分に細かく検知できます。
	defaultTickInterval = time.Minute
	// defaultMaxConcurrent 同時に取得するフィード数の既定の上限です。
	defaultMaxConcurrent = 4
	// defaultPollAllConcurrency 手動の全件取得(PollAllNow/PollAll)で同時に取得するフィード数の上限です。
	// 背景巡回のdefaultMaxConcurrentより高めにし、手動の即時性を優先します。
	defaultPollAllConcurrency = 8
	// defaultJitterRatio 巡回判定に乗せるジッタの割合です。間隔の最大10パーセントを散らします。
	defaultJitterRatio = 0.1
)

// Config Runnerのバックグラウンド巡回の設定を保持します。
type Config struct {
	TickInterval  time.Duration // 期限到来フィードを走査する間隔です
	MaxConcurrent int           // 同時取得するフィード数の上限です
	JitterRatio   float64       // 取得判定に乗せるジッタの割合です。0以上1未満です
}

// DefaultConfig 既定値で初期化した設定を返します。
func DefaultConfig() Config {
	return Config{
		TickInterval:  defaultTickInterval,
		MaxConcurrent: defaultMaxConcurrent,
		JitterRatio:   defaultJitterRatio,
	}
}

// normalize 不正値や未設定値を既定値へ補正した新しい設定を返します。
// 受け取った値は変更せず、補正後の新しいConfigを返します。
func (c Config) normalize() Config {
	out := c
	if out.TickInterval <= 0 {
		out.TickInterval = defaultTickInterval
	}
	if out.MaxConcurrent < 1 {
		out.MaxConcurrent = defaultMaxConcurrent
	}
	if out.JitterRatio < 0 || out.JitterRatio >= 1 {
		out.JitterRatio = defaultJitterRatio
	}
	return out
}
