package auth

import "net/http"

// HeaderConfig セキュリティヘッダの付与方針です。
type HeaderConfig struct {
	EnableHSTS bool // HTTPS公開時にStrict-Transport-Securityを付けるかどうかです
}

// contentSecurityPolicy feedflowのCSPです。
// HTMXとAlpine.jsをベンダーしてselfから配信するためscript-srcはselfに限定します。
// Alpine.jsのインライン属性を許すためstyle-srcにunsafe-inlineを含めます。
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
const permissionsPolicy = "camera=(), microphone=(), geolocation=(), interest-cohort=()"

// hstsValue 1年間のHSTSとサブドメインとプリロードを指示します。
const hstsValue = "max-age=31536000; includeSubDomains; preload"

// SecurityHeaders 全レスポンスにセキュリティヘッダを付与するミドルウェアを返します。
// 設計書のセクション9.1のヘッダ(HSTS、X-Content-Type-Options、Referrer-Policy、Permissions-Policyほか)を付けます。
// HSTSはHeaderConfig.EnableHSTSがtrueのときだけ付けます。
func SecurityHeaders(cfg HeaderConfig) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			h := w.Header()
			h.Set("X-Content-Type-Options", "nosniff")
			h.Set("X-Frame-Options", "DENY")
			h.Set("Referrer-Policy", "strict-origin-when-cross-origin")
			h.Set("Permissions-Policy", permissionsPolicy)
			h.Set("Content-Security-Policy", contentSecurityPolicy)
			if cfg.EnableHSTS {
				h.Set("Strict-Transport-Security", hstsValue)
			}
			next.ServeHTTP(w, r)
		})
	}
}
