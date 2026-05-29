package main

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestBuildApp_HealthzOK(t *testing.T) {
	t.Setenv("FEEDFLOW_DATA_DIR", t.TempDir())

	routes, runner, err := buildApp()
	if err != nil {
		t.Fatalf("buildApp returned error: %v", err)
	}
	if runner == nil {
		t.Fatal("buildApp returned nil runner")
	}

	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	rec := httptest.NewRecorder()
	routes.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status got %d want %d", rec.Code, http.StatusOK)
	}
	if got := rec.Body.String(); got != "ok" {
		t.Fatalf("body got %q want %q", got, "ok")
	}
}

func TestBuildApp_RedirectsToSetupWhenUnregistered(t *testing.T) {
	t.Setenv("FEEDFLOW_DATA_DIR", t.TempDir())

	routes, _, err := buildApp()
	if err != nil {
		t.Fatalf("buildApp returned error: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/app", nil)
	rec := httptest.NewRecorder()
	routes.ServeHTTP(rec, req)

	if rec.Code != http.StatusSeeOther && rec.Code != http.StatusFound {
		t.Fatalf("未登録での /app アクセスはリダイレクトを期待しますが got %d でした", rec.Code)
	}
}

func TestEnvOr_ReturnsValueWhenSet(t *testing.T) {
	t.Setenv("FEEDFLOW_TEST_ENVOR", "value")
	if got := envOr("FEEDFLOW_TEST_ENVOR", "fallback"); got != "value" {
		t.Fatalf("envOr set got %q want %q", got, "value")
	}
}

func TestEnvOr_ReturnsDefaultWhenUnsetOrEmpty(t *testing.T) {
	if got := envOr("FEEDFLOW_TEST_ENVOR_UNSET", "fallback"); got != "fallback" {
		t.Fatalf("envOr unset got %q want %q", got, "fallback")
	}
	t.Setenv("FEEDFLOW_TEST_ENVOR_EMPTY", "")
	if got := envOr("FEEDFLOW_TEST_ENVOR_EMPTY", "fallback"); got != "fallback" {
		t.Fatalf("envOr empty got %q want %q", got, "fallback")
	}
}
