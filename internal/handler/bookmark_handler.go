package handler

import (
	"bytes"
	"errors"
	"fmt"
	"html/template"
	"log/slog"
	"net/http"

	"github.com/okamyuji/feedflow-go-htmx/internal/service"
)

// bookmarkPickerView ブックマーク保存ピッカーの描画モデルです。
type bookmarkPickerView struct {
	FeedID     string           // 対象記事の所属フィードIDです
	ItemID     string           // 対象記事のIDです
	CSRFToken  string           // フォーム送信に使うCSRFトークンです
	Options    []bookmarkOption // 既存ラベルの選択肢です
	Bookmarked bool             // 記事が保存(ブックマーク)済みかどうかです。解除ボタンとカードの保存済み表示の同期に使います
}

// bookmarkOption ピッカー1行ぶんの選択肢です。
type bookmarkOption struct {
	ID      string // ラベルIDです
	Name    string // ラベル名です
	Checked bool   // 対象記事がこのラベルに所属しているかどうかです
}

// bookmarkPicker 記事のブックマーク保存ピッカーを描画します。
func (h *Handler) bookmarkPicker(w http.ResponseWriter, r *http.Request) {
	feedID := r.PathValue("feedID")
	itemID := r.PathValue("itemID")
	h.renderBookmarkPicker(w, r, feedID, itemID, false, false)
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
	h.renderBookmarkPicker(w, r, feedID, itemID, false, true)
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
	h.renderBookmarkPicker(w, r, feedID, itemID, false, true)
}

// renderBookmarkPicker 最新状態のブックマークと記事の所属を読み直し、ピッカー部分テンプレートを描画します。
// 元記事が消えている場合でもエラーにはせず、所属なしのピッカーを返します。
// removeFromListがtrueのときは、ピッカーに続けて当該記事カードを一覧から取り除くOOB断片を付けます。
// ブックマークビューでの解除時に、解除した記事を一覧から消すために使います。
// refreshTreeがtrueのときは、ラベル作成や保存状態変更を左ツリーへ即時反映するためツリーOOB断片も付けます。
func (h *Handler) renderBookmarkPicker(w http.ResponseWriter, r *http.Request, feedID, itemID string, removeFromList, refreshTree bool) {
	sess := sessionFromContext(r.Context())
	bms, err := h.deps.Bookmarks.List()
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	current := make(map[string]bool)
	bookmarked := false
	if it, ok, ferr := h.findItem(feedID, itemID); ferr == nil && ok {
		bookmarked = it.Bookmarked
		for _, id := range it.BookmarkIDs {
			current[id] = true
		}
	}
	options := make([]bookmarkOption, 0, len(bms))
	for _, b := range bms {
		options = append(options, bookmarkOption{ID: b.ID, Name: b.Name, Checked: current[b.ID]})
	}
	view := bookmarkPickerView{
		FeedID:     feedID,
		ItemID:     itemID,
		CSRFToken:  sess.CSRFToken,
		Options:    options,
		Bookmarked: bookmarked,
	}
	var buf bytes.Buffer
	if err := h.templates.ExecuteTemplate(&buf, "_bookmark_picker.html", view); err != nil {
		slog.Error("failed to execute template", "template", "_bookmark_picker.html", "error", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	if removeFromList {
		// hx-swap-oob="delete"で当該記事カードを一覧から取り除きます。idはhtml/templateの属性エスケープに委ねます。
		oob := fmt.Sprintf(`<li id="item-%s" hx-swap-oob="delete"></li>`, template.HTMLEscapeString(itemID))
		buf.WriteString(oob)
	}
	if refreshTree {
		tree, err := h.treeData(r)
		if err != nil {
			slog.Error("failed to build tree for bookmark picker oob swap", "error", err)
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		tree.TreeOOB = true
		if err := h.templates.ExecuteTemplate(&buf, "_tree_pane.html", tree); err != nil {
			slog.Error("failed to execute template", "template", "_tree_pane.html", "error", err)
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	if _, err := buf.WriteTo(w); err != nil {
		slog.Error("failed to write bookmark picker", "error", err)
	}
}

// bookmarkRename 指定IDのラベル名を変更し、ツリーペインを再描画します。
// 空名と重複名は入力誤りとして400を、対象不在は404を返します。
func (h *Handler) bookmarkRename(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	name := r.FormValue("name")
	if err := h.deps.Bookmarks.Rename(id, name); err != nil {
		switch {
		case errors.Is(err, service.ErrBookmarkNameRequired), errors.Is(err, service.ErrBookmarkNameTaken):
			http.Error(w, "invalid name", http.StatusBadRequest)
		case errors.Is(err, service.ErrBookmarkNotFound):
			http.Error(w, "not found", http.StatusNotFound)
		default:
			http.Error(w, "internal error", http.StatusInternalServerError)
		}
		return
	}
	h.renderTreePane(w, r)
}

// bookmarkDelete 指定IDのラベルを削除し、ツリーペインを再描画します。
// 保存した記事自体は残るため、削除してもブックマークビューから記事は消えません。
func (h *Handler) bookmarkDelete(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if err := h.deps.Bookmarks.Delete(id); err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	h.renderTreePane(w, r)
}

// renderTreePane 左ペインのツリーを最新状態で描画します。ラベルのリネーム/削除後の反映に使います。
func (h *Handler) renderTreePane(w http.ResponseWriter, r *http.Request) {
	data, err := h.treeData(r)
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	h.renderPartial(w, http.StatusOK, "_tree_pane.html", data)
}
