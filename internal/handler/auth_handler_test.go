package handler

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

// stubSetup 初回セットアップの状態を固定で返すSetupGuardのスタブです。
type stubSetup struct {
	needs        bool
	authOK       bool
	setupErr     error
	registered   bool
	lastUsername string
}

func (s *stubSetup) NeedsSetup() (bool, error) { return s.needs, nil }
func (s *stubSetup) Setup(username, _ string) error {
	if s.setupErr != nil {
		return s.setupErr
	}
	s.registered = true
	s.lastUsername = username
	s.needs = false
	return nil
}
func (s *stubSetup) Authenticate(_, _ string) (bool, error) { return s.authOK, nil }

func newAuthHandler(t *testing.T, setup *stubSetup, sessions Sessions, limiter RateLimiter) *Handler {
	t.Helper()
	h, err := New(Deps{Setup: setup, Sessions: sessions, LoginLimiter: limiter})
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}
	return h
}

func TestLoginPageRedirectsToSetupWhenNeeded(t *testing.T) {
	t.Parallel()
	h := newAuthHandler(t, &stubSetup{needs: true}, &stubSessions{}, stubLimiter{allow: true})
	req := httptest.NewRequest(http.MethodGet, "/login", nil)
	rec := httptest.NewRecorder()

	h.loginPage(rec, req)

	if rec.Code != http.StatusSeeOther {
		t.Fatalf("status got %d want %d", rec.Code, http.StatusSeeOther)
	}
	if loc := rec.Header().Get("Location"); loc != "/setup" {
		t.Fatalf("Location got %q want %q", loc, "/setup")
	}
}

func TestLoginSubmitSuccess(t *testing.T) {
	t.Parallel()
	sessions := &stubSessions{}
	h := newAuthHandler(t, &stubSetup{needs: false, authOK: true}, sessions, stubLimiter{allow: true})
	form := url.Values{"username": {"owner"}, "password": {"correct-password"}}
	req := httptest.NewRequest(http.MethodPost, "/login", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()

	h.loginSubmit(rec, req)

	if rec.Code != http.StatusSeeOther {
		t.Fatalf("status got %d want %d", rec.Code, http.StatusSeeOther)
	}
	if loc := rec.Header().Get("Location"); loc != "/app" {
		t.Fatalf("Location got %q want %q", loc, "/app")
	}
	if sessions.issued != "owner" {
		t.Fatalf("issued session username got %q want %q", sessions.issued, "owner")
	}
}

func TestLoginSubmitFailureShowsError(t *testing.T) {
	t.Parallel()
	h := newAuthHandler(t, &stubSetup{needs: false, authOK: false}, &stubSessions{}, stubLimiter{allow: true})
	form := url.Values{"username": {"owner"}, "password": {"wrong"}}
	req := httptest.NewRequest(http.MethodPost, "/login", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()

	h.loginSubmit(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status got %d want %d", rec.Code, http.StatusUnauthorized)
	}
	if !strings.Contains(rec.Body.String(), "ユーザー名またはパスワード") {
		t.Fatalf("body does not contain error message: %q", rec.Body.String())
	}
}

func TestSetupSubmitRegistersAndRedirects(t *testing.T) {
	t.Parallel()
	setup := &stubSetup{needs: true}
	h := newAuthHandler(t, setup, &stubSessions{}, stubLimiter{allow: true})
	form := url.Values{"username": {"owner"}, "password": {"strong-password"}}
	req := httptest.NewRequest(http.MethodPost, "/setup", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()

	h.setupSubmit(rec, req)

	if rec.Code != http.StatusSeeOther {
		t.Fatalf("status got %d want %d", rec.Code, http.StatusSeeOther)
	}
	if !setup.registered {
		t.Fatalf("setup was not registered")
	}
	if loc := rec.Header().Get("Location"); loc != "/login" {
		t.Fatalf("Location got %q want %q", loc, "/login")
	}
}

func TestSetupSubmitBlockedWhenAlreadyRegistered(t *testing.T) {
	t.Parallel()
	setup := &stubSetup{needs: false}
	h := newAuthHandler(t, setup, &stubSessions{}, stubLimiter{allow: true})
	form := url.Values{"username": {"intruder"}, "password": {"whatever1"}}
	req := httptest.NewRequest(http.MethodPost, "/setup", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()

	h.setupSubmit(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("status got %d want %d", rec.Code, http.StatusForbidden)
	}
	if setup.registered {
		t.Fatalf("setup must not register when already registered")
	}
}
