# Phase7 ハンドラとUI実装計画

> For agentic workers: REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

Goal: Phase4とPhase5とPhase6で用意したサービスと認証のインターフェースを、コンストラクタ注入でハンドラ層に取り込み、ルーティングとミドルウェアとembedテンプレートとHTMXとAlpine.jsを組み合わせて、ログインと初回セットアップと購読管理と記事閲覧とボードと設定とOPMLの全画面をレイアウトB(2ペインとリーディングオーバーレイ)のダークラグジュアリー配色で完成させます。net/http/httptestでハンドラを統合テストします。

Architecture: internal/handlerはinternal/portのサービスインターフェースとinternal/handler内に定義する認証ポートにだけ依存し、具象型に直接依存しません。依存はすべてhandler.New(deps Deps)のコンストラクタ注入で渡します。画面の部分更新はHTMXで行い、共通レイアウトbase.htmlに対しアンダースコア始まりの部分テンプレートをハンドラから直接ExecuteTemplateで返します。HTMXの部分テンプレートを直接GETやリロードで返すとレイアウトが欠落するため、ハンドラはHX-Requestヘッダで判定し、非HTMXのときはbase.htmlのフルページを返し、HX-Requestのときだけ部分テンプレートを返します。フルページのときはpageData.MainViewでmain-paneへ出す内容を切り替えます。リーディングオーバーレイの開閉とテーマ切替とキーボードショートカットとスクロール追従の自動既読はAlpine.jsで扱います。HTMXとAlpine.jsはベンダーした静的ファイルをembedで同梱し、CDNに依存しません。Alpine.jsはCSPのscript-src selfと標準ビルドが非互換のため、新しいFunctionでunsafe-evalを要求しないCSPビルド(@alpinejs/cspビルド)を採用し、app.jsはAlpine.dataでコンポーネントを登録し、テンプレートはインライン式でなく登録したメソッドとプロパティの参照にします。テンプレートと静的資産はembed.FSで単一バイナリに同梱します。go:embedが同パッケージ相対のため、テンプレートと静的資産はinternal/handler/templatesとinternal/handler/static配下に置き、それぞれをinternal/handler内のソースからembedします。CSSはremを基準にした流体レイアウトとし、固定のmax-widthに依存せずブレークポイントで列構成を調整します。記事の公開日時と取得日時はテンプレート関数でJSTに整形します。

Tech Stack: Goの標準ライブラリ(net/http、html/template、embed、net/http/httptest)、HTMX(ベンダーした静的ファイル)、Alpine.js(ベンダーした静的ファイル)、CSS(remベースの流体レイアウト)。

前提:
- 作業ディレクトリは`/Users/yujiokamoto/devs/golang/feedflow-go-htmx`です
- Phase4とPhase5でinternal/portの各サービスインターフェース(SubscriptionService、ItemService、RetentionService、MuteService、OPMLService、SettingsService、PollService)の実装が用意済みです
- Phase6でinternal/authのscryptパスワード検証、メモリ保持のCookieセッション、CSRFトークン、レートリミット、初回セットアップ可否判定が用意済みです
- Phase1のドメイン型とポート型はそのまま使います。型名やフィールド名やメソッドシグネチャは変更しません

このフェーズで追加する補助型(Phase1の定義と矛盾しない範囲で新設し、ここに明記します):
- `internal/handler`内に認証ポート`Sessions`、`CSRF`、`RateLimiter`、`SetupGuard`を定義します。これらはハンドラがinternal/authの具象に直接依存せずインターフェース経由で受けるための境界です。ポートのメソッドシグネチャはPhase6(07-auth.md)のinternal/authの公開メソッドに厳密に一致させます。具体的には`Sessions`はinternal/authの`*SessionStore`が満たし`Issue(w, username) error`と`Validate(r) (string, bool)`と`Destroy(w, r)`を持ちます。`CSRF`はinternal/authの`*CSRFStore`が満たし`Issue(sessionID) (string, error)`と`Token(sessionID) (string, bool)`と`Verify(sessionID, r) bool`と`Discard(sessionID)`を持ちます。`SetupGuard`はinternal/authの`*Manager`が満たし`NeedsSetup() (bool, error)`と`Setup(username, password) error`と`Authenticate(username, password) (bool, error)`を持ちます。`RateLimiter`はinternal/authのレートリミッタが満たし`Allow(key string) bool`を持ちます。これによりcmd結線時にinternal/authの具象がそのままDepsへ代入でき、コンパイルが通ります
- セッションIDの取得方法を明記します。internal/authの`*SessionStore`はCookie名を非公開フィールドに持つため、ハンドラはCookie値を直接読み取りません。代わりにハンドラ層の`Sessions`ポートへCookie名の解決を委ねず、internal/authが公開するCookie名定数`auth.SessionCookieName`をDepsへ渡し、ハンドラの`requireAuth`がそのCookie名でセッションIDを取り出します。CSRFトークンの照合と発行はこのセッションIDをキーに`CSRF.Verify(sessionID, r)`と`CSRF.Issue(sessionID)`で行います
- `internal/handler`内にビュー専用の表示モデル`pageData`と`itemView`と`feedTreeNode`を定義します。これらはテンプレートへ渡す描画用の構造体であり、ドメイン型を直接テンプレートに渡さず描画に必要な値だけを整形して渡すために使います
- `pageData.CSRFToken`の充填経路を明記します。internal/authのセッションはユーザー名しか保持しないため、ハンドラは画面描画の直前にセッションIDをキーとして`CSRF.Issue(sessionID)`を呼び、得たトークンを`pageData.CSRFToken`へ設定します。`requireAuth`がセッションIDとユーザー名とCSRFトークンをコンテキストへ載せ、各ハンドラはコンテキストから取り出してpageDataへ反映します

---

## Task 1: handlerパッケージの認証ポートと依存集約を定義する

Files:
- Create: `/Users/yujiokamoto/devs/golang/feedflow-go-htmx/internal/handler/ports.go`
- Create: `/Users/yujiokamoto/devs/golang/feedflow-go-htmx/internal/handler/handler.go`

ハンドラ層が認証とサービスの具象に依存せずインターフェース経由で受けられるよう、handler内に認証ポートと依存集約Depsを定義します。認証ポートのメソッドシグネチャはPhase6(07-auth.md)のinternal/authの公開メソッドに厳密に一致させ、internal/authの具象がそのままDepsへ代入できる形にします。Phase4とPhase5のサービスがport側を満たします。

- [ ] Step 1: 認証ポートを定義する

Create `internal/handler/ports.go`:
```go
// Package handler HTTPハンドラとHTMX部分更新と画面描画を提供します。
package handler

import (
	"net/http"
)

// Session 認証済みセッションの描画用情報を表します。
// internal/authのセッションはユーザー名しか保持しないため、CSRFTokenはハンドラが描画直前にCSRFポートから取得して充填します。
type Session struct {
	ID        string // セッションIDです。CSRFトークンの発行と照合のキーに使います
	Username  string // ログイン中のユーザー名です
	CSRFToken string // このセッションに紐づくCSRFトークンです。requireAuthがCSRF.Issueで充填します
}

// Sessions Cookieセッションの発行と検証と破棄を担う抽象です。
// 実装はPhase6のinternal/authの*SessionStoreが満たします。メソッドは07-auth.mdの公開シグネチャに一致させます。
type Sessions interface {
	// Issue ユーザー名に対するセッションを発行し、Set-Cookieをwへ書き込みます。
	Issue(w http.ResponseWriter, username string) error
	// Validate リクエストのCookieから有効なセッションのユーザー名を返します。無効ならokがfalseになります。
	Validate(r *http.Request) (string, bool)
	// Destroy リクエストのセッションを破棄し、失効用のCookieをwへ書き込みます。
	Destroy(w http.ResponseWriter, r *http.Request)
}

// CSRF セッションIDごとのCSRFトークンの発行と取得と検証と破棄を担う抽象です。
// 実装はPhase6のinternal/authの*CSRFStoreが満たします。メソッドは07-auth.mdの公開シグネチャに一致させます。
type CSRF interface {
	// Issue セッションIDに紐づくCSRFトークンを発行して保持し、その値を返します。既に発行済みなら同じ値を返します。
	Issue(sessionID string) (string, error)
	// Token セッションIDに紐づく現在のトークンを返します。未発行ならokがfalseになります。
	Token(sessionID string) (string, bool)
	// Verify リクエストの送信トークンがセッションIDの保持値と一致するかを定数時間比較で検証します。
	Verify(sessionID string, r *http.Request) bool
	// Discard セッションIDに紐づくトークンを破棄します。ログアウト時に呼びます。
	Discard(sessionID string)
}

// RateLimiter トークンバケットによるレート制限を担う抽象です。実装はPhase6のinternal/authが提供します。
type RateLimiter interface {
	// Allow 指定キーに対する1回分の許可を消費し、許可されたかどうかを返します。
	Allow(key string) bool
}

// SetupGuard 初回セットアップの可否判定と登録と認証を担う抽象です。
// 実装はPhase6のinternal/authの*Managerが満たします。メソッドは07-auth.mdの公開シグネチャに一致させます。
type SetupGuard interface {
	// NeedsSetup ユーザーが未登録で初回セットアップが必要かどうかを返します。
	NeedsSetup() (bool, error)
	// Setup 初回セットアップとしてユーザー名とパスワードを登録します。登録済みのときはエラーを返します。
	Setup(username, password string) error
	// Authenticate ユーザー名とパスワードを検証します。成功時にtrueを返します。
	Authenticate(username, password string) (bool, error)
}
```

補足: internal/authの`*SessionStore`はCookie名を非公開フィールドに持つため、ハンドラはセッションIDを取り出すためにinternal/authが公開するCookie名定数`auth.SessionCookieName`を必要とします。これはDepsの`SessionCookieName`フィールドで受け取り、`requireAuth`がそのCookie名でセッションIDを読み取ります。internal/authはこのCookie名定数を公開し、`NewSessionStore`へ渡す`SessionConfig.CookieName`と同じ値を使う前提です。

- [ ] Step 2: コンパイルが通ることを確認する

Run:
```bash
cd /Users/yujiokamoto/devs/golang/feedflow-go-htmx && go build ./internal/handler/
```
Expected: エラーなく完了します。

- [ ] Step 3: 依存集約Depsとコンストラクタを定義する

Create `internal/handler/handler.go`:
```go
package handler

import (
	"html/template"

	"github.com/okamyuji/feedflow-go-htmx/internal/port"
)

// Deps ハンドラが必要とする全依存をまとめた集約です。
// すべてインターフェースとして受け取り、具象型には直接依存しません。
type Deps struct {
	Subscriptions     port.SubscriptionService // 購読の追加と削除と一覧と整理を担います
	Items             port.ItemService         // 記事の既読やスターやあとで読むなどの操作を担います
	Retention         port.RetentionService    // 保持ポリシーの適用を担います
	Mutes             port.MuteService         // ミュートフィルタの管理と適用を担います
	OPML              port.OPMLService         // OPMLの入出力を担います
	Settings          port.SettingsService     // 設定の取得と更新を担います
	Poll              port.PollService         // フィードの取得反映を担います
	Sessions          Sessions                 // Cookieセッションの発行と検証と破棄を担います
	CSRF              CSRF                      // CSRFトークンの発行と検証を担います
	LoginLimiter      RateLimiter              // ログイン試行のレート制限を担います
	Setup             SetupGuard               // 初回セットアップの可否判定と登録を担います
	SessionCookieName string                   // セッションIDを読み取るCookie名です。auth.SessionCookieNameを渡します
	IsHTTPS           bool                      // 公開URLがhttpsかどうかです。HSTS付与の判定に使います
}

// Handler ルーティングとミドルウェアと画面描画を保持するハンドラ集約です。
type Handler struct {
	deps      Deps               // 注入された依存です
	templates *template.Template // ParseFSで読み込んだテンプレート集合です
}

// New 依存を受け取りHandlerを生成します。テンプレートは埋め込みFSから読み込みます。
func New(deps Deps) (*Handler, error) {
	tmpl, err := parseTemplates()
	if err != nil {
		return nil, err
	}
	return &Handler{deps: deps, templates: tmpl}, nil
}
```

- [ ] Step 4: gofmtを適用してコミットする

```bash
cd /Users/yujiokamoto/devs/golang/feedflow-go-htmx && gofmt -w internal/handler/ports.go internal/handler/handler.go && git add internal/handler/ports.go internal/handler/handler.go && git commit -m "feat: ハンドラの認証ポートと依存集約を追加する"
```

補足: この時点では`parseTemplates`が未定義のためコンパイルは通りません。次のTask2でテンプレート読み込みを実装して通します。Step4のコミットはTask2完了後にまとめて実行しても構いませんが、本計画では各Taskの末尾でコミットする方針を維持し、ここではTask2のStep完了後にビルドが通ることを確認します。

---

## Task 2: embedテンプレートのParseFSとFuncMapを実装する

Files:
- Create: `/Users/yujiokamoto/devs/golang/feedflow-go-htmx/internal/handler/render.go`
- Create: `/Users/yujiokamoto/devs/golang/feedflow-go-htmx/internal/handler/templates/base.html`
- Test: `/Users/yujiokamoto/devs/golang/feedflow-go-htmx/internal/handler/render_test.go`

embed.FSからテンプレートを読み込み、JST整形ほかのテンプレート関数を登録します。記事の公開日時と取得日時はAsia/Tokyoに変換して表示します。

- [ ] Step 1: 失敗するテストを書く

Create `internal/handler/render_test.go`:
```go
package handler

import (
	"strings"
	"testing"
	"time"
)

func TestFormatJST(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		in   time.Time
		want string
	}{
		{
			name: "UTC を JST に変換する",
			in:   time.Date(2026, 5, 29, 0, 0, 0, 0, time.UTC),
			want: "2026-05-29 09:00",
		},
		{
			name: "ゼロ値は空文字を返す",
			in:   time.Time{},
			want: "",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := formatJST(tt.in); got != tt.want {
				t.Fatalf("formatJST got %q want %q", got, tt.want)
			}
		})
	}
}

func TestParseTemplates(t *testing.T) {
	t.Parallel()
	tmpl, err := parseTemplates()
	if err != nil {
		t.Fatalf("parseTemplates returned error: %v", err)
	}
	if tmpl.Lookup("base.html") == nil {
		t.Fatalf("base.html template not found")
	}
}

func TestTruncateRunes(t *testing.T) {
	t.Parallel()
	if got := truncateRunes("あいうえお", 3); got != "あいう" {
		t.Fatalf("truncateRunes got %q want %q", got, "あいう")
	}
	if got := truncateRunes("ab", 5); got != "ab" {
		t.Fatalf("truncateRunes got %q want %q", got, "ab")
	}
}

func TestRenderPartialWritesBody(t *testing.T) {
	t.Parallel()
	tmpl, err := parseTemplates()
	if err != nil {
		t.Fatalf("parseTemplates returned error: %v", err)
	}
	var sb strings.Builder
	if err := tmpl.ExecuteTemplate(&sb, "base.html", pageData{Title: "feedflow"}); err != nil {
		t.Fatalf("ExecuteTemplate returned error: %v", err)
	}
	if !strings.Contains(sb.String(), "feedflow") {
		t.Fatalf("rendered body does not contain title: %q", sb.String())
	}
}
```

- [ ] Step 2: テストが失敗することを確認する

Run:
```bash
cd /Users/yujiokamoto/devs/golang/feedflow-go-htmx && go test ./internal/handler/ -run 'TestFormatJST|TestParseTemplates|TestTruncateRunes|TestRenderPartial' -v
```
Expected: コンパイルエラーで失敗します。`undefined: formatJST` や `undefined: parseTemplates` や `undefined: pageData` などが表示されます。

- [ ] Step 3: 描画用の表示モデルとテンプレート読み込みを実装する

Create `internal/handler/render.go`:
```go
package handler

import (
	"bytes"
	"embed"
	"fmt"
	"html/template"
	"log/slog"
	"net/http"
	"time"

	"github.com/okamyuji/feedflow-go-htmx/internal/domain"
)

//go:embed all:templates
var templatesFS embed.FS

// jst 日本標準時のロケーションです。初期化に失敗した場合は固定オフセットで代替します。
var jst = mustLoadJST()

// mustLoadJST Asia/Tokyoのロケーションを読み込みます。失敗時はUTC+9の固定オフセットを返します。
func mustLoadJST() *time.Location {
	loc, err := time.LoadLocation("Asia/Tokyo")
	if err != nil {
		return time.FixedZone("JST", 9*60*60)
	}
	return loc
}

// pageData base.htmlに渡す画面全体の描画モデルです。
type pageData struct {
	Title       string              // ブラウザのタイトルです
	Theme       domain.Theme        // 適用するテーマです
	CSRFToken   string              // フォームに埋め込むCSRFトークンです
	Username    string              // ログイン中のユーザー名です
	DefaultView domain.ViewMode     // 記事リストの既定表示形式です
	Tree        []feedTreeNode      // 左ペインの購読ツリーです
	Items       []itemView          // 右ペインの記事リストです
	ActiveItem  *itemView           // オーバーレイで開いている記事です
	Boards      []domain.Board      // ボード一覧です
	Filters     []domain.MuteFilter // ミュートフィルタ一覧です
	Settings    domain.Settings     // 設定画面で編集する設定です
	Flash       string              // 操作結果の通知メッセージです
	MainView    string              // フルページ描画時にmain-paneへ出す内容の種別です。空ならitem_list、settingsなら設定画面です
}

// feedTreeNode 左ペインの購読ツリーの1ノードを表します。
type feedTreeNode struct {
	Kind        string // ノード種別です(all、unread、starred、readlater、category、feed、boardのいずれか)
	ID          string // フィードやカテゴリやボードのIDです
	Label       string // 表示名です
	UnreadCount int    // 未読件数です
	HasError    bool   // フィードがエラー状態かどうかです
}

// itemView 右ペインとオーバーレイで描画する記事の表示モデルです。
type itemView struct {
	ID          string        // 記事IDです
	FeedID      string        // 所属フィードIDです
	Title       string        // タイトルです
	Link        string        // 元記事のURLです
	Summary     string        // 要約です
	Content     template.HTML // 本文です。html/templateの自動エスケープを経た安全な文字列だけを格納します
	Author      string        // 著者名です
	PublishedAt string        // JST整形済みの公開日時です
	Read        bool          // 既読かどうかです
	Starred     bool          // スター済みかどうかです
	ReadLater   bool          // あとで読む済みかどうかです
}

// formatJST 時刻をJSTに変換して"2006-01-02 15:04"形式で返します。ゼロ値は空文字を返します。
func formatJST(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.In(jst).Format("2006-01-02 15:04")
}

// truncateRunes 文字列をルーン単位でmax文字に切り詰めます。
func truncateRunes(s string, max int) string {
	r := []rune(s)
	if len(r) <= max {
		return s
	}
	return string(r[:max])
}

// templateFuncs テンプレートに登録する関数群を返します。
func templateFuncs() template.FuncMap {
	return template.FuncMap{
		"jst":      formatJST,
		"truncate": truncateRunes,
		"isDark": func(theme domain.Theme) bool {
			return theme == domain.ThemeDark
		},
	}
}

// parseTemplates 埋め込みFSから全テンプレートを読み込み、関数を登録した集合を返します。
func parseTemplates() (*template.Template, error) {
	tmpl, err := template.New("feedflow").Funcs(templateFuncs()).
		ParseFS(templatesFS, "templates/*.html")
	if err != nil {
		return nil, fmt.Errorf("failed to parse templates: %w", err)
	}
	return tmpl, nil
}

// renderPage base.htmlを完全なHTMLとして描画します。
func (h *Handler) renderPage(w http.ResponseWriter, status int, data pageData) {
	h.writeTemplate(w, status, "base.html", data)
}

// renderPartial 指定した部分テンプレートをHTMX向けに描画します。
func (h *Handler) renderPartial(w http.ResponseWriter, status int, name string, data any) {
	h.writeTemplate(w, status, name, data)
}

// writeTemplate テンプレートをバッファ経由で描画し、成功時にだけレスポンスへ書き込みます。
// 途中失敗で部分的なHTMLが露出しないように一旦バッファへ描画し、成功時のみステータスとボディを書き出します。
func (h *Handler) writeTemplate(w http.ResponseWriter, status int, name string, data any) {
	var buf bytes.Buffer
	if err := h.templates.ExecuteTemplate(&buf, name, data); err != nil {
		slog.Error("failed to execute template", "template", name, "error", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(status)
	if _, err := buf.WriteTo(w); err != nil {
		slog.Error("failed to write rendered template", "template", name, "error", err)
	}
}

// isHTMX HTMXのajaxリクエストかどうかをHX-Requestヘッダで判定します。
func isHTMX(r *http.Request) bool {
	return r.Header.Get("HX-Request") == "true"
}

// renderShellPage 左ペインのツリーを伴うフルページをbase.htmlで描画します。
// URL直アクセスやリロードや通常リンク遷移でレイアウトが欠落しないようにします。
// data.MainViewでmain-paneの内容を切り替えます。
func (h *Handler) renderShellPage(w http.ResponseWriter, sess Session, title string, data pageData) {
	tree, err := h.buildTree()
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	settings, err := h.deps.Settings.Get()
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	data.Tree = tree
	data.Title = title
	data.Username = sess.Username
	data.Theme = settings.Theme
	if data.Theme == domain.Theme("") {
		data.Theme = domain.ThemeDark
	}
	if data.DefaultView == domain.ViewMode("") {
		data.DefaultView = settings.DefaultView
	}
	h.renderPage(w, http.StatusOK, data)
}
```

