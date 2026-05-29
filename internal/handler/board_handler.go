package handler

import "net/http"

// itemSetBoards 記事の保存先ボードを更新します。送信されたボードID群で置き換えます。
func (h *Handler) itemSetBoards(w http.ResponseWriter, r *http.Request) {
	feedID := r.PathValue("feedID")
	itemID := r.PathValue("itemID")
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	boardIDs := r.Form["board_ids"]
	if err := h.deps.Items.SetBoards(feedID, itemID, boardIDs); err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
