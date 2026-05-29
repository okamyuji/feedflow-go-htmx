package feed

import (
	"testing"
)

const htmlWithFeeds = `<!DOCTYPE html>
<html>
<head>
  <title>Example Site</title>
  <link rel="alternate" type="application/rss+xml" title="RSS" href="/feed.xml">
  <link rel="alternate" type="application/atom+xml" title="Atom" href="https://cdn.example.com/atom.xml">
  <link rel="stylesheet" href="/style.css">
  <link rel="alternate" type="application/json" href="/feed.json">
</head>
<body><p>hello</p></body>
</html>`

func TestDiscover(t *testing.T) {
	got, err := Discover([]byte(htmlWithFeeds), "https://example.com/blog/")
	if err != nil {
		t.Fatalf("Discover returned error: %v", err)
	}
	want := []string{
		"https://example.com/feed.xml",
		"https://cdn.example.com/atom.xml",
	}
	if len(got) != len(want) {
		t.Fatalf("Discover len got %d (%v) want %d (%v)", len(got), got, len(want), want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("Discover[%d] got %q want %q", i, got[i], want[i])
		}
	}
}

func TestDiscoverNone(t *testing.T) {
	got, err := Discover([]byte(`<html><head></head><body></body></html>`), "https://example.com/")
	if err != nil {
		t.Fatalf("Discover returned error: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("フィードが無いHTMLでは空を期待しましたが %vでした", got)
	}
}

func TestDiscoverInvalidBase(t *testing.T) {
	_, err := Discover([]byte(htmlWithFeeds), "://bad-base")
	if err == nil {
		t.Fatal("不正なbaseURLでエラーを期待しましたがnilでした")
	}
}

func TestDiscoverRelativeResolution(t *testing.T) {
	html := `<head><link rel="alternate" type="application/rss+xml" href="../rss"></head>`
	got, err := Discover([]byte(html), "https://example.com/a/b/")
	if err != nil {
		t.Fatalf("Discover returned error: %v", err)
	}
	if len(got) != 1 || got[0] != "https://example.com/a/rss" {
		t.Fatalf("相対URLの解決結果が想定外ですgot %v", got)
	}
}
