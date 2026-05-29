# Phase6 認証とセキュリティ 実装計画

> For agentic workers: REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

Goal: feedflowのアプリ層の認証とセキュリティを完成させます。golang.org/x/crypto/scryptでパスワードをハッシュ化して検証し、メモリ保持のCookieセッション(HttpOnlyとSecureとSameSite=Lax)とCSRFトークンと標準ライブラリによる簡易トークンバケットのレートリミットを実装します。初回セットアップの可否判定はuser.json未登録時のみ到達できるようにし、セキュリティヘッダを付与するミドルウェア関数を用意します。

Architecture: 認証とセキュリティはinternal/authパッケージに閉じ込めます。永続化はPhase2のport.Repository経由でuser.jsonを読み書きします。authパッケージはport.Repositoryとport.Clockとport.IDGenのインターフェースにコンストラクタ注入で依存し、具象型に直接依存しません。時刻と乱数源とID生成を抽象化することでフェイク注入によりI/Oと非決定性に触れずにユニットテストできます。Phase7のハンドラはここで提供するManagerとセッション、CSRF、レートリミット、セットアップ判定、セキュリティヘッダのミドルウェアをインターフェースまたは具象として受け取って組み立てます。

Tech Stack: Go(標準ライブラリのnet/httpとcrypto/randとcrypto/subtleとcrypto/sha256とencoding/base64とsync)、golang.org/x/crypto/scrypt。外部のセッションライブラリやレートリミットライブラリは導入しません。

前提:
- 作業ディレクトリは`/Users/yujiokamoto/devs/golang/feedflow-go-htmx`です。
- Phase1のinternal/domainとinternal/portが存在します。domain.UserとUser.IsRegistered、port.Repository、port.Clock、port.IDGenを使います。
- Phase2のinternal/storeがport.Repositoryを実装済みである必要はありません。本フェーズはport.Repositoryのインターフェースにのみ依存し、テストではフェイクリポジトリを注入します。
- internal/obsのCloseAndLogを使います。先行フェーズのPhase2(03-store)で確定済みのため、本フェーズでは再定義しません。シグネチャはCloseAndLog(logger *slog.Logger, resource string, closer io.Closer)です。
- `go version`が1.25系以降であることを確認してから始めます。

---

## Task 1: obsクローズヘルパの確認

Files:
- 変更なし

internal/obsのCloseAndLogは先行フェーズのPhase2(03-store)で確定済みです。本フェーズはobs.goを再定義せず、確定済みのシグネチャをそのまま使います。レスポンスボディやファイルのクローズはエラーを握り潰さず、このヘルパで記録します。確定済みのシグネチャは次のとおりです。

```go
// CloseAndLog io.Closerをクローズし、エラーがあればloggerに記録します。
func CloseAndLog(logger *slog.Logger, resource string, closer io.Closer)
```

引数の順序はloggerとresourceとcloserで、全フェーズで一致します。本フェーズのauthパッケージはこのヘルパに直接は依存しませんが、テストのHTTPレスポンス処理などで必要になったときはこのシグネチャで呼び出します。

- [ ] Step 1: obsパッケージが確定済みであることを確認する

Run:
```bash
cd /Users/yujiokamoto/devs/golang/feedflow-go-htmx && go doc ./internal/obs CloseAndLog
```
Expected: `func CloseAndLog(logger *slog.Logger, resource string, closer io.Closer)`の形でシグネチャが表示されます。表示されない場合は先行フェーズのPhase2(03-store)が未完了のため、そちらを先に完了させてから本フェーズへ戻ります。本フェーズではobs.goを作成も再定義もしません。

---

## Task 2: golang.org/x/crypto の追加

Files:
- Modify: `/Users/yujiokamoto/devs/golang/feedflow-go-htmx/go.mod`
- Modify: `/Users/yujiokamoto/devs/golang/feedflow-go-htmx/go.sum`

設計書のセクション13のとおりパスワードハッシュにgolang.org/x/cryptoのscryptを使います。ここでgo getして依存に追加します。

- [ ] Step 1: golang.org/x/crypto を取得する

Run:
```bash
cd /Users/yujiokamoto/devs/golang/feedflow-go-htmx && go get golang.org/x/crypto/scrypt
```
Expected: `go: added golang.org/x/crypto`の行が表示されます。すでに取得済みなら`go: upgraded`または何も表示されない場合があり、いずれも問題ありません。

- [ ] Step 2: go.mod に require が入ったことを確認する

Run:
```bash
cd /Users/yujiokamoto/devs/golang/feedflow-go-htmx && grep golang.org/x/crypto go.mod
```
Expected: `golang.org/x/crypto v0.??.0`の形で1行表示されます。バージョン番号は取得時点のもので構いません。

- [ ] Step 3: モジュールを整理する

Run:
```bash
cd /Users/yujiokamoto/devs/golang/feedflow-go-htmx && go mod tidy
```
Expected: エラーなく完了します。go.sumにgolang.org/x/cryptoのハッシュ行が追加されます。

- [ ] Step 4: コミットする

```bash
cd /Users/yujiokamoto/devs/golang/feedflow-go-htmx && git add go.mod go.sum && git commit -m "chore: scrypt のため golang.org/x/crypto を追加する"
```

---

## Task 3: scryptパスワードハッシュと検証(TDD)

Files:
- Test: `/Users/yujiokamoto/devs/golang/feedflow-go-htmx/internal/auth/password_test.go`
- Create: `/Users/yujiokamoto/devs/golang/feedflow-go-htmx/internal/auth/password.go`

パスワードをscryptでハッシュ化し、ソルトとパラメータを含む自己記述的な文字列として保存します。検証は定数時間比較で行いタイミング攻撃を避けます。フォーマットは`scrypt$N$r$p$base64Salt$base64Hash`とします。

- [ ] Step 1: 失敗するテストを書く

Create `internal/auth/password_test.go`:
```go
package auth

import (
	"strings"
	"testing"
)

func TestHashPasswordProducesVerifiableHash(t *testing.T) {
	t.Parallel()
	// テストでは軽量パラメータを使い実行時間を抑えます。
	p := Params{N: 1 << 10, R: 8, P: 1, KeyLen: 32, SaltLen: 16}

	hash, err := HashPassword("correct horse battery staple", p)
	if err != nil {
		t.Fatalf("HashPassword returned error: %v", err)
	}
	if !strings.HasPrefix(hash, "scrypt$") {
		t.Fatalf("hash prefix got %q want scrypt$...", hash)
	}
	if strings.Count(hash, "$") != 5 {
		t.Fatalf("hash segment count got %d want 5 separators", strings.Count(hash, "$"))
	}

	ok, err := VerifyPassword("correct horse battery staple", hash)
	if err != nil {
		t.Fatalf("VerifyPassword returned error: %v", err)
	}
	if !ok {
		t.Fatalf("VerifyPassword got false want true for correct password")
	}
}

func TestVerifyPasswordRejectsWrongPassword(t *testing.T) {
	t.Parallel()
	p := Params{N: 1 << 10, R: 8, P: 1, KeyLen: 32, SaltLen: 16}
	hash, err := HashPassword("right-password", p)
	if err != nil {
		t.Fatalf("HashPassword returned error: %v", err)
	}

	ok, err := VerifyPassword("wrong-password", hash)
	if err != nil {
		t.Fatalf("VerifyPassword returned error: %v", err)
	}
	if ok {
		t.Fatalf("VerifyPassword got true want false for wrong password")
	}
}

func TestHashPasswordUsesRandomSalt(t *testing.T) {
	t.Parallel()
	p := Params{N: 1 << 10, R: 8, P: 1, KeyLen: 32, SaltLen: 16}
	h1, err := HashPassword("same", p)
	if err != nil {
		t.Fatalf("HashPassword h1 returned error: %v", err)
	}
	h2, err := HashPassword("same", p)
	if err != nil {
		t.Fatalf("HashPassword h2 returned error: %v", err)
	}
	if h1 == h2 {
		t.Fatalf("同一パスワードで同一ハッシュになりました。ソルトが乱数になっていません")
	}
}

func TestVerifyPasswordRejectsMalformedHash(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		hash string
	}{
		{name: "空文字列", hash: ""},
		{name: "区切り不足", hash: "scrypt$16384$8$1$abc"},
		{name: "別アルゴリズム", hash: "bcrypt$16384$8$1$c2FsdA==$aGFzaA=="},
		{name: "Nが数値でない", hash: "scrypt$xx$8$1$c2FsdA==$aGFzaA=="},
		{name: "saltがbase64でない", hash: "scrypt$16384$8$1$!notbase64!$aGFzaA=="},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			ok, err := VerifyPassword("whatever", tt.hash)
			if err == nil {
				t.Fatalf("VerifyPassword error got nil want non-nil for malformed hash")
			}
			if ok {
				t.Fatalf("VerifyPassword got true want false for malformed hash")
			}
		})
	}
}

func TestDefaultParamsAreStrong(t *testing.T) {
	t.Parallel()
	p := DefaultParams()
	if p.N < 1<<15 {
		t.Fatalf("DefaultParams N got %d want >= 32768", p.N)
	}
	if p.KeyLen < 32 {
		t.Fatalf("DefaultParams KeyLen got %d want >= 32", p.KeyLen)
	}
	if p.SaltLen < 16 {
		t.Fatalf("DefaultParams SaltLen got %d want >= 16", p.SaltLen)
	}
}
```

