# 任意URLのブックマーク追加 実装手順書

> For agentic workers: REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

## Goal

ブックマークビューの一覧バーから任意のURLを入力し、タイトル付きのカードとしてブックマークへ保存できるようにする。

## Architecture

予約ID `saved-pages` を持つ合成フィードを初回追加時に遅延作成し、そこへ記事を積む。既存のブックマーク機構（ピッカー、カード、`POST /app/items/{feedID}/{itemID}/bookmark`、ブックマークビュー一覧）は無改修で動く。合成フィードはポーリング、左ツリー、OPML出力の3系統から除外する。

## Tech Stack

Go 1.26、標準ライブラリ `net/url`、`golang.org/x/net/html`、`golang.org/x/net/html/charset`、HTMX、Alpine.js、Go html/template。

## Spec

`docs/superpowers/specs/2026-08-22-bookmark-add-url-design.md`

## Global Constraints

- 合成フィードのIDは `saved-pages` 固定。表示名は `保存したページ` 固定。
- タイトル抽出の優先順位は `og:title` → `<title>` → 入力URL。
- タイトル取得のタイムアウトは20秒。
- URL正規化はscheme/host小文字化、フラグメント除去、末尾スラッシュ除去。クエリは保持する。
- 許可するschemeは `http` と `https` のみ。
- 変更したコードのユニットテストカバレッジは80%以上。変更行のミューテーションはすべてKILLされること。変更した関数のCRAPスコアは15以下。
- テストは正常系だけでなく異常系、境界値、エッジケースを網羅する。
- UIに絵文字を使わない。件数バッジを追加しない。
- コメントは日本語。公開シンボルには必ずdocコメントを付ける（`.golangci.yml` が強制）。
- コミットはConventional Commits。AI帰属（`Co-Authored-By: Claude` 等）を絶対に入れない。
- 状態変更ルートは `h.requireAuth(h.requireCSRF(...))` で包む。

## File Structure

| ファイル | 役割 |
|---|---|
| `internal/domain/saved.go` | 新規。合成フィードのID定数と判定関数 |
| `internal/domain/saved_test.go` | 新規。判定関数のテスト |
| `internal/feed/meta.go` | 新規。HTMLからのタイトル抽出 |
| `internal/feed/meta_test.go` | 新規。タイトル抽出のテスト |
| `internal/service/url.go` | 新規。URL検証と正規化 |
| `internal/service/url_test.go` | 新規。URL検証と正規化のテスト |
| `internal/service/bookmark.go` | 変更。`AddURL` を追加 |
| `internal/service/bookmark_addurl_test.go` | 新規。`AddURL` のテスト |
| `internal/service/item.go` | 変更。`DeleteItem` を追加 |
| `internal/service/item_delete_test.go` | 新規。`DeleteItem` のテスト |
| `internal/service/opml.go` | 変更。Exportから合成フィードを除外 |
| `internal/port/service.go` | 変更。`BookmarkService.AddURL` と `ItemService.DeleteItem` を追加 |
| `internal/poller/service.go` | 変更。合成フィードを取得対象から除外 |
| `internal/poller/runner.go` | 変更。合成フィードを期限判定から除外 |
| `internal/poller/saved_skip_test.go` | 新規。除外のテスト |
| `internal/handler/feed_handler.go` | 変更。左ツリーから合成フィードを除外 |
| `internal/handler/bookmark_handler.go` | 変更。`bookmarkAddURL` を追加 |
| `internal/handler/item_handler.go` | 変更。合成フィードの解除で記事を削除 |
| `internal/handler/render.go` | 変更。`pageData` にフォーム用フィールドを追加 |
| `internal/handler/router.go` | 変更。ルートを1行追加 |
| `internal/handler/templates/_bookmark_add_url.html` | 新規。追加フォーム |
| `internal/handler/templates/_item_list.html` | 変更。フォームを差し込む |
| `internal/handler/static/styles.css` | 変更。フォームのスタイル |
| `internal/handler/bookmark_addurl_handler_test.go` | 新規。ハンドラのテスト |
| `e2e/playwright/tests/bookmark-add-url.spec.ts` | 新規。E2E |
| `README.md` | 変更。機能一覧に1行追加 |

---

## Task 1: 合成フィードの定数と判定

### Files

- Create: `internal/domain/saved.go`
- Test: `internal/domain/saved_test.go`

### Interfaces

- Produces: `domain.SavedPagesFeedID string`、`domain.SavedPagesFeedTitle string`、`domain.IsSavedPagesFeed(feedID string) bool`

- [ ] Step 1: 失敗するテストを書く

`internal/domain/saved_test.go`

```go
package domain_test

import (
	"testing"

	"github.com/okamyuji/feedflow-go-htmx/internal/domain"
)

func TestSavedPagesFeedConstants(t *testing.T) {
	if domain.SavedPagesFeedID != "saved-pages" {
		t.Errorf("SavedPagesFeedID = %q, want %q", domain.SavedPagesFeedID, "saved-pages")
	}
	if domain.SavedPagesFeedTitle != "保存したページ" {
		t.Errorf("SavedPagesFeedTitle = %q, want %q", domain.SavedPagesFeedTitle, "保存したページ")
	}
}

func TestIsSavedPagesFeed(t *testing.T) {
	tests := []struct {
		name   string
		feedID string
		want   bool
	}{
		{name: "合成フィードのID", feedID: "saved-pages", want: true},
		{name: "通常のフィードID", feedID: "a1b2c3", want: false},
		{name: "空文字", feedID: "", want: false},
		{name: "大文字違い", feedID: "SAVED-PAGES", want: false},
		{name: "前方一致するだけの別ID", feedID: "saved-pages-2", want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := domain.IsSavedPagesFeed(tt.feedID); got != tt.want {
				t.Errorf("IsSavedPagesFeed(%q) = %v, want %v", tt.feedID, got, tt.want)
			}
		})
	}
}
```

- [ ] Step 2: テストが失敗することを確認する

Run: `go test ./internal/domain/ -run 'SavedPages|IsSavedPagesFeed' -v`
Expected: FAIL（`undefined: domain.SavedPagesFeedID`）

- [ ] Step 3: 最小の実装を書く

`internal/domain/saved.go`

```go
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
```

- [ ] Step 4: テストが通ることを確認する

Run: `go test ./internal/domain/ -run 'SavedPages|IsSavedPagesFeed' -v`
Expected: PASS

- [ ] Step 5: コミットする

```bash
git add internal/domain/saved.go internal/domain/saved_test.go
git commit -m "feat(domain): 任意URL保存用の合成フィード定数と判定を追加"
```

---

## Task 2: HTMLからのタイトル抽出

### Files

- Create: `internal/feed/meta.go`
- Test: `internal/feed/meta_test.go`

### Interfaces

- Produces: `feed.PageMeta struct{ Title string }`、`feed.ExtractMeta(body []byte, contentType string) PageMeta`

- [ ] Step 1: 失敗するテストを書く

`internal/feed/meta_test.go`

```go
package feed_test

import (
	"strings"
	"testing"

	"github.com/okamyuji/feedflow-go-htmx/internal/feed"
)

func TestExtractMetaTitle(t *testing.T) {
	tests := []struct {
		name        string
		body        string
		contentType string
		want        string
	}{
		{
			name:        "og:titleを優先する",
			body:        `<html><head><meta property="og:title" content="OGタイトル"><title>タイトル要素</title></head><body></body></html>`,
			contentType: "text/html; charset=utf-8",
			want:        "OGタイトル",
		},
		{
			name:        "og:titleが無ければtitle要素を使う",
			body:        `<html><head><title>タイトル要素</title></head><body></body></html>`,
			contentType: "text/html; charset=utf-8",
			want:        "タイトル要素",
		},
		{
			name:        "どちらも無ければ空",
			body:        `<html><head></head><body><p>本文</p></body></html>`,
			contentType: "text/html; charset=utf-8",
			want:        "",
		},
		{
			name:        "og:titleが空白のみならtitle要素にフォールバックする",
			body:        `<html><head><meta property="og:title" content="   "><title>タイトル要素</title></head></html>`,
			contentType: "text/html; charset=utf-8",
			want:        "タイトル要素",
		},
		{
			name:        "前後空白を除去し連続空白を1つに畳む",
			body:        "<html><head><title>  前 \n\t 後  </title></head></html>",
			contentType: "text/html; charset=utf-8",
			want:        "前 後",
		},
		{
			name:        "name属性のog:titleも拾う",
			body:        `<html><head><meta name="og:title" content="name属性のOG"></head></html>`,
			contentType: "text/html; charset=utf-8",
			want:        "name属性のOG",
		},
		{
			name:        "壊れたHTMLでもpanicしない",
			body:        `<html><head><title>壊れた`,
			contentType: "text/html",
			want:        "壊れた",
		},
		{
			name:        "空のbodyで空を返す",
			body:        ``,
			contentType: "text/html",
			want:        "",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := feed.ExtractMeta([]byte(tt.body), tt.contentType)
			if got.Title != tt.want {
				t.Errorf("ExtractMeta().Title = %q, want %q", got.Title, tt.want)
			}
		})
	}
}

func TestExtractMetaTitleTruncatesToMaxRunes(t *testing.T) {
	long := strings.Repeat("あ", 300)
	got := feed.ExtractMeta([]byte("<html><head><title>"+long+"</title></head></html>"), "text/html; charset=utf-8")
	if n := len([]rune(got.Title)); n != 256 {
		t.Errorf("title rune length = %d, want 256", n)
	}
}

func TestExtractMetaDecodesShiftJIS(t *testing.T) {
	// Shift_JISで「日本語」をエンコードしたバイト列です。
	sjis := []byte{
		0x3c, 0x68, 0x74, 0x6d, 0x6c, 0x3e, 0x3c, 0x68, 0x65, 0x61, 0x64, 0x3e,
		0x3c, 0x74, 0x69, 0x74, 0x6c, 0x65, 0x3e,
		0x93, 0xfa, 0x96, 0x7b, 0x8c, 0xea,
		0x3c, 0x2f, 0x74, 0x69, 0x74, 0x6c, 0x65, 0x3e, 0x3c, 0x2f, 0x68, 0x65, 0x61, 0x64, 0x3e, 0x3c, 0x2f, 0x68, 0x74, 0x6d, 0x6c, 0x3e,
	}
	got := feed.ExtractMeta(sjis, "text/html; charset=Shift_JIS")
	if got.Title != "日本語" {
		t.Errorf("ExtractMeta().Title = %q, want %q", got.Title, "日本語")
	}
}
```

