# Phase1 ドメインとポート 実装計画

> For agentic workers: REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

Goal: 設計書のセクション5と6に基づき、internal/domainのエンティティ型と値オブジェクトと純粋関数、internal/portの全インターフェースを定義し、ドメイン純粋関数のテーブル駆動テストを通します。

Architecture: クリーンアーキテクチャの最内周を作ります。internal/domainはI/Oを持たない純粋なエンティティと値オブジェクトと判定関数だけを持ちます。internal/portは上位層が依存するインターフェース境界をすべて宣言し、Repository、Fetcher、FeedParser、Clock、IDGen、および各サービスのインターフェースを置きます。port側はdomain型に依存しますが、domain側はportにもどの実装にも依存しません。後続のstoreとfeedとserviceとpollerとhandlerはこのportのインターフェースにコンストラクタ注入で依存します。

Tech Stack: Go 1.25(標準ライブラリのみ)。domainとportはこの段階で外部依存を一切持ちません。テストは標準のtestingでテーブル駆動とし、-raceで通る前提で書きます。

前提: 作業ディレクトリは`/Users/yujiokamoto/devs/golang/feedflow-go-htmx`です。Phase0が完了し`bash scripts/quality-gate.sh`が緑であることを確認してから始めます。module pathは`github.com/okamyuji/feedflow-go-htmx`です。

---

## Task 1: ドメインの共通型と値オブジェクト

Files:
- Create: `/Users/yujiokamoto/devs/golang/feedflow-go-htmx/internal/domain/types.go`

ポーリング間隔と表示形式とテーマとミュート対象範囲を、文字列の生値ではなく型付きの値オブジェクトとして定義します。これによりサービス層とハンドラ層で不正値の混入を防ぎます。

- [ ] Step 1: 共通型を作成する

Create `internal/domain/types.go`:
```go
// Package domain feedflowのエンティティと値オブジェクトと純粋な判定関数を提供します。
// このパッケージは外部I/Oを持たず、他のinternalパッケージにも依存しません。
package domain

import "time"

// PollInterval フィードのポーリング間隔を表す値オブジェクトです。
type PollInterval string

// ポーリング間隔の取りうる値です。設計書のセクション4.2に対応します。
const (
	PollDefault   PollInterval = "default"  // 全体既定に従います
	Poll15Min     PollInterval = "15m"      // 15分間隔です
	Poll30Min     PollInterval = "30m"      // 30分間隔です
	Poll1Hour     PollInterval = "1h"       // 1時間間隔です
	Poll6Hour     PollInterval = "6h"       // 6時間間隔です
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
```

- [ ] Step 2: コンパイルが通ることを確認する

Run:
```bash
cd /Users/yujiokamoto/devs/golang/feedflow-go-htmx && go build ./internal/domain/
```
Expected: エラーなく完了します。

- [ ] Step 3: gofmtを適用する

Run:
```bash
cd /Users/yujiokamoto/devs/golang/feedflow-go-htmx && gofmt -w internal/domain/types.go
```
Expected: エラーなく完了します。

- [ ] Step 4: コミットする

```bash
cd /Users/yujiokamoto/devs/golang/feedflow-go-htmx && git add internal/domain/types.go && git commit -m "feat: ドメインの共通型と値オブジェクトを追加する"
```

---

## Task 2: Feed エンティティと Category エンティティ

Files:
- Create: `/Users/yujiokamoto/devs/golang/feedflow-go-htmx/internal/domain/feed.go`
- Create: `/Users/yujiokamoto/devs/golang/feedflow-go-htmx/internal/domain/category.go`

設計書セクション6のFeedとCategoryのフィールドをそのまま型に落とします。Feedのエラー状態判定として、連続エラー数がしきい値を超えたかどうかの純粋関数を持たせます。

- [ ] Step 1: 失敗するテストを書く

Create `internal/domain/feed_test.go`:
```go
package domain

import "testing"

func TestFeedHasError(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name           string
		consecutiveErr int
		want           bool
	}{
		{name: "エラーなし", consecutiveErr: 0, want: false},
		{name: "しきい値未満", consecutiveErr: ErrorThreshold - 1, want: false},
		{name: "しきい値ちょうど", consecutiveErr: ErrorThreshold, want: true},
		{name: "しきい値超過", consecutiveErr: ErrorThreshold + 5, want: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			f := Feed{ConsecutiveErrors: tt.consecutiveErr}
			if got := f.HasError(); got != tt.want {
				t.Fatalf("HasError() got %v want %v", got, tt.want)
			}
		})
	}
}

func TestFeedInCategory(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name       string
		categories []string
		query      string
		want       bool
	}{
		{name: "所属あり", categories: []string{"c1", "c2"}, query: "c2", want: true},
		{name: "所属なし", categories: []string{"c1"}, query: "c9", want: false},
		{name: "空のカテゴリ", categories: nil, query: "c1", want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			f := Feed{CategoryIDs: tt.categories}
			if got := f.InCategory(tt.query); got != tt.want {
				t.Fatalf("InCategory(%q) got %v want %v", tt.query, got, tt.want)
			}
		})
	}
}
```

- [ ] Step 2: テストが失敗することを確認する

Run:
```bash
cd /Users/yujiokamoto/devs/golang/feedflow-go-htmx && go test ./internal/domain/ -run 'TestFeed' -v
```
Expected: コンパイルエラーで失敗します。`undefined: Feed` や `undefined: ErrorThreshold` と表示されます。

- [ ] Step 3: Feed の最小実装を書く

