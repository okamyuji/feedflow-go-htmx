// Package feedフィードの取得とパースと本文抽出と自動検出を提供します。
// port.Fetcherとport.FeedParserを実装し、上位層へはインターフェースの形で渡します。
package feed

import (
	"errors"
	"fmt"
	"net/netip"
	"net/url"
	"strings"
)

// ErrPrivateAddress 解決先がプライベートやループバックなど到達を禁じるアドレスだったことを表します。
var ErrPrivateAddress = errors.New("feed: destination resolves to a blocked address")

// ErrDisallowedScheme URLのスキームがhttpとhttpsのいずれでもないことを表します。
var ErrDisallowedScheme = errors.New("feed: only http and https schemes are allowed")

// isBlockedAddr 与えられたIPアドレスがSSRF対策で到達を禁じる分類に該当するかを返します。
// ループバック、プライベート、リンクローカル、ユニークローカル、未指定、マルチキャスト、インターフェースローカルを拒否します。
func isBlockedAddr(addr netip.Addr) bool {
	if !addr.IsValid() {
		return true
	}
	unmapped := addr.Unmap()
	return unmapped.IsLoopback() ||
		unmapped.IsPrivate() ||
		unmapped.IsLinkLocalUnicast() ||
		unmapped.IsLinkLocalMulticast() ||
		unmapped.IsMulticast() ||
		unmapped.IsUnspecified() ||
		unmapped.IsInterfaceLocalMulticast()
}

// checkScheme 与えられたURL文字列がhttpかhttpsで、ホストを持つことを確認します。
// 条件を満たさない場合は文脈付きのエラーを返します。
func checkScheme(raw string) error {
	u, err := url.Parse(raw)
	if err != nil {
		return fmt.Errorf("failed to parse url %q: %w", raw, err)
	}
	scheme := strings.ToLower(u.Scheme)
	if scheme != "http" && scheme != "https" {
		return fmt.Errorf("%w: got %q", ErrDisallowedScheme, u.Scheme)
	}
	if u.Host == "" {
		return fmt.Errorf("%w: url %q has no host", ErrDisallowedScheme, raw)
	}
	return nil
}
