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
