# feedflow 機能追加 設計書（任意URLのブックマーク追加）

- 日付: 2026-08-22
- 対象: feedflow-go-htmx
- ステータス: 承認済み（ブレインストーミング合意）

## 背景と目的

現在のブックマークは、購読フィードから取得した記事にしか付けられない。フィードを持たないページや、購読していないサイトの記事を後で読むために保存したい場合、手段が無い。

本改修では、ブックマークビューの画面から任意のURLを入力して保存できるようにする。保存したページは既存のブックマーク一覧に既存記事と同じカードとして並び、既存のラベル（名称コレクション）機構をそのまま使える。

## 確定した要件（ブレインストーミングでの決定事項）

| 項目 | 決定 |
|------|------|
| 入口 | アプリ内UI。ブックマーク系ビューの一覧バーにURL入力欄を置く |
| 外部連携（ブックマークレット等） | 対象外 |
| 取得するメタデータ | タイトルのみ。`og:title` を優先し、無ければ `<title>` |
| 本文取得 | 行わない。カードは外部リンクとして開く |
| ラベル指定 | 任意。入力欄の隣に既存ラベルのセレクトを置く。未指定ならラベルなしで保存 |
| ラベルの新規作成 | この画面からは行わない。追加後にカードのピッカーで行う |
| 既に購読フィードに存在するURL | 新規作成せず、その既存記事を保存済みにする（＋指定ラベル付与） |
| 保存先 | 予約IDの合成フィード（案A） |
| タイトル取得失敗時 | エラー表示せず、URLをタイトルとして保存を成功させる |

## アーキテクチャ前提（現状）

- レイヤ構成: handler → service → port（インターフェース）→ store（メモリ常駐＋アトミックJSON永続化）。
- フロント: HTMX + Alpine.js + Go html/template（埋め込みFS）。
- 永続化: `data/` 配下に `feeds.json`, `bookmarks.json`, `items/{feedID}.json`。
- `domain.Item` は `FeedID` を必須で持ち、`items/{feedID}.json` に格納される。フィードに属さない記事の置き場は存在しない。
- `domain.Item` は保存状態 `Bookmarked` とラベル所属 `BookmarkIDs` を分離して保持する。
- ブックマークのHTTPルートはすべて `(feedID, itemID)` にキーされている。

## 決定フリーズ表（MECE）

| 決定 | 検討した選択肢 | 採用 | 理由 |
|------|----------------|------|------|
| 保存先の構造 | A: 予約IDの合成フィード / B: Feedに種別フラグを追加 / C: 新エンティティ`SavedPage`と専用ストア | A | 既存のピッカー・カード・オーバーレイ・`POST .../bookmark`・ブックマークビュー一覧が無改修で動く。BはFeedスキーマ変更と後方互換処理が増え、予約ID＋判定ヘルパで同じ効果が得られる。Cは記事パイプラインのフォークになり、UIの複製が必要になる |
| 合成フィードのID | 固定文字列 / 生成ID | 固定文字列 `saved-pages` | `store.idPattern`（`^[A-Za-z0-9_-]{1,64}$`）に適合し、コードから定数で参照できる。生成IDだと参照のたびに検索が要る |
| 合成フィードの生成時点 | 起動時に必ず作成 / 初回追加時に遅延作成 | 初回追加時に遅延作成 | 機能を使わない利用者の`feeds.json`を汚さない |
| 重複判定のキー | `Link` 完全一致 / URL正規化（クエリ除去等） / `GUID` | `Link` の正規化一致（scheme・host小文字化、末尾スラッシュ除去、フラグメント除去） | 完全一致だけでは`https://example.com/a`と`https://example.com/a/`が別扱いになる。クエリ除去は記事を取り違える危険があるため行わない |
| タイトル抽出の優先順位 | `<title>`のみ / `og:title`優先 | `og:title` → `<title>` → URL | `og:title` の方がページ主題を表す場合が多い。両方欠けたらURLで代替する |
| 文字コード | UTF-8決め打ち / `charset`解決 | `golang.org/x/net/html/charset` で解決 | 日本語サイトにEUC-JP・Shift_JISが残る。依存は`go.mod`に既にある |
| 取得タイムアウト | Fetcher既定30秒 / 20秒 | 20秒 | 既存の手動取得`manualPollTimeout`と揃える。HTTP WriteTimeout 30秒より短くする必要がある |
| 合成フィードの記事の解除時の扱い | 記事を残す / 記事を削除する | 削除する | 残すと`!Read && !Bookmarked`となり「すべて」の未読ストリームに出所不明のカードとして出る。未読合計もずれる |
| 既存フィード記事を解除したときの扱い | 削除する / 記事を残す | 記事を残す（現行動作のまま） | フィードから再取得され得る通常記事であり、削除は現行仕様の変更になる |
| ラベル新規作成の可否 | 追加フォームで作れる / 作れない | 作れない | 既存ピッカーに新規作成があり、二重の入口はUIを太らせる。追加後に付け替えられる |
| ポーリング対象からの除外方法 | `PollInterval`を手動のみにする / 取得処理でIDを判定して除外 | 取得処理でIDを判定して除外 | `PollAllNow`は期限判定を無視するため、間隔設定だけでは防げない |

