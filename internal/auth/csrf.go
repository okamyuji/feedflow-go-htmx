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
