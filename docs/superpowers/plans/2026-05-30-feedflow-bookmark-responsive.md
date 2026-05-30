# feedflow ブックマーク・未読集約・既読先頭・レスポンシブ 実装計画

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** スターを廃止しブックマーク（名称付き・新規/既存選択）を導入、未読Feedを先頭集約、単一フィードで既読5件を先頭表示、モバイルのオフキャンバス・ドロワー対応を行う。

**Architecture:** handler → service → port → store（メモリ常駐＋JSON）。Board概念をBookmarkへ統合改名。所属は記事の`BookmarkIDs`に保持。フロントはHTMX＋Alpine＋html/template。

**Tech Stack:** Go 1.x, html/template, HTMX, Alpine.js, Playwright(E2E)。品質ゲート: `scripts/quality-gate.sh`。

---

## 設計参照
`docs/superpowers/specs/2026-05-30-feedflow-bookmark-responsive-design.md`

## ファイル構成（作成/変更）

- 変更: `internal/domain/item.go`（Starred削除, BoardIDs→BookmarkIDs, UnmarshalJSON, HasUserAction）
- 改名: `internal/domain/board.go`→`internal/domain/bookmark.go`（Board→Bookmark）
- 改名: `internal/store/board.go`→`internal/store/bookmark.go`、`internal/store/store.go`（boardsFile→bookmarksFile, マイグレーション）
- 変更: `internal/port/repository.go`, `internal/port/service.go`
- 変更: `internal/service/item.go`（Star削除, SetBoards→SetBookmarks）
- 作成: `internal/service/bookmark.go`（BookmarkService）
- 変更: `internal/service/service.go`（Deps配線）
- 改名: `internal/handler/board_handler.go`→`internal/handler/bookmark_handler.go`
- 変更: `internal/handler/item_handler.go`（filter: board→bookmark, read-head 5）
- 変更: `internal/handler/feed_handler.go`（buildTree: bookmark子ノード, 未読先頭集約, label）
- 変更: `internal/handler/render.go`（pageData/itemView/feedTreeNode 改名・拡張）
- 変更: `internal/handler/router.go`（star削除, bookmark追加）
- 作成: `internal/handler/templates/_bookmark_picker.html`
- 変更: `_item_card.html`, `_item_list.html`, `_overlay_actions.html`, `_tree.html`, `_tree_node.html`, `base.html`
- 変更: `internal/handler/static/app.js`, `internal/handler/static/styles.css`
- 変更: E2E `e2e/playwright/tests/read.spec.ts`、追加 spec
- 変更: `README.md` ほかドキュメント

---

## Phase 1: ドメインとストア

### Task 1: domain Item の改名と後方互換

**Files:**
- Modify: `internal/domain/item.go`
- Test: `internal/domain/item_test.go`

- [ ] **Step 1: 失敗するテストを書く**

`item_test.go` に追記:
```go
func TestItemUnmarshalLegacyBoardIDs(t *testing.T) {
	raw := []byte(`{"id":"i1","board_ids":["b1","b2"],"starred":true}`)
	var it Item
	if err := json.Unmarshal(raw, &it); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(it.BookmarkIDs) != 2 || it.BookmarkIDs[0] != "b1" {
		t.Fatalf("legacy board_ids should map to BookmarkIDs, got %v", it.BookmarkIDs)
	}
}

func TestItemUnmarshalPrefersBookmarkIDs(t *testing.T) {
	raw := []byte(`{"id":"i1","bookmark_ids":["x"],"board_ids":["legacy"]}`)
	var it Item
	if err := json.Unmarshal(raw, &it); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(it.BookmarkIDs) != 1 || it.BookmarkIDs[0] != "x" {
		t.Fatalf("bookmark_ids should win, got %v", it.BookmarkIDs)
	}
}

func TestHasUserActionBookmark(t *testing.T) {
	if !(Item{BookmarkIDs: []string{"b"}}).HasUserAction() {
		t.Fatal("bookmark should count as user action")
	}
	if (Item{Read: true}).HasUserAction() {
		t.Fatal("read alone is not a user action")
	}
}
```
（`encoding/json` を import に追加）

- [ ] **Step 2: テスト失敗を確認** `go test ./internal/domain/ -run Item` → FAIL（BookmarkIDs 未定義）

- [ ] **Step 3: 実装**