- [ ] Step 2: テストが失敗することを確認する

Run: `go test ./internal/feed/ -run ExtractMeta -v`
Expected: FAIL（`undefined: feed.ExtractMeta`）

- [ ] Step 3: 最小の実装を書く

`internal/feed/meta.go`

```go
package feed

import (
	"bytes"
	"strings"

	"golang.org/x/net/html"
	"golang.org/x/net/html/atom"
	"golang.org/x/net/html/charset"
)

// maxTitleRunes 保存するタイトルの最大文字数です。
// 極端に長いタイトルが一覧の描画とJSONの肥大を招くため上限を設けます。
const maxTitleRunes = 256

// PageMeta HTMLページから抽出したメタデータです。
type PageMeta struct {
	Title string // og:titleまたはtitle要素から得たページ名です
}

// ExtractMeta HTMLバイト列とContent-Typeからページのメタデータを抽出します。
// og:title を優先し、無ければ title 要素を使います。どちらも得られない場合はTitleが空になります。
// 文字コードはContent-Typeとmetaタグから解決します。
// パースに失敗した場合はゼロ値のPageMetaを返します。
func ExtractMeta(body []byte, contentType string) PageMeta {
	reader, err := charset.NewReader(bytes.NewReader(body), contentType)
	if err != nil {
		// 文字コードを解決できない場合はバイト列をそのまま読み進めます。
		reader = bytes.NewReader(body)
	}
	doc, err := html.Parse(reader)
	if err != nil {
		return PageMeta{}
	}
	ogTitle, titleTag := scanTitles(doc)
	title := normalizeTitle(ogTitle)
	if title == "" {
		title = normalizeTitle(titleTag)
	}
	return PageMeta{Title: title}
}

// scanTitles 文書を1度だけ走査してog:titleとtitle要素の内容を返します。
// 同じ要素が複数あれば最初のものを採用します。
func scanTitles(doc *html.Node) (ogTitle, titleTag string) {
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.ElementNode {
			switch n.DataAtom {
			case atom.Meta:
				if ogTitle == "" {
					if v, ok := ogTitleContent(n); ok {
						ogTitle = v
					}
				}
			case atom.Title:
				if titleTag == "" {
					titleTag = textContent(n)
				}
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(doc)
	return ogTitle, titleTag
}

// ogTitleContent meta要素がog:titleならそのcontent属性を返します。
// propertyとnameのどちらで指定されていても拾います。
func ogTitleContent(n *html.Node) (string, bool) {
	isOG := false
	content := ""
	for _, a := range n.Attr {
		switch strings.ToLower(a.Key) {
		case "property", "name":
			if strings.EqualFold(strings.TrimSpace(a.Val), "og:title") {
				isOG = true
			}
		case "content":
			content = a.Val
		}
	}
	if !isOG {
		return "", false
	}
	return content, true
}

// textContent 要素配下のテキストノードを連結して返します。
func textContent(n *html.Node) string {
	var sb strings.Builder
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		if c.Type == html.TextNode {
			sb.WriteString(c.Data)
		}
	}
	return sb.String()
}

// normalizeTitle 前後空白を除去し、連続する空白を1つに畳み、上限文字数で切り詰めます。
func normalizeTitle(s string) string {
	s = strings.Join(strings.Fields(s), " ")
	runes := []rune(s)
	if len(runes) > maxTitleRunes {
		return string(runes[:maxTitleRunes])
	}
	return s
}
```

- [ ] Step 4: テストが通ることを確認する

Run: `go test ./internal/feed/ -run ExtractMeta -v`
Expected: PASS

- [ ] Step 5: コミットする

```bash
git add internal/feed/meta.go internal/feed/meta_test.go
git commit -m "feat(feed): HTMLからページタイトルを抽出するExtractMetaを追加"
```

---

## Task 3: URLの検証と正規化

### Files

- Create: `internal/service/url.go`
- Test: `internal/service/url_test.go`

### Interfaces

- Produces: `service.ErrInvalidURL error`、`service.normalizeURL(rawURL string) (string, error)`

`normalizeURL` は非公開関数のため、テストは同一パッケージ（`package service`）に置く。

- [ ] Step 1: 失敗するテストを書く

`internal/service/url_test.go`

```go
package service

import (
	"errors"
	"testing"
)

func TestNormalizeURL(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{name: "そのまま", in: "https://example.com/a", want: "https://example.com/a"},
		{name: "末尾スラッシュを除去", in: "https://example.com/a/", want: "https://example.com/a"},
		{name: "ルートのスラッシュは残す", in: "https://example.com/", want: "https://example.com/"},
		{name: "パス無しはルート扱い", in: "https://example.com", want: "https://example.com"},
		{name: "フラグメントを除去", in: "https://example.com/a#sec", want: "https://example.com/a"},
		{name: "schemeを小文字化", in: "HTTPS://example.com/a", want: "https://example.com/a"},
		{name: "hostを小文字化", in: "https://EXAMPLE.COM/a", want: "https://example.com/a"},
		{name: "パスの大文字は保持", in: "https://example.com/AbC", want: "https://example.com/AbC"},
		{name: "クエリを保持", in: "https://example.com/a?b=1&c=2", want: "https://example.com/a?b=1&c=2"},
		{name: "クエリと末尾スラッシュの併存", in: "https://example.com/a/?b=1", want: "https://example.com/a?b=1"},
		{name: "前後の空白を除去", in: "  https://example.com/a  ", want: "https://example.com/a"},
		{name: "ポート番号を保持", in: "http://example.com:8080/a", want: "http://example.com:8080/a"},
		{name: "httpスキームを許可", in: "http://example.com/a", want: "http://example.com/a"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := normalizeURL(tt.in)
			if err != nil {
				t.Fatalf("normalizeURL(%q) returned error: %v", tt.in, err)
			}
			if got != tt.want {
				t.Errorf("normalizeURL(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestNormalizeURLRejects(t *testing.T) {
	tests := []struct {
		name string
		in   string
	}{
		{name: "空文字", in: ""},
		{name: "空白のみ", in: "   "},
		{name: "javascriptスキーム", in: "javascript:alert(1)"},
		{name: "fileスキーム", in: "file:///etc/passwd"},
		{name: "dataスキーム", in: "data:text/html,<h1>x</h1>"},
		{name: "ftpスキーム", in: "ftp://example.com/a"},
		{name: "スキーム無し", in: "example.com/a"},
		{name: "host無し", in: "http:///a"},
		{name: "解析できない文字列", in: "http://[::1"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := normalizeURL(tt.in)
			if !errors.Is(err, ErrInvalidURL) {
				t.Errorf("normalizeURL(%q) = (%q, %v), want ErrInvalidURL", tt.in, got, err)
			}
		})
	}
}
```

- [ ] Step 2: テストが失敗することを確認する

Run: `go test ./internal/service/ -run NormalizeURL -v`
Expected: FAIL（`undefined: normalizeURL`）

- [ ] Step 3: 最小の実装を書く

`internal/service/url.go`

```go
package service

import (
	"errors"
	"net/url"
	"strings"
)

// ErrInvalidURL 保存対象として受け付けられないURLが渡されたときに返すエラーです。
// httpとhttps以外のスキームや、ホストを持たないURLを弾きます。
var ErrInvalidURL = errors.New("invalid url")

// normalizeURL 入力URLを検証し、重複判定に使える正規形へ整えて返します。
// schemeとhostを小文字にし、フラグメントを除去し、ルート以外の末尾スラッシュを取り除きます。
// クエリは記事の同一性に関わるため保持します。
// httpとhttps以外のスキームとホスト無しのURLはErrInvalidURLを返します。
func normalizeURL(rawURL string) (string, error) {
	rawURL = strings.TrimSpace(rawURL)
	if rawURL == "" {
		return "", ErrInvalidURL
	}
	u, err := url.Parse(rawURL)
	if err != nil {
		return "", errors.Join(ErrInvalidURL, err)
	}
	scheme := strings.ToLower(u.Scheme)
	if scheme != "http" && scheme != "https" {
		return "", ErrInvalidURL
	}
	if u.Host == "" {
		return "", ErrInvalidURL
	}
	u.Scheme = scheme
	u.Host = strings.ToLower(u.Host)
	u.Fragment = ""
	u.RawFragment = ""
	if len(u.Path) > 1 {
		u.Path = strings.TrimRight(u.Path, "/")
		u.RawPath = ""
	}
	return u.String(), nil
}
```

- [ ] Step 4: テストが通ることを確認する

Run: `go test ./internal/service/ -run NormalizeURL -v`
Expected: PASS

- [ ] Step 5: コミットする

```bash
git add internal/service/url.go internal/service/url_test.go
git commit -m "feat(service): 保存URLの検証と正規化を追加"
```

