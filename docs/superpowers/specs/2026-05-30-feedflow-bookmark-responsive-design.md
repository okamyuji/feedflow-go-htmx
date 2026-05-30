# feedflow 機能改修 設計書（ブックマーク・未読集約・既読先頭・レスポンシブ）

- 日付: 2026-05-30
- 対象: feedflow-go-htmx
- ステータス: 承認済み（ブレインストーミング合意）

## 背景と目的

feedflow に以下4点の機能改修を行う。

1. **スター廃止・ブックマーク導入**: スター機能を削除し、階層なしで名称をつけて保存できるブックマークを追加する。保存時に新規作成か既存名称への追加を選択できる。
2. **未読Feedの先頭集約**: 未読のあるFeedをFeed領域の先頭に更新日時順（新しい順）でまとめて表示する。未読が無ければ現行の並び順のまま。
3. **正しいレスポンシブ対応**: iPhone/Android のブラウザでも見やすく操作しやすい表示にする。
4. **既読記事の先頭表示**: 単一フィード表示時、うっかり既読にした記事の再読のため、直近の既読5件を先頭に既読として表示する。

## 確定した要件（ブレインストーミングでの決定事項）

| 項目 | 決定 |
|------|------|
| ブックマークと既存Board/Tagの関係 | 既存 Board を **ブックマークへ統合・改名**。スターは削除。Tag は別途残す |
| 左メニューでのブックマーク表示位置 | 現在の「スター」の位置 |
| 保存UI | 案A: ボタン直下の**インラインドロップダウン**（既存名称をトグルで複数選択＋新規作成欄） |
| 左メニュー表示 | 案A: **展開できる「ブックマーク」ノード**（押下で全件、▸で名称コレクション開閉して個別絞り込み） |
| 元記事が消えた場合 | エラーを出さず**黙って非表示** |
| retention 保護 | ブックマーク済み記事は**自動保持期間の削除から保護**（現スターと同様） |
| 未読Feedの並び順 | **更新日時順（新しい順）** |
| 既読先頭表示の件数 | **5件** |
| 既読先頭表示の適用範囲 | **単一フィード表示時のみ** |
| モバイルナビ方式 | 案A: **オフキャンバス・ドロワー**（本文全幅・記事は全画面リーダー） |

## アーキテクチャ前提（現状）

- レイヤ構成: handler → service → port（インターフェース）→ store（メモリ常駐＋アトミックJSON永続化）。
- フロント: HTMX + Alpine.js + Go html/template（埋め込みFS）。
- 永続化: `data/` 配下に `feeds.json`, `bookmarks.json`(旧 `boards.json`), `items/{feedID}.json` 等。

## 詳細設計

### 1. データモデル（`internal/domain`）

#### `item.go`
- `Starred bool` フィールドを**削除**。
- `BoardIDs []string`（json `board_ids`）→ **`BookmarkIDs []string`**（json `bookmark_ids`）へ改名。
- **後方互換**: `Item` に `UnmarshalJSON` を追加。新キー `bookmark_ids` を優先し、無ければ旧キー `board_ids` を取り込む。旧 `starred` キーは未定義フィールドとして無視（破棄）。
- `HasUserAction()`: `Starred` の参照を除去し、`len(i.BookmarkIDs) > 0` を参照。これにより**ブックマーク済み記事は retention 保護**を継続する。

```go
func (i Item) HasUserAction() bool {
    return i.ReadLater ||
        len(i.BookmarkIDs) > 0 ||
        len(i.Tags) > 0 ||
        len(i.Highlights) > 0 ||
        i.Note != ""
}
```

#### `board.go` → `bookmark.go`
- `domain.Board{ID, Name, Description}` → **`domain.Bookmark{ID, Name}`**。`Description` は今回要件外のため廃止する。

### 2. ストア（`internal/store`）

- 永続ファイル `boards.json` → `bookmarks.json`。
- **起動時マイグレーション**: ロード時に `bookmarks.json` が存在せず `boards.json` が存在する場合のみ、Board を Bookmark（`ID`, `Name`）へ変換して読み込み、`bookmarks.json` として保存する。Board は左メニュー未公開で実データはほぼ無い前提だが、安全側で対応する。
- `store` の board 系メソッドを bookmark 系へ改名。