`item.go`: `Starred` 行を削除、`BoardIDs` を `BookmarkIDs []string json:"bookmark_ids"` に改名。`HasUserAction` から `i.Starred` を除去し `len(i.BookmarkIDs) > 0` へ。`UnmarshalJSON` を追加:
```go
// UnmarshalJSON 旧キー board_ids を BookmarkIDs へ取り込む後方互換を持たせます。
// bookmark_ids があればそれを優先し、無ければ board_ids を使います。starred 等の旧キーは無視します。
func (i *Item) UnmarshalJSON(data []byte) error {
	type alias Item
	aux := struct {
		*alias
		LegacyBoardIDs []string `json:"board_ids"`
	}{alias: (*alias)(i)}
	if err := json.Unmarshal(data, &aux); err != nil {
		return err
	}
	if len(i.BookmarkIDs) == 0 && len(aux.LegacyBoardIDs) > 0 {
		i.BookmarkIDs = aux.LegacyBoardIDs
	}
	return nil
}
```
（`import "encoding/json"` 追加。`ShouldRetain` は変更不要）

- [ ] **Step 4: テスト成功を確認** `go test ./internal/domain/ -run "Item|HasUserAction"` → PASS

- [ ] **Step 5: コミット** `git add -A && git commit -m "refactor(domain): Item.StarredをBookmarkIDsへ置換し後方互換を追加"`

### Task 2: domain Board → Bookmark

**Files:**
- Rename: `internal/domain/board.go` → `internal/domain/bookmark.go`

- [ ] **Step 1: 実装** `bookmark.go` を作成:
```go
package domain

// Bookmark 階層なしの名称付きブックマークを表します。記事は BookmarkIDs で複数所属できます。
type Bookmark struct {
	ID   string `json:"id"`   // 一意な識別子です
	Name string `json:"name"` // ブックマーク名です
}
```
`board.go` を削除。

- [ ] **Step 2: コンパイル確認** `go build ./internal/domain/` → 成功（他層は次タスクで直す）

- [ ] **Step 3: コミット** `git add -A && git commit -m "refactor(domain): Board を Bookmark へ改名"`

### Task 3: store の bookmark 改名とマイグレーション

**Files:**
- Rename: `internal/store/board.go` → `internal/store/bookmark.go`
- Modify: `internal/store/store.go`
- Test: `internal/store/store_test.go`

- [ ] **Step 1: 失敗するテストを書く** `store_test.go` に、`data/boards.json` のみを置いた状態でStoreをロードし `Bookmarks()` が変換結果を返す＋`bookmarks.json` が生成されることを検証するテストを追加（既存テストの board 参照も bookmark へ更新）。
```go
func TestLoadMigratesBoardsToBookmarks(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "boards.json"),
		[]byte(`[{"id":"b1","name":"旧ボード","description":"x"}]`), 0o600); err != nil {
		t.Fatal(err)
	}
	s, err := Open(dir)
	if err != nil { t.Fatal(err) }
	bms, _ := s.Bookmarks()
	if len(bms) != 1 || bms[0].Name != "旧ボード" {
		t.Fatalf("migration failed: %v", bms)
	}
	if _, err := os.Stat(filepath.Join(dir, "bookmarks.json")); err != nil {
		t.Fatalf("bookmarks.json should be created: %v", err)
	}
}
```
（Store の生成関数名・field名は既存 `store.go` に合わせる。`Open`/`New` 実体を確認して合わせること）

- [ ] **Step 2: テスト失敗確認** `go test ./internal/store/ -run Migrat` → FAIL

- [ ] **Step 3: 実装**
  - `store.go`: `boards []domain.Board` → `bookmarks []domain.Bookmark`、定数 `boardsFile="boards.json"` → `bookmarksFile="bookmarks.json"`。ロード処理で `bookmarks.json` を読む。無ければ `boards.json` を読み Board→Bookmark 変換し `bookmarks` に格納、`writeJSONAtomic(bookmarksFile)` で保存。
  - `bookmark.go`: `Boards/SaveBoard/DeleteBoard` → `Bookmarks/SaveBookmark/DeleteBookmark`、型を `domain.Bookmark`、ファイルを `bookmarksFile` に。

- [ ] **Step 4: テスト成功確認** `go test ./internal/store/` → PASS

- [ ] **Step 5: コミット** `git add -A && git commit -m "refactor(store): boardをbookmarkへ改名しboards.jsonマイグレーションを追加"`

