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

// favicon ブラウザのfavicon要求に空応答を返し、ログインや初回セットアップ画面での404を防ぎます。
func (h *Handler) favicon(w http.ResponseWriter, _ *http.Request) {
	w.WriteHeader(http.StatusNoContent)
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