## 詳細設計

### 1. ドメイン（`internal/domain`）

`domain/saved.go` を新規追加する。

```go
// SavedPagesFeedID 任意URLを保存するための合成フィードのIDです。
const SavedPagesFeedID = "saved-pages"

// SavedPagesFeedTitle 合成フィードの表示名です。
const SavedPagesFeedTitle = "保存したページ"

// IsSavedPagesFeed 指定IDが合成フィードかどうかを返します。
func IsSavedPagesFeed(feedID string) bool
```

`domain.Feed` と `domain.Item` の構造体は変更しない。合成フィードは `FeedURL` が空の `Feed` レコードとして `feeds.json` に載る。

`FeedURL` が空という条件だけでは合成フィードを特定できない。判定には必ず `IsSavedPagesFeed` を使う。

### 2. メタデータ取得（`internal/feed`）

`internal/feed/meta.go` を新規追加する。

```go
// PageMeta HTMLページから抽出したメタデータです。
type PageMeta struct {
    Title string // og:titleまたはtitle要素から得たページ名です
}

// ExtractMeta HTMLバイト列とContent-Typeからページのメタデータを抽出します。
// og:title を優先し、無ければ title 要素を使います。どちらも得られない場合はTitleが空になります。
// 文字コードはContent-Typeとmetaタグから解決します。
func ExtractMeta(body []byte, contentType string) PageMeta
```

- 文字コード解決は `golang.org/x/net/html/charset` の `charset.NewReader` を使う。
- `Content-Type` が `text/html` および `application/xhtml+xml` 以外の場合、呼び出し側が本関数を呼ばずタイトル空として扱う。
- 抽出したタイトルは前後の空白を除去し、連続する空白を1つに畳む。長さは256文字で切る。

HTTP取得は既存の `port.Fetcher`（`internal/feed/fetcher.go` の `HTTPFetcher`）をそのまま使う。SSRF対策・サイズ上限・リダイレクト処理は既存実装が担う。

### 3. ポート（`internal/port`）

`port.BookmarkService` にメソッドを1つ追加する。

```go
// AddURL 任意のURLをブックマークに追加し、保存された記事を返します。
// 既に同じURLの記事が購読フィードにあればその記事を保存済みにします。
// 無ければ合成フィードに新しい記事を作ります。
// bookmarkIDが空でなければ、その名称コレクションにも所属させます。
AddURL(ctx context.Context, rawURL, bookmarkID string) (domain.Item, error)
```

### 4. サービス（`internal/service/bookmark.go`）

`BookmarkService.AddURL` を実装する。処理順は次のとおり。

#### 手順1 URLの検証

`url.Parse` で解析する。schemeが `http` と `https` のどちらでもなければ `ErrInvalidURL` を返す。hostが空のときも同じエラーを返す。

#### 手順2 ラベルの存在確認

`bookmarkID` が空でなければ `Repo.Bookmarks()` に存在するか確認する。存在しなければ `ErrBookmarkNotFound` を返す。

#### 手順3 重複記事の探索

全フィードの記事を走査し、正規化したURLが一致する記事を探す。見つかれば `Bookmarked` を真にし、`bookmarkID` があれば `BookmarkIDs` へ追加して保存し、その記事を返す。同じラベルの二重追加はしない。合成フィードの既存記事もこの経路で見つかるため、同じURLを二度追加しても記事は増えない。

#### 手順4 合成フィードの確保

`Repo.Feed(SavedPagesFeedID)` が失敗したら `Feed{ID: SavedPagesFeedID, Title: SavedPagesFeedTitle, PollInterval: domain.PollManualOnly}` を保存する。

#### 手順5 タイトルの取得

20秒のタイムアウトを付けたcontextで `Fetch.Fetch` を呼ぶ。成功してHTMLであれば `ExtractMeta` からタイトルを得る。取得失敗、非HTML、タイトル空のいずれでも、タイトルには正規化前の入力URLを使う。エラーは返さず、ログにだけ残す。

#### 手順6 記事の作成

`Item{ID: IDs.NewID(), FeedID: SavedPagesFeedID, GUID: 正規化URL, Title: タイトル, Link: 正規化URL, PublishedAt: Clock.Now(), FetchedAt: Clock.Now(), Bookmarked: true, BookmarkIDs: 指定があれば1件}` を作る。既存記事の先頭に積んで `SaveItems` する。