## Phase 2: ポートとサービス

### Task 4: port インターフェース改名

**Files:**
- Modify: `internal/port/repository.go`, `internal/port/service.go`

- [ ] **Step 1: 実装**
  - `repository.go`: `Boards()/SaveBoard/DeleteBoard` → `Bookmarks() ([]domain.Bookmark, error)/SaveBookmark(domain.Bookmark) error/DeleteBookmark(id string) error`。
  - `service.go`: `SetBoards(feedID,itemID string, boardIDs []string)` → `SetBookmarks(feedID,itemID string, bookmarkIDs []string) error`。BookmarkService 用に新インターフェース（List/Create/Toggle/CreateAndAdd）を定義。
- [ ] **Step 2: コミット** `git add -A && git commit -m "refactor(port): bookmark向けにインターフェースを改名・追加"`

### Task 5: ItemService 改修と BookmarkService 新設

**Files:**
- Modify: `internal/service/item.go`, `internal/service/service.go`
- Create: `internal/service/bookmark.go`
- Test: `internal/service/bookmark_test.go`, 既存 `item_action_test.go`

- [ ] **Step 1: 失敗するテストを書く** `bookmark_test.go`:
```go
func TestBookmarkCreateDedupesByName(t *testing.T) {
	svc := newTestBookmarkService(t) // fakes_test の Deps を流用
	b1, _ := svc.Create("読み物")
	b2, _ := svc.Create("読み物")
	if b1.ID != b2.ID { t.Fatal("同名は既存を返すべき") }
}
func TestBookmarkCreateRejectsEmpty(t *testing.T) {
	svc := newTestBookmarkService(t)
	if _, err := svc.Create("  "); err == nil { t.Fatal("空名はエラー") }
}
func TestBookmarkToggleAddsAndRemoves(t *testing.T) {
	// item を保存→Toggle で BookmarkIDs に追加→再Toggleで除去
}
```
（既存 `fakes_test.go` のフェイクリポジトリに Bookmark メソッドを追加）

- [ ] **Step 2: 失敗確認** `go test ./internal/service/ -run Bookmark` → FAIL

- [ ] **Step 3: 実装**
  - `item.go`: `Star()` を削除、`SetBoards` → `SetBookmarks`（`item.BookmarkIDs` 全置換）。
  - `bookmark.go`:
```go
package service

import (
	"errors"
	"fmt"
	"strings"

	"github.com/okamyuji/feedflow-go-htmx/internal/domain"
)

type BookmarkService struct {
	deps  Deps
	items *ItemService
}

func NewBookmarkService(deps Deps, items *ItemService) *BookmarkService {
	return &BookmarkService{deps: deps, items: items}
}

func (s *BookmarkService) List() ([]domain.Bookmark, error) {
	bms, err := s.deps.Repo.Bookmarks()
	if err != nil {
		return nil, fmt.Errorf("failed to load bookmarks: %w", err)
	}
	return bms, nil
}

func (s *BookmarkService) Create(name string) (domain.Bookmark, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return domain.Bookmark{}, errors.New("bookmark name is required")
	}
	bms, err := s.deps.Repo.Bookmarks()
	if err != nil {
		return domain.Bookmark{}, err
	}
	for _, b := range bms {
		if b.Name == name {
			return b, nil
		}
	}
	bm := domain.Bookmark{ID: s.deps.IDGen.NewID(), Name: name}
	if err := s.deps.Repo.SaveBookmark(bm); err != nil {
		return domain.Bookmark{}, err
	}
	return bm, nil
}

func (s *BookmarkService) Toggle(feedID, itemID, bookmarkID string) error {
	return s.items.toggleBookmark(feedID, itemID, bookmarkID)
}

func (s *BookmarkService) CreateAndAdd(feedID, itemID, name string) (domain.Bookmark, error) {
	bm, err := s.Create(name)
	if err != nil {
		return domain.Bookmark{}, err
	}
	if err := s.items.addBookmark(feedID, itemID, bm.ID); err != nil {
		return domain.Bookmark{}, err
	}
	return bm, nil
}
```
  - `item.go` に `toggleBookmark`/`addBookmark` を `mutateItem` ベースで追加（重複追加しない、トグルは存在すれば除去）。
  - `service.go`: `Deps` に IDGen がある前提（既存 idgen 利用）。BookmarkService を配線。

- [ ] **Step 4: 成功確認** `go test ./internal/service/` → PASS

