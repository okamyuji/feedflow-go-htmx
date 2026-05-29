package feed

import (
	"testing"
	"time"

	"github.com/okamyuji/feedflow-go-htmx/internal/port"
)

const rdfSample = `<?xml version="1.0" encoding="UTF-8"?>
<rdf:RDF
  xmlns:rdf="http://www.w3.org/1999/02/22-rdf-syntax-ns#"
  xmlns:dc="http://purl.org/dc/elements/1.1/"
  xmlns="http://purl.org/rss/1.0/">
  <channel rdf:about="https://example.com/rss">
    <title>Example RDF</title>
    <link>https://example.com/</link>
    <description>RDF feed</description>
  </channel>
  <item rdf:about="https://example.com/rdf-one">
    <title>RDF Item One</title>
    <link>https://example.com/rdf-one</link>
    <description>rdf summary one</description>
    <dc:creator>Carol</dc:creator>
    <dc:date>2026-02-01T09:30:00Z</dc:date>
  </item>
  <item>
    <title>RDF Item Two</title>
    <link>https://example.com/rdf-two</link>
  </item>
</rdf:RDF>`

func TestParseRDF(t *testing.T) {
	p := NewXMLParser()
	got, err := p.Parse([]byte(rdfSample))
	if err != nil {
		t.Fatalf("Parse returned error: %v", err)
	}
	if got.Format != port.FormatRDF {
		t.Fatalf("Format got %q want %q", got.Format, port.FormatRDF)
	}
	if got.Title != "Example RDF" {
		t.Fatalf("Title got %q", got.Title)
	}
	if got.SiteURL != "https://example.com/" {
		t.Fatalf("SiteURL got %q", got.SiteURL)
	}
	if len(got.Items) != 2 {
		t.Fatalf("Items len got %d want 2", len(got.Items))
	}

	one := got.Items[0]
	if one.Title != "RDF Item One" {
		t.Fatalf("one Title got %q", one.Title)
	}
	if one.GUID != "https://example.com/rdf-one" {
		t.Fatalf("one GUID got %q want rdf:about", one.GUID)
	}
	if one.Link != "https://example.com/rdf-one" {
		t.Fatalf("one Link got %q", one.Link)
	}
	if one.Summary != "rdf summary one" {
		t.Fatalf("one Summary got %q", one.Summary)
	}
	if one.Author != "Carol" {
		t.Fatalf("one Author got %q want dc:creator", one.Author)
	}
	wantTime := time.Date(2026, 2, 1, 9, 30, 0, 0, time.UTC)
	if !one.PublishedAt.Equal(wantTime) {
		t.Fatalf("one PublishedAt got %v want %v", one.PublishedAt, wantTime)
	}

	two := got.Items[1]
	if two.GUID != "https://example.com/rdf-two" {
		t.Fatalf("rdf:about欠落時はlinkで代替するはずですがgot %q", two.GUID)
	}
}
