package auth

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestRequireAuthRedirectsWhenNoSession(t *testing.T) {
	t.Parallel()
	clk := &fakeClock{now: time.Now()}
	sessions := NewSessionStore(SessionConfig{Clock: clk, TTL: time.Hour, CookieName: "feedflow_session", Secure: false})

	called := false
	guarded := RequireAuth(sessions, "/login")(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		called = true
	}))

	rec := httptest.NewRecorder()
	guarded.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))

	if called {
		t.Fatalf("guarded handler was called without a session")
	}
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("status got %d want 303", rec.Code)
	}
	if loc := rec.Result().Header.Get("Location"); loc != "/login" {
		t.Fatalf("redirect Location got %q want /login", loc)
	}
}

func TestRequireAuthPassesWithValidSessionAndInjectsUser(t *testing.T) {
	t.Parallel()
	clk := &fakeClock{now: time.Now()}
	sessions := NewSessionStore(SessionConfig{Clock: clk, TTL: time.Hour, CookieName: "feedflow_session", Secure: false})

	issueRec := httptest.NewRecorder()
	if err := sessions.Issue(issueRec, "owner"); err != nil {
		t.Fatalf("Issue returned error: %v", err)
	}
	cookie := issueRec.Result().Cookies()[0]

	var gotUser string
	guarded := RequireAuth(sessions, "/login")(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		gotUser = UsernameFromContext(r.Context())
	}))

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.AddCookie(cookie)
	guarded.ServeHTTP(rec, req)

	if gotUser != "owner" {
		t.Fatalf("UsernameFromContext got %q want owner", gotUser)
	}
}

func TestRequireCSRFAllowsSafeMethods(t *testing.T) {
	t.Parallel()
	clk := &fakeClock{now: time.Now()}
	sessions := NewSessionStore(SessionConfig{Clock: clk, TTL: time.Hour, CookieName: "feedflow_session", Secure: false})
	csrf := NewCSRFStore()

	called := false
	guarded := RequireCSRF(sessions, csrf)(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		called = true
	}))

	rec := httptest.NewRecorder()
	guarded.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))
	if !called {
		t.Fatalf("GET should bypass CSRF but handler was not called")
	}
}

func TestRequireCSRFRejectsPostWithoutToken(t *testing.T) {
	t.Parallel()
	clk := &fakeClock{now: time.Now()}
	sessions := NewSessionStore(SessionConfig{Clock: clk, TTL: time.Hour, CookieName: "feedflow_session", Secure: false})
	csrf := NewCSRFStore()

	issueRec := httptest.NewRecorder()
	if err := sessions.Issue(issueRec, "owner"); err != nil {
		t.Fatalf("Issue returned error: %v", err)
	}
	cookie := issueRec.Result().Cookies()[0]
	// セッションIDはCookieの値です。そのIDでCSRFトークンを発行しておきます。
	if _, err := csrf.Issue(cookie.Value); err != nil {
		t.Fatalf("csrf Issue returned error: %v", err)
	}

	called := false
	guarded := RequireCSRF(sessions, csrf)(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		called = true
	}))

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/feeds", nil)
	req.AddCookie(cookie)
	guarded.ServeHTTP(rec, req)

	if called {
		t.Fatalf("POST without CSRF token should be rejected")
	}
	if rec.Code != http.StatusForbidden {
		t.Fatalf("status got %d want 403", rec.Code)
	}
}

func TestRequireCSRFAcceptsPostWithToken(t *testing.T) {
	t.Parallel()
	clk := &fakeClock{now: time.Now()}
	sessions := NewSessionStore(SessionConfig{Clock: clk, TTL: time.Hour, CookieName: "feedflow_session", Secure: false})
	csrf := NewCSRFStore()

	issueRec := httptest.NewRecorder()
	if err := sessions.Issue(issueRec, "owner"); err != nil {
		t.Fatalf("Issue returned error: %v", err)
	}
	cookie := issueRec.Result().Cookies()[0]
	token, err := csrf.Issue(cookie.Value)
	if err != nil {
		t.Fatalf("csrf Issue returned error: %v", err)
	}

	called := false
	guarded := RequireCSRF(sessions, csrf)(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		called = true
	}))

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/feeds", nil)
	req.AddCookie(cookie)
	req.Header.Set(CSRFHeaderName, token)
	guarded.ServeHTTP(rec, req)

	if !called {
		t.Fatalf("POST with valid CSRF token should be allowed")
	}
}

func TestClientIP(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name       string
		remoteAddr string
		want       string
	}{
		{name: "ホストとポート", remoteAddr: "203.0.113.5:54321", want: "203.0.113.5"},
		{name: "ポートなし", remoteAddr: "203.0.113.9", want: "203.0.113.9"},
		{name: "IPv6", remoteAddr: "[2001:db8::1]:443", want: "2001:db8::1"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			req := httptest.NewRequest(http.MethodGet, "/", nil)
			req.RemoteAddr = tt.remoteAddr
			if got := ClientIP(req); got != tt.want {
				t.Fatalf("ClientIP got %q want %q", got, tt.want)
			}
		})
	}
}
