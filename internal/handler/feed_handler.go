package handler

import (
	"context"
	"log/slog"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/okamyuji/feedflow-go-htmx/internal/domain"
)

// manualPollTimeout HTTPのWriteTimeoutより短く打ち切り、手動更新がゲートウェイタイムアウトを誘発しないようにします。
const manualPollTimeout = 20 * time.Second

// buildTree 左ペインの購読ツリーを組み立てます。固定の集約ノードに続けてフィードを並べます。
// すべてノードは未読を読む主ストリームのため未読合計を持たせます。
// 既読ノードは既読記事をまとめて見るための入口で、件数バッジは持たせません。
// ブックマークノードは全件への入口で、名称コレクションを子ノードとして開閉表示します。
// フィードノードは設定の並び替えキーと方向に従って並べます。
func (h *Handler) buildTree() ([]feedTreeNode, error) {
	feeds, err := h.deps.Subscriptions.ListFeeds()
	if err != nil {
		return nil, err
	}
	allItems, err := h.deps.Items.ListItems("")
	if err != nil {
		return nil, err
	}

	unreadTotal := 0
	unreadByFeed := make(map[string]int)
	for _, it := range allItems {
		// ブックマーク済みは保管済みとして未読カウントから外します。すべての未読数とフィード別バッジを一致させます。
		if !it.Read && !isBookmarked(it) {
			unreadTotal++
			unreadByFeed[it.FeedID]++
		}
	}

	bookmarkNode, err := h.buildBookmarkNode()
	if err != nil {
		return nil, err
	}

	feedNodes := orderFeedNodes(feeds, unreadByFeed, h.feedSortSettings())
	nodes := make([]feedTreeNode, 0, 4+len(feedNodes))
	nodes = append(nodes,
		feedTreeNode{Kind: "all", Label: "すべて", UnreadCount: unreadTotal},
		feedTreeNode{Kind: "read", Label: "既読"},
		bookmarkNode,
		feedTreeNode{Kind: "readlater", Label: "あとで読む"},
	)
	nodes = append(nodes, feedNodes...)
	return nodes, nil
}

// feedSortSettings 左ペインのフィード並び替え設定を返します。
// テストなどでSettings依存が未注入のハンドラは既定値を使います。
func (h *Handler) feedSortSettings() domain.Settings {
	settings := domain.DefaultSettings()
	if h.deps.Settings == nil {
		return settings
	}
	loaded, err := h.deps.Settings.Get()
	if err != nil {
		slog.Error("failed to load settings, falling back to defaults", "error", err)
		return settings
	}
	if !loaded.Valid() {
		slog.Warn("loaded settings are invalid, falling back to defaults")
		return settings
	}
	return loaded
}

// orderFeedNodes フィードを設定のキーと方向で並べたノード列を返します。
// registeredはリポジトリが返す購読順を基準にし、descなら後から登録したものを先に出します。
func orderFeedNodes(feeds []domain.Feed, unreadByFeed map[string]int, settings domain.Settings) []feedTreeNode {
	ordered := append([]domain.Feed(nil), feeds...)
	if settings.FeedSortKey == domain.FeedSortTitle {
		sort.SliceStable(ordered, func(i, j int) bool {
			left := strings.ToLower(ordered[i].Title)
			right := strings.ToLower(ordered[j].Title)
			if left == right {
				return ordered[i].Title < ordered[j].Title
			}
			return left < right
		})
	}
	if settings.FeedSortDirection == domain.SortDesc {
		for i, j := 0, len(ordered)-1; i < j; i, j = i+1, j-1 {
			ordered[i], ordered[j] = ordered[j], ordered[i]
		}
	}

	out := make([]feedTreeNode, 0, len(ordered))
	for _, f := range ordered {
		out = append(out, feedNode(f, unreadByFeed[f.ID]))
	}
	return out
}

