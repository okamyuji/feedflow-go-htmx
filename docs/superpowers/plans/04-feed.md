# Phase3フィード取得とパース 実装計画

> For agentic workers: REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

Goal: internal/feedにHTTPフェッチャとフィードパーサと本文抽出と自動検出を実装し、port.Fetcherとport.FeedParserを満たします。条件付き取得とgzipとSSRF拒否を備えたフェッチャ、RSS2.0とAtomとRDFを判別するencoding/xmlパーサ、HTMLからのfeed link自動検出、golang.org/x/net/htmlによる本文抽出をそろえ、httptestとローカルサンプルで検証します。

Architecture: internal/feedはinternal/portのインターフェースを実装する具象層です。フェッチャはnet/httpのクライアントを保持し、SSRFガードとサイズ上限とタイムアウトを内側に持ちます。パーサはencoding/xmlでルート要素を判別してRSS2.0とAtomとRDFの各構造へデコードし、共通のport.ParsedFeedへ正規化します。discoverはHTMLのlink要素からフィードURLを抽出します。extractはgolang.org/x/net/htmlで本文要素を選び出す簡易リーダビリティです。各実装はコンストラクタ注入を受け、具象型でなくインターフェースの形で上位層へ渡されます。レスポンスボディのクローズはinternal/obsのCloseAndLog経由で記録します。

Tech Stack: Go(標準ライブラリのnet/httpとencoding/xmlとnet/netipとcompress/gzip)、golang.org/x/net/html(本文抽出と自動検出のHTMLパース)、net/http/httptest(テスト)。

前提: Phase0とPhase1が完了し`bash scripts/quality-gate.sh`が緑であることを確認してから始めます。作業ディレクトリは`/Users/yujiokamoto/devs/golang/feedflow-go-htmx`です。internal/port配下にFetcherとFeedParserとFetchRequestとFetchResultとParsedFeedとParsedItemとFeedFormatが定義済みであることを確認します。

参照する確定済みシグネチャは次のとおりです。これらは変更しません。

```go
// internal/port/fetcher.go
type FetchRequest struct {
	URL          string
	ETag         string
	LastModified string
}
type FetchResult struct {
	StatusCode   int
	NotModified  bool
	Body         []byte
	ContentType  string
	ETag         string
	LastModified string
}
type Fetcher interface {
	Fetch(ctx context.Context, req FetchRequest) (FetchResult, error)
}

// internal/port/parser.go
type FeedFormat string
const (
	FormatRSS2 FeedFormat = "rss2"
	FormatAtom FeedFormat = "atom"
	FormatRDF  FeedFormat = "rdf"
)
type ParsedItem struct {
	GUID        string
	Title       string
	Link        string
	Content     string
	Summary     string
	Author      string
	PublishedAt time.Time
}
type ParsedFeed struct {
	Format  FeedFormat
	Title   string
	SiteURL string
	Items   []ParsedItem
}
type FeedParser interface {
	Parse(data []byte) (ParsedFeed, error)
}
```

このフェーズで追加する補助型と補助関数は次のとおりで、いずれも確定済み定義と矛盾しません。

- internal/obsパッケージの `CloseAndLog(logger *slog.Logger, c io.Closer, context string)` と `WriteAndLog(logger *slog.Logger, w io.Writer, p []byte, context string) (int, error)`(Phase2の03-store.md Task1で作成済みの確定シグネチャです。Phase3では新規作成せず参照のみします)
- internal/feedの `HTTPFetcher` 構造体(port.Fetcherの実装)と `FetcherOption` 関数オプションと `ErrPrivateAddress` などの番兵エラー
- internal/feedの `XMLParser` 構造体(port.FeedParserの実装)
- internal/feedの `Discover(data []byte, baseURL string) ([]string, error)`(自動検出の純粋関数)
- internal/feedの `Extract(htmlBody []byte) (string, error)`(本文抽出の純粋関数)
- パース内部のみで使う非公開のxmlデコード用構造体(rss2Feed、atomFeed、rdfFeedなど)

---

## Task 1: obsパッケージの確定シグネチャを確認する

Files:
- 変更なし(internal/obs/obs.goはPhase2の03-store.md Task1で作成済みです)

internal/feedはHTTPレスポンスのボディとgzipリーダをクローズします。エラーを握り潰さずログへ残すため、Phase2の03-store.md Task1で作成済みのobs.CloseAndLogを使います。obsパッケージはここでは新規作成せず、確定シグネチャが存在することだけを確認します。00-overview.mdセクション3の依存順序ではPhase2がPhase3より先に着手されるため、Phase3の着手時点でobsパッケージは既に存在します。

確定シグネチャは次のとおりで、Phase3でもこの形のまま参照します。

```go
// internal/obs/obs.go(Phase2で作成済み)
func CloseAndLog(logger *slog.Logger, c io.Closer, context string)
func WriteAndLog(logger *slog.Logger, w io.Writer, p []byte, context string) (int, error)
```

CloseAndLogはloggerがnilのときslog.Defaultを使うため、HTTPFetcherはloggerを保持しつつnilでも安全に呼べます。Phase3ではレスポンスボディとgzipリーダのクローズでこのCloseAndLogをdeferから呼びます。

- [ ] Step 1: obsパッケージが存在することを確認する

Run:
```bash
cd /Users/yujiokamoto/devs/golang/feedflow-go-htmx && go build ./internal/obs/ && go doc ./internal/obs/ CloseAndLog
```
Expected: ビルドが成功し、`func CloseAndLog(logger *slog.Logger, c io.Closer, context string)` が表示されます。表示されない場合はPhase2の03-store.mdが未完了です。先にPhase2を完了させてからPhase3へ戻ります。

- [ ] Step 2: WriteAndLogの確定シグネチャを確認する

Run:
```bash
cd /Users/yujiokamoto/devs/golang/feedflow-go-htmx && go doc ./internal/obs/ WriteAndLog
```
Expected: `func WriteAndLog(logger *slog.Logger, w io.Writer, p []byte, context string) (int, error)` が表示されます。Phase3ではこのシグネチャを変更せず、CloseAndLogのみを利用します。

---

## Task 2: SSRFガードのアドレス判定

Files:
- Test: `/Users/yujiokamoto/devs/golang/feedflow-go-htmx/internal/feed/guard_test.go`
- Create: `/Users/yujiokamoto/devs/golang/feedflow-go-htmx/internal/feed/guard.go`

SSRF対策の中核として、解決済みIPがプライベートかループバックかリンクローカルか未指定かマルチキャストかを判定する関数と、URLのスキームをhttpとhttpsのみへ限定する関数を先に作ります。net/netipを使い文字列比較ではなくアドレス分類で判定します。これによりフェッチャ本体から判定ロジックを切り出してテーブル駆動で網羅できます。

- [ ] Step 1: 失敗するテストを書く

Create `internal/feed/guard_test.go`:
```go
package feed

import (
	"net/netip"
	"testing"
)

func TestIsBlockedAddr(t *testing.T) {
	tests := []struct {
		name string
		addr string
		want bool
	}{
		{name: "ループバックv4", addr: "127.0.0.1", want: true},
		{name: "ループバックv6", addr: "::1", want: true},
		{name: "プライベート10", addr: "10.0.0.1", want: true},
		{name: "プライベート172.16", addr: "172.16.5.4", want: true},
		{name: "プライベート192.168", addr: "192.168.1.1", want: true},
		{name: "リンクローカルv4", addr: "169.254.1.1", want: true},
		{name: "リンクローカルv6", addr: "fe80::1", want: true},
		{name: "ユニークローカルv6", addr: "fc00::1", want: true},
		{name: "未指定v4", addr: "0.0.0.0", want: true},
		{name: "未指定v6", addr: "::", want: true},
		{name: "マルチキャスト", addr: "224.0.0.1", want: true},
		{name: "クラウドメタデータ", addr: "169.254.169.254", want: true},
		{name: "グローバルv4", addr: "93.184.216.34", want: false},
		{name: "グローバルv6", addr: "2606:2800:220:1:248:1893:25c8:1946", want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			addr, err := netip.ParseAddr(tt.addr)
			if err != nil {
				t.Fatalf("ParseAddr(%q) returned error: %v", tt.addr, err)
			}
			if got := isBlockedAddr(addr); got != tt.want {
				t.Fatalf("isBlockedAddr(%q) got %v want %v", tt.addr, got, tt.want)
			}
		})
	}
}

func TestCheckScheme(t *testing.T) {
	tests := []struct {
		name    string
		raw     string
		wantErr bool
	}{
		{name: "httpsを許可", raw: "https://example.com/feed.xml", wantErr: false},
		{name: "httpを許可", raw: "http://example.com/feed.xml", wantErr: false},
		{name: "fileを拒否", raw: "file:///etc/passwd", wantErr: true},
		{name: "ftpを拒否", raw: "ftp://example.com/x", wantErr: true},
		{name: "gopherを拒否", raw: "gopher://example.com/", wantErr: true},
		{name: "スキーム無しを拒否", raw: "example.com/feed", wantErr: true},
		{name: "空文字を拒否", raw: "", wantErr: true},
		{name: "ホスト無しを拒否", raw: "https://", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := checkScheme(tt.raw)
			if tt.wantErr && err == nil {
				t.Fatalf("checkScheme(%q) errorを期待しましたがnilでした", tt.raw)
			}
			if !tt.wantErr && err != nil {
				t.Fatalf("checkScheme(%q) returned error: %v", tt.raw, err)
			}
		})
	}
}
```

- [ ] Step 2: テストが失敗することを確認する

Run:
```bash
cd /Users/yujiokamoto/devs/golang/feedflow-go-htmx && go test ./internal/feed/ -run 'TestIsBlockedAddr|TestCheckScheme' -v
```
Expected: コンパイルエラーで失敗します。`undefined: isBlockedAddr` と `undefined: checkScheme` が表示されます。

- [ ] Step 3: 最小実装を書く

