package handler

import (
	"fmt"
	"html/template"
	"net/http"
	"strings"

	"github.com/okamyuji/feedflow-go-htmx/internal/domain"
	"github.com/okamyuji/feedflow-go-htmx/internal/feed"
)

// listItemsFor クエリに応じてミュート適用済みの記事群を取得します。
// feedで単一フィード、categoryでカテゴリ所属フィード、boardでボード保存記事に絞り、
// view(unread、starred、readlater)で状態を絞ります。いずれも無指定なら全件です。
func (h *Handler) listItemsFor(r *http.Request) ([]domain.Item, error) {
	q := r.URL.Query()
	items, err := h.deps.Items.ListItems(q.Get("feed"))
	if err != nil {
		return nil, err
	}

	switch q.Get("view") {
	case "unread":
		items = keepItems(items, func(it domain.Item) bool { return !it.Read })
	case "starred":
		items = keepItems(items, func(it domain.Item) bool { return it.Starred })
	case "readlater":
		items = keepItems(items, func(it domain.Item) bool { return it.ReadLater })
	}

	if boardID := q.Get("board"); boardID != "" {
		items = keepItems(items, func(it domain.Item) bool { return containsString(it.BoardIDs, boardID) })
	}

	if categoryID := q.Get("category"); categoryID != "" {
		feedIDs, ferr := h.feedIDsInCategory(categoryID)
		if ferr != nil {
			return nil, ferr
		}
		items = keepItems(items, func(it domain.Item) bool { return containsString(feedIDs, it.FeedID) })
	}

	filtered, err := h.deps.Mutes.Filter(items)
	if err != nil {
		return nil, err
	}
	return filtered, nil
}

// keepItems 述語を満たす記事だけを順序を保って残します。
func keepItems(items []domain.Item, keep func(domain.Item) bool) []domain.Item {
	out := make([]domain.Item, 0, len(items))
	for _, it := range items {
		if keep(it) {
			out = append(out, it)
		}
	}
	return out
}

// containsString 文字列スライスに対象値が含まれるかどうかを返します。
func containsString(values []string, target string) bool {
	for _, v := range values {
		if v == target {
			return true
		}
	}
	return false
}

// feedIDsInCategory 指定カテゴリに所属するフィードのID群を返します。
func (h *Handler) feedIDsInCategory(categoryID string) ([]string, error) {
	feeds, err := h.deps.Subscriptions.ListFeeds()
	if err != nil {
		return nil, fmt.Errorf("failed to load feeds for category filter: %w", err)
	}
	ids := make([]string, 0, len(feeds))
	for _, f := range feeds {
		if containsString(f.CategoryIDs, categoryID) {
			ids = append(ids, f.ID)
		}
	}
	return ids, nil
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
