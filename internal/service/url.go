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
	if len(u.Path) > 1 {
		u.Path = strings.TrimRight(u.Path, "/")
		u.RawPath = ""
	}
	return u.String(), nil
}
