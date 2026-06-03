package handler

import (
	"io/fs"
	"net/http"
	"net/http/httptest"
	"strings"
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

func TestStaticHandlerETagAnd304(t *testing.T) {
	t.Parallel()
	srv := staticHandler()

	req1 := httptest.NewRequest(http.MethodGet, "/static/htmx.min.js", nil)
	rec1 := httptest.NewRecorder()
	srv.ServeHTTP(rec1, req1)

	etag := rec1.Header().Get("ETag")
	if etag == "" {
		t.Fatal("ETag が付与されていません")
	}
	if cc := rec1.Header().Get("Cache-Control"); cc != "no-cache" {
		t.Fatalf("Cache-Control got %q want %q", cc, "no-cache")
	}

	req2 := httptest.NewRequest(http.MethodGet, "/static/htmx.min.js", nil)
	req2.Header.Set("If-None-Match", etag)
	rec2 := httptest.NewRecorder()
	srv.ServeHTTP(rec2, req2)

	if rec2.Code != http.StatusNotModified {
		t.Fatalf("If-None-Match一致時のstatus got %d want %d", rec2.Code, http.StatusNotModified)
	}
}

func TestAppJSListAutoReadQueuesWithoutOptimisticReadState(t *testing.T) {
	t.Parallel()
	data, err := fs.ReadFile(staticFS, "static/app.js")
	if err != nil {
		t.Fatalf("read app.js: %v", err)
	}
	js := string(data)

	start := strings.Index(js, "onListScroll()")
	end := strings.Index(js, "focusNextCard(delta)")
	if start == -1 || end == -1 || end <= start {
		t.Fatalf("app.js should contain onListScroll followed by focusNextCard")
	}
	onListScroll := js[start:end]

	if strings.Contains(onListScroll, `card.classList.add("is-read")`) {
		t.Fatalf("list auto-read must not mark cards read before the server response")
	}
	if !strings.Contains(js, "readQueue") || !strings.Contains(js, "processReadQueue") {
		t.Fatalf("list auto-read should queue read requests instead of firing concurrent htmx.ajax calls")
	}
	if !strings.Contains(js, "finally(() =>") || !strings.Contains(js, "this.markedRead.delete(") {
		t.Fatalf("queued read requests must clear the in-flight key so failed or unchanged cards can retry")
	}
}
