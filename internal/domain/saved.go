package domain

// SavedPagesFeedID 任意URLを保存するための合成フィードのIDです。
// 購読フィードではないため、ポーリングと左ツリーとOPML出力の対象から外します。
const SavedPagesFeedID = "saved-pages"

// SavedPagesFeedTitle 合成フィードの表示名です。
const SavedPagesFeedTitle = "保存したページ"

// IsSavedPagesFeed 指定IDが合成フィードかどうかを返します。
func IsSavedPagesFeed(feedID string) bool {
	return feedID == SavedPagesFeedID
}