#### URLの正規化ルール

`normalizeURL` は次の4つを行う。schemeとhostを小文字にする。フラグメントを除去する。パスが `/` のみでない場合の末尾スラッシュを除去する。クエリはそのまま残す。

### 5. 全列挙箇所からの除外

合成フィードは購読フィードではないため、次の3系統から除外する。除外しない箇所も明示する。

| 箇所 | 対応 | 理由 |
|------|------|------|
| `internal/poller/service.go` の `PollAll` / `PollAllNow` | 対象IDから除外 | `FeedURL` が空のため取得に失敗し、`ConsecutiveErrors` が積み上がってエラー状態になる |
| `internal/poller/service.go` の `PollFeed` | 早期returnで何もせず成功扱い | 手動取得ボタンから個別に呼ばれ得る |
| `internal/poller/runner.go` の `dueFeedIDs` | 対象IDから除外 | 定期ポーリングの入口 |
| `internal/handler/feed_handler.go` の `buildTree` | `orderFeedNodes` の入力から除外 | 左ツリーに購読フィードとして出さない。合わせて購読解除・改名のUIも対象外になる |
| `internal/service/opml.go` の `Export` | 出力から除外 | `FeedURL` が空のOPMLエントリは他のリーダーで壊れる |
| `internal/service/item.go` の `ListItems("")` | **除外しない** | 保存したページをブックマークビューに載せる経路そのもの |
| `internal/service/retention.go` の `Apply` | **除外しない** | `Item.HasUserAction` が `Bookmarked` を見るため保存済み記事は保持される。合成フィードの記事は常に保存済みなので削除されない |
| `internal/handler/item_handler.go` の未読集計 | **除外しない** | 保存済みは既に未読カウントから外れている |

### 6. 解除時の記事削除

`internal/handler/item_handler.go` の `itemBookmark` は、`bookmarked=false` かつ `IsSavedPagesFeed(feedID)` が真のとき、記事そのものを削除する。この条件では保存状態の更新を行わない。削除は `port.ItemService` に追加する `DeleteItem(feedID, itemID string) error` で行い、合成フィード以外のフィードIDを渡された場合は `ErrNotSavedPagesFeed` を返して拒否する。

レスポンスは既存の解除時と同じく、ブックマークビューでは `hx-swap-oob="delete"` でカードを一覧から消す。`renderBookmarkPicker` の `removeFromList` 経路をそのまま使う。ブックマークビュー以外から解除された場合は、削除済みのカードを再描画できないため同じくOOB削除で消す。

### 7. ルーティングとハンドラ

`internal/handler/router.go` の状態変更ブロックに1行追加する。

```
mux.HandleFunc("POST /app/bookmarks/add-url", h.requireAuth(h.requireCSRF(h.bookmarkAddURL)))
```

`internal/handler/bookmark_handler.go` に `bookmarkAddURL` を追加する。

- フォーム値 `url`（必須）と `bookmark_id`（任意）を読む。
- `Bookmarks.AddURL` を呼ぶ。
- 成功時は現在の一覧を再描画して `#main-pane` へ返す。`itemList` と同じ描画経路を使い、追加した記事が一覧に現れる状態にする。
- 失敗時はHTTP 200のまま、入力欄の上にエラー文言を出した同じフォームを返す。想定するエラーは、URL形式が不正、ラベルが見つからない、保存に失敗、の3種。エラー文言は日本語の平文とする。

### 8. UI（`internal/handler/templates`）

`_bookmark_add_url.html` を新規追加し、`_item_list.html` の `item-list-bar` からブックマーク系ビューのときだけ差し込む。

```
[ URLを貼り付け                    ] [ ラベル: なし ▾ ] [ 追加 ]
```

- フォームは `hx-post="/app/bookmarks/add-url"`、`hx-target="#main-pane"`、`hx-swap="innerHTML"`、`hx-disabled-elt="find button"`。
- `input[type=url]` に `required` と `placeholder="https://example.com/article"`。
- ラベルセレクトの先頭は「ラベルなし」とし、値を空文字にする。以降は既存ラベルを名前順で並べる。ラベルが1件も無ければセレクト自体を出さない。
- CSRFトークンはhidden inputで渡す。`base.html` の `hx-headers` でも付くが、既存フォームの慣習に合わせる。
- 絵文字は使わない。件数バッジも出さない。
- 表示条件は「ブックマーク系ビュー」= `view=bookmark` または `bookmark={id}` が付いている場合。判定は既存の `isBookmarkViewURL` を使う。

`pageData` に `BookmarkOptions []bookmarkOption` と `ShowAddURL bool`、`AddURLError string` を追加する。