Create `internal/feed/guard.go`:
```go
// Package feedフィードの取得とパースと本文抽出と自動検出を提供します。
// port.Fetcherとport.FeedParserを実装し、上位層へはインターフェースの形で渡します。
package feed

import (
	"errors"
	"fmt"
	"net/netip"
	"net/url"
	"strings"
)

// ErrPrivateAddress 解決先がプライベートやループバックなど到達を禁じるアドレスだったことを表します。
var ErrPrivateAddress = errors.New("feed: destination resolves to a blocked address")

// ErrDisallowedScheme URLのスキームがhttpとhttpsのいずれでもないことを表します。
var ErrDisallowedScheme = errors.New("feed: only http and https schemes are allowed")

// isBlockedAddr 与えられたIPアドレスがSSRF対策で到達を禁じる分類に該当するかを返します。
// ループバック、プライベート、リンクローカル、ユニークローカル、未指定、マルチキャスト、インターフェースローカルを拒否します。
func isBlockedAddr(addr netip.Addr) bool {
	if !addr.IsValid() {
		return true
	}
	unmapped := addr.Unmap()
	return unmapped.IsLoopback() ||
		unmapped.IsPrivate() ||
		unmapped.IsLinkLocalUnicast() ||
		unmapped.IsLinkLocalMulticast() ||
		unmapped.IsMulticast() ||
		unmapped.IsUnspecified() ||
		unmapped.IsInterfaceLocalMulticast()
}

// checkScheme 与えられたURL文字列がhttpかhttpsで、ホストを持つことを確認します。
// 条件を満たさない場合は文脈付きのエラーを返します。
func checkScheme(raw string) error {
	u, err := url.Parse(raw)
	if err != nil {
		return fmt.Errorf("failed to parse url %q: %w", raw, err)
	}
	scheme := strings.ToLower(u.Scheme)
	if scheme != "http" && scheme != "https" {
		return fmt.Errorf("%w: got %q", ErrDisallowedScheme, u.Scheme)
	}
	if u.Host == "" {
		return fmt.Errorf("%w: url %q has no host", ErrDisallowedScheme, raw)
	}
	return nil
}
```

- [ ] Step 4: テストが通ることを確認する

Run:
```bash
cd /Users/yujiokamoto/devs/golang/feedflow-go-htmx && go test ./internal/feed/ -run 'TestIsBlockedAddr|TestCheckScheme' -v
```
Expected: 各サブテストがPASSし、最後に `ok` になります。

- [ ] Step 5: gofmtを適用してコミットする

```bash
cd /Users/yujiokamoto/devs/golang/feedflow-go-htmx && gofmt -w internal/feed/guard.go internal/feed/guard_test.go && git add internal/feed/guard.go internal/feed/guard_test.go && git commit -m "feat: SSRFガードのアドレス分類とスキーム検証を追加する"
```

---

## Task 3: HTTPFetcherの構造体とコンストラクタとオプション

Files:
- Test: `/Users/yujiokamoto/devs/golang/feedflow-go-htmx/internal/feed/fetcher_ctor_test.go`
- Create: `/Users/yujiokamoto/devs/golang/feedflow-go-htmx/internal/feed/fetcher.go`

port.Fetcherを満たすHTTPFetcherの骨格を作ります。SSRFガードを効かせるため、DialContextでフックして接続先IPを検査するhttp.Clientを内蔵します。サイズ上限とタイムアウトとユーザーエージェントは関数オプションで上書きできるようにします。Fetchメソッドはこのタスクではまだ未実装相当の最小形(常にエラーを返さずゼロ値を返す形ではなく、コンパイルを通すための骨格)を置き、Task4で条件付き取得を実装します。

- [ ] Step 1: 失敗するテストを書く

Create `internal/feed/fetcher_ctor_test.go`:
```go
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
```

- [ ] Step 2: テストが失敗することを確認する

Run:
```bash
cd /Users/yujiokamoto/devs/golang/feedflow-go-htmx && go test ./internal/feed/ -run 'TestNewHTTPFetcher|TestHTTPFetcherSatisfiesPort' -v
```
Expected: コンパイルエラーで失敗します。`undefined: NewHTTPFetcher` などが表示されます。

- [ ] Step 3: 最小実装を書く

Create `internal/feed/fetcher.go`:
```go
package feed

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"net/netip"
	"time"

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
	client    *http.Client
	logger    *slog.Logger
	maxBytes  int64
	timeout   time.Duration
	userAgent string
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

// NewHTTPFetcher 既定値とオプションからHTTPFetcherを構築します。
// 既定のクライアントは接続先IPを検査するDialContextを備え、リダイレクトのたびにスキームと接続先を再検証します。
func NewHTTPFetcher(opts ...FetcherOption) *HTTPFetcher {
	f := &HTTPFetcher{
		maxBytes:  defaultMaxBytes,
		timeout:   defaultTimeout,
		userAgent: defaultUserAgent,
	}
	transport := &http.Transport{
		DialContext:           guardedDialContext,
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

// guardedDialContext 接続直前に解決済みIPを検査し、ブロック対象なら接続を拒否します。
// DNSリバインディングに対しても、実際に接続するアドレスを検査するため有効です。
func guardedDialContext(ctx context.Context, network, address string) (net.Conn, error) {
	host, port, err := net.SplitHostPort(address)
	if err != nil {
		return nil, fmt.Errorf("failed to split host port %q: %w", address, err)
	}
	ips, err := net.DefaultResolver.LookupNetIP(ctx, "ip", host)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve host %q: %w", host, err)
	}
	for _, ip := range ips {
		if isBlockedAddr(ip) {
			return nil, fmt.Errorf("%w: host %q resolves to %s", ErrPrivateAddress, host, ip)
		}
	}
	dialer := &net.Dialer{Timeout: 10 * time.Second, KeepAlive: 30 * time.Second}
	return dialer.DialContext(ctx, network, net.JoinHostPort(host, port))
}

// ensureAddrParsable netipの利用箇所をビルドへ確実に含めるための内部参照です。
// guard.goのisBlockedAddrがnetip.Addrを受けるため、ここでの参照は不要ですが、将来の直接利用に備えて型を固定します。
var _ = netip.Addr{}

// Fetch port.Fetcherを満たします。実装はTask4で条件付き取得とgzip展開を加えます。
func (f *HTTPFetcher) Fetch(_ context.Context, _ port.FetchRequest) (port.FetchResult, error) {
	return port.FetchResult{}, fmt.Errorf("feed: Fetch not implemented yet")
}
```

- [ ] Step 4: テストが通ることを確認する

Run:
```bash
cd /Users/yujiokamoto/devs/golang/feedflow-go-htmx && go test ./internal/feed/ -run 'TestNewHTTPFetcher|TestHTTPFetcherSatisfiesPort' -v
```
Expected: 3つのテストがPASSし、最後に `ok` になります。

- [ ] Step 5: gofmtを適用してコミットする

```bash
cd /Users/yujiokamoto/devs/golang/feedflow-go-htmx && gofmt -w internal/feed/fetcher.go internal/feed/fetcher_ctor_test.go && git add internal/feed/fetcher.go internal/feed/fetcher_ctor_test.go && git commit -m "feat: HTTPFetcherの骨格とオプションとSSRFガード付きダイヤラを追加する"
```

---

## Task 4: Fetchの条件付き取得とgzipとサイズ上限

Files:
- Test: `/Users/yujiokamoto/devs/golang/feedflow-go-htmx/internal/feed/fetcher_test.go`
- Modify: `/Users/yujiokamoto/devs/golang/feedflow-go-htmx/internal/feed/fetcher.go`

httptestでサーバを立て、Fetchが条件付きヘッダを送り、304で未更新を返し、200で本文とメタ情報を返し、gzipを展開し、サイズ上限を超える本文を拒否し、スキーム違反を拒否することを検証します。テストサーバはローカルループバックを使うため、guardedDialContextはサーバのアドレスを直接ダイヤルできるよう、ここではWithHTTPClientでテスト用クライアントを注入し、SSRFガードのDialContextを介さずに通します。SSRFガードそのものはTask5で専用に検証します。

- [ ] Step 1: 失敗するテストを書く

Create `internal/feed/fetcher_test.go`:
```go
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
	base := []FetcherOption{WithHTTPClient(&http.Client{})}
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
```

- [ ] Step 2: テストが失敗することを確認する

Run:
```bash
cd /Users/yujiokamoto/devs/golang/feedflow-go-htmx && go test ./internal/feed/ -run TestFetch -v
```
Expected: `feed: Fetch not implemented yet` で各テストがFAILします。

- [ ] Step 3: Fetchを実装する

`internal/feed/fetcher.go` のimportを次へ置き換えます。

old:
```go
import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"net/netip"
	"time"

	"github.com/okamyuji/feedflow-go-htmx/internal/port"
)
```

new:
```go
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
```

`internal/feed/fetcher.go` の不要になったnetipダミー参照を削除します。

old:
```go
// ensureAddrParsable netipの利用箇所をビルドへ確実に含めるための内部参照です。
// guard.goのisBlockedAddrがnetip.Addrを受けるため、ここでの参照は不要ですが、将来の直接利用に備えて型を固定します。
var _ = netip.Addr{}

// Fetch port.Fetcherを満たします。実装はTask4で条件付き取得とgzip展開を加えます。
func (f *HTTPFetcher) Fetch(_ context.Context, _ port.FetchRequest) (port.FetchResult, error) {
	return port.FetchResult{}, fmt.Errorf("feed: Fetch not implemented yet")
}
```

new:
```go
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

	resp, err := f.client.Do(httpReq)
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
```

補足: `net` パッケージはguardedDialContextで引き続き使うためimportに残します。

- [ ] Step 4: テストが通ることを確認する

Run:
```bash
cd /Users/yujiokamoto/devs/golang/feedflow-go-htmx && go test ./internal/feed/ -run TestFetch -race -v
```
Expected: TestFetchOKとTestFetchConditionalHeadersとTestFetchGzipとTestFetchSizeLimitとTestFetchRejectsBadSchemeとTestFetchServerErrorがPASSし、最後に `ok` になります。

