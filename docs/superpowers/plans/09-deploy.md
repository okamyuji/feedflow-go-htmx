# Phase8 デプロイ実装計画

> For agentic workers: REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

## 実装更新メモ(計画後の全面変更)

この計画の初版はnginxのmTLSとLet's Encryptとcertbotを前提にしていましたが、実装ではデプロイ方式を全面的に変更しました。以降のタスク本文(mTLS、Let's Encrypt、certbot、security-group.json、ローカルmTLS疎通など)は歴史的経緯として残しますが、現行の正は`deploy/terraform`配下のコードと設計書セクション9.2と11です。現行方式の要点は次のとおりです。

- terraformでAWSとCloudflareの双方を管理します。AWSは既定VPCにElastic IP付きの単一EC2(t4g.small、ap-northeast-1)とEBSを作り、SSHプロビジョナでアプリのtar.gzを配送してEC2上でdocker composeビルドします
- 本人限定はCloudflare Accessで所有者メールだけを通過させます。当初設計のブラウザクライアント証明書(mTLS)はCloudflareで終端され通らないため廃止しました
- エッジHTTPSはCloudflareプロキシ(Aレコードproxied)で終端し、オリジンIPを秘匿します。SSL/TLSモードはFull(strict)です
- オリジンはCloudflare Origin CA証明書を配置します。Origin CA Keyは非推奨のため使わず、terraformの管理するAPIトークンで発行します(プロバイダv3.32.0以降の仕様、トークンにSSL and Certificates編集権限が必要)
- Let's Encryptとcertbotは使いません。証明書更新は不要です
- Security GroupはCloudflareのIP範囲からの443と80に限定し、SSHは運用者IPのみに限定します。nginxはCF-Connecting-IPで実クライアントIPを復元します
- Amazon Linux 2023の既定buildxは古くcompose buildが0.17.0以上を要求するため、最新のbuildxプラグインを起動時に手動導入します
- 秘密値はgitignore済みの`deploy/terraform/secrets.auto.tfvars`に置き、Cloudflare APIトークンとアカウントIDを与えます

Goal: feedflowを本番起動できる形まで結線し、単一EC2(ARMのt4g系)へデプロイする構成一式を整えます。まずstoreとfeedとserviceとpollerとauthの具象をcmd側で組み立て、認証アダプタを介してhandler.Depsへ注入し、Phase7で残ったnil依存を解消します。そのうえでマルチステージのDockerfileでembed同梱の単一バイナリを作り、compose.ymlでnginxコンテナとGoアプリコンテナを同居させ、nginxでTLS終端とmTLSクライアント証明書検証を行い、EBSでdataディレクトリを永続化し、Security Groupは443とSSHのみを開放します。ALBとNLBは使いません。

Architecture: 前段にnginxコンテナを置き、443でTLS終端し、クライアント証明書を検証してから内部ネットワーク経由でアプリコンテナの8080へリバースプロキシします。アプリコンテナはembed同梱の単一バイナリで、EBSにマウントしたdataディレクトリへJSONを永続化します。証明書はLet's Encryptのサーバ証明書と、自前のローカルCAで発行したクライアント証明書を用います。アプリはnginx経由のみ到達可能とし、ホストの443とSSHだけをSecurity Groupで開放します。デプロイ手順とverify-deploy.shで構成の妥当性を機械的に検証します。

