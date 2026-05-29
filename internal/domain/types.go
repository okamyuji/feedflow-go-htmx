// Package domain feedflowのエンティティと値オブジェクトと純粋な判定関数を提供します。
// このパッケージは外部I/Oを持たず、他のinternalパッケージにも依存しません。
package domain

import "time"

// PollInterval フィードのポーリング間隔を表す値オブジェクトです。
type PollInterval string

// ポーリング間隔の取りうる値です。設計書のセクション4.2に対応します。
const (
	PollDefault    PollInterval = "default" // 全体既定に従います
	Poll15Min      PollInterval = "15m"     // 15分間隔です
	Poll30Min      PollInterval = "30m"     // 30分間隔です
	Poll1Hour      PollInterval = "1h"      // 1時間間隔です
	Poll6Hour      PollInterval = "6h"      // 6時間間隔です
	PollManualOnly PollInterval = "manual"  // 手動更新のみです
)

// Duration ポーリング間隔をtime.Durationへ変換します。
// defaultとmanualは固有の長さを持たないためゼロ値とfalseを返します。
func (p PollInterval) Duration() (time.Duration, bool) {
	switch p {
	case Poll15Min:
		return 15 * time.Minute, true
	case Poll30Min:
		return 30 * time.Minute, true
	case Poll1Hour:
		return time.Hour, true
	case Poll6Hour:
		return 6 * time.Hour, true
	default:
		return 0, false
	}
}

// Valid ポーリング間隔が定義済みの値かどうかを返します。
func (p PollInterval) Valid() bool {
	switch p {
	case PollDefault, Poll15Min, Poll30Min, Poll1Hour, Poll6Hour, PollManualOnly:
		return true
	default:
		return false
	}
}

// ViewMode 記事リストの表示形式を表す値オブジェクトです。
type ViewMode string

// 表示形式の取りうる値です。設計書のセクション3.1に対応します。
const (
	ViewTitleOnly ViewMode = "title"    // タイトルのみ表示します
	ViewCard      ViewMode = "card"     // カード表示します
	ViewMagazine  ViewMode = "magazine" // マガジン表示します
	ViewArticle   ViewMode = "article"  // 記事ビューで表示します
)

// Valid 表示形式が定義済みの値かどうかを返します。
func (v ViewMode) Valid() bool {
	switch v {
	case ViewTitleOnly, ViewCard, ViewMagazine, ViewArticle:
		return true
	default:
		return false
	}
}

// Theme 画面テーマを表す値オブジェクトです。
type Theme string

// テーマの取りうる値です。
const (
	ThemeDark  Theme = "dark"  // ダークテーマです
	ThemeLight Theme = "light" // ライトテーマです
)

// Valid テーマが定義済みの値かどうかを返します。
func (t Theme) Valid() bool {
	return t == ThemeDark || t == ThemeLight
}

// MuteScope ミュートフィルタの対象範囲を表す値オブジェクトです。
type MuteScope string

// ミュート対象範囲の取りうる値です。設計書のセクション6に対応します。
const (
	MuteScopeGlobal MuteScope = "global" // 全フィードを対象にします
	MuteScopeFeed   MuteScope = "feed"   // 特定フィードのみを対象にします
)

// Valid 対象範囲が定義済みの値かどうかを返します。
func (s MuteScope) Valid() bool {
	return s == MuteScopeGlobal || s == MuteScopeFeed
}
