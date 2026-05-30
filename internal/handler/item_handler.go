package handler

import (
	"fmt"
	"html/template"
	"net/http"
	"sort"
	"strings"

	"github.com/okamyuji/feedflow-go-htmx/internal/domain"
	"github.com/okamyuji/feedflow-go-htmx/internal/feed"
)

// readHeadLimit 単一フィード表示時に先頭へ既読として再表示する直近件数の上限です。
// うっかり既読にした記事を再読しやすくするための件数です。
const readHeadLimit = 5

// listItemsFor クエリに応じてミュート適用済みの記事群と、未読の開始位置を返します。
// feedで単一フィード、categoryでカテゴリ所属フィード、bookmarkでブックマーク保存記事に絞ります。
// viewが既定(無指定)とカテゴリの一覧は未読だけを残します。
// view=readは既読だけ、bookmarkはブックマーク済み、readlaterはあとで読む済みを既読を問わず残します。
// 単一フィードの既定表示だけは、直近の既読readHeadLimit件を先頭に既読として並べ、続けて未読を並べます。
// 戻り値unreadStartは既読先頭群の直後(未読の開始位置)の0始まり添字です。区切りが不要なら-1を返します。
func (h *Handler) listItemsFor(r *http.Request) (items []domain.Item, unreadStart int, err error) {
	q := r.URL.Query()
	items, err = h.deps.Items.ListItems(q.Get("feed"))
	if err != nil {
		return nil, -1, err
	}
	unreadStart = -1

	switch q.Get("view") {
	case "read":
		// ブックマーク済みは保管済みとして既読/未読管理の対象外にするため、既読ビューにも出しません。
		items = keepItems(items, func(it domain.Item) bool { return it.Read && !isBookmarked(it) })
	case "bookmark":
		// 保存済み(Bookmarked)の記事を出します。ラベルが無くても保存していれば対象です。
		items = keepItems(items, func(it domain.Item) bool { return it.Bookmarked })
	case "readlater":
		items = keepItems(items, func(it domain.Item) bool { return it.ReadLater })
	default:
		switch {
		case q.Get("bookmark") != "":
			// 後段のブックマーク絞り込みに任せます。
		case q.Get("feed") != "" && q.Get("category") == "":
			// 単一フィードの既定表示は、既読先頭群と未読を並べます。
			// ブックマーク済みは保管済みとして除外します。除外しないと、未読バッジ(unreadByFeedは
			// ブックマーク済みを数えない)と一覧の未読件数がずれてしまいます。
			items = keepItems(items, func(it domain.Item) bool { return !isBookmarked(it) })
			items, unreadStart = withReadHead(items, readHeadLimit)
		default:
			// すべてやカテゴリの一覧は未読のみを残します。
			// ブックマーク済みは保管済みとして未読ストリームから外します。
			items = keepItems(items, func(it domain.Item) bool { return !it.Read && !isBookmarked(it) })
		}
	}

	if bookmarkID := q.Get("bookmark"); bookmarkID != "" {
		items = keepItems(items, func(it domain.Item) bool { return containsString(it.BookmarkIDs, bookmarkID) })
	}

	if categoryID := q.Get("category"); categoryID != "" {
		feedIDs, ferr := h.feedIDsInCategory(categoryID)
		if ferr != nil {
			return nil, -1, ferr
		}
		items = keepItems(items, func(it domain.Item) bool { return containsString(feedIDs, it.FeedID) })
	}

	filtered, ferr := h.deps.Mutes.Filter(items)
	if ferr != nil {
		return nil, -1, ferr
	}
	// ミュート適用で件数が変わるため、既読先頭群の直後(未読の開始位置)はフィルタ後の並びで取り直します。
	if unreadStart != -1 {
		unreadStart = unreadStartIndex(filtered)
	}
	return filtered, unreadStart, nil
}