- [ ] Step 5: gofmtを適用してコミットする

```bash
cd /Users/yujiokamoto/devs/golang/feedflow-go-htmx && gofmt -w internal/feed/fetcher.go internal/feed/fetcher_test.go && git add internal/feed/fetcher.go internal/feed/fetcher_test.go && git commit -m "feat: Fetchに条件付き取得とgzip展開とサイズ上限を実装する"
```

---

## Task 5: SSRFガードの結合検証

Files:
- Test: `/Users/yujiokamoto/devs/golang/feedflow-go-htmx/internal/feed/fetcher_ssrf_test.go`
- 変更なし(実装はTask3とTask4で完了)

既定のNewHTTPFetcherが内蔵するSSRFガードが、ループバックや169.254.169.254などへの取得を実際に拒否することをhttptestで確認します。テストサーバはループバックで待ち受けるため、ガード付きの既定クライアントではErrPrivateAddressで拒否されることを期待します。

- [ ] Step 1: 失敗するテストを書く

Create `internal/feed/fetcher_ssrf_test.go`:
```go
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
```

- [ ] Step 2: テストが通ることを確認する

Run:
```bash
cd /Users/yujiokamoto/devs/golang/feedflow-go-htmx && go test ./internal/feed/ -run 'TestFetchBlocks' -race -v
```
Expected: TestFetchBlocksLoopbackとTestFetchBlocksMetadataIPがPASSします。実装はTask3とTask4で済んでいるため、ここはガードの結合検証であり最初から緑になります。

- [ ] Step 3: gofmtを適用してコミットする

```bash
cd /Users/yujiokamoto/devs/golang/feedflow-go-htmx && gofmt -w internal/feed/fetcher_ssrf_test.go && git add internal/feed/fetcher_ssrf_test.go && git commit -m "test: SSRFガードがループバックとメタデータIPを拒否することを検証する"
```

---

## Task 6: フィード形式の判別

Files:
- Test: `/Users/yujiokamoto/devs/golang/feedflow-go-htmx/internal/feed/detect_test.go`
- Create: `/Users/yujiokamoto/devs/golang/feedflow-go-htmx/internal/feed/detect.go`

パース本体に入る前に、バイト列のルート要素からRSS2.0とAtomとRDFを判別する関数を作ります。encoding/xmlのDecoderでトークンを先頭から走査し、最初に現れる開始要素のローカル名と名前空間で判別します。BOMや先行する空白や処理命令やコメントを飛ばして堅牢に判別します。判別はルート要素だけを見れば足りるため、途中で切れた不完全なXMLでもルート要素さえ読めれば形式を返します。本文が壊れているかどうかの検証は、後段のParseで全体をデコードするときに担います。BOMはソースに生バイトで埋め込むとgofmtがillegal byte order markで弾くため、文字列リテラルでは必ずエスケープ表記の\ufeffを使います。

- [ ] Step 1: 失敗するテストを書く

Create `internal/feed/detect_test.go`:
```go
package feed

import (
	"testing"

	"github.com/okamyuji/feedflow-go-htmx/internal/port"
)

func TestDetectFormat(t *testing.T) {
	tests := []struct {
		name    string
		data    string
		want    port.FeedFormat
		wantErr bool
	}{
		{
			name: "RSS2.0",
			data: `<?xml version="1.0"?><rss version="2.0"><channel><title>x</title></channel></rss>`,
			want: port.FormatRSS2,
		},
		{
			name: "Atom",
			data: `<?xml version="1.0" encoding="utf-8"?><feed xmlns="http://www.w3.org/2005/Atom"><title>x</title></feed>`,
			want: port.FormatAtom,
		},
		{
			name: "RDFつまりRSS1.0",
			data: `<?xml version="1.0"?><rdf:RDF xmlns:rdf="http://www.w3.org/1999/02/22-rdf-syntax-ns#" xmlns="http://purl.org/rss/1.0/"><channel><title>x</title></channel></rdf:RDF>`,
			want: port.FormatRDF,
		},
		{
			name: "先頭にBOMと空白",
			data: "\ufeff   \n<rss version=\"2.0\"><channel></channel></rss>",
			want: port.FormatRSS2,
		},
		{
			name: "先頭にコメント",
			data: `<!-- generated --><feed xmlns="http://www.w3.org/2005/Atom"></feed>`,
			want: port.FormatAtom,
		},
		{
			name:    "未知のルート",
			data:    `<html><body>not a feed</body></html>`,
			wantErr: true,
		},
		{
			name:    "空入力",
			data:    ``,
			wantErr: true,
		},
		{
			name:    "開始要素が現れない断片",
			data:    `   <!-- comment only -->`,
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := detectFormat([]byte(tt.data))
			if tt.wantErr {
				if err == nil {
					t.Fatalf("errorを期待しましたがnilでした, got=%q", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("detectFormat returned error: %v", err)
			}
			if got != tt.want {
				t.Fatalf("detectFormat got %q want %q", got, tt.want)
			}
		})
	}
}
```

- [ ] Step 2: テストが失敗することを確認する

Run:
```bash
cd /Users/yujiokamoto/devs/golang/feedflow-go-htmx && go test ./internal/feed/ -run TestDetectFormat -v
```
Expected: コンパイルエラーで失敗します。`undefined: detectFormat` が表示されます。

- [ ] Step 3: 最小実装を書く

Create `internal/feed/detect.go`:
```go
package feed

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"io"
	"strings"

	"github.com/okamyuji/feedflow-go-htmx/internal/port"
)

// rdfNamespace RDFつまりRSS 1.0の名前空間です。
const rdfNamespace = "http://www.w3.org/1999/02/22-rdf-syntax-ns#"

// atomNamespace Atomの名前空間です。
const atomNamespace = "http://www.w3.org/2005/Atom"

// detectFormat バイト列の最初の開始要素からフィード形式を判別します。
// BOMや空白や処理命令やコメントは読み飛ばし、最初の開始要素のローカル名と名前空間で判別します。
func detectFormat(data []byte) (port.FeedFormat, error) {
	trimmed := bytes.TrimLeft(data, "\ufeff \t\r\n")
	if len(trimmed) == 0 {
		return "", fmt.Errorf("feed: empty input")
	}
	dec := xml.NewDecoder(bytes.NewReader(trimmed))
	for {
		tok, err := dec.Token()
		if err == io.EOF {
			return "", fmt.Errorf("feed: no start element found")
		}
		if err != nil {
			return "", fmt.Errorf("failed to tokenize xml: %w", err)
		}
		start, ok := tok.(xml.StartElement)
		if !ok {
			continue
		}
		local := strings.ToLower(start.Name.Local)
		space := start.Name.Space
		switch {
		case local == "rss":
			return port.FormatRSS2, nil
		case local == "feed" && (space == atomNamespace || space == ""):
			return port.FormatAtom, nil
		case local == "rdf" && space == rdfNamespace:
			return port.FormatRDF, nil
		default:
			return "", fmt.Errorf("feed: unrecognized root element %q (ns=%q)", start.Name.Local, space)
		}
	}
}
```

- [ ] Step 4: テストが通ることを確認する

Run:
```bash
cd /Users/yujiokamoto/devs/golang/feedflow-go-htmx && go test ./internal/feed/ -run TestDetectFormat -v
```
Expected: 各サブテストがPASSし、最後に `ok` になります。

- [ ] Step 5: gofmtを適用してコミットする

```bash
cd /Users/yujiokamoto/devs/golang/feedflow-go-htmx && gofmt -w internal/feed/detect.go internal/feed/detect_test.go && git add internal/feed/detect.go internal/feed/detect_test.go && git commit -m "feat: フィード形式の判別detectFormatを追加する"
```

---

## Task 7: XMLParserとコンストラクタとRSS2.0パース

Files:
- Test: `/Users/yujiokamoto/devs/golang/feedflow-go-htmx/internal/feed/parser_rss2_test.go`
- Create: `/Users/yujiokamoto/devs/golang/feedflow-go-htmx/internal/feed/parser.go`

port.FeedParserを満たすXMLParserを作ります。ParseはまずdetectFormatで形式を判別し、RSS2.0の場合にencoding/xmlでデコードしてport.ParsedFeedへ正規化します。公開日時はRFC1123ZとRFC1123とRFC822ZとRFC822の各レイアウトを順に試して解析します。GUIDが空の場合はLinkで代替します。このタスクのswitchはRSS2.0だけを扱い、未対応の形式はまとめてエラーを返します。AtomとRDFのcaseはTask8とTask9でswitchへ追加するため、このタスクでは未実装スタブを置きません。各タスクはこの形でビルドが通り単独で完結します。

- [ ] Step 1: 失敗するテストを書く

Create `internal/feed/parser_rss2_test.go`:
```go
package feed

import (
	"testing"
	"time"

	"github.com/okamyuji/feedflow-go-htmx/internal/port"
)

const rss2Sample = `<?xml version="1.0" encoding="UTF-8"?>
<rss version="2.0">
  <channel>
    <title>Example RSS</title>
    <link>https://example.com/</link>
    <description>An example feed</description>
    <item>
      <title>First Post</title>
      <link>https://example.com/first</link>
      <guid>https://example.com/first</guid>
      <description>summary one</description>
      <author>alice@example.com</author>
      <pubDate>Mon, 02 Jan 2006 15:04:05 -0700</pubDate>
    </item>
    <item>
      <title>Second Post</title>
      <link>https://example.com/second</link>
      <description>summary two</description>
      <pubDate>Tue, 03 Jan 2006 10:00:00 GMT</pubDate>
    </item>
  </channel>
