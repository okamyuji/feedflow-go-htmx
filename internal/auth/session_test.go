package auth

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

type fakeClock struct{ now time.Time }

func (c *fakeClock) Now() time.Time { return c.now }

func newTestSessions(clk *fakeClock, secure bool) *SessionStore {
	return NewSessionStore(SessionConfig{
		Clock:      clk,
		TTL:        2 * time.Hour,
		CookieName: "feedflow_session",
		Secure:     secure,
	})
}

func TestSessionIssueAndValidate(t *testing.T) {
	t.Parallel()
	clk := &fakeClock{now: time.Date(2026, 5, 29, 10, 0, 0, 0, time.UTC)}
	store := newTestSessions(clk, true)

	rec := httptest.NewRecorder()
	if err := store.Issue(rec, "owner"); err != nil {
		t.Fatalf("Issue returned error: %v", err)
	}

	cookies := rec.Result().Cookies()
	if len(cookies) != 1 {
		t.Fatalf("cookie count got %d want 1", len(cookies))
	}
	c := cookies[0]
	if !c.HttpOnly {
		t.Fatalf("cookie HttpOnly got false want true")
	}
	if !c.Secure {
		t.Fatalf("cookie Secure got false want true")
	}
	if c.SameSite != http.SameSiteLaxMode {
		t.Fatalf("cookie SameSite got %v want Lax", c.SameSite)
	}
	if c.Path != "/" {
		t.Fatalf("cookie Path got %q want /", c.Path)
	}

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.AddCookie(c)
	username, ok := store.Validate(req)
	if !ok {
		t.Fatalf("Validate got ok=false want true for freshly issued session")
	}
	if username != "owner" {
		t.Fatalf("Validate username got %q want owner", username)
	}
}

func TestSessionValidateRejectsExpired(t *testing.T) {
	t.Parallel()
	clk := &fakeClock{now: time.Date(2026, 5, 29, 10, 0, 0, 0, time.UTC)}
	store := newTestSessions(clk, true)

	rec := httptest.NewRecorder()
	if err := store.Issue(rec, "owner"); err != nil {
		t.Fatalf("Issue returned error: %v", err)
	}
	c := rec.Result().Cookies()[0]

	// TTL を超えて時刻を進めます。
	clk.now = clk.now.Add(3 * time.Hour)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.AddCookie(c)
	if _, ok := store.Validate(req); ok {
		t.Fatalf("Validate got ok=true want false for expired session")
	}
}

func TestSessionValidateRejectsUnknownCookie(t *testing.T) {
	t.Parallel()
	clk := &fakeClock{now: time.Date(2026, 5, 29, 10, 0, 0, 0, time.UTC)}
	store := newTestSessions(clk, true)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.AddCookie(&http.Cookie{Name: "feedflow_session", Value: "nonexistent"})
	if _, ok := store.Validate(req); ok {
		t.Fatalf("Validate got ok=true want false for unknown session id")
	}
}

func TestSessionDestroyRemovesSession(t *testing.T) {
	t.Parallel()
	clk := &fakeClock{now: time.Date(2026, 5, 29, 10, 0, 0, 0, time.UTC)}
	store := newTestSessions(clk, true)

	rec := httptest.NewRecorder()
	if err := store.Issue(rec, "owner"); err != nil {
		t.Fatalf("Issue returned error: %v", err)
	}
	c := rec.Result().Cookies()[0]

	delRec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/logout", nil)
	req.AddCookie(c)
	store.Destroy(delRec, req)

	// 破棄用CookieはMaxAgeが負で即時失効を指示します。
	out := delRec.Result().Cookies()
	if len(out) != 1 {
		t.Fatalf("destroy cookie count got %d want 1", len(out))
	}
	if out[0].MaxAge >= 0 {
		t.Fatalf("destroy cookie MaxAge got %d want negative", out[0].MaxAge)
	}

	check := httptest.NewRequest(http.MethodGet, "/", nil)
	check.AddCookie(c)
	if _, ok := store.Validate(check); ok {
		t.Fatalf("Validate got ok=true want false after Destroy")
	}
}

func TestSessionSecureFalseOmitsSecureAttribute(t *testing.T) {
	t.Parallel()
	clk := &fakeClock{now: time.Date(2026, 5, 29, 10, 0, 0, 0, time.UTC)}
	store := newTestSessions(clk, false)

	rec := httptest.NewRecorder()
	if err := store.Issue(rec, "owner"); err != nil {
		t.Fatalf("Issue returned error: %v", err)
	}
	if rec.Result().Cookies()[0].Secure {
		t.Fatalf("cookie Secure got true want false when Secure config is false")
	}
}