---

## Task 4: ItemService.DeleteItem

### Files

- Modify: `internal/service/item.go`
- Modify: `internal/port/service.go`
- Test: `internal/service/item_delete_test.go`

### Interfaces

- Consumes: `domain.IsSavedPagesFeed`（Task 1）
- Produces: `service.ErrNotSavedPagesFeed error`、`(*ItemService).DeleteItem(feedID, itemID string) error`、`port.ItemService` に `DeleteItem(feedID, itemID string) error` を追加

- [ ] Step 1: 失敗するテストを書く

`internal/service/item_delete_test.go`

既存の `internal/service/fakes_test.go` のフェイクリポジトリを使う。フェイクの型名と初期化方法はそのファイルを読んで合わせること。

```go
package service_test

import (
	"errors"
	"testing"

	"github.com/okamyuji/feedflow-go-htmx/internal/domain"
	"github.com/okamyuji/feedflow-go-htmx/internal/service"
)

func TestDeleteItemRemovesFromSavedPagesFeed(t *testing.T) {
	repo := newFakeRepo()
	repo.feeds = []domain.Feed{{ID: domain.SavedPagesFeedID, Title: domain.SavedPagesFeedTitle}}
	repo.items[domain.SavedPagesFeedID] = []domain.Item{
		{ID: "i1", FeedID: domain.SavedPagesFeedID, Link: "https://example.com/1", Bookmarked: true},
		{ID: "i2", FeedID: domain.SavedPagesFeedID, Link: "https://example.com/2", Bookmarked: true},
	}
	svc := newItemServiceForTest(repo)

	if err := svc.DeleteItem(domain.SavedPagesFeedID, "i1"); err != nil {
		t.Fatalf("DeleteItem returned error: %v", err)
	}

	got := repo.items[domain.SavedPagesFeedID]
	if len(got) != 1 || got[0].ID != "i2" {
		t.Errorf("items after delete = %+v, want only i2", got)
	}
}

func TestDeleteItemRejectsNonSavedPagesFeed(t *testing.T) {
	repo := newFakeRepo()
	repo.feeds = []domain.Feed{{ID: "f1"}}
	repo.items["f1"] = []domain.Item{{ID: "i1", FeedID: "f1"}}
	svc := newItemServiceForTest(repo)

	err := svc.DeleteItem("f1", "i1")
	if !errors.Is(err, service.ErrNotSavedPagesFeed) {
		t.Fatalf("DeleteItem error = %v, want ErrNotSavedPagesFeed", err)
	}
	if len(repo.items["f1"]) != 1 {
		t.Errorf("items were modified despite rejection: %+v", repo.items["f1"])
	}
}

func TestDeleteItemReturnsNotFoundForUnknownItem(t *testing.T) {
	repo := newFakeRepo()
	repo.feeds = []domain.Feed{{ID: domain.SavedPagesFeedID}}
	repo.items[domain.SavedPagesFeedID] = []domain.Item{{ID: "i1", FeedID: domain.SavedPagesFeedID}}
	svc := newItemServiceForTest(repo)

	if err := svc.DeleteItem(domain.SavedPagesFeedID, "missing"); !errors.Is(err, service.ErrItemNotFound) {
		t.Fatalf("DeleteItem error = %v, want ErrItemNotFound", err)
	}
}
```

`newItemServiceForTest` が既存テストに無ければ、次のヘルパを同ファイルの末尾に足す。既存の構築方法があるならそれに合わせる。

```go
// newItemServiceForTest テスト用にItemServiceを構築します。
func newItemServiceForTest(repo *fakeRepo) *service.ItemService {
	deps := service.Deps{Repo: repo, Clock: newFakeClock(), IDs: newFakeIDGen()}
	return service.NewItemService(deps, service.NewMuteService(deps))
}
```

- [ ] Step 2: テストが失敗することを確認する

Run: `go test ./internal/service/ -run DeleteItem -v`
Expected: FAIL（`svc.DeleteItem undefined`）

- [ ] Step 3: 最小の実装を書く

`internal/service/item.go` の末尾付近に追加する。ファイル先頭の `var` 群の並びに合わせて、エラー変数は既存の `ErrItemNotFound` の隣に置く。

```go
// ErrNotSavedPagesFeed 合成フィード以外の記事に削除を要求されたときに返すエラーです。
// 購読フィードの記事はポーリングで再取得され得るため、削除は合成フィードに限ります。
var ErrNotSavedPagesFeed = errors.New("not a saved pages feed")

// DeleteItem 合成フィードから指定記事を削除します。
// 合成フィード以外のフィードIDを渡された場合はErrNotSavedPagesFeedを返し、何も変更しません。
// 対象記事が見つからない場合はErrItemNotFoundを返します。
func (s *ItemService) DeleteItem(feedID, itemID string) error {
	if !domain.IsSavedPagesFeed(feedID) {
		return ErrNotSavedPagesFeed
	}
	items, err := s.deps.Repo.Items(feedID)
	if err != nil {
		return fmt.Errorf("failed to load items for feed %s: %w", feedID, err)
	}
	kept := make([]domain.Item, 0, len(items))
	found := false
	for _, item := range items {
		if item.ID == itemID {
			found = true
			continue
		}
		kept = append(kept, item)
	}
	if !found {
		return ErrItemNotFound
	}
	if err := s.deps.Repo.SaveItems(feedID, kept); err != nil {
		return fmt.Errorf("failed to save items for feed %s: %w", feedID, err)
	}
	return nil
}
```

`internal/port/service.go` の `ItemService` インターフェースに1行足す。

```go
	// DeleteItem 合成フィードから指定記事を削除します。合成フィード以外のフィードIDは拒否します。
	DeleteItem(feedID, itemID string) error
```

- [ ] Step 4: テストが通ることを確認する

Run: `go test ./internal/service/ ./internal/port/ -run DeleteItem -v && go build ./...`
Expected: PASS、ビルド成功

- [ ] Step 5: コミットする

```bash
git add internal/service/item.go internal/service/item_delete_test.go internal/port/service.go
git commit -m "feat(service): 合成フィードの記事を削除するDeleteItemを追加"
```

---

## Task 5: BookmarkService.AddURL

### Files

- Modify: `internal/service/bookmark.go`
- Modify: `internal/port/service.go`
- Test: `internal/service/bookmark_addurl_test.go`

### Interfaces

- Consumes: `domain.SavedPagesFeedID`、`domain.SavedPagesFeedTitle`（Task 1）、`feed.ExtractMeta`（Task 2）、`normalizeURL`、`ErrInvalidURL`（Task 3）
- Produces: `(*BookmarkService).AddURL(ctx context.Context, rawURL, bookmarkID string) (domain.Item, error)`、`port.BookmarkService` に同メソッドを追加

- [ ] Step 1: 失敗するテストを書く

`internal/service/bookmark_addurl_test.go`