</rss>`

func TestParseRSS2(t *testing.T) {
	p := NewXMLParser()
	got, err := p.Parse([]byte(rss2Sample))
	if err != nil {
		t.Fatalf("Parse returned error: %v", err)
	}
	if got.Format != port.FormatRSS2 {
		t.Fatalf("Format got %q want %q", got.Format, port.FormatRSS2)
	}
	if got.Title != "Example RSS" {
		t.Fatalf("Title got %q", got.Title)
	}
	if got.SiteURL != "https://example.com/" {
		t.Fatalf("SiteURL got %q", got.SiteURL)
	}
	if len(got.Items) != 2 {
		t.Fatalf("Items len got %d want 2", len(got.Items))
	}

	first := got.Items[0]
	if first.Title != "First Post" {
		t.Fatalf("first Title got %q", first.Title)
	}
	if first.Link != "https://example.com/first" {
		t.Fatalf("first Link got %q", first.Link)
	}
	if first.GUID != "https://example.com/first" {
		t.Fatalf("first GUID got %q", first.GUID)
	}
	if first.Summary != "summary one" {
		t.Fatalf("first Summary got %q", first.Summary)
	}
	if first.Author != "alice@example.com" {
		t.Fatalf("first Author got %q", first.Author)
	}
	wantTime := time.Date(2006, 1, 2, 15, 4, 5, 0, time.FixedZone("", -7*3600))
	if !first.PublishedAt.Equal(wantTime) {
		t.Fatalf("first PublishedAt got %v want %v", first.PublishedAt, wantTime)
	}

	second := got.Items[1]
	if second.GUID != "https://example.com/second" {
		t.Fatalf("GUID欠落時はLinkで代替するはずですがgot %q", second.GUID)
	}
}

func TestParseRSS2Invalid(t *testing.T) {
	p := NewXMLParser()
	_, err := p.Parse([]byte(`<html></html>`))
	if err == nil {
		t.Fatal("非フィード入力でエラーを期待しましたがnilでした")
	}
}

func TestParseRSS2BrokenXML(t *testing.T) {
	p := NewXMLParser()
	_, err := p.Parse([]byte(`<rss><channel`))
	if err == nil {
		t.Fatal("途中で切れたXMLでエラーを期待しましたがnilでした")
	}
}

// TestXMLParserSatisfiesPort XMLParserがport.FeedParserを満たすことを検証します。
func TestXMLParserSatisfiesPort(t *testing.T) {
	var _ port.FeedParser = NewXMLParser()
}
```

- [ ] Step 2: テストが失敗することを確認する

Run:
```bash
cd /Users/yujiokamoto/devs/golang/feedflow-go-htmx && go test ./internal/feed/ -run 'TestParseRSS2|TestXMLParserSatisfiesPort' -v
```
Expected: コンパイルエラーで失敗します。`undefined: NewXMLParser` が表示されます。

- [ ] Step 3: 最小実装を書く

Create `internal/feed/parser.go`:
```go
package feed

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"strings"
	"time"

	"github.com/okamyuji/feedflow-go-htmx/internal/port"
)

// XMLParser encoding/xmlを用いたport.FeedParserの実装です。
// RSS2.0とAtomとRDFを判別してパースしport.ParsedFeedへ正規化します。
type XMLParser struct{}

// NewXMLParser XMLParserを構築します。状態を持たないため設定はありません。
func NewXMLParser() *XMLParser {
	return &XMLParser{}
}

// Parse バイト列の形式を判別し、対応するパーサでport.ParsedFeedを返します。
// このタスクではRSS2.0のみを扱い、AtomとRDFのcaseはTask8とTask9で追加します。
func (p *XMLParser) Parse(data []byte) (port.ParsedFeed, error) {
	format, err := detectFormat(data)
	if err != nil {
		return port.ParsedFeed{}, fmt.Errorf("failed to detect feed format: %w", err)
	}
	switch format {
	case port.FormatRSS2:
		return parseRSS2(data)
	default:
		return port.ParsedFeed{}, fmt.Errorf("feed: unsupported format %q", format)
	}
}

// rss2Document RSS2.0のデコード用構造体です。
type rss2Document struct {
	XMLName xml.Name    `xml:"rss"`
	Channel rss2Channel `xml:"channel"`
}

type rss2Channel struct {
	Title string     `xml:"title"`
	Link  string     `xml:"link"`
	Items []rss2Item `xml:"item"`
}

type rss2Item struct {
	Title       string `xml:"title"`
	Link        string `xml:"link"`
	GUID        string `xml:"guid"`
	Description string `xml:"description"`
	Encoded     string `xml:"http://purl.org/rss/1.0/modules/content/ encoded"`
	Author      string `xml:"author"`
	Creator     string `xml:"http://purl.org/dc/elements/1.1/ creator"`
	PubDate     string `xml:"pubDate"`
	DCDate      string `xml:"http://purl.org/dc/elements/1.1/ date"`
}

// parseRSS2 RSS 2.0のバイト列をport.ParsedFeedへ正規化します。
func parseRSS2(data []byte) (port.ParsedFeed, error) {
	var doc rss2Document
	dec := xml.NewDecoder(bytes.NewReader(data))
	dec.Strict = false
	if err := dec.Decode(&doc); err != nil {
		return port.ParsedFeed{}, fmt.Errorf("failed to decode rss2: %w", err)
	}
	feed := port.ParsedFeed{
		Format:  port.FormatRSS2,
		Title:   strings.TrimSpace(doc.Channel.Title),
		SiteURL: strings.TrimSpace(doc.Channel.Link),
		Items:   make([]port.ParsedItem, 0, len(doc.Channel.Items)),
	}
	for _, it := range doc.Channel.Items {
		link := strings.TrimSpace(it.Link)
		guid := strings.TrimSpace(it.GUID)
		if guid == "" {
			guid = link
		}
		author := strings.TrimSpace(it.Author)
		if author == "" {
			author = strings.TrimSpace(it.Creator)
		}
		content := strings.TrimSpace(it.Encoded)
		if content == "" {
			content = strings.TrimSpace(it.Description)
		}
		dateStr := it.PubDate
		if strings.TrimSpace(dateStr) == "" {
			dateStr = it.DCDate
		}
		feed.Items = append(feed.Items, port.ParsedItem{
			GUID:        guid,
			Title:       strings.TrimSpace(it.Title),
			Link:        link,
			Content:     content,
			Summary:     strings.TrimSpace(it.Description),
			Author:      author,
			PublishedAt: parseTime(dateStr),
		})
	}
	return feed, nil
}

// timeLayouts 公開日時の解析で順に試すレイアウトです。
var timeLayouts = []string{
	time.RFC1123Z,
	time.RFC1123,
	time.RFC822Z,
	time.RFC822,
	time.RFC3339,
	"Mon, 2 Jan 2006 15:04:05 -0700",
	"2006-01-02T15:04:05Z07:00",
	"2006-01-02 15:04:05",
	"2006-01-02",
}

// parseTime 与えられた日時文字列を既知のレイアウトで順に解析します。
// いずれにも一致しない場合はゼロ値のtime.Timeを返します。
func parseTime(s string) time.Time {
	s = strings.TrimSpace(s)
	if s == "" {
		return time.Time{}
	}
	for _, layout := range timeLayouts {
		if t, err := time.Parse(layout, s); err == nil {
			return t
		}
	}
	return time.Time{}
}
```

補足: このタスクのswitchはRSS2.0だけを扱うため、parseAtomやparseRDFを参照せずビルドが通ります。AtomとRDFのcaseはTask8とTask9でswitchへ追加し、未実装スタブは置きません。各タスクはこの形で単独でビルドが通り完結します。

- [ ] Step 4: テストが通ることを確認する

Run:
```bash
cd /Users/yujiokamoto/devs/golang/feedflow-go-htmx && go test ./internal/feed/ -run 'TestParseRSS2|TestXMLParserSatisfiesPort' -v
```
Expected: TestParseRSS2とTestParseRSS2InvalidとTestParseRSS2BrokenXMLとTestXMLParserSatisfiesPortがPASSし、最後に `ok` になります。

- [ ] Step 5: gofmtを適用してコミットする

```bash
cd /Users/yujiokamoto/devs/golang/feedflow-go-htmx && gofmt -w internal/feed/parser.go internal/feed/parser_rss2_test.go && git add internal/feed/parser.go internal/feed/parser_rss2_test.go && git commit -m "feat: XMLParserとRSS2.0パースと日時解析を追加する"
```

---

## Task 8: Atomパース

Files:
- Test: `/Users/yujiokamoto/devs/golang/feedflow-go-htmx/internal/feed/parser_atom_test.go`
- Modify: `/Users/yujiokamoto/devs/golang/feedflow-go-htmx/internal/feed/parser.go`

Atomの構造をデコードしてport.ParsedFeedへ正規化します。linkは複数あり、rel属性がalternateまたは未指定のhrefを記事リンクとして採用します。idをGUIDに、summaryをSummaryに、contentをContentにマップします。著者はauthorのnameを使います。日時はupdatedまたはpublishedを使います。

- [ ] Step 1: 失敗するテストを書く

Create `internal/feed/parser_atom_test.go`:
```go
package feed

import (
	"testing"
	"time"

	"github.com/okamyuji/feedflow-go-htmx/internal/port"
)

const atomSample = `<?xml version="1.0" encoding="utf-8"?>
<feed xmlns="http://www.w3.org/2005/Atom">
  <title>Example Atom</title>
  <link href="https://example.com/" rel="alternate"/>
  <link href="https://example.com/feed.xml" rel="self"/>
  <updated>2026-01-02T15:04:05Z</updated>
  <entry>
    <title>Atom Entry One</title>
    <id>urn:uuid:1</id>
    <link href="https://example.com/atom-one" rel="alternate"/>
    <link href="https://example.com/atom-one/comments" rel="replies"/>
    <summary>atom summary</summary>
    <content type="html">&lt;p&gt;atom body&lt;/p&gt;</content>
    <author><name>Bob</name></author>
    <published>2026-01-01T00:00:00Z</published>
    <updated>2026-01-02T12:00:00Z</updated>
  </entry>
  <entry>
    <title>Atom Entry Two</title>
    <id>urn:uuid:2</id>
    <link href="https://example.com/atom-two"/>
    <updated>2026-01-03T00:00:00Z</updated>
  </entry>