- [ ] Step 2: テストが失敗することを確認する

Run:
```bash
cd /Users/yujiokamoto/devs/golang/feedflow-go-htmx && go test ./internal/auth/ -run 'TestHashPassword|TestVerifyPassword|TestDefaultParams' -v
```
Expected: コンパイルエラーで失敗します。`undefined: Params`や`undefined: HashPassword`と表示されます。

- [ ] Step 3: 最小実装を書く

Create `internal/auth/password.go`:
```go
// Package auth feedflowのアプリ層の認証とセキュリティを提供します。
// scryptによるパスワードハッシュと検証、メモリ保持のCookieセッション、CSRFトークン、
// 簡易トークンバケットのレートリミット、初回セットアップの可否判定、セキュリティヘッダ付与を担います。
// 設計書のセクション9に対応します。
package auth

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"fmt"
	"strconv"
	"strings"

	"golang.org/x/crypto/scrypt"
)

// Params scryptのコストパラメータと鍵長とソルト長を保持します。
type Params struct {
	N       int // CPUとメモリのコスト。2のべき乗であることが必要です
	R       int // ブロックサイズ係数
	P       int // 並列度
	KeyLen  int // 派生鍵の長さ(バイト)
	SaltLen int // ソルトの長さ(バイト)
}

// DefaultParams 本番運用に適した既定のscryptパラメータを返します。
// Nは32768でOWASPの推奨水準に沿った強度です。
func DefaultParams() Params {
	return Params{N: 1 << 15, R: 8, P: 1, KeyLen: 32, SaltLen: 16}
}

// hashScheme パスワードハッシュ文字列の先頭に置く識別子です。
const hashScheme = "scrypt"

// HashPassword 平文パスワードをscryptでハッシュ化し、
// schemeとNとrとpとソルトと派生鍵を含む自己記述的な文字列を返します。
// 形式はscrypt$N$r$p$base64Salt$base64Hashです。
func HashPassword(password string, p Params) (string, error) {
	salt := make([]byte, p.SaltLen)
	if _, err := rand.Read(salt); err != nil {
		return "", fmt.Errorf("failed to read salt: %w", err)
	}
	dk, err := scrypt.Key([]byte(password), salt, p.N, p.R, p.P, p.KeyLen)
	if err != nil {
		return "", fmt.Errorf("failed to derive scrypt key: %w", err)
	}
	enc := base64.RawStdEncoding
	return fmt.Sprintf("%s$%d$%d$%d$%s$%s",
		hashScheme, p.N, p.R, p.P, enc.EncodeToString(salt), enc.EncodeToString(dk)), nil
}

// VerifyPassword 平文パスワードが保存済みハッシュ文字列に一致するかを定数時間比較で判定します。
// ハッシュ文字列の形式が不正なときはエラーを返します。
func VerifyPassword(password, encoded string) (bool, error) {
	parts := strings.Split(encoded, "$")
	if len(parts) != 6 {
		return false, fmt.Errorf("invalid hash format: expected 6 fields, got %d", len(parts))
	}
	if parts[0] != hashScheme {
		return false, fmt.Errorf("unsupported hash scheme: %q", parts[0])
	}
	n, err := strconv.Atoi(parts[1])
	if err != nil {
		return false, fmt.Errorf("invalid N field: %w", err)
	}
	r, err := strconv.Atoi(parts[2])
	if err != nil {
		return false, fmt.Errorf("invalid r field: %w", err)
	}
	pp, err := strconv.Atoi(parts[3])
	if err != nil {
		return false, fmt.Errorf("invalid p field: %w", err)
	}
	enc := base64.RawStdEncoding
	salt, err := enc.DecodeString(parts[4])
	if err != nil {
		return false, fmt.Errorf("invalid salt encoding: %w", err)
	}
	want, err := enc.DecodeString(parts[5])
	if err != nil {
		return false, fmt.Errorf("invalid hash encoding: %w", err)
	}
	got, err := scrypt.Key([]byte(password), salt, n, r, pp, len(want))
	if err != nil {
		return false, fmt.Errorf("failed to derive scrypt key: %w", err)
	}
	return subtle.ConstantTimeCompare(got, want) == 1, nil
}
```

- [ ] Step 4: テストが通ることを確認する

Run:
```bash
cd /Users/yujiokamoto/devs/golang/feedflow-go-htmx && go test ./internal/auth/ -run 'TestHashPassword|TestVerifyPassword|TestDefaultParams' -v
```
Expected: 全サブテストがPASSします。

- [ ] Step 5: gofmtを適用してコミットする

```bash
cd /Users/yujiokamoto/devs/golang/feedflow-go-htmx && gofmt -w internal/auth/password.go internal/auth/password_test.go && git add internal/auth/password.go internal/auth/password_test.go && git commit -m "feat: scrypt によるパスワードハッシュと検証を追加する"
```

---

## Task 4: メモリ保持のCookieセッション(TDD)

Files:
- Test: `/Users/yujiokamoto/devs/golang/feedflow-go-htmx/internal/auth/session_test.go`
- Create: `/Users/yujiokamoto/devs/golang/feedflow-go-htmx/internal/auth/session.go`

セッションIDはサーバ側のメモリに保持し、Cookieには推測困難なIDのみを置きます。Cookie属性はHttpOnlyとSameSite=Laxを必須とし、Secureは設定で切り替えます。プロセス再起動でセッションは失効します。時刻はport.Clockから取得して期限切れ判定をテスト可能にします。

- [ ] Step 1: 失敗するテストを書く

Create `internal/auth/session_test.go`:
```go
package auth

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

type fakeClock struct{ now time.Time }

func (c *fakeClock) Now() time.Time { return c.now }

func newTestSessions(clk *fakeClock, secure bool) *SessionStore {
	return NewSessionStore(SessionConfig{
		Clock:      clk,
		TTL:        2 * time.Hour,
		CookieName: "feedflow_session",
		Secure:     secure,
	})
}

func TestSessionIssueAndValidate(t *testing.T) {
	t.Parallel()
	clk := &fakeClock{now: time.Date(2026, 5, 29, 10, 0, 0, 0, time.UTC)}
	store := newTestSessions(clk, true)

	rec := httptest.NewRecorder()
	if err := store.Issue(rec, "owner"); err != nil {
		t.Fatalf("Issue returned error: %v", err)
	}

	cookies := rec.Result().Cookies()
	if len(cookies) != 1 {
		t.Fatalf("cookie count got %d want 1", len(cookies))
	}
	c := cookies[0]
	if !c.HttpOnly {
		t.Fatalf("cookie HttpOnly got false want true")
	}
	if !c.Secure {
		t.Fatalf("cookie Secure got false want true")
	}
	if c.SameSite != http.SameSiteLaxMode {
		t.Fatalf("cookie SameSite got %v want Lax", c.SameSite)
	}
	if c.Path != "/" {
		t.Fatalf("cookie Path got %q want /", c.Path)
	}

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.AddCookie(c)
	username, ok := store.Validate(req)
	if !ok {
		t.Fatalf("Validate got ok=false want true for freshly issued session")
	}
	if username != "owner" {
		t.Fatalf("Validate username got %q want owner", username)
	}
}

func TestSessionValidateRejectsExpired(t *testing.T) {
	t.Parallel()
	clk := &fakeClock{now: time.Date(2026, 5, 29, 10, 0, 0, 0, time.UTC)}
	store := newTestSessions(clk, true)

	rec := httptest.NewRecorder()
	if err := store.Issue(rec, "owner"); err != nil {
		t.Fatalf("Issue returned error: %v", err)
	}
	c := rec.Result().Cookies()[0]

	// TTL を超えて時刻を進めます。
	clk.now = clk.now.Add(3 * time.Hour)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.AddCookie(c)
	if _, ok := store.Validate(req); ok {
		t.Fatalf("Validate got ok=true want false for expired session")
	}
}

func TestSessionValidateRejectsUnknownCookie(t *testing.T) {
	t.Parallel()
	clk := &fakeClock{now: time.Date(2026, 5, 29, 10, 0, 0, 0, time.UTC)}
	store := newTestSessions(clk, true)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.AddCookie(&http.Cookie{Name: "feedflow_session", Value: "nonexistent"})
	if _, ok := store.Validate(req); ok {
		t.Fatalf("Validate got ok=true want false for unknown session id")
	}
}

func TestSessionDestroyRemovesSession(t *testing.T) {
	t.Parallel()
	clk := &fakeClock{now: time.Date(2026, 5, 29, 10, 0, 0, 0, time.UTC)}
	store := newTestSessions(clk, true)

	rec := httptest.NewRecorder()
	if err := store.Issue(rec, "owner"); err != nil {
		t.Fatalf("Issue returned error: %v", err)
	}
	c := rec.Result().Cookies()[0]

	delRec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/logout", nil)
	req.AddCookie(c)
	store.Destroy(delRec, req)

	// 破棄用CookieはMaxAgeが負で即時失効を指示します。
	out := delRec.Result().Cookies()
	if len(out) != 1 {
		t.Fatalf("destroy cookie count got %d want 1", len(out))
	}
	if out[0].MaxAge >= 0 {
		t.Fatalf("destroy cookie MaxAge got %d want negative", out[0].MaxAge)
	}

	check := httptest.NewRequest(http.MethodGet, "/", nil)
	check.AddCookie(c)
	if _, ok := store.Validate(check); ok {
		t.Fatalf("Validate got ok=true want false after Destroy")
	}
}

func TestSessionSecureFalseOmitsSecureAttribute(t *testing.T) {
	t.Parallel()
	clk := &fakeClock{now: time.Date(2026, 5, 29, 10, 0, 0, 0, time.UTC)}
	store := newTestSessions(clk, false)

	rec := httptest.NewRecorder()
	if err := store.Issue(rec, "owner"); err != nil {
		t.Fatalf("Issue returned error: %v", err)
	}
	if rec.Result().Cookies()[0].Secure {
		t.Fatalf("cookie Secure got true want false when Secure config is false")
	}
}
```