```go
package service_test

import (
	"context"
	"errors"
	"testing"

	"github.com/okamyuji/feedflow-go-htmx/internal/domain"
	"github.com/okamyuji/feedflow-go-htmx/internal/port"
	"github.com/okamyuji/feedflow-go-htmx/internal/service"
)

// htmlFetcher 固定のHTMLを返すフェイクフェッチャです。
type htmlFetcher struct {
	body        string
	contentType string
	err         error
	calls       int
}

func (f *htmlFetcher) Fetch(_ context.Context, _ port.FetchRequest) (port.FetchResult, error) {
	f.calls++
	if f.err != nil {
		return port.FetchResult{}, f.err
	}
	ct := f.contentType
	if ct == "" {
		ct = "text/html; charset=utf-8"
	}
	return port.FetchResult{StatusCode: 200, Body: []byte(f.body), ContentType: ct}, nil
}

func newBookmarkServiceForTest(repo *fakeRepo, fetcher port.Fetcher) *service.BookmarkService {
	deps := service.Deps{Repo: repo, Fetch: fetcher, Clock: newFakeClock(), IDs: newFakeIDGen()}
	items := service.NewItemService(deps, service.NewMuteService(deps))
	return service.NewBookmarkService(deps, items)
}

func TestAddURLCreatesSavedPagesFeedAndItem(t *testing.T) {
	repo := newFakeRepo()
	fetcher := &htmlFetcher{body: `<html><head><title>記事タイトル</title></head></html>`}
	svc := newBookmarkServiceForTest(repo, fetcher)

	it, err := svc.AddURL(context.Background(), "https://example.com/a/", "")
	if err != nil {
		t.Fatalf("AddURL returned error: %v", err)
	}
	if it.FeedID != domain.SavedPagesFeedID {
		t.Errorf("FeedID = %q, want %q", it.FeedID, domain.SavedPagesFeedID)
	}
	if it.Title != "記事タイトル" {
		t.Errorf("Title = %q, want %q", it.Title, "記事タイトル")
	}
	if it.Link != "https://example.com/a" {
		t.Errorf("Link = %q, want normalized url", it.Link)
	}
	if !it.Bookmarked {
		t.Error("Bookmarked = false, want true")
	}
	if len(it.BookmarkIDs) != 0 {
		t.Errorf("BookmarkIDs = %v, want empty", it.BookmarkIDs)
	}

	f, ferr := repo.Feed(domain.SavedPagesFeedID)
	if ferr != nil {
		t.Fatalf("saved pages feed was not created: %v", ferr)
	}
	if f.Title != domain.SavedPagesFeedTitle {
		t.Errorf("feed title = %q, want %q", f.Title, domain.SavedPagesFeedTitle)
	}
	if f.PollInterval != domain.PollManualOnly {
		t.Errorf("feed poll interval = %v, want PollManualOnly", f.PollInterval)
	}
}

func TestAddURLAssignsBookmarkLabel(t *testing.T) {
	repo := newFakeRepo()
	repo.bookmarks = []domain.Bookmark{{ID: "b1", Name: "あとで"}}
	svc := newBookmarkServiceForTest(repo, &htmlFetcher{body: `<html><head><title>t</title></head></html>`})

	it, err := svc.AddURL(context.Background(), "https://example.com/a", "b1")
	if err != nil {
		t.Fatalf("AddURL returned error: %v", err)
	}
	if len(it.BookmarkIDs) != 1 || it.BookmarkIDs[0] != "b1" {
		t.Errorf("BookmarkIDs = %v, want [b1]", it.BookmarkIDs)
	}
}

func TestAddURLSecondTimeReusesExistingItem(t *testing.T) {
	repo := newFakeRepo()
	repo.bookmarks = []domain.Bookmark{{ID: "b1", Name: "あとで"}}
	svc := newBookmarkServiceForTest(repo, &htmlFetcher{body: `<html><head><title>t</title></head></html>`})

	if _, err := svc.AddURL(context.Background(), "https://example.com/a", ""); err != nil {
		t.Fatalf("first AddURL returned error: %v", err)
	}
	it, err := svc.AddURL(context.Background(), "https://example.com/a/", "b1")
	if err != nil {
		t.Fatalf("second AddURL returned error: %v", err)
	}
	if got := len(repo.items[domain.SavedPagesFeedID]); got != 1 {
		t.Errorf("item count = %d, want 1", got)
	}
	if len(it.BookmarkIDs) != 1 || it.BookmarkIDs[0] != "b1" {
		t.Errorf("BookmarkIDs = %v, want [b1]", it.BookmarkIDs)
	}
}

func TestAddURLReusesExistingSubscribedItem(t *testing.T) {
	repo := newFakeRepo()
	repo.feeds = []domain.Feed{{ID: "f1", FeedURL: "https://example.com/feed"}}
	repo.items["f1"] = []domain.Item{{ID: "i1", FeedID: "f1", Link: "https://example.com/a"}}
	repo.bookmarks = []domain.Bookmark{{ID: "b1", Name: "あとで"}}
	fetcher := &htmlFetcher{body: `<html><head><title>t</title></head></html>`}
	svc := newBookmarkServiceForTest(repo, fetcher)

	it, err := svc.AddURL(context.Background(), "https://example.com/a", "b1")
	if err != nil {
		t.Fatalf("AddURL returned error: %v", err)
	}
	if it.FeedID != "f1" || it.ID != "i1" {
		t.Errorf("returned item = %+v, want the existing f1/i1", it)
	}
	if !it.Bookmarked {
		t.Error("existing item was not marked bookmarked")
	}
	if _, err := repo.Feed(domain.SavedPagesFeedID); err == nil {
		t.Error("saved pages feed was created even though an existing item matched")
	}
	if fetcher.calls != 0 {
		t.Errorf("fetcher was called %d times, want 0 for an existing item", fetcher.calls)
	}
}

func TestAddURLFallsBackToURLWhenTitleUnavailable(t *testing.T) {
	tests := []struct {
		name    string
		fetcher *htmlFetcher
	}{
		{name: "取得に失敗", fetcher: &htmlFetcher{err: errors.New("network down")}},
		{name: "HTML以外", fetcher: &htmlFetcher{body: "%PDF-1.7", contentType: "application/pdf"}},
		{name: "タイトルが空", fetcher: &htmlFetcher{body: `<html><head></head><body>x</body></html>`}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := newFakeRepo()
			svc := newBookmarkServiceForTest(repo, tt.fetcher)
			it, err := svc.AddURL(context.Background(), "https://example.com/a", "")
			if err != nil {
				t.Fatalf("AddURL returned error: %v", err)
			}
			if it.Title != "https://example.com/a" {
				t.Errorf("Title = %q, want the input url", it.Title)
			}
		})
	}
}

func TestAddURLRejectsInvalidInput(t *testing.T) {
	tests := []struct {
		name       string
		rawURL     string
		bookmarkID string
		wantErr    error
	}{
		{name: "javascriptスキーム", rawURL: "javascript:alert(1)", wantErr: service.ErrInvalidURL},
		{name: "fileスキーム", rawURL: "file:///etc/passwd", wantErr: service.ErrInvalidURL},
		{name: "空文字", rawURL: "", wantErr: service.ErrInvalidURL},
		{name: "host無し", rawURL: "http:///a", wantErr: service.ErrInvalidURL},
		{name: "存在しないラベル", rawURL: "https://example.com/a", bookmarkID: "nope", wantErr: service.ErrBookmarkNotFound},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := newFakeRepo()
			svc := newBookmarkServiceForTest(repo, &htmlFetcher{body: "<html></html>"})
			if _, err := svc.AddURL(context.Background(), tt.rawURL, tt.bookmarkID); !errors.Is(err, tt.wantErr) {
				t.Errorf("AddURL error = %v, want %v", err, tt.wantErr)
			}
			if len(repo.items[domain.SavedPagesFeedID]) != 0 {
				t.Error("an item was created despite the rejection")
			}
		})
	}
}
```

- [ ] Step 2: テストが失敗することを確認する

Run: `go test ./internal/service/ -run AddURL -v`
Expected: FAIL（`svc.AddURL undefined`）

- [ ] Step 3: 最小の実装を書く

`internal/service/bookmark.go` に追加する。インポート宣言へ `context`、`log/slog`、`mime`、`time`、`internal/feed`、`internal/port` を足す。

```go
// addURLFetchTimeout 保存対象ページのタイトル取得に使う制限時間です。
// HTTPサーバのWriteTimeout(30秒)より短くし、手動取得(20秒)と揃えます。
const addURLFetchTimeout = 20 * time.Second

// AddURL 任意のURLをブックマークに追加し、保存された記事を返します。
// 既に同じURLの記事が購読フィードにあればその記事を保存済みにします。
// 無ければ合成フィードに新しい記事を作ります。
// bookmarkIDが空でなければ、その名称コレクションにも所属させます。
// タイトルの取得に失敗しても保存は成功させ、タイトルには入力URLを使います。
func (s *BookmarkService) AddURL(ctx context.Context, rawURL, bookmarkID string) (domain.Item, error) {
	normalized, err := normalizeURL(rawURL)
	if err != nil {
		return domain.Item{}, err
	}
	if bookmarkID != "" {
		if err := s.requireBookmark(bookmarkID); err != nil {
			return domain.Item{}, err
		}
	}
	if it, found, err := s.bookmarkExistingItem(normalized, bookmarkID); err != nil {
		return domain.Item{}, err
	} else if found {
		return it, nil
	}
	if err := s.ensureSavedPagesFeed(); err != nil {
		return domain.Item{}, err
	}
	return s.appendSavedPage(ctx, normalized, bookmarkID)
}

// requireBookmark 指定IDのラベルが存在することを確かめます。
func (s *BookmarkService) requireBookmark(bookmarkID string) error {
	bms, err := s.deps.Repo.Bookmarks()
	if err != nil {
		return fmt.Errorf("failed to load bookmarks: %w", err)
	}
	for _, b := range bms {
		if b.ID == bookmarkID {
			return nil
		}
	}
	return fmt.Errorf("bookmark %q: %w", bookmarkID, ErrBookmarkNotFound)
}

// bookmarkExistingItem 正規化URLに一致する既存記事を探し、見つかれば保存済みにして返します。
// 合成フィードの記事も探索対象に含むため、同じURLを二度追加しても記事は増えません。
func (s *BookmarkService) bookmarkExistingItem(normalized, bookmarkID string) (domain.Item, bool, error) {
	feeds, err := s.deps.Repo.Feeds()
	if err != nil {
		return domain.Item{}, false, fmt.Errorf("failed to load feeds: %w", err)
	}
	for _, f := range feeds {
		items, err := s.deps.Repo.Items(f.ID)
		if err != nil {
			return domain.Item{}, false, fmt.Errorf("failed to load items for feed %s: %w", f.ID, err)
		}
		for _, it := range items {
			if !sameNormalizedURL(it.Link, normalized) {
				continue
			}
			updated, err := s.markSaved(f.ID, it.ID, bookmarkID)
			if err != nil {
				return domain.Item{}, false, err
			}
			return updated, true, nil
		}
	}
	return domain.Item{}, false, nil
}

// sameNormalizedURL 記事のリンクが対象の正規化URLと同じページを指すかどうかを返します。
// 正規化できないリンクは一致しないものとして扱います。
func sameNormalizedURL(link, normalized string) bool {
	got, err := normalizeURL(link)
	if err != nil {
		return false
	}
	return got == normalized
}

// markSaved 指定記事を保存済みにし、ラベル指定があれば所属させて、更新後の記事を返します。
func (s *BookmarkService) markSaved(feedID, itemID, bookmarkID string) (domain.Item, error) {
	if err := s.items.SetBookmarked(feedID, itemID, true); err != nil {
		return domain.Item{}, err
	}
	if bookmarkID != "" {
		if err := s.items.addBookmark(feedID, itemID, bookmarkID); err != nil {
			return domain.Item{}, err
		}
	}
	items, err := s.deps.Repo.Items(feedID)
	if err != nil {
		return domain.Item{}, fmt.Errorf("failed to load items for feed %s: %w", feedID, err)
	}
	for _, it := range items {
		if it.ID == itemID {
			return it, nil
		}
	}
	return domain.Item{}, ErrItemNotFound
}

// ensureSavedPagesFeed 合成フィードが無ければ作成します。
// 購読フィードではないため、ポーリング間隔は手動のみにします。
func (s *BookmarkService) ensureSavedPagesFeed() error {
	if _, err := s.deps.Repo.Feed(domain.SavedPagesFeedID); err == nil {
		return nil
	}
	f := domain.Feed{
		ID:           domain.SavedPagesFeedID,
		Title:        domain.SavedPagesFeedTitle,
		PollInterval: domain.PollManualOnly,
	}
	if err := s.deps.Repo.SaveFeed(f); err != nil {
		return fmt.Errorf("failed to create saved pages feed: %w", err)
	}
	return nil
}

// appendSavedPage 合成フィードの先頭に保存ページの記事を1件積みます。
func (s *BookmarkService) appendSavedPage(ctx context.Context, normalized, bookmarkID string) (domain.Item, error) {
	now := s.deps.Clock.Now()
	item := domain.Item{
		ID:          s.deps.IDs.NewID(),
		FeedID:      domain.SavedPagesFeedID,
		GUID:        normalized,
		Title:       s.fetchTitle(ctx, normalized),
		Link:        normalized,
		PublishedAt: now,
		FetchedAt:   now,
		Bookmarked:  true,
	}
	if bookmarkID != "" {
		item.BookmarkIDs = []string{bookmarkID}
	}
	existing, err := s.deps.Repo.Items(domain.SavedPagesFeedID)
	if err != nil {
		return domain.Item{}, fmt.Errorf("failed to load saved pages items: %w", err)
	}
	next := append([]domain.Item{item}, existing...)
	if err := s.deps.Repo.SaveItems(domain.SavedPagesFeedID, next); err != nil {
		return domain.Item{}, fmt.Errorf("failed to save saved pages items: %w", err)
	}
	return item, nil
}

// fetchTitle 対象ページを取得してタイトルを返します。
// 取得できない場合やHTMLでない場合やタイトルが空の場合はURLをそのまま返します。
// 保存自体は成功させたいため、エラーは返さずログにだけ残します。
func (s *BookmarkService) fetchTitle(ctx context.Context, pageURL string) string {
	ctx, cancel := context.WithTimeout(ctx, addURLFetchTimeout)
	defer cancel()
	res, err := s.deps.Fetch.Fetch(ctx, port.FetchRequest{URL: pageURL})
	if err != nil {
		slog.Warn("failed to fetch page for title", "url", pageURL, "error", err)
		return pageURL
	}
	if !isHTMLContentType(res.ContentType) {
		return pageURL
	}
	if title := feed.ExtractMeta(res.Body, res.ContentType).Title; title != "" {
		return title
	}
	return pageURL
}

// isHTMLContentType Content-TypeがHTMLを表すかどうかを返します。
// パラメータ付き(text/html; charset=utf-8)にも対応します。
func isHTMLContentType(contentType string) bool {
	mediaType, _, err := mime.ParseMediaType(contentType)
	if err != nil {
		return false
	}
	return mediaType == "text/html" || mediaType == "application/xhtml+xml"
}
```