</feed>`

func TestParseAtom(t *testing.T) {
	p := NewXMLParser()
	got, err := p.Parse([]byte(atomSample))
	if err != nil {
		t.Fatalf("Parse returned error: %v", err)
	}
	if got.Format != port.FormatAtom {
		t.Fatalf("Format got %q want %q", got.Format, port.FormatAtom)
	}
	if got.Title != "Example Atom" {
		t.Fatalf("Title got %q", got.Title)
	}
	if got.SiteURL != "https://example.com/" {
		t.Fatalf("SiteURL got %q want alternate link", got.SiteURL)
	}
	if len(got.Items) != 2 {
		t.Fatalf("Items len got %d want 2", len(got.Items))
	}

	one := got.Items[0]
	if one.Title != "Atom Entry One" {
		t.Fatalf("one Title got %q", one.Title)
	}
	if one.GUID != "urn:uuid:1" {
		t.Fatalf("one GUID got %q", one.GUID)
	}
	if one.Link != "https://example.com/atom-one" {
		t.Fatalf("one Link got %q want alternate link", one.Link)
	}
	if one.Summary != "atom summary" {
		t.Fatalf("one Summary got %q", one.Summary)
	}
	if one.Content != "<p>atom body</p>" {
		t.Fatalf("one Content got %q", one.Content)
	}
	if one.Author != "Bob" {
		t.Fatalf("one Author got %q", one.Author)
	}
	wantTime := time.Date(2026, 1, 2, 12, 0, 0, 0, time.UTC)
	if !one.PublishedAt.Equal(wantTime) {
		t.Fatalf("one PublishedAt got %v want updated %v", one.PublishedAt, wantTime)
	}

	two := got.Items[1]
	if two.Link != "https://example.com/atom-two" {
		t.Fatalf("two Link got %q want rel未指定のhref", two.Link)
	}
}
```

- [ ] Step 2: テストが失敗することを確認する

Run:
```bash
cd /Users/yujiokamoto/devs/golang/feedflow-go-htmx && go test ./internal/feed/ -run TestParseAtom -v
```
Expected: `feed: unsupported format "atom"` でFAILします。このタスクのStep3でParseのswitchにAtomのcaseとparseAtomを追加して通します。

- [ ] Step 3: ParseにAtomのcaseを追加しparseAtomを実装する

まず`internal/feed/parser.go`のParseのswitchへAtomのcaseを追加します。

old:
```go
	switch format {
	case port.FormatRSS2:
		return parseRSS2(data)
	default:
		return port.ParsedFeed{}, fmt.Errorf("feed: unsupported format %q", format)
	}
```

new:
```go
	switch format {
	case port.FormatRSS2:
		return parseRSS2(data)
	case port.FormatAtom:
		return parseAtom(data)
	default:
		return port.ParsedFeed{}, fmt.Errorf("feed: unsupported format %q", format)
	}
```

次に`internal/feed/parser.go`の末尾へAtomのデコード用構造体とparseAtomを追記します。

追記する内容:
```go
// atomDocument Atomのデコード用構造体です。
type atomDocument struct {
	XMLName xml.Name    `xml:"feed"`
	Title   string      `xml:"title"`
	Links   []atomLink  `xml:"link"`
	Entries []atomEntry `xml:"entry"`
}

type atomLink struct {
	Href string `xml:"href,attr"`
	Rel  string `xml:"rel,attr"`
}

type atomEntry struct {
	Title     string     `xml:"title"`
	ID        string     `xml:"id"`
	Links     []atomLink `xml:"link"`
	Summary   string     `xml:"summary"`
	Content   string     `xml:"content"`
	Author    atomAuthor `xml:"author"`
	Published string     `xml:"published"`
	Updated   string     `xml:"updated"`
}

type atomAuthor struct {
	Name string `xml:"name"`
}

// pickAtomLink relがalternateまたは未指定のhrefを優先して返します。
// 該当が無い場合は最初のhrefを返します。
func pickAtomLink(links []atomLink) string {
	for _, l := range links {
		rel := strings.ToLower(strings.TrimSpace(l.Rel))
		if rel == "alternate" || rel == "" {
			if strings.TrimSpace(l.Href) != "" {
				return strings.TrimSpace(l.Href)
			}
		}
	}
	for _, l := range links {
		if strings.TrimSpace(l.Href) != "" {
			return strings.TrimSpace(l.Href)
		}
	}
	return ""
}

// parseAtom Atomのバイト列をport.ParsedFeedへ正規化します。
func parseAtom(data []byte) (port.ParsedFeed, error) {
	var doc atomDocument
	dec := xml.NewDecoder(bytes.NewReader(data))
	dec.Strict = false
	if err := dec.Decode(&doc); err != nil {
		return port.ParsedFeed{}, fmt.Errorf("failed to decode atom: %w", err)
	}
	feed := port.ParsedFeed{
		Format:  port.FormatAtom,
		Title:   strings.TrimSpace(doc.Title),
		SiteURL: pickAtomLink(doc.Links),
		Items:   make([]port.ParsedItem, 0, len(doc.Entries)),
	}
	for _, e := range doc.Entries {
		summary := strings.TrimSpace(e.Summary)
		content := strings.TrimSpace(e.Content)
		if content == "" {
			content = summary
		}
		dateStr := e.Updated
		if strings.TrimSpace(dateStr) == "" {
			dateStr = e.Published
		}
		feed.Items = append(feed.Items, port.ParsedItem{
			GUID:        strings.TrimSpace(e.ID),
			Title:       strings.TrimSpace(e.Title),
			Link:        pickAtomLink(e.Links),
			Content:     content,
			Summary:     summary,
			Author:      strings.TrimSpace(e.Author.Name),
			PublishedAt: parseTime(dateStr),
		})
	}
	return feed, nil
}
```

- [ ] Step 4: テストが通ることを確認する

Run:
```bash
cd /Users/yujiokamoto/devs/golang/feedflow-go-htmx && go test ./internal/feed/ -run TestParseAtom -v
```
Expected: TestParseAtomがPASSし、最後に `ok` になります。

- [ ] Step 5: gofmtを適用してコミットする

```bash
cd /Users/yujiokamoto/devs/golang/feedflow-go-htmx && gofmt -w internal/feed/parser.go internal/feed/parser_atom_test.go && git add internal/feed/parser.go internal/feed/parser_atom_test.go && git commit -m "feat: Atomパースを実装する"
```

---

## Task 9: RDFつまりRSS1.0パース

Files:
- Test: `/Users/yujiokamoto/devs/golang/feedflow-go-htmx/internal/feed/parser_rdf_test.go`
- Modify: `/Users/yujiokamoto/devs/golang/feedflow-go-htmx/internal/feed/parser.go`

RDFはchannelとitemがrdf:RDFの直下に並列に並びます。channelのtitleとlinkをフィードのメタ情報に、各itemのtitleとlinkとdescriptionをマップします。著者と日時はDublin Coreのcreatorとdateを使います。GUIDはrdf:aboutまたはlinkを使います。

- [ ] Step 1: 失敗するテストを書く

Create `internal/feed/parser_rdf_test.go`:
```go
package feed

import (
	"testing"
	"time"

	"github.com/okamyuji/feedflow-go-htmx/internal/port"
)

const rdfSample = `<?xml version="1.0" encoding="UTF-8"?>
<rdf:RDF
  xmlns:rdf="http://www.w3.org/1999/02/22-rdf-syntax-ns#"
  xmlns:dc="http://purl.org/dc/elements/1.1/"
  xmlns="http://purl.org/rss/1.0/">
  <channel rdf:about="https://example.com/rss">
    <title>Example RDF</title>
    <link>https://example.com/</link>
    <description>RDF feed</description>
  </channel>
  <item rdf:about="https://example.com/rdf-one">
    <title>RDF Item One</title>
    <link>https://example.com/rdf-one</link>
    <description>rdf summary one</description>
    <dc:creator>Carol</dc:creator>
    <dc:date>2026-02-01T09:30:00Z</dc:date>
  </item>
  <item>
    <title>RDF Item Two</title>
    <link>https://example.com/rdf-two</link>
  </item>
</rdf:RDF>`

func TestParseRDF(t *testing.T) {
	p := NewXMLParser()
	got, err := p.Parse([]byte(rdfSample))
	if err != nil {
		t.Fatalf("Parse returned error: %v", err)
	}
	if got.Format != port.FormatRDF {
		t.Fatalf("Format got %q want %q", got.Format, port.FormatRDF)
	}
	if got.Title != "Example RDF" {
		t.Fatalf("Title got %q", got.Title)
	}
	if got.SiteURL != "https://example.com/" {
		t.Fatalf("SiteURL got %q", got.SiteURL)
	}
	if len(got.Items) != 2 {
		t.Fatalf("Items len got %d want 2", len(got.Items))
	}

	one := got.Items[0]
	if one.Title != "RDF Item One" {
		t.Fatalf("one Title got %q", one.Title)
	}
	if one.GUID != "https://example.com/rdf-one" {
		t.Fatalf("one GUID got %q want rdf:about", one.GUID)
	}
	if one.Link != "https://example.com/rdf-one" {
		t.Fatalf("one Link got %q", one.Link)
	}
	if one.Summary != "rdf summary one" {
		t.Fatalf("one Summary got %q", one.Summary)
	}
	if one.Author != "Carol" {
		t.Fatalf("one Author got %q want dc:creator", one.Author)
	}
	wantTime := time.Date(2026, 2, 1, 9, 30, 0, 0, time.UTC)
	if !one.PublishedAt.Equal(wantTime) {
		t.Fatalf("one PublishedAt got %v want %v", one.PublishedAt, wantTime)
	}

	two := got.Items[1]
	if two.GUID != "https://example.com/rdf-two" {
		t.Fatalf("rdf:about欠落時はlinkで代替するはずですがgot %q", two.GUID)
	}
}
```

