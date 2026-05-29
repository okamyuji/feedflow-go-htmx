package feed

import (
	"compress/gzip"
	"context"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/okamyuji/feedflow-go-htmx/internal/obs"
	"github.com/okamyuji/feedflow-go-htmx/internal/port"
)

// フェッチャの既定値です。
const (
	defaultMaxBytes  int64         = 10 << 20 // 1件のレスポンス本文の上限を10MiBとします
	defaultTimeout   time.Duration = 30 * time.Second
	defaultUserAgent string        = "feedflow/1.0 (+https://github.com/okamyuji/feedflow-go-htmx)"
)

// HTTPFetcher net/httpを用いたport.Fetcherの実装です。
// ETagとLast-Modifiedによる条件付き取得とgzip展開を行い、SSRF対策として接続先IPを検査します。
type HTTPFetcher struct {
	client       *http.Client
	logger       *slog.Logger
	maxBytes     int64
	timeout      time.Duration
	userAgent    string
	allowPrivate bool // trueのときSSRFガードを無効化しプライベートやループバック宛も許可します。テスト専用です
}

// FetcherOption HTTPFetcherの任意設定を上書きする関数オプションです。
type FetcherOption func(*HTTPFetcher)

// WithMaxBytes レスポンス本文の上限バイト数を設定します。
func WithMaxBytes(n int64) FetcherOption {
	return func(f *HTTPFetcher) { f.maxBytes = n }
}

// WithTimeout 1回の取得のタイムアウトを設定します。
func WithTimeout(d time.Duration) FetcherOption {
	return func(f *HTTPFetcher) { f.timeout = d }
}

// WithUserAgent リクエストのUser-Agentを設定します。
func WithUserAgent(ua string) FetcherOption {
	return func(f *HTTPFetcher) { f.userAgent = ua }
}

// WithLogger クローズ失敗の記録に使うloggerを設定します。
// 未設定のときはobs.CloseAndLogがslog.Defaultを使います。
func WithLogger(l *slog.Logger) FetcherOption {
	return func(f *HTTPFetcher) { f.logger = l }
}

// WithHTTPClient テスト用に内部のhttp.Clientを差し替えます。
// 差し替えたクライアントにはSSRFガードのDialContextが含まれない点に注意します。
func WithHTTPClient(c *http.Client) FetcherOption {
	return func(f *HTTPFetcher) { f.client = c }
}

// WithAllowPrivateAddresses プライベートやループバック宛の取得を許可します。
// SSRF対策を無効化するため本番では使わず、ローカルのE2Eなど信頼できる環境専用です。
func WithAllowPrivateAddresses(allow bool) FetcherOption {
	return func(f *HTTPFetcher) { f.allowPrivate = allow }
}

// NewHTTPFetcher 既定値とオプションからHTTPFetcherを構築します。
// 既定のクライアントは接続先IPを検査するDialContextを備え、リダイレクトのたびにスキームと接続先を再検証します。
func NewHTTPFetcher(opts ...FetcherOption) *HTTPFetcher {
	f := &HTTPFetcher{
		maxBytes:  defaultMaxBytes,
		timeout:   defaultTimeout,
		userAgent: defaultUserAgent,
	}
	transport := &http.Transport{
		DialContext:           f.dialContext,
		MaxIdleConns:          10,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
	}
	f.client = &http.Client{
		Transport: transport,
		CheckRedirect: func(req *http.Request, _ []*http.Request) error {
			if err := checkScheme(req.URL.String()); err != nil {
				return err
			}
			return nil
		},
	}
	for _, opt := range opts {
		opt(f)
	}
	return f
}

