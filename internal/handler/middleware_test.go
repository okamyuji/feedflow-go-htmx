package handler

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// stubSessions 認証状態を固定で返すSessionsのスタブです。
// Validateはusernameとokを固定で返し、issuedは発行されたユーザー名を記録します。
type stubSessions struct {
	username string
	ok       bool
	issued   string
}

func (s *stubSessions) Issue(_ http.ResponseWriter, username string) error {
	s.issued = username
	return nil
}
func (s *stubSessions) Validate(_ *http.Request) (string, bool)        { return s.username, s.ok }
func (s *stubSessions) Destroy(_ http.ResponseWriter, _ *http.Request) {}

// stubCSRF トークン発行と一致判定を固定で返すCSRFのスタブです。
type stubCSRF struct {
	ok    bool
	token string
}

func (c *stubCSRF) Issue(_ string) (string, error)        { return c.token, nil }
func (c *stubCSRF) Token(_ string) (string, bool)         { return c.token, c.token != "" }
func (c *stubCSRF) Verify(_ string, _ *http.Request) bool { return c.ok }
func (c *stubCSRF) Discard(_ string)                      {}

// stubLimiter 許可を固定で返すRateLimiterのスタブです。
type stubLimiter struct{ allow bool }

func (l stubLimiter) Allow(_ string) bool { return l.allow }

func TestSecurityHeadersAlwaysSetsBaseHeaders(t *testing.T) {
	t.Parallel()
	h := &Handler{}
	called := false
	next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	})
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()

	h.securityHeaders(next).ServeHTTP(rec, req)

	if !called {
		t.Fatalf("next handler was not called")
	}
	wantHeaders := map[string]string{
		"X-Content-Type-Options": "nosniff",
		"Referrer-Policy":        "strict-origin-when-cross-origin",
		"X-Frame-Options":        "DENY",
		"Permissions-Policy":     "camera=(), microphone=(), geolocation=()",
	}
	for k, want := range wantHeaders {
		if got := rec.Header().Get(k); got != want {
			t.Fatalf("header %s got %q want %q", k, got, want)
		}
	}
	if csp := rec.Header().Get("Content-Security-Policy"); !strings.Contains(csp, "form-action 'self'") {
		t.Fatalf("CSP should contain form-action 'self': %q", csp)
	}
}

func TestSecurityHeadersOmitsHSTSWhenNotHTTPS(t *testing.T) {
	t.Parallel()
	h := &Handler{deps: Deps{IsHTTPS: false}}
	next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) })
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()

	h.securityHeaders(next).ServeHTTP(rec, req)

	if got := rec.Header().Get("Strict-Transport-Security"); got != "" {
		t.Fatalf("HSTS got %q want empty when not https", got)
	}
}

func TestSecurityHeadersAddsHSTSWhenHTTPS(t *testing.T) {
	t.Parallel()
	h := &Handler{deps: Deps{IsHTTPS: true}}
	next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) })
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()

	h.securityHeaders(next).ServeHTTP(rec, req)

	if got := rec.Header().Get("Strict-Transport-Security"); got == "" {
		t.Fatalf("HSTS should be set when https")
	}
}

func TestRequireAuthRedirectsWhenUnauthenticated(t *testing.T) {
	t.Parallel()
	h := &Handler{deps: Deps{Sessions: &stubSessions{ok: false}, CSRF: &stubCSRF{token: "tok"}, SessionCookieName: "feedflow_session"}}
	next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	req := httptest.NewRequest(http.MethodGet, "/app", nil)
	rec := httptest.NewRecorder()

	h.requireAuth(next).ServeHTTP(rec, req)

	if rec.Code != http.StatusSeeOther {
		t.Fatalf("status got %d want %d", rec.Code, http.StatusSeeOther)
	}
	if loc := rec.Header().Get("Location"); loc != "/login" {
		t.Fatalf("Location got %q want %q", loc, "/login")
	}
}

func TestRequireAuthPassesWhenAuthenticated(t *testing.T) {
	t.Parallel()
	h := &Handler{deps: Deps{
		Sessions:          &stubSessions{username: "owner", ok: true},
		CSRF:              &stubCSRF{token: "csrf-value"},
		SessionCookieName: "feedflow_session",
	}}
	gotUser := ""
	gotToken := ""
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sess := sessionFromContext(r.Context())
		gotUser = sess.Username
		gotToken = sess.CSRFToken
		w.WriteHeader(http.StatusOK)
	})
	req := httptest.NewRequest(http.MethodGet, "/app", nil)
	req.AddCookie(&http.Cookie{Name: "feedflow_session", Value: "sess-1"})
	rec := httptest.NewRecorder()

	h.requireAuth(next).ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status got %d want %d", rec.Code, http.StatusOK)
	}
	if gotUser != "owner" {
		t.Fatalf("session username got %q want %q", gotUser, "owner")
	}
	if gotToken != "csrf-value" {
		t.Fatalf("session CSRFToken got %q want %q", gotToken, "csrf-value")
	}
}

func TestRequireCSRFRejectsBadToken(t *testing.T) {
	t.Parallel()
	h := &Handler{deps: Deps{
		Sessions:          &stubSessions{username: "owner", ok: true},
		CSRF:              &stubCSRF{ok: false, token: "good"},
		SessionCookieName: "feedflow_session",
	}}
	next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	req := httptest.NewRequest(http.MethodPost, "/app/items/mark", nil)
	req.AddCookie(&http.Cookie{Name: "feedflow_session", Value: "sess-1"})
	rec := httptest.NewRecorder()

	h.requireCSRF(next).ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("status got %d want %d", rec.Code, http.StatusForbidden)
	}
}
