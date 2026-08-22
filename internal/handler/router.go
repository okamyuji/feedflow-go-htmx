package handler

import (
	"log/slog"
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

// faviconSVG 落ち着いた青系のRSSグリフを描いたファビコンです。白黒を避け、ブランドに合わせた色付きにします。
const faviconSVG = `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 32 32">` +
	`<rect width="32" height="32" rx="7" fill="rgb(74,120,196)"/>` +
	`<circle cx="10" cy="22" r="2.6" fill="white"/>` +
	`<path d="M8 8a16 16 0 0 1 16 16" fill="none" stroke="white" stroke-width="3.2" stroke-linecap="round"/>` +
	`<path d="M8 14a10 10 0 0 1 10 10" fill="none" stroke="white" stroke-width="3.2" stroke-linecap="round"/>` +
	`</svg>`

// favicon 色付きのSVGファビコンを返します。ブラウザの/favicon.ico要求と<link>の両方から利用します。
func (h *Handler) favicon(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "image/svg+xml")
	w.Header().Set("Cache-Control", "public, max-age=86400")
	if _, err := w.Write([]byte(faviconSVG)); err != nil {
		slog.Error("failed to write favicon", "error", err)
	}
}

// rootIndex ルートパスへのアクセスをアプリ画面へ誘導します。未認証なら/loginへ、オーナー未登録なら/setupへ順に転送されます。
func (h *Handler) rootIndex(w http.ResponseWriter, r *http.Request) {
	http.Redirect(w, r, "/app", http.StatusSeeOther)
}

// appPage 2ペインとオーバーレイのアプリ画面を完全描画します。
func (h *Handler) appPage(w http.ResponseWriter, r *http.Request) {
	sess := sessionFromContext(r.Context())
	tree, err := h.buildTree()
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	items, unreadStart, err := h.listItemsFor(r)
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	views := make([]itemView, 0, len(items))
	for i, it := range items {
		v := toItemView(it)
		if i == unreadStart {
			v.UnreadStart = true
		}
		views = append(views, v)
	}
	settings, err := h.deps.Settings.Get()
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	scope, feedID, feedTitle := h.bulkReadContext(r)
	data := pageData{
		Title:            "feedflow",
		Theme:            settings.Theme,
		CSRFToken:        sess.CSRFToken,
		Username:         sess.Username,
		DefaultView:      settings.DefaultView,
		Tree:             markActiveNodes(tree, r),
		Items:            views,
		AutoReadOnScroll: settings.AutoReadOnScroll,
		BulkRead:         scope,
		CurrentFeedID:    feedID,
		CurrentFeedTitle: feedTitle,
		CurrentLabel:     h.currentSelectionLabel(r),
		ManualPollURL:    manualPollURL(r),
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
	mux.HandleFunc("GET /{$}", h.rootIndex)
	mux.HandleFunc("GET /healthz", h.healthz)
	mux.HandleFunc("GET /favicon.ico", h.favicon)
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
	mux.Handle("GET /app/items/{feedID}/{itemID}/bookmarks", h.requireAuth(http.HandlerFunc(h.bookmarkPicker)))

	// 認証とCSRFが必要な状態変更系ルートです。
	mux.Handle("POST /app/feeds", h.requireAuth(h.requireCSRF(http.HandlerFunc(h.feedSubscribe))))
	mux.Handle("POST /app/feeds/poll", h.requireAuth(h.requireCSRF(http.HandlerFunc(h.feedPoll))))
	mux.Handle("POST /app/feeds/{feedID}/poll", h.requireAuth(h.requireCSRF(http.HandlerFunc(h.feedPoll))))
	mux.Handle("DELETE /app/feeds/{feedID}", h.requireAuth(h.requireCSRF(http.HandlerFunc(h.feedUnsubscribe))))
	mux.Handle("POST /app/items/markall", h.requireAuth(h.requireCSRF(http.HandlerFunc(h.itemMarkAll))))
	mux.Handle("POST /app/items/{feedID}/{itemID}/read", h.requireAuth(h.requireCSRF(http.HandlerFunc(h.itemMarkRead))))
	mux.Handle("POST /app/items/{feedID}/{itemID}/readlater", h.requireAuth(h.requireCSRF(http.HandlerFunc(h.itemReadLater))))
	mux.Handle("POST /app/items/{feedID}/{itemID}/bookmark", h.requireAuth(h.requireCSRF(http.HandlerFunc(h.itemBookmark))))
	mux.Handle("POST /app/items/{feedID}/{itemID}/bookmarks/toggle", h.requireAuth(h.requireCSRF(http.HandlerFunc(h.bookmarkToggle))))
	mux.Handle("POST /app/bookmarks", h.requireAuth(h.requireCSRF(http.HandlerFunc(h.bookmarkCreate))))
	mux.Handle("POST /app/bookmarks/add-url", h.requireAuth(h.requireCSRF(http.HandlerFunc(h.bookmarkAddURL))))
	mux.Handle("POST /app/bookmarks/{id}/rename", h.requireAuth(h.requireCSRF(http.HandlerFunc(h.bookmarkRename))))
	mux.Handle("DELETE /app/bookmarks/{id}", h.requireAuth(h.requireCSRF(http.HandlerFunc(h.bookmarkDelete))))
	mux.Handle("POST /app/settings", h.requireAuth(h.requireCSRF(http.HandlerFunc(h.settingsUpdate))))
	mux.Handle("POST /app/opml/import", h.requireAuth(h.requireCSRF(http.HandlerFunc(h.opmlImport))))

	return h.securityHeaders(mux)
}