補足: `templates/*.html`のグロブはアンダースコア始まりの部分テンプレートも含めて読み込みます。`//go:embed all:templates`はアンダースコア始まりのファイルもembedに含めるための指定です。go:embedは同パッケージ相対のため、このembedはinternal/handler/templates配下を対象にします。writeTemplateはまずbytes.Bufferへ描画し、テンプレート実行が成功したときだけステータスとボディを書き出すため、途中失敗で破損HTMLがクライアントへ送出されません。書き込みやテンプレート実行の失敗はfmt.Printfではなくslogで構造化記録します。`isHTMX`はHX-Requestヘッダで部分更新の要否を判定し、`renderShellPage`は左ペインのツリーと設定を取り込んでbase.htmlのフルページを描画します。各画面ハンドラは`isHTMX`がtrueなら部分テンプレートを返し、falseなら`renderShellPage`でフルページを返します。

- [ ] Step 4: 最小のbase.htmlを作成する

Create `internal/handler/templates/base.html`:
```html
<!doctype html>
<html lang="ja" data-theme="{{ if isDark .Theme }}dark{{ else }}light{{ end }}">
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width, initial-scale=1">
  <title>{{ .Title }}</title>
  <link rel="stylesheet" href="/static/styles.css">
  <script src="/static/htmx.min.js" defer></script>
  <script src="/static/app.js" defer></script>
  <script src="/static/alpine.min.js" defer></script>
</head>
<body>
  <h1>{{ .Title }}</h1>
</body>
</html>
```

補足: このbase.htmlはTask2でParseFSとExecuteTemplateを検証するための最小版です。Task6で2ペインとオーバーレイの完全なレイアウトに置き換えます。app.jsはAlpine.dataでコンポーネントを登録するため、alpine.min.jsより前に読み込み、alpine:initで登録が走るようにします。

- [ ] Step 5: テストが通ることを確認する

Run:
```bash
cd /Users/yujiokamoto/devs/golang/feedflow-go-htmx && go test ./internal/handler/ -run 'TestFormatJST|TestParseTemplates|TestTruncateRunes|TestRenderPartial' -v
```
Expected: 4件すべてPASSします。

- [ ] Step 6: gofmtを適用してコミットする

```bash
cd /Users/yujiokamoto/devs/golang/feedflow-go-htmx && gofmt -w internal/handler/render.go internal/handler/render_test.go && git add internal/handler/render.go internal/handler/render_test.go internal/handler/templates/base.html && git commit -m "feat: embed テンプレートの ParseFS と FuncMap と描画ヘルパを追加する"
```

---

## Task 3: 静的ファイルのベンダーとembed配信を実装する

Files:
- Create: `/Users/yujiokamoto/devs/golang/feedflow-go-htmx/internal/handler/static/htmx.min.js`
- Create: `/Users/yujiokamoto/devs/golang/feedflow-go-htmx/internal/handler/static/alpine.min.js`
- Create: `/Users/yujiokamoto/devs/golang/feedflow-go-htmx/internal/handler/static.go`
- Test: `/Users/yujiokamoto/devs/golang/feedflow-go-htmx/internal/handler/static_test.go`

HTMXとAlpine.jsはベンダーした静的ファイルをembedで同梱し、CDNに依存しません。embedの静的ファイルはhttp.FileServerがETagを付けないため、各ファイルのSHA256をETagにしてCache-Control no-cacheで配信し、内容が変わると再取得され、未変更ならIf-None-Matchで304を返します。これによりデプロイ後の資産更新が確実に反映されます。

- [ ] Step 1: HTMXをベンダーする

Run:
```bash
cd /Users/yujiokamoto/devs/golang/feedflow-go-htmx && curl -sSL -o internal/handler/static/htmx.min.js https://unpkg.com/htmx.org@2.0.4/dist/htmx.min.js && head -c 80 internal/handler/static/htmx.min.js && echo
```
Expected: ファイルが保存され、先頭にHTMXのミニファイ済みJavaScriptが表示されます。`(function(e,t)`のような関数式で始まります。

- [ ] Step 2: Alpine.jsをベンダーする

Alpine.jsの標準ビルドは新しいFunctionでunsafe-evalを要求し、CSPのscript-src selfと非互換のため、unsafe-evalを使わない@alpinejs/cspビルドをベンダーします。

Run:
```bash
cd /Users/yujiokamoto/devs/golang/feedflow-go-htmx && curl -sSL -o internal/handler/static/alpine.min.js https://unpkg.com/@alpinejs/csp@3.14.8/dist/cdn.min.js && head -c 80 internal/handler/static/alpine.min.js && echo
```
Expected: ファイルが保存され、先頭にAlpine.jsのCSPビルドのミニファイ済みJavaScriptが表示されます。

- [ ] Step 3: 失敗するテストを書く

Create `internal/handler/static_test.go`:
```go
package handler

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestStaticHandlerServesHTMX(t *testing.T) {
	t.Parallel()
	srv := staticHandler()
	req := httptest.NewRequest(http.MethodGet, "/static/htmx.min.js", nil)
	rec := httptest.NewRecorder()

	srv.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status got %d want %d", rec.Code, http.StatusOK)
	}
	if ct := rec.Header().Get("Content-Type"); ct == "" {
		t.Fatalf("Content-Type is empty")
	}
	if cc := rec.Header().Get("Cache-Control"); cc != "no-cache" {
		t.Fatalf("Cache-Control got %q want %q", cc, "no-cache")
	}
	if et := rec.Header().Get("ETag"); et == "" {
		t.Fatalf("ETag is empty")
	}
	if rec.Body.Len() == 0 {
		t.Fatalf("body is empty")
	}
}

func TestStaticHandlerReturnsNotModified(t *testing.T) {
	t.Parallel()
	srv := staticHandler()
	first := httptest.NewRequest(http.MethodGet, "/static/htmx.min.js", nil)
	firstRec := httptest.NewRecorder()
	srv.ServeHTTP(firstRec, first)
	etag := firstRec.Header().Get("ETag")
	if etag == "" {
		t.Fatalf("ETag is empty on first request")
	}

	second := httptest.NewRequest(http.MethodGet, "/static/htmx.min.js", nil)
	second.Header.Set("If-None-Match", etag)
	secondRec := httptest.NewRecorder()
	srv.ServeHTTP(secondRec, second)

	if secondRec.Code != http.StatusNotModified {
		t.Fatalf("status got %d want %d", secondRec.Code, http.StatusNotModified)
	}
}

func TestStaticHandlerNotFound(t *testing.T) {
	t.Parallel()
	srv := staticHandler()
	req := httptest.NewRequest(http.MethodGet, "/static/missing.js", nil)
	rec := httptest.NewRecorder()

	srv.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status got %d want %d", rec.Code, http.StatusNotFound)
	}
}
```

- [ ] Step 4: テストが失敗することを確認する

Run:
```bash
cd /Users/yujiokamoto/devs/golang/feedflow-go-htmx && go test ./internal/handler/ -run 'TestStatic' -v
```
Expected: コンパイルエラーで失敗します。`undefined: staticHandler` が表示されます。

- [ ] Step 5: 静的ファイル配信を実装する

Create `internal/handler/static.go`:
```go
package handler

import (
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"io"
	"io/fs"
	"net/http"
)

//go:embed static
var staticFS embed.FS

// staticHandler 埋め込みの静的ファイルを/static配下で配信するハンドラを返します。
// 各ファイルのコンテンツハッシュをETagに用い、内容が変わると再取得され、
// 未変更ならIf-None-Matchで304を返します。デプロイ後の資産更新が確実に反映されます。
func staticHandler() http.Handler {
	sub, err := fs.Sub(staticFS, "static")
	if err != nil {
		// embed対象が存在しないビルド構成は想定外のためpanicで早期に検出します。
		panic("failed to create static sub fs: " + err.Error())
	}
	etags := computeETags(sub)
	fileServer := http.FileServer(http.FS(sub))
	return http.StripPrefix("/static/", cacheControl(etags, fileServer))
}

// computeETags 配信対象の各ファイルのSHA256ハッシュからETag値のマップを作ります。
// キーはStripPrefix後のリクエストパス(先頭スラッシュなし)に合わせます。
// 起動時に1度だけ計算します。個別ファイルの読み取り失敗はETagなしで配信を続けます。
func computeETags(fsys fs.FS) map[string]string {
	etags := make(map[string]string)
	_ = fs.WalkDir(fsys, ".", func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil //nolint:nilerr // 個別の読み取り失敗は致命的でないため配信を継続します
		}
		f, openErr := fsys.Open(path)
		if openErr != nil {
			return nil
		}
		defer func() { _ = f.Close() }()
		h := sha256.New()
		if _, copyErr := io.Copy(h, f); copyErr != nil {
			return nil
		}
		etags[path] = `"` + hex.EncodeToString(h.Sum(nil))[:16] + `"`
		return nil
	})
	return etags
}

// cacheControl ETagによる再検証を行うミドルウェアです。
// 内容が変わると新しいETagになり再取得され、未変更ならIf-None-Matchで304を返します。
func cacheControl(etags map[string]string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if etag, ok := etags[r.URL.Path]; ok {
			w.Header().Set("ETag", etag)
			w.Header().Set("Cache-Control", "no-cache")
			if r.Header.Get("If-None-Match") == etag {
				w.WriteHeader(http.StatusNotModified)
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}
```

- [ ] Step 6: テストが通ることを確認する

Run:
```bash
cd /Users/yujiokamoto/devs/golang/feedflow-go-htmx && go test ./internal/handler/ -run 'TestStatic' -v
```
Expected: 3件すべてPASSします。

- [ ] Step 7: gofmtを適用してコミットする

```bash
cd /Users/yujiokamoto/devs/golang/feedflow-go-htmx && gofmt -w internal/handler/static.go internal/handler/static_test.go && git add internal/handler/static.go internal/handler/static_test.go internal/handler/static/htmx.min.js internal/handler/static/alpine.min.js && git commit -m "feat: HTMX と Alpine.js をベンダーし embed で配信する"
```

---

## Task 4: セキュリティヘッダとレートリミットとリクエストヘルパのミドルウェアを実装する

Files:
- Create: `/Users/yujiokamoto/devs/golang/feedflow-go-htmx/internal/handler/middleware.go`
- Test: `/Users/yujiokamoto/devs/golang/feedflow-go-htmx/internal/handler/middleware_test.go`

設計書セクション9.1のセキュリティヘッダ(HSTS、X-Content-Type-Options、Referrer-Policy、Permissions-Policy)を全レスポンスに付与し、認証とCSRFとレートリミットのミドルウェアを用意します。

- [ ] Step 1: 失敗するテストを書く

Create `internal/handler/middleware_test.go`:
```go
package handler

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// stubSessions 認証状態を固定で返すSessionsのスタブです。
// Validateはusernameとokを固定で返し、issuedは発行されたユーザー名を記録します。
type stubSessions struct {
	username string
	ok       bool
	issued   string
}

func (s *stubSessions) Issue(_ http.ResponseWriter, username string) error {
	s.issued = username
	return nil
}
func (s *stubSessions) Validate(_ *http.Request) (string, bool) { return s.username, s.ok }
func (s *stubSessions) Destroy(_ http.ResponseWriter, _ *http.Request) {}

// stubCSRF トークン発行と一致判定を固定で返すCSRFのスタブです。
type stubCSRF struct {
	ok    bool
	token string
}

func (c *stubCSRF) Issue(_ string) (string, error) { return c.token, nil }
func (c *stubCSRF) Token(_ string) (string, bool)  { return c.token, c.token != "" }
func (c *stubCSRF) Verify(_ string, _ *http.Request) bool { return c.ok }
func (c *stubCSRF) Discard(_ string)               {}

// stubLimiter 許可を固定で返すRateLimiterのスタブです。
type stubLimiter struct{ allow bool }

func (l stubLimiter) Allow(_ string) bool { return l.allow }

func TestSecurityHeadersAlwaysSetsBaseHeaders(t *testing.T) {
	t.Parallel()
	h := &Handler{}
	called := false
	next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	})
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()

	h.securityHeaders(next).ServeHTTP(rec, req)

	if !called {
		t.Fatalf("next handler was not called")
	}
	wantHeaders := map[string]string{
		"X-Content-Type-Options": "nosniff",
		"Referrer-Policy":        "strict-origin-when-cross-origin",
		"X-Frame-Options":        "DENY",
		"Permissions-Policy":     "camera=(), microphone=(), geolocation=()",
	}
	for k, want := range wantHeaders {
		if got := rec.Header().Get(k); got != want {
			t.Fatalf("header %s got %q want %q", k, got, want)
		}
	}
	if csp := rec.Header().Get("Content-Security-Policy"); !strings.Contains(csp, "form-action 'self'") {
		t.Fatalf("CSP should contain form-action 'self': %q", csp)
	}
}

func TestSecurityHeadersOmitsHSTSWhenNotHTTPS(t *testing.T) {
	t.Parallel()
	h := &Handler{deps: Deps{IsHTTPS: false}}
	next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) })
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()

	h.securityHeaders(next).ServeHTTP(rec, req)

	if got := rec.Header().Get("Strict-Transport-Security"); got != "" {
		t.Fatalf("HSTS got %q want empty when not https", got)
	}
}

func TestSecurityHeadersAddsHSTSWhenHTTPS(t *testing.T) {
	t.Parallel()
	h := &Handler{deps: Deps{IsHTTPS: true}}
	next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) })
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()

	h.securityHeaders(next).ServeHTTP(rec, req)

	if got := rec.Header().Get("Strict-Transport-Security"); got == "" {
		t.Fatalf("HSTS should be set when https")
	}
}

func TestRequireAuthRedirectsWhenUnauthenticated(t *testing.T) {
	t.Parallel()
	h := &Handler{deps: Deps{Sessions: &stubSessions{ok: false}, CSRF: &stubCSRF{token: "tok"}, SessionCookieName: "feedflow_session"}}
	next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	req := httptest.NewRequest(http.MethodGet, "/app", nil)
	rec := httptest.NewRecorder()

	h.requireAuth(next).ServeHTTP(rec, req)

	if rec.Code != http.StatusSeeOther {
		t.Fatalf("status got %d want %d", rec.Code, http.StatusSeeOther)
	}
	if loc := rec.Header().Get("Location"); loc != "/login" {
		t.Fatalf("Location got %q want %q", loc, "/login")
	}
}

func TestRequireAuthPassesWhenAuthenticated(t *testing.T) {
	t.Parallel()
	h := &Handler{deps: Deps{
		Sessions:          &stubSessions{username: "owner", ok: true},
		CSRF:              &stubCSRF{token: "csrf-value"},
		SessionCookieName: "feedflow_session",
	}}
	gotUser := ""
	gotToken := ""
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sess := sessionFromContext(r.Context())
		gotUser = sess.Username
		gotToken = sess.CSRFToken
		w.WriteHeader(http.StatusOK)
	})
	req := httptest.NewRequest(http.MethodGet, "/app", nil)
	req.AddCookie(&http.Cookie{Name: "feedflow_session", Value: "sess-1"})
	rec := httptest.NewRecorder()

	h.requireAuth(next).ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status got %d want %d", rec.Code, http.StatusOK)
	}
	if gotUser != "owner" {
		t.Fatalf("session username got %q want %q", gotUser, "owner")
	}
	if gotToken != "csrf-value" {
		t.Fatalf("session CSRFToken got %q want %q", gotToken, "csrf-value")
	}
}

func TestRequireCSRFRejectsBadToken(t *testing.T) {
	t.Parallel()
	h := &Handler{deps: Deps{
		Sessions:          &stubSessions{username: "owner", ok: true},
		CSRF:              &stubCSRF{ok: false, token: "good"},
		SessionCookieName: "feedflow_session",
	}}
	next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	req := httptest.NewRequest(http.MethodPost, "/app/items/mark", nil)
	req.AddCookie(&http.Cookie{Name: "feedflow_session", Value: "sess-1"})
	rec := httptest.NewRecorder()

	h.requireCSRF(next).ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("status got %d want %d", rec.Code, http.StatusForbidden)
	}
}
```

- [ ] Step 2: テストが失敗することを確認する

Run:
```bash
cd /Users/yujiokamoto/devs/golang/feedflow-go-htmx && go test ./internal/handler/ -run 'TestSecurityHeaders|TestRequireAuth|TestRequireCSRF' -v
```
Expected: コンパイルエラーで失敗します。`undefined: (*Handler).securityHeaders` などが表示されます。

- [ ] Step 3: ミドルウェアとコンテキストヘルパを実装する