### 3. ポート（`internal/port`）

- `repository.go`: `Boards()/SaveBoard()/DeleteBoard()` → `Bookmarks()/SaveBookmark()/DeleteBookmark()`。
- `service.go`: `SetBoards(...)` → `SetBookmarks(...)`。BookmarkService 用インターフェースを追加。

### 4. サービス（`internal/service`）

- `ItemService.Star()` を**削除**。
- `ItemService.SetBoards()` → `SetBookmarks()`（記事の `BookmarkIDs` 全置換）。
- **`BookmarkService` を新設**（`internal/service/bookmark.go`）:
  - `List() ([]domain.Bookmark, error)`
  - `Create(name string) (domain.Bookmark, error)`: 同名が既存ならそれを返す（重複作成しない）。空名はエラー。
  - `Toggle(feedID, itemID, bookmarkID string) error`: 記事の `BookmarkIDs` に対象IDを追加/解除。
  - `CreateAndAdd(feedID, itemID, name string) (domain.Bookmark, error)`: 新規作成して当該記事を追加。
- 記事の所属トグルは `ItemService.mutateItem` を利用（不変更新）。

### 5. ルーティング・ハンドラ（`internal/handler`）

- **削除**: `POST /app/items/{feedID}/{itemID}/star`、`itemStar`。
- **改名**: `board_handler.go` → `bookmark_handler.go`。`?board=` クエリ → `?bookmark=`。tree 種別 `starred` を `bookmark` へ。
- **追加エンドポイント**:
  - `GET /app/items/{feedID}/{itemID}/bookmarks`: インラインピッカー（`_bookmark_picker.html`）を描画。全ブックマークと当該記事のチェック状態を渡す。
  - `POST /app/items/{feedID}/{itemID}/bookmarks/toggle`（form: `bookmark`=ID, CSRF必須）: 所属トグル後、ピッカーを再描画し、カードのブックマーク表示を `hx-swap-oob` で更新。
  - `POST /app/bookmarks`（form: `name`, `feed`, `item`, CSRF必須）: 新規作成＋当該記事追加後、ピッカーを再描画。
- `render.go`: `pageData.Boards` → `Bookmarks`。`feedTreeNode.Kind` の説明を `starred`→`bookmark` に更新。子ノード（名称コレクション）を保持できるようツリーモデルを拡張。

### 6. ブックマークUI（テンプレート・JS・CSS）

- **保存UI（案A）** `_bookmark_picker.html`（新規）:
  - 「🔖ブックマーク」ボタン直下に開くパネル。既存名称を行リストで表示、各行タップで toggle（チェック反映）。
  - 最下部に新規名称入力欄（入力＋送信で `POST /app/bookmarks`）。
  - 開閉は Alpine のローカル状態、内容更新は HTMX。`_item_card.html` と `_overlay_actions.html` から利用。
- `_item_card.html` / `_overlay_actions.html`: スターボタンを「🔖ブックマーク（済）」表示へ置換。`Item.BookmarkIDs` の有無で保存済み表示。
- **左メニュー（案A）** `_tree`/`_tree_node.html`:
  - 固定ノードの「スター」を「ブックマーク」に置換（押下で `?view=bookmark` 全件）。
  - 配下に名称コレクションの子ノードを開閉表示（各 `?bookmark=ID`、件数バッジ付き）。
- `app.js`: スターのキーボードショートカット（`s`）を削除し、ブックマークのトグルへ割当（`b` など）。`postAction` の star 参照を除去。
- `styles.css`: ブックマークピッカー・子ノードのスタイル追加。

### 7. 未読Feedの先頭集約（機能改修2）

- `feed_handler.go` の `buildTree()`:
  - フィードを `unreadCount > 0`（未読群）と `== 0`（残り）に分割。
  - **未読群は `Feed.LastFetchedAt` 降順（新しい順）**に並べる。残りは現行（購読順）を維持。
  - 連結順 = 未読群 + 残り。未読群が空なら全体が現行順のまま。
  - 未読群と残りの境界に**控えめな区切り線**（テンプレートで `IsUnreadGroupEnd` 的フラグ、または見出し）を描画。