- [ ] Step 2: テストが失敗することを確認する

Run:
```bash
cd /Users/yujiokamoto/devs/golang/feedflow-go-htmx && go test ./internal/feed/ -run TestParseRDF -v
```
Expected: `feed: unsupported format "rdf"` でFAILします。このタスクのStep3でParseのswitchにRDFのcaseとparseRDFを追加して通します。

- [ ] Step 3: ParseにRDFのcaseを追加しparseRDFを実装する

まず`internal/feed/parser.go`のParseのswitchへRDFのcaseを追加します。

old:
```go
	switch format {
	case port.FormatRSS2:
		return parseRSS2(data)
	case port.FormatAtom:
		return parseAtom(data)
	default:
		return port.ParsedFeed{}, fmt.Errorf("feed: unsupported format %q", format)
	}
```

new:
```go
	switch format {
	case port.FormatRSS2:
		return parseRSS2(data)
	case port.FormatAtom:
		return parseAtom(data)
	case port.FormatRDF:
		return parseRDF(data)
	default:
		return port.ParsedFeed{}, fmt.Errorf("feed: unsupported format %q", format)
	}
```

次に`internal/feed/parser.go`の末尾へRDFのデコード用構造体とparseRDFを追記します。

追記する内容:
```go
// rdfDocument RDFつまりRSS1.0のデコード用構造体です。
type rdfDocument struct {
	XMLName xml.Name   `xml:"RDF"`
	Channel rdfChannel `xml:"channel"`
	Items   []rdfItem  `xml:"item"`
}

type rdfChannel struct {
	Title string `xml:"title"`
	Link  string `xml:"link"`
}

type rdfItem struct {
	About       string `xml:"http://www.w3.org/1999/02/22-rdf-syntax-ns# about,attr"`
	Title       string `xml:"title"`
	Link        string `xml:"link"`
	Description string `xml:"description"`
	Creator     string `xml:"http://purl.org/dc/elements/1.1/ creator"`
	Date        string `xml:"http://purl.org/dc/elements/1.1/ date"`
}

// parseRDF RDFつまりRSS1.0のバイト列をport.ParsedFeedへ正規化します。
func parseRDF(data []byte) (port.ParsedFeed, error) {
	var doc rdfDocument
	dec := xml.NewDecoder(bytes.NewReader(data))
	dec.Strict = false
	if err := dec.Decode(&doc); err != nil {
		return port.ParsedFeed{}, fmt.Errorf("failed to decode rdf: %w", err)
	}
	feed := port.ParsedFeed{
		Format:  port.FormatRDF,
		Title:   strings.TrimSpace(doc.Channel.Title),
		SiteURL: strings.TrimSpace(doc.Channel.Link),
		Items:   make([]port.ParsedItem, 0, len(doc.Items)),
	}
	for _, it := range doc.Items {
		link := strings.TrimSpace(it.Link)
		guid := strings.TrimSpace(it.About)
		if guid == "" {
			guid = link
		}
		feed.Items = append(feed.Items, port.ParsedItem{
			GUID:        guid,
			Title:       strings.TrimSpace(it.Title),
			Link:        link,
			Content:     strings.TrimSpace(it.Description),
			Summary:     strings.TrimSpace(it.Description),
			Author:      strings.TrimSpace(it.Creator),
			PublishedAt: parseTime(it.Date),
		})
	}
	return feed, nil
}
```

- [ ] Step 4: テストが通ることを確認する

Run:
```bash
cd /Users/yujiokamoto/devs/golang/feedflow-go-htmx && go test ./internal/feed/ -run TestParseRDF -v
```
Expected: TestParseRDFがPASSし、最後に `ok` になります。

- [ ] Step 5: gofmtを適用してコミットする

```bash
cd /Users/yujiokamoto/devs/golang/feedflow-go-htmx && gofmt -w internal/feed/parser.go internal/feed/parser_rdf_test.go && git add internal/feed/parser.go internal/feed/parser_rdf_test.go && git commit -m "feat: RDFつまりRSS1.0パースを実装する"
```

---

## Task 10: golang.org/x/net/htmlの追加

Files:
- Modify: `/Users/yujiokamoto/devs/golang/feedflow-go-htmx/go.mod`
- Modify: `/Users/yujiokamoto/devs/golang/feedflow-go-htmx/go.sum`

自動検出と本文抽出でHTMLをDOMとして走査するため、golang.org/x/net/htmlを依存に追加します。設計書のセクション13で許可された例外依存です。go getで取得しgo mod tidyで整えます。

- [ ] Step 1: golang.org/x/netを取得する

Run:
```bash
cd /Users/yujiokamoto/devs/golang/feedflow-go-htmx && go get golang.org/x/net@v0.43.0
```
Expected: `go: added golang.org/x/net v0.43.0` と表示されます。バージョンは取得時点の最新パッチでも問題ありません。表示されたバージョンを以降のコミットに含めます。

- [ ] Step 2: モジュールを整える

Run:
```bash
cd /Users/yujiokamoto/devs/golang/feedflow-go-htmx && go mod tidy
```
Expected: エラーなく完了します。go.modにgolang.org/x/netのrequireが残り、go.sumにハッシュが追加されます。この時点ではまだhtmlを参照するコードが無いため、tidyがrequireを間接依存として残すか削除する可能性があります。次のTask11で実際にimportするため、Task11実装後に再度tidyして直接依存へ昇格させます。

補足: もしTask10単独のコミットでgo mod tidyによりrequireが消える場合は、このTask10のコミットはスキップし、Task11でhtmlをimportしたあとにgo getとgo mod tidyをまとめて実行してコミットします。判断基準は、go.modのrequireにgolang.org/x/netが残っているかどうかです。

- [ ] Step 3: requireが残っている場合のみコミットする

Run:
```bash
cd /Users/yujiokamoto/devs/golang/feedflow-go-htmx && grep -q 'golang.org/x/net' go.mod && git add go.mod go.sum && git commit -m "chore: 本文抽出と自動検出のためgolang.org/x/netを追加する" || echo "requireが未確定のためTask11でまとめてコミットします"
```
Expected: requireが残っていればコミットされます。残っていなければスキップのメッセージが出ます。

---

## Task 11: HTMLからのフィード自動検出

Files:
- Test: `/Users/yujiokamoto/devs/golang/feedflow-go-htmx/internal/feed/discover_test.go`
- Create: `/Users/yujiokamoto/devs/golang/feedflow-go-htmx/internal/feed/discover.go`

HTMLのhead内のlink要素から、typeがRSSやAtomを示すものを抽出し、hrefをbaseURLで絶対URL化して返します。サイトURLからフィードを自動検出する機能の中核で、サービス層のSubscribeFromSiteがこの結果を使います。golang.org/x/net/htmlでDOMを走査します。

- [ ] Step 1: 失敗するテストを書く

Create `internal/feed/discover_test.go`:
```go
package feed

import (
	"testing"
)

const htmlWithFeeds = `<!DOCTYPE html>
<html>
<head>
  <title>Example Site</title>
  <link rel="alternate" type="application/rss+xml" title="RSS" href="/feed.xml">
  <link rel="alternate" type="application/atom+xml" title="Atom" href="https://cdn.example.com/atom.xml">
  <link rel="stylesheet" href="/style.css">
  <link rel="alternate" type="application/json" href="/feed.json">
</head>
<body><p>hello</p></body>
</html>`

func TestDiscover(t *testing.T) {
	got, err := Discover([]byte(htmlWithFeeds), "https://example.com/blog/")
	if err != nil {
		t.Fatalf("Discover returned error: %v", err)
	}
	want := []string{
		"https://example.com/feed.xml",
		"https://cdn.example.com/atom.xml",
	}
	if len(got) != len(want) {
		t.Fatalf("Discover len got %d (%v) want %d (%v)", len(got), got, len(want), want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("Discover[%d] got %q want %q", i, got[i], want[i])
		}
	}
}

func TestDiscoverNone(t *testing.T) {
	got, err := Discover([]byte(`<html><head></head><body></body></html>`), "https://example.com/")
	if err != nil {
		t.Fatalf("Discover returned error: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("フィードが無いHTMLでは空を期待しましたが %vでした", got)
	}
}

func TestDiscoverInvalidBase(t *testing.T) {
	_, err := Discover([]byte(htmlWithFeeds), "://bad-base")
	if err == nil {
		t.Fatal("不正なbaseURLでエラーを期待しましたがnilでした")
	}
}

func TestDiscoverRelativeResolution(t *testing.T) {
	html := `<head><link rel="alternate" type="application/rss+xml" href="../rss"></head>`
	got, err := Discover([]byte(html), "https://example.com/a/b/")
	if err != nil {
		t.Fatalf("Discover returned error: %v", err)
	}
	if len(got) != 1 || got[0] != "https://example.com/a/rss" {
		t.Fatalf("相対URLの解決結果が想定外ですgot %v", got)
	}
}
```

- [ ] Step 2: テストが失敗することを確認する

Run:
```bash
cd /Users/yujiokamoto/devs/golang/feedflow-go-htmx && go test ./internal/feed/ -run TestDiscover -v
```
Expected: コンパイルエラーで失敗します。`undefined: Discover` が表示されます。

- [ ] Step 3: 最小実装を書く