Create `internal/handler/middleware.go`:
```go
package handler

import (
	"context"
	"net"
	"net/http"
	"strings"
)

// ctxKey コンテキストに値を格納するためのキー型です。
type ctxKey int

const sessionCtxKey ctxKey = iota

// hstsValue 1年間のHSTSとサブドメインとプリロードを指示します。internal/authのhstsValueと文言を一致させます。
const hstsValue = "max-age=31536000; includeSubDomains; preload"

// contentSecurityPolicy feedflowのCSPです。internal/authのcontentSecurityPolicyと文言を一致させ二重定義の不整合を避けます。
// HTMXとAlpine.jsをベンダーしてselfから配信するためscript-srcはselfに限定します。
// Alpine.jsは標準ビルドがunsafe-evalを要求しselfと非互換のため、unsafe-evalを使わないCSPビルドを採用してscript-src selfを維持します。
// Alpine.jsのインライン属性を許すためstyle-srcにunsafe-inlineを含めます。
// form-action 'self'でフォーム送信先を自オリジンに限定し、注入時の外部送信を防ぎます。
const contentSecurityPolicy = "default-src 'self'; " +
	"script-src 'self'; " +
	"style-src 'self' 'unsafe-inline'; " +
	"img-src 'self' data: https:; " +
	"font-src 'self'; " +
	"connect-src 'self'; " +
	"frame-ancestors 'none'; " +
	"base-uri 'self'; " +
	"form-action 'self'; " +
	"object-src 'none'"

// permissionsPolicy 不要なブラウザ機能を一括で無効化します。
const permissionsPolicy = "camera=(), microphone=(), geolocation=()"

// sessionFromContext コンテキストからセッションを取り出します。未設定ならゼロ値を返します。
func sessionFromContext(ctx context.Context) Session {
	sess, _ := ctx.Value(sessionCtxKey).(Session)
	return sess
}

// withSession セッションをコンテキストへ格納したリクエストを返します。
func withSession(r *http.Request, sess Session) *http.Request {
	ctx := context.WithValue(r.Context(), sessionCtxKey, sess)
	return r.WithContext(ctx)
}

// sessionID リクエストのCookieからセッションIDを取り出します。Cookie名はDeps.SessionCookieNameを使います。
func (h *Handler) sessionID(r *http.Request) (string, bool) {
	c, err := r.Cookie(h.deps.SessionCookieName)
	if err != nil {
		return "", false
	}
	return c.Value, true
}

// securityHeaders 全レスポンスにセキュリティヘッダを付与します。設計書のセクション9.1に対応します。
// HSTSはhttps公開時(Deps.IsHTTPS)にだけ付与し、平文の開発アクセスでHTTPS強制が起きるのを防ぎます。
func (h *Handler) securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		head := w.Header()
		head.Set("X-Content-Type-Options", "nosniff")
		head.Set("Referrer-Policy", "strict-origin-when-cross-origin")
		head.Set("X-Frame-Options", "DENY")
		head.Set("Permissions-Policy", permissionsPolicy)
		head.Set("Content-Security-Policy", contentSecurityPolicy)
		if h.deps.IsHTTPS {
			head.Set("Strict-Transport-Security", hstsValue)
		}
		next.ServeHTTP(w, r)
	})
}

// requireAuth 未認証アクセスを/loginへリダイレクトし、認証済みならセッションをコンテキストへ載せます。
// 認証済みのときはセッションIDをキーにCSRFトークンを発行し、Sessionへ充填してコンテキストへ載せます。
func (h *Handler) requireAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		username, ok := h.deps.Sessions.Validate(r)
		if !ok {
			http.Redirect(w, r, "/login", http.StatusSeeOther)
			return
		}
		id, _ := h.sessionID(r)
		token, err := h.deps.CSRF.Issue(id)
		if err != nil {
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		sess := Session{ID: id, Username: username, CSRFToken: token}
		next.ServeHTTP(w, withSession(r, sess))
	})
}

// requireCSRF 状態変更系のPOSTに対しCSRFトークンを検証します。設計書のセクション9.1に対応します。
// 照合はセッションIDをキーにCSRF.Verifyへ委ね、ヘッダとフォーム値の両方をサポートします。
func (h *Handler) requireCSRF(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		username, ok := h.deps.Sessions.Validate(r)
		if !ok {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		id, hasID := h.sessionID(r)
		if !hasID || !h.deps.CSRF.Verify(id, r) {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		token, _ := h.deps.CSRF.Token(id)
		sess := Session{ID: id, Username: username, CSRFToken: token}
		next.ServeHTTP(w, withSession(r, sess))
	})
}

// rateLimitLogin ログイン試行をクライアントIP単位でレート制限します。設計書のセクション9.1に対応します。
func (h *Handler) rateLimitLogin(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !h.deps.LoginLimiter.Allow(clientKey(r)) {
			http.Error(w, "too many requests", http.StatusTooManyRequests)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// clientKey レート制限のキーをクライアントIPから生成します。
// 前段のnginxがリバースプロキシするため、信頼できる前段が付与するX-Real-IPを優先します。
// 無ければX-Forwarded-Forの先頭ホップ、さらに無ければRemoteAddrへフォールバックします。
// X-Real-IPとX-Forwarded-Forはnginxが上書き付与する前提で、直接公開時はスプーフィングされうる点に注意します。
func clientKey(r *http.Request) string {
	if ip := strings.TrimSpace(r.Header.Get("X-Real-IP")); ip != "" {
		return ip
	}
	if fwd := r.Header.Get("X-Forwarded-For"); fwd != "" {
		parts := strings.Split(fwd, ",")
		if first := strings.TrimSpace(parts[0]); first != "" {
			return first
		}
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}
```

補足: `requireCSRF`はinternal/authの`*CSRFStore.Verify`がヘッダとフォーム値の両方を見るため、ハンドラ側でトークンの抽出をやり直さずセッションIDだけを渡します。リクエストのbodyをVerify内でPostFormValueが読むため、後続ハンドラでParseFormを呼ぶ前提のフォームでも検証は機能します。

- [ ] Step 4: テストが通ることを確認する

Run:
```bash
cd /Users/yujiokamoto/devs/golang/feedflow-go-htmx && go test ./internal/handler/ -run 'TestSecurityHeaders|TestRequireAuth|TestRequireCSRF' -v
```
Expected: 5件すべてPASSします。

- [ ] Step 5: gofmtを適用してコミットする

```bash
cd /Users/yujiokamoto/devs/golang/feedflow-go-htmx && gofmt -w internal/handler/middleware.go internal/handler/middleware_test.go && git add internal/handler/middleware.go internal/handler/middleware_test.go && git commit -m "feat: セキュリティヘッダと認証と CSRF とレートリミットのミドルウェアを追加する"
```

---

## Task 5: ログインと初回セットアップのハンドラを実装する

Files:
- Create: `/Users/yujiokamoto/devs/golang/feedflow-go-htmx/internal/handler/auth_handler.go`
- Create: `/Users/yujiokamoto/devs/golang/feedflow-go-htmx/internal/handler/templates/login.html`
- Create: `/Users/yujiokamoto/devs/golang/feedflow-go-htmx/internal/handler/templates/setup.html`
- Test: `/Users/yujiokamoto/devs/golang/feedflow-go-htmx/internal/handler/auth_handler_test.go`

設計書セクション9.3のとおり、user.jsonが未登録のときだけ初回セットアップ画面へ到達でき、登録済みでは無効化します。ログイン成功でセッションを発行し、アプリ画面へリダイレクトします。

- [ ] Step 1: ログインとセットアップのテンプレートを作成する

Create `internal/handler/templates/login.html`:
```html
{{ define "login.html" }}
<!doctype html>
<html lang="ja" data-theme="dark">
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width, initial-scale=1">
  <title>ログイン feedflow</title>
  <link rel="stylesheet" href="/static/styles.css">
</head>
<body class="auth-page">
  <main class="auth-card">
    <h1 class="auth-title">feedflow</h1>
    {{ if .Flash }}<p class="auth-error" role="alert">{{ .Flash }}</p>{{ end }}
    <form method="post" action="/login" class="auth-form">
      <label class="field">
        <span class="field-label">ユーザー名</span>
        <input class="field-input" type="text" name="username" autocomplete="username" required>
      </label>
      <label class="field">
        <span class="field-label">パスワード</span>
        <input class="field-input" type="password" name="password" autocomplete="current-password" required>
      </label>
      <button class="btn-primary" type="submit">ログイン</button>
    </form>
  </main>
</body>
</html>
{{ end }}
```

Create `internal/handler/templates/setup.html`:
```html
{{ define "setup.html" }}
<!doctype html>
<html lang="ja" data-theme="dark">
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width, initial-scale=1">
  <title>初回セットアップ feedflow</title>
  <link rel="stylesheet" href="/static/styles.css">
</head>
<body class="auth-page">
  <main class="auth-card">
    <h1 class="auth-title">初回セットアップ</h1>
    <p class="auth-lead">所有者のユーザー名とパスワードを登録します。</p>
    {{ if .Flash }}<p class="auth-error" role="alert">{{ .Flash }}</p>{{ end }}
    <form method="post" action="/setup" class="auth-form">
      <label class="field">
        <span class="field-label">ユーザー名</span>
        <input class="field-input" type="text" name="username" autocomplete="username" required>
      </label>
      <label class="field">
        <span class="field-label">パスワード</span>
        <input class="field-input" type="password" name="password" autocomplete="new-password" minlength="8" required>
      </label>
      <button class="btn-primary" type="submit">登録する</button>
    </form>
  </main>
</body>
</html>
{{ end }}
```

- [ ] Step 2: 失敗するテストを書く

Create `internal/handler/auth_handler_test.go`:
```go
package handler

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

// stubSetup 初回セットアップの状態を固定で返すSetupGuardのスタブです。
type stubSetup struct {
	needs        bool
	authOK       bool
	setupErr     error
	registered   bool
	lastUsername string
}

func (s *stubSetup) NeedsSetup() (bool, error) { return s.needs, nil }
func (s *stubSetup) Setup(username, _ string) error {
	if s.setupErr != nil {
		return s.setupErr
	}
	s.registered = true
	s.lastUsername = username
	s.needs = false
	return nil
}
func (s *stubSetup) Authenticate(_, _ string) (bool, error) { return s.authOK, nil }

func newAuthHandler(t *testing.T, setup *stubSetup, sessions Sessions, limiter RateLimiter) *Handler {
	t.Helper()
	h, err := New(Deps{Setup: setup, Sessions: sessions, LoginLimiter: limiter})
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}
	return h
}

func TestLoginPageRedirectsToSetupWhenNeeded(t *testing.T) {
	t.Parallel()
	h := newAuthHandler(t, &stubSetup{needs: true}, &stubSessions{}, stubLimiter{allow: true})
	req := httptest.NewRequest(http.MethodGet, "/login", nil)
	rec := httptest.NewRecorder()

	h.loginPage(rec, req)

	if rec.Code != http.StatusSeeOther {
		t.Fatalf("status got %d want %d", rec.Code, http.StatusSeeOther)
	}
	if loc := rec.Header().Get("Location"); loc != "/setup" {
		t.Fatalf("Location got %q want %q", loc, "/setup")
	}
}

func TestLoginSubmitSuccess(t *testing.T) {
	t.Parallel()
	sessions := &stubSessions{}
	h := newAuthHandler(t, &stubSetup{needs: false, authOK: true}, sessions, stubLimiter{allow: true})
	form := url.Values{"username": {"owner"}, "password": {"correct-password"}}
	req := httptest.NewRequest(http.MethodPost, "/login", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()

	h.loginSubmit(rec, req)

	if rec.Code != http.StatusSeeOther {
		t.Fatalf("status got %d want %d", rec.Code, http.StatusSeeOther)
	}
	if loc := rec.Header().Get("Location"); loc != "/app" {
		t.Fatalf("Location got %q want %q", loc, "/app")
	}
	if sessions.issued != "owner" {
		t.Fatalf("issued session username got %q want %q", sessions.issued, "owner")
	}
}

func TestLoginSubmitFailureShowsError(t *testing.T) {
	t.Parallel()
	h := newAuthHandler(t, &stubSetup{needs: false, authOK: false}, &stubSessions{}, stubLimiter{allow: true})
	form := url.Values{"username": {"owner"}, "password": {"wrong"}}
	req := httptest.NewRequest(http.MethodPost, "/login", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()

	h.loginSubmit(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status got %d want %d", rec.Code, http.StatusUnauthorized)
	}
	if !strings.Contains(rec.Body.String(), "ユーザー名またはパスワード") {
		t.Fatalf("body does not contain error message: %q", rec.Body.String())
	}
}

func TestSetupSubmitRegistersAndRedirects(t *testing.T) {
	t.Parallel()
	setup := &stubSetup{needs: true}
	h := newAuthHandler(t, setup, &stubSessions{}, stubLimiter{allow: true})
	form := url.Values{"username": {"owner"}, "password": {"strong-password"}}
	req := httptest.NewRequest(http.MethodPost, "/setup", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()

	h.setupSubmit(rec, req)

	if rec.Code != http.StatusSeeOther {
		t.Fatalf("status got %d want %d", rec.Code, http.StatusSeeOther)
	}
	if !setup.registered {
		t.Fatalf("setup was not registered")
	}
	if loc := rec.Header().Get("Location"); loc != "/login" {
		t.Fatalf("Location got %q want %q", loc, "/login")
	}
}

func TestSetupSubmitBlockedWhenAlreadyRegistered(t *testing.T) {
	t.Parallel()
	setup := &stubSetup{needs: false}
	h := newAuthHandler(t, setup, &stubSessions{}, stubLimiter{allow: true})
	form := url.Values{"username": {"intruder"}, "password": {"whatever1"}}
	req := httptest.NewRequest(http.MethodPost, "/setup", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()

	h.setupSubmit(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("status got %d want %d", rec.Code, http.StatusForbidden)
	}
	if setup.registered {
		t.Fatalf("setup must not register when already registered")
	}
}
```

- [ ] Step 3: テストが失敗することを確認する

Run:
```bash
cd /Users/yujiokamoto/devs/golang/feedflow-go-htmx && go test ./internal/handler/ -run 'TestLogin|TestSetup' -v
```
Expected: コンパイルエラーで失敗します。`undefined: (*Handler).loginPage` などが表示されます。

- [ ] Step 4: 認証ハンドラを実装する

Create `internal/handler/auth_handler.go`:
```go
package handler

import (
	"net/http"
)

// loginPage ログイン画面を表示します。初回セットアップが必要なら/setupへ誘導します。
func (h *Handler) loginPage(w http.ResponseWriter, r *http.Request) {
	needs, err := h.deps.Setup.NeedsSetup()
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	if needs {
		http.Redirect(w, r, "/setup", http.StatusSeeOther)
		return
	}
	h.writeTemplate(w, http.StatusOK, "login.html", pageData{Title: "ログイン feedflow"})
}

// loginSubmit ログインフォームを検証し、成功でセッションを発行してアプリ画面へ遷移します。
func (h *Handler) loginSubmit(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	username := r.FormValue("username")
	password := r.FormValue("password")
	ok, err := h.deps.Setup.Authenticate(username, password)
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	if !ok {
		h.writeTemplate(w, http.StatusUnauthorized, "login.html",
			pageData{Title: "ログイン feedflow", Flash: "ユーザー名またはパスワードが違います"})
		return
	}
	if err := h.deps.Sessions.Issue(w, username); err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	http.Redirect(w, r, "/app", http.StatusSeeOther)
}

// logout セッションを破棄してログイン画面へ戻します。CSRFトークンもセッションIDをキーに破棄します。
func (h *Handler) logout(w http.ResponseWriter, r *http.Request) {
	if id, ok := h.sessionID(r); ok {
		h.deps.CSRF.Discard(id)
	}
	h.deps.Sessions.Destroy(w, r)
	http.Redirect(w, r, "/login", http.StatusSeeOther)
}

// setupPage 初回セットアップ画面を表示します。登録済みなら無効化してログインへ戻します。
func (h *Handler) setupPage(w http.ResponseWriter, r *http.Request) {
	needs, err := h.deps.Setup.NeedsSetup()
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	if !needs {
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}
	h.writeTemplate(w, http.StatusOK, "setup.html", pageData{Title: "初回セットアップ feedflow"})
}

// setupSubmit 初回セットアップを登録します。登録済みの状態では拒否します。
func (h *Handler) setupSubmit(w http.ResponseWriter, r *http.Request) {
	needs, err := h.deps.Setup.NeedsSetup()
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	if !needs {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	username := r.FormValue("username")
	password := r.FormValue("password")
	if username == "" || len(password) < 8 {
		h.writeTemplate(w, http.StatusBadRequest, "setup.html",
			pageData{Title: "初回セットアップ feedflow", Flash: "ユーザー名と8文字以上のパスワードを入力してください"})
		return
	}
	if err := h.deps.Setup.Setup(username, password); err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	http.Redirect(w, r, "/login", http.StatusSeeOther)
}
```

補足: ログインとセットアップの両画面はセッション発行前のためCSRFトークンを埋め込みません。状態変更POSTはログインフォームとセットアップフォームの2つだけで、いずれもセッション確立前のフローのためrequireCSRFの対象外とし、初回セットアップは登録済み判定で多重実行を防ぎます。loginとsetupのテンプレートはCSRFトークンを参照しません。

- [ ] Step 5: テストが通ることを確認する

Run:
```bash
cd /Users/yujiokamoto/devs/golang/feedflow-go-htmx && go test ./internal/handler/ -run 'TestLogin|TestSetup' -v
```
Expected: 5件すべてPASSします。

- [ ] Step 6: gofmtを適用してコミットする

```bash
cd /Users/yujiokamoto/devs/golang/feedflow-go-htmx && gofmt -w internal/handler/auth_handler.go internal/handler/auth_handler_test.go && git add internal/handler/auth_handler.go internal/handler/auth_handler_test.go internal/handler/templates/login.html internal/handler/templates/setup.html && git commit -m "feat: ログインと初回セットアップのハンドラと画面を追加する"
```

---

## Task 6: 2ペインとオーバーレイのレイアウトと部分テンプレートを作成する

Files:
- Modify: `/Users/yujiokamoto/devs/golang/feedflow-go-htmx/internal/handler/templates/base.html`
- Create: `/Users/yujiokamoto/devs/golang/feedflow-go-htmx/internal/handler/templates/_tree.html`
- Create: `/Users/yujiokamoto/devs/golang/feedflow-go-htmx/internal/handler/templates/_item_list.html`
- Create: `/Users/yujiokamoto/devs/golang/feedflow-go-htmx/internal/handler/templates/_item_overlay.html`

レイアウトB(2ペインとリーディングオーバーレイ)の完全なレイアウトに置き換えます。左に購読ツリー、右に記事リストを置き、記事を開くと本文が手前にオーバーレイ表示されます。Alpine.jsでオーバーレイの開閉とテーマ切替とキーボードショートカットを扱います。

- [ ] Step 1: base.htmlを完全なレイアウトに置き換える

