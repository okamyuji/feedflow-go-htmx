package handler

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/okamyuji/feedflow-go-htmx/internal/domain"
)

func newFullHandler(t *testing.T, authenticated bool) *Handler {
	t.Helper()
	subs := &stubSubscriptions{feeds: []domain.Feed{{ID: "f1", Title: "f1"}}}
	items := &stubItems{items: map[string][]domain.Item{
		"f1": {{ID: "i1", FeedID: "f1", Title: "記事1"}},
	}}
	h, err := New(Deps{
		Subscriptions:     subs,
		Items:             items,
		Bookmarks:         &stubBookmarks{},
		Mutes:             &stubMutes{},
		Settings:          &stubSettings{current: domain.DefaultSettings()},
		OPML:              &stubOPML{},
		Sessions:          &stubSessions{username: "owner", ok: authenticated},
		CSRF:              &stubCSRF{ok: true, token: "tok"},
		LoginLimiter:      stubLimiter{allow: true},
		Setup:             &stubSetup{needs: false, authOK: true},
		SessionCookieName: "feedflow_session",
	})
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}
	return h
}

func TestRouterHealthz(t *testing.T) {
	t.Parallel()
	h := newFullHandler(t, true)
	srv := h.Routes()
	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	rec := httptest.NewRecorder()

	srv.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status got %d want %d", rec.Code, http.StatusOK)
	}
	if rec.Header().Get("X-Content-Type-Options") != "nosniff" {
		t.Fatalf("security header missing on healthz")
	}
}

func TestRouterRootRedirectsToApp(t *testing.T) {
	t.Parallel()
	h := newFullHandler(t, true)
	srv := h.Routes()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()

	srv.ServeHTTP(rec, req)

	if rec.Code != http.StatusSeeOther {
		t.Fatalf("status got %d want %d", rec.Code, http.StatusSeeOther)
	}
	if loc := rec.Header().Get("Location"); loc != "/app" {
		t.Fatalf("Location got %q want %q", loc, "/app")
	}
}

func TestRouterAppRequiresAuth(t *testing.T) {
	t.Parallel()
	h := newFullHandler(t, false)
	srv := h.Routes()
	req := httptest.NewRequest(http.MethodGet, "/app", nil)
	rec := httptest.NewRecorder()

	srv.ServeHTTP(rec, req)

	if rec.Code != http.StatusSeeOther {
		t.Fatalf("status got %d want %d", rec.Code, http.StatusSeeOther)
	}
	if loc := rec.Header().Get("Location"); loc != "/login" {
		t.Fatalf("Location got %q want %q", loc, "/login")
	}
}

func TestRouterAppRendersWhenAuthenticated(t *testing.T) {
	t.Parallel()
	h := newFullHandler(t, true)
	srv := h.Routes()
	req := httptest.NewRequest(http.MethodGet, "/app", nil)
	rec := httptest.NewRecorder()

	srv.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status got %d want %d", rec.Code, http.StatusOK)
	}
	if rec.Header().Get("Content-Type") == "" {
		t.Fatalf("Content-Type missing")
	}
}

func TestRouterAppDisablesHTMXScriptExecution(t *testing.T) {
	t.Parallel()
	h := newFullHandler(t, true)
	srv := h.Routes()
	req := httptest.NewRequest(http.MethodGet, "/app", nil)
	rec := httptest.NewRecorder()

	srv.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status got %d want %d", rec.Code, http.StatusOK)
	}
	if !strings.Contains(rec.Body.String(), `"allowScriptTags":false`) {
		t.Fatalf("htmx config should disable swapped script execution: %q", rec.Body.String())
	}
}

func TestRouterStaticServed(t *testing.T) {
	t.Parallel()
	h := newFullHandler(t, true)
	srv := h.Routes()
	req := httptest.NewRequest(http.MethodGet, "/static/htmx.min.js", nil)
	rec := httptest.NewRecorder()

	srv.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status got %d want %d", rec.Code, http.StatusOK)
	}
}

func TestRouterItemActionRequiresCSRF(t *testing.T) {
	t.Parallel()
	subs := &stubSubscriptions{}
	items := &stubItems{items: map[string][]domain.Item{"f1": {{ID: "i1", FeedID: "f1"}}}}
	h, err := New(Deps{
		Subscriptions:     subs,
		Items:             items,
		Bookmarks:         &stubBookmarks{},
		Mutes:             &stubMutes{},
		Settings:          &stubSettings{current: domain.DefaultSettings()},
		OPML:              &stubOPML{},
		Sessions:          &stubSessions{username: "owner", ok: true},
		CSRF:              &stubCSRF{ok: false, token: "tok"},
		LoginLimiter:      stubLimiter{allow: true},
		Setup:             &stubSetup{},
		SessionCookieName: "feedflow_session",
	})
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}
	srv := h.Routes()
	req := httptest.NewRequest(http.MethodPost, "/app/items/f1/i1/read", nil)
	req.AddCookie(&http.Cookie{Name: "feedflow_session", Value: "sess-1"})
	rec := httptest.NewRecorder()

	srv.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("status got %d want %d", rec.Code, http.StatusForbidden)
	}
}

func TestRouterAddURLRequiresAuth(t *testing.T) {
	t.Parallel()
	h := newFullHandler(t, false)
	srv := h.Routes()
	req := httptest.NewRequest(http.MethodPost, "/app/bookmarks/add-url", strings.NewReader("url=https%3A%2F%2Fexample.com%2Fa"))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()

	srv.ServeHTTP(rec, req)

	if rec.Code == http.StatusOK {
		t.Fatalf("未認証のURL追加は拒否すべき status got %d", rec.Code)
	}
}

func TestRouterAddURLRequiresCSRF(t *testing.T) {
	t.Parallel()
	h, err := New(Deps{
		Subscriptions:     &stubSubscriptions{},
		Items:             &stubItems{items: map[string][]domain.Item{}},
		Bookmarks:         &stubBookmarks{},
		Mutes:             &stubMutes{},
		Settings:          &stubSettings{current: domain.DefaultSettings()},
		OPML:              &stubOPML{},
		Sessions:          &stubSessions{username: "owner", ok: true},
		CSRF:              &stubCSRF{ok: false, token: "tok"},
		LoginLimiter:      stubLimiter{allow: true},
		Setup:             &stubSetup{},
		SessionCookieName: "feedflow_session",
	})
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}
	srv := h.Routes()
	req := httptest.NewRequest(http.MethodPost, "/app/bookmarks/add-url", strings.NewReader("url=https%3A%2F%2Fexample.com%2Fa"))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(&http.Cookie{Name: "feedflow_session", Value: "sess-1"})
	rec := httptest.NewRecorder()

	srv.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("status got %d want %d", rec.Code, http.StatusForbidden)
	}
}

func TestRouterAddURLRejectsGET(t *testing.T) {
	t.Parallel()
	h := newFullHandler(t, true)
	srv := h.Routes()
	req := httptest.NewRequest(http.MethodGet, "/app/bookmarks/add-url", nil)
	req.AddCookie(&http.Cookie{Name: "feedflow_session", Value: "sess-1"})
	rec := httptest.NewRecorder()

	srv.ServeHTTP(rec, req)

	if rec.Code == http.StatusOK {
		t.Fatalf("GETでのURL追加は受け付けてはいけません status got %d", rec.Code)
	}
}