使わなくなったパッケージがあればインポート宣言から外す。

`internal/port/service.go` の `BookmarkService` インターフェースに1行足す。ファイル先頭のインポート宣言へ `context` を足す。

```go
	// AddURL 任意のURLをブックマークに追加し、保存された記事を返します。
	// 既存の同一URL記事があればそれを保存済みにし、無ければ合成フィードへ新規作成します。
	AddURL(ctx context.Context, rawURL, bookmarkID string) (domain.Item, error)
```

- [ ] Step 4: テストが通ることを確認する

Run: `go test ./internal/service/ -run AddURL -v && go build ./...`
Expected: PASS、ビルド成功

- [ ] Step 5: コミットする

```bash
git add internal/service/bookmark.go internal/service/bookmark_addurl_test.go internal/port/service.go
git commit -m "feat(service): 任意URLをブックマークへ追加するAddURLを実装"
```

---

## Task 6: 合成フィードをポーリングとOPMLと左ツリーから除外

### Files

- Modify: `internal/poller/service.go`
- Modify: `internal/poller/runner.go`
- Modify: `internal/service/opml.go`
- Modify: `internal/handler/feed_handler.go`
- Test: `internal/poller/saved_skip_test.go`
- Test: 既存の `internal/handler/bookmark_tree_test.go` にケースを追加

### Interfaces

- Consumes: `domain.IsSavedPagesFeed`（Task 1）

- [ ] Step 1: 失敗するテストを書く

`internal/poller/saved_skip_test.go`

既存の `internal/poller/pollall_test.go` のフェイク（リポジトリ、フェッチャ、パーサ、クロック、IDGen、ミュート）を再利用する。型名と構築方法はそのファイルを読んで合わせること。

```go
package poller_test

import (
	"context"
	"testing"

	"github.com/okamyuji/feedflow-go-htmx/internal/domain"
)

func TestPollFeedSkipsSavedPagesFeed(t *testing.T) {
	repo := newFakeRepo()
	repo.feeds = []domain.Feed{{ID: domain.SavedPagesFeedID, Title: domain.SavedPagesFeedTitle}}
	fetcher := newFakeFetcher()
	svc := newPollServiceForTest(repo, fetcher)

	added, err := svc.PollFeed(context.Background(), domain.SavedPagesFeedID)
	if err != nil {
		t.Fatalf("PollFeed returned error: %v", err)
	}
	if added != 0 {
		t.Errorf("added = %d, want 0", added)
	}
	if fetcher.calls != 0 {
		t.Errorf("fetcher called %d times, want 0", fetcher.calls)
	}
	f, _ := repo.Feed(domain.SavedPagesFeedID)
	if f.ConsecutiveErrors != 0 {
		t.Errorf("ConsecutiveErrors = %d, want 0", f.ConsecutiveErrors)
	}
}

func TestPollAllNowSkipsSavedPagesFeed(t *testing.T) {
	repo := newFakeRepo()
	repo.feeds = []domain.Feed{
		{ID: domain.SavedPagesFeedID, Title: domain.SavedPagesFeedTitle},
		{ID: "f1", FeedURL: "https://example.com/feed"},
	}
	fetcher := newFakeFetcher()
	svc := newPollServiceForTest(repo, fetcher)

	processed, err := svc.PollAllNow(context.Background())
	if err != nil {
		t.Fatalf("PollAllNow returned error: %v", err)
	}
	if processed != 1 {
		t.Errorf("processed = %d, want 1", processed)
	}
}
```

`newPollServiceForTest` と `newFakeFetcher` が既存に無ければ、既存テストの構築コードに合わせてヘルパを作る。`fetcher.calls` に相当する呼び出し回数カウンタが既存フェイクに無い場合は追加する。

`internal/handler/bookmark_tree_test.go` に次のケースを追加する。既存テストのハンドラ構築ヘルパに合わせること。

```go
func TestBuildTreeExcludesSavedPagesFeed(t *testing.T) {
	// 合成フィードと通常フィードを1件ずつ持つ状態でツリーを描画し、
	// 左ペインに合成フィードのノードが出ないことを確かめます。
	h := newHandlerWithFeeds(t, []domain.Feed{
		{ID: domain.SavedPagesFeedID, Title: domain.SavedPagesFeedTitle},
		{ID: "f1", Title: "通常フィード"},
	})
	nodes, err := h.BuildTreeForTest()
	if err != nil {
		t.Fatalf("buildTree returned error: %v", err)
	}
	for _, n := range nodes {
		if n.Kind == "feed" && n.ID == domain.SavedPagesFeedID {
			t.Fatal("saved pages feed appeared in the tree")
		}
	}
}
```

`buildTree` は非公開のため、テストは `package handler`（内部テスト）で書き、`h.buildTree()` を直接呼ぶ。既存の `bookmark_tree_test.go` のパッケージ宣言に合わせること。その場合 `BuildTreeForTest` は不要で `h.buildTree()` を呼ぶ。

- [ ] Step 2: テストが失敗することを確認する

Run: `go test ./internal/poller/ ./internal/handler/ -run 'SavedPages' -v`
Expected: FAIL

- [ ] Step 3: 最小の実装を書く

`internal/poller/service.go` の `PollFeed` の先頭に早期returnを足す。docコメントも更新する。

```go
// PollFeed 指定フィードを取得し新着記事を反映して新着件数を返します。
// 取得に失敗した場合は連続エラー数を1増やして保存し、エラーを返します。
// サーバが未更新を示した場合は記事を増やさず最終取得時刻だけ更新します。
// 合成フィードは取得元URLを持たないため、何もせず0件を返します。
func (s *Service) PollFeed(ctx context.Context, feedID string) (int, error) {
	if domain.IsSavedPagesFeed(feedID) {
		return 0, nil
	}
	feed, err := s.repo.Feed(feedID)
	...
```

`PollAll` と `PollAllNow` の対象ID収集ループに条件を足す。

```go
	for _, feed := range feeds {
		if domain.IsSavedPagesFeed(feed.ID) {
			continue
		}
		if dueForPollWithJitter(feed, settings, now, s.jitter) {
			ids = append(ids, feed.ID)
		}
	}
```

```go
	for _, feed := range feeds {
		if domain.IsSavedPagesFeed(feed.ID) {
			continue
		}
		ids = append(ids, feed.ID)
	}
```

`internal/poller/runner.go` の `dueFeedIDs` のループにも同じ条件を足す。実際のループ本体を読み、`domain.IsSavedPagesFeed(feed.ID)` なら `continue` する1行を先頭に置く。`domain` パッケージが未importなら足す。

`internal/service/opml.go` の `Export` のループに条件を足す。