Create `internal/domain/feed.go`:
```go
package domain

import "time"

// ErrorThreshold この回数以上連続でエラーが続いたフィードをエラー状態とみなします。
const ErrorThreshold = 5

// Feed フィードの購読単位を表します。設計書のセクション6に対応します。
type Feed struct {
	ID                string       `json:"id"`                // 一意な識別子です
	FeedURL           string       `json:"feed_url"`          // フィード本体のURLです
	SiteURL           string       `json:"site_url"`          // サイトのトップURLです
	Title             string       `json:"title"`             // フィードのタイトルです
	CategoryIDs       []string     `json:"category_ids"`      // 所属するカテゴリのID群です
	PollInterval      PollInterval `json:"poll_interval"`     // ポーリング間隔の上書き値です
	ETag              string       `json:"etag"`              // 前回取得時のETagです
	LastModified      string       `json:"last_modified"`     // 前回取得時のLast-Modifiedです
	LastFetchedAt     time.Time    `json:"last_fetched_at"`   // 最終取得時刻です
	ConsecutiveErrors int          `json:"consecutive_errors"` // 連続して失敗した回数です
	Favorite          bool         `json:"favorite"`          // お気に入りフラグです
}

// HasError 連続エラー数がしきい値以上でエラー状態かどうかを返します。
func (f Feed) HasError() bool {
	return f.ConsecutiveErrors >= ErrorThreshold
}

// InCategory 指定したカテゴリIDに所属するかどうかを返します。
func (f Feed) InCategory(categoryID string) bool {
	for _, id := range f.CategoryIDs {
		if id == categoryID {
			return true
		}
	}
	return false
}
```

- [ ] Step 4: Category の実装を書く

Create `internal/domain/category.go`:
```go
package domain

// Category フィードを分類するカテゴリを表します。設計書のセクション6に対応します。
type Category struct {
	ID    string `json:"id"`    // 一意な識別子です
	Name  string `json:"name"`  // カテゴリ名です
	Order int    `json:"order"` // 並び順です。小さいほど先頭に並びます
}
```

- [ ] Step 5: テストが通ることを確認する

Run:
```bash
cd /Users/yujiokamoto/devs/golang/feedflow-go-htmx && go test ./internal/domain/ -run 'TestFeed' -v
```
Expected: `TestFeedHasError` と `TestFeedInCategory` がいずれも PASS します。

- [ ] Step 6: gofmtを適用してコミットする

```bash
cd /Users/yujiokamoto/devs/golang/feedflow-go-htmx && gofmt -w internal/domain/feed.go internal/domain/feed_test.go internal/domain/category.go && git add internal/domain/feed.go internal/domain/feed_test.go internal/domain/category.go && git commit -m "feat: Feed と Category エンティティを追加する"
```

---

## Task 3: Item エンティティと保持除外判定

Files:
- Create: `/Users/yujiokamoto/devs/golang/feedflow-go-htmx/internal/domain/item.go`

設計書セクション4.1と6の保持ポリシーが核心です。所有者がアクションした記事(スター、あとで読む、ボード保存、タグ付け、メモやハイライトのいずれか)は、N件とM日の制限に関わらず永久保持します。この判定を純粋関数`HasUserAction`として持たせ、保持除外判定`ShouldRetain`を実装します。

- [ ] Step 1: 失敗するテストを書く

Create `internal/domain/item_test.go`:
```go
package domain

import (
	"testing"
	"time"
)

func TestItemHasUserAction(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		item Item
		want bool
	}{
		{name: "アクションなし", item: Item{}, want: false},
		{name: "スターのみ", item: Item{Starred: true}, want: true},
		{name: "あとで読むのみ", item: Item{ReadLater: true}, want: true},
		{name: "ボード保存のみ", item: Item{BoardIDs: []string{"b1"}}, want: true},
		{name: "タグのみ", item: Item{Tags: []string{"go"}}, want: true},
		{name: "メモのみ", item: Item{Note: "あとで確認します"}, want: true},
		{name: "ハイライトのみ", item: Item{Highlights: []string{"重要な一文"}}, want: true},
		{name: "既読だけではアクション扱いしない", item: Item{Read: true}, want: false},
		{name: "空ボードと空タグはアクションなし", item: Item{BoardIDs: []string{}, Tags: []string{}}, want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := tt.item.HasUserAction(); got != tt.want {
				t.Fatalf("HasUserAction() got %v want %v", got, tt.want)
			}
		})
	}
}

func TestItemShouldRetain(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 5, 29, 12, 0, 0, 0, time.UTC)
	retainDays := 30
	maxItems := 200
	old := now.AddDate(0, 0, -40) // 40日前でM=30を超過します
	recent := now.AddDate(0, 0, -10)
	tests := []struct {
		name      string
		item      Item
		rankIndex int // 新しい順で何番目か。0始まりです
		want      bool
	}{
		{
			name:      "未読は件数内かつ期限内なら保持する",
			item:      Item{Read: false, FetchedAt: recent},
			rankIndex: 10,
			want:      true,
		},
		{
			name:      "未読でも件数上限を超えたら削除対象",
			item:      Item{Read: false, FetchedAt: recent},
			rankIndex: maxItems,
			want:      false,
		},
		{
			name:      "既読で M 日経過は削除対象",
			item:      Item{Read: true, FetchedAt: old},
			rankIndex: 10,
			want:      false,
		},
		{
			name:      "既読でも M 日以内は保持する",
			item:      Item{Read: true, FetchedAt: recent},
			rankIndex: 10,
			want:      true,
		},
		{
			name:      "アクション済みは件数超過でも永久保持する",
			item:      Item{Starred: true, Read: true, FetchedAt: old},
			rankIndex: maxItems + 100,
			want:      true,
		},
		{
			name:      "アクション済みは M 日経過でも永久保持する",
			item:      Item{Note: "重要", Read: true, FetchedAt: old},
			rankIndex: 5,
			want:      true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := tt.item.ShouldRetain(now, tt.rankIndex, maxItems, retainDays)
			if got != tt.want {
				t.Fatalf("ShouldRetain() got %v want %v", got, tt.want)
			}
		})
	}
}
```

- [ ] Step 2: テストが失敗することを確認する

