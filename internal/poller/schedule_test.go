package poller

import (
	"testing"
	"time"

	"github.com/okamyuji/feedflow-go-htmx/internal/domain"
)

func TestEffectiveInterval(t *testing.T) {
	t.Parallel()
	settings := domain.Settings{PollInterval: domain.Poll1Hour}
	tests := []struct {
		name string
		feed domain.Feed
		want time.Duration
	}{
		{name: "上書きなしは全体設定に従う", feed: domain.Feed{PollInterval: domain.PollDefault}, want: time.Hour},
		{name: "空も全体設定に従う", feed: domain.Feed{PollInterval: ""}, want: time.Hour},
		{name: "15分上書き", feed: domain.Feed{PollInterval: domain.Poll15Min}, want: 15 * time.Minute},
		{name: "6時間上書き", feed: domain.Feed{PollInterval: domain.Poll6Hour}, want: 6 * time.Hour},
		{name: "手動のみは対象外でゼロ", feed: domain.Feed{PollInterval: domain.PollManualOnly}, want: 0},
		{name: "不正値はゼロ", feed: domain.Feed{PollInterval: "weekly"}, want: 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := effectiveInterval(tt.feed, settings); got != tt.want {
				t.Fatalf("effectiveInterval() got %v want %v", got, tt.want)
			}
		})
	}
}

func TestDueForPoll(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 5, 29, 12, 0, 0, 0, time.UTC)
	settings := domain.Settings{PollInterval: domain.Poll30Min}
	noJitter := func(time.Duration) time.Duration { return 0 }
	tests := []struct {
		name   string
		feed   domain.Feed
		jitter jitterFunc
		want   bool
	}{
		{
			name:   "未取得は常に対象",
			feed:   domain.Feed{PollInterval: domain.Poll30Min},
			jitter: noJitter,
			want:   true,
		},
		{
			name:   "間隔未経過は対象外",
			feed:   domain.Feed{PollInterval: domain.Poll30Min, LastFetchedAt: now.Add(-10 * time.Minute)},
			jitter: noJitter,
			want:   false,
		},
		{
			name:   "間隔経過は対象",
			feed:   domain.Feed{PollInterval: domain.Poll30Min, LastFetchedAt: now.Add(-31 * time.Minute)},
			jitter: noJitter,
			want:   true,
		},
		{
			name:   "手動のみは経過しても対象外",
			feed:   domain.Feed{PollInterval: domain.PollManualOnly, LastFetchedAt: now.Add(-10 * time.Hour)},
			jitter: noJitter,
			want:   false,
		},
		{
			name:   "ジッタで前倒し取得を許す",
			feed:   domain.Feed{PollInterval: domain.Poll30Min, LastFetchedAt: now.Add(-26 * time.Minute)},
			jitter: func(time.Duration) time.Duration { return 5 * time.Minute },
			want:   true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := dueForPollWithJitter(tt.feed, settings, now, tt.jitter)
			if got != tt.want {
				t.Fatalf("dueForPollWithJitter() got %v want %v", got, tt.want)
			}
		})
	}
}

func TestRatioJitterBounds(t *testing.T) {
	t.Parallel()
	j := ratioJitter(0.1)
	interval := 30 * time.Minute
	for i := 0; i < 1000; i++ {
		got := j(interval)
		if got < 0 || got > interval/10 {
			t.Fatalf("ratioJitter out of bounds: got %v want in [0, %v]", got, interval/10)
		}
	}
}

func TestRatioJitterZeroRatio(t *testing.T) {
	t.Parallel()
	j := ratioJitter(0)
	if got := j(30 * time.Minute); got != 0 {
		t.Fatalf("ratioJitter(0) got %v want 0", got)
	}
}
