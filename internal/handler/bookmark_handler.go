package handler

import (
	"errors"
	"net/http"

	"github.com/okamyuji/feedflow-go-htmx/internal/service"
)

// bookmarkPickerView ブックマーク保存ピッカーの描画モデルです。
type bookmarkPickerView struct {
	FeedID     string           // 対象記事の所属フィードIDです
	ItemID     string           // 対象記事のIDです
	CSRFToken  string           // フォーム送信に使うCSRFトークンです
	Options    []bookmarkOption // 既存ブックマークの選択肢です
	AnyChecked bool             // いずれかのブックマークに所属しているかどうかです。カードの保存済み表示の同期に使います
}

// bookmarkOption ピッカー1行ぶんの選択肢です。
type bookmarkOption struct {
	ID      string // ブックマークIDです
	Name    string // ブックマーク名です
	Checked bool   // 対象記事が所属しているかどうかです
}

// bookmarkPicker 記事のブックマーク保存ピッカーを描画します。
func (h *Handler) bookmarkPicker(w http.ResponseWriter, r *http.Request) {
	feedID := r.PathValue("feedID")
	itemID := r.PathValue("itemID")
	h.renderBookmarkPicker(w, r, feedID, itemID)
}

// bookmarkToggle 記事のブックマーク所属を切り替え、ピッカーを再描画します。
func (h *Handler) bookmarkToggle(w http.ResponseWriter, r *http.Request) {
	feedID := r.PathValue("feedID")
	itemID := r.PathValue("itemID")
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	bookmarkID := r.FormValue("bookmark")
	if bookmarkID == "" {
		http.Error(w, "bookmark is required", http.StatusBadRequest)
		return
	}
	if err := h.deps.Bookmarks.Toggle(feedID, itemID, bookmarkID); err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	h.renderBookmarkPicker(w, r, feedID, itemID)
}

// bookmarkCreate 指定名のブックマークを作成して当該記事を所属させ、ピッカーを再描画します。
// 空名は入力誤りとして400を返します。
func (h *Handler) bookmarkCreate(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	feedID := r.FormValue("feed")
	itemID := r.FormValue("item")
	name := r.FormValue("name")
	if _, err := h.deps.Bookmarks.CreateAndAdd(feedID, itemID, name); err != nil {
		if errors.Is(err, service.ErrBookmarkNameRequired) {
			http.Error(w, "name is required", http.StatusBadRequest)
			return
		}
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	h.renderBookmarkPicker(w, r, feedID, itemID)
}

// renderBookmarkPicker 最新状態のブックマークと記事の所属を読み直し、ピッカー部分テンプレートを描画します。
// 元記事が消えている場合でもエラーにはせず、所属なしのピッカーを返します。
func (h *Handler) renderBookmarkPicker(w http.ResponseWriter, r *http.Request, feedID, itemID string) {
	sess := sessionFromContext(r.Context())
	bms, err := h.deps.Bookmarks.List()
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	current := make(map[string]bool)
	if it, ok, ferr := h.findItem(feedID, itemID); ferr == nil && ok {
		for _, id := range it.BookmarkIDs {
			current[id] = true
		}
	}
	options := make([]bookmarkOption, 0, len(bms))
	anyChecked := false
	for _, b := range bms {
		checked := current[b.ID]
		if checked {
			anyChecked = true
		}
		options = append(options, bookmarkOption{ID: b.ID, Name: b.Name, Checked: checked})
	}
	view := bookmarkPickerView{
		FeedID:     feedID,
		ItemID:     itemID,
		CSRFToken:  sess.CSRFToken,
		Options:    options,
		AnyChecked: anyChecked,
	}
	h.renderPartial(w, http.StatusOK, "_bookmark_picker.html", view)
}