Run:
```bash
cd /Users/yujiokamoto/devs/golang/feedflow-go-htmx && go test ./internal/domain/ -run 'TestItem' -v
```
Expected: コンパイルエラーで失敗します。`undefined: Item` と表示されます。

- [ ] Step 3: Item の最小実装を書く

Create `internal/domain/item.go`:
```go
package domain

import "time"

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
	Starred     bool      `json:"starred"`      // スターフラグです
	ReadLater   bool      `json:"read_later"`   // あとで読むフラグです
	BoardIDs    []string  `json:"board_ids"`    // 保存先ボードのID群です
	Tags        []string  `json:"tags"`         // タグ群です
	Highlights  []string  `json:"highlights"`   // ハイライトした本文断片の群です
	Note        string    `json:"note"`         // 自由記述のメモです
}

// HasUserAction 所有者が何らかのアクションを記録した記事かどうかを返します。
// スター、あとで読む、ボード保存、タグ付け、メモ、ハイライトのいずれかを持つと真になります。
// 既読は閲覧の結果にすぎないためアクションには含めません。
func (i Item) HasUserAction() bool {
	return i.Starred ||
		i.ReadLater ||
		len(i.BoardIDs) > 0 ||
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
```

- [ ] Step 4: テストが通ることを確認する

Run:
```bash
cd /Users/yujiokamoto/devs/golang/feedflow-go-htmx && go test ./internal/domain/ -run 'TestItem' -v
```
Expected: `TestItemHasUserAction` と `TestItemShouldRetain` がいずれも PASS します。

- [ ] Step 5: gofmtを適用してコミットする

```bash
cd /Users/yujiokamoto/devs/golang/feedflow-go-htmx && gofmt -w internal/domain/item.go internal/domain/item_test.go && git add internal/domain/item.go internal/domain/item_test.go && git commit -m "feat: Item エンティティと保持除外判定を追加する"
```

---

## Task 4: Board エンティティと MuteFilter と文字列一致判定

Files:
- Create: `/Users/yujiokamoto/devs/golang/feedflow-go-htmx/internal/domain/board.go`
- Create: `/Users/yujiokamoto/devs/golang/feedflow-go-htmx/internal/domain/filter.go`

設計書セクション3.1のミュートフィルタ(キーワードや送信元による文字列一致での除外)を、MuteFilterの純粋関数`Matches`として実装します。大文字小文字を区別せず、対象範囲がfeedの場合は対象FeedIDが一致したときだけ判定します。

- [ ] Step 1: 失敗するテストを書く

Create `internal/domain/filter_test.go`:
```go
package domain

import "testing"

func TestMuteFilterMatches(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		filter MuteFilter
		title  string
		feedID string
		want   bool
	}{
		{
			name:   "全体ミュートでタイトルに含む",
			filter: MuteFilter{Keyword: "広告", Scope: MuteScopeGlobal},
			title:  "本日の広告まとめ",
			feedID: "f1",
			want:   true,
		},
		{
			name:   "全体ミュートで含まない",
			filter: MuteFilter{Keyword: "広告", Scope: MuteScopeGlobal},
			title:  "技術記事のまとめ",
			feedID: "f1",
			want:   false,
		},
		{
			name:   "大文字小文字を区別しない",
			filter: MuteFilter{Keyword: "Sale", Scope: MuteScopeGlobal},
			title:  "BIG SALE TODAY",
			feedID: "f1",
			want:   true,
		},
		{
			name:   "フィード限定で対象フィードに一致",
			filter: MuteFilter{Keyword: "PR", Scope: MuteScopeFeed, FeedID: "f1"},
			title:  "これはPRです",
			feedID: "f1",
			want:   true,
		},
		{
			name:   "フィード限定で対象外フィードは一致しない",
			filter: MuteFilter{Keyword: "PR", Scope: MuteScopeFeed, FeedID: "f1"},
			title:  "これはPRです",
			feedID: "f2",
			want:   false,
		},
		{
			name:   "空キーワードは一致しない",
			filter: MuteFilter{Keyword: "", Scope: MuteScopeGlobal},
			title:  "任意のタイトル",
			feedID: "f1",
			want:   false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := tt.filter.Matches(tt.title, tt.feedID)
			if got != tt.want {
				t.Fatalf("Matches(%q, %q) got %v want %v", tt.title, tt.feedID, got, tt.want)
			}
		})
	}
}
```

- [ ] Step 2: テストが失敗することを確認する

Run:
```bash
cd /Users/yujiokamoto/devs/golang/feedflow-go-htmx && go test ./internal/domain/ -run 'TestMuteFilter' -v
```
Expected: コンパイルエラーで失敗します。`undefined: MuteFilter` と表示されます。

- [ ] Step 3: Board の実装を書く

Create `internal/domain/board.go`:
```go
package domain

// Board テーマ別に記事を保存するボードを表します。設計書のセクション6に対応します。
type Board struct {
	ID          string `json:"id"`          // 一意な識別子です
	Name        string `json:"name"`        // ボード名です
	Description string `json:"description"` // ボードの説明です
}
```

- [ ] Step 4: MuteFilter の実装を書く

Create `internal/domain/filter.go`:
```go
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
```

- [ ] Step 5: テストが通ることを確認する

Run:
```bash
cd /Users/yujiokamoto/devs/golang/feedflow-go-htmx && go test ./internal/domain/ -run 'TestMuteFilter' -v
```
Expected: `TestMuteFilterMatches` の全サブテストが PASS します。

- [ ] Step 6: gofmtを適用してコミットする

```bash
cd /Users/yujiokamoto/devs/golang/feedflow-go-htmx && gofmt -w internal/domain/board.go internal/domain/filter.go internal/domain/filter_test.go && git add internal/domain/board.go internal/domain/filter.go internal/domain/filter_test.go && git commit -m "feat: Board と MuteFilter と文字列一致判定を追加する"
```

---

## Task 5: Settings エンティティと既定値

