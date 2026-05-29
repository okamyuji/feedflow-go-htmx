package auth

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/okamyuji/feedflow-go-htmx/internal/domain"
)

func TestRequireSetupAvailableAllowsWhenUnregistered(t *testing.T) {
	t.Parallel()
	repo := &fakeUserRepo{user: domain.User{}}
	m := NewManager(repo, testParams())

	called := false
	guarded := RequireSetupAvailable(m, "/login")(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		called = true
	}))

	rec := httptest.NewRecorder()
	guarded.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/setup", nil))

	if !called {
		t.Fatalf("setup handler should run when no owner is registered")
	}
	if rec.Code == http.StatusSeeOther {
		t.Fatalf("unexpected redirect when setup is available")
	}
}

func TestRequireSetupAvailableRedirectsWhenRegistered(t *testing.T) {
	t.Parallel()
	repo := &fakeUserRepo{user: domain.User{Username: "owner", PasswordHash: "scrypt$1024$8$1$c2FsdA$aGFzaA"}}
	m := NewManager(repo, testParams())

	called := false
	guarded := RequireSetupAvailable(m, "/login")(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		called = true
	}))

	rec := httptest.NewRecorder()
	guarded.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/setup", nil))

	if called {
		t.Fatalf("setup handler must not run when owner is already registered")
	}
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("status got %d want 303", rec.Code)
	}
	if loc := rec.Result().Header.Get("Location"); loc != "/login" {
		t.Fatalf("redirect Location got %q want /login", loc)
	}
}

func TestRequireSetupAvailableServerErrorOnRepoFailure(t *testing.T) {
	t.Parallel()
	repo := &fakeUserRepo{loadErr: errLoad}
	m := NewManager(repo, testParams())

	guarded := RequireSetupAvailable(m, "/login")(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))

	rec := httptest.NewRecorder()
	guarded.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/setup", nil))

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status got %d want 500 on repo failure", rec.Code)
	}
}
