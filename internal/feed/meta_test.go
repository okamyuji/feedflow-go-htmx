package feed_test

import (
	"strings"
	"testing"

	"github.com/okamyuji/feedflow-go-htmx/internal/feed"
)

func TestExtractMetaTitle(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name        string
		body        string
		contentType string
		want        string
	}{
		{
			name:        "og:titleを優先する",
			body:        `<html><head><meta property="og:title" content="OGタイトル"><title>タイトル要素</title></head><body></body></html>`,
			contentType: "text/html; charset=utf-8",
			want:        "OGタイトル",
		},
		{
			name:        "og:titleが無ければtitle要素を使う",
			body:        `<html><head><title>タイトル要素</title></head><body></body></html>`,
			contentType: "text/html; charset=utf-8",
			want:        "タイトル要素",
		},
		{
			name:        "どちらも無ければ空",
			body:        `<html><head></head><body><p>本文</p></body></html>`,
			contentType: "text/html; charset=utf-8",
			want:        "",
		},
		{
			name:        "og:titleが空白のみならtitle要素にフォールバックする",
			body:        `<html><head><meta property="og:title" content="   "><title>タイトル要素</title></head></html>`,
			contentType: "text/html; charset=utf-8",
			want:        "タイトル要素",
		},
		{
			name:        "前後空白を除去し連続空白を1つに畳む",
			body:        "<html><head><title>  前 \n\t 後  </title></head></html>",
			contentType: "text/html; charset=utf-8",
			want:        "前 後",
		},
		{
			name:        "name属性のog:titleも拾う",
			body:        `<html><head><meta name="og:title" content="name属性のOG"></head></html>`,
			contentType: "text/html; charset=utf-8",
			want:        "name属性のOG",
		},
		{
			name:        "content属性が無いog:titleはtitle要素にフォールバックする",
			body:        `<html><head><meta property="og:title"><title>タイトル要素</title></head></html>`,
			contentType: "text/html; charset=utf-8",
			want:        "タイトル要素",
		},
		{
			name:        "og:title以外のmetaは拾わない",
			body:        `<html><head><meta property="og:description" content="説明"><title>タイトル要素</title></head></html>`,
			contentType: "text/html; charset=utf-8",
			want:        "タイトル要素",
		},
		{
			name:        "最初のtitle要素を採用する",
			body:        `<html><head><title>1つ目</title><title>2つ目</title></head></html>`,
			contentType: "text/html; charset=utf-8",
			want:        "1つ目",
		},
		{
			name:        "壊れたHTMLでもpanicしない",
			body:        `<html><head><title>壊れた`,
			contentType: "text/html",
			want:        "壊れた",
		},
		{
			name:        "空のbodyで空を返す",
			body:        ``,
			contentType: "text/html",
			want:        "",
		},
		{
			name:        "Content-Typeが空でも読める",
			body:        `<html><head><title>タイトル要素</title></head></html>`,
			contentType: "",
			want:        "タイトル要素",
		},
		{
			name:        "壊れたContent-Typeでも読める",
			body:        `<html><head><title>タイトル要素</title></head></html>`,
			contentType: "text/html; charset=",
			want:        "タイトル要素",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := feed.ExtractMeta([]byte(tt.body), tt.contentType)
			if got.Title != tt.want {
				t.Errorf("ExtractMeta().Title = %q, want %q", got.Title, tt.want)
			}
		})
	}
}

func TestExtractMetaTitleTruncatesToMaxRunes(t *testing.T) {
	t.Parallel()
	long := strings.Repeat("あ", 300)
	got := feed.ExtractMeta([]byte("<html><head><title>"+long+"</title></head></html>"), "text/html; charset=utf-8")
	if n := len([]rune(got.Title)); n != 256 {
		t.Errorf("title rune length = %d, want 256", n)
	}
}

func TestExtractMetaTitleKeepsExactMaxRunes(t *testing.T) {
	t.Parallel()
	exact := strings.Repeat("あ", 256)
	got := feed.ExtractMeta([]byte("<html><head><title>"+exact+"</title></head></html>"), "text/html; charset=utf-8")
	if got.Title != exact {
		t.Errorf("title of exactly the limit was altered: len = %d, want 256", len([]rune(got.Title)))
	}
}

func TestExtractMetaDecodesShiftJIS(t *testing.T) {
	t.Parallel()
	// Shift_JISで<html><head><title>日本語</title></head></html>をエンコードしたバイト列です。
	sjis := []byte{
		0x3c, 0x68, 0x74, 0x6d, 0x6c, 0x3e, 0x3c, 0x68, 0x65, 0x61, 0x64, 0x3e,
		0x3c, 0x74, 0x69, 0x74, 0x6c, 0x65, 0x3e,
		0x93, 0xfa, 0x96, 0x7b, 0x8c, 0xea,
		0x3c, 0x2f, 0x74, 0x69, 0x74, 0x6c, 0x65, 0x3e,
		0x3c, 0x2f, 0x68, 0x65, 0x61, 0x64, 0x3e, 0x3c, 0x2f, 0x68, 0x74, 0x6d, 0x6c, 0x3e,
	}
	got := feed.ExtractMeta(sjis, "text/html; charset=Shift_JIS")
	if got.Title != "日本語" {
		t.Errorf("ExtractMeta().Title = %q, want %q", got.Title, "日本語")
	}
}

func TestExtractMetaDecodesEUCJP(t *testing.T) {
	t.Parallel()
	// EUC-JPで<html><head><title>日本語</title></head></html>をエンコードしたバイト列です。
	eucjp := []byte{
		0x3c, 0x68, 0x74, 0x6d, 0x6c, 0x3e, 0x3c, 0x68, 0x65, 0x61, 0x64, 0x3e,
		0x3c, 0x74, 0x69, 0x74, 0x6c, 0x65, 0x3e,
		0xc6, 0xfc, 0xcb, 0xdc, 0xb8, 0xec,
		0x3c, 0x2f, 0x74, 0x69, 0x74, 0x6c, 0x65, 0x3e,
		0x3c, 0x2f, 0x68, 0x65, 0x61, 0x64, 0x3e, 0x3c, 0x2f, 0x68, 0x74, 0x6d, 0x6c, 0x3e,
	}
	got := feed.ExtractMeta(eucjp, "text/html; charset=EUC-JP")
	if got.Title != "日本語" {
		t.Errorf("ExtractMeta().Title = %q, want %q", got.Title, "日本語")
	}
}

func TestExtractMetaDecodesCharsetFromMetaTag(t *testing.T) {
	t.Parallel()
	// Content-Typeに文字コードが無くても、meta charsetから解決します。
	sjis := append(
		[]byte(`<html><head><meta charset="Shift_JIS"><title>`),
		append([]byte{0x93, 0xfa, 0x96, 0x7b, 0x8c, 0xea}, []byte(`</title></head></html>`)...)...,
	)
	got := feed.ExtractMeta(sjis, "text/html")
	if got.Title != "日本語" {
		t.Errorf("ExtractMeta().Title = %q, want %q", got.Title, "日本語")
	}
}