Files:
- Create: `/Users/yujiokamoto/devs/golang/feedflow-go-htmx/internal/domain/settings.go`

設計書セクション6と15のSettingsを型に落とします。既定値を返す`DefaultSettings`と、設定値が妥当かどうかを返す`Valid`を持たせます。Nとmは正の数を要求します。

- [ ] Step 1: 失敗するテストを書く

Create `internal/domain/settings_test.go`:
```go
package domain

import "testing"

func TestDefaultSettings(t *testing.T) {
	t.Parallel()
	s := DefaultSettings()
	if s.PollInterval != Poll30Min {
		t.Fatalf("PollInterval got %q want %q", s.PollInterval, Poll30Min)
	}
	if s.MaxItems != 200 {
		t.Fatalf("MaxItems got %d want 200", s.MaxItems)
	}
	if s.ReadRetentionDays != 30 {
		t.Fatalf("ReadRetentionDays got %d want 30", s.ReadRetentionDays)
	}
	if s.Theme != ThemeDark {
		t.Fatalf("Theme got %q want %q", s.Theme, ThemeDark)
	}
	if s.DefaultView != ViewCard {
		t.Fatalf("DefaultView got %q want %q", s.DefaultView, ViewCard)
	}
	if !s.Valid() {
		t.Fatalf("DefaultSettings() must be valid")
	}
}

func TestSettingsValid(t *testing.T) {
	t.Parallel()
	base := DefaultSettings()
	tests := []struct {
		name   string
		mutate func(s Settings) Settings
		want   bool
	}{
		{name: "既定は妥当", mutate: func(s Settings) Settings { return s }, want: true},
		{name: "件数 0 は不正", mutate: func(s Settings) Settings { s.MaxItems = 0; return s }, want: false},
		{name: "件数 負は不正", mutate: func(s Settings) Settings { s.MaxItems = -1; return s }, want: false},
		{name: "保持日数 0 は不正", mutate: func(s Settings) Settings { s.ReadRetentionDays = 0; return s }, want: false},
		{name: "ポーリング間隔 不正値", mutate: func(s Settings) Settings { s.PollInterval = "weekly"; return s }, want: false},
		{name: "テーマ 不正値", mutate: func(s Settings) Settings { s.Theme = "neon"; return s }, want: false},
		{name: "表示形式 不正値", mutate: func(s Settings) Settings { s.DefaultView = "grid"; return s }, want: false},
		{name: "ポーリング間隔 manual も妥当", mutate: func(s Settings) Settings { s.PollInterval = PollManualOnly; return s }, want: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			s := tt.mutate(base)
			if got := s.Valid(); got != tt.want {
				t.Fatalf("Valid() got %v want %v", got, tt.want)
			}
		})
	}
}
```

- [ ] Step 2: テストが失敗することを確認する

Run:
```bash
cd /Users/yujiokamoto/devs/golang/feedflow-go-htmx && go test ./internal/domain/ -run 'TestSettings|TestDefaultSettings' -v
```
Expected: コンパイルエラーで失敗します。`undefined: Settings` や `undefined: DefaultSettings` と表示されます。

- [ ] Step 3: Settings の実装を書く

Create `internal/domain/settings.go`:
```go
package domain

// 設定の既定値です。設計書のセクション15に対応します。
const (
	DefaultMaxItems          = 200 // フィードごとの保持件数Nの既定値です
	DefaultReadRetentionDays = 30  // 既読の自動削除日数Mの既定値です
)

// Settings アプリ全体の設定を表します。設計書のセクション6に対応します。
type Settings struct {
	PollInterval      PollInterval `json:"poll_interval"`       // 全体既定のポーリング間隔です
	MaxItems          int          `json:"max_items"`           // フィードごとの保持件数Nです
	ReadRetentionDays int          `json:"read_retention_days"` // 既読の自動削除日数Mです
	Theme             Theme        `json:"theme"`               // 既定のテーマです
	DefaultView       ViewMode     `json:"default_view"`        // 既定の表示形式です
}

// DefaultSettings 設計書の既定値で初期化した設定を返します。
func DefaultSettings() Settings {
	return Settings{
		PollInterval:      Poll30Min,
		MaxItems:          DefaultMaxItems,
		ReadRetentionDays: DefaultReadRetentionDays,
		Theme:             ThemeDark,
		DefaultView:       ViewCard,
	}
}

// Valid 設定値がすべて妥当かどうかを返します。
// 保持件数Nと保持日数Mは正の数を要求します。
// ポーリング間隔はmanualを含む定義済みの値、テーマと表示形式も定義済みの値を要求します。
func (s Settings) Valid() bool {
	if s.MaxItems <= 0 || s.ReadRetentionDays <= 0 {
		return false
	}
	return s.PollInterval.Valid() && s.Theme.Valid() && s.DefaultView.Valid()
}
```

- [ ] Step 4: テストが通ることを確認する

Run:
```bash
cd /Users/yujiokamoto/devs/golang/feedflow-go-htmx && go test ./internal/domain/ -run 'TestSettings|TestDefaultSettings' -v
```
Expected: `TestDefaultSettings` と `TestSettingsValid` がいずれも PASS します。

- [ ] Step 5: gofmtを適用してコミットする

```bash
cd /Users/yujiokamoto/devs/golang/feedflow-go-htmx && gofmt -w internal/domain/settings.go internal/domain/settings_test.go && git add internal/domain/settings.go internal/domain/settings_test.go && git commit -m "feat: Settings エンティティと既定値を追加する"
```

---

## Task 6: User エンティティ

Files:
- Create: `/Users/yujiokamoto/devs/golang/feedflow-go-htmx/internal/domain/user.go`

設計書セクション6と9のUserを型に落とします。単一ユーザーでユーザー名とscryptハッシュを持ちます。ハッシュ計算はPhase6のinternal/authが担うため、ここではフィールド定義と登録済みかどうかの純粋関数だけを持たせます。

