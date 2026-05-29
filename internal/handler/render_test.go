package handler

import (
	"strings"
	"testing"
	"time"
)

func TestFormatJST(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		in   time.Time
		want string
	}{
		{
			name: "UTC を JST に変換する",
			in:   time.Date(2026, 5, 29, 0, 0, 0, 0, time.UTC),
			want: "2026-05-29 09:00",
		},
		{
			name: "ゼロ値は空文字を返す",
			in:   time.Time{},
			want: "",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := formatJST(tt.in); got != tt.want {
				t.Fatalf("formatJST got %q want %q", got, tt.want)
			}
		})
	}
}

func TestParseTemplates(t *testing.T) {
	t.Parallel()
	tmpl, err := parseTemplates()
	if err != nil {
		t.Fatalf("parseTemplates returned error: %v", err)
	}
	if tmpl.Lookup("base.html") == nil {
		t.Fatalf("base.html template not found")
	}
}

func TestTruncateRunes(t *testing.T) {
	t.Parallel()
	if got := truncateRunes("あいうえお", 3); got != "あいう" {
		t.Fatalf("truncateRunes got %q want %q", got, "あいう")
	}
	if got := truncateRunes("ab", 5); got != "ab" {
		t.Fatalf("truncateRunes got %q want %q", got, "ab")
	}
}

func TestRenderPartialWritesBody(t *testing.T) {
	t.Parallel()
	tmpl, err := parseTemplates()
	if err != nil {
		t.Fatalf("parseTemplates returned error: %v", err)
	}
	var sb strings.Builder
	if err := tmpl.ExecuteTemplate(&sb, "base.html", pageData{Title: "feedflow"}); err != nil {
		t.Fatalf("ExecuteTemplate returned error: %v", err)
	}
	if !strings.Contains(sb.String(), "feedflow") {
		t.Fatalf("rendered body does not contain title: %q", sb.String())
	}
}
