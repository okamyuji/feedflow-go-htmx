package main

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestBuildHandlerHealthz(t *testing.T) {
	h, err := buildHandler()
	if err != nil {
		t.Fatalf("buildHandler returned error: %v", err)
	}
	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	rec := httptest.NewRecorder()

	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status got %d want %d", rec.Code, http.StatusOK)
	}
	if got := rec.Body.String(); got != "ok" {
		t.Fatalf("body got %q want %q", got, "ok")
	}
}