- [ ] Step 1: 失敗するテストを書く

Create `internal/domain/user_test.go`:
```go
package domain

import "testing"

func TestUserIsRegistered(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		user User
		want bool
	}{
		{name: "未登録は空", user: User{}, want: false},
		{name: "名前のみでハッシュなしは未登録", user: User{Username: "owner"}, want: false},
		{name: "ハッシュのみで名前なしは未登録", user: User{PasswordHash: "abc"}, want: false},
		{name: "名前とハッシュありで登録済み", user: User{Username: "owner", PasswordHash: "abc"}, want: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := tt.user.IsRegistered(); got != tt.want {
				t.Fatalf("IsRegistered() got %v want %v", got, tt.want)
			}
		})
	}
}
```

- [ ] Step 2: テストが失敗することを確認する

Run:
```bash
cd /Users/yujiokamoto/devs/golang/feedflow-go-htmx && go test ./internal/domain/ -run 'TestUser' -v
```
Expected: コンパイルエラーで失敗します。`undefined: User` と表示されます。

- [ ] Step 3: User の実装を書く

Create `internal/domain/user.go`:
```go
package domain

// User アプリの所有者を表します。単一ユーザーの運用です。設計書のセクション6と9に対応します。
type User struct {
	Username     string `json:"username"`      // ログインに使うユーザー名です
	PasswordHash string `json:"password_hash"` // scryptで生成したパスワードハッシュです
}

// IsRegistered 所有者が登録済みかどうかを返します。
// ユーザー名とパスワードハッシュの両方が設定されているときに登録済みとみなします。
// 初回セットアップの可否判定の基礎になります。設計書のセクション9.3に対応します。
func (u User) IsRegistered() bool {
	return u.Username != "" && u.PasswordHash != ""
}
```

- [ ] Step 4: テストが通ることを確認する

Run:
```bash
cd /Users/yujiokamoto/devs/golang/feedflow-go-htmx && go test ./internal/domain/ -run 'TestUser' -v
```
Expected: `TestUserIsRegistered` の全サブテストが PASS します。

- [ ] Step 5: gofmtを適用してドメイン全体のテストを通す

Run:
```bash
cd /Users/yujiokamoto/devs/golang/feedflow-go-htmx && gofmt -w internal/domain/user.go internal/domain/user_test.go && go test -race ./internal/domain/...
```
Expected: `ok  github.com/okamyuji/feedflow-go-htmx/internal/domain` と表示されます。

- [ ] Step 6: コミットする

```bash
cd /Users/yujiokamoto/devs/golang/feedflow-go-htmx && git add internal/domain/user.go internal/domain/user_test.go && git commit -m "feat: User エンティティと登録判定を追加する"
```

---

## Task 7: Clock と IDGen のポート

Files:
- Create: `/Users/yujiokamoto/devs/golang/feedflow-go-htmx/internal/port/clock.go`
- Create: `/Users/yujiokamoto/devs/golang/feedflow-go-htmx/internal/port/idgen.go`

時刻取得とID生成を抽象化します。テストでは固定クロックと連番IDを注入してI/Oや非決定性に触れずに検証できるようにします。設計書セクション5.2に対応します。

- [ ] Step 1: Clock ポートを作成する

Create `internal/port/clock.go`:
```go
// Package port feedflowの各層境界となるインターフェースを定義します。
// このパッケージはinternal/domainにのみ依存し、具体的な実装には依存しません。
package port

import "time"

// Clock 現在時刻を返す抽象です。テストでは固定時刻を返すフェイクを注入します。
type Clock interface {
	// Now 現在時刻を返します。
	Now() time.Time
}
```

- [ ] Step 2: IDGen ポートを作成する

Create `internal/port/idgen.go`:
```go
package port

// IDGen 一意な識別子を生成する抽象です。テストでは決定的な連番を返すフェイクを注入します。
type IDGen interface {
	// NewID 一意な識別子の文字列を返します。
	NewID() string
}
```

- [ ] Step 3: コンパイルが通ることを確認する

Run:
```bash
cd /Users/yujiokamoto/devs/golang/feedflow-go-htmx && go build ./internal/port/
```
Expected: エラーなく完了します。

- [ ] Step 4: gofmtを適用してコミットする

```bash
cd /Users/yujiokamoto/devs/golang/feedflow-go-htmx && gofmt -w internal/port/clock.go internal/port/idgen.go && git add internal/port/clock.go internal/port/idgen.go && git commit -m "feat: Clock と IDGen のポートを追加する"
```

---

## Task 8: Fetcher と FetchResult のポート

Files:
- Create: `/Users/yujiokamoto/devs/golang/feedflow-go-htmx/internal/port/fetcher.go`

設計書セクション5.2と8のHTTP取得を抽象化します。ETagとLast-Modifiedを送って条件付き取得を行い、結果としてステータス、本文、新しいETagとLast-Modifiedを返します。304のときは本文を持たず未更新を示します。実装はPhase3のinternal/feedが担います。

- [ ] Step 1: Fetcher ポートを作成する

Create `internal/port/fetcher.go`:
```go
package port

import "context"

// FetchRequest 条件付き取得のための入力です。
// 前回保存したETagとLast-Modifiedを渡すと、フェッチャはそれらを使い未更新かどうかを問い合わせます。
type FetchRequest struct {
	URL          string // 取得対象のURLです
	ETag         string // 前回取得時のETagです。空なら条件付けしません
	LastModified string // 前回取得時のLast-Modifiedです。空なら条件付けしません
}

// FetchResult 取得結果です。
// NotModifiedが真のときはBodyを持たず、サーバが未更新を示したことを表します。
type FetchResult struct {
	StatusCode   int    // HTTPのステータスコードです
	NotModified  bool   // サーバが304で未更新を示したかどうかです
	Body         []byte // 取得した本文です。NotModifiedが真のときは空です
	ContentType  string // レスポンスのContent-Typeです
	ETag         string // レスポンスのETagです
	LastModified string // レスポンスのLast-Modifiedです
}

// Fetcher URLの内容をETagとLast-Modifiedを考慮して取得する抽象です。
// SSRF対策やサイズ上限やタイムアウトは実装側で担います。設計書のセクション8に対応します。
type Fetcher interface {
	// Fetch 指定したリクエストに従い内容を取得します。
	// contextのキャンセルとタイムアウトを尊重します。
	Fetch(ctx context.Context, req FetchRequest) (FetchResult, error)
}
```