- [ ] Step 2: テストが失敗することを確認する

Run:
```bash
cd /Users/yujiokamoto/devs/golang/feedflow-go-htmx && go test ./internal/auth/ -run TestSession -v
```
Expected: コンパイルエラーで失敗します。`undefined: SessionStore`や`undefined: NewSessionStore`と表示されます。

- [ ] Step 3: 最小実装を書く

Create `internal/auth/session.go`:
```go
package auth

import (
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/okamyuji/feedflow-go-htmx/internal/port"
)

// sessionIDBytes セッションIDの乱数バイト数です。192ビットで推測困難にします。
const sessionIDBytes = 24

// session サーバ側に保持する1件のセッション状態です。
type session struct {
	username  string
	expiresAt time.Time
}

// SessionConfig セッションストアの設定です。
type SessionConfig struct {
	Clock      port.Clock    // 期限判定に使う時刻源です
	TTL        time.Duration // セッションの有効期間です
	CookieName string        // セッションCookieの名前です
	Secure     bool          // CookieにSecure属性を付けるかどうかです
}

// SessionStore メモリ保持のCookieセッションを管理します。
// セッションIDをキーにサーバ側状態を保持し、CookieにはIDのみを置きます。
// プロセス再起動で全セッションは失効します。設計書のセクション9.3に対応します。
type SessionStore struct {
	mu       sync.Mutex
	sessions map[string]session
	clock    port.Clock
	ttl      time.Duration
	name     string
	secure   bool
}

// NewSessionStore 設定からセッションストアを生成します。
func NewSessionStore(cfg SessionConfig) *SessionStore {
	return &SessionStore{
		sessions: make(map[string]session),
		clock:    cfg.Clock,
		ttl:      cfg.TTL,
		name:     cfg.CookieName,
		secure:   cfg.Secure,
	}
}

// newSessionID 推測困難なセッションIDをURLセーフなbase64で生成します。
func newSessionID() (string, error) {
	b := make([]byte, sessionIDBytes)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("failed to read session id: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

// Issue 新しいセッションを発行し、Cookieをレスポンスに付与します。
func (s *SessionStore) Issue(w http.ResponseWriter, username string) error {
	id, err := newSessionID()
	if err != nil {
		return fmt.Errorf("failed to issue session: %w", err)
	}
	now := s.clock.Now()
	s.mu.Lock()
	s.sessions[id] = session{username: username, expiresAt: now.Add(s.ttl)}
	s.mu.Unlock()

	http.SetCookie(w, &http.Cookie{
		Name:     s.name,
		Value:    id,
		Path:     "/",
		HttpOnly: true,
		Secure:   s.secure,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   int(s.ttl.Seconds()),
	})
	return nil
}

// Validate リクエストのCookieから有効なセッションを引き、ユーザー名を返します。
// セッションが無いか期限切れのときはokをfalseにします。期限切れは破棄します。
func (s *SessionStore) Validate(r *http.Request) (string, bool) {
	c, err := r.Cookie(s.name)
	if err != nil {
		return "", false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	sess, ok := s.sessions[c.Value]
	if !ok {
		return "", false
	}
	if !s.clock.Now().Before(sess.expiresAt) {
		delete(s.sessions, c.Value)
		return "", false
	}
	return sess.username, true
}

// Destroy リクエストのセッションをサーバ側から削除し、破棄用Cookieを付与します。
func (s *SessionStore) Destroy(w http.ResponseWriter, r *http.Request) {
	if c, err := r.Cookie(s.name); err == nil {
		s.mu.Lock()
		delete(s.sessions, c.Value)
		s.mu.Unlock()
	}
	http.SetCookie(w, &http.Cookie{
		Name:     s.name,
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		Secure:   s.secure,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   -1,
	})
}
```

- [ ] Step 4: テストが通ることを確認する

Run:
```bash
cd /Users/yujiokamoto/devs/golang/feedflow-go-htmx && go test ./internal/auth/ -run TestSession -v
```
Expected: 全サブテストがPASSします。

- [ ] Step 5: gofmtを適用してコミットする

```bash
cd /Users/yujiokamoto/devs/golang/feedflow-go-htmx && gofmt -w internal/auth/session.go internal/auth/session_test.go && git add internal/auth/session.go internal/auth/session_test.go && git commit -m "feat: メモリ保持の Cookie セッションを追加する"
```

---

## Task 5: CSRFトークン(TDD)

Files:
- Test: `/Users/yujiokamoto/devs/golang/feedflow-go-htmx/internal/auth/csrf_test.go`
- Create: `/Users/yujiokamoto/devs/golang/feedflow-go-htmx/internal/auth/csrf.go`

状態変更を伴うPOSTにCSRFトークンを要求します。トークンはセッションごとにサーバ側で保持し、フォームのhiddenフィールドかX-CSRF-Tokenヘッダで送られた値を定数時間比較で検証します。GETやHEADなど安全なメソッドは検証をスキップします。

- [ ] Step 1: 失敗するテストを書く

Create `internal/auth/csrf_test.go`:
```go
package auth

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

func TestCSRFIssueProducesToken(t *testing.T) {
	t.Parallel()
	store := NewCSRFStore()
	token, err := store.Issue("sess-1")
	if err != nil {
		t.Fatalf("Issue returned error: %v", err)
	}
	if token == "" {
		t.Fatalf("Issue token got empty want non-empty")
	}
}

func TestCSRFVerifyAcceptsHeaderToken(t *testing.T) {
	t.Parallel()
	store := NewCSRFStore()
	token, err := store.Issue("sess-1")
	if err != nil {
		t.Fatalf("Issue returned error: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/feeds", nil)
	req.Header.Set("X-CSRF-Token", token)
	if !store.Verify("sess-1", req) {
		t.Fatalf("Verify got false want true for valid header token")
	}
}

func TestCSRFVerifyAcceptsFormToken(t *testing.T) {
	t.Parallel()
	store := NewCSRFStore()
	token, err := store.Issue("sess-1")
	if err != nil {
		t.Fatalf("Issue returned error: %v", err)
	}

	form := url.Values{}
	form.Set(CSRFFieldName, token)
	req := httptest.NewRequest(http.MethodPost, "/feeds", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	if !store.Verify("sess-1", req) {
		t.Fatalf("Verify got false want true for valid form token")
	}
}

func TestCSRFVerifyRejectsWrongToken(t *testing.T) {
	t.Parallel()
	store := NewCSRFStore()
	if _, err := store.Issue("sess-1"); err != nil {
		t.Fatalf("Issue returned error: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/feeds", nil)
	req.Header.Set("X-CSRF-Token", "forged-token")
	if store.Verify("sess-1", req) {
		t.Fatalf("Verify got true want false for forged token")
	}
}

func TestCSRFVerifyRejectsUnknownSession(t *testing.T) {
	t.Parallel()
	store := NewCSRFStore()
	req := httptest.NewRequest(http.MethodPost, "/feeds", nil)
	req.Header.Set("X-CSRF-Token", "anything")
	if store.Verify("no-such-session", req) {
		t.Fatalf("Verify got true want false for unknown session")
	}
}

func TestCSRFDiscardRemovesToken(t *testing.T) {
	t.Parallel()
	store := NewCSRFStore()
	token, err := store.Issue("sess-1")
	if err != nil {
		t.Fatalf("Issue returned error: %v", err)
	}
	store.Discard("sess-1")

	req := httptest.NewRequest(http.MethodPost, "/feeds", nil)
	req.Header.Set("X-CSRF-Token", token)
	if store.Verify("sess-1", req) {
		t.Fatalf("Verify got true want false after Discard")
	}
}
```

