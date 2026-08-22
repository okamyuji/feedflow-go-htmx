package service

import (
	"errors"
	"net/url"
	"strings"
)

// ErrInvalidURL 保存対象として受け付けられないURLが渡されたときに返すエラーです。
// httpとhttps以外のスキームや、ホストを持たないURLを弾きます。
var ErrInvalidURL = errors.New("invalid url")

// normalizeURL 入力URLを検証し、重複判定に使える正規形へ整えて返します。
// schemeとhostを小文字にし、フラグメントを除去し、ルート以外の末尾スラッシュを取り除きます。
// クエリは記事の同一性に関わるため保持します。
// httpとhttps以外のスキームとホスト無しのURLはErrInvalidURLを返します。
func normalizeURL(rawURL string) (string, error) {
	rawURL = strings.TrimSpace(rawURL)
	if rawURL == "" {
		return "", ErrInvalidURL
	}
	u, err := url.Parse(rawURL)
	if err != nil {
		return "", errors.Join(ErrInvalidURL, err)
	}
	scheme := strings.ToLower(u.Scheme)
	if scheme != "http" && scheme != "https" {
		return "", ErrInvalidURL
	}
	if u.Host == "" {
		return "", ErrInvalidURL
	}
	u.Scheme = scheme
	u.Host = strings.ToLower(u.Host)
	u.Fragment = ""
	u.RawFragment = ""
	// 認証情報はブックマークのリンクとして保存も表示もしたくないため落とします。
	// あわせて、同じページをuserinfoの有無で二重に保存してしまうのも防げます。
	u.User = nil
	if err := trimTrailingSlash(u); err != nil {
		return "", err
	}
	return u.String(), nil
}

// trimTrailingSlash ルート以外のパスの末尾スラッシュを取り除きます。
// エスケープ済みのパスを操作してPathとRawPathを両方そろえます。
// デコード済みのPathだけを触ってRawPathを捨てると、%2Fのような
// エンコード済みの区切り文字が生のスラッシュとして再解釈され、別のリソースを指してしまいます。
func trimTrailingSlash(u *url.URL) error {
	escaped := u.EscapedPath()
	if len(escaped) <= 1 {
		return nil
	}
	trimmed := strings.TrimRight(escaped, "/")
	if trimmed == escaped {
		return nil
	}
	if trimmed == "" {
		trimmed = "/"
	}
	decoded, err := url.PathUnescape(trimmed)
	if err != nil {
		return errors.Join(ErrInvalidURL, err)
	}
	u.Path = decoded
	u.RawPath = trimmed
	return nil
}
