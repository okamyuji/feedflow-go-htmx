package feed

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/okamyuji/feedflow-go-htmx/internal/port"
)

func TestParseFromFiles(t *testing.T) {
	tests := []struct {
		file       string
		wantFormat port.FeedFormat
		wantTitle  string
		wantItem   string
	}{
		{file: "rss2.xml", wantFormat: port.FormatRSS2, wantTitle: "Sample RSS Feed", wantItem: "Hello RSS"},
		{file: "atom.xml", wantFormat: port.FormatAtom, wantTitle: "Sample Atom Feed", wantItem: "Hello Atom"},
		{file: "rdf.xml", wantFormat: port.FormatRDF, wantTitle: "Sample RDF Feed", wantItem: "Hello RDF"},
	}
	p := NewXMLParser()
	for _, tt := range tests {
		t.Run(tt.file, func(t *testing.T) {
			data, err := os.ReadFile(filepath.Join("testdata", tt.file))
			if err != nil {
				t.Fatalf("ReadFile returned error: %v", err)
			}
			got, err := p.Parse(data)
			if err != nil {
				t.Fatalf("Parse returned error: %v", err)
			}
			if got.Format != tt.wantFormat {
				t.Fatalf("Format got %q want %q", got.Format, tt.wantFormat)
			}
			if got.Title != tt.wantTitle {
				t.Fatalf("Title got %q want %q", got.Title, tt.wantTitle)
			}
			if len(got.Items) != 1 {
				t.Fatalf("Items len got %d want 1", len(got.Items))
			}
			if got.Items[0].Title != tt.wantItem {
				t.Fatalf("Item title got %q want %q", got.Items[0].Title, tt.wantItem)
			}
			if got.Items[0].GUID == "" {
				t.Fatal("GUIDは空であってはいけません")
			}
		})
	}
}