- [ ] Step 2: テストが失敗することを確認する

Run:
```bash
cd /Users/yujiokamoto/devs/golang/feedflow-go-htmx && go test ./internal/auth/ -run TestCSRF -v
```
Expected: コンパイルエラーで失敗します。`undefined: NewCSRFStore`や`undefined: CSRFFieldName`と表示されます。

- [ ] Step 3: 最小実装を書く

Create `internal/auth/csrf.go`:
```go
package auth

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"fmt"
	"net/http"
	"sync"
)

// CSRFFieldName CSRFトークンを載せるフォームのhiddenフィールド名です。
const CSRFFieldName = "csrf_token"

// CSRFHeaderName CSRFトークンを載せるリクエストヘッダ名です。HTMXからはこのヘッダで送ります。
const CSRFHeaderName = "X-CSRF-Token"

// csrfTokenBytes CSRFトークンの乱数バイト数です。
const csrfTokenBytes = 32

// CSRFStore セッションIDごとにCSRFトークンを保持し検証します。
// トークンはサーバ側に保持し、リクエストの値と定数時間比較します。設計書のセクション9.1に対応します。
type CSRFStore struct {
	mu     sync.Mutex
	tokens map[string]string
}

// NewCSRFStore 空のCSRFストアを生成します。
func NewCSRFStore() *CSRFStore {
	return &CSRFStore{tokens: make(map[string]string)}
}

// Issue セッションIDに紐づくCSRFトークンを発行して保持し、その値を返します。
// 既にトークンがあれば同じ値を返し、テンプレート埋め込みとの整合を保ちます。
func (c *CSRFStore) Issue(sessionID string) (string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if t, ok := c.tokens[sessionID]; ok {
		return t, nil
	}
	b := make([]byte, csrfTokenBytes)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("failed to read csrf token: %w", err)
	}
	token := base64.RawURLEncoding.EncodeToString(b)
	c.tokens[sessionID] = token
	return token, nil
}

// Token セッションIDに紐づく現在のトークンを返します。未発行のときは空文字列とfalseを返します。
func (c *CSRFStore) Token(sessionID string) (string, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	t, ok := c.tokens[sessionID]
	return t, ok
}

// Verify リクエストのCSRFトークンがセッションの保持値に一致するかを定数時間比較で判定します。
// トークンはヘッダを優先し、無ければフォーム値を見ます。
func (c *CSRFStore) Verify(sessionID string, r *http.Request) bool {
	c.mu.Lock()
	want, ok := c.tokens[sessionID]
	c.mu.Unlock()
	if !ok {
		return false
	}
	got := r.Header.Get(CSRFHeaderName)
	if got == "" {
		got = r.PostFormValue(CSRFFieldName)
	}
	if got == "" {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(got), []byte(want)) == 1
}

// Discard セッションIDに紐づくCSRFトークンを破棄します。ログアウト時に呼びます。
func (c *CSRFStore) Discard(sessionID string) {
	c.mu.Lock()
	delete(c.tokens, sessionID)
	c.mu.Unlock()
}
```

- [ ] Step 4: テストが通ることを確認する

Run:
```bash
cd /Users/yujiokamoto/devs/golang/feedflow-go-htmx && go test ./internal/auth/ -run TestCSRF -v
```
Expected: 全サブテストがPASSします。

- [ ] Step 5: gofmtを適用してコミットする

```bash
cd /Users/yujiokamoto/devs/golang/feedflow-go-htmx && gofmt -w internal/auth/csrf.go internal/auth/csrf_test.go && git add internal/auth/csrf.go internal/auth/csrf_test.go && git commit -m "feat: セッション単位の CSRF トークンを追加する"
```

---

## Task 6: 簡易トークンバケットのレートリミット(TDD)

Files:
- Test: `/Users/yujiokamoto/devs/golang/feedflow-go-htmx/internal/auth/ratelimit_test.go`
- Create: `/Users/yujiokamoto/devs/golang/feedflow-go-htmx/internal/auth/ratelimit.go`

ログイン試行のレートリミットを標準ライブラリだけで実装します。キー(クライアントIP)ごとにトークンバケットを持ち、port.Clockの時刻に基づいて経過時間ぶんのトークンを補充します。外部依存は増やしません。

- [ ] Step 1: 失敗するテストを書く

Create `internal/auth/ratelimit_test.go`:
```go
package auth

import (
	"testing"
	"time"
)

func TestRateLimiterAllowsUpToBurst(t *testing.T) {
	t.Parallel()
	clk := &fakeClock{now: time.Date(2026, 5, 29, 0, 0, 0, 0, time.UTC)}
	// 毎分5回まで、バースト5。
	rl := NewRateLimiter(RateLimitConfig{
		Clock:       clk,
		Burst:       5,
		RefillEvery: time.Minute / 5,
	})

	for i := 0; i < 5; i++ {
		if !rl.Allow("1.2.3.4") {
			t.Fatalf("Allow attempt %d got false want true within burst", i+1)
		}
	}
	if rl.Allow("1.2.3.4") {
		t.Fatalf("Allow attempt 6 got true want false beyond burst")
	}
}

func TestRateLimiterRefillsOverTime(t *testing.T) {
	t.Parallel()
	clk := &fakeClock{now: time.Date(2026, 5, 29, 0, 0, 0, 0, time.UTC)}
	rl := NewRateLimiter(RateLimitConfig{
		Clock:       clk,
		Burst:       2,
		RefillEvery: 10 * time.Second,
	})

	if !rl.Allow("ip") || !rl.Allow("ip") {
		t.Fatalf("初期バースト 2 回が許可されませんでした")
	}
	if rl.Allow("ip") {
		t.Fatalf("バースト超過が許可されました")
	}

	// 10秒経過で1トークン補充されます。
	clk.now = clk.now.Add(10 * time.Second)
	if !rl.Allow("ip") {
		t.Fatalf("補充後の 1 回が許可されませんでした")
	}
	if rl.Allow("ip") {
		t.Fatalf("補充は 1 トークンのはずが追加で許可されました")
	}
}

func TestRateLimiterIsolatesKeys(t *testing.T) {
	t.Parallel()
	clk := &fakeClock{now: time.Date(2026, 5, 29, 0, 0, 0, 0, time.UTC)}
	rl := NewRateLimiter(RateLimitConfig{
		Clock:       clk,
		Burst:       1,
		RefillEvery: time.Minute,
	})

	if !rl.Allow("a") {
		t.Fatalf("key a の初回が拒否されました")
	}
	if rl.Allow("a") {
		t.Fatalf("key a の 2 回目が許可されました")
	}
	if !rl.Allow("b") {
		t.Fatalf("別 key b の初回が拒否されました。キーが分離されていません")
	}
}

func TestRateLimiterCapsAtBurst(t *testing.T) {
	t.Parallel()
	clk := &fakeClock{now: time.Date(2026, 5, 29, 0, 0, 0, 0, time.UTC)}
	rl := NewRateLimiter(RateLimitConfig{
		Clock:       clk,
		Burst:       3,
		RefillEvery: time.Second,
	})

	// 1回だけ消費し、長時間放置してもバースト上限を超えて貯まらないことを確認します。
	if !rl.Allow("ip") {
		t.Fatalf("初回が拒否されました")
	}
	clk.now = clk.now.Add(time.Hour)
	allowed := 0
	for i := 0; i < 10; i++ {
		if rl.Allow("ip") {
			allowed++
		}
	}
	if allowed != 3 {
		t.Fatalf("補充上限後に許可された回数 got %d want 3", allowed)
	}
}
```

- [ ] Step 2: テストが失敗することを確認する

Run:
```bash
cd /Users/yujiokamoto/devs/golang/feedflow-go-htmx && go test ./internal/auth/ -run TestRateLimiter -v
```
Expected: コンパイルエラーで失敗します。`undefined: NewRateLimiter`や`undefined: RateLimitConfig`と表示されます。

- [ ] Step 3: 最小実装を書く

