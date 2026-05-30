package domain

// Bookmark 階層なしの名称付きブックマークを表します。
// 記事は Item.BookmarkIDs を通じて複数のブックマークへ所属できます。
type Bookmark struct {
	ID   string `json:"id"`   // 一意な識別子です
	Name string `json:"name"` // ブックマーク名です
}
