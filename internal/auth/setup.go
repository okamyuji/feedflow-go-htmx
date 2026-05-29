package auth

import "net/http"

// RequireSetupAvailable 初回セットアップ画面の可否を判定するミドルウェアを返します。
// 所有者が未登録のときだけ後続ハンドラへ通します。登録済みのときはloginPathへリダイレクトします。
// リポジトリ読み込みに失敗したときは500を返します。設計書のセクション9.3に対応します。
func RequireSetupAvailable(m *Manager, loginPath string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			needs, err := m.NeedsSetup()
			if err != nil {
				http.Error(w, "internal server error", http.StatusInternalServerError)
				return
			}
			if !needs {
				http.Redirect(w, r, loginPath, http.StatusSeeOther)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}
