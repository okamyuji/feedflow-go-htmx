package feed

import (
	"testing"
	"time"

	"github.com/okamyuji/feedflow-go-htmx/internal/port"
)

const atomSample = `<?xml version="1.0" encoding="utf-8"?>
<feed xmlns="http://www.w3.org/2005/Atom">
  <title>Example Atom</title>
  <link href="https://example.com/" rel="alternate"/>
  <link href="https://example.com/feed.xml" rel="self"/>
  <updated>2026-01-02T15:04:05Z</updated>
  <entry>
    <title>Atom Entry One</title>
    <id>urn:uuid:1</id>
    <link href="https://example.com/atom-one" rel="alternate"/>
    <link href="https://example.com/atom-one/comments" rel="replies"/>
    <summary>atom summary</summary>
    <content type="html">&lt;p&gt;atom body&lt;/p&gt;</content>
    <author><name>Bob</name></author>
    <published>2026-01-01T00:00:00Z</published>
    <updated>2026-01-02T12:00:00Z</updated>
  </entry>
  <entry>
    <title>Atom Entry Two</title>
    <id>urn:uuid:2</id>
    <link href="https://example.com/atom-two"/>
    <updated>2026-01-03T00:00:00Z</updated>
  </entry>
</feed>`

func TestParseAtom(t *testing.T) {
	p := NewXMLParser()
	got, err := p.Parse([]byte(atomSample))
	if err != nil {
		t.Fatalf("Parse returned error: %v", err)
	}
	if got.Format != port.FormatAtom {
		t.Fatalf("Format got %q want %q", got.Format, port.FormatAtom)
	}
	if got.Title != "Example Atom" {
		t.Fatalf("Title got %q", got.Title)
	}
	if got.SiteURL != "https://example.com/" {
		t.Fatalf("SiteURL got %q want alternate link", got.SiteURL)
	}
	if len(got.Items) != 2 {
		t.Fatalf("Items len got %d want 2", len(got.Items))
	}

	one := got.Items[0]
	if one.Title != "Atom Entry One" {
		t.Fatalf("one Title got %q", one.Title)
	}
	if one.GUID != "urn:uuid:1" {
		t.Fatalf("one GUID got %q", one.GUID)
	}
	if one.Link != "https://example.com/atom-one" {
		t.Fatalf("one Link got %q want alternate link", one.Link)
	}
	if one.Summary != "atom summary" {
		t.Fatalf("one Summary got %q", one.Summary)
	}
	if one.Content != "<p>atom body</p>" {
		t.Fatalf("one Content got %q", one.Content)
	}
	if one.Author != "Bob" {
		t.Fatalf("one Author got %q", one.Author)
	}
	wantTime := time.Date(2026, 1, 2, 12, 0, 0, 0, time.UTC)
	if !one.PublishedAt.Equal(wantTime) {
		t.Fatalf("one PublishedAt got %v want updated %v", one.PublishedAt, wantTime)
	}

	two := got.Items[1]
	if two.Link != "https://example.com/atom-two" {
		t.Fatalf("two Link got %q want rel未指定のhref", two.Link)
	}
}