// dialContext 接続直前に解決済みIPを検査し、ブロック対象なら接続を拒否します。
// DNSリバインディングに対しても、実際に接続するアドレスを検査するため有効です。
// allowPrivateが真のときは検査を飛ばし、ローカルのE2Eなどでループバック宛を許可します。
func (f *HTTPFetcher) dialContext(ctx context.Context, network, address string) (net.Conn, error) {
	host, port, err := net.SplitHostPort(address)
	if err != nil {
		return nil, fmt.Errorf("failed to split host port %q: %w", address, err)
	}
	if !f.allowPrivate {
		ips, lookupErr := net.DefaultResolver.LookupNetIP(ctx, "ip", host)
		if lookupErr != nil {
			return nil, fmt.Errorf("failed to resolve host %q: %w", host, lookupErr)
		}
		for _, ip := range ips {
			if isBlockedAddr(ip) {
				return nil, fmt.Errorf("%w: host %q resolves to %s", ErrPrivateAddress, host, ip)
			}
		}
	}
	dialer := &net.Dialer{Timeout: 10 * time.Second, KeepAlive: 30 * time.Second}
	return dialer.DialContext(ctx, network, net.JoinHostPort(host, port))
}

// Fetch 条件付きヘッダを付けてURLを取得し、gzipを展開してサイズ上限内で本文を読み取ります。
// 304のときはNotModifiedを真にし本文を持ちません。SSRF対策は内部クライアントのDialContextが担います。
func (f *HTTPFetcher) Fetch(ctx context.Context, req port.FetchRequest) (port.FetchResult, error) {
	if err := checkScheme(req.URL); err != nil {
		return port.FetchResult{}, err
	}

	ctx, cancel := context.WithTimeout(ctx, f.timeout)
	defer cancel()

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, req.URL, nil)
	if err != nil {
		return port.FetchResult{}, fmt.Errorf("failed to build request for %q: %w", req.URL, err)
	}
	httpReq.Header.Set("User-Agent", f.userAgent)
	httpReq.Header.Set("Accept", "application/rss+xml, application/atom+xml, application/xml;q=0.9, text/xml;q=0.8, */*;q=0.1")
	httpReq.Header.Set("Accept-Encoding", "gzip")
	if req.ETag != "" {
		httpReq.Header.Set("If-None-Match", req.ETag)
	}
	if req.LastModified != "" {
		httpReq.Header.Set("If-Modified-Since", req.LastModified)
	}

	resp, err := f.client.Do(httpReq) //nolint:bodyclose // 直後のdefer obs.CloseAndLogでresp.Bodyを閉じています
	if err != nil {
		return port.FetchResult{}, fmt.Errorf("failed to fetch %q: %w", req.URL, err)
	}
	defer obs.CloseAndLog(f.logger, resp.Body, "fetch response body")

	result := port.FetchResult{
		StatusCode:   resp.StatusCode,
		ContentType:  resp.Header.Get("Content-Type"),
		ETag:         resp.Header.Get("ETag"),
		LastModified: resp.Header.Get("Last-Modified"),
	}

	if resp.StatusCode == http.StatusNotModified {
		result.NotModified = true
		return result, nil
	}

	body, err := f.readBody(resp)
	if err != nil {
		return port.FetchResult{}, fmt.Errorf("failed to read body of %q: %w", req.URL, err)
	}
	result.Body = body
	return result, nil
}

// readBody レスポンス本文をgzip展開しつつサイズ上限内で読み取ります。
// 上限を1バイトでも超える本文はエラーとして拒否します。
func (f *HTTPFetcher) readBody(resp *http.Response) ([]byte, error) {
	var reader io.Reader = resp.Body
	if strings.EqualFold(resp.Header.Get("Content-Encoding"), "gzip") {
		gz, err := gzip.NewReader(resp.Body)
		if err != nil {
			return nil, fmt.Errorf("failed to create gzip reader: %w", err)
		}
		defer obs.CloseAndLog(f.logger, gz, "gzip reader")
		reader = gz
	}

	limited := io.LimitReader(reader, f.maxBytes+1)
	body, err := io.ReadAll(limited)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}
	if int64(len(body)) > f.maxBytes {
		return nil, fmt.Errorf("feed: response body exceeds limit of %d bytes", f.maxBytes)
	}
	return body, nil
}