Create `internal/auth/ratelimit.go`:
```go
package auth

import (
	"sync"
	"time"

	"github.com/okamyuji/feedflow-go-htmx/internal/port"
)

// bucket 1キーぶんのトークンバケットの状態です。
type bucket struct {
	tokens     float64
	lastRefill time.Time
}

// RateLimitConfig レートリミッタの設定です。
type RateLimitConfig struct {
	Clock       port.Clock    // トークン補充の基準になる時刻源です
	Burst       int           // 同時に許す最大トークン数です
	RefillEvery time.Duration // トークンを1個補充する間隔です
}

// RateLimiter キーごとの簡易トークンバケットによるレートリミッタです。
// 標準ライブラリだけで実装し外部依存を増やしません。ログイン試行の抑制に使います。設計書のセクション9.1に対応します。
type RateLimiter struct {
	mu          sync.Mutex
	buckets     map[string]*bucket
	clock       port.Clock
	burst       float64
	refillEvery time.Duration
}

// NewRateLimiter 設定からレートリミッタを生成します。
func NewRateLimiter(cfg RateLimitConfig) *RateLimiter {
	return &RateLimiter{
		buckets:     make(map[string]*bucket),
		clock:       cfg.Clock,
		burst:       float64(cfg.Burst),
		refillEvery: cfg.RefillEvery,
	}
}

// Allow キーに対する1回の試行を許可するかどうかを返します。
// 経過時間に応じてトークンを補充し、1個以上あれば1個消費してtrueを返します。
func (rl *RateLimiter) Allow(key string) bool {
	now := rl.clock.Now()
	rl.mu.Lock()
	defer rl.mu.Unlock()

	b, ok := rl.buckets[key]
	if !ok {
		b = &bucket{tokens: rl.burst, lastRefill: now}
		rl.buckets[key] = b
	}

	if rl.refillEvery > 0 {
		elapsed := now.Sub(b.lastRefill)
		refill := float64(elapsed) / float64(rl.refillEvery)
		if refill > 0 {
			b.tokens += refill
			if b.tokens > rl.burst {
				b.tokens = rl.burst
			}
			b.lastRefill = now
		}
	}

	if b.tokens >= 1 {
		b.tokens--
		return true
	}
	return false
}
```

- [ ] Step 4: テストが通ることを確認する

Run:
```bash
cd /Users/yujiokamoto/devs/golang/feedflow-go-htmx && go test ./internal/auth/ -run TestRateLimiter -v
```
Expected: 全サブテストがPASSします。

- [ ] Step 5: gofmtを適用してコミットする

```bash
cd /Users/yujiokamoto/devs/golang/feedflow-go-htmx && gofmt -w internal/auth/ratelimit.go internal/auth/ratelimit_test.go && git add internal/auth/ratelimit.go internal/auth/ratelimit_test.go && git commit -m "feat: 標準ライブラリの簡易トークンバケットレートリミットを追加する"
```

---

## Task 7: 初回セットアップの可否判定とManager(TDD)

Files:
- Test: `/Users/yujiokamoto/devs/golang/feedflow-go-htmx/internal/auth/manager_test.go`
- Create: `/Users/yujiokamoto/devs/golang/feedflow-go-htmx/internal/auth/manager.go`

ManagerはパスワードハッシュとリポジトリのUser読み書きをまとめます。初回セットアップはuser.json未登録(domain.User.IsRegisteredがfalse)のときだけ可能です。登録済みでのSetupは拒否します。Authenticateはユーザー名一致とパスワード検証を行います。port.Repositoryへはインターフェース経由で依存しコンストラクタ注入します。

- [ ] Step 1: 失敗するテストを書く

Create `internal/auth/manager_test.go`:
```go
package auth

import (
	"errors"
	"testing"

	"github.com/okamyuji/feedflow-go-htmx/internal/domain"
)

// fakeUserRepo Managerのテストに必要なUserの読み書きだけを実装するフェイクです。
type fakeUserRepo struct {
	user    domain.User
	saveErr error
	loadErr error
}

func (r *fakeUserRepo) User() (domain.User, error) {
	if r.loadErr != nil {
		return domain.User{}, r.loadErr
	}
	return r.user, nil
}

func (r *fakeUserRepo) SaveUser(u domain.User) error {
	if r.saveErr != nil {
		return r.saveErr
	}
	r.user = u
	return nil
}

// testParams テスト用の軽量scryptパラメータです。
func testParams() Params {
	return Params{N: 1 << 10, R: 8, P: 1, KeyLen: 32, SaltLen: 16}
}

func TestNeedsSetupWhenUserUnregistered(t *testing.T) {
	t.Parallel()
	repo := &fakeUserRepo{user: domain.User{}}
	m := NewManager(repo, testParams())

	needs, err := m.NeedsSetup()
	if err != nil {
		t.Fatalf("NeedsSetup returned error: %v", err)
	}
	if !needs {
		t.Fatalf("NeedsSetup got false want true for unregistered user")
	}
}

func TestNeedsSetupWhenUserRegistered(t *testing.T) {
	t.Parallel()
	repo := &fakeUserRepo{user: domain.User{Username: "owner", PasswordHash: "scrypt$1024$8$1$c2FsdA$aGFzaA"}}
	m := NewManager(repo, testParams())

	needs, err := m.NeedsSetup()
	if err != nil {
		t.Fatalf("NeedsSetup returned error: %v", err)
	}
	if needs {
		t.Fatalf("NeedsSetup got true want false for registered user")
	}
}

func TestSetupRegistersOwnerWhenUnregistered(t *testing.T) {
	t.Parallel()
	repo := &fakeUserRepo{user: domain.User{}}
	m := NewManager(repo, testParams())

	if err := m.Setup("owner", "s3cr3t-passw0rd"); err != nil {
		t.Fatalf("Setup returned error: %v", err)
	}
	if repo.user.Username != "owner" {
		t.Fatalf("saved username got %q want owner", repo.user.Username)
	}
	if repo.user.PasswordHash == "" {
		t.Fatalf("saved password hash is empty")
	}
	ok, err := VerifyPassword("s3cr3t-passw0rd", repo.user.PasswordHash)
	if err != nil {
		t.Fatalf("VerifyPassword returned error: %v", err)
	}
	if !ok {
		t.Fatalf("保存したハッシュが元パスワードを検証できません")
	}
}

func TestSetupRejectedWhenAlreadyRegistered(t *testing.T) {
	t.Parallel()
	repo := &fakeUserRepo{user: domain.User{Username: "owner", PasswordHash: "scrypt$1024$8$1$c2FsdA$aGFzaA"}}
	m := NewManager(repo, testParams())

	err := m.Setup("intruder", "another-pass")
	if !errors.Is(err, ErrAlreadyRegistered) {
		t.Fatalf("Setup error got %v want ErrAlreadyRegistered", err)
	}
}

func TestSetupRejectsEmptyCredentials(t *testing.T) {
	t.Parallel()
	repo := &fakeUserRepo{user: domain.User{}}
	m := NewManager(repo, testParams())

	if err := m.Setup("", "password"); !errors.Is(err, ErrInvalidCredentials) {
		t.Fatalf("Setup empty username error got %v want ErrInvalidCredentials", err)
	}
	if err := m.Setup("owner", ""); !errors.Is(err, ErrInvalidCredentials) {
		t.Fatalf("Setup empty password error got %v want ErrInvalidCredentials", err)
	}
}

func TestAuthenticateSucceedsForCorrectCredentials(t *testing.T) {
	t.Parallel()
	repo := &fakeUserRepo{user: domain.User{}}
	m := NewManager(repo, testParams())
	if err := m.Setup("owner", "right-pass"); err != nil {
		t.Fatalf("Setup returned error: %v", err)
	}

	ok, err := m.Authenticate("owner", "right-pass")
	if err != nil {
		t.Fatalf("Authenticate returned error: %v", err)
	}
	if !ok {
		t.Fatalf("Authenticate got false want true for correct credentials")
	}
}

func TestAuthenticateFailsForWrongPassword(t *testing.T) {
	t.Parallel()
	repo := &fakeUserRepo{user: domain.User{}}
	m := NewManager(repo, testParams())
	if err := m.Setup("owner", "right-pass"); err != nil {
		t.Fatalf("Setup returned error: %v", err)
	}

	ok, err := m.Authenticate("owner", "wrong-pass")
	if err != nil {
		t.Fatalf("Authenticate returned error: %v", err)
	}
	if ok {
		t.Fatalf("Authenticate got true want false for wrong password")
	}
}

func TestAuthenticateFailsForWrongUsername(t *testing.T) {
	t.Parallel()
	repo := &fakeUserRepo{user: domain.User{}}
	m := NewManager(repo, testParams())
	if err := m.Setup("owner", "right-pass"); err != nil {
		t.Fatalf("Setup returned error: %v", err)
	}

	ok, err := m.Authenticate("someone-else", "right-pass")
	if err != nil {
		t.Fatalf("Authenticate returned error: %v", err)
	}
	if ok {
		t.Fatalf("Authenticate got true want false for wrong username")
	}
}

func TestAuthenticateFailsWhenUnregistered(t *testing.T) {
	t.Parallel()
	repo := &fakeUserRepo{user: domain.User{}}
	m := NewManager(repo, testParams())

	ok, err := m.Authenticate("owner", "any")
	if err != nil {
		t.Fatalf("Authenticate returned error: %v", err)
	}
	if ok {
		t.Fatalf("Authenticate got true want false when no user is registered")
	}
}
```

- [ ] Step 2: テストが失敗することを確認する

Run:
```bash
cd /Users/yujiokamoto/devs/golang/feedflow-go-htmx && go test ./internal/auth/ -run 'TestNeedsSetup|TestSetup|TestAuthenticate' -v
```
Expected: コンパイルエラーで失敗します。`undefined: NewManager`や`undefined: ErrAlreadyRegistered`と表示されます。