- [ ] Step 2: コンパイルが通ることを確認する

Run:
```bash
cd /Users/yujiokamoto/devs/golang/feedflow-go-htmx && go build ./internal/port/
```
Expected: エラーなく完了します。

- [ ] Step 3: gofmtを適用してコミットする

```bash
cd /Users/yujiokamoto/devs/golang/feedflow-go-htmx && gofmt -w internal/port/fetcher.go && git add internal/port/fetcher.go && git commit -m "feat: Fetcher と FetchResult のポートを追加する"
```

---

## Task 9: FeedParser と ParsedFeed のポート

Files:
- Create: `/Users/yujiokamoto/devs/golang/feedflow-go-htmx/internal/port/parser.go`

設計書セクション5.2と8のパースを抽象化します。バイト列を受け取りRSS2.0とAtomとRDFを判別してパースし、フィードのメタ情報と記事群を返します。記事は永続化の都合でdomain.Itemそのものではなく、パース段階の中間表現ParsedItemとして返し、ID付与やFeedIDの紐付けはサービス層が行います。実装はPhase3のinternal/feedが担います。

- [ ] Step 1: FeedParser ポートを作成する

Create `internal/port/parser.go`:
```go
package port

import "time"

// FeedFormat パースで判別したフィードの形式を表します。
type FeedFormat string

// フィード形式の取りうる値です。設計書のセクション8に対応します。
const (
	FormatRSS2 FeedFormat = "rss2" // RSS 2.0です
	FormatAtom FeedFormat = "atom" // Atomです
	FormatRDF  FeedFormat = "rdf"  // RDFつまりRSS 1.0です
)

// ParsedItem パース段階の記事の中間表現です。
// 永続化用のIDやFeedIDの付与はサービス層が担うため、ここには含めません。
type ParsedItem struct {
	GUID        string    // フィード内での記事の一意キーです
	Title       string    // 記事のタイトルです
	Link        string    // 元記事のURLです
	Content     string    // 記事本文です
	Summary     string    // 記事の要約です
	Author      string    // 著者名です
	PublishedAt time.Time // 公開日時です
}

// ParsedFeed パース結果のフィード全体です。
type ParsedFeed struct {
	Format  FeedFormat   // 判別したフィード形式です
	Title   string       // フィードのタイトルです
	SiteURL string       // フィードが指すサイトのURLです
	Items   []ParsedItem // パースした記事群です
}

// FeedParser バイト列を受け取り形式を判別してフィードをパースする抽象です。
// 設計書のセクション8に対応します。
type FeedParser interface {
	// Parse 与えられたバイト列をRSS 2.0とAtomとRDFのいずれかとして判別しパースします。
	// 判別に失敗した場合やパースに失敗した場合はエラーを返します。
	Parse(data []byte) (ParsedFeed, error)
}
```

- [ ] Step 2: コンパイルが通ることを確認する

Run:
```bash
cd /Users/yujiokamoto/devs/golang/feedflow-go-htmx && go build ./internal/port/
```
Expected: エラーなく完了します。

- [ ] Step 3: gofmtを適用してコミットする

```bash
cd /Users/yujiokamoto/devs/golang/feedflow-go-htmx && gofmt -w internal/port/parser.go && git add internal/port/parser.go && git commit -m "feat: FeedParser と ParsedFeed のポートを追加する"
```

---

## Task 10: Repository のポート

Files:
- Create: `/Users/yujiokamoto/devs/golang/feedflow-go-htmx/internal/port/repository.go`

設計書セクション5.2と7のとおり、フィード、カテゴリ、記事、ボード、フィルタ、設定、ユーザーの取得と保存を担う永続化境界を定義します。記事はフィードごとに分割保存するため、FeedID単位の取得と保存と削除を持たせます。実装はPhase2のinternal/storeが担います。すべてのメソッドはエラーを返します。

- [ ] Step 1: Repository ポートを作成する

Create `internal/port/repository.go`:
```go
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
```

- [ ] Step 2: コンパイルが通ることを確認する

Run:
```bash
cd /Users/yujiokamoto/devs/golang/feedflow-go-htmx && go build ./internal/port/
```
Expected: エラーなく完了します。

- [ ] Step 3: gofmtを適用してコミットする

```bash
cd /Users/yujiokamoto/devs/golang/feedflow-go-htmx && gofmt -w internal/port/repository.go && git add internal/port/repository.go && git commit -m "feat: Repository のポートを追加する"
```

---

## Task 11: サービスのポート

Files:
- Create: `/Users/yujiokamoto/devs/golang/feedflow-go-htmx/internal/port/service.go`

設計書セクション5.2のとおり、ハンドラ層がサービスをインターフェース経由で受けられるよう、各サービスのインターフェースを定義します。購読管理、記事操作、保持ポリシー、ミュート、OPML、設定、取得反映を分割します。実装はPhase4とPhase5のinternal/serviceとinternal/pollerが担います。OPML入出力の戻り値は標準ライブラリのbyte列で表します。

- [ ] Step 1: サービスのポートを作成する