- [ ] **Step 5: コミット** `git add -A && git commit -m "feat(service): BookmarkServiceを追加しStarを廃止"`

## Phase 3: ハンドラとルーティング

### Task 6: render.go モデル改名・拡張

**Files:**
- Modify: `internal/handler/render.go`

- [ ] **Step 1: 実装**
  - `pageData.Boards []domain.Board` → `Bookmarks []domain.Bookmark`。
  - `itemView.Starred` を削除し `Bookmarked bool`（所属が1つ以上なら真）を追加。`CurrentLabel` コメントのスター→ブックマーク。
  - `feedTreeNode`: `Children []feedTreeNode` を追加（ブックマーク子ノード用）。Kind コメントを `starred`→`bookmark`。
- [ ] **Step 2: ビルド確認**（他参照は後続で修正）`go build ./... 2>&1 | head` で残課題把握
- [ ] **Step 3: コミット** `git add -A && git commit -m "refactor(handler): 描画モデルをbookmark対応へ更新"`

### Task 7: bookmark ハンドラとルート

**Files:**
- Rename: `internal/handler/board_handler.go` → `internal/handler/bookmark_handler.go`
- Modify: `internal/handler/router.go`, `internal/handler/item_handler.go`
- Test: `internal/handler/bookmark_handler_test.go`（旧 board テスト改名）

- [ ] **Step 1: 失敗するテストを書く** ピッカーGET、toggle POST、create POST の各ハンドラの挙動テスト（CSRF付き、レスポンスに名称が含まれる/204等）。
- [ ] **Step 2: 失敗確認** `go test ./internal/handler/ -run Bookmark` → FAIL
- [ ] **Step 3: 実装**
  - `bookmark_handler.go` に `bookmarkPicker`(GET), `bookmarkToggle`(POST), `bookmarkCreate`(POST) を実装。`itemSetBoards` は削除（または保持不要）。
  - `router.go`: `POST .../star` 行を削除。`POST .../boards` を削除。以下追加（authとCSRFは既存ラッパ流用）:
    - `GET /app/items/{feedID}/{itemID}/bookmarks` → `bookmarkPicker`
    - `POST /app/items/{feedID}/{itemID}/bookmarks/toggle` → `bookmarkToggle`（CSRF必須）
    - `POST /app/bookmarks` → `bookmarkCreate`（CSRF必須）
  - `item_handler.go`: `listItemsFor` の `q.Get("board")` を `q.Get("bookmark")` に、`it.BoardIDs`→`it.BookmarkIDs`。`view=="bookmark"` を保存系（未読フィルタ除外）として追加。`bulkReadContext`/`currentSelectionLabel`/`markActiveNodes` の `board`/`starred` 参照を `bookmark` に更新。`toItemView` の Starred→Bookmarked。
- [ ] **Step 4: 成功確認** `go test ./internal/handler/` → PASS（テンプレート未更新分は次タスクで）
- [ ] **Step 5: コミット** `git add -A && git commit -m "feat(handler): bookmarkエンドポイントを追加しstar/boardを除去"`

## Phase 4: テンプレート・JS・CSS

### Task 8: ブックマークピッカーとカード/オーバーレイ

**Files:**
- Create: `internal/handler/templates/_bookmark_picker.html`
- Modify: `_item_card.html`, `_overlay_actions.html`

- [ ] **Step 1: 実装**
  - `_bookmark_picker.html`: 全ブックマークを行表示（各行 `POST .../bookmarks/toggle` を hx-post、当該記事所属なら `is-active`）、最下部に新規名称 input＋`POST /app/bookmarks`（hx-post, target=ピッカー）。CSRFは `body` の `hx-headers` を継承するため不要だが form 経由なら hidden 付与。
  - `_item_card.html`: スターボタンを削除し「🔖ブックマーク」ボタンへ。`{{ if .Bookmarked }}` で保存済み表示。ピッカーは Alpine ローカル開閉 or `hx-get` でカード内に差し込む領域を用意。
  - `_overlay_actions.html`: スターボタンをブックマークボタンへ置換（surface=overlay）。
- [ ] **Step 2: 描画テスト**（handler テストで `bookmark` 文言を検証）→ PASS
- [ ] **Step 3: コミット** `git add -A && git commit -m "feat(ui): ブックマーク保存UI(インラインピッカー)を追加"`

### Task 9: 左メニュー（展開できるブックマークノード）

