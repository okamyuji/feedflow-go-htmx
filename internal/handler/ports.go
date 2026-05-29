// Package handler HTTPハンドラとHTMX部分更新と画面描画を提供します。
package handler

import (
	"net/http"
)

// Session 認証済みセッションの描画用情報を表します。
// internal/authのセッションはユーザー名しか保持しないため、CSRFTokenはハンドラが描画直前にCSRFポートから取得して充填します。
type Session struct {
	ID        string // セッションIDです。CSRFトークンの発行と照合のキーに使います
	Username  string // ログイン中のユーザー名です
	CSRFToken string // このセッションに紐づくCSRFトークンです。requireAuthがCSRF.Issueで充填します
}

// Sessions Cookieセッションの発行と検証と破棄を担う抽象です。
// 実装はPhase6のinternal/authの*SessionStoreが満たします。メソッドは07-auth.mdの公開シグネチャに一致させます。
type Sessions interface {
	// Issue ユーザー名に対するセッションを発行し、Set-Cookieをwへ書き込みます。
	Issue(w http.ResponseWriter, username string) error
	// Validate リクエストのCookieから有効なセッションのユーザー名を返します。無効ならokがfalseになります。
	Validate(r *http.Request) (string, bool)
	// Destroy リクエストのセッションを破棄し、失効用のCookieをwへ書き込みます。
	Destroy(w http.ResponseWriter, r *http.Request)
}

// CSRF セッションIDごとのCSRFトークンの発行と取得と検証と破棄を担う抽象です。
// 実装はPhase6のinternal/authの*CSRFStoreが満たします。メソッドは07-auth.mdの公開シグネチャに一致させます。
type CSRF interface {
	// Issue セッションIDに紐づくCSRFトークンを発行して保持し、その値を返します。既に発行済みなら同じ値を返します。
	Issue(sessionID string) (string, error)
	// Token セッションIDに紐づく現在のトークンを返します。未発行ならokがfalseになります。
	Token(sessionID string) (string, bool)
	// Verify リクエストの送信トークンがセッションIDの保持値と一致するかを定数時間比較で検証します。
	Verify(sessionID string, r *http.Request) bool
	// Discard セッションIDに紐づくトークンを破棄します。ログアウト時に呼びます。
	Discard(sessionID string)
}

// RateLimiter トークンバケットによるレート制限を担う抽象です。実装はPhase6のinternal/authが提供します。
type RateLimiter interface {
	// Allow 指定キーに対する1回分の許可を消費し、許可されたかどうかを返します。
	Allow(key string) bool
}

// SetupGuard 初回セットアップの可否判定と登録と認証を担う抽象です。
// 実装はPhase6のinternal/authの*Managerが満たします。メソッドは07-auth.mdの公開シグネチャに一致させます。
type SetupGuard interface {
	// NeedsSetup ユーザーが未登録で初回セットアップが必要かどうかを返します。
	NeedsSetup() (bool, error)
	// Setup 初回セットアップとしてユーザー名とパスワードを登録します。登録済みのときはエラーを返します。
	Setup(username, password string) error
	// Authenticate ユーザー名とパスワードを検証します。成功時にtrueを返します。
	Authenticate(username, password string) (bool, error)
}
