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