- [ ] Step 3: 最小実装を書く

Create `internal/auth/manager.go`:
```go
package auth

import (
	"errors"
	"fmt"

	"github.com/okamyuji/feedflow-go-htmx/internal/domain"
)

// ErrAlreadyRegistered 所有者が登録済みなのに初回セットアップを試みたときに返します。
var ErrAlreadyRegistered = errors.New("owner is already registered")

// ErrInvalidCredentials ユーザー名かパスワードが空のときに返します。
var ErrInvalidCredentials = errors.New("username and password must not be empty")

// userRepository Managerが必要とするUserの読み書きだけを切り出したインターフェースです。
// port.Repositoryはこれを満たします。狭いインターフェースに依存することでテストのフェイクを最小化します。
type userRepository interface {
	User() (domain.User, error)
	SaveUser(user domain.User) error
}

// Manager 所有者の登録と認証を担います。
// パスワードハッシュ生成と検証、リポジトリ経由のUser読み書き、初回セットアップの可否判定をまとめます。
// userRepositoryにインターフェースとして依存しコンストラクタ注入で受け取ります。設計書のセクション9.3に対応します。
type Manager struct {
	repo   userRepository
	params Params
}

// NewManager リポジトリとscryptパラメータからManagerを生成します。
func NewManager(repo userRepository, params Params) *Manager {
	return &Manager{repo: repo, params: params}
}

// NeedsSetup 初回セットアップが必要かどうかを返します。
// user.jsonが未登録(IsRegisteredがfalse)のときだけtrueになります。
func (m *Manager) NeedsSetup() (bool, error) {
	user, err := m.repo.User()
	if err != nil {
		return false, fmt.Errorf("failed to load user: %w", err)
	}
	return !user.IsRegistered(), nil
}

// Setup 初回の所有者を登録します。
// 登録済みのときはErrAlreadyRegisteredを返し、上書きや乗っ取りを防ぎます。
// ユーザー名かパスワードが空のときはErrInvalidCredentialsを返します。
func (m *Manager) Setup(username, password string) error {
	if username == "" || password == "" {
		return ErrInvalidCredentials
	}
	user, err := m.repo.User()
	if err != nil {
		return fmt.Errorf("failed to load user: %w", err)
	}
	if user.IsRegistered() {
		return ErrAlreadyRegistered
	}
	hash, err := HashPassword(password, m.params)
	if err != nil {
		return fmt.Errorf("failed to hash password: %w", err)
	}
	if err := m.repo.SaveUser(domain.User{Username: username, PasswordHash: hash}); err != nil {
		return fmt.Errorf("failed to save user: %w", err)
	}
	return nil
}

// Authenticate ユーザー名とパスワードが登録済みの所有者と一致するかを返します。
// 未登録やユーザー名不一致やパスワード不一致のときはfalseを返します。
func (m *Manager) Authenticate(username, password string) (bool, error) {
	user, err := m.repo.User()
	if err != nil {
		return false, fmt.Errorf("failed to load user: %w", err)
	}
	if !user.IsRegistered() || user.Username != username {
		return false, nil
	}
	ok, err := VerifyPassword(password, user.PasswordHash)
	if err != nil {
		return false, fmt.Errorf("failed to verify password: %w", err)
	}
	return ok, nil
}
```

- [ ] Step 4: テストが通ることを確認する

Run:
```bash
cd /Users/yujiokamoto/devs/golang/feedflow-go-htmx && go test ./internal/auth/ -run 'TestNeedsSetup|TestSetup|TestAuthenticate' -v
```
Expected: 全サブテストがPASSします。

- [ ] Step 5: gofmtを適用してコミットする

```bash
cd /Users/yujiokamoto/devs/golang/feedflow-go-htmx && gofmt -w internal/auth/manager.go internal/auth/manager_test.go && git add internal/auth/manager.go internal/auth/manager_test.go && git commit -m "feat: 認証と初回セットアップ可否判定の Manager を追加する"
```

---

## Task 8: セキュリティヘッダ付与関数(TDD)

Files:
- Test: `/Users/yujiokamoto/devs/golang/feedflow-go-htmx/internal/auth/headers_test.go`
- Create: `/Users/yujiokamoto/devs/golang/feedflow-go-htmx/internal/auth/headers.go`

設計書のセクション9.1のセキュリティヘッダを付与するミドルウェア関数を用意します。ハンドラ(Phase7)はこの関数で全レスポンスにヘッダを付けます。HSTSは公開環境(Secure=true)のときだけ付与します。CSPはHTMXとAlpine.jsを同梱配信する前提でself中心に組みます。

- [ ] Step 1: 失敗するテストを書く

Create `internal/auth/headers_test.go`:
```go
package auth

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func okHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})
}

func TestSecurityHeadersSetsBaselineHeaders(t *testing.T) {
	t.Parallel()
	h := SecurityHeaders(HeaderConfig{EnableHSTS: false})(okHandler())

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	h.ServeHTTP(rec, req)

	res := rec.Result()
	checks := map[string]string{
		"X-Content-Type-Options": "nosniff",
		"X-Frame-Options":        "DENY",
		"Referrer-Policy":        "strict-origin-when-cross-origin",
	}
	for k, want := range checks {
		if got := res.Header.Get(k); got != want {
			t.Fatalf("header %s got %q want %q", k, got, want)
		}
	}
	if res.Header.Get("Permissions-Policy") == "" {
		t.Fatalf("Permissions-Policy is empty")
	}
	if res.Header.Get("Content-Security-Policy") == "" {
		t.Fatalf("Content-Security-Policy is empty")
	}
}

func TestSecurityHeadersOmitsHSTSWhenDisabled(t *testing.T) {
	t.Parallel()
	h := SecurityHeaders(HeaderConfig{EnableHSTS: false})(okHandler())

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	h.ServeHTTP(rec, req)

	if got := rec.Result().Header.Get("Strict-Transport-Security"); got != "" {
		t.Fatalf("HSTS got %q want empty when disabled", got)
	}
}

func TestSecurityHeadersSetsHSTSWhenEnabled(t *testing.T) {
	t.Parallel()
	h := SecurityHeaders(HeaderConfig{EnableHSTS: true})(okHandler())

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	h.ServeHTTP(rec, req)

	got := rec.Result().Header.Get("Strict-Transport-Security")
	if got == "" {
		t.Fatalf("HSTS is empty when enabled")
	}
}

func TestSecurityHeadersPassesThroughResponse(t *testing.T) {
	t.Parallel()
	h := SecurityHeaders(HeaderConfig{EnableHSTS: false})(okHandler())
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status got %d want 200", rec.Code)
	}
	if rec.Body.String() != "ok" {
		t.Fatalf("body got %q want ok", rec.Body.String())
	}
}
```

- [ ] Step 2: テストが失敗することを確認する

Run:
```bash
cd /Users/yujiokamoto/devs/golang/feedflow-go-htmx && go test ./internal/auth/ -run TestSecurityHeaders -v
```
Expected: コンパイルエラーで失敗します。`undefined: SecurityHeaders`や`undefined: HeaderConfig`と表示されます。

- [ ] Step 3: 最小実装を書く

Create `internal/auth/headers.go`:
```go
package auth

import "net/http"

// HeaderConfig セキュリティヘッダの付与方針です。
type HeaderConfig struct {
	EnableHSTS bool // HTTPS公開時にStrict-Transport-Securityを付けるかどうかです
}

// contentSecurityPolicy feedflowのCSPです。
// HTMXとAlpine.jsをベンダーしてselfから配信するためscript-srcはselfに限定します。
// Alpine.jsのインライン属性を許すためstyle-srcにunsafe-inlineを含めます。
const contentSecurityPolicy = "default-src 'self'; " +
	"script-src 'self'; " +
	"style-src 'self' 'unsafe-inline'; " +
	"img-src 'self' data: https:; " +
	"font-src 'self'; " +
	"connect-src 'self'; " +
	"frame-ancestors 'none'; " +
	"base-uri 'self'; " +
	"form-action 'self'; " +
	"object-src 'none'"

// permissionsPolicy 不要なブラウザ機能を一括で無効化します。
const permissionsPolicy = "camera=(), microphone=(), geolocation=(), interest-cohort=()"

// hstsValue 1年間のHSTSとサブドメインとプリロードを指示します。
const hstsValue = "max-age=31536000; includeSubDomains; preload"

// SecurityHeaders 全レスポンスにセキュリティヘッダを付与するミドルウェアを返します。
// 設計書のセクション9.1のヘッダ(HSTS、X-Content-Type-Options、Referrer-Policy、Permissions-Policyほか)を付けます。
// HSTSはHeaderConfig.EnableHSTSがtrueのときだけ付けます。
func SecurityHeaders(cfg HeaderConfig) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			h := w.Header()
			h.Set("X-Content-Type-Options", "nosniff")
			h.Set("X-Frame-Options", "DENY")
			h.Set("Referrer-Policy", "strict-origin-when-cross-origin")
			h.Set("Permissions-Policy", permissionsPolicy)
			h.Set("Content-Security-Policy", contentSecurityPolicy)
			if cfg.EnableHSTS {
				h.Set("Strict-Transport-Security", hstsValue)
			}
			next.ServeHTTP(w, r)
		})
	}
}
```

