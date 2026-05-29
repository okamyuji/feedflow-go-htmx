package domain

import "strings"

// MuteFilter キーワードや送信元による記事の除外条件を表します。設計書のセクション6に対応します。
type MuteFilter struct {
	ID      string    `json:"id"`      // 一意な識別子です
	Keyword string    `json:"keyword"` // 除外判定に使うキーワードです
	Scope   MuteScope `json:"scope"`   // 対象範囲です。全体か特定フィードかを表します
	FeedID  string    `json:"feed_id"` // 対象範囲がfeedのときの対象フィードIDです
}

// Matches 指定したタイトルと所属フィードがこのフィルタの除外条件に一致するかどうかを返します。
// キーワードは大文字小文字を区別せずタイトルへの部分一致で判定します。
// 対象範囲がfeedの場合は所属フィードがフィルタの対象フィードと一致するときだけ判定します。
// キーワードが空の場合は常に一致しないものとして扱います。
func (m MuteFilter) Matches(title, feedID string) bool {
	if m.Keyword == "" {
		return false
	}
	if m.Scope == MuteScopeFeed && m.FeedID != feedID {
		return false
	}
	return strings.Contains(strings.ToLower(title), strings.ToLower(m.Keyword))
}
