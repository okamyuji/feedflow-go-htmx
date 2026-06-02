package domain

import (
	"slices"
	"time"
)

// ErrorThreshold この回数以上連続でエラーが続いたフィードをエラー状態とみなします。
const ErrorThreshold = 5

// Feed フィードの購読単位を表します。設計書のセクション6に対応します。
type Feed struct {
	ID                string       `json:"id"`                 // 一意な識別子です
	FeedURL           string       `json:"feed_url"`           // フィード本体のURLです
	SiteURL           string       `json:"site_url"`           // サイトのトップURLです
	Title             string       `json:"title"`              // フィードのタイトルです
	CategoryIDs       []string     `json:"category_ids"`       // 所属するカテゴリのID群です
	PollInterval      PollInterval `json:"poll_interval"`      // ポーリング間隔の上書き値です
	ETag              string       `json:"etag"`               // 前回取得時のETagです
	LastModified      string       `json:"last_modified"`      // 前回取得時のLast-Modifiedです
	LastFetchedAt     time.Time    `json:"last_fetched_at"`    // 最終取得時刻です
	ConsecutiveErrors int          `json:"consecutive_errors"` // 連続して失敗した回数です
	Favorite          bool         `json:"favorite"`           // お気に入りフラグです
}

// HasError 連続エラー数がしきい値以上でエラー状態かどうかを返します。
func (f Feed) HasError() bool {
	return f.ConsecutiveErrors >= ErrorThreshold
}

// InCategory 指定したカテゴリIDに所属するかどうかを返します。
func (f Feed) InCategory(categoryID string) bool {
	return slices.Contains(f.CategoryIDs, categoryID)
}
