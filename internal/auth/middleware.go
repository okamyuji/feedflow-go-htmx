package auth

import (
	"context"
	"net"
	"net/http"
)

// ctxKey contextに値を載せるための非公開キー型です。
type ctxKey int

// userCtxKey 認証済みユーザー名をcontextに載せるキーです。
const userCtxKey ctxKey = iota

// UsernameFromContext contextから認証済みユーザー名を取り出します。未設定のときは空文字列を返します。
func UsernameFromContext(ctx context.Context) string {
	if v, ok := ctx.Value(userCtxKey).(string); ok {
		return v
	}
	return ""
}

// RequireAuth 有効なセッションを持たないリクエストをloginPathへリダイレクトするミドルウェアを返します。
// 認証済みのときはユーザー名をcontextに載せて後続ハンドラへ渡します。
func RequireAuth(sessions *SessionStore, loginPath string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			username, ok := sessions.Validate(r)
			if !ok {
				http.Redirect(w, r, loginPath, http.StatusSeeOther)
				return
			}
			ctx := context.WithValue(r.Context(), userCtxKey, username)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// isSafeMethod CSRF検証を要しない安全なHTTPメソッドかどうかを返します。
func isSafeMethod(method string) bool {
	switch method {
	case http.MethodGet, http.MethodHead, http.MethodOptions, http.MethodTrace:
		return true
	default:
		return false
	}
}

// RequireCSRF 非安全メソッドのリクエストに対し、セッションに紐づくCSRFトークンの一致を要求するミドルウェアを返します。
// 安全なメソッドは素通しします。検証に失敗したら403を返します。
func RequireCSRF(sessions *SessionStore, csrf *CSRFStore) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if isSafeMethod(r.Method) {
				next.ServeHTTP(w, r)
				return
			}
			c, err := r.Cookie(sessions.name)
			if err != nil {
				http.Error(w, "forbidden", http.StatusForbidden)
				return
			}
			if !csrf.Verify(c.Value, r) {
				http.Error(w, "forbidden", http.StatusForbidden)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// ClientIP リクエストの送信元IPアドレスを返します。ポート付きのRemoteAddrからホスト部だけを取り出します。
// レートリミットのキーに使います。
func ClientIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}