Create `internal/port/service.go`:
```go
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

// ItemService 記事の既読やスターやあとで読むやタグやボードやメモの操作を担う抽象です。
// 設計書のセクション3.1に対応します。
type ItemService interface {
	// ListItems 指定フィードの記事をミュート適用済みで返します。feedIDが空なら全フィード横断で返します。
	ListItems(feedID string) ([]domain.Item, error)
	// MarkRead 指定記事の既読状態を設定します。
	MarkRead(feedID, itemID string, read bool) error
	// MarkAllRead 指定フィードの全記事を既読にします。feedIDが空なら全フィードを対象にします。
	MarkAllRead(feedID string) error
	// Star 指定記事のスター状態を設定します。
	Star(feedID, itemID string, starred bool) error
	// ReadLater 指定記事のあとで読む状態を設定します。
	ReadLater(feedID, itemID string, readLater bool) error
	// SetTags 指定記事のタグを更新します。
	SetTags(feedID, itemID string, tags []string) error
	// SetBoards 指定記事の保存先ボードを更新します。
	SetBoards(feedID, itemID string, boardIDs []string) error
	// SetNote 指定記事のメモを更新します。
	SetNote(feedID, itemID, note string) error
	// AddHighlight 指定記事にハイライトを追加します。
	AddHighlight(feedID, itemID, highlight string) error
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
```

- [ ] Step 2: コンパイルが通ることを確認する

Run:
```bash
cd /Users/yujiokamoto/devs/golang/feedflow-go-htmx && go build ./internal/port/
```
Expected: エラーなく完了します。

- [ ] Step 3: gofmtを適用してコミットする

```bash
cd /Users/yujiokamoto/devs/golang/feedflow-go-htmx && gofmt -w internal/port/service.go && git add internal/port/service.go && git commit -m "feat: 各サービスのポートを追加する"
```

---

## Task 12: ポートを満たすフェイク実装とインターフェース充足の検証

Files:
- Create: `/Users/yujiokamoto/devs/golang/feedflow-go-htmx/internal/port/port_test.go`

後続フェーズがフェイク注入でテストできることを早期に保証します。Clock、IDGen、Fetcher、FeedParser、Repositoryの各ポートを満たす最小フェイクをテスト内に定義し、コンパイル時点でインターフェース充足を検証します。これにより設計したシグネチャの実装可能性をこのフェーズで確定させます。

- [ ] Step 1: ポート充足の検証テストを書く

Create `internal/port/port_test.go`:
```go
package port_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/okamyuji/feedflow-go-htmx/internal/domain"
	"github.com/okamyuji/feedflow-go-htmx/internal/port"
)

// fakeClock 固定時刻を返すClockのフェイクです。
type fakeClock struct{ now time.Time }

func (c fakeClock) Now() time.Time { return c.now }

// fakeIDGen 決定的な連番IDを返すIDGenのフェイクです。
type fakeIDGen struct{ n int }

func (g *fakeIDGen) NewID() string {
	g.n++
	return "id-" + time.Duration(g.n).String()
}

// fakeFetcher 固定の結果を返すFetcherのフェイクです。
type fakeFetcher struct {
	result port.FetchResult
	err    error
}

func (f fakeFetcher) Fetch(_ context.Context, _ port.FetchRequest) (port.FetchResult, error) {
	return f.result, f.err
}

// fakeParser 固定の結果を返すFeedParserのフェイクです。
type fakeParser struct {
	parsed port.ParsedFeed
	err    error
}

func (p fakeParser) Parse(_ []byte) (port.ParsedFeed, error) {
	return p.parsed, p.err
}

// fakeRepo メモリ上で全エンティティを保持するRepositoryのフェイクです。
type fakeRepo struct {
	feeds      map[string]domain.Feed
	categories map[string]domain.Category
	items      map[string][]domain.Item
	boards     map[string]domain.Board
	filters    map[string]domain.MuteFilter
	settings   domain.Settings
	user       domain.User
}

func newFakeRepo() *fakeRepo {
	return &fakeRepo{
		feeds:      map[string]domain.Feed{},
		categories: map[string]domain.Category{},
		items:      map[string][]domain.Item{},
		boards:     map[string]domain.Board{},
		filters:    map[string]domain.MuteFilter{},
		settings:   domain.DefaultSettings(),
	}
}

var errNotFound = errors.New("not found")

func (r *fakeRepo) Feeds() ([]domain.Feed, error) {
	out := make([]domain.Feed, 0, len(r.feeds))
	for _, f := range r.feeds {
		out = append(out, f)
	}
	return out, nil
}

func (r *fakeRepo) Feed(id string) (domain.Feed, error) {
	f, ok := r.feeds[id]
	if !ok {
		return domain.Feed{}, errNotFound
	}
	return f, nil
}

func (r *fakeRepo) SaveFeed(feed domain.Feed) error {
	r.feeds[feed.ID] = feed
	return nil
}

func (r *fakeRepo) DeleteFeed(id string) error {
	delete(r.feeds, id)
	delete(r.items, id)
	return nil
}

func (r *fakeRepo) Categories() ([]domain.Category, error) {
	out := make([]domain.Category, 0, len(r.categories))
	for _, c := range r.categories {
		out = append(out, c)
	}
	return out, nil
}

func (r *fakeRepo) SaveCategory(category domain.Category) error {
	r.categories[category.ID] = category
	return nil
}

func (r *fakeRepo) DeleteCategory(id string) error {
	delete(r.categories, id)
	return nil
}

func (r *fakeRepo) Items(feedID string) ([]domain.Item, error) {
	return r.items[feedID], nil
}

func (r *fakeRepo) SaveItems(feedID string, items []domain.Item) error {
	r.items[feedID] = items
	return nil
}

func (r *fakeRepo) Boards() ([]domain.Board, error) {
	out := make([]domain.Board, 0, len(r.boards))
	for _, b := range r.boards {
		out = append(out, b)
	}
	return out, nil
}

func (r *fakeRepo) SaveBoard(board domain.Board) error {
	r.boards[board.ID] = board
	return nil
}

func (r *fakeRepo) DeleteBoard(id string) error {
	delete(r.boards, id)
	return nil
}

func (r *fakeRepo) Filters() ([]domain.MuteFilter, error) {
	out := make([]domain.MuteFilter, 0, len(r.filters))
	for _, f := range r.filters {
		out = append(out, f)
	}
	return out, nil
}

func (r *fakeRepo) SaveFilter(filter domain.MuteFilter) error {
	r.filters[filter.ID] = filter
	return nil
}

func (r *fakeRepo) DeleteFilter(id string) error {
	delete(r.filters, id)
	return nil
}

func (r *fakeRepo) Settings() (domain.Settings, error) {
	return r.settings, nil
}

func (r *fakeRepo) SaveSettings(settings domain.Settings) error {
	r.settings = settings
	return nil
}

func (r *fakeRepo) User() (domain.User, error) {
	return r.user, nil
}

func (r *fakeRepo) SaveUser(user domain.User) error {
	r.user = user
	return nil
}

// インターフェース充足をコンパイル時に検証します。
var (
	_ port.Clock      = fakeClock{}
	_ port.IDGen      = (*fakeIDGen)(nil)
	_ port.Fetcher    = fakeFetcher{}
	_ port.FeedParser = fakeParser{}
	_ port.Repository = (*fakeRepo)(nil)
)

func TestFakesSatisfyPorts(t *testing.T) {
	t.Parallel()
	repo := newFakeRepo()
	if err := repo.SaveFeed(domain.Feed{ID: "f1", Title: "feed"}); err != nil {
		t.Fatalf("SaveFeed returned error: %v", err)
	}
	got, err := repo.Feed("f1")
	if err != nil {
		t.Fatalf("Feed returned error: %v", err)
	}
	if got.Title != "feed" {
		t.Fatalf("Feed title got %q want %q", got.Title, "feed")
	}

	clk := fakeClock{now: time.Date(2026, 5, 29, 0, 0, 0, 0, time.UTC)}
	if clk.Now().Year() != 2026 {
		t.Fatalf("fakeClock year got %d want 2026", clk.Now().Year())
	}

	gen := &fakeIDGen{}
	if gen.NewID() == gen.NewID() {
		t.Fatalf("fakeIDGen must return distinct ids")
	}

	f := fakeFetcher{result: port.FetchResult{StatusCode: 200, Body: []byte("ok")}}
	res, err := f.Fetch(context.Background(), port.FetchRequest{URL: "https://example.com"})
	if err != nil {
		t.Fatalf("Fetch returned error: %v", err)
	}
	if res.StatusCode != 200 {
		t.Fatalf("Fetch status got %d want 200", res.StatusCode)
	}

	p := fakeParser{parsed: port.ParsedFeed{Format: port.FormatRSS2, Title: "t"}}
	parsed, err := p.Parse([]byte("<rss></rss>"))
	if err != nil {
		t.Fatalf("Parse returned error: %v", err)
	}
	if parsed.Format != port.FormatRSS2 {
		t.Fatalf("Parse format got %q want %q", parsed.Format, port.FormatRSS2)
	}
}
```