// unreadStartIndex 既読が先頭に並んだ列で、最初に現れる未読の0始まり添字を返します。
// 先頭に既読が無い、または未読が無い場合は区切りが不要なため-1を返します。
func unreadStartIndex(items []domain.Item) int {
	sawRead := false
	for i, it := range items {
		if it.Read {
			sawRead = true
			continue
		}
		if sawRead {
			return i
		}
	}
	return -1
}

// withReadHead 直近の既読limit件を先頭にまとめ、続けて未読を元の並びのまま並べた列を返します。
// 未読の並びは既存の表示順を維持します。既読の先頭群だけは公開日時の新しい順(直近)で抽出します。
// 戻り値の第2値は未読の開始位置(0始まり添字)です。既読先頭群が無い、または未読が無い場合は-1を返します。
func withReadHead(items []domain.Item, limit int) ([]domain.Item, int) {
	unread := make([]domain.Item, 0, len(items))
	read := make([]domain.Item, 0, len(items))
	for _, it := range items {
		if it.Read {
			read = append(read, it)
		} else {
			unread = append(unread, it)
		}
	}

	// 既読は直近(公開日時の新しい順)を優先して先頭へ載せます。
	sort.SliceStable(read, func(i, j int) bool {
		if read[i].PublishedAt.Equal(read[j].PublishedAt) {
			return read[i].FetchedAt.After(read[j].FetchedAt)
		}
		return read[i].PublishedAt.After(read[j].PublishedAt)
	})
	if len(read) > limit {
		read = read[:limit]
	}

	out := make([]domain.Item, 0, len(read)+len(unread))
	out = append(out, read...)
	out = append(out, unread...)
	unreadStart := -1
	if len(read) > 0 && len(unread) > 0 {
		unreadStart = len(read)
	}
	return out, unreadStart
}

// isBookmarked 記事がいずれかのブックマークに所属しているかどうかを返します。
// ブックマーク済みの記事は保管済みとみなし、未読ストリームや既読ビューや未読カウントの対象から外します。
func isBookmarked(it domain.Item) bool {
	return it.Bookmarked
}

// isBookmarkView リクエストがブックマーク(保存)記事のビューかどうかを返します。
// view=bookmark(全保存記事)とbookmark={id}(ラベル別)の両方を対象とします。
func isBookmarkView(r *http.Request) bool {
	q := r.URL.Query()
	return q.Get("view") == "bookmark" || q.Get("bookmark") != ""
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
	items, unreadStart, err := h.listItemsFor(r)
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	inBookmarkView := isBookmarkView(r)
	views := make([]itemView, 0, len(items))
	for i, it := range items {
		v := toItemView(it)
		v.InBookmarkView = inBookmarkView
		if i == unreadStart {
			v.UnreadStart = true
		}
		views = append(views, v)
	}
	scope, feedID, feedTitle := h.bulkReadContext(r)
	data := pageData{
		CSRFToken:        sess.CSRFToken,
		DefaultView:      domain.ViewCard,
		Items:            views,
		BulkRead:         scope,
		CurrentFeedID:    feedID,
		CurrentFeedTitle: feedTitle,
		CurrentLabel:     h.currentSelectionLabel(r),
	}
	if isHTMX(r) {
		h.renderWithTreeOOB(w, r, http.StatusOK, "_item_list.html", data)
		return
	}
	h.renderShellPage(w, r, sess, "feedflow", data)
}

