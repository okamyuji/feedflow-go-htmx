package domain

// Category フィードを分類するカテゴリを表します。設計書のセクション6に対応します。
type Category struct {
	ID    string `json:"id"`    // 一意な識別子です
	Name  string `json:"name"`  // カテゴリ名です
	Order int    `json:"order"` // 並び順です。小さいほど先頭に並びます
}