- [ ] Step 2: テストが通ることを確認する

Run:
```bash
cd /Users/yujiokamoto/devs/golang/feedflow-go-htmx && go test -race ./internal/port/...
```
Expected: `ok  github.com/okamyuji/feedflow-go-htmx/internal/port` と表示されます。コンパイルが通ること自体が、設計したインターフェースをフェイクで満たせる証明になります。

- [ ] Step 3: gofmtを適用してコミットする

```bash
cd /Users/yujiokamoto/devs/golang/feedflow-go-htmx && gofmt -w internal/port/port_test.go && git add internal/port/port_test.go && git commit -m "test: ポートを満たすフェイクでインターフェース充足を検証する"
```

---

## Task 13: フェーズ全体のテストと品質ゲート

Files:
- 変更なし

- [ ] Step 1: ドメインとポートの全テストを race で実行する

Run:
```bash
cd /Users/yujiokamoto/devs/golang/feedflow-go-htmx && go test -race -count=1 ./internal/domain/... ./internal/port/...
```
Expected: 両パッケージとも `ok` と表示されます。

- [ ] Step 2: カバレッジを確認する

Run:
```bash
cd /Users/yujiokamoto/devs/golang/feedflow-go-htmx && go test -coverprofile=coverage.out ./internal/domain/... && go tool cover -func=coverage.out | tail -n 1
```
Expected: domainパッケージの純粋関数は網羅済みで、合計カバレッジが80パーセント前後以上になります。目安であり厳密な合否基準ではありません。

- [ ] Step 3: 品質ゲートを実行する

Run:
```bash
cd /Users/yujiokamoto/devs/golang/feedflow-go-htmx && bash scripts/quality-gate.sh
```
Expected: `all quality checks passed` で終わります。lintやvetの指摘が出たら修正してから再実行します。

- [ ] Step 4: 品質ゲート緑のままコミットする

```bash
cd /Users/yujiokamoto/devs/golang/feedflow-go-htmx && git add -A && git commit -m "chore: Phase1 のドメインとポートで品質ゲートを緑化する"
```
Expected: コミット時に quality-gate が走り、緑のままコミットされます。差分が無ければこのコミットは省略できます。

---

## Phase1 完了条件

- [ ] `go test -race ./internal/domain/... ./internal/port/...` が通る
- [ ] domainの純粋関数(HasError、InCategory、HasUserAction、ShouldRetain、Matches、Valid、DefaultSettings、IsRegistered)にテーブル駆動テストがある
- [ ] internal/portの全インターフェース(Repository、Fetcher、FeedParser、Clock、IDGen、および各サービス)が定義済みである
- [ ] フェイク実装でインターフェース充足がコンパイル時に検証されている
- [ ] `bash scripts/quality-gate.sh` が `all quality checks passed` で終わる
- [ ] コミットが規約に沿って積まれている
