package domain

import (
	"encoding/json"
	"fmt"
	"time"
)

// Item フィードから取得した個々の記事を表します。設計書のセクション6に対応します。
type Item struct {
	ID          string    `json:"id"`           // 一意な識別子です
	FeedID      string    `json:"feed_id"`      // 所属するフィードのIDです
	GUID        string    `json:"guid"`         // フィード内での記事の一意キーです
	Title       string    `json:"title"`        // 記事のタイトルです
	Link        string    `json:"link"`         // 元記事のURLです
	Content     string    `json:"content"`      // 記事本文です
	Summary     string    `json:"summary"`      // 記事の要約です
	Author      string    `json:"author"`       // 著者名です
	PublishedAt time.Time `json:"published_at"` // 公開日時です
	FetchedAt   time.Time `json:"fetched_at"`   // 取得日時です
	Read        bool      `json:"read"`         // 既読フラグです
	ReadLater   bool      `json:"read_later"`   // あとで読むフラグです
	Bookmarked  bool      `json:"bookmarked"`   // ブックマーク(保存)済みかどうかです。ラベル所属とは独立した保存状態の真実です
	BookmarkIDs []string  `json:"bookmark_ids"` // 所属するラベル(名称コレクション)のID群です。空でも保存状態は維持されます
	Tags        []string  `json:"tags"`         // タグ群です
	Highlights  []string  `json:"highlights"`   // ハイライトした本文断片の群です
	Note        string    `json:"note"`         // 自由記述のメモです
}

// UnmarshalJSON 旧スキーマとの後方互換を保ちながら記事をデコードします。
// 新キー bookmark_ids を優先し、無ければ旧キー board_ids を BookmarkIDs に取り込みます。
// 旧 starred キーは廃止済みのため読み捨てます。
// 保存とラベルを分離する前の旧データには bookmarked キーが無く、ラベル所属の有無で保存を表していました。
// そのため bookmarked キーが無く(=RawBookmarkedがnil)かつラベルを持つ記事は保存済みとみなして移行します。
// bookmarked が明示的にfalseで書かれている新データはその値を尊重します(falseとキー不在を区別するためポインタで受けます)。
func (i *Item) UnmarshalJSON(data []byte) error {
	type alias Item
	aux := struct {
		*alias
		LegacyBoardIDs []string `json:"board_ids"`
		RawBookmarked  *bool    `json:"bookmarked"`
	}{alias: (*alias)(i)}
	if err := json.Unmarshal(data, &aux); err != nil {
		return fmt.Errorf("failed to unmarshal item: %w", err)
	}
	if len(i.BookmarkIDs) == 0 && len(aux.LegacyBoardIDs) > 0 {
		i.BookmarkIDs = aux.LegacyBoardIDs
	}
	if aux.RawBookmarked == nil && len(i.BookmarkIDs) > 0 {
		i.Bookmarked = true
	}
	return nil
}

// HasUserAction 所有者が何らかのアクションを記録した記事かどうかを返します。
// ブックマーク(保存)、あとで読む、タグ付け、メモ、ハイライトのいずれかを持つと真になります。
// 既読は閲覧の結果にすぎないためアクションには含めません。
// 保存状態は Bookmarked が真実です。ラベル所属(BookmarkIDs)があれば不変条件で必ず Bookmarked も真になります。
func (i Item) HasUserAction() bool {
	return i.Bookmarked ||
		i.ReadLater ||
		len(i.Tags) > 0 ||
		len(i.Highlights) > 0 ||
		i.Note != ""
}

// ShouldRetain 保持ポリシーに照らして記事を残すべきかどうかを返します。
// nowは現在時刻、rankIndexは同一フィード内の新しい順での0始まりの順位、
// maxItemsはフィードごとの保持件数N、retainDaysは既読の自動削除日数Mです。
// アクション済みの記事はNとMに関わらず常に保持します。
// それ以外は、件数上限を超えた記事と、既読かつM日を経過した記事を削除対象とします。
func (i Item) ShouldRetain(now time.Time, rankIndex, maxItems, retainDays int) bool {
	if i.HasUserAction() {
		return true
	}
	if rankIndex >= maxItems {
		return false
	}
	if i.Read {
		cutoff := now.AddDate(0, 0, -retainDays)
		if i.FetchedAt.Before(cutoff) {
			return false
		}
	}
	return true
}
