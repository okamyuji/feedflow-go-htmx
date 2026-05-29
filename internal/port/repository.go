package port

import "github.com/okamyuji/feedflow-go-htmx/internal/domain"

// Repository 全エンティティの取得と保存を担う永続化境界です。
// 実装はメモリ常駐とアトミックJSON書き込みで満たします。設計書のセクション5.2と7に対応します。
type Repository interface {
	// Feeds 登録済みの全フィードを返します。
	Feeds() ([]domain.Feed, error)
	// Feed 指定IDのフィードを返します。見つからない場合はエラーを返します。
	Feed(id string) (domain.Feed, error)
	// SaveFeed フィードを新規追加または更新します。
	SaveFeed(feed domain.Feed) error
	// DeleteFeed 指定IDのフィードと、それに属する全記事を削除します。
	DeleteFeed(id string) error

	// Categories 全カテゴリを返します。
	Categories() ([]domain.Category, error)
	// SaveCategory カテゴリを新規追加または更新します。
	SaveCategory(category domain.Category) error
	// DeleteCategory 指定IDのカテゴリを削除します。
	DeleteCategory(id string) error

	// Items 指定フィードの全記事を返します。
	Items(feedID string) ([]domain.Item, error)
	// SaveItems 指定フィードの記事群をまとめて保存します。既存の記事群を置き換えます。
	SaveItems(feedID string, items []domain.Item) error

	// Boards 全ボードを返します。
	Boards() ([]domain.Board, error)
	// SaveBoard ボードを新規追加または更新します。
	SaveBoard(board domain.Board) error
	// DeleteBoard 指定IDのボードを削除します。
	DeleteBoard(id string) error

	// Filters 全ミュートフィルタを返します。
	Filters() ([]domain.MuteFilter, error)
	// SaveFilter ミュートフィルタを新規追加または更新します。
	SaveFilter(filter domain.MuteFilter) error
	// DeleteFilter 指定IDのミュートフィルタを削除します。
	DeleteFilter(id string) error

	// Settings 現在の設定を返します。
	Settings() (domain.Settings, error)
	// SaveSettings 設定を保存します。
	SaveSettings(settings domain.Settings) error

	// User 所有者ユーザーを返します。未登録の場合はゼロ値のUserを返します。
	User() (domain.User, error)
	// SaveUser 所有者ユーザーを保存します。
	SaveUser(user domain.User) error
}
