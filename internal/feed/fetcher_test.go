package feed

import (
	"bytes"
	"compress/gzip"
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/okamyuji/feedflow-go-htmx/internal/port"
)

// newTestFetcher テストサーバへ素直に接続するためのSSRFガードを介さないフェッチャを作ります。
func newTestFetcher(opts ...FetcherOption) *HTTPFetcher {
	base := make([]FetcherOption, 0, 1+len(opts))
	base = append(base, WithHTTPClient(&http.Client{}))
	return NewHTTPFetcher(append(base, opts...)...)
}

func TestFetchOK(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("User-Agent"); got == "" {
			t.Errorf("User-Agentが空です")
		}
		w.Header().Set("Content-Type", "application/rss+xml")
		w.Header().Set("ETag", `"abc"`)
		w.Header().Set("Last-Modified", "Wed, 21 Oct 2026 07:28:00 GMT")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("<rss></rss>"))
	}))
	defer srv.Close()

	f := newTestFetcher()
	res, err := f.Fetch(context.Background(), port.FetchRequest{URL: srv.URL})
	if err != nil {
		t.Fatalf("Fetch returned error: %v", err)
	}
	if res.StatusCode != http.StatusOK {
		t.Fatalf("StatusCode got %d want 200", res.StatusCode)
	}
	if res.NotModified {
		t.Fatal("NotModified must be false on 200")
	}
	if string(res.Body) != "<rss></rss>" {
		t.Fatalf("Body got %q", string(res.Body))
	}
	if res.ETag != `"abc"` {
		t.Fatalf("ETag got %q", res.ETag)
	}
	if res.LastModified != "Wed, 21 Oct 2026 07:28:00 GMT" {
		t.Fatalf("LastModified got %q", res.LastModified)
	}
	if res.ContentType != "application/rss+xml" {
		t.Fatalf("ContentType got %q", res.ContentType)
	}
}

func TestFetchConditionalHeaders(t *testing.T) {
	var gotETag, gotIMS string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotETag = r.Header.Get("If-None-Match")
		gotIMS = r.Header.Get("If-Modified-Since")
		w.WriteHeader(http.StatusNotModified)
	}))
	defer srv.Close()

	f := newTestFetcher()
	res, err := f.Fetch(context.Background(), port.FetchRequest{
		URL:          srv.URL,
		ETag:         `"abc"`,
		LastModified: "Wed, 21 Oct 2026 07:28:00 GMT",
	})
	if err != nil {
		t.Fatalf("Fetch returned error: %v", err)
	}
	if gotETag != `"abc"` {
		t.Fatalf("If-None-Match got %q want %q", gotETag, `"abc"`)
	}
	if gotIMS != "Wed, 21 Oct 2026 07:28:00 GMT" {
		t.Fatalf("If-Modified-Since got %q", gotIMS)
	}
	if !res.NotModified {
		t.Fatal("NotModified must be true on 304")
	}
	if res.StatusCode != http.StatusNotModified {
		t.Fatalf("StatusCode got %d want 304", res.StatusCode)
	}
	if len(res.Body) != 0 {
		t.Fatalf("Body must be empty on 304, got %q", string(res.Body))
	}
}

func TestFetchGzip(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Accept-Encoding"); got != "gzip" {
			t.Errorf("Accept-Encoding got %q want gzip", got)
		}
		var buf bytes.Buffer
		gz := gzip.NewWriter(&buf)
		_, _ = gz.Write([]byte("<feed>gzipped</feed>"))
		_ = gz.Close()
		w.Header().Set("Content-Encoding", "gzip")
		w.Header().Set("Content-Type", "application/atom+xml")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(buf.Bytes())
	}))
	defer srv.Close()

	f := newTestFetcher()
	res, err := f.Fetch(context.Background(), port.FetchRequest{URL: srv.URL})
	if err != nil {
		t.Fatalf("Fetch returned error: %v", err)
	}
	if string(res.Body) != "<feed>gzipped</feed>" {
		t.Fatalf("Body got %q want gzipped content decoded", string(res.Body))
	}
}

func TestFetchSizeLimit(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(bytes.Repeat([]byte("a"), 100))
	}))
	defer srv.Close()

	f := newTestFetcher(WithMaxBytes(10))
	_, err := f.Fetch(context.Background(), port.FetchRequest{URL: srv.URL})
	if err == nil {
		t.Fatal("サイズ上限超過でエラーを期待しましたがnilでした")
	}
}

func TestFetchRejectsBadScheme(t *testing.T) {
	f := newTestFetcher()
	_, err := f.Fetch(context.Background(), port.FetchRequest{URL: "file:///etc/passwd"})
	if err == nil {
		t.Fatal("スキーム違反でエラーを期待しましたがnilでした")
	}
}

func TestFetchServerError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte("boom"))
	}))
	defer srv.Close()

	f := newTestFetcher()
	res, err := f.Fetch(context.Background(), port.FetchRequest{URL: srv.URL})
	if err != nil {
		t.Fatalf("Fetch returned error: %v", err)
	}
	if res.StatusCode != http.StatusInternalServerError {
		t.Fatalf("StatusCode got %d want 500", res.StatusCode)
	}
}