Create `internal/feed/discover.go`:
```go
package feed

import (
	"bytes"
	"fmt"
	"net/url"
	"strings"

	"golang.org/x/net/html"
)

// feedLinkTypes link要素のtype属性のうちフィードとみなす値です。
var feedLinkTypes = map[string]struct{}{
	"application/rss+xml":  {},
	"application/atom+xml": {},
	"application/rdf+xml":  {},
	"application/xml":      {},
	"text/xml":             {},
}

// Discover HTMLのlink要素からフィードのURLを抽出しbaseURLで絶対URL化して返します。
// relがalternateでtypeがフィードを示すlinkを対象にします。出現順を保ち重複を除きます。
func Discover(data []byte, baseURL string) ([]string, error) {
	base, err := url.Parse(baseURL)
	if err != nil {
		return nil, fmt.Errorf("failed to parse base url %q: %w", baseURL, err)
	}
	if base.Scheme == "" || base.Host == "" {
		return nil, fmt.Errorf("feed: base url %q must be absolute", baseURL)
	}

	doc, err := html.Parse(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("failed to parse html: %w", err)
	}

	var found []string
	seen := make(map[string]struct{})
	var walk func(n *html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.ElementNode && strings.EqualFold(n.Data, "link") {
			if href, ok := feedHref(n); ok {
				ref, perr := url.Parse(href)
				if perr == nil {
					abs := base.ResolveReference(ref).String()
					if _, dup := seen[abs]; !dup {
						seen[abs] = struct{}{}
						found = append(found, abs)
					}
				}
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(doc)
	return found, nil
}

// feedHref link要素がフィードを指すならhrefを返します。
// relにalternateを含みtypeがフィード種別でhrefが空でないことを条件にします。
func feedHref(n *html.Node) (string, bool) {
	var rel, typ, href string
	for _, attr := range n.Attr {
		switch strings.ToLower(attr.Key) {
		case "rel":
			rel = strings.ToLower(attr.Val)
		case "type":
			typ = strings.ToLower(strings.TrimSpace(attr.Val))
		case "href":
			href = strings.TrimSpace(attr.Val)
		}
	}
	if !strings.Contains(rel, "alternate") {
		return "", false
	}
	if _, ok := feedLinkTypes[typ]; !ok {
		return "", false
	}
	if href == "" {
		return "", false
	}
	return href, true
}
```

- [ ] Step 4: モジュールを整えて依存を確定する

Run:
```bash
cd /Users/yujiokamoto/devs/golang/feedflow-go-htmx && go mod tidy && grep 'golang.org/x/net' go.mod
```
Expected: `golang.org/x/net vX.Y.Z` が直接依存として表示されます(間接を示す `// indirect` が付きません)。

- [ ] Step 5: テストが通ることを確認する

Run:
```bash
cd /Users/yujiokamoto/devs/golang/feedflow-go-htmx && go test ./internal/feed/ -run TestDiscover -v
```
Expected: TestDiscoverとTestDiscoverNoneとTestDiscoverInvalidBaseとTestDiscoverRelativeResolutionがPASSし、最後に `ok` になります。

- [ ] Step 6: gofmtを適用してコミットする

```bash
cd /Users/yujiokamoto/devs/golang/feedflow-go-htmx && gofmt -w internal/feed/discover.go internal/feed/discover_test.go && git add internal/feed/discover.go internal/feed/discover_test.go go.mod go.sum && git commit -m "feat: HTMLからのフィード自動検出Discoverを追加する"
```

---

## Task 12: 本文抽出の簡易リーダビリティ

Files:
- Test: `/Users/yujiokamoto/devs/golang/feedflow-go-htmx/internal/feed/extract_test.go`
- Create: `/Users/yujiokamoto/devs/golang/feedflow-go-htmx/internal/feed/extract.go`

元記事HTMLから本文を抽出する簡易リーダビリティを実装します。golang.org/x/net/htmlでDOMを走査し、scriptとstyleとnavとheaderとfooterとasideを除外したうえで、テキスト量が最大の本文候補要素を選び、その配下のテキストを段落区切りで連結して返します。articleやmain要素があれば優先します。記事の全文取得Clean view相当の中核です。

- [ ] Step 1: 失敗するテストを書く

Create `internal/feed/extract_test.go`:
```go
package feed

import (
	"strings"
	"testing"
)

const articleHTML = `<!DOCTYPE html>
<html>
<head><title>Article</title><style>.x{color:red}</style></head>
<body>
  <header><nav>menu menu menu menu</nav></header>
  <article>
    <h1>Real Title</h1>
    <p>This is the first substantial paragraph with enough text to matter.</p>
    <p>This is the second substantial paragraph that also has meaningful length.</p>
    <script>console.log("ignore me ignore me ignore me")</script>
  </article>
  <footer>copyright footer footer footer</footer>
</body>
</html>`

func TestExtract(t *testing.T) {
	got, err := Extract([]byte(articleHTML))
	if err != nil {
		t.Fatalf("Extract returned error: %v", err)
	}
	if !strings.Contains(got, "first substantial paragraph") {
		t.Fatalf("本文の一段落目が欠落していますgot %q", got)
	}
	if !strings.Contains(got, "second substantial paragraph") {
		t.Fatalf("本文の二段落目が欠落していますgot %q", got)
	}
	if strings.Contains(got, "console.log") {
		t.Fatalf("scriptの内容が混入していますgot %q", got)
	}
	if strings.Contains(got, "menu menu") {
		t.Fatalf("navの内容が混入していますgot %q", got)
	}
	if strings.Contains(got, "copyright footer") {
		t.Fatalf("footerの内容が混入していますgot %q", got)
	}
}

func TestExtractFallbackToBody(t *testing.T) {
	html := `<html><body><div><p>Only a div wraps this readable sentence of content.</p></div></body></html>`
	got, err := Extract([]byte(html))
	if err != nil {
		t.Fatalf("Extract returned error: %v", err)
	}
	if !strings.Contains(got, "readable sentence of content") {
		t.Fatalf("articleやmainが無くても本文を抽出するはずですがgot %q", got)
	}
}

func TestExtractEmpty(t *testing.T) {
	got, err := Extract([]byte(`<html><head></head><body></body></html>`))
	if err != nil {
		t.Fatalf("Extract returned error: %v", err)
	}
	if strings.TrimSpace(got) != "" {
		t.Fatalf("本文が無い場合は空を期待しましたが %qでした", got)
	}
}

func TestExtractInvalidHTMLStillParses(t *testing.T) {
	// net/htmlは壊れた断片も寛容にパースする。エラーにせず可能な範囲で抽出する。
	got, err := Extract([]byte(`<p>loose paragraph without wrapper tags at all here</p>`))
	if err != nil {
		t.Fatalf("Extract returned error: %v", err)
	}
	if !strings.Contains(got, "loose paragraph") {
		t.Fatalf("断片HTMLからも抽出するはずですがgot %q", got)
	}
}
```

- [ ] Step 2: テストが失敗することを確認する

Run:
```bash
cd /Users/yujiokamoto/devs/golang/feedflow-go-htmx && go test ./internal/feed/ -run TestExtract -v
```
Expected: コンパイルエラーで失敗します。`undefined: Extract` が表示されます。

- [ ] Step 3: 最小実装を書く

Create `internal/feed/extract.go`:
```go
package feed

import (
	"bytes"
	"fmt"
	"strings"

	"golang.org/x/net/html"
	"golang.org/x/net/html/atom"
)

// excludedTags 本文抽出で内容を無視する要素です。
var excludedTags = map[atom.Atom]struct{}{
	atom.Script: {},
	atom.Style:  {},
	atom.Nav:    {},
	atom.Header: {},
	atom.Footer: {},
	atom.Aside:  {},
	atom.Form:   {},
	atom.Noscript: {},
}

// Extract 記事HTMLから本文テキストを段落区切りで抽出します。
// articleまたはmain要素があれば優先し、無ければ本文テキスト量が最大の要素を選びます。
// scriptやstyleやnavやheaderやfooterやasideの内容は除外します。
func Extract(data []byte) (string, error) {
	doc, err := html.Parse(bytes.NewReader(data))
	if err != nil {
		return "", fmt.Errorf("failed to parse html: %w", err)
	}

	if candidate := findPreferred(doc); candidate != nil {
		return collectText(candidate), nil
	}

	best := pickBestByTextLength(doc)
	if best == nil {
		return "", nil
	}
	return collectText(best), nil
}

// findPreferred articleまたはmain要素を深さ優先で探して返します。見つからなければnilを返します。
func findPreferred(n *html.Node) *html.Node {
	if n.Type == html.ElementNode && (n.DataAtom == atom.Article || n.DataAtom == atom.Main) {
		return n
	}
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		if found := findPreferred(c); found != nil {
			return found
		}
	}
	return nil
}

// pickBestByTextLength divやsectionやbodyのうち、除外要素を差し引いた本文テキスト量が最大の要素を返します。
func pickBestByTextLength(root *html.Node) *html.Node {
	var best *html.Node
	bestLen := 0
	var walk func(n *html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.ElementNode {
			switch n.DataAtom {
			case atom.Div, atom.Section, atom.Body, atom.Td, atom.Li:
				if l := len(collectText(n)); l > bestLen {
					bestLen = l
					best = n
				}
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(root)
	return best
}

// collectText 要素配下のテキストを、ブロック要素の境界で段落区切りにしながら連結します。
// 除外要素の内容は取り込みません。
func collectText(n *html.Node) string {
	var b strings.Builder
	var walk func(n *html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.ElementNode {
			if _, skip := excludedTags[n.DataAtom]; skip {
				return
			}
		}
		if n.Type == html.TextNode {
			text := strings.TrimSpace(n.Data)
			if text != "" {
				b.WriteString(text)
				b.WriteString(" ")
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
		if n.Type == html.ElementNode && isBlockElement(n.DataAtom) {
			b.WriteString("\n\n")
		}
	}
	walk(n)
	return normalizeWhitespace(b.String())
}

// isBlockElement 段落区切りを入れるブロック要素かどうかを返します。
func isBlockElement(a atom.Atom) bool {
	switch a {
	case atom.P, atom.Div, atom.Section, atom.Article, atom.H1, atom.H2, atom.H3,
		atom.H4, atom.H5, atom.H6, atom.Ul, atom.Ol, atom.Li, atom.Blockquote, atom.Pre:
		return true
	default:
		return false
	}
}

// normalizeWhitespace 連続する空白と過剰な改行を整理します。
func normalizeWhitespace(s string) string {
	lines := strings.Split(s, "\n")
	out := make([]string, 0, len(lines))
	for _, line := range lines {
		fields := strings.Fields(line)
		if len(fields) == 0 {
			continue
		}
		out = append(out, strings.Join(fields, " "))
	}
	return strings.Join(out, "\n\n")
}
```