Replace `internal/handler/templates/base.html` with:
```html
{{ define "base.html" }}
<!doctype html>
<html lang="ja" data-theme="{{ if isDark .Theme }}dark{{ else }}light{{ end }}">
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width, initial-scale=1">
  <title>{{ .Title }}</title>
  <link rel="icon" href="data:,">
  <meta name="csrf-token" content="{{ .CSRFToken }}">
  <link rel="stylesheet" href="/static/styles.css">
  <script src="/static/htmx.min.js" defer></script>
  <script src="/static/app.js" defer></script>
  <script src="/static/alpine.min.js" defer></script>
</head>
<body
  x-data="feedflow"
  @keydown.window="onKey"
  hx-headers='{"X-CSRF-Token": "{{ .CSRFToken }}"}'
>
  <div class="app-shell">
    <header class="app-bar">
      <span class="app-brand">feedflow</span>
      <div class="app-actions">
        <button class="icon-btn" @click="toggleTheme" aria-label="テーマ切替">
          <span x-text="themeLabel"></span>
        </button>
        <a class="icon-btn" href="/app/settings" hx-get="/app/settings" hx-target="#main-pane" hx-push-url="true">設定</a>
        <form method="post" action="/logout" class="inline-form">
          <input type="hidden" name="csrf_token" value="{{ .CSRFToken }}">
          <button class="icon-btn" type="submit">ログアウト</button>
        </form>
      </div>
    </header>

    <div class="app-body">
      <aside class="tree-pane" id="tree-pane">
        {{ template "_tree.html" . }}
      </aside>

      <main class="main-pane" id="main-pane">
        {{ if eq .MainView "settings" }}{{ template "_settings.html" . }}{{ else }}{{ template "_item_list.html" . }}{{ end }}
      </main>
    </div>
  </div>

  <div
    class="overlay-backdrop"
    x-show="overlayOpen"
    x-transition.opacity
    @click="closeOverlay"
    style="display:none"
  ></div>
  <section
    class="reading-overlay"
    id="reading-overlay"
    x-show="overlayOpen"
    x-transition
    @scroll.debounce.300ms="onOverlayScroll"
    style="display:none"
  >
    {{ if .ActiveItem }}{{ template "_item_overlay.html" .ActiveItem }}{{ end }}
  </section>
</body>
</html>
{{ end }}
```

補足: Alpine.jsのCSPビルドはインライン式を評価せず、登録済みのメソッドとプロパティの参照だけを許します。そのため`x-data="feedflow"`はコンポーネント名を指すだけにし、初期化はapp.jsのinitで行い、`@keydown.window="onKey"`や`@click="toggleTheme"`や`@click="closeOverlay"`や`@scroll...="onOverlayScroll"`はメソッド名のみを指定し、`$event`や引数を式で渡しません。テーマのラベルは`x-text="themeLabel"`のように算出プロパティを参照し、三項演算のインライン式を書きません。app.jsはalpine.min.jsより前に読み込み、`alpine:init`でfeedflowコンポーネントを登録します。main-paneは`.MainView`が"settings"なら設定画面を、それ以外なら記事一覧をフルページに埋め込みます。

- [ ] Step 2: 購読ツリーの部分テンプレートを作成する

Create `internal/handler/templates/_tree.html`:
```html
{{ define "_tree.html" }}
<nav class="tree" aria-label="購読ツリー">
  <ul class="tree-list">
    {{ range .Tree }}
    <li class="tree-item tree-{{ .Kind }}{{ if .HasError }} tree-error{{ end }}">
      <a
        class="tree-link"
        href="/app/items?{{ if eq .Kind "feed" }}feed={{ .ID }}{{ else if eq .Kind "category" }}category={{ .ID }}{{ else if eq .Kind "board" }}board={{ .ID }}{{ else }}view={{ .Kind }}{{ end }}"
        hx-get="/app/items?{{ if eq .Kind "feed" }}feed={{ .ID }}{{ else if eq .Kind "category" }}category={{ .ID }}{{ else if eq .Kind "board" }}board={{ .ID }}{{ else }}view={{ .Kind }}{{ end }}"
        hx-target="#main-pane"
        hx-push-url="true"
      >
        <span class="tree-label">{{ .Label }}</span>
        {{ if gt .UnreadCount 0 }}<span class="tree-badge">{{ .UnreadCount }}</span>{{ end }}
        {{ if .HasError }}<span class="tree-error-mark" title="取得エラー">!</span>{{ end }}
      </a>
    </li>
    {{ end }}
  </ul>

  <form
    class="subscribe-form"
    hx-post="/app/feeds"
    hx-target="#tree-pane"
    hx-swap="outerHTML"
  >
    <input type="hidden" name="csrf_token" value="{{ .CSRFToken }}">
    <input class="field-input" type="url" name="url" placeholder="フィードURLまたはサイトURL" required>
    <button class="btn-primary" type="submit">購読する</button>
  </form>
</nav>
{{ end }}
```

- [ ] Step 3: 記事リストの部分テンプレートを作成する

Create `internal/handler/templates/_item_list.html`:
```html
{{ define "_item_list.html" }}
<div class="item-list" data-view="{{ .DefaultView }}">
  <div class="item-list-bar">
    <form method="post" action="/app/items/markall" class="inline-form" hx-post="/app/items/markall" hx-target="#main-pane">
      <input type="hidden" name="csrf_token" value="{{ .CSRFToken }}">
      <button class="btn-ghost" type="submit">全既読</button>
    </form>
  </div>
  <ul class="item-cards">
    {{ range .Items }}
    <li
      class="item-card{{ if .Read }} is-read{{ end }}"
      id="item-{{ .ID }}"
      data-feed="{{ .FeedID }}"
      data-item="{{ .ID }}"
    >
      <a
        class="item-open"
        href="{{ .Link }}"
        hx-get="/app/items/{{ .FeedID }}/{{ .ID }}"
        hx-target="#reading-overlay"
        hx-swap="innerHTML"
        @click.prevent="openOverlay"
      >
        <h3 class="item-title">{{ .Title }}</h3>
        <p class="item-summary">{{ .Summary }}</p>
        <div class="item-meta">
          <time>{{ .PublishedAt }}</time>
          {{ if .Starred }}<span class="item-star">保存済み</span>{{ end }}
        </div>
      </a>
      <div class="item-quick">
        <button
          class="quick-btn"
          hx-post="/app/items/{{ .FeedID }}/{{ .ID }}/star"
          hx-vals='{"starred": "{{ if .Starred }}false{{ else }}true{{ end }}"}'
          hx-target="#item-{{ .ID }}"
          hx-swap="outerHTML"
        >{{ if .Starred }}スター解除{{ else }}スター{{ end }}</button>
        <button
          class="quick-btn"
          hx-post="/app/items/{{ .FeedID }}/{{ .ID }}/read"
          hx-vals='{"read": "{{ if .Read }}false{{ else }}true{{ end }}"}'
          hx-target="#item-{{ .ID }}"
          hx-swap="outerHTML"
        >{{ if .Read }}未読に戻す{{ else }}既読{{ end }}</button>
      </div>
    </li>
    {{ end }}
  </ul>
</div>
{{ end }}
```

- [ ] Step 4: 記事本文オーバーレイの部分テンプレートを作成する

Create `internal/handler/templates/_item_overlay.html`:
```html
{{ define "_item_overlay.html" }}
<article class="reading-article" data-feed="{{ .FeedID }}" data-item="{{ .ID }}">
  <header class="reading-head">
    <button class="icon-btn" @click="closeOverlay" aria-label="閉じる">閉じる</button>
    <div class="reading-actions">
      <button
        class="quick-btn"
        hx-post="/app/items/{{ .FeedID }}/{{ .ID }}/star"
        hx-vals='{"starred": "{{ if .Starred }}false{{ else }}true{{ end }}"}'
        hx-swap="none"
      >{{ if .Starred }}スター解除{{ else }}スター{{ end }}</button>
      <button
        class="quick-btn"
        hx-post="/app/items/{{ .FeedID }}/{{ .ID }}/readlater"
        hx-vals='{"read_later": "{{ if .ReadLater }}false{{ else }}true{{ end }}"}'
        hx-swap="none"
      >{{ if .ReadLater }}あとで読む解除{{ else }}あとで読む{{ end }}</button>
      <a class="quick-btn" href="{{ .Link }}" target="_blank" rel="noopener noreferrer">元記事</a>
    </div>
  </header>
  <h1 class="reading-title">{{ .Title }}</h1>
  <div class="reading-meta">
    <span>{{ .Author }}</span>
    <time>{{ .PublishedAt }}</time>
  </div>
  <div class="reading-body">{{ .Content }}</div>
</article>
{{ end }}
```

- [ ] Step 5: テンプレートが読み込めることを確認する

Run:
```bash
cd /Users/yujiokamoto/devs/golang/feedflow-go-htmx && go test ./internal/handler/ -run 'TestParseTemplates|TestRenderPartial' -v
```
Expected: 2件ともPASSします。ParseFSが全テンプレートを問題なく読み込めることを確認します。

- [ ] Step 6: コミットする

```bash
cd /Users/yujiokamoto/devs/golang/feedflow-go-htmx && git add internal/handler/templates/base.html internal/handler/templates/_tree.html internal/handler/templates/_item_list.html internal/handler/templates/_item_overlay.html && git commit -m "feat: 2ペインとリーディングオーバーレイのレイアウトと部分テンプレートを追加する"
```

---

## Task 7: 購読の追加削除一覧のハンドラを実装する

Files:
- Create: `/Users/yujiokamoto/devs/golang/feedflow-go-htmx/internal/handler/feed_handler.go`
- Test: `/Users/yujiokamoto/devs/golang/feedflow-go-htmx/internal/handler/feed_handler_test.go`

設計書セクション3.1のとおり、フィードURLの登録による購読追加、サイトURLからのフィード自動検出、購読の削除と一覧を実装します。サービスはport.SubscriptionServiceのインターフェース経由で受けます。HTMXでツリーペインを部分更新します。

- [ ] Step 1: 失敗するテストを書く

Create `internal/handler/feed_handler_test.go`:
```go
package handler

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/okamyuji/feedflow-go-htmx/internal/domain"
)

// stubSubscriptions SubscriptionServiceのスタブです。
type stubSubscriptions struct {
	feeds         []domain.Feed
	subscribed    domain.Feed
	subscribeErr  error
	unsubscribeID string
}

func (s *stubSubscriptions) Subscribe(_ context.Context, feedURL string, _ []string) (domain.Feed, error) {
	if s.subscribeErr != nil {
		return domain.Feed{}, s.subscribeErr
	}
	f := domain.Feed{ID: "new", FeedURL: feedURL, Title: "新規フィード"}
	s.feeds = append(s.feeds, f)
	s.subscribed = f
	return f, nil
}

func (s *stubSubscriptions) SubscribeFromSite(_ context.Context, siteURL string, _ []string) (domain.Feed, error) {
	if s.subscribeErr != nil {
		return domain.Feed{}, s.subscribeErr
	}
	f := domain.Feed{ID: "new", SiteURL: siteURL, Title: "検出フィード"}
	s.feeds = append(s.feeds, f)
	s.subscribed = f
	return f, nil
}

func (s *stubSubscriptions) Unsubscribe(feedID string) error {
	s.unsubscribeID = feedID
	return nil
}
func (s *stubSubscriptions) ListFeeds() ([]domain.Feed, error)         { return s.feeds, nil }
func (s *stubSubscriptions) Reorder(_ []string) error                  { return nil }
func (s *stubSubscriptions) SetFeedCategories(_ string, _ []string) error { return nil }

// stubItems ItemServiceの最小スタブです。ツリー描画の未読集計に使います。
type stubItems struct {
	items map[string][]domain.Item
}

func (s *stubItems) ListItems(feedID string) ([]domain.Item, error) {
	if feedID == "" {
		var all []domain.Item
		for _, v := range s.items {
			all = append(all, v...)
		}
		return all, nil
	}
	return s.items[feedID], nil
}
func (s *stubItems) MarkRead(_, _ string, _ bool) error    { return nil }
func (s *stubItems) MarkAllRead(_ string) error            { return nil }
func (s *stubItems) Star(_, _ string, _ bool) error        { return nil }
func (s *stubItems) ReadLater(_, _ string, _ bool) error   { return nil }
func (s *stubItems) SetTags(_, _ string, _ []string) error { return nil }
func (s *stubItems) SetBoards(_, _ string, _ []string) error { return nil }
func (s *stubItems) SetNote(_, _, _ string) error          { return nil }
func (s *stubItems) AddHighlight(_, _, _ string) error     { return nil }

// stubMutes MuteServiceの最小スタブです。フィルタなしで素通しします。
type stubMutes struct {
	filters []domain.MuteFilter
}

func (s *stubMutes) ListFilters() ([]domain.MuteFilter, error) { return s.filters, nil }
func (s *stubMutes) AddFilter(keyword string, scope domain.MuteScope, feedID string) (domain.MuteFilter, error) {
	f := domain.MuteFilter{ID: "mf", Keyword: keyword, Scope: scope, FeedID: feedID}
	s.filters = append(s.filters, f)
	return f, nil
}
func (s *stubMutes) DeleteFilter(_ string) error { return nil }
func (s *stubMutes) Filter(items []domain.Item) ([]domain.Item, error) { return items, nil }

func newAppHandler(t *testing.T, subs *stubSubscriptions, items *stubItems) *Handler {
	t.Helper()
	h, err := New(Deps{
		Subscriptions:     subs,
		Items:             items,
		Mutes:             &stubMutes{},
		Sessions:          &stubSessions{username: "owner", ok: true},
		CSRF:              &stubCSRF{ok: true, token: "tok"},
		SessionCookieName: "feedflow_session",
	})
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}
	return h
}

func TestFeedSubscribeWithFeedURL(t *testing.T) {
	t.Parallel()
	subs := &stubSubscriptions{}
	h := newAppHandler(t, subs, &stubItems{items: map[string][]domain.Item{}})
	form := url.Values{"url": {"https://example.com/feed.xml"}}
	req := httptest.NewRequest(http.MethodPost, "/app/feeds", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req = withSession(req, Session{Username: "owner", CSRFToken: "tok"})
	rec := httptest.NewRecorder()

	h.feedSubscribe(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status got %d want %d", rec.Code, http.StatusOK)
	}
	if subs.subscribed.FeedURL != "https://example.com/feed.xml" {
		t.Fatalf("subscribed FeedURL got %q", subs.subscribed.FeedURL)
	}
	if !strings.Contains(rec.Body.String(), "tree") {
		t.Fatalf("body should render tree partial: %q", rec.Body.String())
	}
}

func TestFeedSubscribeWithSiteURLFallback(t *testing.T) {
	t.Parallel()
	subs := &stubSubscriptions{}
	h := newAppHandler(t, subs, &stubItems{items: map[string][]domain.Item{}})
	form := url.Values{"url": {"https://example.com/"}, "from_site": {"true"}}
	req := httptest.NewRequest(http.MethodPost, "/app/feeds", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req = withSession(req, Session{Username: "owner", CSRFToken: "tok"})
	rec := httptest.NewRecorder()

	h.feedSubscribe(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status got %d want %d", rec.Code, http.StatusOK)
	}
	if subs.subscribed.SiteURL != "https://example.com/" {
		t.Fatalf("subscribed SiteURL got %q", subs.subscribed.SiteURL)
	}
}

func TestFeedUnsubscribe(t *testing.T) {
	t.Parallel()
	subs := &stubSubscriptions{feeds: []domain.Feed{{ID: "f1", Title: "f1"}}}
	h := newAppHandler(t, subs, &stubItems{items: map[string][]domain.Item{}})
	req := httptest.NewRequest(http.MethodDelete, "/app/feeds/f1", nil)
	req.SetPathValue("feedID", "f1")
	req = withSession(req, Session{Username: "owner", CSRFToken: "tok"})
	rec := httptest.NewRecorder()

	h.feedUnsubscribe(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status got %d want %d", rec.Code, http.StatusOK)
	}
	if subs.unsubscribeID != "f1" {
		t.Fatalf("unsubscribed id got %q want %q", subs.unsubscribeID, "f1")
	}
}
```

- [ ] Step 2: テストが失敗することを確認する

Run:
```bash
cd /Users/yujiokamoto/devs/golang/feedflow-go-htmx && go test ./internal/handler/ -run 'TestFeed' -v
```
Expected: コンパイルエラーで失敗します。`undefined: (*Handler).feedSubscribe` などが表示されます。

- [ ] Step 3: ツリー組み立てと購読ハンドラを実装する

Create `internal/handler/feed_handler.go`:
```go
package handler

import (
	"net/http"

	"github.com/okamyuji/feedflow-go-htmx/internal/domain"
)

// buildTree 左ペインの購読ツリーを組み立てます。固定の集約ノードに続けてフィードを並べます。
func (h *Handler) buildTree() ([]feedTreeNode, error) {
	feeds, err := h.deps.Subscriptions.ListFeeds()
	if err != nil {
		return nil, err
	}
	nodes := []feedTreeNode{
		{Kind: "all", Label: "すべて"},
		{Kind: "unread", Label: "未読"},
		{Kind: "starred", Label: "スター"},
		{Kind: "readlater", Label: "あとで読む"},
	}
	for _, f := range feeds {
		unread, err := h.unreadCount(f.ID)
		if err != nil {
			return nil, err
		}
		nodes = append(nodes, feedTreeNode{
			Kind:        "feed",
			ID:          f.ID,
			Label:       f.Title,
			UnreadCount: unread,
			HasError:    f.HasError(),
		})
	}
	return nodes, nil
}

// unreadCount 指定フィードの未読件数を数えます。引数feedIDが空のときは全フィードを対象にします。
func (h *Handler) unreadCount(feedID string) (int, error) {
	items, err := h.deps.Items.ListItems(feedID)
	if err != nil {
		return 0, err
	}
	count := 0
	for _, it := range items {
		if !it.Read {
			count++
		}
	}
	return count, nil
}

// treeData ツリー部分テンプレートに渡す描画モデルを組み立てます。
func (h *Handler) treeData(r *http.Request) (pageData, error) {
	sess := sessionFromContext(r.Context())
	tree, err := h.buildTree()
	if err != nil {
		return pageData{}, err
	}
	return pageData{CSRFToken: sess.CSRFToken, Username: sess.Username, Tree: tree}, nil
}

// feedSubscribe フィードURLまたはサイトURLから購読を追加し、ツリーペインを部分更新で返します。
func (h *Handler) feedSubscribe(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	rawURL := r.FormValue("url")
	if rawURL == "" {
		http.Error(w, "url is required", http.StatusBadRequest)
		return
	}
	fromSite := r.FormValue("from_site") == "true"
	var err error
	if fromSite {
		_, err = h.deps.Subscriptions.SubscribeFromSite(r.Context(), rawURL, nil)
	} else {
		_, err = h.deps.Subscriptions.Subscribe(r.Context(), rawURL, nil)
	}
	if err != nil {
		http.Error(w, "failed to subscribe", http.StatusBadGateway)
		return
	}
	data, err := h.treeData(r)
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	h.renderPartial(w, http.StatusOK, "_tree.html", data)
}

// feedUnsubscribe 指定フィードの購読を解除し、ツリーペインを部分更新で返します。
func (h *Handler) feedUnsubscribe(w http.ResponseWriter, r *http.Request) {
	feedID := r.PathValue("feedID")
	if feedID == "" {
		http.Error(w, "feedID is required", http.StatusBadRequest)
		return
	}
	if err := h.deps.Subscriptions.Unsubscribe(feedID); err != nil {
		http.Error(w, "failed to unsubscribe", http.StatusInternalServerError)
		return
	}
	data, err := h.treeData(r)
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	h.renderPartial(w, http.StatusOK, "_tree.html", data)
}

// toItemView ドメインの記事を表示モデルへ変換します。本文はhtml/templateの自動エスケープに委ねます。
func toItemView(it domain.Item) itemView {
	return itemView{
		ID:          it.ID,
		FeedID:      it.FeedID,
		Title:       it.Title,
		Link:        it.Link,
		Summary:     truncateRunes(it.Summary, 160),
		Author:      it.Author,
		PublishedAt: formatJST(it.PublishedAt),
		Read:        it.Read,
		Starred:     it.Starred,
		ReadLater:   it.ReadLater,
	}
}
```

