package handler

import (
	"bytes"
	"embed"
	"fmt"
	"html"
	"html/template"
	"log/slog"
	"net/http"
	"regexp"
	"strings"
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
	Title            string              // ブラウザのタイトルです
	Theme            domain.Theme        // 適用するテーマです
	CSRFToken        string              // フォームに埋め込むCSRFトークンです
	Username         string              // ログイン中のユーザー名です
	DefaultView      domain.ViewMode     // 記事リストの既定表示形式です
	Tree             []feedTreeNode      // 左ペインの購読ツリーです
	Items            []itemView          // 右ペインの記事リストです
	ActiveItem       *itemView           // オーバーレイで開いている記事です
	Bookmarks        []domain.Bookmark   // ブックマーク一覧です
	Filters          []domain.MuteFilter // ミュートフィルタ一覧です
	Settings         domain.Settings     // 設定画面で編集する設定です
	Flash            string              // 操作結果の通知メッセージです
	MainView         string              // フルページ描画時にmain-paneへ出す内容の種別です。空ならitem_list、settingsなら設定画面です
	TreeOOB          bool                // ツリーペインをHTMXのout-of-bandスワップで差し替えるかどうかです
	AutoReadOnScroll bool                // オーバーレイのスクロール自動既読を有効にするかどうかです
	BulkRead         string              // 一括既読コントロールの表示範囲です。feedは表示中フィード、allは全フィード、noneは非表示を表します
	CurrentFeedID    string              // 表示中フィードのIDです。BulkReadがfeedのときに使います
	CurrentFeedTitle string              // 表示中フィードの名称です。一括既読ボタンのラベルに使います
	CurrentLabel     string              // 右ペイン左上に出す、選択中の項目名です。すべて、既読、ブックマーク、あとで読む、フィード名のいずれかです
	ManualPollURL    string              // 現在の表示条件を保ったまま手動取得するHTMX送信先です
	ShowAddURL       bool                // ブックマークビューで任意URLの追加フォームを出すかどうかです
	AddURLPostURL    string              // URL追加フォームの送信先です。現在の表示条件をクエリで引き継ぎます
	AddURLError      string              // URL追加に失敗したときに入力欄の上へ出す文言です
	BookmarkOptions  []bookmarkOption    // URL追加フォームのラベル選択肢です
}

// feedTreeNode 左ペインの購読ツリーの1ノードを表します。
type feedTreeNode struct {
	Kind           string         // ノード種別です(all、read、bookmark、bookmarkItem、readlater、category、feed のいずれか)
	ID             string         // フィードやカテゴリやブックマークのIDです
	Label          string         // 表示名です
	UnreadCount    int            // 未読件数です
	HasError       bool           // フィードがエラー状態かどうかです
	Active         bool           // 現在選択中のノードかどうかです。選択中は左ペインで強調表示します
	Children       []feedTreeNode // 展開時に表示する子ノードです。ブックマークノードの名称コレクションに使います
	UnreadGroupEnd bool           // 未読フィード群の末尾かどうかです。次の通常フィード群との区切り線描画に使います
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
	Bookmarked  bool          // ブックマーク(保存)済みかどうかです
	ReadLater   bool          // あとで読む済みかどうかです
	HasContent  bool          // 本文を持つかどうかです。本文の無い外部リンクはオーバーレイを開かず元記事を直接開きます
	UnreadStart bool          // 単一フィード表示で既読先頭群の直後、未読の開始位置かどうかです。区切り線描画に使います
	CardOOB     bool          // HTMX out-of-bandでカードを差し替えるかどうかです
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

// htmlTagRe HTMLタグにマッチする正規表現です。要約のタグ除去に使います。
var htmlTagRe = regexp.MustCompile(`<[^>]*>`)

// stripHTML HTMLタグを除去しエンティティを復号して、空白を整理したプレーンテキストを返します。
// 記事一覧の要約は生HTMLを表示せず読みやすいテキストにします。
func stripHTML(s string) string {
	noTags := htmlTagRe.ReplaceAllString(s, " ")
	text := html.UnescapeString(noTags)
	return strings.Join(strings.Fields(text), " ")
}

// templateFuncs テンプレートに登録する関数群を返します。
func templateFuncs() template.FuncMap {
	return template.FuncMap{
		"jst":      formatJST,
		"truncate": truncateRunes,
		"isDark": func(theme domain.Theme) bool {
			return theme == domain.ThemeDark
		},
		"staticVersion": staticVersion,
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

// renderWithTreeOOB 主たる部分テンプレートに続けて、ツリーペインをHTMXのout-of-bandスワップで差し替える断片を付けて描画します。
// 既読操作のレスポンスにツリーを同梱して、左ペインの未読数をリアルタイムに更新します。
func (h *Handler) renderWithTreeOOB(w http.ResponseWriter, r *http.Request, status int, name string, primary any) {
	var buf bytes.Buffer
	if err := h.templates.ExecuteTemplate(&buf, name, primary); err != nil {
		slog.Error("failed to execute template", "template", name, "error", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	tree, err := h.treeData(r)
	if err != nil {
		slog.Error("failed to build tree for oob swap", "error", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	tree.TreeOOB = true
	if err := h.templates.ExecuteTemplate(&buf, "_tree_pane.html", tree); err != nil {
		slog.Error("failed to execute template", "template", "_tree_pane.html", "error", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(status)
	if _, err := buf.WriteTo(w); err != nil {
		slog.Error("failed to write rendered template with oob tree", "template", name, "error", err)
	}
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
func (h *Handler) renderShellPage(w http.ResponseWriter, r *http.Request, sess Session, title string, data pageData) {
	tree, err := h.buildTree()
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	tree = markActiveNodes(tree, r)
	settings, err := h.deps.Settings.Get()
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	data.Tree = tree
	data.Title = title
	data.Username = sess.Username
	data.Theme = settings.Theme
	data.AutoReadOnScroll = settings.AutoReadOnScroll
	if data.Theme == domain.Theme("") {
		data.Theme = domain.ThemeDark
	}
	if data.DefaultView == domain.ViewMode("") {
		data.DefaultView = settings.DefaultView
	}
	h.renderPage(w, http.StatusOK, data)
}
