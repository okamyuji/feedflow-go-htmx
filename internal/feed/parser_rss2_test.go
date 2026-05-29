package feed

import (
	"testing"
	"time"

	"github.com/okamyuji/feedflow-go-htmx/internal/port"
)

const rss2Sample = `<?xml version="1.0" encoding="UTF-8"?>
<rss version="2.0">
  <channel>
    <title>Example RSS</title>
    <link>https://example.com/</link>
    <description>An example feed</description>
    <item>
      <title>First Post</title>
      <link>https://example.com/first</link>
      <guid>https://example.com/first</guid>
      <description>summary one</description>
      <author>alice@example.com</author>
      <pubDate>Mon, 02 Jan 2006 15:04:05 -0700</pubDate>
    </item>
    <item>
      <title>Second Post</title>
      <link>https://example.com/second</link>
      <description>summary two</description>
      <pubDate>Tue, 03 Jan 2006 10:00:00 GMT</pubDate>
    </item>
  </channel>
</rss>`

func TestParseRSS2(t *testing.T) {
	p := NewXMLParser()
	got, err := p.Parse([]byte(rss2Sample))
	if err != nil {
		t.Fatalf("Parse returned error: %v", err)
	}
	if got.Format != port.FormatRSS2 {
		t.Fatalf("Format got %q want %q", got.Format, port.FormatRSS2)
	}
	if got.Title != "Example RSS" {
		t.Fatalf("Title got %q", got.Title)
	}
	if got.SiteURL != "https://example.com/" {
		t.Fatalf("SiteURL got %q", got.SiteURL)
	}
	if len(got.Items) != 2 {
		t.Fatalf("Items len got %d want 2", len(got.Items))
	}

	first := got.Items[0]
	if first.Title != "First Post" {
		t.Fatalf("first Title got %q", first.Title)
	}
	if first.Link != "https://example.com/first" {
		t.Fatalf("first Link got %q", first.Link)
	}
	if first.GUID != "https://example.com/first" {
		t.Fatalf("first GUID got %q", first.GUID)
	}
	if first.Summary != "summary one" {
		t.Fatalf("first Summary got %q", first.Summary)
	}
	if first.Author != "alice@example.com" {
		t.Fatalf("first Author got %q", first.Author)
	}
	wantTime := time.Date(2006, 1, 2, 15, 4, 5, 0, time.FixedZone("", -7*3600))
	if !first.PublishedAt.Equal(wantTime) {
		t.Fatalf("first PublishedAt got %v want %v", first.PublishedAt, wantTime)
	}

	second := got.Items[1]
	if second.GUID != "https://example.com/second" {
		t.Fatalf("GUID欠落時はLinkで代替するはずですがgot %q", second.GUID)
	}
}

func TestParseRSS2Invalid(t *testing.T) {
	p := NewXMLParser()
	_, err := p.Parse([]byte(`<html></html>`))
	if err == nil {
		t.Fatal("非フィード入力でエラーを期待しましたがnilでした")
	}
}

func TestParseRSS2BrokenXML(t *testing.T) {
	p := NewXMLParser()
	_, err := p.Parse([]byte(`<rss><channel`))
	if err == nil {
		t.Fatal("途中で切れたXMLでエラーを期待しましたがnilでした")
	}
}

// TestXMLParserSatisfiesPort XMLParserがport.FeedParserを満たすことを検証します。
func TestXMLParserSatisfiesPort(t *testing.T) {
	var _ port.FeedParser = NewXMLParser()
}
