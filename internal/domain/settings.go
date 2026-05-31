package domain

// 設定の既定値です。設計書のセクション15に対応します。
const (
	DefaultMaxItems          = 200 // フィードごとの保持件数Nの既定値です
	DefaultReadRetentionDays = 30  // 既読の自動削除日数Mの既定値です
)

// Settings アプリ全体の設定を表します。設計書のセクション6に対応します。
type Settings struct {
	PollInterval      PollInterval  `json:"poll_interval"`       // 全体既定のポーリング間隔です
	MaxItems          int           `json:"max_items"`           // フィードごとの保持件数Nです
	ReadRetentionDays int           `json:"read_retention_days"` // 既読の自動削除日数Mです
	Theme             Theme         `json:"theme"`               // 既定のテーマです
	DefaultView       ViewMode      `json:"default_view"`        // 既定の表示形式です
	AutoReadOnScroll  bool          `json:"auto_read_on_scroll"` // オーバーレイを末尾までスクロールしたとき自動で既読にするかどうかです
	FeedSortKey       FeedSortKey   `json:"feed_sort_key"`       // 左ペインのフィード並び替えキーです
	FeedSortDirection SortDirection `json:"feed_sort_direction"` // 左ペインのフィード並び替え方向です
}

// DefaultSettings 設計書の既定値で初期化した設定を返します。
func DefaultSettings() Settings {
	return Settings{
		PollInterval:      Poll30Min,
		MaxItems:          DefaultMaxItems,
		ReadRetentionDays: DefaultReadRetentionDays,
		Theme:             ThemeDark,
		DefaultView:       ViewCard,
		AutoReadOnScroll:  true,
		FeedSortKey:       FeedSortTitle,
		FeedSortDirection: SortAsc,
	}
}

// Valid 設定値がすべて妥当かどうかを返します。
// 保持件数Nと保持日数Mは正の数を要求します。
// ポーリング間隔はmanualを含む定義済みの値、テーマと表示形式も定義済みの値を要求します。
func (s Settings) Valid() bool {
	if s.MaxItems <= 0 || s.ReadRetentionDays <= 0 {
		return false
	}
	return s.PollInterval.Valid() && s.Theme.Valid() && s.DefaultView.Valid() &&
		s.FeedSortKey.Valid() && s.FeedSortDirection.Valid()
}