```go
	for _, f := range feeds {
		// 合成フィードは取得元URLを持たないため、OPMLに出すと他のリーダーで壊れます。
		if domain.IsSavedPagesFeed(f.ID) {
			continue
		}
		outlines = append(outlines, opmlOutline{
```

`internal/handler/feed_handler.go` の `buildTree` で `orderFeedNodes` へ渡す前にフィードを絞る。

```go
	// 合成フィードは購読フィードではないため、左ペインには出しません。
	subscribed := make([]domain.Feed, 0, len(feeds))
	for _, f := range feeds {
		if domain.IsSavedPagesFeed(f.ID) {
			continue
		}
		subscribed = append(subscribed, f)
	}
	feedNodes := orderFeedNodes(subscribed, unreadByFeed, h.feedSortSettings())
```

- [ ] Step 4: テストが通ることを確認する

Run: `go test ./... `
Expected: PASS

- [ ] Step 5: コミットする

```bash
git add internal/poller/ internal/service/opml.go internal/handler/feed_handler.go
git commit -m "feat: 合成フィードをポーリングとOPML出力と左ツリーから除外"
```

---

## Task 7: 解除で合成フィードの記事を削除

### Files

- Modify: `internal/handler/item_handler.go:419-433`
- Test: `internal/handler/bookmark_save_test.go` にケースを追加

### Interfaces

- Consumes: `domain.IsSavedPagesFeed`（Task 1）、`port.ItemService.DeleteItem`（Task 4）

- [ ] Step 1: 失敗するテストを書く

`internal/handler/bookmark_save_test.go` に追加する。既存テストのハンドラ構築とフェイクに合わせること。

```go
func TestItemBookmarkDeletesSavedPageOnUnsave(t *testing.T) {
	// 合成フィードの記事を解除すると、保存状態の更新ではなく記事そのものが消えます。
	h, deps := newHandlerForBookmarkTest(t)
	deps.repo.feeds = []domain.Feed{{ID: domain.SavedPagesFeedID, Title: domain.SavedPagesFeedTitle}}
	deps.repo.items[domain.SavedPagesFeedID] = []domain.Item{
		{ID: "i1", FeedID: domain.SavedPagesFeedID, Link: "https://example.com/a", Bookmarked: true},
	}

	req := newBookmarkPostRequest(t, "/app/items/"+domain.SavedPagesFeedID+"/i1/bookmark", "bookmarked=false")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if got := len(deps.repo.items[domain.SavedPagesFeedID]); got != 0 {
		t.Errorf("item count = %d, want 0 (the saved page should be deleted)", got)
	}
	if !strings.Contains(rec.Body.String(), `hx-swap-oob="delete"`) {
		t.Error("response did not contain the oob delete fragment")
	}
}

func TestItemBookmarkKeepsSubscribedItemOnUnsave(t *testing.T) {
	// 購読フィードの記事は解除しても消えません。保存状態だけがオフになります。
	h, deps := newHandlerForBookmarkTest(t)
	deps.repo.feeds = []domain.Feed{{ID: "f1", FeedURL: "https://example.com/feed"}}
	deps.repo.items["f1"] = []domain.Item{
		{ID: "i1", FeedID: "f1", Link: "https://example.com/a", Bookmarked: true},
	}

	req := newBookmarkPostRequest(t, "/app/items/f1/i1/bookmark", "bookmarked=false")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	items := deps.repo.items["f1"]
	if len(items) != 1 {
		t.Fatalf("item count = %d, want 1", len(items))
	}
	if items[0].Bookmarked {
		t.Error("Bookmarked = true, want false")
	}
}
```

`newHandlerForBookmarkTest` と `newBookmarkPostRequest` は既存テストのヘルパ名に合わせる。存在しなければ既存テストの構築コードをそのまま使う。POSTには認証セッションとCSRFトークンが要る点に注意する。

- [ ] Step 2: テストが失敗することを確認する

Run: `go test ./internal/handler/ -run 'ItemBookmark.*SavedPage|ItemBookmarkKeepsSubscribed' -v`
Expected: FAIL（記事が消えない）

- [ ] Step 3: 最小の実装を書く

`internal/handler/item_handler.go` の `itemBookmark` を書き換える。

```go
// itemBookmark 記事の保存(ブックマーク)状態を設定します。
// 保存の付け外しはブックマークボタンのピッカー1か所で行うため、応答はピッカーの再描画に一本化します。
// ブックマークビューで解除した場合は、ピッカー更新に加えて当該記事カードを一覧から取り除きます
// (記事カード内の解除ボタンを廃し、ブックマークボタンの解除でその挙動を代用するため)。
// 合成フィードの記事を解除した場合は記事そのものを削除します。
// 保存状態を落として残すと、未読ストリームに出所不明のカードとして現れてしまうためです。
// この場合はビューに関わらず一覧から取り除きます。
func (h *Handler) itemBookmark(w http.ResponseWriter, r *http.Request) {
	feedID := r.PathValue("feedID")
	itemID := r.PathValue("itemID")
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	bookmarked := r.FormValue("bookmarked") == "true"
	if !bookmarked && domain.IsSavedPagesFeed(feedID) {
		if err := h.deps.Items.DeleteItem(feedID, itemID); err != nil {
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		h.renderBookmarkPicker(w, r, feedID, itemID, true, true)
		return
	}
	if err := h.deps.Items.SetBookmarked(feedID, itemID, bookmarked); err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	removeFromList := !bookmarked && isBookmarkViewURL(r)
	h.renderBookmarkPicker(w, r, feedID, itemID, removeFromList, true)
}
```

- [ ] Step 4: テストが通ることを確認する

Run: `go test ./internal/handler/ -v && go build ./...`
Expected: PASS

- [ ] Step 5: コミットする

```bash
git add internal/handler/item_handler.go internal/handler/bookmark_save_test.go
git commit -m "feat(handler): 保存ページの解除で記事そのものを削除"
```

---

## Task 8: 追加フォームのハンドラとルート

### Files

- Modify: `internal/handler/bookmark_handler.go`
- Modify: `internal/handler/router.go:122付近`
- Modify: `internal/handler/render.go`（`pageData` にフィールド追加）
- Modify: `internal/handler/item_handler.go`（`itemList` でフィールドを埋める）
- Test: `internal/handler/bookmark_addurl_handler_test.go`

### Interfaces

- Consumes: `port.BookmarkService.AddURL`（Task 5）、`service.ErrInvalidURL`、`service.ErrBookmarkNotFound`
- Produces: `pageData.ShowAddURL bool`、`pageData.AddURLPostURL string`、`pageData.AddURLError string`、`pageData.BookmarkOptions []bookmarkOption`

- [ ] Step 1: 失敗するテストを書く

`internal/handler/bookmark_addurl_handler_test.go`

```go
package handler_test

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/okamyuji/feedflow-go-htmx/internal/domain"
)

func TestBookmarkAddURLCreatesItemAndRendersList(t *testing.T) {
	h, deps := newHandlerForBookmarkTest(t)
	deps.fetcher.body = `<html><head><title>保存したページのタイトル</title></head></html>`

	form := url.Values{"url": {"https://example.com/a"}}
	req := newBookmarkPostRequest(t, "/app/bookmarks/add-url?view=bookmark", form.Encode())
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "保存したページのタイトル") {
		t.Error("response did not contain the saved page title")
	}
	if got := len(deps.repo.items[domain.SavedPagesFeedID]); got != 1 {
		t.Errorf("item count = %d, want 1", got)
	}
}

func TestBookmarkAddURLShowsErrorForInvalidURL(t *testing.T) {
	h, deps := newHandlerForBookmarkTest(t)

	form := url.Values{"url": {"javascript:alert(1)"}}
	req := newBookmarkPostRequest(t, "/app/bookmarks/add-url?view=bookmark", form.Encode())
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "URLの形式が正しくありません") {
		t.Errorf("response did not contain the error message: %s", rec.Body.String())
	}
	if got := len(deps.repo.items[domain.SavedPagesFeedID]); got != 0 {
		t.Errorf("item count = %d, want 0", got)
	}
}

func TestBookmarkAddURLShowsErrorForUnknownLabel(t *testing.T) {
	h, _ := newHandlerForBookmarkTest(t)

	form := url.Values{"url": {"https://example.com/a"}, "bookmark_id": {"nope"}}
	req := newBookmarkPostRequest(t, "/app/bookmarks/add-url?view=bookmark", form.Encode())
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if !strings.Contains(rec.Body.String(), "指定のラベルが見つかりません") {
		t.Errorf("response did not contain the label error: %s", rec.Body.String())
	}
}

func TestBookmarkAddURLRequiresCSRF(t *testing.T) {
	h, _ := newHandlerForBookmarkTest(t)

	form := url.Values{"url": {"https://example.com/a"}}
	req := httptest.NewRequest(http.MethodPost, "/app/bookmarks/add-url", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	attachSession(t, req) // CSRFトークンは付けません
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code == http.StatusOK {
		t.Errorf("status = 200, want a rejection for the missing csrf token")
	}
}

func TestBookmarkAddURLRequiresAuth(t *testing.T) {
	h, _ := newHandlerForBookmarkTest(t)

	form := url.Values{"url": {"https://example.com/a"}}
	req := httptest.NewRequest(http.MethodPost, "/app/bookmarks/add-url", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code == http.StatusOK {
		t.Errorf("status = 200, want a rejection for the unauthenticated request")
	}
}
```

`attachSession` は既存テストのセッション付与ヘルパ名に合わせる。既存の `newHandlerForBookmarkTest` にフェイクフェッチャが無ければ、テスト用Depsに追加する。

- [ ] Step 2: テストが失敗することを確認する

Run: `go test ./internal/handler/ -run BookmarkAddURL -v`
Expected: FAIL（404が返る）

