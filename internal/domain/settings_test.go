package domain

import "testing"

func TestDefaultSettings(t *testing.T) {
	t.Parallel()
	s := DefaultSettings()
	if s.PollInterval != Poll30Min {
		t.Fatalf("PollInterval got %q want %q", s.PollInterval, Poll30Min)
	}
	if s.MaxItems != 200 {
		t.Fatalf("MaxItems got %d want 200", s.MaxItems)
	}
	if s.ReadRetentionDays != 30 {
		t.Fatalf("ReadRetentionDays got %d want 30", s.ReadRetentionDays)
	}
	if s.Theme != ThemeDark {
		t.Fatalf("Theme got %q want %q", s.Theme, ThemeDark)
	}
	if s.DefaultView != ViewCard {
		t.Fatalf("DefaultView got %q want %q", s.DefaultView, ViewCard)
	}
	if !s.AutoReadOnScroll {
		t.Fatalf("AutoReadOnScroll got %v want true", s.AutoReadOnScroll)
	}
	if s.FeedSortKey != FeedSortTitle {
		t.Fatalf("FeedSortKey got %q want %q", s.FeedSortKey, FeedSortTitle)
	}
	if s.FeedSortDirection != SortAsc {
		t.Fatalf("FeedSortDirection got %q want %q", s.FeedSortDirection, SortAsc)
	}
	if !s.Valid() {
		t.Fatalf("DefaultSettings() must be valid")
	}
}

func TestSettingsValidIgnoresAutoRead(t *testing.T) {
	t.Parallel()
	s := DefaultSettings()
	s.AutoReadOnScroll = false
	if !s.Valid() {
		t.Fatalf("Valid() must not depend on AutoReadOnScroll")
	}
}

func TestSettingsValid(t *testing.T) {
	t.Parallel()
	base := DefaultSettings()
	tests := []struct {
		name   string
		mutate func(s Settings) Settings
		want   bool
	}{
		{name: "既定は妥当", mutate: func(s Settings) Settings { return s }, want: true},
		{name: "件数 0 は不正", mutate: func(s Settings) Settings { s.MaxItems = 0; return s }, want: false},
		{name: "件数 負は不正", mutate: func(s Settings) Settings { s.MaxItems = -1; return s }, want: false},
		{name: "保持日数 0 は不正", mutate: func(s Settings) Settings { s.ReadRetentionDays = 0; return s }, want: false},
		{name: "ポーリング間隔 不正値", mutate: func(s Settings) Settings { s.PollInterval = "weekly"; return s }, want: false},
		{name: "テーマ 不正値", mutate: func(s Settings) Settings { s.Theme = "neon"; return s }, want: false},
		{name: "表示形式 不正値", mutate: func(s Settings) Settings { s.DefaultView = "grid"; return s }, want: false},
		{name: "フィード並び替えキー 不正値", mutate: func(s Settings) Settings { s.FeedSortKey = "updated"; return s }, want: false},
		{name: "フィード並び替え方向 不正値", mutate: func(s Settings) Settings { s.FeedSortDirection = "sideways"; return s }, want: false},
		{name: "ポーリング間隔 manual も妥当", mutate: func(s Settings) Settings { s.PollInterval = PollManualOnly; return s }, want: true},
		{name: "登録順 降順 も妥当", mutate: func(s Settings) Settings {
			s.FeedSortKey = FeedSortRegistered
			s.FeedSortDirection = SortDesc
			return s
		}, want: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			s := tt.mutate(base)
			if got := s.Valid(); got != tt.want {
				t.Fatalf("Valid() got %v want %v", got, tt.want)
			}
		})
	}
}