Tech Stack: Docker、Docker Compose、nginx(alpine)、Go 1.25、distrolessランタイムイメージ、OpenSSL、certbot(Let's Encrypt)、AWS EC2(ARM t4g)、EBS。

前提: 作業ディレクトリは`/Users/yujiokamoto/devs/golang/feedflow-go-htmx`です。Phase7までが完了し`bash scripts/quality-gate.sh`が緑であることを前提とします。`docker version`と`openssl version`がローカルで利用できることを確認してから始めます。

このフェーズで参照するPhase1の型とインターフェースは次のとおりです。本フェーズはこれらの定義を変更しません。

- アプリのエントリは`cmd/feedflow/main.go`で、バージョンは`-ldflags "-X main.version=..."`で埋め込みます(Phase0で確定)。
- 環境変数は`FEEDFLOW_ADDR`、`FEEDFLOW_DATA_DIR`、`FEEDFLOW_BASE_URL`、`FEEDFLOW_SESSION_KEY`を用います(Phase0の`.env.example`で確定)。
- 死活監視は`GET /healthz`で`ok`を返します(Phase0で確定)。

このフェーズで追加する補助型は次のとおりです。Phase1の型定義と矛盾しません。

- `Config`(`cmd/feedflow/config.go`に置く起動設定値の構造体)。フィールドはAddr、DataDir、BaseURL、SessionKey、TLSCertFile、TLSKeyFileです。環境変数からの読み取りと検証だけを担い、ドメイン型には依存しません。Phase0のmainが環境変数を直接読んでいる場合は、本フェーズでこの構造体へ集約します。
- `systemClock`と`cryptoIDGen`(`cmd/feedflow/runtime.go`に置く`port.Clock`と`port.IDGen`の本番実装)。Phase1からPhase7まではフェイクだけが存在し、本番の時刻源とID生成器がどのフェーズでも作られていないため、結線の起点となる本フェーズで具象を用意します。
- `sessionsAdapter`と`csrfAdapter`と`setupAdapter`(`cmd/feedflow/auth_adapter.go`に置く認証アダプタ)。Phase6(07-auth.md)のinternal/authの具象`*auth.SessionStore`と`*auth.CSRFStore`と`*auth.Manager`は、handler側の認証ポート(`handler.Sessions`、`handler.CSRF`、`handler.SetupGuard`)とシグネチャが異なります。具体的には`SessionStore`は`Issue(w, username) error`と`Validate(r) (string, bool)`を持ち、handlerは`Create(w, username) (handler.Session, error)`と`Get(r) (handler.Session, bool)`を要求します。`CSRFStore.Verify(sessionID, r) bool`に対しhandlerは`CSRF.Verify(sess handler.Session, token string) bool`を要求します。`Manager.Setup`に対しhandlerは`SetupGuard.Register`を要求します。これらの差をアダプタで吸収し、handler.Depsへ注入できる形にします。`*auth.RateLimiter`は`Allow(key) bool`がhandler.RateLimiterと一致するためアダプタ不要で直接代入します。

設計判断: 認証アダプタはinternal/authとinternal/handlerのどちらの公開シグネチャも変更しないため、本フェーズのcmd側だけで完結します。handlerの`handler.Session`はCSRFトークンを保持しますが、auth側のセッションはユーザー名しか保持しないため、アダプタはセッションIDをキーに`*auth.CSRFStore`からトークンを補い、`handler.Session.CSRFToken`へ充填します。セッションIDはアダプタが構築時に保持するCookie名でリクエストCookieから読み取り、`Create`時はレスポンスへ書き込んだSet-Cookieを解析して取り出します。

---

## Task 1: 起動設定をConfig構造体へ集約する(TDD)

Files:
- Test: `/Users/yujiokamoto/devs/golang/feedflow-go-htmx/cmd/feedflow/config_test.go`
- Create: `/Users/yujiokamoto/devs/golang/feedflow-go-htmx/cmd/feedflow/config.go`

- [ ] Step 1: 失敗するテストを書く

Create `/Users/yujiokamoto/devs/golang/feedflow-go-htmx/cmd/feedflow/config_test.go`:
```go
package main

import "testing"

func TestLoadConfigDefaults(t *testing.T) {
	get := func(key string) string { return "" }

	cfg := loadConfig(get)

	if cfg.Addr != ":8080" {
		t.Fatalf("Addr got %q want %q", cfg.Addr, ":8080")
	}
	if cfg.DataDir != "./data" {
		t.Fatalf("DataDir got %q want %q", cfg.DataDir, "./data")
	}
}

func TestLoadConfigFromEnv(t *testing.T) {
	values := map[string]string{
		"FEEDFLOW_ADDR":        ":9090",
		"FEEDFLOW_DATA_DIR":    "/var/lib/feedflow/data",
		"FEEDFLOW_BASE_URL":    "https://feedflow.example.com",
		"FEEDFLOW_SESSION_KEY": "secret-key",
	}
	get := func(key string) string { return values[key] }

	cfg := loadConfig(get)

	if cfg.Addr != ":9090" {
		t.Fatalf("Addr got %q want %q", cfg.Addr, ":9090")
	}
	if cfg.DataDir != "/var/lib/feedflow/data" {
		t.Fatalf("DataDir got %q want %q", cfg.DataDir, "/var/lib/feedflow/data")
	}
	if cfg.BaseURL != "https://feedflow.example.com" {
		t.Fatalf("BaseURL got %q want %q", cfg.BaseURL, "https://feedflow.example.com")
	}
	if cfg.SessionKey != "secret-key" {
		t.Fatalf("SessionKey got %q want %q", cfg.SessionKey, "secret-key")
	}
}

func TestConfigValidate(t *testing.T) {
	tests := []struct {
		name    string
		cfg     Config
		wantErr bool
	}{
		{
			name:    "valid",
			cfg:     Config{Addr: ":8080", DataDir: "./data", BaseURL: "https://feedflow.example.com", SessionKey: "k"},
			wantErr: false,
		},
		{
			name:    "empty addr",
			cfg:     Config{Addr: "", DataDir: "./data", BaseURL: "https://x", SessionKey: "k"},
			wantErr: true,
		},
		{
			name:    "empty data dir",
			cfg:     Config{Addr: ":8080", DataDir: "", BaseURL: "https://x", SessionKey: "k"},
			wantErr: true,
		},
		{
			name:    "empty session key",
			cfg:     Config{Addr: ":8080", DataDir: "./data", BaseURL: "https://x", SessionKey: ""},
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.cfg.Validate()
			if (err != nil) != tt.wantErr {
				t.Fatalf("Validate err got %v wantErr %v", err, tt.wantErr)
			}
		})
	}
}
```

- [ ] Step 2: テストが失敗することを確認する

Run:
```bash
go test ./cmd/feedflow/ -run 'TestLoadConfig|TestConfigValidate' -v
```
Expected: コンパイルエラーで失敗します。`undefined: loadConfig`と`undefined: Config`が表示されます。

- [ ] Step 3: 最小実装を書く

Create `/Users/yujiokamoto/devs/golang/feedflow-go-htmx/cmd/feedflow/config.go`:
```go
package main

import (
	"errors"
	"fmt"
)

// Config feedflowの起動設定値を表します。環境変数から読み取り検証だけを担います。
type Config struct {
	Addr        string
	DataDir     string
	BaseURL     string
	SessionKey  string
	TLSCertFile string
	TLSKeyFile  string
}

// loadConfig 環境変数取得関数getから設定値を読み取りConfigを返します。
// 未設定の項目には既定値を補います。
func loadConfig(get func(string) string) Config {
	return Config{
		Addr:        valueOr(get("FEEDFLOW_ADDR"), ":8080"),
		DataDir:     valueOr(get("FEEDFLOW_DATA_DIR"), "./data"),
		BaseURL:     get("FEEDFLOW_BASE_URL"),
		SessionKey:  get("FEEDFLOW_SESSION_KEY"),
		TLSCertFile: get("FEEDFLOW_TLS_CERT_FILE"),
		TLSKeyFile:  get("FEEDFLOW_TLS_KEY_FILE"),
	}
}

// valueOr 値vが空のとき既定値defを返します。
func valueOr(v, def string) string {
	if v == "" {
		return def
	}
	return v
}

// Validate 起動に必須の項目が埋まっているか検証します。
func (c Config) Validate() error {
	if c.Addr == "" {
		return errors.New("FEEDFLOW_ADDR が空です")
	}
	if c.DataDir == "" {
		return errors.New("FEEDFLOW_DATA_DIR が空です")
	}
	if c.SessionKey == "" {
		return fmt.Errorf("FEEDFLOW_SESSION_KEY が空です: 起動前に設定してください")
	}
	return nil
}
```

- [ ] Step 4: テストが通ることを確認する

Run:
```bash
go test ./cmd/feedflow/ -run 'TestLoadConfig|TestConfigValidate' -v
```
Expected: すべてPASSします。

- [ ] Step 5: gofmtを適用してテストを通す

main.goへの`loadConfig`の結線はTask2で実依存の組み立てとあわせて行います。本Taskではconfig.goとconfig_test.goだけを対象にします。

Run:
```bash
gofmt -w cmd/feedflow/config.go cmd/feedflow/config_test.go
go test ./cmd/feedflow/ -run 'TestLoadConfig|TestConfigValidate' -v
```
Expected: すべてPASSします。

- [ ] Step 6: コミットする

```bash
git add cmd/feedflow/config.go cmd/feedflow/config_test.go
git commit -m "feat: 起動設定をConfig構造体へ集約する"
```

---

## Task 2: 実依存を組み立ててhandler.Depsへ注入する(TDD)

Files:
- Create: `/Users/yujiokamoto/devs/golang/feedflow-go-htmx/cmd/feedflow/runtime.go`
- Create: `/Users/yujiokamoto/devs/golang/feedflow-go-htmx/cmd/feedflow/auth_adapter.go`
- Create: `/Users/yujiokamoto/devs/golang/feedflow-go-htmx/cmd/feedflow/auth_adapter_test.go`
- Modify: `/Users/yujiokamoto/devs/golang/feedflow-go-htmx/cmd/feedflow/main.go`
- Modify: `/Users/yujiokamoto/devs/golang/feedflow-go-htmx/cmd/feedflow/main_test.go`

Phase7(08-handler-ui.md Task13)の`buildHandler`は`deps := handler.Deps{}`で全フィールドがnilのままです。このままでは本番起動時にSessionsやCSRFやSetupや各サービスがnilになり、requireAuthやrequireCSRFやloginが最初のアクセスでpanicします。本Taskでstoreとfeedとserviceとpollerとauthの具象を生成し、auth具象をhandlerポートへ適合させるアダプタを介してhandler.Depsへ注入します。これによりmain.goが/healthz以外も実依存で応答します。

依存の対応関係は次のとおりです。型とシグネチャはそれぞれのフェーズの計画に厳密に一致させます。

- `store.New(dataDir) (*store.Store, error)`がport.Repositoryを満たします(03-store.md)。
- `feed.NewHTTPFetcher() *feed.HTTPFetcher`がport.Fetcherを、`feed.NewXMLParser() *feed.XMLParser`がport.FeedParserを満たします(04-feed.md)。
- `service.NewMuteService(deps) *service.MuteService`、`service.NewItemService(deps, mute)`、`service.NewRetentionService(deps)`、`service.NewSubscriptionService(deps)`、`service.NewSettingsService(deps)`、`service.NewOPMLService(deps, subs)`が各port.*Serviceを満たします(05-service.md)。`service.Deps`のフィールドはRepoとFetchとParseとClockとIDsです。
- `poller.NewService(repo, fetcher, parser, clock, ids, mute) *poller.Service`がport.PollServiceを満たします(06-poller.md)。
- `auth.NewSessionStore(auth.SessionConfig{Clock, TTL, CookieName, Secure}) *auth.SessionStore`、`auth.NewCSRFStore() *auth.CSRFStore`、`auth.NewRateLimiter(auth.RateLimitConfig{Clock, Burst, RefillEvery}) *auth.RateLimiter`、`auth.NewManager(repo, auth.DefaultParams()) *auth.Manager`を使います(07-auth.md)。

- [ ] Step 1: 失敗するテストを書く

認証アダプタが`handler.Sessions`と`handler.CSRF`と`handler.SetupGuard`を満たし、セッション発行からCSRF照合までが一巡することを検証します。

Create `/Users/yujiokamoto/devs/golang/feedflow-go-htmx/cmd/feedflow/auth_adapter_test.go`:
```go
package main

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/okamyuji/feedflow-go-htmx/internal/auth"
	"github.com/okamyuji/feedflow-go-htmx/internal/handler"
)

func TestAuthAdaptersSatisfyPorts(t *testing.T) {
	var (
		_ handler.Sessions   = (*sessionsAdapter)(nil)
		_ handler.CSRF       = (*csrfAdapter)(nil)
		_ handler.SetupGuard = (*setupAdapter)(nil)
		_ handler.RateLimiter = (*auth.RateLimiter)(nil)
	)
}

func TestSessionsAdapterRoundTrip(t *testing.T) {
	clock := systemClock{}
	const cookieName = "feedflow_session"
	store := auth.NewSessionStore(auth.SessionConfig{
		Clock:      clock,
		TTL:        time.Hour,
		CookieName: cookieName,
		Secure:     false,
	})
	csrf := auth.NewCSRFStore()
	sessions := newSessionsAdapter(store, csrf, cookieName)
	csrfPort := newCSRFAdapter()

	rec := httptest.NewRecorder()
	created, err := sessions.Create(rec, "owner")
	if err != nil {
		t.Fatalf("Create returned error: %v", err)
	}
	if created.Username != "owner" {
		t.Fatalf("Username got %q want %q", created.Username, "owner")
	}
	if created.CSRFToken == "" {
		t.Fatalf("Create must populate CSRFToken")
	}

	req := httptest.NewRequest(http.MethodGet, "/app", nil)
	for _, c := range rec.Result().Cookies() {
		req.AddCookie(c)
	}
	got, ok := sessions.Get(req)
	if !ok {
		t.Fatalf("Get must find the session created above")
	}
	if got.Username != "owner" {
		t.Fatalf("Get Username got %q want %q", got.Username, "owner")
	}
	if got.CSRFToken != created.CSRFToken {
		t.Fatalf("CSRFToken got %q want %q", got.CSRFToken, created.CSRFToken)
	}

	if !csrfPort.Verify(got, got.CSRFToken) {
		t.Fatalf("Verify must accept the matching token")
	}
	if csrfPort.Verify(got, "wrong-token") {
		t.Fatalf("Verify must reject a mismatched token")
	}
}
```

- [ ] Step 2: テストが失敗することを確認する

Run:
```bash
cd /Users/yujiokamoto/devs/golang/feedflow-go-htmx && go test ./cmd/feedflow/ -run 'TestAuthAdapters|TestSessionsAdapter' -v
```
Expected: コンパイルエラーで失敗します。`undefined: sessionsAdapter`や`undefined: systemClock`と表示されます。

- [ ] Step 3: 本番のClockとIDGenを実装する

Create `/Users/yujiokamoto/devs/golang/feedflow-go-htmx/cmd/feedflow/runtime.go`:
```go
package main

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"time"
)

// systemClock 実時刻を返すport.Clockの本番実装です。
type systemClock struct{}

// Now 現在時刻を返します。
func (systemClock) Now() time.Time {
	return time.Now()
}

// cryptoIDGen 暗号論的乱数から一意なIDを生成するport.IDGenの本番実装です。
type cryptoIDGen struct{}

// NewID 16バイトの乱数を16進文字列にしたIDを返します。
// 乱数の取得に失敗した場合はナノ秒時刻を退避値として用い、空文字列を返しません。
func (cryptoIDGen) NewID() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return fmt.Sprintf("ts-%d", time.Now().UnixNano())
	}
	return hex.EncodeToString(b)
}
```

- [ ] Step 4: 認証アダプタを実装する

handlerの`Sessions`と`CSRF`と`SetupGuard`へauthの具象を適合させます。`handler.Session`はCSRFトークンを保持するため、アダプタはセッションIDをキーに`*auth.CSRFStore`からトークンを補います。セッションIDは`Create`時にレスポンスのSet-Cookieから取り出し、`Get`時はリクエストCookieから読み取ります。

Create `/Users/yujiokamoto/devs/golang/feedflow-go-htmx/cmd/feedflow/auth_adapter.go`:
```go
package main

import (
	"crypto/subtle"
	"fmt"
	"net/http"
	"time"

	"github.com/okamyuji/feedflow-go-htmx/internal/auth"
	"github.com/okamyuji/feedflow-go-htmx/internal/handler"
)

// sessionsAdapter auth.SessionStoreとauth.CSRFStoreをhandler.Sessionsへ適合させます。
// handler.SessionはCSRFトークンを保持するため、セッションIDをキーにトークンを補います。
type sessionsAdapter struct {
	store      *auth.SessionStore
	csrf       *auth.CSRFStore
	cookieName string
}

// newSessionsAdapter セッションストアとCSRFストアとCookie名からアダプタを生成します。
func newSessionsAdapter(store *auth.SessionStore, csrf *auth.CSRFStore, cookieName string) *sessionsAdapter {
	return &sessionsAdapter{store: store, csrf: csrf, cookieName: cookieName}
}

// Create セッションを発行し、対応するCSRFトークンを補ってhandler.Sessionを返します。
// auth.SessionStore.Issueはセッションを発行しレスポンスへCookieを書き込みます。
// セッションIDはIssueが書き込んだSet-Cookieから取り出し、そのIDでCSRFトークンを発行します。
func (a *sessionsAdapter) Create(w http.ResponseWriter, username string) (handler.Session, error) {
	if err := a.store.Issue(w, username); err != nil {
		return handler.Session{}, fmt.Errorf("failed to issue session: %w", err)
	}
	id, ok := sessionIDFromResponse(w, a.cookieName)
	if !ok {
		return handler.Session{}, fmt.Errorf("failed to read session cookie after issue")
	}
	token, err := a.csrf.Issue(id)
	if err != nil {
		return handler.Session{}, fmt.Errorf("failed to issue csrf token: %w", err)
	}
	return handler.Session{Username: username, CSRFToken: token}, nil
}

// Get リクエストのCookieからセッションを検証し、CSRFトークンを補って返します。
// 未認証の場合はokをfalseにします。
func (a *sessionsAdapter) Get(r *http.Request) (handler.Session, bool) {
	username, ok := a.store.Validate(r)
	if !ok {
		return handler.Session{}, false
	}
	c, err := r.Cookie(a.cookieName)
	if err != nil {
		return handler.Session{}, false
	}
	token, err := a.csrf.Issue(c.Value)
	if err != nil {
		return handler.Session{}, false
	}
	return handler.Session{Username: username, CSRFToken: token}, true
}

// Destroy セッションと対応するCSRFトークンを破棄します。
func (a *sessionsAdapter) Destroy(w http.ResponseWriter, r *http.Request) {
	if c, err := r.Cookie(a.cookieName); err == nil {
		a.csrf.Discard(c.Value)
	}
	a.store.Destroy(w, r)
}

// sessionIDFromResponse レスポンスへ書き込んだSet-CookieからセッションIDを取り出します。
func sessionIDFromResponse(w http.ResponseWriter, cookieName string) (string, bool) {
	header := http.Header{}
	for _, line := range w.Header().Values("Set-Cookie") {
		header.Add("Set-Cookie", line)
	}
	dummy := &http.Response{Header: header}
	for _, c := range dummy.Cookies() {
		if c.Name == cookieName {
			return c.Value, true
		}
	}
	return "", false
}

// csrfAdapter handler.CSRFを満たし、handler.Sessionが保持するトークンと送信トークンを照合します。
// handler.Sessionはアダプタが充填した正規のトークンを保持するため、定数時間比較で照合します。
type csrfAdapter struct{}

// newCSRFAdapter CSRFアダプタを生成します。
func newCSRFAdapter() *csrfAdapter {
	return &csrfAdapter{}
}

// Verify セッションのトークンと送信トークンが一致するかを定数時間比較で判定します。
func (a *csrfAdapter) Verify(sess handler.Session, sentToken string) bool {
	if sess.CSRFToken == "" || sentToken == "" {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(sess.CSRFToken), []byte(sentToken)) == 1
}

// setupAdapter auth.Managerをhandler.SetupGuardへ適合させます。
// Managerは初回登録をSetupという名前で公開するため、Registerへ読み替えます。
type setupAdapter struct {
	manager *auth.Manager
}

// newSetupAdapter Managerからセットアップアダプタを生成します。
func newSetupAdapter(manager *auth.Manager) *setupAdapter {
	return &setupAdapter{manager: manager}
}

// NeedsSetup 初回セットアップが必要かどうかを返します。
func (a *setupAdapter) NeedsSetup() (bool, error) {
	return a.manager.NeedsSetup()
}

// Register 初回の所有者を登録します。auth.Manager.Setupへ委譲します。
func (a *setupAdapter) Register(username, password string) error {
	return a.manager.Setup(username, password)
}

// Authenticate ユーザー名とパスワードを検証します。
func (a *setupAdapter) Authenticate(username, password string) (bool, error) {
	return a.manager.Authenticate(username, password)
}

// sessionTTL Cookieセッションの有効期間です。
const sessionTTL = 24 * time.Hour

// loginBurst ログイン試行の同時許容数です。
const loginBurst = 5

// loginRefill ログイン試行トークンを1個補充する間隔です。
const loginRefill = time.Minute

// sessionCookieName セッションCookieの名前です。アダプタとSessionStoreで共有します。
const sessionCookieName = "feedflow_session"
```

- [ ] Step 5: アダプタのテストが通ることを確認する

Run:
```bash
cd /Users/yujiokamoto/devs/golang/feedflow-go-htmx && go test ./cmd/feedflow/ -run 'TestAuthAdapters|TestSessionsAdapter' -v
```
Expected: すべてPASSします。

- [ ] Step 6: buildHandlerで実依存を組み立てて注入する

`cmd/feedflow/main.go`の骨組みだった`buildHandler`を、Configを受け取り実依存を組み立てる形へ置き換えます。auth.RateLimiterはhandler.RateLimiterと一致するため直接代入し、Sessions、CSRF、Setupはアダプタを介します。

Replace `/Users/yujiokamoto/devs/golang/feedflow-go-htmx/cmd/feedflow/main.go` with:
```go
// Package main feedflowのエントリポイントを提供します。
package main

import (
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/okamyuji/feedflow-go-htmx/internal/auth"
	"github.com/okamyuji/feedflow-go-htmx/internal/feed"
	"github.com/okamyuji/feedflow-go-htmx/internal/handler"
	"github.com/okamyuji/feedflow-go-htmx/internal/poller"
	"github.com/okamyuji/feedflow-go-htmx/internal/service"
	"github.com/okamyuji/feedflow-go-htmx/internal/store"
)

// version ビルド時に-ldflagsで埋め込むバージョン文字列です。
var version = "dev"

func main() {
	cfg := loadConfig(getenv)
	if err := cfg.Validate(); err != nil {
		log.Fatalf("invalid config: %v", err)
	}

	srvHandler, err := buildHandler(cfg)
	if err != nil {
		log.Fatalf("failed to build handler: %v", err)
	}

	srv := &http.Server{
		Addr:              cfg.Addr,
		Handler:           srvHandler,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	log.Printf("feedflow %s listening on %s", version, srv.Addr)
	if err := srv.ListenAndServe(); err != nil {
		log.Fatalf("server stopped: %v", err)
	}
}

// buildHandler Configから実依存を組み立て、ルーティング済みハンドラを返します。
// store、feed、service、poller、authの具象を生成し、認証アダプタを介してhandler.Depsへ注入します。
func buildHandler(cfg Config) (http.Handler, error) {
	repo, err := store.New(cfg.DataDir)
	if err != nil {
		return nil, fmt.Errorf("failed to open store: %w", err)
	}

	clock := systemClock{}
	ids := cryptoIDGen{}
	fetcher := feed.NewHTTPFetcher()
	parser := feed.NewXMLParser()

	svcDeps := service.Deps{
		Repo:  repo,
		Fetch: fetcher,
		Parse: parser,
		Clock: clock,
		IDs:   ids,
	}

	mutes := service.NewMuteService(svcDeps)
	items := service.NewItemService(svcDeps, mutes)
	retention := service.NewRetentionService(svcDeps)
	subscriptions := service.NewSubscriptionService(svcDeps)
	settings := service.NewSettingsService(svcDeps)
	opml := service.NewOPMLService(svcDeps, subscriptions)
	poll := poller.NewService(repo, fetcher, parser, clock, ids, mutes)

	sessionStore := auth.NewSessionStore(auth.SessionConfig{
		Clock:      clock,
		TTL:        sessionTTL,
		CookieName: sessionCookieName,
		Secure:     cfg.isSecure(),
	})
	csrfStore := auth.NewCSRFStore()
	loginLimiter := auth.NewRateLimiter(auth.RateLimitConfig{
		Clock:       clock,
		Burst:       loginBurst,
		RefillEvery: loginRefill,
	})
	manager := auth.NewManager(repo, auth.DefaultParams())

	deps := handler.Deps{
		Subscriptions: subscriptions,
		Items:         items,
		Retention:     retention,
		Mutes:         mutes,
		OPML:          opml,
		Settings:      settings,
		Poll:          poll,
		Sessions:      newSessionsAdapter(sessionStore, csrfStore, sessionCookieName),
		CSRF:          newCSRFAdapter(),
		LoginLimiter:  loginLimiter,
		Setup:         newSetupAdapter(manager),
	}

	h, err := handler.New(deps)
	if err != nil {
		return nil, fmt.Errorf("failed to build handler: %w", err)
	}
	return h.Routes(), nil
}
```

補足: `getenv`と`cfg.isSecure`はconfig.goに小さなヘルパとして加えます。`getenv`は`os.Getenv`を包んで本番の環境変数を読み、`isSecure`はBaseURLがhttpsで始まるときにCookieのSecure属性を有効にします。次のStepでconfig.goへ追加します。

- [ ] Step 7: config.goへ実行時ヘルパを加える

Edit `/Users/yujiokamoto/devs/golang/feedflow-go-htmx/cmd/feedflow/config.go`の末尾に、本番の環境変数読み取りとSecure判定を加えます。importに`os`と`strings`を追加します。
```go
// getenv 本番の環境変数を読み取るloadConfig用の取得関数です。
func getenv(key string) string {
	return os.Getenv(key)
}

// isSecure BaseURLがhttpsのときにCookieへSecure属性を付けるべきかを返します。
func (c Config) isSecure() bool {
	return strings.HasPrefix(c.BaseURL, "https://")
}
```

- [ ] Step 8: cmdのテストを実依存つきへ更新する

nil依存では起動できないことを完了条件にするため、buildHandlerが実依存で組み上がり/healthz以外も応答することを検証します。一時データディレクトリを使い、未認証アクセスが/loginへリダイレクトされること、つまりSessionsがnilでなく実際に評価されることを確認します。

Replace `/Users/yujiokamoto/devs/golang/feedflow-go-htmx/cmd/feedflow/main_test.go` with:
```go
package main

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

// newTestHandler 一時データディレクトリで実依存つきのハンドラを構築します。
func newTestHandler(t *testing.T) http.Handler {
	t.Helper()
	cfg := Config{
		Addr:       ":8080",
		DataDir:    t.TempDir(),
		BaseURL:    "https://feedflow.example.com",
		SessionKey: "test-session-key",
	}
	h, err := buildHandler(cfg)
	if err != nil {
		t.Fatalf("buildHandler returned error: %v", err)
	}
	return h
}

func TestBuildHandlerHealthz(t *testing.T) {
	h := newTestHandler(t)
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

func TestBuildHandlerProtectedRouteUsesRealDeps(t *testing.T) {
	h := newTestHandler(t)
	req := httptest.NewRequest(http.MethodGet, "/app", nil)
	rec := httptest.NewRecorder()

	h.ServeHTTP(rec, req)

	// 実依存のSessionsが評価され、未認証は/loginへリダイレクトされます。
	// nil依存ならここでpanicするため、リダイレクトが返ること自体が結線の証明になります。
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("status got %d want %d", rec.Code, http.StatusSeeOther)
	}
	if loc := rec.Header().Get("Location"); loc != "/login" {
		t.Fatalf("redirect location got %q want %q", loc, "/login")
	}
}
```

補足: 認証必須のルートパスとリダイレクト先は08-handler-ui.mdのルーティングに合わせます。`/app`が認証必須でないか、リダイレクト先が異なる場合は、08-handler-ui.mdのTask12のルーティング定義に従いパスと期待値を読み替えます。重要なのは、実依存のSessionsが評価されてpanicしないことを検証する点です。

- [ ] Step 9: gofmtを適用してテストを通す

Run:
```bash
cd /Users/yujiokamoto/devs/golang/feedflow-go-htmx && gofmt -w cmd/feedflow/ && go test ./cmd/feedflow/ -v
```
Expected: すべてPASSします。

- [ ] Step 10: コミットする

```bash
cd /Users/yujiokamoto/devs/golang/feedflow-go-htmx && git add cmd/feedflow/ && git commit -m "feat: 実依存を組み立ててhandler.Depsへ注入する"
```

---

## Task 3: 単一バイナリのマルチステージDockerfile

Files:
- Create: `/Users/yujiokamoto/devs/golang/feedflow-go-htmx/Dockerfile`
- Create: `/Users/yujiokamoto/devs/golang/feedflow-go-htmx/.dockerignore`

- [ ] Step 1: .dockerignoreを作成する

Create `/Users/yujiokamoto/devs/golang/feedflow-go-htmx/.dockerignore`:
```
# ビルドコンテキストから除外します。embed対象のweb配下とGoソースだけを送ります。
.git
.github
bin
dist
coverage.out
coverage.html
data
*.pem
*.key
*.crt
.env
node_modules
e2e
docs
deploy
.superpowers
.DS_Store
```

- [ ] Step 2: Dockerfileを作成する

web配下のテンプレートと静的資産はembedで単一バイナリへ同梱するため、ランタイムイメージにはバイナリだけを置きます。ビルダで`CGO_ENABLED=0`の静的バイナリを作り、distrolessのnonrootで動かします。

Create `/Users/yujiokamoto/devs/golang/feedflow-go-htmx/Dockerfile`:
```dockerfile
# syntax=docker/dockerfile:1.7
# feedflowのマルチステージビルドです。embed同梱の単一バイナリをdistrolessのnonrootで動かします。

# ---- build stage ----
FROM golang:1.25-bookworm AS build

ARG VERSION=dev
ARG TARGETOS=linux
ARG TARGETARCH=arm64

WORKDIR /src

# 依存解決を先に行いレイヤキャッシュを効かせます。
COPY go.mod go.sum* ./
RUN go mod download

# ソースとembed対象を投入します。
COPY . .

# 静的バイナリを生成します。CGO_ENABLED=0でdistroless上でも動きます。
RUN --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 GOOS="${TARGETOS}" GOARCH="${TARGETARCH}" \
    go build -trimpath -ldflags "-s -w -X main.version=${VERSION}" \
    -o /out/feedflow ./cmd/feedflow

# ---- runtime stage ----
FROM gcr.io/distroless/static-debian12:nonroot

WORKDIR /app

# データディレクトリはEBSをマウントするため空で用意します。nonrootが書き込めるようにします。
COPY --from=build --chown=nonroot:nonroot /out/feedflow /app/feedflow

USER nonroot:nonroot

EXPOSE 8080

ENV FEEDFLOW_ADDR=:8080
ENV FEEDFLOW_DATA_DIR=/data

ENTRYPOINT ["/app/feedflow"]
```

- [ ] Step 3: go.sumが無い場合に備える

`go.sum`はgolang.org/x依存を入れた後に生成されます。存在しない場合でも`go.sum*`のワイルドカードでCOPYが失敗しないようにしてあります。ローカルで存在を確認します。

Run:
```bash
ls go.sum 2>/dev/null && echo "go.sum exists" || echo "go.sum not present (ok if no external deps yet)"
```
Expected: いずれかのメッセージが表示されます。どちらでもDockerfileは成立します。

- [ ] Step 4: イメージをビルドして起動を確認する

Run:
```bash
docker build --build-arg VERSION=test --build-arg TARGETARCH=arm64 -t feedflow:test .
docker run --rm -d --name feedflow-smoke -p 18080:8080 -e FEEDFLOW_SESSION_KEY=smoke-key -e FEEDFLOW_DATA_DIR=/tmp feedflow:test
sleep 2
curl -fsS http://localhost:18080/healthz; echo
docker rm -f feedflow-smoke
```
Expected: `ok`と表示され、コンテナが正常に停止します。ローカルがamd64でビルドが遅い場合は`--build-arg TARGETARCH=amd64`に変えて疎通確認だけ行い、本番ARMビルドはEC2上で行います。

- [ ] Step 5: コミットする

```bash
git add Dockerfile .dockerignore
git commit -m "feat: embed同梱単一バイナリのマルチステージDockerfileを追加する"
```

---

## Task 4: nginxのTLSとmTLS設定

Files:
- Create: `/Users/yujiokamoto/devs/golang/feedflow-go-htmx/deploy/nginx/nginx.conf`
- Create: `/Users/yujiokamoto/devs/golang/feedflow-go-htmx/deploy/nginx/conf.d/feedflow.conf`

- [ ] Step 1: nginx本体設定を作成する

Create `/Users/yujiokamoto/devs/golang/feedflow-go-htmx/deploy/nginx/nginx.conf`:
```nginx
# feedflow前段のnginx本体設定です。TLS終端とmTLSとfeedflowアプリへのリバースプロキシを担います。
user  nginx;
worker_processes  auto;

error_log  /var/log/nginx/error.log warn;
pid        /var/run/nginx.pid;

events {
    worker_connections  1024;
}

http {
    include       /etc/nginx/mime.types;
    default_type  application/octet-stream;

    log_format main '$remote_addr - $remote_user [$time_local] "$request" '
                    '$status $body_bytes_sent "$http_referer" '
                    '"$http_user_agent" client_verify=$ssl_client_verify '
                    'client_dn="$ssl_client_s_dn"';

    access_log  /var/log/nginx/access.log  main;

    sendfile        on;
    tcp_nopush      on;
    keepalive_timeout  65;

    # リクエストボディの上限です。OPMLインポートを許容しつつ過大を拒否します。
    client_max_body_size 8m;

    # アプリへ送るレスポンスの圧縮はアプリ側に任せnginx側では最小限とします。
    gzip on;
    gzip_types text/css application/javascript application/json text/plain;
    gzip_min_length 1024;

    include /etc/nginx/conf.d/*.conf;
}
```

- [ ] Step 2: feedflowのサーバ設定を作成する

80番は443へリダイレクトしつつ、Let's Encryptのhttp-01チャレンジ用に`/.well-known/acme-challenge/`だけは平文で配ります。443でTLS終端し、クライアント証明書を必須検証してからアプリへプロキシします。証明書を持たない接続は403で拒否します。

Create `/Users/yujiokamoto/devs/golang/feedflow-go-htmx/deploy/nginx/conf.d/feedflow.conf`:
```nginx
# feedflowのサーバブロックです。TLS終端とmTLSクライアント証明書検証を行います。
# server_nameはデプロイ時に実ドメインへ置き換えます。

upstream feedflow_app {
    # composeのサービス名appで名前解決します。アプリは内部ネットワークの8080を待ち受けます。
    server app:8080;
    keepalive 16;
}

# HTTPはACMEチャレンジ以外をHTTPSへ恒久リダイレクトします。
server {
    listen 80;
    listen [::]:80;
    server_name feedflow.example.com;

    location /.well-known/acme-challenge/ {
        root /var/www/certbot;
    }

    location / {
        return 301 https://$host$request_uri;
    }
}

# HTTPS終端とmTLSです。
server {
    listen 443 ssl;
    listen [::]:443 ssl;
    http2 on;
    server_name feedflow.example.com;

    # Let's Encryptのサーバ証明書です。
    ssl_certificate     /etc/letsencrypt/live/feedflow.example.com/fullchain.pem;
    ssl_certificate_key /etc/letsencrypt/live/feedflow.example.com/privkey.pem;

    ssl_protocols       TLSv1.2 TLSv1.3;
    ssl_ciphers         ECDHE-ECDSA-AES128-GCM-SHA256:ECDHE-RSA-AES128-GCM-SHA256:ECDHE-ECDSA-AES256-GCM-SHA384:ECDHE-RSA-AES256-GCM-SHA384;
    ssl_prefer_server_ciphers on;
    ssl_session_cache   shared:SSL:10m;
    ssl_session_timeout 1d;
    ssl_session_tickets off;

    # mTLSです。自前ローカルCAを信頼ルートにしクライアント証明書を必須にします。
    ssl_client_certificate /etc/nginx/mtls/ca.crt;
    ssl_verify_client on;
    ssl_verify_depth 1;

    # セキュリティヘッダです。アプリ側でも付与しますが前段でも保証します。
    add_header Strict-Transport-Security "max-age=31536000; includeSubDomains" always;
    add_header X-Content-Type-Options "nosniff" always;
    add_header Referrer-Policy "strict-origin-when-cross-origin" always;
    add_header X-Frame-Options "DENY" always;

    # クライアント証明書の検証に失敗した場合は403で拒否します。
    if ($ssl_client_verify != SUCCESS) {
        return 403;
    }

    location / {
        proxy_pass http://feedflow_app;
        proxy_http_version 1.1;
        proxy_set_header Host              $host;
        proxy_set_header X-Real-IP         $remote_addr;
        proxy_set_header X-Forwarded-For   $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto https;
        proxy_set_header X-Forwarded-Host  $host;
        proxy_set_header Connection        "";
        # クライアント証明書のDNを下流へ渡します。アプリのログ突合に使えます。
        proxy_set_header X-Client-DN       $ssl_client_s_dn;
        proxy_read_timeout 60s;
    }
}
```

- [ ] Step 3: nginx設定の構文をローカルで検証する

ローカルにnginxイメージがあれば、ボリュームをマウントして構文だけ検証できます。証明書ファイルは存在しないため`-t`は証明書読み込みで止まる場合があります。その場合は構文確認をスキップし、Task8のverify-deploy.shとEC2上の`docker compose exec nginx nginx -t`で確認します。

Run:
```bash
docker run --rm \
  -v "$PWD/deploy/nginx/nginx.conf:/etc/nginx/nginx.conf:ro" \
  -v "$PWD/deploy/nginx/conf.d:/etc/nginx/conf.d:ro" \
  nginx:1.27-alpine nginx -t 2>&1 | grep -E "syntax|emerg" || true
```
Expected: 証明書ファイルが無いため`cannot load certificate`の警告が出ますが、`syntax is ok`相当の構文検証が先に走ります。本番では証明書配置後に`nginx -t`が成功します。

- [ ] Step 4: コミットする

```bash
git add deploy/nginx/nginx.conf deploy/nginx/conf.d/feedflow.conf
git commit -m "feat: nginxのTLS終端とmTLS設定を追加する"
```

---

## Task 5: compose.ymlでnginxとアプリを同居させる

Files:
- Create: `/Users/yujiokamoto/devs/golang/feedflow-go-htmx/compose.yml`

- [ ] Step 1: compose.ymlを作成する

nginxコンテナとアプリコンテナを内部ネットワークで結びます。アプリのポートはホストへ公開せず、nginxの443と80だけをホストへ出します。dataディレクトリはEBSのマウント先をバインドします。証明書はホスト側のディレクトリをnginxへ読み取り専用でマウントします。

Create `/Users/yujiokamoto/devs/golang/feedflow-go-htmx/compose.yml`:
```yaml
# feedflowのデプロイ用composeです。nginx前段とアプリを同居させALBとNLBは使いません。
# アプリは内部ネットワークのみに露出しホストへ出すのはnginxの80と443だけとします。

services:
  app:
    build:
      context: .
      dockerfile: Dockerfile
      args:
        VERSION: ${FEEDFLOW_VERSION:-dev}
        TARGETARCH: arm64
    image: feedflow:${FEEDFLOW_VERSION:-dev}
    restart: unless-stopped
    environment:
      FEEDFLOW_ADDR: ":8080"
      FEEDFLOW_DATA_DIR: "/data"
      FEEDFLOW_BASE_URL: "${FEEDFLOW_BASE_URL:?set FEEDFLOW_BASE_URL}"
      FEEDFLOW_SESSION_KEY: "${FEEDFLOW_SESSION_KEY:?set FEEDFLOW_SESSION_KEY}"
    volumes:
      # EBSをマウントしたホストの/mnt/feedflow-dataを永続化先にします。
      - /mnt/feedflow-data:/data
    networks:
      - internal
    expose:
      - "8080"
    # ホストへはポートを公開しません。到達はnginx経由のみとします。

  nginx:
    image: nginx:1.27-alpine
    restart: unless-stopped
    depends_on:
      - app
    ports:
      - "80:80"
      - "443:443"
    volumes:
      - ./deploy/nginx/nginx.conf:/etc/nginx/nginx.conf:ro
      - ./deploy/nginx/conf.d:/etc/nginx/conf.d:ro
      # Let's Encryptの証明書ストアです。certbotが書き込みnginxが読みます。
      - /etc/letsencrypt:/etc/letsencrypt:ro
      # mTLSのローカルCA証明書です。
      - /etc/feedflow/mtls:/etc/nginx/mtls:ro
      # ACME http-01チャレンジ用のwebルートです。
      - /var/www/certbot:/var/www/certbot:ro
    networks:
      - internal

networks:
  internal:
    driver: bridge
```

- [ ] Step 2: compose設定の妥当性を検証する

`FEEDFLOW_BASE_URL`と`FEEDFLOW_SESSION_KEY`は必須変数のため、検証時はダミー値を与えて`config`を実行します。

Run:
```bash
FEEDFLOW_BASE_URL=https://feedflow.example.com FEEDFLOW_SESSION_KEY=dummy \
  docker compose -f compose.yml config >/dev/null && echo "compose config ok"
```
Expected: `compose config ok`と表示されます。変数未設定だと`set FEEDFLOW_BASE_URL`などのエラーで止まり、必須化が効いていることを確認できます。

- [ ] Step 3: コミットする

```bash
git add compose.yml
git commit -m "feat: nginxとアプリを同居させるcompose.ymlを追加する"
```

---

## Task 6: TLS証明書取得とmTLSクライアント証明書の作成スクリプト

Files:
- Create: `/Users/yujiokamoto/devs/golang/feedflow-go-htmx/deploy/scripts/issue-tls-cert.sh`
- Create: `/Users/yujiokamoto/devs/golang/feedflow-go-htmx/deploy/scripts/make-mtls-ca.sh`
- Create: `/Users/yujiokamoto/devs/golang/feedflow-go-htmx/deploy/scripts/make-client-cert.sh`

- [ ] Step 1: Let's EncryptのTLS証明書取得スクリプトを作成する

http-01チャレンジで初回取得し、以降は同じwebrootで更新します。実行前に80番が到達可能でドメインがEC2のEIPを指している必要があります。

Create `/Users/yujiokamoto/devs/golang/feedflow-go-htmx/deploy/scripts/issue-tls-cert.sh`:
```bash
#!/usr/bin/env bash
# issue-tls-cert.sh
# Let's Encryptのサーバ証明書をhttp-01チャレンジで取得します。
# 前提として80番が到達可能でDOMAINがEC2のEIPを指していることが必要です。
set -euo pipefail

DOMAIN="${1:?usage: issue-tls-cert.sh <domain> <email>}"
EMAIL="${2:?usage: issue-tls-cert.sh <domain> <email>}"
WEBROOT="/var/www/certbot"

sudo mkdir -p "$WEBROOT"

# certbotをDockerで実行しwebroot方式で取得します。
sudo docker run --rm \
  -v /etc/letsencrypt:/etc/letsencrypt \
  -v "$WEBROOT:$WEBROOT" \
  certbot/certbot certonly \
  --webroot -w "$WEBROOT" \
  -d "$DOMAIN" \
  --email "$EMAIL" \
  --agree-tos --no-eff-email --non-interactive

echo "発行済み証明書のパスは次のとおりです /etc/letsencrypt/live/$DOMAIN/fullchain.pem"
echo "次のコマンドでnginxをリロードして反映します sudo docker compose exec nginx nginx -s reload"
```

- [ ] Step 2: mTLSのローカルCAを作成するスクリプトを作成する

クライアント証明書を発行するための自前CAを作ります。CA秘密鍵は厳重に保管し、リポジトリには絶対に置きません。出力先はホストの`/etc/feedflow/mtls`です。

Create `/Users/yujiokamoto/devs/golang/feedflow-go-htmx/deploy/scripts/make-mtls-ca.sh`:
```bash
#!/usr/bin/env bash
# make-mtls-ca.sh
# mTLSクライアント証明書を発行するための自前ローカルCAを作成します。
# CA秘密鍵はリポジトリへ置かずホストの保護ディレクトリにだけ保管します。
set -euo pipefail

OUT_DIR="${1:-/etc/feedflow/mtls}"
DAYS="${2:-3650}"

sudo mkdir -p "$OUT_DIR"
sudo chmod 700 "$OUT_DIR"

# CA秘密鍵を生成します。
sudo openssl genrsa -out "$OUT_DIR/ca.key" 4096
sudo chmod 600 "$OUT_DIR/ca.key"

# 自己署名のCA証明書を生成します。
sudo openssl req -x509 -new -nodes \
  -key "$OUT_DIR/ca.key" \
  -sha256 -days "$DAYS" \
  -subj "/CN=feedflow-mtls-ca/O=feedflow" \
  -out "$OUT_DIR/ca.crt"

echo "CA証明書をnginxのssl_client_certificateに指定します パスは $OUT_DIR/ca.crt"
echo "CA秘密鍵は絶対に配布しません パスは $OUT_DIR/ca.key"
```

- [ ] Step 3: クライアント証明書の作成と配布用スクリプトを作成する

CAでクライアント証明書を発行し、ブラウザへ取り込めるPKCS#12(.p12)を生成します。配布時はこの.p12と取り込みパスフレーズだけを所有者へ渡します。

Create `/Users/yujiokamoto/devs/golang/feedflow-go-htmx/deploy/scripts/make-client-cert.sh`:
```bash
#!/usr/bin/env bash
# make-client-cert.sh
# ローカルCAでクライアント証明書を発行しブラウザ取り込み用のPKCS#12を生成します。
# 出力する.p12とパスフレーズだけを所有者へ安全に配布します。
set -euo pipefail

CA_DIR="${1:-/etc/feedflow/mtls}"
CLIENT_NAME="${2:?usage: make-client-cert.sh <ca_dir> <client_name> <out_dir>}"
OUT_DIR="${3:-./client-certs}"
DAYS="${4:-825}"

mkdir -p "$OUT_DIR"

# クライアント秘密鍵を生成します。
openssl genrsa -out "$OUT_DIR/$CLIENT_NAME.key" 2048

# CSRを生成します。
openssl req -new \
  -key "$OUT_DIR/$CLIENT_NAME.key" \
  -subj "/CN=$CLIENT_NAME/O=feedflow-client" \
  -out "$OUT_DIR/$CLIENT_NAME.csr"

# CAで署名しクライアント証明書を発行します。
sudo openssl x509 -req \
  -in "$OUT_DIR/$CLIENT_NAME.csr" \
  -CA "$CA_DIR/ca.crt" -CAkey "$CA_DIR/ca.key" \
  -CAcreateserial \
  -days "$DAYS" -sha256 \
  -out "$OUT_DIR/$CLIENT_NAME.crt"

# ブラウザ取り込み用のPKCS#12を生成します。取り込みパスフレーズを対話入力します。
openssl pkcs12 -export \
  -inkey "$OUT_DIR/$CLIENT_NAME.key" \
  -in "$OUT_DIR/$CLIENT_NAME.crt" \
  -certfile "$CA_DIR/ca.crt" \
  -name "feedflow client $CLIENT_NAME" \
  -out "$OUT_DIR/$CLIENT_NAME.p12"

echo "配布物のパスは次のとおりです $OUT_DIR/$CLIENT_NAME.p12"
echo "取り込み手順はブラウザの証明書管理から個人証明書として.p12をインポートしパスフレーズを入力します"
echo "csrとkeyは配布せずローカルで破棄してよいです"
```

- [ ] Step 4: 実行権限を付与する

Run:
```bash
chmod +x deploy/scripts/issue-tls-cert.sh deploy/scripts/make-mtls-ca.sh deploy/scripts/make-client-cert.sh
```
Expected: エラーなく完了します。

- [ ] Step 5: スクリプトの構文をローカルで検証する

ローカルでCAとクライアント証明書を一時ディレクトリへ作り、生成物が揃うことと、発行した証明書がCAで検証できることを確認します。ここではsudo不要にするため一時ディレクトリで直接opensslを実行します。

Run:
```bash
bash -n deploy/scripts/issue-tls-cert.sh deploy/scripts/make-mtls-ca.sh deploy/scripts/make-client-cert.sh && echo "syntax ok"
TMP="$(mktemp -d)"
openssl genrsa -out "$TMP/ca.key" 2048
openssl req -x509 -new -nodes -key "$TMP/ca.key" -sha256 -days 1 -subj "/CN=test-ca" -out "$TMP/ca.crt"
openssl genrsa -out "$TMP/cli.key" 2048
openssl req -new -key "$TMP/cli.key" -subj "/CN=test-client" -out "$TMP/cli.csr"
openssl x509 -req -in "$TMP/cli.csr" -CA "$TMP/ca.crt" -CAkey "$TMP/ca.key" -CAcreateserial -days 1 -sha256 -out "$TMP/cli.crt"
openssl verify -CAfile "$TMP/ca.crt" "$TMP/cli.crt"
rm -rf "$TMP"
```
Expected: `syntax ok`の後、最後に`$TMP/cli.crt: OK`と表示され、CAでクライアント証明書が検証できることを確認できます。

- [ ] Step 6: コミットする

```bash
git add deploy/scripts/issue-tls-cert.sh deploy/scripts/make-mtls-ca.sh deploy/scripts/make-client-cert.sh
git commit -m "feat: TLS証明書取得とmTLSクライアント証明書の作成スクリプトを追加する"
```

---

## Task 7: EC2デプロイ手順とSecurity Group定義

Files:
- Create: `/Users/yujiokamoto/devs/golang/feedflow-go-htmx/deploy/README.md`
- Create: `/Users/yujiokamoto/devs/golang/feedflow-go-htmx/deploy/security-group.json`

- [ ] Step 1: Security Group定義を作成する

443とSSHだけを開放します。SSHは運用元のIPに限定する想定で、配布する定義のCIDRはプレースホルダではなく明示の例値`203.0.113.10/32`を置き、利用時にこの行のIPだけを自分のグローバルIPへ書き換えます。443は全世界へ開放しますが、到達後はmTLSで弾くため証明書なしの接続は通りません。

Create `/Users/yujiokamoto/devs/golang/feedflow-go-htmx/deploy/security-group.json`:
```json
{
  "GroupName": "feedflow-sg",
  "Description": "feedflow inbound rules. 443 for mTLS terminated by nginx and SSH from operator IP only.",
  "IngressRules": [
    {
      "IpProtocol": "tcp",
      "FromPort": 443,
      "ToPort": 443,
      "IpRanges": [
        {
          "CidrIp": "0.0.0.0/0",
          "Description": "HTTPS. mTLS by nginx rejects clients without a certificate."
        }
      ]
    },
    {
      "IpProtocol": "tcp",
      "FromPort": 80,
      "ToPort": 80,
      "IpRanges": [
        {
          "CidrIp": "0.0.0.0/0",
          "Description": "HTTP for ACME http-01 challenge and redirect to HTTPS only."
        }
      ]
    },
    {
      "IpProtocol": "tcp",
      "FromPort": 22,
      "ToPort": 22,
      "IpRanges": [
        {
          "CidrIp": "203.0.113.10/32",
          "Description": "SSH from operator IP only. Replace with your global IP /32."
        }
      ]
    }
  ],
  "EgressRules": [
    {
      "IpProtocol": "-1",
      "IpRanges": [
        {
          "CidrIp": "0.0.0.0/0",
          "Description": "All outbound. Required for feed polling and Let's Encrypt."
        }
      ]
    }
  ]
}
```
補足: 設計書のSecurity Groupは443とSSHのみという要件に対し、80はLet's Encryptのhttp-01チャレンジと443への恒久リダイレクトのためだけに開けます。443で待ち受けるサービス本体はmTLSで保護され、80はチャレンジ応答とリダイレクトしか返さないため、保護対象のアプリ面は443のみで提供されます。証明書をDNS-01などへ切り替えて80を閉じる運用も可能です。

- [ ] Step 2: デプロイ手順READMEを作成する

Create `/Users/yujiokamoto/devs/golang/feedflow-go-htmx/deploy/README.md`:
```markdown
# feedflow デプロイ手順

単一EC2(ARMのt4g系)とEBS、nginxコンテナとGoアプリコンテナの同居でfeedflowを公開します。ALBとNLBは使いません。前段のnginxでTLS終端とmTLSを行い、所有者だけがアクセスできるようにします。

## 構成図

```
[ブラウザとクライアント証明書]
        | 443でTLSとmTLS
        v
[EC2 t4g] -- compose --+-- nginx(443終端 / mTLS検証 / リバースプロキシ)
                       |        | 内部ネットワーク8080
                       +-- app(feedflow単一バイナリ / embed同梱)
                                | /data
                       [EBSマウント /mnt/feedflow-data]
```

## 1 EC2とEBSの準備

1. ARMのt4gインスタンスをAmazon Linux 2023のARM版で起動します。インスタンスタイプはt4g.smallなどを選びます
2. EIPを割り当てて固定し、DNSのAレコードを`feedflow.example.com`からこのEIPへ向けます
3. データ用のEBSボリュームを作成しインスタンスへアタッチします
4. EBSをフォーマットして`/mnt/feedflow-data`へマウントします

```bash
# デバイス名は環境で異なるためlsblkで確認します
lsblk
sudo mkfs -t ext4 /dev/nvme1n1
sudo mkdir -p /mnt/feedflow-data
sudo mount /dev/nvme1n1 /mnt/feedflow-data
# 再起動後も維持するためfstabへ追記します
echo "/dev/nvme1n1 /mnt/feedflow-data ext4 defaults,nofail 0 2" | sudo tee -a /etc/fstab
```

## 2 Security Group

`deploy/security-group.json`の内容で受信規則を設定します。443は全世界へ開きますがmTLSで証明書なしを拒否します。80はLet's Encryptのhttp-01チャレンジと443へのリダイレクトのみに使います。SSHは運用元のIPの/32だけに限定し、JSON内の`203.0.113.10/32`を自分のグローバルIPへ書き換えます。

```bash
MYIP="$(curl -fsS https://checkip.amazonaws.com)"
echo "SSHを許可する自分のIPを表示します $MYIP/32 をsecurity-group.jsonの203.0.113.10/32と置換します"
```

## 3 Dockerとcomposeの導入

```bash
sudo dnf install -y docker
sudo systemctl enable --now docker
sudo usermod -aG docker ec2-user
# composeプラグインを導入します
sudo mkdir -p /usr/libexec/docker/cli-plugins
ARCH="$(uname -m)"
sudo curl -fsSL "https://github.com/docker/compose/releases/latest/download/docker-compose-linux-${ARCH}" \
  -o /usr/libexec/docker/cli-plugins/docker-compose
sudo chmod +x /usr/libexec/docker/cli-plugins/docker-compose
docker compose version
```

## 4 ソース配置と環境変数

```bash
git clone https://github.com/okamyuji/feedflow-go-htmx.git
cd feedflow-go-htmx
cp .env.example .env
# .envを編集します。最低でも次を設定します
#   FEEDFLOW_BASE_URL=https://feedflow.example.com
#   FEEDFLOW_SESSION_KEY=$(openssl rand -base64 32)
export FEEDFLOW_BASE_URL=https://feedflow.example.com
export FEEDFLOW_SESSION_KEY="$(openssl rand -base64 32)"
```

## 5 mTLSのCAとクライアント証明書

```bash
# CAを作成します。出力は/etc/feedflow/mtlsです
sudo bash deploy/scripts/make-mtls-ca.sh /etc/feedflow/mtls
# 自分用のクライアント証明書を発行します
bash deploy/scripts/make-client-cert.sh /etc/feedflow/mtls okamyuji ./client-certs
# 生成された./client-certs/okamyuji.p12をローカル端末へscpで取得しブラウザへ取り込みます
```

クライアント証明書の取り込み手順です。

- macOSのChromeとSafariはキーチェーンアクセスへ.p12をインポートし、発行時のパスフレーズを入力します
- Firefoxは設定の証明書マネージャの個人タブから.p12をインポートします
- iOSは構成プロファイルとして.p12を取り込みます

CA秘密鍵`/etc/feedflow/mtls/ca.key`は配布せず、サーバ上だけに保管します。

## 6 TLSサーバ証明書

初回はnginxが443で起動できるよう、先に80だけでチャレンジを通します。`deploy/nginx/conf.d/feedflow.conf`の`server_name`を実ドメインへ書き換えてから取得します。

```bash
# server_nameを実ドメインへ置換します
sed -i 's/feedflow.example.com/your-real-domain.example/g' deploy/nginx/conf.d/feedflow.conf
# 証明書を取得します
sudo bash deploy/scripts/issue-tls-cert.sh your-real-domain.example you@example.com
```

## 7 起動

```bash
docker compose up -d --build
docker compose ps
# nginxの設定を検証します
docker compose exec nginx nginx -t
```

## 8 動作確認

```bash
# 証明書なしは403で拒否されます
curl -k -sS -o /dev/null -w "%{http_code}\n" https://your-real-domain.example/
# 期待値は403です

# クライアント証明書つきは到達します
curl --cert ./client-certs/okamyuji.crt --key ./client-certs/okamyuji.key \
  -sS -o /dev/null -w "%{http_code}\n" https://your-real-domain.example/healthz
# 期待値は200です
```

## 9 証明書の更新

Let's Encryptは90日で失効するため定期更新します。cronで月次更新しnginxをリロードします。

```bash
( sudo crontab -l 2>/dev/null; \
  echo "0 3 1 * * cd $HOME/feedflow-go-htmx && bash deploy/scripts/issue-tls-cert.sh your-real-domain.example you@example.com && docker compose exec -T nginx nginx -s reload" ) \
  | sudo crontab -
```

## 10 バックアップ

dataディレクトリはEBS上にあります。EBSスナップショットを定期取得します。アプリは全データをメモリ常駐しつつ`os.Rename`でアトミックにJSONへ書き込むため、スナップショット時点の整合は保たれます。
```

- [ ] Step 3: Security Group定義が妥当なJSONか検証する

Run:
```bash
python3 -c "import json,sys; d=json.load(open('deploy/security-group.json')); ports=sorted(r['FromPort'] for r in d['IngressRules']); print('ingress ports:', ports); assert ports==[22,80,443], ports; print('security-group.json ok')"
```
Expected: `ingress ports: [22, 80, 443]`と`security-group.json ok`が表示されます。

- [ ] Step 4: コミットする

```bash
git add deploy/README.md deploy/security-group.json
git commit -m "docs: EC2デプロイ手順とSecurity Group定義を追加する"
```

---

## Task 8: デプロイ構成検証スクリプト(TDD相当)

Files:
- Create: `/Users/yujiokamoto/devs/golang/feedflow-go-htmx/scripts/verify-deploy.sh`

- [ ] Step 1: 検証スクリプトを作成する

デプロイ成果物が要件を満たすことを機械的に確認します。Dockerfileがマルチステージかつ静的バイナリを作ること、composeがアプリポートをホストへ公開していないこと、nginxがmTLSを必須にしていること、Security Groupが443とSSHと80に限られることを検査します。

Create `/Users/yujiokamoto/devs/golang/feedflow-go-htmx/scripts/verify-deploy.sh`:
```bash
#!/usr/bin/env bash
# verify-deploy.sh
# デプロイ構成が設計要件を満たすことを機械的に検証します。
# 依存はbashとgrepとpython3のみです。外部ホストへは接続しません。
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"

YELLOW='\033[33m'
GREEN='\033[32m'
RED='\033[31m'
NC='\033[0m'

pass=0
fail=0

check() {
  local name="$1"
  shift
  printf "${YELLOW}>>> %s${NC}\n" "$name"
  if "$@"; then
    printf "${GREEN}    PASS${NC}\n"
    pass=$((pass+1))
  else
    printf "${RED}    FAIL${NC}\n"
    fail=$((fail+1))
  fi
}

dockerfile_multistage() {
  [ "$(grep -c '^FROM ' Dockerfile)" -ge 2 ]
}

dockerfile_static_build() {
  grep -q 'CGO_ENABLED=0' Dockerfile
}

dockerfile_runtime_distroless() {
  grep -q 'gcr.io/distroless/static' Dockerfile
}

compose_app_not_published() {
  # appサービスがportsでホストへ公開していないことを確認します。
  ! python3 - <<'PY'
import sys, re
text = open("compose.yml").read()
# appサービスブロックをservices直下からnginxの前まで切り出します。
m = re.search(r"\n  app:\n(.*?)\n  nginx:", text, re.S)
block = m.group(1) if m else ""
sys.exit(0 if re.search(r"^\s{4}ports:", block, re.M) else 1)
PY
}

compose_nginx_publishes_443() {
  python3 - <<'PY'
import re, sys
text = open("compose.yml").read()
m = re.search(r"\n  nginx:\n(.*?)(\nnetworks:|\Z)", text, re.S)
block = m.group(1) if m else ""
sys.exit(0 if '"443:443"' in block else 1)
PY
}

nginx_requires_mtls() {
  grep -q 'ssl_verify_client on;' deploy/nginx/conf.d/feedflow.conf \
    && grep -q 'ssl_client_certificate' deploy/nginx/conf.d/feedflow.conf
}

nginx_rejects_no_cert() {
  grep -q 'ssl_client_verify != SUCCESS' deploy/nginx/conf.d/feedflow.conf
}

sg_only_expected_ports() {
  python3 - <<'PY'
import json, sys
d = json.load(open("deploy/security-group.json"))
ports = sorted(r["FromPort"] for r in d["IngressRules"])
sys.exit(0 if ports == [22, 80, 443] else 1)
PY
}

scripts_executable() {
  [ -x deploy/scripts/make-mtls-ca.sh ] \
    && [ -x deploy/scripts/make-client-cert.sh ] \
    && [ -x deploy/scripts/issue-tls-cert.sh ]
}

check "Dockerfileがマルチステージである" dockerfile_multistage
check "DockerfileがCGO_ENABLED=0の静的ビルドである" dockerfile_static_build
check "Dockerfileのランタイムがdistroless staticである" dockerfile_runtime_distroless
check "composeのappがホストへポート公開していない" compose_app_not_published
check "composeのnginxが443を公開している" compose_nginx_publishes_443
check "nginxがmTLSを必須にしている" nginx_requires_mtls
check "nginxが証明書なしを拒否する" nginx_rejects_no_cert
check "Security Groupが22と80と443のみである" sg_only_expected_ports
check "証明書スクリプトに実行権限がある" scripts_executable

echo
echo "----------------------------------------"
printf "passed: ${GREEN}%d${NC}  failed: ${RED}%d${NC}\n" "$pass" "$fail"
echo "----------------------------------------"
if [ "$fail" -gt 0 ]; then
  exit 1
fi
```

- [ ] Step 2: 実行権限を付与する

Run:
```bash
chmod +x scripts/verify-deploy.sh
```
Expected: エラーなく完了します。

- [ ] Step 3: 検証スクリプトを実行する

Run:
```bash
bash scripts/verify-deploy.sh
```
Expected: 9件すべてがPASSし、末尾に`passed: 9  failed: 0`と表示されます。

- [ ] Step 4: コミットする

```bash
git add scripts/verify-deploy.sh
git commit -m "test: デプロイ構成を検証するverify-deploy.shを追加する"
```

---

## Task 9: CIにデプロイ構成検証を組み込む

Files:
- Modify: `/Users/yujiokamoto/devs/golang/feedflow-go-htmx/.github/workflows/ci.yml`
- Modify: `/Users/yujiokamoto/devs/golang/feedflow-go-htmx/.env.example`

- [ ] Step 1: .env.exampleにデプロイ向けの変数を追記する

Edit `/Users/yujiokamoto/devs/golang/feedflow-go-htmx/.env.example`の末尾に次の3行を追記します。
```
# デプロイ時のイメージタグでcomposeのFEEDFLOW_VERSIONに対応します
FEEDFLOW_VERSION=dev

# nginxが直接TLSを扱わずアプリでTLSを終端する場合にのみ使います。通常は空でcomposeではnginxが終端します
FEEDFLOW_TLS_CERT_FILE=
FEEDFLOW_TLS_KEY_FILE=
```

- [ ] Step 2: CIにverify-deployステップを追加する

Edit `/Users/yujiokamoto/devs/golang/feedflow-go-htmx/.github/workflows/ci.yml`の`Run hardening verification`ステップの直後に、次のステップを追加します。
```yaml
      - name: Verify deploy configuration
        run: bash scripts/verify-deploy.sh
```

- [ ] Step 3: CIのYAMLが妥当か確認する

Run:
```bash
python3 -c "import yaml,sys; yaml.safe_load(open('.github/workflows/ci.yml')); print('ci.yml yaml ok')"
```
Expected: `ci.yml yaml ok`と表示されます。pyyamlが無い場合は`pip install pyyaml`を実行してから再試行します。

- [ ] Step 4: 品質ゲートとデプロイ検証をローカルで通す

Run:
```bash
bash scripts/quality-gate.sh
bash scripts/verify-deploy.sh
```
Expected: 品質ゲートは`all quality checks passed`で終わり、デプロイ検証は`passed: 9  failed: 0`で終わります。

- [ ] Step 5: コミットする

```bash
git add .github/workflows/ci.yml .env.example
git commit -m "ci: デプロイ構成検証をCIへ組み込む"
```

---

## Task 10: ローカルmTLS疎通の統合確認

Files:
- 変更なし(既存の構成ファイルとスクリプトを使ったエンドツーエンドの疎通確認)

このタスクはEC2を使わず、ローカルのDockerだけでnginxのmTLSが証明書なしを拒否し、証明書ありを通すことを確認します。Let's Encryptの代わりに自己署名のサーバ証明書を一時生成して使います。

- [ ] Step 1: 一時的なサーバ証明書とCAとクライアント証明書を作る

Run:
```bash
WORK="$(mktemp -d)"; export WORK
# サーバ証明書です。自己署名でlocalhost用です
openssl req -x509 -newkey rsa:2048 -nodes -keyout "$WORK/server.key" -out "$WORK/server.crt" \
  -days 2 -subj "/CN=localhost" -addext "subjectAltName=DNS:localhost"
# mTLSのCAです
openssl genrsa -out "$WORK/ca.key" 2048
openssl req -x509 -new -nodes -key "$WORK/ca.key" -sha256 -days 2 -subj "/CN=local-mtls-ca" -out "$WORK/ca.crt"
# クライアント証明書
openssl genrsa -out "$WORK/client.key" 2048
openssl req -new -key "$WORK/client.key" -subj "/CN=local-client" -out "$WORK/client.csr"
openssl x509 -req -in "$WORK/client.csr" -CA "$WORK/ca.crt" -CAkey "$WORK/ca.key" -CAcreateserial -days 2 -sha256 -out "$WORK/client.crt"
echo "WORK=$WORK"
```
Expected: 各opensslコマンドが成功し、`WORK=`に一時ディレクトリのパスが表示されます。

- [ ] Step 2: localhost検証用の一時nginx設定を作る

Run:
```bash
cat > "$WORK/test.conf" <<'EOF'
server {
    listen 443 ssl;
    server_name localhost;
    ssl_certificate     /certs/server.crt;
    ssl_certificate_key /certs/server.key;
    ssl_client_certificate /certs/ca.crt;
    ssl_verify_client on;
    if ($ssl_client_verify != SUCCESS) { return 403; }
    location / { return 200 "mtls ok\n"; }
}
EOF
echo "wrote $WORK/test.conf"
```
Expected: `wrote`に続けてパスが表示されます。この設定は本番の`feedflow.conf`と同じmTLSディレクティブ(ssl_client_certificate、ssl_verify_client on、ssl_client_verifyによる403)を使い、本番設定の挙動を代表します。

- [ ] Step 3: nginxを一時起動する

Run:
```bash
docker run --rm -d --name feedflow-mtls-test -p 8443:443 \
  -v "$WORK:/certs:ro" \
  -v "$WORK/test.conf:/etc/nginx/conf.d/default.conf:ro" \
  nginx:1.27-alpine
sleep 2
docker exec feedflow-mtls-test nginx -t
```
Expected: `nginx: configuration file /etc/nginx/nginx.conf test is successful`が表示されます。

- [ ] Step 4: 証明書なしが拒否されることを確認する

Run:
```bash
curl -k -sS -o /dev/null -w "no-cert status: %{http_code}\n" https://localhost:8443/ || true
```
Expected: `no-cert status: 400`または`no-cert status: 403`が表示されます。nginxはクライアント証明書を要求し、提示が無い接続を拒否します。

- [ ] Step 5: 証明書ありが通ることを確認する

Run:
```bash
curl -k --cert "$WORK/client.crt" --key "$WORK/client.key" \
  -sS -w "\nwith-cert status: %{http_code}\n" https://localhost:8443/
```
Expected: `mtls ok`と`with-cert status: 200`が表示されます。

- [ ] Step 6: 後始末する

Run:
```bash
docker rm -f feedflow-mtls-test
rm -rf "$WORK"
unset WORK
```
Expected: コンテナが削除され、一時ディレクトリが消えます。

このタスクはコード変更を伴わないため、コミットはありません。確認結果をデプロイ手順の妥当性の裏付けとします。

---

## Phase8 完了条件

- [ ] `go test ./cmd/feedflow/ -v`が通り、Config構造体と認証アダプタのテストがPASSする
- [ ] `buildHandler`がstoreとfeedとserviceとpollerとauthの具象を組み立て、認証アダプタを介してhandler.Depsへ注入する
- [ ] handler.Depsのどのフィールドもnilのままにせず、未認証で認証必須ルートを叩いてもpanicせず/loginへリダイレクトすることをテストで担保する
- [ ] `docker build`でembed同梱の単一バイナリイメージが作れ、`/healthz`が`ok`を返す
- [ ] `docker compose config`がダミーの必須変数つきで通る
- [ ] `bash scripts/verify-deploy.sh`が`passed: 9  failed: 0`で終わる
- [ ] Task10のローカルmTLS疎通で、証明書なしが拒否され証明書ありが通ることを確認した
- [ ] `bash scripts/quality-gate.sh`が`all quality checks passed`で終わる
- [ ] コミットが規約に沿って積まれている
