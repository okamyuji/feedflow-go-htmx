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
		Summary:     truncateRunes(stripHTML(it.Summary), 160),
		Author:      it.Author,
		PublishedAt: formatJST(it.PublishedAt),
		Read:        it.Read,
		Starred:     it.Starred,
		ReadLater:   it.ReadLater,
	}
}