**Files:**
- Modify: `internal/handler/feed_handler.go`, `_tree.html`, `_tree_node.html`

- [ ] **Step 1: 失敗するテストを書く** `feed_handler_test.go`: buildTree が `bookmark` ノードを持ち、その `Children` が各ブックマーク名と件数を含むことを検証。
- [ ] **Step 2: 失敗確認** → FAIL
- [ ] **Step 3: 実装**
  - `buildTree`: 固定ノードの `starred` を `bookmark`（Label「ブックマーク」, Kind「bookmark」, Children=各ブックマーク[Kind「bookmarkItem」, ID, Label=名称, UnreadCount=該当件数]）に置換。件数はそのブックマークIDを持つ記事数。
  - `_tree_node.html`: Kind「bookmark」は `?view=bookmark`、子ノード「bookmarkItem」は `?bookmark=ID`。Children を `<ul class="tree-sub">` で開閉表示（Alpine で開閉、既定閉）。
  - `_tree.html`: 非feedノード描画は現状維持（buildTree の Children をテンプレートで展開）。
- [ ] **Step 4: 成功確認** `go test ./internal/handler/ -run Tree` → PASS
- [ ] **Step 5: コミット** `git add -A && git commit -m "feat(ui): 左メニューに展開式ブックマークノードを追加"`

### Task 10: app.js / styles.css の star 除去とピッカー/ノードスタイル

**Files:**
- Modify: `internal/handler/static/app.js`, `internal/handler/static/styles.css`

- [ ] **Step 1: 実装**
  - `app.js`: `s` キーのスター処理を削除。ブックマークは `b` キーでピッカー開閉（または toggle）に割当。`postAction` の star 参照除去。
  - `styles.css`: `.item-star` を `.item-bookmark` 等へ。ピッカー（`.bookmark-picker` 行/入力）、子ノード（`.tree-sub`）のスタイル追加。
- [ ] **Step 2: ビルド/手動確認**（後段の実ブラウザ検証で担保）
- [ ] **Step 3: コミット** `git add -A && git commit -m "refactor(ui): JS/CSSのstar参照を除去しブックマーク用スタイルを追加"`

## Phase 5: 未読Feedの先頭集約（機能改修2）

### Task 11: buildTree のフィード並べ替え

**Files:**
- Modify: `internal/handler/feed_handler.go`, `_tree.html`(区切り)
- Test: `internal/handler/feed_handler_test.go`

- [ ] **Step 1: 失敗するテストを書く** 未読を持つフィード2件（LastFetchedAt 異なる）＋未読0のフィード1件を用意し、出力フィード順が「未読群（LastFetchedAt降順）→残り（購読順）」になること、未読群が無ければ購読順のままを検証。
- [ ] **Step 2: 失敗確認** → FAIL
- [ ] **Step 3: 実装** `buildTree` のフィードノード生成後、`unread>0` と `==0` で分割、未読群を `LastFetchedAt` 降順で安定ソート、連結。未読群の末尾ノードに区切り表示用フラグ（例 `UnreadGroupEnd bool`）を立て、`_tree.html`/`_tree_node.html` で控えめな区切り線を描画。
- [ ] **Step 4: 成功確認** `go test ./internal/handler/ -run "Tree|Unread"` → PASS
- [ ] **Step 5: コミット** `git add -A && git commit -m "feat(ui): 未読のあるFeedを更新日時順で先頭に集約"`

## Phase 6: 既読記事の先頭表示（機能改修4）

### Task 12: 単一フィードの read-head 5件

**Files:**
- Modify: `internal/handler/item_handler.go`, `_item_list.html`
- Test: `internal/handler/item_handler_test.go`

- [ ] **Step 1: 失敗するテストを書く** 単一フィード（`?feed=f1`）で、既読6件＋未読数件のとき、先頭に既読5件（recency降順）→未読、の順で並ぶこと。`?view=...`/`?bookmark=` 等では従来どおり既読が出ないこと。
- [ ] **Step 2: 失敗確認** → FAIL
- [ ] **Step 3: 実装** `listItemsFor` の既定分岐（feed指定かつ view/category/bookmark 無し）で、recency降順に整列→既読上位 `readHeadLimit=5` 件を先頭、続けて未読全件を返す。定数 `const readHeadLimit = 5`。`_item_list.html` に「ここから未読」の控えめ区切り（itemView に `UnreadStart bool` 等のフラグ、または handler で境界 index を渡す）。
- [ ] **Step 4: 成功確認** `go test ./internal/handler/ -run "Item|ReadHead"` → PASS
- [ ] **Step 5: コミット** `git add -A && git commit -m "feat(ui): 単一フィードで既読5件を先頭表示"`