- [ ] Step 4: テストが通ることを確認する

Run:
```bash
cd /Users/yujiokamoto/devs/golang/feedflow-go-htmx && go test ./internal/auth/ -run TestSecurityHeaders -v
```
Expected: 全サブテストがPASSします。

- [ ] Step 5: gofmtを適用してコミットする

```bash
cd /Users/yujiokamoto/devs/golang/feedflow-go-htmx && gofmt -w internal/auth/headers.go internal/auth/headers_test.go && git add internal/auth/headers.go internal/auth/headers_test.go && git commit -m "feat: セキュリティヘッダ付与のミドルウェア関数を追加する"
```

---

## Task 9: 認証ミドルウェアとクライアントIP抽出(TDD)

Files:
- Test: `/Users/yujiokamoto/devs/golang/feedflow-go-htmx/internal/auth/middleware_test.go`
- Create: `/Users/yujiokamoto/devs/golang/feedflow-go-htmx/internal/auth/middleware.go`

セッション必須のルートを守るミドルウェアと、CSRF検証をPOSTなど非安全メソッドに強制するミドルウェアを用意します。ハンドラ(Phase7)が認証要否に応じて適用します。クライアントIP抽出はレートリミットのキーに使います。認証済みユーザー名はcontextに載せて後続ハンドラへ渡します。

- [ ] Step 1: 失敗するテストを書く

Create `internal/auth/middleware_test.go`:
```go
package auth

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestRequireAuthRedirectsWhenNoSession(t *testing.T) {
	t.Parallel()
	clk := &fakeClock{now: time.Now()}
	sessions := NewSessionStore(SessionConfig{Clock: clk, TTL: time.Hour, CookieName: "feedflow_session", Secure: false})

	called := false
	guarded := RequireAuth(sessions, "/login")(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		called = true
	}))

	rec := httptest.NewRecorder()
	guarded.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))

	if called {
		t.Fatalf("guarded handler was called without a session")
	}
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("status got %d want 303", rec.Code)
	}
	if loc := rec.Result().Header.Get("Location"); loc != "/login" {
		t.Fatalf("redirect Location got %q want /login", loc)
	}
}

func TestRequireAuthPassesWithValidSessionAndInjectsUser(t *testing.T) {
	t.Parallel()
	clk := &fakeClock{now: time.Now()}
	sessions := NewSessionStore(SessionConfig{Clock: clk, TTL: time.Hour, CookieName: "feedflow_session", Secure: false})

	issueRec := httptest.NewRecorder()
	if err := sessions.Issue(issueRec, "owner"); err != nil {
		t.Fatalf("Issue returned error: %v", err)
	}
	cookie := issueRec.Result().Cookies()[0]

	var gotUser string
	guarded := RequireAuth(sessions, "/login")(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		gotUser = UsernameFromContext(r.Context())
	}))

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.AddCookie(cookie)
	guarded.ServeHTTP(rec, req)

	if gotUser != "owner" {
		t.Fatalf("UsernameFromContext got %q want owner", gotUser)
	}
}

func TestRequireCSRFAllowsSafeMethods(t *testing.T) {
	t.Parallel()
	clk := &fakeClock{now: time.Now()}
	sessions := NewSessionStore(SessionConfig{Clock: clk, TTL: time.Hour, CookieName: "feedflow_session", Secure: false})
	csrf := NewCSRFStore()

	called := false
	guarded := RequireCSRF(sessions, csrf)(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		called = true
	}))

	rec := httptest.NewRecorder()
	guarded.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))
	if !called {
		t.Fatalf("GET should bypass CSRF but handler was not called")
	}
}

func TestRequireCSRFRejectsPostWithoutToken(t *testing.T) {
	t.Parallel()
	clk := &fakeClock{now: time.Now()}
	sessions := NewSessionStore(SessionConfig{Clock: clk, TTL: time.Hour, CookieName: "feedflow_session", Secure: false})
	csrf := NewCSRFStore()

	issueRec := httptest.NewRecorder()
	if err := sessions.Issue(issueRec, "owner"); err != nil {
		t.Fatalf("Issue returned error: %v", err)
	}
	cookie := issueRec.Result().Cookies()[0]
	// セッションIDはCookieの値です。そのIDでCSRFトークンを発行しておきます。
	if _, err := csrf.Issue(cookie.Value); err != nil {
		t.Fatalf("csrf Issue returned error: %v", err)
	}

	called := false
	guarded := RequireCSRF(sessions, csrf)(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		called = true
	}))

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/feeds", nil)
	req.AddCookie(cookie)
	guarded.ServeHTTP(rec, req)

	if called {
		t.Fatalf("POST without CSRF token should be rejected")
	}
	if rec.Code != http.StatusForbidden {
		t.Fatalf("status got %d want 403", rec.Code)
	}
}

func TestRequireCSRFAcceptsPostWithToken(t *testing.T) {
	t.Parallel()
	clk := &fakeClock{now: time.Now()}
	sessions := NewSessionStore(SessionConfig{Clock: clk, TTL: time.Hour, CookieName: "feedflow_session", Secure: false})
	csrf := NewCSRFStore()

	issueRec := httptest.NewRecorder()
	if err := sessions.Issue(issueRec, "owner"); err != nil {
		t.Fatalf("Issue returned error: %v", err)
	}
	cookie := issueRec.Result().Cookies()[0]
	token, err := csrf.Issue(cookie.Value)
	if err != nil {
		t.Fatalf("csrf Issue returned error: %v", err)
	}

	called := false
	guarded := RequireCSRF(sessions, csrf)(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		called = true
	}))

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/feeds", nil)
	req.AddCookie(cookie)
	req.Header.Set(CSRFHeaderName, token)
	guarded.ServeHTTP(rec, req)

	if !called {
		t.Fatalf("POST with valid CSRF token should be allowed")
	}
}

func TestClientIP(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name       string
		remoteAddr string
		want       string
	}{
		{name: "ホストとポート", remoteAddr: "203.0.113.5:54321", want: "203.0.113.5"},
		{name: "ポートなし", remoteAddr: "203.0.113.9", want: "203.0.113.9"},
		{name: "IPv6", remoteAddr: "[2001:db8::1]:443", want: "2001:db8::1"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			req := httptest.NewRequest(http.MethodGet, "/", nil)
			req.RemoteAddr = tt.remoteAddr
			if got := ClientIP(req); got != tt.want {
				t.Fatalf("ClientIP got %q want %q", got, tt.want)
			}
		})
	}
}
```

- [ ] Step 2: テストが失敗することを確認する

Run:
```bash
cd /Users/yujiokamoto/devs/golang/feedflow-go-htmx && go test ./internal/auth/ -run 'TestRequireAuth|TestRequireCSRF|TestClientIP' -v
```
Expected: コンパイルエラーで失敗します。`undefined: RequireAuth`や`undefined: ClientIP`と表示されます。

- [ ] Step 3: 最小実装を書く

Create `internal/auth/middleware.go`:
```go
package auth

import (
	"context"
	"net"
	"net/http"
)

// ctxKey contextに値を載せるための非公開キー型です。
type ctxKey int

// userCtxKey 認証済みユーザー名をcontextに載せるキーです。
const userCtxKey ctxKey = iota

// UsernameFromContext contextから認証済みユーザー名を取り出します。未設定のときは空文字列を返します。
func UsernameFromContext(ctx context.Context) string {
	if v, ok := ctx.Value(userCtxKey).(string); ok {
		return v
	}
	return ""
}

// RequireAuth 有効なセッションを持たないリクエストをloginPathへリダイレクトするミドルウェアを返します。
// 認証済みのときはユーザー名をcontextに載せて後続ハンドラへ渡します。
func RequireAuth(sessions *SessionStore, loginPath string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			username, ok := sessions.Validate(r)
			if !ok {
				http.Redirect(w, r, loginPath, http.StatusSeeOther)
				return
			}
			ctx := context.WithValue(r.Context(), userCtxKey, username)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// isSafeMethod CSRF検証を要しない安全なHTTPメソッドかどうかを返します。
func isSafeMethod(method string) bool {
	switch method {
	case http.MethodGet, http.MethodHead, http.MethodOptions, http.MethodTrace:
		return true
	default:
		return false
	}
}

// RequireCSRF 非安全メソッドのリクエストに対し、セッションに紐づくCSRFトークンの一致を要求するミドルウェアを返します。
// 安全なメソッドは素通しします。検証に失敗したら403を返します。
func RequireCSRF(sessions *SessionStore, csrf *CSRFStore) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if isSafeMethod(r.Method) {
				next.ServeHTTP(w, r)
				return
			}
			c, err := r.Cookie(sessions.name)
			if err != nil {
				http.Error(w, "forbidden", http.StatusForbidden)
				return
			}
			if !csrf.Verify(c.Value, r) {
				http.Error(w, "forbidden", http.StatusForbidden)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// ClientIP リクエストの送信元IPアドレスを返します。ポート付きのRemoteAddrからホスト部だけを取り出します。
// レートリミットのキーに使います。
func ClientIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}
```

