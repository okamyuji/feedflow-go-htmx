package feed

import (
	"testing"

	"github.com/okamyuji/feedflow-go-htmx/internal/port"
)

func TestDetectFormat(t *testing.T) {
	tests := []struct {
		name    string
		data    string
		want    port.FeedFormat
		wantErr bool
	}{
		{
			name: "RSS2.0",
			data: `<?xml version="1.0"?><rss version="2.0"><channel><title>x</title></channel></rss>`,
			want: port.FormatRSS2,
		},
		{
			name: "Atom",
			data: `<?xml version="1.0" encoding="utf-8"?><feed xmlns="http://www.w3.org/2005/Atom"><title>x</title></feed>`,
			want: port.FormatAtom,
		},
		{
			name: "RDFつまりRSS1.0",
			data: `<?xml version="1.0"?><rdf:RDF xmlns:rdf="http://www.w3.org/1999/02/22-rdf-syntax-ns#" xmlns="http://purl.org/rss/1.0/"><channel><title>x</title></channel></rdf:RDF>`,
			want: port.FormatRDF,
		},
		{
			name: "先頭にBOMと空白",
			data: "\ufeff   \n<rss version=\"2.0\"><channel></channel></rss>",
			want: port.FormatRSS2,
		},
		{
			name: "先頭にコメント",
			data: `<!-- generated --><feed xmlns="http://www.w3.org/2005/Atom"></feed>`,
			want: port.FormatAtom,
		},
		{
			name:    "未知のルート",
			data:    `<html><body>not a feed</body></html>`,
			wantErr: true,
		},
		{
			name:    "空入力",
			data:    ``,
			wantErr: true,
		},
		{
			name:    "開始要素が現れない断片",
			data:    `   <!-- comment only -->`,
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := detectFormat([]byte(tt.data))
			if tt.wantErr {
				if err == nil {
					t.Fatalf("errorを期待しましたがnilでした, got=%q", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("detectFormat returned error: %v", err)
			}
			if got != tt.want {
				t.Fatalf("detectFormat got %q want %q", got, tt.want)
			}
		})
	}
}