// feedNode フィードから表示ノードを作ります。
func feedNode(f domain.Feed, unread int) feedTreeNode {
	return feedTreeNode{
		Kind:        "feed",
		ID:          f.ID,
		Label:       f.Title,
		UnreadCount: unread,
		HasError:    f.HasError(),
	}
}

// buildBookmarkNode ブックマークの親ノードと名称コレクションの子ノードを組み立てます。
// ブックマークは意図的に保存したものなので、子ノードに所属件数バッジは出しません(見やすさ優先)。
func (h *Handler) buildBookmarkNode() (feedTreeNode, error) {
	bookmarks, err := h.deps.Bookmarks.List()
	if err != nil {
		return feedTreeNode{}, err
	}
	children := make([]feedTreeNode, 0, len(bookmarks))
	for _, b := range bookmarks {
		children = append(children, feedTreeNode{
			Kind:  "bookmarkItem",
			ID:    b.ID,
			Label: b.Name,
		})
	}
	return feedTreeNode{Kind: "bookmark", Label: "ブックマーク", Children: children}, nil
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
// feedとcategoryとbookmarkはIDで、それ以外はviewの種別で一致を判定します。view未指定はすべて(all)を選択中とみなします。
// ブックマーク子ノード(bookmarkItem)はbookmarkクエリのIDで一致を判定します。
func markActiveNodes(nodes []feedTreeNode, r *http.Request) []feedTreeNode {
	q := r.URL.Query()
	var kind, id string
	switch {
	case q.Get("feed") != "":
		kind, id = "feed", q.Get("feed")
	case q.Get("category") != "":
		kind, id = "category", q.Get("category")
	case q.Get("bookmark") != "":
		kind, id = "bookmarkItem", q.Get("bookmark")
	default:
		kind = q.Get("view")
		if kind == "" {
			kind = "all"
		}
	}
	out := make([]feedTreeNode, len(nodes))
	for i, n := range nodes {
		n.Active = n.Kind == kind && n.ID == id
		if len(n.Children) > 0 {
			children := make([]feedTreeNode, len(n.Children))
			for j, c := range n.Children {
				c.Active = c.Kind == kind && c.ID == id
				children[j] = c
			}
			n.Children = children
		}
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
	if bookmarkID := q.Get("bookmark"); bookmarkID != "" {
		return h.currentBookmarkLabel(bookmarkID)
	}
	switch q.Get("view") {
	case "read":
		return "既読"
	case "bookmark":
		return "ブックマーク"
	case "readlater":
		return "あとで読む"
	default:
		return "すべて"
	}
}

// currentBookmarkLabel bookmarkクエリで選択中の名称を返します。見つからない場合は「ブックマーク」を返します。
func (h *Handler) currentBookmarkLabel(bookmarkID string) string {
	if bms, err := h.deps.Bookmarks.List(); err == nil {
		for _, b := range bms {
			if b.ID == bookmarkID {
				return b.Name
			}
		}
	}
	return "ブックマーク"
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
	h.itemList(w, r)
}

// feedPoll 指定フィードを手動で即時取得し、記事一覧を部分更新で返します。
func (h *Handler) feedPoll(w http.ResponseWriter, r *http.Request) {
	feedID := r.PathValue("feedID")
	if feedID == "" {
		feedID = r.URL.Query().Get("feed")
	}
	pollCtx, cancel := context.WithTimeout(r.Context(), manualPollTimeout)
	defer cancel()
	if feedID == "" {
		if _, err := h.deps.Poll.PollAllNow(pollCtx); err != nil {
			slog.Warn("manual poll all failed", "error", err)
		}
	} else {
		if _, err := h.deps.Poll.PollFeed(pollCtx, feedID); err != nil {
			slog.Warn("manual poll feed failed", "feed_id", feedID, "error", err)
		}
	}
	h.itemList(w, r)
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
		Bookmarked:  it.Bookmarked,
		ReadLater:   it.ReadLater,
		HasContent:  strings.TrimSpace(it.Content) != "",
	}
}