## Phase 7: レスポンシブ（機能改修3）

### Task 13: モバイル・オフキャンバス・ドロワー

**Files:**
- Modify: `internal/handler/static/styles.css`, `base.html`, `internal/handler/static/app.js`

- [ ] **Step 1: 実装**
  - `styles.css` `<48rem`: `.tree-pane { position:fixed; top:app-bar下; left:0; bottom:0; width:min(80vw,18rem); transform:translateX(-100%); transition; z-index }`、`.app-body.sidebar-open .tree-pane { transform:translateX(0) }`、スクリム `.drawer-scrim`。`.main-pane` 全幅。`.reading-overlay` 全画面。タップ領域 44px。`48rem`以上は現状グリッド維持。
  - `base.html`: `.app-body` に `:class="sidebarClass"`（既存）。スクリム要素を追加（`x-show="sidebarOpen && isMobile"`, `@click="closeSidebar"`）。
  - `app.js`: `isMobile`（matchMedia `(max-width:48rem)`）を持ち、`sidebarOpen` 初期値をモバイルでは false。`sidebarClass` を `sidebar-open`/`sidebar-collapsed` に整理。ドロワー内リンクタップ後モバイルなら閉じる。
- [ ] **Step 2: 手動/E2Eで確認**（Task 15 と実ブラウザ検証）
- [ ] **Step 3: コミット** `git add -A && git commit -m "feat(responsive): モバイルのオフキャンバス・ドロワー対応"`

## Phase 8: E2E・ドキュメント・品質ゲート

### Task 14: ドキュメント更新

**Files:**
- Modify: `README.md`, 関連 `docs/`

- [ ] **Step 1: 実装** README の機能一覧・操作説明から「スター」を「ブックマーク」へ。ボード記述があれば統合。左メニュー/未読集約/既読先頭/モバイル対応を追記。
- [ ] **Step 2: コミット** `git add -A && git commit -m "docs: ブックマーク等の改修に合わせてREADMEを更新"`

### Task 15: E2E 更新

**Files:**
- Modify: `e2e/playwright/tests/read.spec.ts`、追加 spec

- [ ] **Step 1: 実装** `read.spec.ts` のスター操作 → ブックマーク（保存・新規作成・既存追加・`?bookmark=` 絞り込み）へ。tree 未読先頭・read-head・モバイルドロワー開閉の spec を追加（`page.setViewportSize({width:375})`）。
- [ ] **Step 2: ローカル実行**（後述の検証フェーズ）
- [ ] **Step 3: コミット** `git add -A && git commit -m "test(e2e): スターをブックマークへ更新しレスポンシブ等のspecを追加"`

### Task 16: 品質ゲート＆実ブラウザ検証

- [ ] **Step 1:** `bash scripts/quality-gate.sh` を全通過（gofmt/vet/staticcheck/golangci-lint/govulncheck/test -race -cover/build/gitleaks）。
- [ ] **Step 2:** ローカルでアプリ起動＋Playwright E2E green。
- [ ] **Step 3:** Playwright MCP で PC幅・375px幅の実ブラウザ検証（ブックマーク保存/左メニュー/未読集約/既読先頭/ドロワー）。
- [ ] **Step 4:** ブランチで commit & push、PR 作成。

---

## Self-Review（spec 対応確認）

- スター削除＋ブックマーク（名称・新規/既存・案A UI・左メニュー案A）: Task 1,2,4,5,6,7,8,9,10 ✓
- 元記事消滅時の黙って非表示 / retention 保護: Task 1（HasUserAction）＋所属を記事保持で構造的に充足 ✓
- 未読Feed先頭集約（更新日時降順）: Task 11 ✓
- 既読5件先頭（単一フィードのみ）: Task 12 ✓
- レスポンシブ（ドロワー案A）: Task 13 ✓
- テスト/ドキュメント/品質ゲート/実ブラウザ: Task 14,15,16 ✓

依存: Store の生成関数名・Deps の IDGen 名は既存実体に合わせて実装時に確定する（`internal/store/store.go`, `internal/service/service.go` を参照）。
