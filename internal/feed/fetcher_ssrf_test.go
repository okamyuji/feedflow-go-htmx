package feed

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/okamyuji/feedflow-go-htmx/internal/port"
)

func TestFetchBlocksLoopback(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("should not reach"))
	}))
	defer srv.Close()

	// 既定のSSRFガード付きクライアントを使う。
	f := NewHTTPFetcher()
	_, err := f.Fetch(context.Background(), port.FetchRequest{URL: srv.URL})
	if err == nil {
		t.Fatal("ループバックへの取得はブロックされるべきですがnilでした")
	}
	if !errors.Is(err, ErrPrivateAddress) {
		t.Fatalf("ErrPrivateAddressを期待しましたが %vでした", err)
	}
}

func TestFetchBlocksMetadataIP(t *testing.T) {
	f := NewHTTPFetcher(WithTimeout(2 * 1e9))
	_, err := f.Fetch(context.Background(), port.FetchRequest{URL: "http://169.254.169.254/latest/meta-data/"})
	if err == nil {
		t.Fatal("クラウドメタデータIPへの取得はブロックされるべきですがnilでした")
	}
	if !errors.Is(err, ErrPrivateAddress) {
		t.Fatalf("ErrPrivateAddressを期待しましたが %vでした", err)
	}
}