補足: `_tree.html`はrange内で`{{ .CSRFToken }}`を参照しますが、Goのhtml/templateではrange内のドットがループ要素に変わるため、CSRFトークンはツリー直下のフォームでだけ使います。`_tree.html`内のsubscribe-formはrangeの外にあり、トップレベルの`.CSRFToken`を参照できます。

- [ ] Step 4: テストが通ることを確認する

Run:
```bash
cd /Users/yujiokamoto/devs/golang/feedflow-go-htmx && go test ./internal/handler/ -run 'TestFeed' -v
```
Expected: 3件すべてPASSします。

- [ ] Step 5: gofmtを適用してコミットする

```bash
cd /Users/yujiokamoto/devs/golang/feedflow-go-htmx && gofmt -w internal/handler/feed_handler.go internal/handler/feed_handler_test.go && git add internal/handler/feed_handler.go internal/handler/feed_handler_test.go && git commit -m "feat: 購読の追加削除一覧のハンドラを追加する"
```

---

## Task 8: 記事一覧と本文オーバーレイと既読とスター操作のハンドラを実装する

Files:
- Create: `/Users/yujiokamoto/devs/golang/feedflow-go-htmx/internal/handler/item_handler.go`
- Test: `/Users/yujiokamoto/devs/golang/feedflow-go-htmx/internal/handler/item_handler_test.go`

設計書セクション3.1のとおり、記事一覧の表示、本文オーバーレイ、既読とスターとあとで読むの操作を実装します。記事はport.MuteServiceでミュート適用してから返します。HTMXでメインペインとオーバーレイと記事カードを部分更新します。

- [ ] Step 1: 失敗するテストを書く

Create `internal/handler/item_handler_test.go`:
```go
package handler

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/okamyuji/feedflow-go-htmx/internal/domain"
)

func sampleItems() map[string][]domain.Item {
	return map[string][]domain.Item{
		"f1": {
			{ID: "i1", FeedID: "f1", Title: "記事1", Summary: "要約1", PublishedAt: time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC)},
			{ID: "i2", FeedID: "f1", Title: "記事2", Summary: "要約2", Read: true},
		},
	}
}

func TestItemListRendersCards(t *testing.T) {
	t.Parallel()
	subs := &stubSubscriptions{feeds: []domain.Feed{{ID: "f1", Title: "f1"}}}
	h := newAppHandler(t, subs, &stubItems{items: sampleItems()})
	req := httptest.NewRequest(http.MethodGet, "/app/items?feed=f1", nil)
	req = withSession(req, Session{Username: "owner", CSRFToken: "tok"})
	rec := httptest.NewRecorder()

	h.itemList(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status got %d want %d", rec.Code, http.StatusOK)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "記事1") || !strings.Contains(body, "記事2") {
		t.Fatalf("body should list both items: %q", body)
	}
}

func TestItemOverlayRendersContent(t *testing.T) {
	t.Parallel()
	subs := &stubSubscriptions{feeds: []domain.Feed{{ID: "f1", Title: "f1"}}}
	h := newAppHandler(t, subs, &stubItems{items: sampleItems()})
	req := httptest.NewRequest(http.MethodGet, "/app/items/f1/i1", nil)
	req.SetPathValue("feedID", "f1")
	req.SetPathValue("itemID", "i1")
	req = withSession(req, Session{Username: "owner", CSRFToken: "tok"})
	rec := httptest.NewRecorder()

	h.itemOverlay(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status got %d want %d", rec.Code, http.StatusOK)
	}
	if !strings.Contains(rec.Body.String(), "記事1") {
		t.Fatalf("overlay should render item title: %q", rec.Body.String())
	}
}

func TestItemOverlayNotFound(t *testing.T) {
	t.Parallel()
	subs := &stubSubscriptions{}
	h := newAppHandler(t, subs, &stubItems{items: sampleItems()})
	req := httptest.NewRequest(http.MethodGet, "/app/items/f1/missing", nil)
	req.SetPathValue("feedID", "f1")
	req.SetPathValue("itemID", "missing")
	req = withSession(req, Session{Username: "owner", CSRFToken: "tok"})
	rec := httptest.NewRecorder()

	h.itemOverlay(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status got %d want %d", rec.Code, http.StatusNotFound)
	}
}

func TestItemMarkRead(t *testing.T) {
	t.Parallel()
	subs := &stubSubscriptions{}
	items := &stubItems{items: sampleItems()}
	h := newAppHandler(t, subs, items)
	form := url.Values{"read": {"true"}}
	req := httptest.NewRequest(http.MethodPost, "/app/items/f1/i1/read", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.SetPathValue("feedID", "f1")
	req.SetPathValue("itemID", "i1")
	req = withSession(req, Session{Username: "owner", CSRFToken: "tok"})
	rec := httptest.NewRecorder()

	h.itemMarkRead(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status got %d want %d", rec.Code, http.StatusOK)
	}
	if !strings.Contains(rec.Body.String(), "item-card") {
		t.Fatalf("body should re-render the card: %q", rec.Body.String())
	}
}

func TestItemStar(t *testing.T) {
	t.Parallel()
	subs := &stubSubscriptions{}
	items := &stubItems{items: sampleItems()}
	h := newAppHandler(t, subs, items)
	form := url.Values{"starred": {"true"}}
	req := httptest.NewRequest(http.MethodPost, "/app/items/f1/i1/star", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.SetPathValue("feedID", "f1")
	req.SetPathValue("itemID", "i1")
	req = withSession(req, Session{Username: "owner", CSRFToken: "tok"})
	rec := httptest.NewRecorder()

	h.itemStar(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status got %d want %d", rec.Code, http.StatusOK)
	}
}

func TestItemMarkAll(t *testing.T) {
	t.Parallel()
	subs := &stubSubscriptions{feeds: []domain.Feed{{ID: "f1", Title: "f1"}}}
	items := &stubItems{items: sampleItems()}
	h := newAppHandler(t, subs, items)
	req := httptest.NewRequest(http.MethodPost, "/app/items/markall", nil)
	req = withSession(req, Session{Username: "owner", CSRFToken: "tok"})
	rec := httptest.NewRecorder()

	h.itemMarkAll(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status got %d want %d", rec.Code, http.StatusOK)
	}
}
```

- [ ] Step 2: テストが失敗することを確認する

Run:
```bash
cd /Users/yujiokamoto/devs/golang/feedflow-go-htmx && go test ./internal/handler/ -run 'TestItem' -v
```
Expected: コンパイルエラーで失敗します。`undefined: (*Handler).itemList` などが表示されます。既存の`TestItemHasUserAction`はdomainパッケージのため影響しません。

- [ ] Step 3: 記事ハンドラを実装する

Create `internal/handler/item_handler.go`:
```go
package handler

import (
	"html/template"
	"net/http"
	"strings"

	"github.com/okamyuji/feedflow-go-htmx/internal/domain"
	"github.com/okamyuji/feedflow-go-htmx/internal/feed"
)

// listItemsFor クエリに応じてミュート適用済みの記事群を取得します。feedクエリがあればそのフィード、無ければ全件です。
func (h *Handler) listItemsFor(r *http.Request) ([]domain.Item, error) {
	feedID := r.URL.Query().Get("feed")
	items, err := h.deps.Items.ListItems(feedID)
	if err != nil {
		return nil, err
	}
	filtered, err := h.deps.Mutes.Filter(items)
	if err != nil {
		return nil, err
	}
	return filtered, nil
}

// cleanArticleHTML フィード本文からテキストを抽出し、安全な段落HTMLに整形します。
// golang.org/x/net/htmlでパースした本文テキストを段落ごとにHTMLエスケープして組み立てます。
// 生のHTMLタグを露出させず、かつXSSを避けます。
func cleanArticleHTML(raw string) template.HTML {
	text, err := feed.Extract([]byte(raw))
	if err != nil || strings.TrimSpace(text) == "" {
		text = raw
	}
	var sb strings.Builder
	for _, para := range strings.Split(text, "\n\n") {
		para = strings.TrimSpace(para)
		if para == "" {
			continue
		}
		sb.WriteString("<p>")
		sb.WriteString(template.HTMLEscapeString(para))
		sb.WriteString("</p>")
	}
	return template.HTML(sb.String()) //nolint:gosec // 各段落はHTMLEscapeStringでエスケープ済みのため安全です
}

// itemList 記事一覧の部分テンプレートを描画します。
func (h *Handler) itemList(w http.ResponseWriter, r *http.Request) {
	sess := sessionFromContext(r.Context())
	items, err := h.listItemsFor(r)
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	views := make([]itemView, 0, len(items))
	for _, it := range items {
		views = append(views, toItemView(it))
	}
	data := pageData{CSRFToken: sess.CSRFToken, DefaultView: domain.ViewCard, Items: views}
	if isHTMX(r) {
		h.renderPartial(w, http.StatusOK, "_item_list.html", data)
		return
	}
	h.renderShellPage(w, sess, "feedflow", data)
}

// findItem 指定フィードと記事IDの記事を返します。見つからない場合はokがfalseになります。
func (h *Handler) findItem(feedID, itemID string) (domain.Item, bool, error) {
	items, err := h.deps.Items.ListItems(feedID)
	if err != nil {
		return domain.Item{}, false, err
	}
	for _, it := range items {
		if it.ID == itemID {
			return it, true, nil
		}
	}
	return domain.Item{}, false, nil
}

// itemOverlay 記事本文のオーバーレイ部分テンプレートを描画します。表示の副作用として既読にします。
func (h *Handler) itemOverlay(w http.ResponseWriter, r *http.Request) {
	feedID := r.PathValue("feedID")
	itemID := r.PathValue("itemID")
	it, ok, err := h.findItem(feedID, itemID)
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	if !ok {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	if !it.Read {
		if err := h.deps.Items.MarkRead(feedID, itemID, true); err != nil {
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
	}
	view := toItemView(it)
	view.Content = cleanArticleHTML(it.Content)
	h.renderPartial(w, http.StatusOK, "_item_overlay.html", view)
}

// renderCard 操作後の単一記事カードを再描画します。
func (h *Handler) renderCard(w http.ResponseWriter, feedID, itemID string) {
	it, ok, err := h.findItem(feedID, itemID)
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	if !ok {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	h.renderPartial(w, http.StatusOK, "_item_card.html", toItemView(it))
}

// itemMarkRead 既読状態を設定し、記事カードを再描画します。
func (h *Handler) itemMarkRead(w http.ResponseWriter, r *http.Request) {
	feedID := r.PathValue("feedID")
	itemID := r.PathValue("itemID")
	read := r.FormValue("read") == "true"
	if err := h.deps.Items.MarkRead(feedID, itemID, read); err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	h.renderCard(w, feedID, itemID)
}

// itemStar スター状態を設定し、記事カードを再描画します。
func (h *Handler) itemStar(w http.ResponseWriter, r *http.Request) {
	feedID := r.PathValue("feedID")
	itemID := r.PathValue("itemID")
	starred := r.FormValue("starred") == "true"
	if err := h.deps.Items.Star(feedID, itemID, starred); err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	h.renderCard(w, feedID, itemID)
}

// itemReadLater あとで読む状態を設定します。オーバーレイからの呼び出しが多いため本文は返しません。
func (h *Handler) itemReadLater(w http.ResponseWriter, r *http.Request) {
	feedID := r.PathValue("feedID")
	itemID := r.PathValue("itemID")
	readLater := r.FormValue("read_later") == "true"
	if err := h.deps.Items.ReadLater(feedID, itemID, readLater); err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// itemMarkAll 指定フィードまたは全フィードを既読にし、記事一覧を再描画します。
func (h *Handler) itemMarkAll(w http.ResponseWriter, r *http.Request) {
	feedID := r.URL.Query().Get("feed")
	if err := h.deps.Items.MarkAllRead(feedID); err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	h.itemList(w, r)
}
```

補足: `renderCard`は単一カード用の部分テンプレート`_item_card.html`を参照します。これは次のStepで`_item_list.html`から1カード分を切り出して作成します。`itemOverlay`はゼロ値のIDなどに依存せず、`findItem`の探索結果に基づいて404を返します。`renderCard`は操作後に`findItem`で最新の記事を再取得してから描画するため、サービスが状態を更新していればその最新状態が反映されます。`itemList`はHX-Requestヘッダで判定し、HTMXのときだけ`_item_list.html`の部分テンプレートを返し、URL直アクセスやリロードのときは`renderShellPage`でbase.htmlのフルページを返します。記事本文は生HTMLをそのままエスケープすると生タグが文字列として露出するため、`cleanArticleHTML`が`feed.Extract`でテキストを抽出し、段落ごとにHTMLエスケープした段落HTMLへ整形してから描画します。

- [ ] Step 4: 単一カードの部分テンプレートを作成する

Create `internal/handler/templates/_item_card.html`:
```html
{{ define "_item_card.html" }}
<li
  class="item-card{{ if .Read }} is-read{{ end }}"
  id="item-{{ .ID }}"
  data-feed="{{ .FeedID }}"
  data-item="{{ .ID }}"
>
  <a
    class="item-open"
    href="{{ .Link }}"
    hx-get="/app/items/{{ .FeedID }}/{{ .ID }}"
    hx-target="#reading-overlay"
    hx-swap="innerHTML"
    @click.prevent="openOverlay"
  >
    <h3 class="item-title">{{ .Title }}</h3>
    <p class="item-summary">{{ .Summary }}</p>
    <div class="item-meta">
      <time>{{ .PublishedAt }}</time>
      {{ if .Starred }}<span class="item-star">保存済み</span>{{ end }}
    </div>
  </a>
  <div class="item-quick">
    <button
      class="quick-btn"
      hx-post="/app/items/{{ .FeedID }}/{{ .ID }}/star"
      hx-vals='{"starred": "{{ if .Starred }}false{{ else }}true{{ end }}"}'
      hx-target="#item-{{ .ID }}"
      hx-swap="outerHTML"
    >{{ if .Starred }}スター解除{{ else }}スター{{ end }}</button>
    <button
      class="quick-btn"
      hx-post="/app/items/{{ .FeedID }}/{{ .ID }}/read"
      hx-vals='{"read": "{{ if .Read }}false{{ else }}true{{ end }}"}'
      hx-target="#item-{{ .ID }}"
      hx-swap="outerHTML"
    >{{ if .Read }}未読に戻す{{ else }}既読{{ end }}</button>
  </div>
</li>
{{ end }}
```

- [ ] Step 5: テストが通ることを確認する

Run:
```bash
cd /Users/yujiokamoto/devs/golang/feedflow-go-htmx && go test ./internal/handler/ -run 'TestItemList|TestItemOverlay|TestItemMarkRead|TestItemStar|TestItemMarkAll' -v
```
Expected: 6件すべてPASSします。

- [ ] Step 6: gofmtを適用してコミットする

```bash
cd /Users/yujiokamoto/devs/golang/feedflow-go-htmx && gofmt -w internal/handler/item_handler.go internal/handler/item_handler_test.go && git add internal/handler/item_handler.go internal/handler/item_handler_test.go internal/handler/templates/_item_card.html && git commit -m "feat: 記事一覧と本文オーバーレイと既読とスター操作のハンドラを追加する"
```

---

## Task 9: ボード操作のハンドラを実装する

Files:
- Create: `/Users/yujiokamoto/devs/golang/feedflow-go-htmx/internal/handler/board_handler.go`
- Create: `/Users/yujiokamoto/devs/golang/feedflow-go-htmx/internal/handler/templates/_boards.html`
- Test: `/Users/yujiokamoto/devs/golang/feedflow-go-htmx/internal/handler/board_handler_test.go`

設計書セクション3.1のボード(テーマ別に記事を保存)を実装します。記事の保存先ボードはport.ItemServiceのSetBoardsで更新します。ボードの一覧と記事へのボード割り当てを扱います。

- [ ] Step 1: ボード一覧の部分テンプレートを作成する

Create `internal/handler/templates/_boards.html`:
```html
{{ define "_boards.html" }}
<section class="boards" id="boards">
  <h2 class="boards-title">ボード</h2>
  <ul class="boards-list">
    {{ range .Boards }}
    <li class="board-item">
      <a
        class="board-link"
        href="/app/items?board={{ .ID }}"
        hx-get="/app/items?board={{ .ID }}"
        hx-target="#main-pane"
        hx-push-url="true"
      >{{ .Name }}</a>
      {{ if .Description }}<p class="board-desc">{{ .Description }}</p>{{ end }}
    </li>
    {{ end }}
  </ul>
</section>
{{ end }}
```

- [ ] Step 2: 失敗するテストを書く

Create `internal/handler/board_handler_test.go`:
```go
package handler

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/okamyuji/feedflow-go-htmx/internal/domain"
)

func newBoardHandler(t *testing.T, items *stubItems) *Handler {
	t.Helper()
	subs := &stubSubscriptions{}
	h, err := New(Deps{
		Subscriptions:     subs,
		Items:             items,
		Mutes:             &stubMutes{},
		Sessions:          &stubSessions{username: "owner", ok: true},
		CSRF:              &stubCSRF{ok: true, token: "tok"},
		SessionCookieName: "feedflow_session",
	})
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}
	return h
}

// boardItems ボード割り当てを記録するItemServiceスタブです。
type boardItems struct {
	stubItems
	lastFeed   string
	lastItem   string
	lastBoards []string
}

func (b *boardItems) SetBoards(feedID, itemID string, boardIDs []string) error {
	b.lastFeed = feedID
	b.lastItem = itemID
	b.lastBoards = boardIDs
	return nil
}

func TestItemSetBoards(t *testing.T) {
	t.Parallel()
	items := &boardItems{stubItems: stubItems{items: map[string][]domain.Item{
		"f1": {{ID: "i1", FeedID: "f1", Title: "記事1"}},
	}}}
	h := newBoardHandler(t, &items.stubItems)
	h.deps.Items = items
	form := url.Values{"board_ids": {"b1", "b2"}}
	req := httptest.NewRequest(http.MethodPost, "/app/items/f1/i1/boards", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.SetPathValue("feedID", "f1")
	req.SetPathValue("itemID", "i1")
	req = withSession(req, Session{Username: "owner", CSRFToken: "tok"})
	rec := httptest.NewRecorder()

	h.itemSetBoards(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("status got %d want %d", rec.Code, http.StatusNoContent)
	}
	if items.lastItem != "i1" {
		t.Fatalf("lastItem got %q want %q", items.lastItem, "i1")
	}
	if len(items.lastBoards) != 2 {
		t.Fatalf("lastBoards len got %d want 2", len(items.lastBoards))
	}
}
```

