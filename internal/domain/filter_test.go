package domain

import "testing"

func TestMuteFilterMatches(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		filter MuteFilter
		title  string
		feedID string
		want   bool
	}{
		{
			name:   "全体ミュートでタイトルに含む",
			filter: MuteFilter{Keyword: "広告", Scope: MuteScopeGlobal},
			title:  "本日の広告まとめ",
			feedID: "f1",
			want:   true,
		},
		{
			name:   "全体ミュートで含まない",
			filter: MuteFilter{Keyword: "広告", Scope: MuteScopeGlobal},
			title:  "技術記事のまとめ",
			feedID: "f1",
			want:   false,
		},
		{
			name:   "大文字小文字を区別しない",
			filter: MuteFilter{Keyword: "Sale", Scope: MuteScopeGlobal},
			title:  "BIG SALE TODAY",
			feedID: "f1",
			want:   true,
		},
		{
			name:   "フィード限定で対象フィードに一致",
			filter: MuteFilter{Keyword: "PR", Scope: MuteScopeFeed, FeedID: "f1"},
			title:  "これはPRです",
			feedID: "f1",
			want:   true,
		},
		{
			name:   "フィード限定で対象外フィードは一致しない",
			filter: MuteFilter{Keyword: "PR", Scope: MuteScopeFeed, FeedID: "f1"},
			title:  "これはPRです",
			feedID: "f2",
			want:   false,
		},
		{
			name:   "空キーワードは一致しない",
			filter: MuteFilter{Keyword: "", Scope: MuteScopeGlobal},
			title:  "任意のタイトル",
			feedID: "f1",
			want:   false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := tt.filter.Matches(tt.title, tt.feedID)
			if got != tt.want {
				t.Fatalf("Matches(%q, %q) got %v want %v", tt.title, tt.feedID, got, tt.want)
			}
		})
	}
}
