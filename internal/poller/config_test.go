package poller

import "testing"

func TestDefaultConfig(t *testing.T) {
	t.Parallel()
	c := DefaultConfig()
	if c.TickInterval <= 0 {
		t.Fatalf("TickInterval got %v want positive", c.TickInterval)
	}
	if c.MaxConcurrent <= 0 {
		t.Fatalf("MaxConcurrent got %d want positive", c.MaxConcurrent)
	}
	if c.JitterRatio < 0 || c.JitterRatio >= 1 {
		t.Fatalf("JitterRatio got %v want in [0,1)", c.JitterRatio)
	}
}

func TestConfigNormalize(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name             string
		in               Config
		wantTick         bool // TickIntervalが正かどうか
		wantConcurrentGE int  // MaxConcurrentが下限以上か
	}{
		{
			name:             "ゼロ値は既定で補完する",
			in:               Config{},
			wantTick:         true,
			wantConcurrentGE: 1,
		},
		{
			name:             "負の並行数は1へ補正する",
			in:               Config{MaxConcurrent: -3},
			wantTick:         true,
			wantConcurrentGE: 1,
		},
		{
			name:             "正の値はそのまま保つ",
			in:               Config{MaxConcurrent: 8},
			wantTick:         true,
			wantConcurrentGE: 8,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := tt.in.normalize()
			if tt.wantTick && got.TickInterval <= 0 {
				t.Fatalf("TickInterval got %v want positive", got.TickInterval)
			}
			if got.MaxConcurrent < tt.wantConcurrentGE {
				t.Fatalf("MaxConcurrent got %d want >= %d", got.MaxConcurrent, tt.wantConcurrentGE)
			}
			if got.JitterRatio < 0 || got.JitterRatio >= 1 {
				t.Fatalf("JitterRatio got %v want in [0,1)", got.JitterRatio)
			}
		})
	}
}
