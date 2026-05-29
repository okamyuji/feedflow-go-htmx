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

// errLoad リポジトリ読み込み失敗を模すテスト用のエラー値です。
var errLoad = errors.New("load failed")
