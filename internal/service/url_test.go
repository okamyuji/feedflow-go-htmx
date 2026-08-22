package service

import (
	"errors"
	"testing"
)

func TestNormalizeURL(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		in   string
		want string
	}{
		{name: "そのまま", in: "https://example.com/a", want: "https://example.com/a"},
		{name: "末尾スラッシュを除去", in: "https://example.com/a/", want: "https://example.com/a"},
		{name: "末尾スラッシュが複数でも除去", in: "https://example.com/a///", want: "https://example.com/a"},
		{name: "ルートのスラッシュは残す", in: "https://example.com/", want: "https://example.com/"},
		{name: "パス無しはそのまま", in: "https://example.com", want: "https://example.com"},
		{name: "フラグメントを除去", in: "https://example.com/a#sec", want: "https://example.com/a"},
		{name: "schemeを小文字化", in: "HTTPS://example.com/a", want: "https://example.com/a"},
		{name: "hostを小文字化", in: "https://EXAMPLE.COM/a", want: "https://example.com/a"},
		{name: "パスの大文字は保持", in: "https://example.com/AbC", want: "https://example.com/AbC"},
		{name: "クエリを保持", in: "https://example.com/a?b=1&c=2", want: "https://example.com/a?b=1&c=2"},
		{name: "クエリと末尾スラッシュの併存", in: "https://example.com/a/?b=1", want: "https://example.com/a?b=1"},
		{name: "前後の空白を除去", in: "  https://example.com/a  ", want: "https://example.com/a"},
		{name: "ポート番号を保持", in: "http://example.com:8080/a", want: "http://example.com:8080/a"},
		{name: "httpスキームを許可", in: "http://example.com/a", want: "http://example.com/a"},
		{name: "日本語パスをエスケープして保持", in: "https://example.com/記事", want: "https://example.com/%E8%A8%98%E4%BA%8B"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := normalizeURL(tt.in)
			if err != nil {
				t.Fatalf("normalizeURL(%q) returned error: %v", tt.in, err)
			}
			if got != tt.want {
				t.Errorf("normalizeURL(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestNormalizeURLRejects(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		in   string
	}{
		{name: "空文字", in: ""},
		{name: "空白のみ", in: "   "},
		{name: "javascriptスキーム", in: "javascript:alert(1)"},
		{name: "大文字のjavascriptスキーム", in: "JavaScript:alert(1)"},
		{name: "fileスキーム", in: "file:///etc/passwd"},
		{name: "dataスキーム", in: "data:text/html,<h1>x</h1>"},
		{name: "ftpスキーム", in: "ftp://example.com/a"},
		{name: "スキーム無し", in: "example.com/a"},
		{name: "スキーム相対", in: "//example.com/a"},
		{name: "host無し", in: "http:///a"},
		{name: "解析できない文字列", in: "http://[::1"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := normalizeURL(tt.in)
			if !errors.Is(err, ErrInvalidURL) {
				t.Errorf("normalizeURL(%q) = (%q, %v), want ErrInvalidURL", tt.in, got, err)
			}
		})
	}
}

func TestNormalizeURLIsIdempotent(t *testing.T) {
	t.Parallel()
	inputs := []string{
		"https://EXAMPLE.com/a/#x",
		"http://example.com:8080/a/b/",
		"https://example.com/",
		"https://example.com/a?b=1",
	}
	for _, in := range inputs {
		once, err := normalizeURL(in)
		if err != nil {
			t.Fatalf("normalizeURL(%q) returned error: %v", in, err)
		}
		twice, err := normalizeURL(once)
		if err != nil {
			t.Fatalf("normalizeURL(%q) returned error: %v", once, err)
		}
		if once != twice {
			t.Errorf("normalizeURL is not idempotent for %q: %q then %q", in, once, twice)
		}
	}
}

func TestNormalizeURLPreservesEncodedSlash(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		in   string
		want string
	}{
		{name: "エンコード済みスラッシュを保持", in: "https://example.com/a%2Fb", want: "https://example.com/a%2Fb"},
		{name: "末尾除去後もエンコードを保持", in: "https://example.com/a%2Fb/", want: "https://example.com/a%2Fb"},
		{name: "エンコード済み疑問符を保持", in: "https://example.com/a%3Fb", want: "https://example.com/a%3Fb"},
		{name: "エンコード済みシャープを保持", in: "https://example.com/a%23b", want: "https://example.com/a%23b"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := normalizeURL(tt.in)
			if err != nil {
				t.Fatalf("normalizeURL(%q) returned error: %v", tt.in, err)
			}
			if got != tt.want {
				t.Errorf("normalizeURL(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestNormalizeURLStripsUserinfo(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		in   string
		want string
	}{
		{name: "ユーザー名とパスワード", in: "https://user:pass@example.com/a", want: "https://example.com/a"},
		{name: "ユーザー名のみ", in: "https://user@example.com/a", want: "https://example.com/a"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := normalizeURL(tt.in)
			if err != nil {
				t.Fatalf("normalizeURL(%q) returned error: %v", tt.in, err)
			}
			if got != tt.want {
				t.Errorf("normalizeURL(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}
