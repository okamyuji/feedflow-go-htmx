package handler

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestStaticHandlerServesHTMX(t *testing.T) {
	t.Parallel()
	srv := staticHandler()
	req := httptest.NewRequest(http.MethodGet, "/static/htmx.min.js", nil)
	rec := httptest.NewRecorder()

	srv.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status got %d want %d", rec.Code, http.StatusOK)
	}
	if ct := rec.Header().Get("Content-Type"); ct == "" {
		t.Fatalf("Content-Type is empty")
	}
	if cc := rec.Header().Get("Cache-Control"); cc == "" {
		t.Fatalf("Cache-Control is empty")
	}
	if rec.Body.Len() == 0 {
		t.Fatalf("body is empty")
	}
}

func TestStaticHandlerNotFound(t *testing.T) {
	t.Parallel()
	srv := staticHandler()
	req := httptest.NewRequest(http.MethodGet, "/static/missing.js", nil)
	rec := httptest.NewRecorder()

	srv.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status got %d want %d", rec.Code, http.StatusNotFound)
	}
}
