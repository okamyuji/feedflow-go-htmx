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

	http.SetCookie(w, &http.Cookie{ //nolint:gosec // Secureは本番でtrueにする設定値で、ローカルHTTP開発のため切り替え可能にしています
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
	http.SetCookie(w, &http.Cookie{ //nolint:gosec // Secureは本番でtrueにする設定値で、ローカルHTTP開発のため切り替え可能にしています
		Name:     s.name,
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		Secure:   s.secure,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   -1,
	})
}
