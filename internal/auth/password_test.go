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