補足: 上の`boardItems`は`stubItems`を埋め込み、`SetBoards`だけ上書きします。`newBoardHandler`へ渡したあと`h.deps.Items`を`boardItems`へ差し替えることで、記録付きスタブで検証します。

- [ ] Step 3: テストが失敗することを確認する

Run:
```bash
cd /Users/yujiokamoto/devs/golang/feedflow-go-htmx && go test ./internal/handler/ -run 'TestItemSetBoards' -v
```
Expected: コンパイルエラーで失敗します。`undefined: (*Handler).itemSetBoards` が表示されます。

- [ ] Step 4: ボードハンドラを実装する

Create `internal/handler/board_handler.go`:
```go
package handler

import "net/http"

// itemSetBoards 記事の保存先ボードを更新します。送信されたボードID群で置き換えます。
func (h *Handler) itemSetBoards(w http.ResponseWriter, r *http.Request) {
	feedID := r.PathValue("feedID")
	itemID := r.PathValue("itemID")
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	boardIDs := r.Form["board_ids"]
	if err := h.deps.Items.SetBoards(feedID, itemID, boardIDs); err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
```

- [ ] Step 5: テストが通ることを確認する

Run:
```bash
cd /Users/yujiokamoto/devs/golang/feedflow-go-htmx && go test ./internal/handler/ -run 'TestItemSetBoards' -v
```
Expected: PASSします。

- [ ] Step 6: gofmtを適用してコミットする

```bash
cd /Users/yujiokamoto/devs/golang/feedflow-go-htmx && gofmt -w internal/handler/board_handler.go internal/handler/board_handler_test.go && git add internal/handler/board_handler.go internal/handler/board_handler_test.go internal/handler/templates/_boards.html && git commit -m "feat: ボード操作のハンドラと一覧テンプレートを追加する"
```

---

## Task 10: 設定とOPMLのハンドラを実装する

Files:
- Create: `/Users/yujiokamoto/devs/golang/feedflow-go-htmx/internal/handler/settings_handler.go`
- Create: `/Users/yujiokamoto/devs/golang/feedflow-go-htmx/internal/handler/templates/_settings.html`
- Test: `/Users/yujiokamoto/devs/golang/feedflow-go-htmx/internal/handler/settings_handler_test.go`

設計書セクション4と3.1のとおり、設定の取得と更新、OPMLのインポートとエクスポートを実装します。設定はport.SettingsService、OPMLはport.OPMLServiceのインターフェース経由で受けます。設定の検証はサービス側のUpdateに委ね、不正値はエラーとして画面に反映します。

- [ ] Step 1: 設定画面の部分テンプレートを作成する

Create `internal/handler/templates/_settings.html`:
```html
{{ define "_settings.html" }}
<section class="settings" id="settings">
  <h2 class="settings-title">設定</h2>
  {{ if .Flash }}<p class="settings-flash" role="status">{{ .Flash }}</p>{{ end }}
  <form
    class="settings-form"
    hx-post="/app/settings"
    hx-target="#main-pane"
    hx-swap="innerHTML"
  >
    <input type="hidden" name="csrf_token" value="{{ .CSRFToken }}">
    <label class="field">
      <span class="field-label">ポーリング間隔</span>
      <select class="field-input" name="poll_interval">
        <option value="15m"{{ if eq (printf "%s" .Settings.PollInterval) "15m" }} selected{{ end }}>15分</option>
        <option value="30m"{{ if eq (printf "%s" .Settings.PollInterval) "30m" }} selected{{ end }}>30分</option>
        <option value="1h"{{ if eq (printf "%s" .Settings.PollInterval) "1h" }} selected{{ end }}>1時間</option>
        <option value="6h"{{ if eq (printf "%s" .Settings.PollInterval) "6h" }} selected{{ end }}>6時間</option>
        <option value="manual"{{ if eq (printf "%s" .Settings.PollInterval) "manual" }} selected{{ end }}>手動のみ</option>
      </select>
    </label>
    <label class="field">
      <span class="field-label">保持件数N</span>
      <input class="field-input" type="number" name="max_items" min="1" value="{{ .Settings.MaxItems }}">
    </label>
    <label class="field">
      <span class="field-label">既読の自動削除日数M</span>
      <input class="field-input" type="number" name="read_retention_days" min="1" value="{{ .Settings.ReadRetentionDays }}">
    </label>
    <label class="field">
      <span class="field-label">テーマ</span>
      <select class="field-input" name="theme">
        <option value="dark"{{ if eq (printf "%s" .Settings.Theme) "dark" }} selected{{ end }}>ダーク</option>
        <option value="light"{{ if eq (printf "%s" .Settings.Theme) "light" }} selected{{ end }}>ライト</option>
      </select>
    </label>
    <label class="field">
      <span class="field-label">既定の表示形式</span>
      <select class="field-input" name="default_view">
        <option value="title"{{ if eq (printf "%s" .Settings.DefaultView) "title" }} selected{{ end }}>タイトルのみ</option>
        <option value="card"{{ if eq (printf "%s" .Settings.DefaultView) "card" }} selected{{ end }}>カード</option>
        <option value="magazine"{{ if eq (printf "%s" .Settings.DefaultView) "magazine" }} selected{{ end }}>マガジン</option>
        <option value="article"{{ if eq (printf "%s" .Settings.DefaultView) "article" }} selected{{ end }}>記事ビュー</option>
      </select>
    </label>
    <button class="btn-primary" type="submit">保存する</button>
  </form>

  <div class="opml-actions">
    <h3 class="opml-title">OPML</h3>
    <a class="btn-ghost" href="/app/opml/export">エクスポート</a>
    <form method="post" action="/app/opml/import" enctype="multipart/form-data" class="opml-import">
      <input type="hidden" name="csrf_token" value="{{ .CSRFToken }}">
      <input class="field-input" type="file" name="opml" accept=".opml,.xml" required>
      <button class="btn-ghost" type="submit">インポート</button>
    </form>
  </div>
</section>
{{ end }}
```

- [ ] Step 2: 失敗するテストを書く

Create `internal/handler/settings_handler_test.go`:
```go
package handler

import (
	"bytes"
	"context"
	"errors"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/okamyuji/feedflow-go-htmx/internal/domain"
)

// stubSettings SettingsServiceのスタブです。
type stubSettings struct {
	current   domain.Settings
	updateErr error
	updated   domain.Settings
}

func (s *stubSettings) Get() (domain.Settings, error) { return s.current, nil }
func (s *stubSettings) Update(settings domain.Settings) error {
	if s.updateErr != nil {
		return s.updateErr
	}
	s.updated = settings
	s.current = settings
	return nil
}

// stubOPML OPMLServiceのスタブです。
type stubOPML struct {
	imported  int
	exportOut []byte
}

func (s *stubOPML) Import(_ context.Context, _ []byte) (int, error) { return s.imported, nil }
func (s *stubOPML) Export() ([]byte, error)                         { return s.exportOut, nil }

func newSettingsHandler(t *testing.T, st *stubSettings, op *stubOPML) *Handler {
	t.Helper()
	h, err := New(Deps{
		Settings:          st,
		OPML:              op,
		Sessions:          &stubSessions{username: "owner", ok: true},
		CSRF:              &stubCSRF{ok: true, token: "tok"},
		SessionCookieName: "feedflow_session",
	})
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}
	return h
}

func TestSettingsPageRenders(t *testing.T) {
	t.Parallel()
	st := &stubSettings{current: domain.DefaultSettings()}
	h := newSettingsHandler(t, st, &stubOPML{})
	req := httptest.NewRequest(http.MethodGet, "/app/settings", nil)
	req = withSession(req, Session{Username: "owner", CSRFToken: "tok"})
	rec := httptest.NewRecorder()

	h.settingsPage(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status got %d want %d", rec.Code, http.StatusOK)
	}
	if !strings.Contains(rec.Body.String(), "設定") {
		t.Fatalf("body should render settings: %q", rec.Body.String())
	}
}

func TestSettingsUpdateSuccess(t *testing.T) {
	t.Parallel()
	st := &stubSettings{current: domain.DefaultSettings()}
	h := newSettingsHandler(t, st, &stubOPML{})
	form := url.Values{
		"poll_interval":       {"1h"},
		"max_items":           {"100"},
		"read_retention_days": {"14"},
		"theme":               {"light"},
		"default_view":        {"magazine"},
	}
	req := httptest.NewRequest(http.MethodPost, "/app/settings", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req = withSession(req, Session{Username: "owner", CSRFToken: "tok"})
	rec := httptest.NewRecorder()

	h.settingsUpdate(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status got %d want %d", rec.Code, http.StatusOK)
	}
	if st.updated.MaxItems != 100 {
		t.Fatalf("updated MaxItems got %d want 100", st.updated.MaxItems)
	}
	if st.updated.Theme != domain.ThemeLight {
		t.Fatalf("updated Theme got %q want %q", st.updated.Theme, domain.ThemeLight)
	}
}

func TestSettingsUpdateInvalidShowsError(t *testing.T) {
	t.Parallel()
	st := &stubSettings{current: domain.DefaultSettings(), updateErr: errors.New("invalid settings")}
	h := newSettingsHandler(t, st, &stubOPML{})
	form := url.Values{
		"poll_interval":       {"30m"},
		"max_items":           {"0"},
		"read_retention_days": {"30"},
		"theme":               {"dark"},
		"default_view":        {"card"},
	}
	req := httptest.NewRequest(http.MethodPost, "/app/settings", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req = withSession(req, Session{Username: "owner", CSRFToken: "tok"})
	rec := httptest.NewRecorder()

	h.settingsUpdate(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status got %d want %d", rec.Code, http.StatusBadRequest)
	}
	if !strings.Contains(rec.Body.String(), "設定を保存できませんでした") {
		t.Fatalf("body should contain error: %q", rec.Body.String())
	}
}

func TestOPMLExport(t *testing.T) {
	t.Parallel()
	op := &stubOPML{exportOut: []byte(`<opml version="2.0"></opml>`)}
	h := newSettingsHandler(t, &stubSettings{current: domain.DefaultSettings()}, op)
	req := httptest.NewRequest(http.MethodGet, "/app/opml/export", nil)
	req = withSession(req, Session{Username: "owner", CSRFToken: "tok"})
	rec := httptest.NewRecorder()

	h.opmlExport(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status got %d want %d", rec.Code, http.StatusOK)
	}
	if ct := rec.Header().Get("Content-Type"); !strings.Contains(ct, "xml") {
		t.Fatalf("Content-Type got %q want xml", ct)
	}
	if !strings.Contains(rec.Body.String(), "opml") {
		t.Fatalf("body should contain opml: %q", rec.Body.String())
	}
}

func TestOPMLImport(t *testing.T) {
	t.Parallel()
	op := &stubOPML{imported: 3}
	h := newSettingsHandler(t, &stubSettings{current: domain.DefaultSettings()}, op)

	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
	part, err := mw.CreateFormFile("opml", "feeds.opml")
	if err != nil {
		t.Fatalf("CreateFormFile returned error: %v", err)
	}
	if _, err := part.Write([]byte(`<opml version="2.0"></opml>`)); err != nil {
		t.Fatalf("write part returned error: %v", err)
	}
	if err := mw.Close(); err != nil {
		t.Fatalf("close writer returned error: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/app/opml/import", &body)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	req = withSession(req, Session{Username: "owner", CSRFToken: "tok"})
	rec := httptest.NewRecorder()

	h.opmlImport(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status got %d want %d", rec.Code, http.StatusOK)
	}
	if !strings.Contains(rec.Body.String(), "3") {
		t.Fatalf("body should report import count: %q", rec.Body.String())
	}
}
```

補足: このテストファイルは`context`をimportするため、ファイル冒頭のimportに`"context"`を含めます。

- [ ] Step 3: テストが失敗することを確認する

Run:
```bash
cd /Users/yujiokamoto/devs/golang/feedflow-go-htmx && go test ./internal/handler/ -run 'TestSettings|TestOPML' -v
```
Expected: コンパイルエラーで失敗します。`undefined: (*Handler).settingsPage` などが表示されます。

- [ ] Step 4: 設定とOPMLのハンドラを実装する

Create `internal/handler/settings_handler.go`:
```go
package handler

import (
	"fmt"
	"net/http"
	"strconv"

	"github.com/okamyuji/feedflow-go-htmx/internal/domain"
	"github.com/okamyuji/feedflow-go-htmx/internal/obs"
)

// maxOPMLBytes OPMLインポートの最大バイト数です。過大なアップロードを防ぎます。
const maxOPMLBytes = 8 << 20

// settingsPage 設定画面の部分テンプレートを描画します。
func (h *Handler) settingsPage(w http.ResponseWriter, r *http.Request) {
	sess := sessionFromContext(r.Context())
	settings, err := h.deps.Settings.Get()
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	data := pageData{CSRFToken: sess.CSRFToken, Settings: settings}
	h.renderPartial(w, http.StatusOK, "_settings.html", data)
}

// settingsUpdate 設定フォームを受け取り、サービスの検証を経て保存します。不正値は画面にエラーを表示します。
func (h *Handler) settingsUpdate(w http.ResponseWriter, r *http.Request) {
	sess := sessionFromContext(r.Context())
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	maxItems, err := strconv.Atoi(r.FormValue("max_items"))
	if err != nil {
		h.renderSettingsError(w, sess, "保持件数は数値で入力してください")
		return
	}
	retainDays, err := strconv.Atoi(r.FormValue("read_retention_days"))
	if err != nil {
		h.renderSettingsError(w, sess, "保持日数は数値で入力してください")
		return
	}
	settings := domain.Settings{
		PollInterval:      domain.PollInterval(r.FormValue("poll_interval")),
		MaxItems:          maxItems,
		ReadRetentionDays: retainDays,
		Theme:             domain.Theme(r.FormValue("theme")),
		DefaultView:       domain.ViewMode(r.FormValue("default_view")),
	}
	if err := h.deps.Settings.Update(settings); err != nil {
		h.renderSettingsError(w, sess, "設定を保存できませんでした。入力値を確認してください")
		return
	}
	data := pageData{CSRFToken: sess.CSRFToken, Settings: settings, Flash: "設定を保存しました"}
	h.renderPartial(w, http.StatusOK, "_settings.html", data)
}

// renderSettingsError 設定の保存失敗を現在値とともに画面へ表示します。
func (h *Handler) renderSettingsError(w http.ResponseWriter, sess Session, msg string) {
	settings, err := h.deps.Settings.Get()
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	data := pageData{CSRFToken: sess.CSRFToken, Settings: settings, Flash: msg}
	h.renderPartial(w, http.StatusBadRequest, "_settings.html", data)
}

// opmlExport 現在の購読をOPMLとして返します。
func (h *Handler) opmlExport(w http.ResponseWriter, _ *http.Request) {
	data, err := h.deps.OPML.Export()
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/x-opml; charset=utf-8")
	w.Header().Set("Content-Disposition", `attachment; filename="feedflow.opml"`)
	w.WriteHeader(http.StatusOK)
	obs.WriteAndLog(nil, "opml export response", w, data)
}

// opmlImport アップロードされたOPMLを読み込み、購読を追加した件数を返します。
func (h *Handler) opmlImport(w http.ResponseWriter, r *http.Request) {
	sess := sessionFromContext(r.Context())
	if err := r.ParseMultipartForm(maxOPMLBytes); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	file, _, err := r.FormFile("opml")
	if err != nil {
		http.Error(w, "opml file is required", http.StatusBadRequest)
		return
	}
	defer obs.CloseAndLog(nil, "opml upload file", file)
	data := make([]byte, 0, 4096)
	buf := make([]byte, 4096)
	for {
		n, rerr := file.Read(buf)
		data = append(data, buf[:n]...)
		if len(data) > maxOPMLBytes {
			http.Error(w, "opml too large", http.StatusRequestEntityTooLarge)
			return
		}
		if rerr != nil {
			break
		}
	}
	count, err := h.deps.OPML.Import(r.Context(), data)
	if err != nil {
		http.Error(w, "failed to import opml", http.StatusBadRequest)
		return
	}
	page := pageData{CSRFToken: sess.CSRFToken, Flash: fmt.Sprintf("%d件のフィードをインポートしました", count)}
	settings, serr := h.deps.Settings.Get()
	if serr == nil {
		page.Settings = settings
	}
	h.renderPartial(w, http.StatusOK, "_settings.html", page)
}
```

補足: `opmlImport`はio.ReadAllを使わず上限付きの逐次読み込みで過大アップロードを防ぎます。defer内のCloseはオーバービュー1節の共通規約に従いinternal/obsのCloseAndLog経由で記録し、握り潰しと非構造化出力の両方を避けます。OPMLのエクスポート応答への書き込みも同じくobs.WriteAndLogで記録します。obsのCloseAndLogとWriteAndLogはloggerにnilを渡すとslog.Defaultを使うため、handlerはloggerを保持せずにそのまま呼べます。`OPML.Import`は個別フィードの取得失敗で全体を中断せず、個別失敗をslog.Warnで記録して継続し、成功件数を返す契約です。handlerはその成功件数を受け取り、何件インポートしたかを画面へ表示します。

- [ ] Step 5: テストが通ることを確認する

Run:
```bash
cd /Users/yujiokamoto/devs/golang/feedflow-go-htmx && go test ./internal/handler/ -run 'TestSettings|TestOPML' -v
```
Expected: 5件すべてPASSします。

- [ ] Step 6: gofmtを適用してコミットする

```bash
cd /Users/yujiokamoto/devs/golang/feedflow-go-htmx && gofmt -w internal/handler/settings_handler.go internal/handler/settings_handler_test.go && git add internal/handler/settings_handler.go internal/handler/settings_handler_test.go internal/handler/templates/_settings.html && git commit -m "feat: 設定とOPMLのハンドラと設定画面テンプレートを追加する"
```

---

## Task 11: ルーティング登録を実装する

Files:
- Create: `/Users/yujiokamoto/devs/golang/feedflow-go-htmx/internal/handler/router.go`
- Test: `/Users/yujiokamoto/devs/golang/feedflow-go-htmx/internal/handler/router_test.go`

net/httpのServeMuxにメソッド付きパターンで全ルートを登録します。認証が必要なルートはrequireAuthを通し、状態変更系のPOSTはrequireCSRFを通します。全レスポンスにsecurityHeadersを付与します。アプリのトップ画面/appは2ペインを完全描画します。

- [ ] Step 1: 失敗するテストを書く

