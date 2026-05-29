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
