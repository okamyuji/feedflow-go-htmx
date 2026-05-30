package feed

import (
	"strings"
	"testing"
)

// TestParseStripsIllegalXMLChars 実在のフィードには XML 1.0 で不正な制御文字
// (U+0008 など) が紛れ込むことがあります。encoding/xml は Strict=false でも
// これを拒否するため、パース前に除去して購読できることを担保します。
func TestParseStripsIllegalXMLChars(t *testing.T) {
	// description 本文に U+0008 (バックスペース) を埋め込んだ RSS2.0。
	raw := "<?xml version=\"1.0\"?>\n" +
		"<rss version=\"2.0\"><channel>\n" +
		"<title>sample</title><link>https://example.com</link>\n" +
		"<item><title>hello\x08world</title><link>https://example.com/1</link>\n" +
		"<guid>https://example.com/1</guid>\n" +
		"<description>body\x08text</description></item>\n" +
		"</channel></rss>\n"

	p := NewXMLParser()
	parsed, err := p.Parse([]byte(raw))
	if err != nil {
		t.Fatalf("Parse returned error for feed with illegal char: %v", err)
	}
	if len(parsed.Items) != 1 {
		t.Fatalf("want 1 item, got %d", len(parsed.Items))
	}
	if got := parsed.Items[0].Title; got != "helloworld" {
		t.Errorf("title not sanitized: got %q want %q", got, "helloworld")
	}
	if strings.ContainsRune(parsed.Items[0].Summary, 0x08) {
		t.Errorf("summary still contains illegal U+0008: %q", parsed.Items[0].Summary)
	}
}

// TestSanitizeXMLCharsKeepsValid 正常な文字(タブ/改行/日本語/絵文字)は保持し、
// 不正な制御文字のみを除去することを確認します。
func TestSanitizeXMLCharsKeepsValid(t *testing.T) {
	in := []byte("a\tb\nc\rd 日本語 😀\x00\x08\x0b\x0c\x1fe")
	got := string(sanitizeXMLChars(in))
	want := "a\tb\nc\rd 日本語 😀e"
	if got != want {
		t.Errorf("sanitizeXMLChars = %q, want %q", got, want)
	}
}
