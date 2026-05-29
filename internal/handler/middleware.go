package handler

import (
	"context"
	"net"
	"net/http"
	"strings"
)

// ctxKey コンテキストに値を格納するためのキー型です。
type ctxKey int

const sessionCtxKey ctxKey = iota

// hstsValue 1年間のHSTSとサブドメインとプリロードを指示します。internal/authのhstsValueと文言を一致させます。
const hstsValue = "max-age=31536000; includeSubDomains; preload"

// contentSecurityPolicy feedflowのCSPです。internal/authのcontentSecurityPolicyと文言を一致させ二重定義の不整合を避けます。
// HTMXとAlpine.jsをベンダーしてselfから配信するためscript-srcはselfに限定します。
// Alpine.jsのインライン属性を許すためstyle-srcにunsafe-inlineを含めます。
// form-action 'self'でフォーム送信先を自オリジンに限定し、注入時の外部送信を防ぎます。
const contentSecurityPolicy = "default-src 'self'; " +
	"script-src 'self'; " +
	"style-src 'self' 'unsafe-inline'; " +
	"img-src 'self' data: https:; " +
	"font-src 'self'; " +
	"connect-src 'self'; " +
	"frame-ancestors 'none'; " +
	"base-uri 'self'; " +
	"form-action 'self'; " +
	"object-src 'none'"

// permissionsPolicy 不要なブラウザ機能を一括で無効化します。
const permissionsPolicy = "camera=(), microphone=(), geolocation=()"

// sessionFromContext コンテキストからセッションを取り出します。未設定ならゼロ値を返します。
func sessionFromContext(ctx context.Context) Session {
	sess, _ := ctx.Value(sessionCtxKey).(Session)
	return sess
}

// withSession セッションをコンテキストへ格納したリクエストを返します。
func withSession(r *http.Request, sess Session) *http.Request {
	ctx := context.WithValue(r.Context(), sessionCtxKey, sess)
	return r.WithContext(ctx)
}

// sessionID リクエストのCookieからセッションIDを取り出します。Cookie名はDeps.SessionCookieNameを使います。
func (h *Handler) sessionID(r *http.Request) (string, bool) {
	c, err := r.Cookie(h.deps.SessionCookieName)
	if err != nil {
		return "", false
	}
	return c.Value, true
}

// securityHeaders 全レスポンスにセキュリティヘッダを付与します。設計書のセクション9.1に対応します。
// HSTSはhttps公開時(Deps.IsHTTPS)にだけ付与し、平文の開発アクセスでHTTPS強制が起きるのを防ぎます。
func (h *Handler) securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		head := w.Header()
		head.Set("X-Content-Type-Options", "nosniff")
		head.Set("Referrer-Policy", "strict-origin-when-cross-origin")
		head.Set("X-Frame-Options", "DENY")
		head.Set("Permissions-Policy", permissionsPolicy)
		head.Set("Content-Security-Policy", contentSecurityPolicy)
		if h.deps.IsHTTPS {
			head.Set("Strict-Transport-Security", hstsValue)
		}
		next.ServeHTTP(w, r)
	})
}

// requireAuth 未認証アクセスを/loginへリダイレクトし、認証済みならセッションをコンテキストへ載せます。
// 認証済みのときはセッションIDをキーにCSRFトークンを発行し、Sessionへ充填してコンテキストへ載せます。
func (h *Handler) requireAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		username, ok := h.deps.Sessions.Validate(r)
		if !ok {
			http.Redirect(w, r, "/login", http.StatusSeeOther)
			return
		}
		id, _ := h.sessionID(r)
		token, err := h.deps.CSRF.Issue(id)
		if err != nil {
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		sess := Session{ID: id, Username: username, CSRFToken: token}
		next.ServeHTTP(w, withSession(r, sess))
	})
}

// requireCSRF 状態変更系のPOSTに対しCSRFトークンを検証します。設計書のセクション9.1に対応します。
// 照合はセッションIDをキーにCSRF.Verifyへ委ね、ヘッダとフォーム値の両方をサポートします。
func (h *Handler) requireCSRF(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		username, ok := h.deps.Sessions.Validate(r)
		if !ok {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		id, hasID := h.sessionID(r)
		if !hasID || !h.deps.CSRF.Verify(id, r) {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		token, _ := h.deps.CSRF.Token(id)
		sess := Session{ID: id, Username: username, CSRFToken: token}
		next.ServeHTTP(w, withSession(r, sess))
	})
}

// rateLimitLogin ログイン試行をクライアントIP単位でレート制限します。設計書のセクション9.1に対応します。
func (h *Handler) rateLimitLogin(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !h.deps.LoginLimiter.Allow(clientKey(r)) {
			http.Error(w, "too many requests", http.StatusTooManyRequests)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// clientKey レート制限のキーをクライアントIPから生成します。
// 前段のnginxがリバースプロキシするため、信頼できる前段が付与するX-Real-IPを優先します。
// 無ければX-Forwarded-Forの先頭ホップ、さらに無ければRemoteAddrへフォールバックします。
// X-Real-IPとX-Forwarded-Forはnginxが上書き付与する前提で、直接公開時はスプーフィングされうる点に注意します。
func clientKey(r *http.Request) string {
	if ip := strings.TrimSpace(r.Header.Get("X-Real-IP")); ip != "" {
		return ip
	}
	if fwd := r.Header.Get("X-Forwarded-For"); fwd != "" {
		parts := strings.Split(fwd, ",")
		if first := strings.TrimSpace(parts[0]); first != "" {
			return first
		}
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}
