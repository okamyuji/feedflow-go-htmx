package feed

import (
	"net/netip"
	"testing"
)

func TestIsBlockedAddr(t *testing.T) {
	tests := []struct {
		name string
		addr string
		want bool
	}{
		{name: "ループバックv4", addr: "127.0.0.1", want: true},
		{name: "ループバックv6", addr: "::1", want: true},
		{name: "プライベート10", addr: "10.0.0.1", want: true},
		{name: "プライベート172.16", addr: "172.16.5.4", want: true},
		{name: "プライベート192.168", addr: "192.168.1.1", want: true},
		{name: "リンクローカルv4", addr: "169.254.1.1", want: true},
		{name: "リンクローカルv6", addr: "fe80::1", want: true},
		{name: "ユニークローカルv6", addr: "fc00::1", want: true},
		{name: "未指定v4", addr: "0.0.0.0", want: true},
		{name: "未指定v6", addr: "::", want: true},
		{name: "マルチキャスト", addr: "224.0.0.1", want: true},
		{name: "クラウドメタデータ", addr: "169.254.169.254", want: true},
		{name: "グローバルv4", addr: "93.184.216.34", want: false},
		{name: "グローバルv6", addr: "2606:2800:220:1:248:1893:25c8:1946", want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			addr, err := netip.ParseAddr(tt.addr)
			if err != nil {
				t.Fatalf("ParseAddr(%q) returned error: %v", tt.addr, err)
			}
			if got := isBlockedAddr(addr); got != tt.want {
				t.Fatalf("isBlockedAddr(%q) got %v want %v", tt.addr, got, tt.want)
			}
		})
	}
}

func TestCheckScheme(t *testing.T) {
	tests := []struct {
		name    string
		raw     string
		wantErr bool
	}{
		{name: "httpsを許可", raw: "https://example.com/feed.xml", wantErr: false},
		{name: "httpを許可", raw: "http://example.com/feed.xml", wantErr: false},
		{name: "fileを拒否", raw: "file:///etc/passwd", wantErr: true},
		{name: "ftpを拒否", raw: "ftp://example.com/x", wantErr: true},
		{name: "gopherを拒否", raw: "gopher://example.com/", wantErr: true},
		{name: "スキーム無しを拒否", raw: "example.com/feed", wantErr: true},
		{name: "空文字を拒否", raw: "", wantErr: true},
		{name: "ホスト無しを拒否", raw: "https://", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := checkScheme(tt.raw)
			if tt.wantErr && err == nil {
				t.Fatalf("checkScheme(%q) errorを期待しましたがnilでした", tt.raw)
			}
			if !tt.wantErr && err != nil {
				t.Fatalf("checkScheme(%q) returned error: %v", tt.raw, err)
			}
		})
	}
}
