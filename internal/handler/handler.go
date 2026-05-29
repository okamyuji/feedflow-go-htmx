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
	CSRF              CSRF                     // CSRFトークンの発行と検証を担います
	LoginLimiter      RateLimiter              // ログイン試行のレート制限を担います
	Setup             SetupGuard               // 初回セットアップの可否判定と登録を担います
	SessionCookieName string                   // セッションIDを読み取るCookie名です。auth.SessionCookieNameを渡します
	IsHTTPS           bool                     // 公開URLがhttpsかどうかです。HSTS付与の判定に使います
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