Create `internal/handler/router_test.go`:
```go
package handler

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/okamyuji/feedflow-go-htmx/internal/domain"
)

func newFullHandler(t *testing.T, authenticated bool) *Handler {
	t.Helper()
	subs := &stubSubscriptions{feeds: []domain.Feed{{ID: "f1", Title: "f1"}}}
	items := &stubItems{items: map[string][]domain.Item{
		"f1": {{ID: "i1", FeedID: "f1", Title: "記事1"}},
	}}
	h, err := New(Deps{
		Subscriptions:     subs,
		Items:             items,
		Mutes:             &stubMutes{},
		Settings:          &stubSettings{current: domain.DefaultSettings()},
		OPML:              &stubOPML{},
		Sessions:          &stubSessions{username: "owner", ok: authenticated},
		CSRF:              &stubCSRF{ok: true, token: "tok"},
		LoginLimiter:      stubLimiter{allow: true},
		Setup:             &stubSetup{needs: false, authOK: true},
		SessionCookieName: "feedflow_session",
	})
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}
	return h
}

func TestRouterHealthz(t *testing.T) {
	t.Parallel()
	h := newFullHandler(t, true)
	srv := h.Routes()
	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	rec := httptest.NewRecorder()

	srv.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status got %d want %d", rec.Code, http.StatusOK)
	}
	if rec.Header().Get("X-Content-Type-Options") != "nosniff" {
		t.Fatalf("security header missing on healthz")
	}
}

func TestRouterAppRequiresAuth(t *testing.T) {
	t.Parallel()
	h := newFullHandler(t, false)
	srv := h.Routes()
	req := httptest.NewRequest(http.MethodGet, "/app", nil)
	rec := httptest.NewRecorder()

	srv.ServeHTTP(rec, req)

	if rec.Code != http.StatusSeeOther {
		t.Fatalf("status got %d want %d", rec.Code, http.StatusSeeOther)
	}
	if loc := rec.Header().Get("Location"); loc != "/login" {
		t.Fatalf("Location got %q want %q", loc, "/login")
	}
}

func TestRouterAppRendersWhenAuthenticated(t *testing.T) {
	t.Parallel()
	h := newFullHandler(t, true)
	srv := h.Routes()
	req := httptest.NewRequest(http.MethodGet, "/app", nil)
	rec := httptest.NewRecorder()

	srv.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status got %d want %d", rec.Code, http.StatusOK)
	}
	if rec.Header().Get("Content-Type") == "" {
		t.Fatalf("Content-Type missing")
	}
}

func TestRouterStaticServed(t *testing.T) {
	t.Parallel()
	h := newFullHandler(t, true)
	srv := h.Routes()
	req := httptest.NewRequest(http.MethodGet, "/static/htmx.min.js", nil)
	rec := httptest.NewRecorder()

	srv.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status got %d want %d", rec.Code, http.StatusOK)
	}
}

func TestRouterItemActionRequiresCSRF(t *testing.T) {
	t.Parallel()
	subs := &stubSubscriptions{}
	items := &stubItems{items: map[string][]domain.Item{"f1": {{ID: "i1", FeedID: "f1"}}}}
	h, err := New(Deps{
		Subscriptions:     subs,
		Items:             items,
		Mutes:             &stubMutes{},
		Settings:          &stubSettings{current: domain.DefaultSettings()},
		OPML:              &stubOPML{},
		Sessions:          &stubSessions{username: "owner", ok: true},
		CSRF:              &stubCSRF{ok: false, token: "tok"},
		LoginLimiter:      stubLimiter{allow: true},
		Setup:             &stubSetup{},
		SessionCookieName: "feedflow_session",
	})
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}
	srv := h.Routes()
	req := httptest.NewRequest(http.MethodPost, "/app/items/f1/i1/read", nil)
	req.AddCookie(&http.Cookie{Name: "feedflow_session", Value: "sess-1"})
	rec := httptest.NewRecorder()

	srv.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("status got %d want %d", rec.Code, http.StatusForbidden)
	}
}
```

- [ ] Step 2: テストが失敗することを確認する

Run:
```bash
cd /Users/yujiokamoto/devs/golang/feedflow-go-htmx && go test ./internal/handler/ -run 'TestRouter' -v
```
Expected: コンパイルエラーで失敗します。`undefined: (*Handler).Routes` や `undefined: (*Handler).appPage` が表示されます。

- [ ] Step 3: アプリトップ画面とルーティングを実装する

Create `internal/handler/router.go`:
```go
package handler

import (
	"net/http"

	"github.com/okamyuji/feedflow-go-htmx/internal/domain"
)

// healthz 死活監視用のエンドポイントです。認証なしで応答します。
func (h *Handler) healthz(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	if _, err := w.Write([]byte("ok")); err != nil {
		return
	}
}

// appPage 2ペインとオーバーレイのアプリ画面を完全描画します。
func (h *Handler) appPage(w http.ResponseWriter, r *http.Request) {
	sess := sessionFromContext(r.Context())
	tree, err := h.buildTree()
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	items, err := h.listItemsFor(r)
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	views := make([]itemView, 0, len(items))
	for _, it := range items {
		views = append(views, toItemView(it))
	}
	settings, err := h.deps.Settings.Get()
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	data := pageData{
		Title:       "feedflow",
		Theme:       settings.Theme,
		CSRFToken:   sess.CSRFToken,
		Username:    sess.Username,
		DefaultView: settings.DefaultView,
		Tree:        tree,
		Items:       views,
	}
	if data.Theme == domain.Theme("") {
		data.Theme = domain.ThemeDark
	}
	h.renderPage(w, http.StatusOK, data)
}

// Routes 全ルートを登録したhttp.Handlerを返します。全レスポンスにセキュリティヘッダを付与します。
func (h *Handler) Routes() http.Handler {
	mux := http.NewServeMux()

	// 認証不要の公開ルートです。
	mux.HandleFunc("GET /healthz", h.healthz)
	mux.Handle("GET /static/", staticHandler())
	mux.HandleFunc("GET /login", h.loginPage)
	mux.Handle("POST /login", h.rateLimitLogin(http.HandlerFunc(h.loginSubmit)))
	mux.HandleFunc("GET /setup", h.setupPage)
	mux.HandleFunc("POST /setup", h.setupSubmit)
	mux.HandleFunc("POST /logout", h.logout)

	// 認証が必要な読み取り系ルートです。
	mux.Handle("GET /app", h.requireAuth(http.HandlerFunc(h.appPage)))
	mux.Handle("GET /app/items", h.requireAuth(http.HandlerFunc(h.itemList)))
	mux.Handle("GET /app/items/{feedID}/{itemID}", h.requireAuth(http.HandlerFunc(h.itemOverlay)))
	mux.Handle("GET /app/settings", h.requireAuth(http.HandlerFunc(h.settingsPage)))
	mux.Handle("GET /app/opml/export", h.requireAuth(http.HandlerFunc(h.opmlExport)))

	// 認証とCSRFが必要な状態変更系ルートです。
	mux.Handle("POST /app/feeds", h.requireAuth(h.requireCSRF(http.HandlerFunc(h.feedSubscribe))))
	mux.Handle("DELETE /app/feeds/{feedID}", h.requireAuth(h.requireCSRF(http.HandlerFunc(h.feedUnsubscribe))))
	mux.Handle("POST /app/items/markall", h.requireAuth(h.requireCSRF(http.HandlerFunc(h.itemMarkAll))))
	mux.Handle("POST /app/items/{feedID}/{itemID}/read", h.requireAuth(h.requireCSRF(http.HandlerFunc(h.itemMarkRead))))
	mux.Handle("POST /app/items/{feedID}/{itemID}/star", h.requireAuth(h.requireCSRF(http.HandlerFunc(h.itemStar))))
	mux.Handle("POST /app/items/{feedID}/{itemID}/readlater", h.requireAuth(h.requireCSRF(http.HandlerFunc(h.itemReadLater))))
	mux.Handle("POST /app/items/{feedID}/{itemID}/boards", h.requireAuth(h.requireCSRF(http.HandlerFunc(h.itemSetBoards))))
	mux.Handle("POST /app/settings", h.requireAuth(h.requireCSRF(http.HandlerFunc(h.settingsUpdate))))
	mux.Handle("POST /app/opml/import", h.requireAuth(h.requireCSRF(http.HandlerFunc(h.opmlImport))))

	return h.securityHeaders(mux)
}
```

- [ ] Step 4: テストが通ることを確認する

Run:
```bash
cd /Users/yujiokamoto/devs/golang/feedflow-go-htmx && go test ./internal/handler/ -run 'TestRouter' -v
```
Expected: 5件すべてPASSします。

- [ ] Step 5: gofmtを適用してコミットする

```bash
cd /Users/yujiokamoto/devs/golang/feedflow-go-htmx && gofmt -w internal/handler/router.go internal/handler/router_test.go && git add internal/handler/router.go internal/handler/router_test.go && git commit -m "feat: ルーティング登録とアプリトップ画面を追加する"
```

---

## Task 12: Alpine.jsのフロントスクリプトとCSSを実装する

Files:
- Create: `/Users/yujiokamoto/devs/golang/feedflow-go-htmx/internal/handler/static/app.js`
- Create: `/Users/yujiokamoto/devs/golang/feedflow-go-htmx/internal/handler/static/styles.css`

リーディングオーバーレイの開閉、テーマ切替、キーボードショートカット(jとkで移動、mで既読、sで保存)、スクロール追従の自動既読をAlpine.jsで実装します。Alpine.jsはCSPビルドのため、app.jsはAlpine.dataでfeedflowコンポーネントを登録し、テンプレートからはインライン式でなくメソッドとプロパティの参照だけを使います。これによりscript-src selfのCSP下でunsafe-evalなしで動きます。CSSはダークラグジュアリー配色でライトとダーク両対応とし、remベースの流体レイアウトでブレークポイントにより列構成を調整します。

- [ ] Step 1: app.jsを作成する

Create `internal/handler/static/app.js`:
```javascript
// feedflow Alpine.jsコンポーネントです。オーバーレイ開閉とテーマ切替とキーボードショートカットと自動既読を担います。
// CSPビルドのAlpineで動かすため、テンプレートのインライン式は使わず、ここで登録した
// プロパティとメソッドだけを参照します。script-srcはselfに限定しunsafe-evalを使いません。

// csrfToken metaタグからCSRFトークンを取得します。
function csrfToken() {
  const meta = document.querySelector('meta[name="csrf-token"]');
  return meta ? meta.getAttribute("content") : "";
}

// postAction 状態変更系のアクションをCSRFトークン付きで送信します。
async function postAction(url, params) {
  const body = new URLSearchParams(params || {});
  await fetch(url, {
    method: "POST",
    headers: {
      "Content-Type": "application/x-www-form-urlencoded",
      "X-CSRF-Token": csrfToken(),
    },
    body: body.toString(),
  });
}

// cardActionData クリック起点の要素から所属カードのフィードIDと記事IDを取り出します。
function cardActionData(target) {
  const card = target.closest(".item-card");
  if (!card) {
    return { feedID: "", itemID: "", card: null };
  }
  return {
    feedID: card.getAttribute("data-feed") || "",
    itemID: card.getAttribute("data-item") || "",
    card,
  };
}

// registerFeedflow Alpineの初期化時にfeedflowコンポーネントを登録します。
function registerFeedflow() {
  window.Alpine.data("feedflow", () => ({
    theme: "dark",
    overlayOpen: false,
    activeFeed: "",
    activeItem: "",

    init() {
      const saved = localStorage.getItem("feedflow-theme");
      const initial = document.documentElement.getAttribute("data-theme");
      if (saved === "dark" || saved === "light") {
        this.theme = saved;
      } else if (initial === "dark" || initial === "light") {
        this.theme = initial;
      }
      this.applyTheme();
    },

    get themeLabel() {
      return this.theme === "dark" ? "昼" : "夜";
    },

    applyTheme() {
      document.documentElement.setAttribute("data-theme", this.theme);
    },

    toggleTheme() {
      this.theme = this.theme === "dark" ? "light" : "dark";
      this.applyTheme();
      localStorage.setItem("feedflow-theme", this.theme);
    },

    openOverlay(event) {
      const { feedID, itemID, card } = cardActionData(event.currentTarget);
      this.activeFeed = feedID;
      this.activeItem = itemID;
      this.overlayOpen = true;
      if (card) {
        card.classList.add("is-read");
      }
    },

    closeOverlay() {
      this.overlayOpen = false;
      this.activeFeed = "";
      this.activeItem = "";
    },

    onOverlayScroll(event) {
      const el = event.target;
      const reachedEnd = el.scrollTop + el.clientHeight >= el.scrollHeight - 4;
      if (reachedEnd && this.activeItem) {
        postAction(
          "/app/items/" + this.activeFeed + "/" + this.activeItem + "/read",
          { read: "true" }
        );
      }
    },

    focusNextCard(delta) {
      const cards = Array.from(document.querySelectorAll(".item-card"));
      if (cards.length === 0) {
        return;
      }
      const current = document.activeElement.closest(".item-card");
      let index = current ? cards.indexOf(current) : -1;
      index = Math.min(Math.max(index + delta, 0), cards.length - 1);
      const link = cards[index].querySelector(".item-open");
      if (link) {
        link.focus();
      }
    },

    onKey(event) {
      const tag = (event.target.tagName || "").toLowerCase();
      if (tag === "input" || tag === "textarea" || tag === "select") {
        return;
      }
      if (event.key === "Escape" && this.overlayOpen) {
        this.closeOverlay();
        return;
      }
      if (event.key === "j") {
        this.focusNextCard(1);
        event.preventDefault();
        return;
      }
      if (event.key === "k") {
        this.focusNextCard(-1);
        event.preventDefault();
        return;
      }
      const card = document.activeElement.closest(".item-card");
      if (!card) {
        return;
      }
      const feedID = card.getAttribute("data-feed");
      const itemID = card.getAttribute("data-item");
      if (event.key === "m") {
        postAction("/app/items/" + feedID + "/" + itemID + "/read", { read: "true" });
        card.classList.add("is-read");
        event.preventDefault();
      }
      if (event.key === "s") {
        postAction("/app/items/" + feedID + "/" + itemID + "/star", { starred: "true" });
        event.preventDefault();
      }
    },
  }));
}

document.addEventListener("alpine:init", registerFeedflow);
```

補足: Alpine.jsのCSPビルドはインライン式を評価しないため、`window.Alpine.data`でコンポーネントを登録し、テンプレートからはここで定義したメソッドとプロパティだけを参照します。`openOverlay`はテンプレートから引数を受け取らず、`event.currentTarget`から所属カードのdata属性を読み取って対象を特定します。テーマのラベルは算出プロパティ`themeLabel`で返します。app.jsはalpine.min.jsより前に読み込み、`alpine:init`で登録が走るようにします。

- [ ] Step 2: styles.cssを作成する

Create `internal/handler/static/styles.css`:
```css
/* feedflow スタイルシートです。ダークラグジュアリー配色でライトとダーク両対応とし、remベースの流体レイアウトを採用します。 */

:root {
  --space-1: 0.5rem;
  --space-2: 1rem;
  --space-3: 1.5rem;
  --space-4: 2rem;
  --radius: 0.625rem;
  --text-sm: clamp(0.8rem, 0.76rem + 0.2vw, 0.9rem);
  --text-base: clamp(0.95rem, 0.9rem + 0.3vw, 1.05rem);
  --text-lg: clamp(1.25rem, 1.05rem + 1vw, 1.75rem);
  --duration: 220ms;
  --ease: cubic-bezier(0.16, 1, 0.3, 1);
}

[data-theme="dark"] {
  --bg: oklch(20% 0.03 264);
  --surface: oklch(26% 0.04 264);
  --surface-2: oklch(30% 0.04 264);
  --border: oklch(40% 0.03 264);
  --text: oklch(94% 0.01 264);
  --text-muted: oklch(72% 0.02 264);
  --accent: oklch(80% 0.16 80);
  --accent-2: oklch(78% 0.13 210);
}

[data-theme="light"] {
  --bg: oklch(97% 0.01 264);
  --surface: oklch(99% 0.005 264);
  --surface-2: oklch(94% 0.01 264);
  --border: oklch(85% 0.02 264);
  --text: oklch(24% 0.03 264);
  --text-muted: oklch(48% 0.03 264);
  --accent: oklch(58% 0.16 70);
  --accent-2: oklch(55% 0.14 220);
}

* {
  box-sizing: border-box;
}

body {
  margin: 0;
  font-family: system-ui, -apple-system, "Hiragino Sans", "Noto Sans JP", sans-serif;
  font-size: var(--text-base);
  color: var(--text);
  background: var(--bg);
  line-height: 1.6;
}

.app-shell {
  display: flex;
  flex-direction: column;
  min-height: 100vh;
}

.app-bar {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: var(--space-1) var(--space-3);
  border-bottom: 1px solid var(--border);
  background: var(--surface);
}

.app-brand {
  font-size: var(--text-lg);
  font-weight: 700;
  letter-spacing: 0.02em;
  color: var(--accent);
}

.app-actions {
  display: flex;
  gap: var(--space-1);
  align-items: center;
}

.inline-form {
  display: inline;
}

.app-body {
  display: grid;
  grid-template-columns: 1fr;
  flex: 1;
  min-height: 0;
}

@media (min-width: 48rem) {
  .app-body {
    grid-template-columns: 18rem 1fr;
  }
}

@media (min-width: 80rem) {
  .app-body {
    grid-template-columns: 22rem 1fr;
  }
}

.tree-pane {
  border-right: 1px solid var(--border);
  padding: var(--space-2);
  background: var(--surface);
  overflow-y: auto;
}

.tree-list {
  list-style: none;
  margin: 0 0 var(--space-3);
  padding: 0;
}

.tree-link {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: var(--space-1);
  padding: var(--space-1) var(--space-2);
  border-radius: var(--radius);
  color: var(--text);
  text-decoration: none;
  transition: background var(--duration) var(--ease);
}

.tree-link:hover,
.tree-link:focus-visible {
  background: var(--surface-2);
  outline: none;
}

.tree-badge {
  min-width: 1.5rem;
  padding: 0 var(--space-1);
  border-radius: 999px;
  font-size: var(--text-sm);
  text-align: center;
  color: var(--bg);
  background: var(--accent-2);
}

.tree-error .tree-label {
  color: var(--accent);
}

.tree-error-mark {
  color: var(--accent);
  font-weight: 700;
}

.main-pane {
  padding: var(--space-3);
  overflow-y: auto;
}

.item-list-bar {
  display: flex;
  justify-content: flex-end;
  margin-bottom: var(--space-2);
}

.item-cards {
  list-style: none;
  margin: 0;
  padding: 0;
  display: grid;
  gap: var(--space-2);
}

@media (min-width: 64rem) {
  .item-list[data-view="card"] .item-cards {
    grid-template-columns: repeat(2, 1fr);
  }
}

.item-card {
  border: 1px solid var(--border);
  border-radius: var(--radius);
  padding: var(--space-2);
  background: var(--surface);
  transition: transform var(--duration) var(--ease), border-color var(--duration) var(--ease);
}

.item-card:hover {
  transform: translateY(-2px);
  border-color: var(--accent-2);
}

.item-card.is-read {
  opacity: 0.65;
}

.item-open {
  display: block;
  color: inherit;
  text-decoration: none;
}

.item-title {
  margin: 0 0 var(--space-1);
  font-size: var(--text-lg);
  line-height: 1.3;
}

.item-summary {
  margin: 0 0 var(--space-1);
  color: var(--text-muted);
}

.item-meta {
  display: flex;
  gap: var(--space-2);
  font-size: var(--text-sm);
  color: var(--text-muted);
}

.item-quick {
  display: flex;
  gap: var(--space-1);
  margin-top: var(--space-1);
}

.quick-btn,
.btn-ghost,
.icon-btn {
  border: 1px solid var(--border);
  border-radius: var(--radius);
  padding: var(--space-1) var(--space-2);
  background: var(--surface-2);
  color: var(--text);
  font-size: var(--text-sm);
  cursor: pointer;
  transition: background var(--duration) var(--ease);
}

.quick-btn:hover,
.btn-ghost:hover,
.icon-btn:hover {
  background: var(--border);
}

.btn-primary {
  border: none;
  border-radius: var(--radius);
  padding: var(--space-1) var(--space-3);
  background: var(--accent);
  color: var(--bg);
  font-weight: 700;
  cursor: pointer;
  transition: filter var(--duration) var(--ease);
}

.btn-primary:hover {
  filter: brightness(1.08);
}

.overlay-backdrop {
  position: fixed;
  inset: 0;
  background: oklch(10% 0.02 264 / 0.6);
  backdrop-filter: blur(2px);
  z-index: 10;
}

.reading-overlay {
  position: fixed;
  top: 0;
  right: 0;
  bottom: 0;
  width: min(46rem, 100%);
  padding: var(--space-4);
  background: var(--surface);
  border-left: 1px solid var(--border);
  overflow-y: auto;
  z-index: 20;
  box-shadow: -0.5rem 0 2rem oklch(10% 0.02 264 / 0.35);
}

.reading-head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  margin-bottom: var(--space-3);
}

.reading-actions {
  display: flex;
  gap: var(--space-1);
}

.reading-title {
  font-size: clamp(1.5rem, 1.2rem + 1.5vw, 2.25rem);
  line-height: 1.25;
  margin: 0 0 var(--space-2);
}

.reading-meta {
  display: flex;
  gap: var(--space-2);
  color: var(--text-muted);
  font-size: var(--text-sm);
  margin-bottom: var(--space-3);
}

.reading-body {
  font-size: var(--text-base);
}

.reading-body img {
  max-width: 100%;
  height: auto;
}

.subscribe-form,
.settings-form,
.auth-form {
  display: flex;
  flex-direction: column;
  gap: var(--space-2);
}

.field {
  display: flex;
  flex-direction: column;
  gap: var(--space-1);
}

.field-label {
  font-size: var(--text-sm);
  color: var(--text-muted);
}

.field-input {
  padding: var(--space-1) var(--space-2);
  border: 1px solid var(--border);
  border-radius: var(--radius);
  background: var(--surface-2);
  color: var(--text);
  font-size: var(--text-base);
}

.field-input:focus-visible {
  outline: 2px solid var(--accent-2);
  outline-offset: 1px;
}

.auth-page {
  display: grid;
  place-items: center;
  min-height: 100vh;
}

.auth-card {
  width: min(24rem, 92vw);
  padding: var(--space-4);
  border: 1px solid var(--border);
  border-radius: var(--radius);
  background: var(--surface);
}

.auth-title {
  margin: 0 0 var(--space-2);
  color: var(--accent);
}

.auth-error,
.settings-flash {
  margin: 0 0 var(--space-2);
  padding: var(--space-1) var(--space-2);
  border-radius: var(--radius);
  background: var(--surface-2);
  color: var(--accent);
  font-size: var(--text-sm);
}

.settings,
.boards {
  max-width: 40rem;
}

.opml-actions {
  margin-top: var(--space-4);
  padding-top: var(--space-3);
  border-top: 1px solid var(--border);
  display: flex;
  flex-direction: column;
  gap: var(--space-2);
}

.opml-import {
  display: flex;
  gap: var(--space-1);
  align-items: center;
}

@media (prefers-reduced-motion: reduce) {
  * {
    transition: none !important;
  }
}
```

