package domain

import "testing"

func TestFeedHasError(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name           string
		consecutiveErr int
		want           bool
	}{
		{name: "エラーなし", consecutiveErr: 0, want: false},
		{name: "しきい値未満", consecutiveErr: ErrorThreshold - 1, want: false},
		{name: "しきい値ちょうど", consecutiveErr: ErrorThreshold, want: true},
		{name: "しきい値超過", consecutiveErr: ErrorThreshold + 5, want: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			f := Feed{ConsecutiveErrors: tt.consecutiveErr}
			if got := f.HasError(); got != tt.want {
				t.Fatalf("HasError() got %v want %v", got, tt.want)
			}
		})
	}
}

func TestFeedInCategory(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name       string
		categories []string
		query      string
		want       bool
	}{
		{name: "所属あり", categories: []string{"c1", "c2"}, query: "c2", want: true},
		{name: "所属なし", categories: []string{"c1"}, query: "c9", want: false},
		{name: "空のカテゴリ", categories: nil, query: "c1", want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			f := Feed{CategoryIDs: tt.categories}
			if got := f.InCategory(tt.query); got != tt.want {
				t.Fatalf("InCategory(%q) got %v want %v", tt.query, got, tt.want)
			}
		})
	}
}
