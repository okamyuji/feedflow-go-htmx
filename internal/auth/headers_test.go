package auth

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func okHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})
}

func TestSecurityHeadersSetsBaselineHeaders(t *testing.T) {
	t.Parallel()
	h := SecurityHeaders(HeaderConfig{EnableHSTS: false})(okHandler())

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	h.ServeHTTP(rec, req)

	res := rec.Result()
	checks := map[string]string{
		"X-Content-Type-Options": "nosniff",
		"X-Frame-Options":        "DENY",
		"Referrer-Policy":        "strict-origin-when-cross-origin",
	}
	for k, want := range checks {
		if got := res.Header.Get(k); got != want {
			t.Fatalf("header %s got %q want %q", k, got, want)
		}
	}
	if res.Header.Get("Permissions-Policy") == "" {
		t.Fatalf("Permissions-Policy is empty")
	}
	if res.Header.Get("Content-Security-Policy") == "" {
		t.Fatalf("Content-Security-Policy is empty")
	}
}

func TestSecurityHeadersOmitsHSTSWhenDisabled(t *testing.T) {
	t.Parallel()
	h := SecurityHeaders(HeaderConfig{EnableHSTS: false})(okHandler())

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	h.ServeHTTP(rec, req)

	if got := rec.Result().Header.Get("Strict-Transport-Security"); got != "" {
		t.Fatalf("HSTS got %q want empty when disabled", got)
	}
}

func TestSecurityHeadersSetsHSTSWhenEnabled(t *testing.T) {
	t.Parallel()
	h := SecurityHeaders(HeaderConfig{EnableHSTS: true})(okHandler())

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	h.ServeHTTP(rec, req)

	got := rec.Result().Header.Get("Strict-Transport-Security")
	if got == "" {
		t.Fatalf("HSTS is empty when enabled")
	}
}

func TestSecurityHeadersPassesThroughResponse(t *testing.T) {
	t.Parallel()
	h := SecurityHeaders(HeaderConfig{EnableHSTS: false})(okHandler())
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status got %d want 200", rec.Code)
	}
	if rec.Body.String() != "ok" {
		t.Fatalf("body got %q want ok", rec.Body.String())
	}
}