- [ ] Step 3: テンプレートとembedの整合を再確認する

Run:
```bash
cd /Users/yujiokamoto/devs/golang/feedflow-go-htmx && go test ./internal/handler/ -run 'TestParseTemplates|TestStatic|TestRouter' -v
```
Expected: テンプレート読み込みと静的配信とルーティングのテストがすべてPASSします。app.jsとstyles.cssはembedの対象に含まれ、/static配下で配信されます。

- [ ] Step 4: コミットする

```bash
cd /Users/yujiokamoto/devs/golang/feedflow-go-htmx && git add internal/handler/static/app.js internal/handler/static/styles.css && git commit -m "feat: Alpine.js のフロントスクリプトとダークラグジュアリーの CSS を追加する"
```

---

## Task 13: cmd/feedflowへハンドラを結線する

Files:
- Create: `/Users/yujiokamoto/devs/golang/feedflow-go-htmx/internal/sys/clock.go`
- Create: `/Users/yujiokamoto/devs/golang/feedflow-go-htmx/internal/sys/idgen.go`
- Modify: `/Users/yujiokamoto/devs/golang/feedflow-go-htmx/cmd/feedflow/main.go`

port.Clockとport.IDGenの本番用の具象実装が設計に欠けていたため、このフェーズで`internal/sys`へ新設します。`SystemClock`がport.Clockを、`RandomIDGen`がport.IDGenを満たし、cmd/feedflow/main.goの`buildApp`関数で各層へ注入します。cmd/feedflow/main.goは`buildApp`関数で全依存を組み立て、ルーティング済みハンドラとバックグラウンドのポーラーを返します。`buildApp`はinternal/sysの`SystemClock`(port.Clock)と`RandomIDGen`(port.IDGen)を生成し、feedのフェッチャとパーサ、Phase4とPhase5のservice、Phase6のauthをそれぞれ生成して各層へ注入します。各層はインターフェース経由で受け取り、具象はこの関数だけが知ります。`run`関数はシグナルを待ってグレースフルシャットダウンを行い、起動時にポーラーをバックグラウンドで走らせます。

- [ ] Step 1: port.Clockとport.IDGenの具象実装を作成する

Create `internal/sys/clock.go`:
```go
// Package sys feedflowの本番用の時刻源とID生成の具象実装を提供します。
// port.Clockとport.IDGenを満たし、main.goから各層へコンストラクタ注入します。
// テストではこれらを使わずフェイクを注入するため、この層は薄く保ちます。
package sys

import "time"

// SystemClock 実時刻を返すport.Clockの実装です。
type SystemClock struct{}

// Now 現在時刻を返します。
func (SystemClock) Now() time.Time {
	return time.Now()
}
```

Create `internal/sys/idgen.go`:
```go
package sys

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
)

// idBytes 生成するIDの乱数バイト数です。16バイトで128ビットの一意性を確保します。
const idBytes = 16

// RandomIDGen 暗号論的乱数から一意なIDを生成するport.IDGenの実装です。
type RandomIDGen struct{}

// NewRandomIDGen RandomIDGenを生成します。
func NewRandomIDGen() RandomIDGen {
	return RandomIDGen{}
}

// NewID 16バイトの乱数を16進文字列にしたIDを返します。
// crypto/randの読み取りはOSのエントロピー枯渇など致命的な状況でのみ失敗します。
// port.IDGenはerrorを返さない契約のため、その稀な失敗はpanicで顕在化させ、握り潰しません。
func (RandomIDGen) NewID() string {
	b := make([]byte, idBytes)
	if _, err := rand.Read(b); err != nil {
		panic(fmt.Sprintf("sys: failed to read crypto/rand for id: %v", err))
	}
	return hex.EncodeToString(b)
}
```

- [ ] Step 2: main.goを更新する

Replace `cmd/feedflow/main.go` with:
```go
// Package main feedflowのエントリポイントを提供します。
// 設定を環境変数から読み、各層の具象を生成してインターフェース経由で注入し、
// HTTPサーバとバックグラウンドのポーラーを起動します。
package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/okamyuji/feedflow-go-htmx/internal/auth"
	"github.com/okamyuji/feedflow-go-htmx/internal/feed"
	"github.com/okamyuji/feedflow-go-htmx/internal/handler"
	"github.com/okamyuji/feedflow-go-htmx/internal/poller"
	"github.com/okamyuji/feedflow-go-htmx/internal/service"
	"github.com/okamyuji/feedflow-go-htmx/internal/store"
	"github.com/okamyuji/feedflow-go-htmx/internal/sys"
)

// version ビルド時に-ldflagsで埋め込むバージョン文字列です。
var version = "dev"

// sessionCookieName セッションIDを載せるCookie名です。
const sessionCookieName = "feedflow_session"

// sessionTTL セッションの有効期間です。個人利用のため長めに取ります。
const sessionTTL = 30 * 24 * time.Hour

// loginBurst ログイン試行で同時に許す回数です。
const loginBurst = 5

// loginRefill ログイン試行トークンを1個補充する間隔です。
const loginRefill = time.Minute

func main() {
	if err := run(); err != nil {
		log.Fatalf("feedflow exited with error: %v", err)
	}
}

// run アプリを組み立ててサーバとポーラーを起動し、終了シグナルまで動かします。
func run() error {
	addr := envOr("FEEDFLOW_ADDR", ":8080")

	routes, runner, err := buildApp()
	if err != nil {
		return err
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	go runner.Run(ctx)

	srv := &http.Server{
		Addr:              addr,
		Handler:           routes,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	errCh := make(chan error, 1)
	go func() {
		log.Printf("feedflow %s listening on %s", version, addr)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
		}
	}()

	select {
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := srv.Shutdown(shutdownCtx); err != nil {
			return fmt.Errorf("failed to shut down server: %w", err)
		}
		return nil
	case err := <-errCh:
		return fmt.Errorf("server error: %w", err)
	}
}

// buildApp 設定から全依存を組み立て、ルーティング済みハンドラとポーラーを返します。
// 各層はインターフェース経由で注入し、具象はこの関数だけが知ります。
func buildApp() (http.Handler, *poller.Runner, error) {
	dataDir := envOr("FEEDFLOW_DATA_DIR", "./data")
	baseURL := envOr("FEEDFLOW_BASE_URL", "")
	isHTTPS := strings.HasPrefix(baseURL, "https://")

	repo, err := store.New(dataDir)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to open store at %s: %w", dataDir, err)
	}

	clock := sys.SystemClock{}
	ids := sys.NewRandomIDGen()
	fetcher := feed.NewHTTPFetcher()
	parser := feed.NewXMLParser()

	sdeps := service.Deps{Repo: repo, Fetch: fetcher, Parse: parser, Clock: clock, IDs: ids}
	mute := service.NewMuteService(sdeps)
	subs := service.NewSubscriptionService(sdeps)
	items := service.NewItemService(sdeps, mute)
	retention := service.NewRetentionService(sdeps)
	opml := service.NewOPMLService(sdeps, subs)
	settings := service.NewSettingsService(sdeps)
	pollSvc := poller.NewService(repo, fetcher, parser, clock, ids, mute)
	runner := poller.NewRunner(pollSvc, repo, clock, poller.DefaultConfig())

	sessions := auth.NewSessionStore(auth.SessionConfig{
		Clock:      clock,
		TTL:        sessionTTL,
		CookieName: sessionCookieName,
		Secure:     isHTTPS,
	})
	csrf := auth.NewCSRFStore()
	limiter := auth.NewRateLimiter(auth.RateLimitConfig{
		Clock:       clock,
		Burst:       loginBurst,
		RefillEvery: loginRefill,
	})
	manager := auth.NewManager(repo, auth.DefaultParams())

	h, err := handler.New(handler.Deps{
		Subscriptions:     subs,
		Items:             items,
		Retention:         retention,
		Mutes:             mute,
		OPML:              opml,
		Settings:          settings,
		Poll:              pollSvc,
		Sessions:          sessions,
		CSRF:              csrf,
		LoginLimiter:      limiter,
		Setup:             manager,
		SessionCookieName: sessionCookieName,
		IsHTTPS:           isHTTPS,
	})
	if err != nil {
		return nil, nil, fmt.Errorf("failed to build handler: %w", err)
	}

	return h.Routes(), runner, nil
}

// envOr 環境変数keyの値を返します。未設定や空のときはdefを返します。
func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
```

補足: Phase0の`healthz`関数とそのテストはhandlerパッケージへ移したため、cmd側からは削除します。port.Clockとport.IDGenの具象実装はこのフェーズで`internal/sys`に新設します。`SystemClock`がport.Clockを、`RandomIDGen`がport.IDGenを満たし、`buildApp`がそれぞれを生成して各層へ注入します。Cookieの`SessionConfig.CookieName`は`Deps.SessionCookieName`と同じ`sessionCookieName`定数を使い、`requireAuth`が同じCookie名でセッションIDを読み取れるようにします。`FEEDFLOW_BASE_URL`のスキームがhttpsかどうかを判定し、その`isHTTPS`を`Deps.IsHTTPS`と`SessionConfig.Secure`の両方へ同じ判定で渡します。これによりCookieのSecure属性とHSTS付与が同じhttps判定で駆動され、平文の開発時にSecure既定がfalseになり、本番httpsでtrueになります。

- [ ] Step 3: cmdのテストを更新する

Replace `cmd/feedflow/main_test.go` with:
```go
package main

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestBuildApp_HealthzOK(t *testing.T) {
	t.Setenv("FEEDFLOW_DATA_DIR", t.TempDir())

	routes, runner, err := buildApp()
	if err != nil {
		t.Fatalf("buildApp returned error: %v", err)
	}
	if runner == nil {
		t.Fatal("buildApp returned nil runner")
	}

	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	rec := httptest.NewRecorder()
	routes.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status got %d want %d", rec.Code, http.StatusOK)
	}
	if got := rec.Body.String(); got != "ok" {
		t.Fatalf("body got %q want %q", got, "ok")
	}
}

func TestBuildApp_RedirectsToSetupWhenUnregistered(t *testing.T) {
	t.Setenv("FEEDFLOW_DATA_DIR", t.TempDir())

	routes, _, err := buildApp()
	if err != nil {
		t.Fatalf("buildApp returned error: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/app", nil)
	rec := httptest.NewRecorder()
	routes.ServeHTTP(rec, req)

	if rec.Code != http.StatusSeeOther && rec.Code != http.StatusFound {
		t.Fatalf("未登録での /app アクセスはリダイレクトを期待しますが got %d でした", rec.Code)
	}
}

func TestEnvOr_ReturnsValueWhenSet(t *testing.T) {
	t.Setenv("FEEDFLOW_TEST_ENVOR", "value")
	if got := envOr("FEEDFLOW_TEST_ENVOR", "fallback"); got != "value" {
		t.Fatalf("envOr set got %q want %q", got, "value")
	}
}

func TestEnvOr_ReturnsDefaultWhenUnsetOrEmpty(t *testing.T) {
	if got := envOr("FEEDFLOW_TEST_ENVOR_UNSET", "fallback"); got != "fallback" {
		t.Fatalf("envOr unset got %q want %q", got, "fallback")
	}
	t.Setenv("FEEDFLOW_TEST_ENVOR_EMPTY", "")
	if got := envOr("FEEDFLOW_TEST_ENVOR_EMPTY", "fallback"); got != "fallback" {
		t.Fatalf("envOr empty got %q want %q", got, "fallback")
	}
}
```

補足: `buildApp`は実依存を組み立てるため、テストは`FEEDFLOW_DATA_DIR`へ一時ディレクトリを渡してから呼びます。/healthzは認証不要のため200と`ok`を返し、未登録状態の/appアクセスは初回セットアップへリダイレクトします。これによりcmdの結線が実依存込みで正しいことを検証できます。

- [ ] Step 4: テストが通ることを確認する

Run:
```bash
cd /Users/yujiokamoto/devs/golang/feedflow-go-htmx && go test ./cmd/feedflow/ -run 'TestBuildApp|TestEnvOr' -v
```
Expected: すべてPASSします。

- [ ] Step 5: gofmtを適用してコミットする

```bash
cd /Users/yujiokamoto/devs/golang/feedflow-go-htmx && gofmt -w internal/sys/clock.go internal/sys/idgen.go cmd/feedflow/main.go cmd/feedflow/main_test.go && git add internal/sys/clock.go internal/sys/idgen.go cmd/feedflow/main.go cmd/feedflow/main_test.go && git commit -m "feat: port.Clockとport.IDGenの具象実装を追加しcmd/feedflowへ全依存を結線する"
```

---

## Task 14: フェーズ全体のテストと品質ゲート

Files:
- 変更なし

- [ ] Step 1: ハンドラとcmdの全テストをraceで実行する

Run:
```bash
cd /Users/yujiokamoto/devs/golang/feedflow-go-htmx && go test -race -count=1 ./internal/handler/... ./cmd/feedflow/...
```
Expected: 両パッケージとも `ok` と表示されます。

- [ ] Step 2: カバレッジを確認する

Run:
```bash
cd /Users/yujiokamoto/devs/golang/feedflow-go-htmx && go test -coverprofile=coverage.out ./internal/handler/... && go tool cover -func=coverage.out | tail -n 1
```
Expected: handlerパッケージの合計カバレッジが80パーセント前後以上になります。目安であり厳密な合否基準ではありません。

- [ ] Step 3: 品質ゲートを実行する

Run:
```bash
cd /Users/yujiokamoto/devs/golang/feedflow-go-htmx && bash scripts/quality-gate.sh
```
Expected: `all quality checks passed` で終わります。lintやvetやgosecの指摘が出たら修正してから再実行します。本文を出力するtemplate.HTMLの箇所はHTMLEscapeStringで事前にエスケープ済みのため、gosecのG203相当の指摘が出た場合はnolintコメントの妥当性を確認します。

- [ ] Step 4: 品質ゲート緑のままコミットする

```bash
cd /Users/yujiokamoto/devs/golang/feedflow-go-htmx && git add -A && git commit -m "chore: Phase7 のハンドラとUIで品質ゲートを緑化する"
```
Expected: コミット時にquality-gateが走り、緑のままコミットされます。差分が無ければこのコミットは省略できます。

---

## Phase7 完了条件

- [ ] `go test -race ./internal/handler/... ./cmd/feedflow/...` が通る
- [ ] ルーティング(/healthz、/login、/setup、/logout、/app、/app/items、/app/items/{feedID}/{itemID}、/app/settings、/app/opml/export、/app/feeds、/app/items系のPOST、/app/settings、/app/opml/import)が登録されている
- [ ] ミドルウェア(requireAuth、requireCSRF、rateLimitLogin、securityHeaders)が動作し、テストで検証されている
- [ ] embedテンプレートのParseFSとFuncMap(JST整形、truncate、isDark)が動作する
- [ ] ログインと初回セットアップ画面が動作し、登録済みでは初回セットアップが無効化される
- [ ] 購読の追加削除一覧が動作する
- [ ] 記事一覧と本文オーバーレイと既読とスター操作が動作する
- [ ] ボード操作が動作する
- [ ] 設定とOPMLの入出力が動作する
- [ ] レイアウトB(2ペインとリーディングオーバーレイ)とダークラグジュアリー配色のライトとダーク両対応のCSSがある
- [ ] HTMXとAlpine.jsがベンダーした静的ファイルとしてembedで同梱され、CDNに依存しない
- [ ] CSSがremベースの流体レイアウトである
- [ ] net/http/httptestでハンドラを統合テストしている
- [ ] `bash scripts/quality-gate.sh` が `all quality checks passed` で終わる
