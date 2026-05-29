package feed

import (
	"testing"
	"time"

	"github.com/okamyuji/feedflow-go-htmx/internal/port"
)

func TestNewHTTPFetcherDefaults(t *testing.T) {
	f := NewHTTPFetcher()
	if f == nil {
		t.Fatal("NewHTTPFetcher returned nil")
	}
	if f.maxBytes != defaultMaxBytes {
		t.Fatalf("maxBytes got %d want %d", f.maxBytes, defaultMaxBytes)
	}
	if f.timeout != defaultTimeout {
		t.Fatalf("timeout got %v want %v", f.timeout, defaultTimeout)
	}
	if f.userAgent != defaultUserAgent {
		t.Fatalf("userAgent got %q want %q", f.userAgent, defaultUserAgent)
	}
	if f.client == nil {
		t.Fatal("client must not be nil")
	}
}

func TestNewHTTPFetcherOptions(t *testing.T) {
	f := NewHTTPFetcher(
		WithMaxBytes(123),
		WithTimeout(7*time.Second),
		WithUserAgent("custom/1.0"),
	)
	if f.maxBytes != 123 {
		t.Fatalf("maxBytes got %d want 123", f.maxBytes)
	}
	if f.timeout != 7*time.Second {
		t.Fatalf("timeout got %v want %v", f.timeout, 7*time.Second)
	}
	if f.userAgent != "custom/1.0" {
		t.Fatalf("userAgent got %q want %q", f.userAgent, "custom/1.0")
	}
}

// TestHTTPFetcherSatisfiesPort HTTPFetcherがport.Fetcherを満たすことをコンパイル時に検証します。
func TestHTTPFetcherSatisfiesPort(t *testing.T) {
	var _ port.Fetcher = NewHTTPFetcher()
}
