package domain_test

import (
	"testing"

	"github.com/okamyuji/feedflow-go-htmx/internal/domain"
)

func TestSavedPagesFeedConstants(t *testing.T) {
	t.Parallel()
	if domain.SavedPagesFeedID != "saved-pages" {
		t.Errorf("SavedPagesFeedID = %q, want %q", domain.SavedPagesFeedID, "saved-pages")
	}
	if domain.SavedPagesFeedTitle != "保存したページ" {
		t.Errorf("SavedPagesFeedTitle = %q, want %q", domain.SavedPagesFeedTitle, "保存したページ")
	}
}

func TestIsSavedPagesFeed(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		feedID string
		want   bool
	}{
		{name: "合成フィードのID", feedID: "saved-pages", want: true},
		{name: "通常のフィードID", feedID: "a1b2c3", want: false},
		{name: "空文字", feedID: "", want: false},
		{name: "大文字違い", feedID: "SAVED-PAGES", want: false},
		{name: "前方一致するだけの別ID", feedID: "saved-pages-2", want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := domain.IsSavedPagesFeed(tt.feedID); got != tt.want {
				t.Errorf("IsSavedPagesFeed(%q) = %v, want %v", tt.feedID, got, tt.want)
			}
		})
	}
}
