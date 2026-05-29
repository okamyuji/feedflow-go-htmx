package handler

import (
	"net/http"

	"github.com/okamyuji/feedflow-go-htmx/internal/domain"
)

// buildTree 左ペインの購読ツリーを組み立てます。固定の集約ノードに続けてフィードを並べます。
// すべてノードは未読を読む主ストリームのため未読合計を持たせます。
// 既読ノードは既読記事をまとめて見るための入口で、件数バッジは持たせません。
func (h *Handler) buildTree() ([]feedTreeNode, error) {
	feeds, err := h.deps.Subscriptions.ListFeeds()
	if err != nil {
		return nil, err
	}
	_, unreadTotal, err := h.itemCounts("")
	if err != nil {
		return nil, err
	}
	nodes := []feedTreeNode{
		{Kind: "all", Label: "すべて", UnreadCount: unreadTotal},
		{Kind: "read", Label: "既読"},
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

// itemCounts 指定フィードの総件数と未読件数を返します。引数feedIDが空のときは全フィードを対象にします。
func (h *Handler) itemCounts(feedID string) (total, unread int, err error) {
	items, err := h.deps.Items.ListItems(feedID)
	if err != nil {
		return 0, 0, err
	}
	for _, it := range items {
		if !it.Read {
			unread++
		}
	}
	return len(items), unread, nil
}

// unreadCount 指定フィードの未読件数を数えます。引数feedIDが空のときは全フィードを対象にします。
func (h *Handler) unreadCount(feedID string) (int, error) {
	_, unread, err := h.itemCounts(feedID)
	if err != nil {
		return 0, err
	}
	return unread, nil
}

// treeData ツリー部分テンプレートに渡す描画モデルを組み立てます。
func (h *Handler) treeData(r *http.Request) (pageData, error) {
	sess := sessionFromContext(r.Context())
	tree, err := h.buildTree()
	if err != nil {
		return pageData{}, err
	}
	tree = markActiveNodes(tree, r)
	return pageData{CSRFToken: sess.CSRFToken, Username: sess.Username, Tree: tree}, nil
}

// markActiveNodes リクエストのクエリに対応するノードをActiveにした新しいスライスを返します。
// feedとcategoryとboardはIDで、それ以外はviewの種別で一致を判定します。view未指定はすべて(all)を選択中とみなします。
func markActiveNodes(nodes []feedTreeNode, r *http.Request) []feedTreeNode {
	q := r.URL.Query()
	var kind, id string
	switch {
	case q.Get("feed") != "":
		kind, id = "feed", q.Get("feed")
	case q.Get("category") != "":
		kind, id = "category", q.Get("category")
	case q.Get("board") != "":
		kind, id = "board", q.Get("board")
	default:
		kind = q.Get("view")
		if kind == "" {
			kind = "all"
		}
	}
	out := make([]feedTreeNode, len(nodes))
	for i, n := range nodes {
		n.Active = n.Kind == kind && n.ID == id
		out[i] = n
	}
	return out
}

// currentSelectionLabel 右ペイン左上に出す、選択中の項目名を返します。
// フィード選択時はフィード名、各ビューは固定の名称を返します。
func (h *Handler) currentSelectionLabel(r *http.Request) string {
	q := r.URL.Query()
	if feed := q.Get("feed"); feed != "" {
		if feeds, err := h.deps.Subscriptions.ListFeeds(); err == nil {
			for _, f := range feeds {
				if f.ID == feed {
					return f.Title
				}
			}
		}
		return "フィード"
	}
	switch q.Get("view") {
	case "read":
		return "既読"
	case "starred":
		return "スター"
	case "readlater":
		return "あとで読む"
	default:
		return "すべて"
	}
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
	h.renderPartial(w, http.StatusOK, "_tree_pane.html", data)
}

// feedUnsubscribe 指定フィードの購読を解除し、属する記事も削除したうえでツリーペイン全体を部分更新で返します。
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
	h.renderPartial(w, http.StatusOK, "_tree_pane.html", data)
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
