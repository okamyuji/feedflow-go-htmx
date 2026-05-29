package auth

import (
	"sync"
	"time"

	"github.com/okamyuji/feedflow-go-htmx/internal/port"
)

// bucket 1キーぶんのトークンバケットの状態です。
type bucket struct {
	tokens     float64
	lastRefill time.Time
}

// RateLimitConfig レートリミッタの設定です。
type RateLimitConfig struct {
	Clock       port.Clock    // トークン補充の基準になる時刻源です
	Burst       int           // 同時に許す最大トークン数です
	RefillEvery time.Duration // トークンを1個補充する間隔です
}

// RateLimiter キーごとの簡易トークンバケットによるレートリミッタです。
// 標準ライブラリだけで実装し外部依存を増やしません。ログイン試行の抑制に使います。設計書のセクション9.1に対応します。
type RateLimiter struct {
	mu          sync.Mutex
	buckets     map[string]*bucket
	clock       port.Clock
	burst       float64
	refillEvery time.Duration
}

// NewRateLimiter 設定からレートリミッタを生成します。
func NewRateLimiter(cfg RateLimitConfig) *RateLimiter {
	return &RateLimiter{
		buckets:     make(map[string]*bucket),
		clock:       cfg.Clock,
		burst:       float64(cfg.Burst),
		refillEvery: cfg.RefillEvery,
	}
}

// Allow キーに対する1回の試行を許可するかどうかを返します。
// 経過時間に応じてトークンを補充し、1個以上あれば1個消費してtrueを返します。
func (rl *RateLimiter) Allow(key string) bool {
	now := rl.clock.Now()
	rl.mu.Lock()
	defer rl.mu.Unlock()

	b, ok := rl.buckets[key]
	if !ok {
		b = &bucket{tokens: rl.burst, lastRefill: now}
		rl.buckets[key] = b
	}

	if rl.refillEvery > 0 {
		elapsed := now.Sub(b.lastRefill)
		refill := float64(elapsed) / float64(rl.refillEvery)
		if refill > 0 {
			b.tokens += refill
			if b.tokens > rl.burst {
				b.tokens = rl.burst
			}
			b.lastRefill = now
		}
	}

	if b.tokens >= 1 {
		b.tokens--
		return true
	}
	return false
}
