package port

import (
	"context"

	"github.com/okamyuji/feedflow-go-htmx/internal/domain"
)

// SubscriptionService 購読の追加と削除と一覧と整理を担う抽象です。設計書のセクション3.1に対応します。
type SubscriptionService interface {
	// Subscribe フィードURLを購読に追加し、追加後のフィードを返します。
	Subscribe(ctx context.Context, feedURL string, categoryIDs []string) (domain.Feed, error)
	// SubscribeFromSite サイトURLからフィードを自動検出して購読に追加します。
	SubscribeFromSite(ctx context.Context, siteURL string, categoryIDs []string) (domain.Feed, error)
	// Unsubscribe 指定フィードの購読を解除し、記事も削除します。
	Unsubscribe(feedID string) error
	// ListFeeds 購読中の全フィードを返します。
	ListFeeds() ([]domain.Feed, error)
	// Reorder カテゴリの並び順を指定したID順に更新します。
	Reorder(categoryIDs []string) error
	// SetFeedCategories 指定フィードの所属カテゴリを更新します。
	SetFeedCategories(feedID string, categoryIDs []string) error
}

// ItemService 記事の既読やあとで読むやタグやブックマークやメモの操作を担う抽象です。
// 設計書のセクション3.1に対応します。
type ItemService interface {
	// ListItems 指定フィードの記事をミュート適用済みで返します。feedIDが空なら全フィード横断で返します。
	ListItems(feedID string) ([]domain.Item, error)
	// MarkRead 指定記事の既読状態を設定します。
	MarkRead(feedID, itemID string, read bool) error
	// MarkAllRead 指定フィードの全記事を既読にします。feedIDが空なら全フィードを対象にします。
	MarkAllRead(feedID string) error
	// ReadLater 指定記事のあとで読む状態を設定します。
	ReadLater(feedID, itemID string, readLater bool) error
	// SetTags 指定記事のタグを更新します。
	SetTags(feedID, itemID string, tags []string) error
	// SetBookmarks 指定記事の所属ブックマークを与えた内容で置き換えます。
	SetBookmarks(feedID, itemID string, bookmarkIDs []string) error
	// SetNote 指定記事のメモを更新します。
	SetNote(feedID, itemID, note string) error
	// AddHighlight 指定記事にハイライトを追加します。
	AddHighlight(feedID, itemID, highlight string) error
}

// BookmarkService 名称付きブックマークの一覧と作成、記事の所属操作を担う抽象です。
type BookmarkService interface {
	// List 全ブックマークを返します。
	List() ([]domain.Bookmark, error)
	// Create 指定名のブックマークを作成して返します。同名が既存ならそれを返します。
	Create(name string) (domain.Bookmark, error)
	// Toggle 指定記事のブックマーク所属を切り替えます。
	Toggle(feedID, itemID, bookmarkID string) error
	// CreateAndAdd 指定名のブックマークを用意し、指定記事を所属させて返します。
	CreateAndAdd(feedID, itemID, name string) (domain.Bookmark, error)
}

// RetentionService 保持ポリシーの適用を担う抽象です。設計書のセクション4.1に対応します。
type RetentionService interface {
	// Apply 全フィードに保持ポリシーを適用し、削除した記事の総数を返します。
	Apply() (int, error)
	// ApplyFeed 指定フィードに保持ポリシーを適用し、削除した記事数を返します。
	ApplyFeed(feedID string) (int, error)
}

// MuteService ミュートフィルタの管理と適用を担う抽象です。設計書のセクション3.1に対応します。
type MuteService interface {
	// ListFilters 全ミュートフィルタを返します。
	ListFilters() ([]domain.MuteFilter, error)
	// AddFilter ミュートフィルタを追加し、追加後のフィルタを返します。
	AddFilter(keyword string, scope domain.MuteScope, feedID string) (domain.MuteFilter, error)
	// DeleteFilter 指定IDのミュートフィルタを削除します。
	DeleteFilter(id string) error
	// Filter 与えた記事群からミュート対象を除いた記事群を返します。
	Filter(items []domain.Item) ([]domain.Item, error)
}

// OPMLService OPMLの入出力を担う抽象です。設計書のセクション3.1に対応します。
type OPMLService interface {
	// Import OPMLのバイト列を読み込み、新規に購読したフィード数を返します。
	Import(ctx context.Context, data []byte) (int, error)
	// Export 現在の購読をOPMLのバイト列として返します。
	Export() ([]byte, error)
}

// SettingsService 設定の取得と更新を担う抽象です。設計書のセクション4に対応します。
type SettingsService interface {
	// Get 現在の設定を返します。
	Get() (domain.Settings, error)
	// Update 設定を検証してから保存します。不正値の場合はエラーを返します。
	Update(settings domain.Settings) error
}

// PollService フィードの取得反映を担う抽象です。設計書のセクション4.2と8に対応します。
type PollService interface {
	// PollFeed 指定フィードを取得し、新着記事を反映して新着件数を返します。
	PollFeed(ctx context.Context, feedID string) (int, error)
	// PollAll 期限の来た全フィードを取得して反映し、処理したフィード数を返します。
	PollAll(ctx context.Context) (int, error)
}
