package poller

import (
	"math/rand/v2"
	"time"

	"github.com/okamyuji/feedflow-go-htmx/internal/domain"
)

// jitterFunc 取得判定に乗せるジッタを返す関数型です。
// 引数は対象フィードのポーリング間隔で、戻り値は前倒しを許す時間幅です。
// テストではジッタを固定するためにこの関数を差し替えます。
type jitterFunc func(interval time.Duration) time.Duration

// ratioJitter 間隔に対する割合で0以上その割合分以下のジッタを返す関数を生成します。
// 割合が0以下のときは常に0を返し、ジッタを無効にします。
func ratioJitter(ratio float64) jitterFunc {
	return func(interval time.Duration) time.Duration {
		if ratio <= 0 || interval <= 0 {
			return 0
		}
		maxJitter := time.Duration(float64(interval) * ratio)
		if maxJitter <= 0 {
			return 0
		}
		// ジッタは取得時刻を散らすための非暗号用途のため、弱い乱数で十分です。
		return time.Duration(rand.Int64N(int64(maxJitter) + 1)) // #nosec G404
	}
}

// effectiveInterval 指定フィードに適用するポーリング間隔を返します。
// フィードの上書きがdefaultまたは空のときは全体設定の間隔を使います。
// manualのときと不正値のときはゼロを返し、定期取得の対象外とします。
func effectiveInterval(feed domain.Feed, settings domain.Settings) time.Duration {
	pi := feed.PollInterval
	if pi == "" || pi == domain.PollDefault {
		pi = settings.PollInterval
	}
	d, ok := pi.Duration()
	if !ok {
		return 0
	}
	return d
}

// dueForPollWithJitter 指定フィードが現時点で取得対象かどうかをジッタ込みで返します。
// 間隔がゼロのフィード(手動のみや不正値)は常に対象外です。
// 最終取得が未設定のフィードは常に対象です。
// それ以外は最終取得からの経過が、間隔からジッタ分を引いた値以上のとき対象とします。
func dueForPollWithJitter(feed domain.Feed, settings domain.Settings, now time.Time, jitter jitterFunc) bool {
	interval := effectiveInterval(feed, settings)
	if interval <= 0 {
		return false
	}
	if feed.LastFetchedAt.IsZero() {
		return true
	}
	threshold := max(interval-jitter(interval), 0)
	return now.Sub(feed.LastFetchedAt) >= threshold
}
