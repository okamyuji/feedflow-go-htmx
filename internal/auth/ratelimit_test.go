package auth

import (
	"testing"
	"time"
)

func TestRateLimiterAllowsUpToBurst(t *testing.T) {
	t.Parallel()
	clk := &fakeClock{now: time.Date(2026, 5, 29, 0, 0, 0, 0, time.UTC)}
	// 毎分5回まで、バースト5。
	rl := NewRateLimiter(RateLimitConfig{
		Clock:       clk,
		Burst:       5,
		RefillEvery: time.Minute / 5,
	})

	for i := range 5 {
		if !rl.Allow("1.2.3.4") {
			t.Fatalf("Allow attempt %d got false want true within burst", i+1)
		}
	}
	if rl.Allow("1.2.3.4") {
		t.Fatalf("Allow attempt 6 got true want false beyond burst")
	}
}

func TestRateLimiterRefillsOverTime(t *testing.T) {
	t.Parallel()
	clk := &fakeClock{now: time.Date(2026, 5, 29, 0, 0, 0, 0, time.UTC)}
	rl := NewRateLimiter(RateLimitConfig{
		Clock:       clk,
		Burst:       2,
		RefillEvery: 10 * time.Second,
	})

	first := rl.Allow("ip")
	second := rl.Allow("ip")
	if !first || !second {
		t.Fatalf("初期バースト 2 回が許可されませんでした")
	}
	if rl.Allow("ip") {
		t.Fatalf("バースト超過が許可されました")
	}

	// 10秒経過で1トークン補充されます。
	clk.now = clk.now.Add(10 * time.Second)
	if !rl.Allow("ip") {
		t.Fatalf("補充後の 1 回が許可されませんでした")
	}
	if rl.Allow("ip") {
		t.Fatalf("補充は 1 トークンのはずが追加で許可されました")
	}
}

func TestRateLimiterIsolatesKeys(t *testing.T) {
	t.Parallel()
	clk := &fakeClock{now: time.Date(2026, 5, 29, 0, 0, 0, 0, time.UTC)}
	rl := NewRateLimiter(RateLimitConfig{
		Clock:       clk,
		Burst:       1,
		RefillEvery: time.Minute,
	})

	if !rl.Allow("a") {
		t.Fatalf("key a の初回が拒否されました")
	}
	if rl.Allow("a") {
		t.Fatalf("key a の 2 回目が許可されました")
	}
	if !rl.Allow("b") {
		t.Fatalf("別 key b の初回が拒否されました。キーが分離されていません")
	}
}

func TestRateLimiterCapsAtBurst(t *testing.T) {
	t.Parallel()
	clk := &fakeClock{now: time.Date(2026, 5, 29, 0, 0, 0, 0, time.UTC)}
	rl := NewRateLimiter(RateLimitConfig{
		Clock:       clk,
		Burst:       3,
		RefillEvery: time.Second,
	})

	// 1回だけ消費し、長時間放置してもバースト上限を超えて貯まらないことを確認します。
	if !rl.Allow("ip") {
		t.Fatalf("初回が拒否されました")
	}
	clk.now = clk.now.Add(time.Hour)
	allowed := 0
	for range 10 {
		if rl.Allow("ip") {
			allowed++
		}
	}
	if allowed != 3 {
		t.Fatalf("補充上限後に許可された回数 got %d want 3", allowed)
	}
}