- 並べ替えは新スライスを生成（既存スライスを破壊しない）。

### 8. 既読記事の先頭表示（機能改修4）

- `item_handler.go` の `listItemsFor()`:
  - 条件: `feed` 指定あり、かつ `view`/`category`/`bookmark` 未指定の既定ビュー。
  - `sortByrecency`（公開日時新しい順）相当で並べたうえで、**既読の上位5件**を抽出して先頭に置き（is-read 表示）、続けて未読全件を表示。
  - 「ここから未読」の控えめな区切りを挿入（テンプレートのフラグ）。
  - `すべて`/カテゴリ/ブックマーク/既読/あとで読むビューは従来どおり（変更しない）。
- 件数上限 `5` は名前付き定数で定義。

### 9. レスポンシブ（機能改修3・案A）

- `styles.css`:
  - `<48rem`: `.tree-pane` を `position:fixed; inset` で左オフキャンバス・ドロワー化。`transform: translateX(-100%)`、開時 `translateX(0)`。背後にスクリム。`.main-pane` は常時全幅。
  - `.reading-overlay` はモバイルで全画面（100vw/100vh）。
  - タップ領域は最小 44px を確保（ボタン・リンク・ツリー行）。
  - `48rem` 以上は現行グリッド（18rem/22rem）を維持。
- `base.html` / `app.js`:
  - モバイル既定はドロワー閉。`sidebarOpen` 初期値をビューポート幅（`matchMedia`）で判定。
  - スクリム要素を追加し、タップでドロワーを閉じる。
  - ドロワー内リンクタップ後はモバイルで自動的に閉じる。

### 10. 「元記事が消えた場合は黙って非表示」

- ブックマーク所属は記事自身（`BookmarkIDs`）に持つため、記事削除時にブックマーク表示から自然に消える（エラーなし）。
- ブックマーク名（`bookmarks.json`）は記事0件でも残り得るが、その場合は空一覧を表示しエラーにしない。
- ブックマークビュー・ピッカーは、存在しない記事参照に対して例外を出さずスキップする。

## テスト計画

### ユニットテスト（Go・80%以上）
- `domain`: `UnmarshalJSON` の後方互換（`board_ids`→`BookmarkIDs`、`starred` 無視）、`HasUserAction()`（ブックマークで真、スター除去）。
- `store`: `boards.json`→`bookmarks.json` マイグレーション、bookmark CRUD。
- `service`: `BookmarkService`（Create の重複名・空名、Toggle、CreateAndAdd）、`SetBookmarks`、Star削除の確認。
- `handler`: bookmark ピッカー描画/トグル/新規作成、tree 順序（未読先頭・更新日時降順・未読ゼロで現行順）、read-head 5件（単一フィードのみ）。

### E2E（Playwright）
- `read.spec.ts`: スター操作 → ブックマーク操作（保存・新規作成・既存追加・一覧絞り込み）へ更新。
- tree 順序・read-head・モバイルドロワー開閉の spec を追加。

### 実ブラウザ検証
- PC幅とモバイル幅（375px 等）でブックマーク保存・左メニュー・未読集約・既読先頭・ドロワー操作を確認。

## ドキュメント更新

- `README.md` ほか、star/board に言及する記述をブックマークへ更新。
- 必要に応じて `docs/` 配下の設計記述を同期。

## 品質ゲート

- `scripts/quality-gate.sh`（gofmt, go vet, staticcheck, golangci-lint, govulncheck, go test -race -cover, build, gitleaks）を全通過。
- CI（pre-commit ＋ e2e ジョブ）green。

## マイグレーション・互換性メモ

- 単一ユーザーのローカル/デプロイ用途。Board は左メニュー未公開のため実データはほぼ無い前提。
- star データは機能廃止に伴い破棄（後方互換ロードで無視）。
- `board_ids` データは `UnmarshalJSON` で `BookmarkIDs` へ取り込み、`boards.json` は起動時に `bookmarks.json` へ変換。

## 非対象（YAGNI）

- ブックマークの階層・並べ替え・説明文。
- タグ機能の再設計（現状維持）。
- 既読先頭表示の `すべて`/カテゴリへの拡張。
