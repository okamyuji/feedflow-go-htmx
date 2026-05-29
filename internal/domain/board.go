package domain

// Board テーマ別に記事を保存するボードを表します。設計書のセクション6に対応します。
type Board struct {
	ID          string `json:"id"`          // 一意な識別子です
	Name        string `json:"name"`        // ボード名です
	Description string `json:"description"` // ボードの説明です
}
