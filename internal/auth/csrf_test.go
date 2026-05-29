package auth

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

func TestCSRFIssueProducesToken(t *testing.T) {
	t.Parallel()
	store := NewCSRFStore()
	token, err := store.Issue("sess-1")
	if err != nil {
		t.Fatalf("Issue returned error: %v", err)
	}
	if token == "" {
		t.Fatalf("Issue token got empty want non-empty")
	}
}

func TestCSRFVerifyAcceptsHeaderToken(t *testing.T) {
	t.Parallel()
	store := NewCSRFStore()
	token, err := store.Issue("sess-1")
	if err != nil {
		t.Fatalf("Issue returned error: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/feeds", nil)
	req.Header.Set("X-CSRF-Token", token)
	if !store.Verify("sess-1", req) {
		t.Fatalf("Verify got false want true for valid header token")
	}
}

func TestCSRFVerifyAcceptsFormToken(t *testing.T) {
	t.Parallel()
	store := NewCSRFStore()
	token, err := store.Issue("sess-1")
	if err != nil {
		t.Fatalf("Issue returned error: %v", err)
	}

	form := url.Values{}
	form.Set(CSRFFieldName, token)
	req := httptest.NewRequest(http.MethodPost, "/feeds", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	if !store.Verify("sess-1", req) {
		t.Fatalf("Verify got false want true for valid form token")
	}
}

func TestCSRFVerifyRejectsWrongToken(t *testing.T) {
	t.Parallel()
	store := NewCSRFStore()
	if _, err := store.Issue("sess-1"); err != nil {
		t.Fatalf("Issue returned error: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/feeds", nil)
	req.Header.Set("X-CSRF-Token", "forged-token")
	if store.Verify("sess-1", req) {
		t.Fatalf("Verify got true want false for forged token")
	}
}

func TestCSRFVerifyRejectsUnknownSession(t *testing.T) {
	t.Parallel()
	store := NewCSRFStore()
	req := httptest.NewRequest(http.MethodPost, "/feeds", nil)
	req.Header.Set("X-CSRF-Token", "anything")
	if store.Verify("no-such-session", req) {
		t.Fatalf("Verify got true want false for unknown session")
	}
}

func TestCSRFDiscardRemovesToken(t *testing.T) {
	t.Parallel()
	store := NewCSRFStore()
	token, err := store.Issue("sess-1")
	if err != nil {
		t.Fatalf("Issue returned error: %v", err)
	}
	store.Discard("sess-1")

	req := httptest.NewRequest(http.MethodPost, "/feeds", nil)
	req.Header.Set("X-CSRF-Token", token)
	if store.Verify("sess-1", req) {
		t.Fatalf("Verify got true want false after Discard")
	}
}