// bulkReadContext 一括既読コントロールの表示範囲をリクエストのクエリから決めます。
// 特定フィード選択時はそのフィードだけ、すべて(未読ストリーム)表示時は全フィード、
// 既読やブックマークやあとで読むやカテゴリやブックマーク絞り込みのビューでは一括既読の対象が定まらないため非表示にします。
func (h *Handler) bulkReadContext(r *http.Request) (scope, feedID, feedTitle string) {
	q := r.URL.Query()
	if feed := q.Get("feed"); feed != "" {
		title := feed
		if feeds, err := h.deps.Subscriptions.ListFeeds(); err == nil {
			for _, f := range feeds {
				if f.ID == feed {
					title = f.Title
					break
				}
			}
		}
		return "feed", feed, title
	}
	if q.Get("category") != "" || q.Get("bookmark") != "" {
		return "none", "", ""
	}
	switch q.Get("view") {
	case "read", "bookmark", "readlater":
		return "none", "", ""
	default:
		return "all", "", ""
	}
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
	markedRead := false
	if !it.Read {
		if err := h.deps.Items.MarkRead(feedID, itemID, true); err != nil {
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		markedRead = true
	}
	view := toItemView(it)
	view.Content = cleanArticleHTML(it.Content)
	if markedRead {
		h.renderWithTreeOOB(w, r, http.StatusOK, "_item_overlay.html", view)
		return
	}
	h.renderPartial(w, http.StatusOK, "_item_overlay.html", view)
}

// renderCard 操作後の単一記事カードを再描画します。
// 既読状態が変わるため、ツリーの未読数をout-of-bandスワップで同時に更新します。
func (h *Handler) renderCard(w http.ResponseWriter, r *http.Request, feedID, itemID string) {
	it, ok, err := h.findItem(feedID, itemID)
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	if !ok {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	v := toItemView(it)
	v.InBookmarkView = isBookmarkView(r)
	h.renderWithTreeOOB(w, r, http.StatusOK, "_item_card.html", v)
}

// itemMarkRead 既読状態を設定し、記事カードとツリーの未読数を再描画します。
func (h *Handler) itemMarkRead(w http.ResponseWriter, r *http.Request) {
	feedID := r.PathValue("feedID")
	itemID := r.PathValue("itemID")
	read := r.FormValue("read") == "true"
	if err := h.deps.Items.MarkRead(feedID, itemID, read); err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	h.renderCard(w, r, feedID, itemID)
}

// itemBookmark 記事の保存(ブックマーク)状態を設定します。
// 保存オンのときはカードを再描画して「保存済み」表示にします。
// 保存オフ(解除)のときは、ブックマークビューから当該記事が消え未読数も整合させるため、一覧全体を再描画します。
func (h *Handler) itemBookmark(w http.ResponseWriter, r *http.Request) {
	feedID := r.PathValue("feedID")
	itemID := r.PathValue("itemID")
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	bookmarked := r.FormValue("bookmarked") == "true"
	if err := h.deps.Items.SetBookmarked(feedID, itemID, bookmarked); err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	// ピッカー(記事カード内のパネル)からの操作はピッカーを再描画して開いたまま状態を更新します。
	// ピッカーはOOBでカードの保存済み表示も同期します。
	if r.FormValue("surface") == "picker" {
		h.renderBookmarkPicker(w, r, feedID, itemID)
		return
	}
	// カードの操作ボタンからの解除は、ブックマークビューから当該記事を消し未読数も整合させるため一覧全体を再描画します。
	if !bookmarked {
		h.itemList(w, r)
		return
	}
	h.renderCard(w, r, feedID, itemID)
}

// itemReadLater あとで読む状態を設定し、オーバーレイのアクション群を再描画します。
// あとで読むボタンはオーバーレイにのみ存在するため、状態を反映したボタン群を返します。
func (h *Handler) itemReadLater(w http.ResponseWriter, r *http.Request) {
	feedID := r.PathValue("feedID")
	itemID := r.PathValue("itemID")
	readLater := r.FormValue("read_later") == "true"
	if err := h.deps.Items.ReadLater(feedID, itemID, readLater); err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	h.renderOverlayActions(w, feedID, itemID)
}

// renderOverlayActions 最新状態の記事を読み直し、オーバーレイのアクション群を再描画します。
func (h *Handler) renderOverlayActions(w http.ResponseWriter, feedID, itemID string) {
	it, ok, err := h.findItem(feedID, itemID)
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	if !ok {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	h.renderPartial(w, http.StatusOK, "_overlay_actions.html", toItemView(it))
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