- [ ] Step 3: 最小の実装を書く

`internal/handler/render.go` の `pageData` に4フィールドを足す。

```go
	ShowAddURL       bool             // ブックマークビューでURL追加フォームを出すかどうかです
	AddURLPostURL    string           // URL追加フォームの送信先です。現在の表示条件をクエリで引き継ぎます
	AddURLError      string           // URL追加に失敗したときに入力欄の上へ出す文言です
	BookmarkOptions  []bookmarkOption // URL追加フォームのラベル選択肢です
```

`internal/handler/bookmark_handler.go` に追加する。

```go
// addURLErrorMessage AddURLのエラーを画面に出す日本語の文言へ変換します。
// 想定外のエラーは内部事情を漏らさない一般的な文言にまとめます。
func addURLErrorMessage(err error) string {
	switch {
	case errors.Is(err, service.ErrInvalidURL):
		return "URLの形式が正しくありません。httpまたはhttpsで始まるURLを入力してください。"
	case errors.Is(err, service.ErrBookmarkNotFound):
		return "指定のラベルが見つかりません。画面を再読み込みしてからもう一度お試しください。"
	default:
		return "保存できませんでした。しばらくしてからもう一度お試しください。"
	}
}

// bookmarkAddURL 入力されたURLをブックマークへ追加し、現在の一覧を再描画します。
// 追加に失敗した場合もHTTP 200のまま、入力欄の上にエラー文言を出したフォームを返します。
// 一覧の描画条件は送信先クエリで引き継ぎ、追加前と同じ絞り込みを保ちます。
func (h *Handler) bookmarkAddURL(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	rawURL := r.FormValue("url")
	bookmarkID := r.FormValue("bookmark_id")
	if _, err := h.deps.Bookmarks.AddURL(r.Context(), rawURL, bookmarkID); err != nil {
		slog.Warn("failed to add url to bookmarks", "error", err)
		h.renderAddURLForm(w, r, addURLErrorMessage(err))
		return
	}
	h.itemList(w, r)
}

// renderAddURLForm URL追加フォームだけを描画して返します。エラー文言を伴う再描画に使います。
func (h *Handler) renderAddURLForm(w http.ResponseWriter, r *http.Request, message string) {
	sess := sessionFromContext(r.Context())
	data := pageData{
		CSRFToken:       sess.CSRFToken,
		ShowAddURL:      true,
		AddURLPostURL:   addURLPostURL(r),
		AddURLError:     message,
		BookmarkOptions: h.bookmarkOptions(),
	}
	h.renderPartial(w, http.StatusOK, "_bookmark_add_url.html", data)
}

// bookmarkOptions URL追加フォームのラベル選択肢を返します。
// 一覧の取得に失敗した場合は選択肢なしとして扱い、URLの追加自体は続けられるようにします。
func (h *Handler) bookmarkOptions() []bookmarkOption {
	bms, err := h.deps.Bookmarks.List()
	if err != nil {
		slog.Error("failed to load bookmarks for the add url form", "error", err)
		return nil
	}
	options := make([]bookmarkOption, 0, len(bms))
	for _, b := range bms {
		options = append(options, bookmarkOption{ID: b.ID, Name: b.Name})
	}
	return options
}

// addURLPostURL 現在の表示条件を保ったままURLを追加するPOST先を返します。
func addURLPostURL(r *http.Request) string {
	u := url.URL{Path: "/app/bookmarks/add-url", RawQuery: r.URL.RawQuery}
	return u.String()
}
```

インポート宣言へ `net/url` を足す。`renderPartial` のシグネチャは `internal/handler/render.go:143` を読んで合わせる。

`internal/handler/item_handler.go` の `itemList` で `pageData` を組む箇所に3行足す。

```go
		ShowAddURL:      isBookmarkListRequest(r),
		AddURLPostURL:   addURLPostURL(r),
		BookmarkOptions: h.bookmarkOptions(),
```

同ファイルに述語を足す。`isBookmarkViewURL` は `HX-Current-URL` を見るのに対し、こちらはリクエストURL自身のクエリを見る。

```go
// isBookmarkListRequest リクエスト自身のクエリがブックマーク記事の一覧を指すかどうかを返します。
// isBookmarkViewURLはHX-Current-URLを見るのに対し、こちらは描画対象のクエリを直接見ます。
func isBookmarkListRequest(r *http.Request) bool {
	q := r.URL.Query()
	return q.Get("view") == "bookmark" || q.Get("bookmark") != ""
}
```

`internal/handler/router.go` の状態変更ブロックに1行足す。既存の `POST /app/bookmarks` の隣に置く。

```go
	mux.HandleFunc("POST /app/bookmarks/add-url", h.requireAuth(h.requireCSRF(h.bookmarkAddURL)))
```

- [ ] Step 4: テストが通ることを確認する

Run: `go test ./internal/handler/ -v`
Expected: PASS

- [ ] Step 5: コミットする

```bash
git add internal/handler/
git commit -m "feat(handler): URL追加フォームのルートとハンドラを追加"
```

---

## Task 9: 追加フォームのテンプレートとスタイル

### Files

- Create: `internal/handler/templates/_bookmark_add_url.html`
- Modify: `internal/handler/templates/_item_list.html`
- Modify: `internal/handler/static/styles.css`
- Test: `internal/handler/bookmark_addurl_handler_test.go` にケースを追加

### Interfaces

- Consumes: `pageData.ShowAddURL`、`pageData.AddURLPostURL`、`pageData.AddURLError`、`pageData.BookmarkOptions`、`pageData.CSRFToken`（Task 8）

- [ ] Step 1: 失敗するテストを書く

`internal/handler/bookmark_addurl_handler_test.go` に追加する。

```go
func TestItemListShowsAddURLFormOnlyInBookmarkViews(t *testing.T) {
	tests := []struct {
		name  string
		query string
		want  bool
	}{
		{name: "ブックマークビュー", query: "?view=bookmark", want: true},
		{name: "ラベル絞り込み", query: "?bookmark=b1", want: true},
		{name: "すべて", query: "", want: false},
		{name: "既読", query: "?view=read", want: false},
		{name: "あとで読む", query: "?view=readlater", want: false},
		{name: "単一フィード", query: "?feed=f1", want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h, _ := newHandlerForBookmarkTest(t)
			req := newBookmarkGetRequest(t, "/app/items"+tt.query)
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, req)

			got := strings.Contains(rec.Body.String(), `class="add-url-form"`)
			if got != tt.want {
				t.Errorf("add url form present = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestAddURLFormListsExistingLabels(t *testing.T) {
	h, deps := newHandlerForBookmarkTest(t)
	deps.repo.bookmarks = []domain.Bookmark{{ID: "b1", Name: "あとで"}, {ID: "b2", Name: "資料"}}

	req := newBookmarkGetRequest(t, "/app/items?view=bookmark")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	body := rec.Body.String()
	for _, want := range []string{`value="b1"`, "あとで", `value="b2"`, "資料", "ラベルなし"} {
		if !strings.Contains(body, want) {
			t.Errorf("response did not contain %q", want)
		}
	}
}

func TestAddURLFormOmitsSelectWhenNoLabels(t *testing.T) {
	h, _ := newHandlerForBookmarkTest(t)

	req := newBookmarkGetRequest(t, "/app/items?view=bookmark")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if strings.Contains(rec.Body.String(), `name="bookmark_id"`) {
		t.Error("the label select was rendered even though no labels exist")
	}
}
```

`newBookmarkGetRequest` は既存のGET用ヘルパ名に合わせる。

- [ ] Step 2: テストが失敗することを確認する

Run: `go test ./internal/handler/ -run 'AddURLForm|ShowsAddURLForm' -v`
Expected: FAIL

- [ ] Step 3: 最小の実装を書く

`internal/handler/templates/_bookmark_add_url.html`

```html
{{ define "_bookmark_add_url.html" }}
<form
  class="add-url-form"
  hx-post="{{ .AddURLPostURL }}"
  hx-target="#main-pane"
  hx-swap="innerHTML"
  hx-disabled-elt="find button"
>
  <input type="hidden" name="csrf_token" value="{{ .CSRFToken }}">
  {{ if .AddURLError }}<p class="add-url-error" role="alert">{{ .AddURLError }}</p>{{ end }}
  <div class="add-url-row">
    <input
      class="add-url-input"
      type="url"
      name="url"
      required
      placeholder="https://example.com/article"
      aria-label="保存するURL"
    >
    {{ if .BookmarkOptions }}
    <select class="add-url-select" name="bookmark_id" aria-label="追加先のラベル">
      <option value="">ラベルなし</option>
      {{ range .BookmarkOptions }}<option value="{{ .ID }}">{{ .Name }}</option>{{ end }}
    </select>
    {{ end }}
    <button class="btn-ghost" type="submit">追加</button>
  </div>
</form>
{{ end }}
```

`internal/handler/templates/_item_list.html` の `item-list-bar` の直後、`{{ if not .Items }}` の前に差し込む。

```html
  {{ if .ShowAddURL }}{{ template "_bookmark_add_url.html" . }}{{ end }}
```

`internal/handler/static/styles.css` の末尾に足す。既存のカスタムプロパティ名（色、余白）はファイル冒頭を読んで合わせること。

