package handler

import (
	"context"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/okamyuji/feedflow-go-htmx/internal/domain"
)

// maxOPMLBytes OPMLインポートの最大バイト数です。過大なアップロードを防ぎます。
const maxOPMLBytes = 8 << 20

// opmlImportTimeout バックグラウンドで実行するOPMLインポート全体の制限時間です。大量購読の取得に余裕を持たせます。
const opmlImportTimeout = 30 * time.Minute

// settingsPage 設定画面の部分テンプレートを描画します。
func (h *Handler) settingsPage(w http.ResponseWriter, r *http.Request) {
	sess := sessionFromContext(r.Context())
	settings, err := h.deps.Settings.Get()
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	data := pageData{CSRFToken: sess.CSRFToken, Settings: settings}
	if isHTMX(r) {
		h.renderPartial(w, http.StatusOK, "_settings.html", data)
		return
	}
	data.MainView = "settings"
	h.renderShellPage(w, r, sess, "feedflow 設定", data)
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
		AutoReadOnScroll:  r.FormValue("auto_read_on_scroll") != "",
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
	w.Header().Set("Content-Type", "text/xml; charset=utf-8")
	w.Header().Set("Content-Disposition", `attachment; filename="feedflow.opml"`)
	w.WriteHeader(http.StatusOK)
	if _, werr := w.Write(data); werr != nil {
		slog.Error("failed to write opml export response", "error", werr)
	}
}

// opmlImport アップロードされたOPMLを読み込み、購読を追加した件数を返します。
func (h *Handler) opmlImport(w http.ResponseWriter, r *http.Request) {
	sess := sessionFromContext(r.Context())
	// MaxBytesReaderでリクエストボディの総量を上限で囲み、無制限なフォーム解析によるメモリ枯渇を防ぎます。
	r.Body = http.MaxBytesReader(w, r.Body, maxOPMLBytes)
	if err := r.ParseMultipartForm(maxOPMLBytes); err != nil { //nolint:gosec // 直前のMaxBytesReaderでボディ総量を上限に制限済みです
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	file, _, err := r.FormFile("opml")
	if err != nil {
		http.Error(w, "opml file is required", http.StatusBadRequest)
		return
	}
	defer func() {
		if cerr := file.Close(); cerr != nil {
			slog.Error("failed to close opml upload file", "error", cerr)
		}
	}()
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
	// インポートはフィードごとにネットワーク取得を伴うため、件数が多いと1リクエストの処理が
	// Cloudflareプロキシのタイムアウトを超え504になります。リクエスト由来のコンテキストは応答後に
	// 打ち切られるため、独立したコンテキストでバックグラウンド実行し、リクエストは即座に返します。
	// 取得結果は購読ツリーへ順次反映され、画面の再読み込みで確認できます。重複URLはスキップされます。
	go func(payload []byte) { //nolint:gosec,contextcheck // 取得はリクエスト応答後も継続させるため、意図的にリクエスト由来でない独立したコンテキストを使います
		ctx, cancel := context.WithTimeout(context.Background(), opmlImportTimeout)
		defer cancel()
		count, ierr := h.deps.OPML.Import(ctx, payload)
		if ierr != nil {
			slog.Error("opml import failed", "error", ierr)
			return
		}
		slog.Info("opml import finished", "imported", count)
	}(data)

	page := pageData{CSRFToken: sess.CSRFToken, Flash: "OPMLのインポートを開始しました。フィードの取得はバックグラウンドで進みます。しばらくして画面を再読み込みしてください。"}
	settings, serr := h.deps.Settings.Get()
	if serr == nil {
		page.Settings = settings
	}
	h.renderPartial(w, http.StatusOK, "_settings.html", page)
}