- [ ] Step 4: テストが通ることを確認する

Run:
```bash
cd /Users/yujiokamoto/devs/golang/feedflow-go-htmx && go test ./internal/auth/ -run 'TestRequireAuth|TestRequireCSRF|TestClientIP' -v
```
Expected: 全サブテストがPASSします。

- [ ] Step 5: gofmtを適用してコミットする

```bash
cd /Users/yujiokamoto/devs/golang/feedflow-go-htmx && gofmt -w internal/auth/middleware.go internal/auth/middleware_test.go && git add internal/auth/middleware.go internal/auth/middleware_test.go && git commit -m "feat: 認証と CSRF のミドルウェアとクライアント IP 抽出を追加する"
```

---

## Task 10: 初回セットアップ無効化ミドルウェア(TDD)

Files:
- Test: `/Users/yujiokamoto/devs/golang/feedflow-go-htmx/internal/auth/setup_test.go`
- Create: `/Users/yujiokamoto/devs/golang/feedflow-go-htmx/internal/auth/setup.go`

初回セットアップ画面はuser.jsonが未登録のときだけ到達でき、登録済みでは無効化します。設計書のセクション9.3の要件です。Managerの登録状態を見て、登録済みのアクセスはログイン画面へリダイレクトします。

- [ ] Step 1: 失敗するテストを書く

Create `internal/auth/setup_test.go`:
```go
package auth

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/okamyuji/feedflow-go-htmx/internal/domain"
)

func TestRequireSetupAvailableAllowsWhenUnregistered(t *testing.T) {
	t.Parallel()
	repo := &fakeUserRepo{user: domain.User{}}
	m := NewManager(repo, testParams())

	called := false
	guarded := RequireSetupAvailable(m, "/login")(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		called = true
	}))

	rec := httptest.NewRecorder()
	guarded.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/setup", nil))

	if !called {
		t.Fatalf("setup handler should run when no owner is registered")
	}
	if rec.Code == http.StatusSeeOther {
		t.Fatalf("unexpected redirect when setup is available")
	}
}

func TestRequireSetupAvailableRedirectsWhenRegistered(t *testing.T) {
	t.Parallel()
	repo := &fakeUserRepo{user: domain.User{Username: "owner", PasswordHash: "scrypt$1024$8$1$c2FsdA$aGFzaA"}}
	m := NewManager(repo, testParams())

	called := false
	guarded := RequireSetupAvailable(m, "/login")(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		called = true
	}))

	rec := httptest.NewRecorder()
	guarded.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/setup", nil))

	if called {
		t.Fatalf("setup handler must not run when owner is already registered")
	}
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("status got %d want 303", rec.Code)
	}
	if loc := rec.Result().Header.Get("Location"); loc != "/login" {
		t.Fatalf("redirect Location got %q want /login", loc)
	}
}

func TestRequireSetupAvailableServerErrorOnRepoFailure(t *testing.T) {
	t.Parallel()
	repo := &fakeUserRepo{loadErr: errLoad}
	m := NewManager(repo, testParams())

	guarded := RequireSetupAvailable(m, "/login")(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))

	rec := httptest.NewRecorder()
	guarded.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/setup", nil))

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status got %d want 500 on repo failure", rec.Code)
	}
}
```

このテストは`errLoad`という共通のエラー値を使います。次のStepで実装ファイルとは別に小さなテストヘルパへ加えても良いのですが、ここでは実装ファイル側に置かず、テスト専用に同パッケージのmanager_test.goへ次の1行を追加します。Run前に追記してください。

manager_test.goの末尾に追記:
```go
// errLoad リポジトリ読み込み失敗を模すテスト用のエラー値です。
var errLoad = errors.New("load failed")
```

- [ ] Step 2: errLoad を manager_test.go に追記する

Run:
```bash
cd /Users/yujiokamoto/devs/golang/feedflow-go-htmx && printf '\n// errLoad リポジトリ読み込み失敗を模すテスト用のエラー値です。\nvar errLoad = errors.New("load failed")\n' >> internal/auth/manager_test.go
```
Expected: エラーなく完了します。manager_test.goの末尾にerrLoadの定義が追加されます。

- [ ] Step 3: テストが失敗することを確認する

Run:
```bash
cd /Users/yujiokamoto/devs/golang/feedflow-go-htmx && go test ./internal/auth/ -run TestRequireSetupAvailable -v
```
Expected: コンパイルエラーで失敗します。`undefined: RequireSetupAvailable`と表示されます。

- [ ] Step 4: 最小実装を書く

Create `internal/auth/setup.go`:
```go
package auth

import "net/http"

// RequireSetupAvailable 初回セットアップ画面の可否を判定するミドルウェアを返します。
// 所有者が未登録のときだけ後続ハンドラへ通します。登録済みのときはloginPathへリダイレクトします。
// リポジトリ読み込みに失敗したときは500を返します。設計書のセクション9.3に対応します。
func RequireSetupAvailable(m *Manager, loginPath string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			needs, err := m.NeedsSetup()
			if err != nil {
				http.Error(w, "internal server error", http.StatusInternalServerError)
				return
			}
			if !needs {
				http.Redirect(w, r, loginPath, http.StatusSeeOther)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}
```

- [ ] Step 5: テストが通ることを確認する

Run:
```bash
cd /Users/yujiokamoto/devs/golang/feedflow-go-htmx && go test ./internal/auth/ -run TestRequireSetupAvailable -v
```
Expected: 全サブテストがPASSします。

- [ ] Step 6: gofmtを適用してコミットする

```bash
cd /Users/yujiokamoto/devs/golang/feedflow-go-htmx && gofmt -w internal/auth/setup.go internal/auth/manager_test.go && git add internal/auth/setup.go internal/auth/manager_test.go && git commit -m "feat: 初回セットアップ無効化ミドルウェアを追加する"
```

---

## Task 11: パッケージ全体テストと品質ゲート

Files:
- 変更なし

- [ ] Step 1: authパッケージの全テストを race で実行する

Run:
```bash
cd /Users/yujiokamoto/devs/golang/feedflow-go-htmx && go test -race -count=1 ./internal/auth/...
```
Expected: `ok  github.com/okamyuji/feedflow-go-htmx/internal/auth`と表示されます。

- [ ] Step 2: カバレッジを確認する

Run:
```bash
cd /Users/yujiokamoto/devs/golang/feedflow-go-htmx && go test -coverprofile=coverage.out ./internal/auth/... && go tool cover -func=coverage.out | tail -n 1
```
Expected: authパッケージの合計カバレッジが80パーセント前後以上になります。目安であり厳密な合否基準ではありません。

- [ ] Step 3: 品質ゲートを実行する

Run:
```bash
cd /Users/yujiokamoto/devs/golang/feedflow-go-htmx && bash scripts/quality-gate.sh
```
Expected: `all quality checks passed`で終わります。lintやvetやgosecの指摘が出たら修正してから再実行します。

- [ ] Step 4: 品質ゲート緑のままコミットする

```bash
cd /Users/yujiokamoto/devs/golang/feedflow-go-htmx && git add -A && git commit -m "chore: Phase6 の認証とセキュリティで品質ゲートを緑化する"
```
Expected: コミット時にquality-gateが走り、緑のままコミットされます。差分が無ければこのコミットは省略できます。

---

## Phase6 完了条件

- [ ] `go test -race ./internal/auth/...`が通る
- [ ] scryptのHashPasswordとVerifyPasswordが乱数ソルトと定数時間比較で動作する
- [ ] SessionStoreがHttpOnlyとSameSite=Laxを必須にしSecureを設定で切り替え、期限切れと未知Cookieを拒否する
- [ ] CSRFStoreがセッション単位のトークンをヘッダとフォームの両方で検証し、不正トークンと未知セッションを拒否する
- [ ] RateLimiterが標準ライブラリのみでキーごとのトークンバケットを実装しバースト上限と時間補充とキー分離を満たす
- [ ] Managerが初回セットアップをuser.json未登録時のみ許し、登録済みのSetupをErrAlreadyRegisteredで拒否する
- [ ] SecurityHeadersが設計書セクション9.1のヘッダを付与しHSTSを設定で切り替える
- [ ] RequireAuthとRequireCSRFとRequireSetupAvailableのミドルウェアが期待どおりに通過と拒否を行う
- [ ] authパッケージがport.RepositoryのUser読み書きにインターフェース経由で依存し具象型に直接依存しない
- [ ] `bash scripts/quality-gate.sh`が緑である
- [ ] コミットが規約に沿って積まれている
```