- [ ] Step 4: テストが通ることを確認する

Run:
```bash
cd /Users/yujiokamoto/devs/golang/feedflow-go-htmx && go test ./internal/feed/ -run TestExtract -v
```
Expected: TestExtractとTestExtractFallbackToBodyとTestExtractEmptyとTestExtractInvalidHTMLStillParsesがPASSし、最後に `ok` になります。

- [ ] Step 5: gofmtを適用してコミットする

```bash
cd /Users/yujiokamoto/devs/golang/feedflow-go-htmx && gofmt -w internal/feed/extract.go internal/feed/extract_test.go && git add internal/feed/extract.go internal/feed/extract_test.go && git commit -m "feat: golang.org/x/net/htmlによる本文抽出Extractを追加する"
```

---

## Task 13: ローカルサンプルファイルによる回帰テスト

Files:
- Create: `/Users/yujiokamoto/devs/golang/feedflow-go-htmx/internal/feed/testdata/rss2.xml`
- Create: `/Users/yujiokamoto/devs/golang/feedflow-go-htmx/internal/feed/testdata/atom.xml`
- Create: `/Users/yujiokamoto/devs/golang/feedflow-go-htmx/internal/feed/testdata/rdf.xml`
- Test: `/Users/yujiokamoto/devs/golang/feedflow-go-htmx/internal/feed/parser_files_test.go`

設計書のセクション5.3に沿い、各形式の実サンプルファイルをtestdataに置き、ファイルから読み込んでパースする回帰テストを加えます。インラインのサンプルに加えてファイルベースでも形式判別とパースが安定して通ることを保証します。

- [ ] Step 1: RSS2.0サンプルを作成する

Create `internal/feed/testdata/rss2.xml`:
```xml
<?xml version="1.0" encoding="UTF-8"?>
<rss version="2.0" xmlns:content="http://purl.org/rss/1.0/modules/content/">
  <channel>
    <title>Sample RSS Feed</title>
    <link>https://sample.example.com/</link>
    <description>A sample RSS 2.0 feed for tests</description>
    <item>
      <title>Hello RSS</title>
      <link>https://sample.example.com/hello-rss</link>
      <guid isPermaLink="true">https://sample.example.com/hello-rss</guid>
      <description>Short summary of the RSS post.</description>
      <content:encoded><![CDATA[<p>Full RSS body content.</p>]]></content:encoded>
      <author>writer@sample.example.com</author>
      <pubDate>Mon, 02 Mar 2026 08:00:00 +0000</pubDate>
    </item>
  </channel>
</rss>
```

- [ ] Step 2: Atomサンプルを作成する

Create `internal/feed/testdata/atom.xml`:
```xml
<?xml version="1.0" encoding="utf-8"?>
<feed xmlns="http://www.w3.org/2005/Atom">
  <title>Sample Atom Feed</title>
  <link href="https://sample.example.com/" rel="alternate"/>
  <link href="https://sample.example.com/atom.xml" rel="self"/>
  <updated>2026-03-02T08:00:00Z</updated>
  <entry>
    <title>Hello Atom</title>
    <id>tag:sample.example.com,2026:hello-atom</id>
    <link href="https://sample.example.com/hello-atom" rel="alternate"/>
    <summary>Short summary of the Atom entry.</summary>
    <content type="html">&lt;p&gt;Full Atom body content.&lt;/p&gt;</content>
    <author><name>Atom Writer</name></author>
    <published>2026-03-01T00:00:00Z</published>
    <updated>2026-03-02T08:00:00Z</updated>
  </entry>
</feed>
```

- [ ] Step 3: RDFサンプルを作成する

Create `internal/feed/testdata/rdf.xml`:
```xml
<?xml version="1.0" encoding="UTF-8"?>
<rdf:RDF
  xmlns:rdf="http://www.w3.org/1999/02/22-rdf-syntax-ns#"
  xmlns:dc="http://purl.org/dc/elements/1.1/"
  xmlns="http://purl.org/rss/1.0/">
  <channel rdf:about="https://sample.example.com/rdf">
    <title>Sample RDF Feed</title>
    <link>https://sample.example.com/</link>
    <description>A sample RSS 1.0 RDF feed for tests</description>
  </channel>
  <item rdf:about="https://sample.example.com/hello-rdf">
    <title>Hello RDF</title>
    <link>https://sample.example.com/hello-rdf</link>
    <description>Short summary of the RDF item.</description>
    <dc:creator>RDF Writer</dc:creator>
    <dc:date>2026-03-02T08:00:00Z</dc:date>
  </item>
</rdf:RDF>
```

- [ ] Step 4: ファイルベースのテストを書く

Create `internal/feed/parser_files_test.go`:
```go
package feed

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/okamyuji/feedflow-go-htmx/internal/port"
)

func TestParseFromFiles(t *testing.T) {
	tests := []struct {
		file       string
		wantFormat port.FeedFormat
		wantTitle  string
		wantItem   string
	}{
		{file: "rss2.xml", wantFormat: port.FormatRSS2, wantTitle: "Sample RSS Feed", wantItem: "Hello RSS"},
		{file: "atom.xml", wantFormat: port.FormatAtom, wantTitle: "Sample Atom Feed", wantItem: "Hello Atom"},
		{file: "rdf.xml", wantFormat: port.FormatRDF, wantTitle: "Sample RDF Feed", wantItem: "Hello RDF"},
	}
	p := NewXMLParser()
	for _, tt := range tests {
		t.Run(tt.file, func(t *testing.T) {
			data, err := os.ReadFile(filepath.Join("testdata", tt.file))
			if err != nil {
				t.Fatalf("ReadFile returned error: %v", err)
			}
			got, err := p.Parse(data)
			if err != nil {
				t.Fatalf("Parse returned error: %v", err)
			}
			if got.Format != tt.wantFormat {
				t.Fatalf("Format got %q want %q", got.Format, tt.wantFormat)
			}
			if got.Title != tt.wantTitle {
				t.Fatalf("Title got %q want %q", got.Title, tt.wantTitle)
			}
			if len(got.Items) != 1 {
				t.Fatalf("Items len got %d want 1", len(got.Items))
			}
			if got.Items[0].Title != tt.wantItem {
				t.Fatalf("Item title got %q want %q", got.Items[0].Title, tt.wantItem)
			}
			if got.Items[0].GUID == "" {
				t.Fatal("GUIDは空であってはいけません")
			}
		})
	}
}
```

- [ ] Step 5: テストが通ることを確認する

Run:
```bash
cd /Users/yujiokamoto/devs/golang/feedflow-go-htmx && go test ./internal/feed/ -run TestParseFromFiles -v
```
Expected: rss2.xmlとatom.xmlとrdf.xmlの各サブテストがPASSし、最後に `ok` になります。

- [ ] Step 6: コミットする

```bash
cd /Users/yujiokamoto/devs/golang/feedflow-go-htmx && git add internal/feed/testdata internal/feed/parser_files_test.go && git commit -m "test: RSS2.0とAtomとRDFのローカルサンプルで回帰テストを追加する"
```

---

## Task 14: フェーズ全体のテストと品質ゲート

Files:
- 変更なし

- [ ] Step 1: feedとobsの全テストをraceで実行する

Run:
```bash
cd /Users/yujiokamoto/devs/golang/feedflow-go-htmx && go test -race -count=1 ./internal/feed/... ./internal/obs/...
```
Expected: 両パッケージとも `ok` と表示されます。データ競合の警告は出ません。

- [ ] Step 2: カバレッジを確認する

Run:
```bash
cd /Users/yujiokamoto/devs/golang/feedflow-go-htmx && go test -coverprofile=coverage.out ./internal/feed/... ./internal/obs/... && go tool cover -func=coverage.out | tail -n 1
```
Expected: 合計カバレッジが80パーセント前後以上になります。目安であり厳密な合否基準ではありません。HTTPFetcherのネットワークエラー経路の一部は到達しにくいため、未達でもfailさせません。

- [ ] Step 3: 品質ゲートを実行する

Run:
```bash
cd /Users/yujiokamoto/devs/golang/feedflow-go-htmx && bash scripts/quality-gate.sh
```
Expected: `all quality checks passed` で終わります。bodycloseやnoctxやgosecの指摘が出たら修正してから再実行します。

- [ ] Step 4: 品質ゲート緑のままコミットする

```bash
cd /Users/yujiokamoto/devs/golang/feedflow-go-htmx && git add -A && git commit -m "chore: Phase3のフィード取得とパースで品質ゲートを緑化する"
```
Expected: コミット時にquality-gateが走り、緑のままコミットされます。差分が無ければこのコミットは省略できます。

---

## Phase3完了条件

- [ ] `go test -race ./internal/feed/... ./internal/obs/...` が通る
- [ ] HTTPFetcherがport.Fetcherを満たし、条件付き取得(If-None-MatchとIf-Modified-Since)と304の未更新とgzip展開とサイズ上限とタイムアウトが動作する
- [ ] SSRFガードがプライベートIPとループバックとクラウドメタデータIPをErrPrivateAddressで拒否し、スキームをhttpとhttpsに限定する
- [ ] XMLParserがport.FeedParserを満たし、RSS2.0とAtomとRDFを判別してパースする
- [ ] DiscoverがHTMLのlink要素からフィードURLを抽出しbaseURLで絶対URL化する
- [ ] Extractがgolang.org/x/net/htmlで本文を抽出しscriptやnavやfooterを除外する
- [ ] go.modにgolang.org/x/netが直接依存として入っている
- [ ] レスポンスボディとgzipリーダのクローズがobs.CloseAndLog経由で記録される
- [ ] `bash scripts/quality-gate.sh` が `all quality checks passed` で終わる