スタイルは `internal/handler/static/styles.css` に `.add-url-form` を追加する。既存の `.inline-form` と同じ高さに揃え、モバイル幅では入力欄を全幅にして折り返す。

### 9. 記事カードの挙動

合成フィードの記事は `Content` が空なので、`_item_card.html` の `HasContent` が偽になり、`target="_blank" rel="noopener noreferrer"` で元URLを直接開く。オーバーレイは開かない。これは意図した挙動であり、テンプレートの変更は不要。

## テスト計画

### ユニットテスト

| 対象 | 検証内容 |
|------|----------|
| `feed.ExtractMeta` | `og:title` 優先、`<title>` フォールバック、両方無しで空、Shift_JIS/EUC-JPの文字化けなし、空白畳み込み、256文字切り詰め、不正HTMLでpanicしない |
| `service.normalizeURL` | 末尾スラッシュ除去、フラグメント除去、scheme/host小文字化、クエリ保持、ルートパスの`/`保持 |
| `service.AddURL` | 新規追加で合成フィードと記事が作られる、2回目の追加が既存記事のラベル付与になる、購読フィードの既存記事を再利用する、ラベル指定なしで保存される、存在しないラベルIDでエラー、`javascript:`や`file:`スキームを拒否、hostなしを拒否、取得失敗時にURLをタイトルにして成功する、非HTMLレスポンスでURLをタイトルにする |
| `service` の合成フィード除外 | `opml.Export` に合成フィードが出ない |
| `poller` の合成フィード除外 | `PollAll`・`PollAllNow`・`dueFeedIDs` が合成フィードを対象にしない、`PollFeed` が合成フィードで何もせず成功する、`ConsecutiveErrors` が増えない |
| `handler.buildTree` | 左ツリーに合成フィードのノードが出ない |
| `handler.bookmarkAddURL` | 正常系で一覧が返る、URL不正でエラー文言付きフォームが返る、CSRFなしで拒否される、未認証で拒否される |
| `handler.itemBookmark` | 合成フィードの記事を解除すると記事が消える、購読フィードの記事を解除しても記事は残る |
| `service.DeleteItem` | 合成フィード以外のIDを渡すと拒否される |

カバレッジは変更したコードで80%以上とする。変更行のミューテーションはすべてKILLされること。変更した関数のCRAPスコアは15以下とする。

### E2Eテスト（`e2e/playwright/tests/bookmark-add-url.spec.ts`）

ローカルHTTPサーバで `<title>` と `og:title` を持つHTMLを配信し、そのURLを使う。`FEEDFLOW_ALLOW_PRIVATE_FETCH=1` は `scripts/run-server.sh` で設定済み。

1. ブックマークビューでURLを追加すると、タイトル付きのカードが再読込なしで一覧に現れる。
2. ラベルを選んで追加すると、そのラベルの絞り込みにカードが現れる。
3. 追加したカードのブックマークを解除すると一覧から消え、再度一覧を開いても現れない。
4. 不正なURLを送るとエラー文言が出て、カードは増えない。
5. 左ツリーに「保存したページ」が購読フィードとして現れない。

### 実ブラウザ検証

モバイル幅（375px）でフォームが折り返し、入力欄とボタンが操作できることを確認する。

## ドキュメント更新

- `README.md` の機能一覧に、任意URLの保存を1行追加する。
- 本設計書と対になる実装手順書を `docs/superpowers/plans/2026-08-22-bookmark-add-url.md` に置く。

## 品質ゲート

`scripts/quality-gate.sh` を通す。`go fix -diff`、`gofmt`、`go vet`、`golangci-lint`、`go test`、カバレッジ確認が含まれる。

## マイグレーション・互換性メモ

- 既存の `feeds.json` `items/*.json` `bookmarks.json` のスキーマは変更しない。既存データはそのまま読める。
- 合成フィードは初回のURL追加時に作られる。機能を使わなければ `feeds.json` に現れない。
- 既に `saved-pages` というIDの購読フィードを持つ利用者がいた場合、そのフィードが合成フィードとして扱われポーリング対象から外れる。IDは16バイト乱数のhex（32文字）で生成されるため、既存フィードがこのIDを持つことはない。
- OPMLインポートで `saved-pages` というIDのフィードが作られる経路は無い。インポートは新規IDを採番する。

## 非対象（YAGNI）

- ブックマークレット、iOS共有シート、外部API連携。
- 記事本文の取得とアプリ内リーダーでの表示。
- 追加フォームからのラベル新規作成。
- タイトルの手動編集。
- 説明文・サムネイル画像・著者などタイトル以外のメタデータ。
- 保存したページの一括インポート。
- 未配線の `ItemService.SetBookmarks` のルート配線。
