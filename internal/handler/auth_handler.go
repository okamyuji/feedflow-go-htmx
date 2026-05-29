package handler

import (
	"net/http"
)

// loginPage ログイン画面を表示します。初回セットアップが必要なら/setupへ誘導します。
func (h *Handler) loginPage(w http.ResponseWriter, r *http.Request) {
	needs, err := h.deps.Setup.NeedsSetup()
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	if needs {
		http.Redirect(w, r, "/setup", http.StatusSeeOther)
		return
	}
	h.writeTemplate(w, http.StatusOK, "login.html", pageData{Title: "ログイン feedflow"})
}

// loginSubmit ログインフォームを検証し、成功でセッションを発行してアプリ画面へ遷移します。
func (h *Handler) loginSubmit(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	username := r.FormValue("username")
	password := r.FormValue("password")
	ok, err := h.deps.Setup.Authenticate(username, password)
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	if !ok {
		h.writeTemplate(w, http.StatusUnauthorized, "login.html",
			pageData{Title: "ログイン feedflow", Flash: "ユーザー名またはパスワードが違います"})
		return
	}
	if err := h.deps.Sessions.Issue(w, username); err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	http.Redirect(w, r, "/app", http.StatusSeeOther)
}

// logout セッションを破棄してログイン画面へ戻します。CSRFトークンもセッションIDをキーに破棄します。
func (h *Handler) logout(w http.ResponseWriter, r *http.Request) {
	if id, ok := h.sessionID(r); ok {
		h.deps.CSRF.Discard(id)
	}
	h.deps.Sessions.Destroy(w, r)
	http.Redirect(w, r, "/login", http.StatusSeeOther)
}

// setupPage 初回セットアップ画面を表示します。登録済みなら無効化してログインへ戻します。
func (h *Handler) setupPage(w http.ResponseWriter, r *http.Request) {
	needs, err := h.deps.Setup.NeedsSetup()
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	if !needs {
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}
	h.writeTemplate(w, http.StatusOK, "setup.html", pageData{Title: "初回セットアップ feedflow"})
}

// setupSubmit 初回セットアップを登録します。登録済みの状態では拒否します。
func (h *Handler) setupSubmit(w http.ResponseWriter, r *http.Request) {
	needs, err := h.deps.Setup.NeedsSetup()
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	if !needs {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	username := r.FormValue("username")
	password := r.FormValue("password")
	if username == "" || len(password) < 8 {
		h.writeTemplate(w, http.StatusBadRequest, "setup.html",
			pageData{Title: "初回セットアップ feedflow", Flash: "ユーザー名と8文字以上のパスワードを入力してください"})
		return
	}
	if err := h.deps.Setup.Setup(username, password); err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	http.Redirect(w, r, "/login", http.StatusSeeOther)
}