```css
/* 任意URLの保存フォーム。ブックマークビューの一覧バー直下に置きます。 */
.add-url-form {
  padding: 0.5rem 0.75rem;
}

.add-url-row {
  display: flex;
  flex-wrap: wrap;
  gap: 0.5rem;
  align-items: center;
}

.add-url-input {
  flex: 1 1 18rem;
  min-width: 0;
  padding: 0.35rem 0.5rem;
}

.add-url-select {
  flex: 0 0 auto;
  padding: 0.35rem 0.5rem;
}

.add-url-error {
  margin: 0 0 0.5rem;
  font-size: 0.875rem;
}

@media (max-width: 640px) {
  .add-url-input,
  .add-url-select {
    flex: 1 1 100%;
  }
}
```

- [ ] Step 4: テストが通ることを確認する

Run: `go test ./internal/handler/ -v`
Expected: PASS

- [ ] Step 5: コミットする

```bash
git add internal/handler/templates/ internal/handler/static/styles.css internal/handler/bookmark_addurl_handler_test.go
git commit -m "feat(ui): ブックマークビューにURL追加フォームを追加"
```

---

## Task 10: E2Eとドキュメントと品質ゲート

### Files

- Create: `e2e/playwright/tests/bookmark-add-url.spec.ts`
- Modify: `README.md`

### Interfaces

- Consumes: 全タスクの成果

- [ ] Step 1: E2Eテストを書く

`e2e/playwright/tests/bookmark-add-url.spec.ts`

既存の `e2e/playwright/tests/bookmark.spec.ts` と `tests/helpers.ts` を読み、`setupAndLogin` とローカルサーバ起動ヘルパ（`startFeedServer`）の使い方に合わせること。`startFeedServer` はフィードXMLを配信する用途だが、任意のHTMLを配信できるなら流用する。できない場合は同ファイル内に次のヘルパを足す。

```ts
import { createServer, type Server } from 'node:http';

/** 指定HTMLを1つのパスで配信するローカルサーバを起動し、そのURLを返します。 */
async function startPageServer(html: string): Promise<{ url: string; close: () => Promise<void> }> {
  const server: Server = createServer((_req, res) => {
    res.writeHead(200, { 'Content-Type': 'text/html; charset=utf-8' });
    res.end(html);
  });
  await new Promise<void>((resolve) => server.listen(0, '127.0.0.1', resolve));
  const address = server.address();
  if (address === null || typeof address === 'string') {
    throw new Error('failed to determine the page server address');
  }
  const url = `http://127.0.0.1:${address.port}/page`;
  return {
    url,
    close: () => new Promise<void>((resolve) => server.close(() => resolve())),
  };
}
```

テスト本体。

```ts
import { expect, test } from '@playwright/test';
import { setupAndLogin } from './helpers';

const PAGE_HTML = `<!doctype html><html lang="ja"><head>
<meta charset="utf-8">
<meta property="og:title" content="保存したページの見出し">
<title>タイトル要素</title>
</head><body><p>本文</p></body></html>`;

test.describe('任意URLのブックマーク追加', () => {
  test('URLを追加するとタイトル付きカードが一覧に現れる', async ({ page }) => {
    await setupAndLogin(page);
    const server = await startPageServer(PAGE_HTML);
    try {
      await page.goto('/app/items?view=bookmark');
      await page.locator('.add-url-input').fill(server.url);
      await page.locator('.add-url-form button[type=submit]').click();
      await expect(page.locator('.item-title', { hasText: '保存したページの見出し' })).toBeVisible();
    } finally {
      await server.close();
    }
  });

  test('ラベルを選んで追加するとそのラベルの絞り込みに現れる', async ({ page }) => {
    await setupAndLogin(page);
    const server = await startPageServer(PAGE_HTML);
    try {
      // 先にラベルなしで1件保存し、そのカードのピッカーからラベルを作ります。
      await page.goto('/app/items?view=bookmark');
      await page.locator('.add-url-input').fill(server.url);
      await page.locator('.add-url-form button[type=submit]').click();
      await expect(page.locator('.item-card')).toHaveCount(1);

      await page.locator('.item-card .bookmark-btn').first().click();
      await page.locator('.bookmark-panel input[name=name]').fill('資料');
      await page.locator('.bookmark-panel button[type=submit]').click();
      await expect(page.locator('.tree-node', { hasText: '資料' })).toBeVisible();

      // 別のURLをそのラベル指定で追加します。
      await page.goto('/app/items?view=bookmark');
      await page.locator('.add-url-input').fill(`${server.url}?v=2`);
      await page.locator('.add-url-select').selectOption({ label: '資料' });
      await page.locator('.add-url-form button[type=submit]').click();

      await page.locator('.tree-node', { hasText: '資料' }).click();
      await expect(page.locator('.item-card')).toHaveCount(2);
    } finally {
      await server.close();
    }
  });

  test('保存したページを解除すると一覧から消え、再表示しても現れない', async ({ page }) => {
    await setupAndLogin(page);
    const server = await startPageServer(PAGE_HTML);
    try {
      await page.goto('/app/items?view=bookmark');
      await page.locator('.add-url-input').fill(server.url);
      await page.locator('.add-url-form button[type=submit]').click();
      await expect(page.locator('.item-card')).toHaveCount(1);

      await page.locator('.item-card .bookmark-btn').first().click();
      await page.locator('.bookmark-panel button', { hasText: '解除' }).click();
      await expect(page.locator('.item-card')).toHaveCount(0);

      await page.goto('/app/items?view=bookmark');
      await expect(page.locator('.item-card')).toHaveCount(0);
    } finally {
      await server.close();
    }
  });

  test('不正なURLはエラー文言が出てカードは増えない', async ({ page }) => {
    await setupAndLogin(page);
    await page.goto('/app/items?view=bookmark');
    // type=urlのブラウザ検証を避けるため、値を直接書き換えてから送信します。
    await page.locator('.add-url-input').evaluate((el: HTMLInputElement) => {
      el.type = 'text';
      el.value = 'ftp://example.com/a';
    });
    await page.locator('.add-url-form button[type=submit]').click();
    await expect(page.locator('.add-url-error')).toContainText('URLの形式が正しくありません');
    await expect(page.locator('.item-card')).toHaveCount(0);
  });

  test('左ツリーに保存したページが購読フィードとして現れない', async ({ page }) => {
    await setupAndLogin(page);
    const server = await startPageServer(PAGE_HTML);
    try {
      await page.goto('/app/items?view=bookmark');
      await page.locator('.add-url-input').fill(server.url);
      await page.locator('.add-url-form button[type=submit]').click();
      await expect(page.locator('.item-card')).toHaveCount(1);

      await expect(page.locator('.tree-node', { hasText: '保存したページ' })).toHaveCount(0);
    } finally {
      await server.close();
    }
  });
});
```

セレクタ（`.tree-node`、解除ボタンの文言、ピッカーの入力名）は実際のテンプレートと既存E2Eを読んで合わせること。推測のまま書かない。

- [ ] Step 2: E2Eを実行する

Run:
```bash
lsof -ti tcp:8099 | xargs -r kill
cd e2e/playwright && npx playwright test bookmark-add-url.spec.ts
```
Expected: 5件すべてPASS

ポート8099に古いサーバが残っていると修正前のバイナリで検証され、全テストが嘘の失敗をする。先にkillすること。

- [ ] Step 3: READMEを更新する

`README.md` の機能一覧に1行足す。既存の書式に合わせること。

```markdown
- 任意のURLを入力してブックマークに保存（タイトルは元ページから自動取得）
```

- [ ] Step 4: 品質ゲートを通す

Run: `bash scripts/quality-gate.sh`
Expected: すべてのチェックがPASS

失敗したら該当箇所を直し、再実行する。`go fix -diff` が差分を出したらその修正を取り込む。

- [ ] Step 5: カバレッジとCRAPを確認する

Run:
```bash
go test ./internal/domain/ ./internal/feed/ ./internal/service/ ./internal/poller/ ./internal/handler/ -coverprofile=/tmp/cover.out
go tool cover -func=/tmp/cover.out | grep -E 'saved.go|meta.go|url.go|bookmark.go|item.go'
```
Expected: 追加した関数がいずれも80%以上

- [ ] Step 6: コミットする

```bash
git add e2e/playwright/tests/bookmark-add-url.spec.ts README.md
git commit -m "test(e2e): 任意URL追加のE2Eを追加しREADMEを更新"
```

---

## Self-Review

### 1. 仕様カバレッジ

| 設計書の節 | 実装するタスク |
|---|---|
| 1. ドメイン | Task 1 |
| 2. メタデータ取得 | Task 2 |
| 3. ポート | Task 4、Task 5 |
| 4. サービス（手順1から6、URL正規化） | Task 3、Task 5 |
| 5. 全列挙箇所からの除外 | Task 6 |
| 6. 解除時の記事削除 | Task 4、Task 7 |
| 7. ルーティングとハンドラ | Task 8 |
| 8. UI | Task 9 |
| 9. 記事カードの挙動 | 変更不要（Task 10のE2Eで確認） |
| テスト計画（ユニット） | Task 1から9の各Step 1 |
| テスト計画（E2E） | Task 10 |
| ドキュメント更新 | Task 10 |
| 品質ゲート | Task 10 |

### 2. 型と名前の整合

`domain.IsSavedPagesFeed`、`feed.ExtractMeta`、`feed.PageMeta`、`service.normalizeURL`、`service.ErrInvalidURL`、`service.ErrNotSavedPagesFeed`、`(*BookmarkService).AddURL`、`(*ItemService).DeleteItem`、`pageData.ShowAddURL`、`pageData.AddURLPostURL`、`pageData.AddURLError`、`pageData.BookmarkOptions` を全タスクで同じ綴りで使っている。

### 3. 実装者への注意

既存テストのフェイク名とヘルパ名は推測で書いてある箇所がある。実装時は必ず該当ファイルを読み、実在する名前に合わせること。存在しないヘルパは既存の構築コードに倣って作る。
