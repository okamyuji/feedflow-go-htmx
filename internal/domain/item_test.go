package domain

import (
	"testing"
	"time"
)

func TestItemHasUserAction(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		item Item
		want bool
	}{
		{name: "アクションなし", item: Item{}, want: false},
		{name: "スターのみ", item: Item{Starred: true}, want: true},
		{name: "あとで読むのみ", item: Item{ReadLater: true}, want: true},
		{name: "ボード保存のみ", item: Item{BoardIDs: []string{"b1"}}, want: true},
		{name: "タグのみ", item: Item{Tags: []string{"go"}}, want: true},
		{name: "メモのみ", item: Item{Note: "あとで確認します"}, want: true},
		{name: "ハイライトのみ", item: Item{Highlights: []string{"重要な一文"}}, want: true},
		{name: "既読だけではアクション扱いしない", item: Item{Read: true}, want: false},
		{name: "空ボードと空タグはアクションなし", item: Item{BoardIDs: []string{}, Tags: []string{}}, want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := tt.item.HasUserAction(); got != tt.want {
				t.Fatalf("HasUserAction() got %v want %v", got, tt.want)
			}
		})
	}
}

func TestItemShouldRetain(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 5, 29, 12, 0, 0, 0, time.UTC)
	retainDays := 30
	maxItems := 200
	old := now.AddDate(0, 0, -40) // 40日前でM=30を超過します
	recent := now.AddDate(0, 0, -10)
	tests := []struct {
		name      string
		item      Item
		rankIndex int // 新しい順で何番目か。0始まりです
		want      bool
	}{
		{
			name:      "未読は件数内かつ期限内なら保持する",
			item:      Item{Read: false, FetchedAt: recent},
			rankIndex: 10,
			want:      true,
		},
		{
			name:      "未読でも件数上限を超えたら削除対象",
			item:      Item{Read: false, FetchedAt: recent},
			rankIndex: maxItems,
			want:      false,
		},
		{
			name:      "既読で M 日経過は削除対象",
			item:      Item{Read: true, FetchedAt: old},
			rankIndex: 10,
			want:      false,
		},
		{
			name:      "既読でも M 日以内は保持する",
			item:      Item{Read: true, FetchedAt: recent},
			rankIndex: 10,
			want:      true,
		},
		{
			name:      "アクション済みは件数超過でも永久保持する",
			item:      Item{Starred: true, Read: true, FetchedAt: old},
			rankIndex: maxItems + 100,
			want:      true,
		},
		{
			name:      "アクション済みは M 日経過でも永久保持する",
			item:      Item{Note: "重要", Read: true, FetchedAt: old},
			rankIndex: 5,
			want:      true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := tt.item.ShouldRetain(now, tt.rankIndex, maxItems, retainDays)
			if got != tt.want {
				t.Fatalf("ShouldRetain() got %v want %v", got, tt.want)
			}
		})
	}
}
