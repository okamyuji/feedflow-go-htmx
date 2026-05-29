package feed

import (
	"strings"
	"testing"
)

const articleHTML = `<!DOCTYPE html>
<html>
<head><title>Article</title><style>.x{color:red}</style></head>
<body>
  <header><nav>menu menu menu menu</nav></header>
  <article>
    <h1>Real Title</h1>
    <p>This is the first substantial paragraph with enough text to matter.</p>
    <p>This is the second substantial paragraph that also has meaningful length.</p>
    <script>console.log("ignore me ignore me ignore me")</script>
  </article>
  <footer>copyright footer footer footer</footer>
</body>
</html>`

func TestExtract(t *testing.T) {
	got, err := Extract([]byte(articleHTML))
	if err != nil {
		t.Fatalf("Extract returned error: %v", err)
	}
	if !strings.Contains(got, "first substantial paragraph") {
		t.Fatalf("本文の一段落目が欠落していますgot %q", got)
	}
	if !strings.Contains(got, "second substantial paragraph") {
		t.Fatalf("本文の二段落目が欠落していますgot %q", got)
	}
	if strings.Contains(got, "console.log") {
		t.Fatalf("scriptの内容が混入していますgot %q", got)
	}
	if strings.Contains(got, "menu menu") {
		t.Fatalf("navの内容が混入していますgot %q", got)
	}
	if strings.Contains(got, "copyright footer") {
		t.Fatalf("footerの内容が混入していますgot %q", got)
	}
}

func TestExtractFallbackToBody(t *testing.T) {
	html := `<html><body><div><p>Only a div wraps this readable sentence of content.</p></div></body></html>`
	got, err := Extract([]byte(html))
	if err != nil {
		t.Fatalf("Extract returned error: %v", err)
	}
	if !strings.Contains(got, "readable sentence of content") {
		t.Fatalf("articleやmainが無くても本文を抽出するはずですがgot %q", got)
	}
}

func TestExtractEmpty(t *testing.T) {
	got, err := Extract([]byte(`<html><head></head><body></body></html>`))
	if err != nil {
		t.Fatalf("Extract returned error: %v", err)
	}
	if strings.TrimSpace(got) != "" {
		t.Fatalf("本文が無い場合は空を期待しましたが %qでした", got)
	}
}

func TestExtractInvalidHTMLStillParses(t *testing.T) {
	// net/htmlは壊れた断片も寛容にパースする。エラーにせず可能な範囲で抽出する。
	got, err := Extract([]byte(`<p>loose paragraph without wrapper tags at all here</p>`))
	if err != nil {
		t.Fatalf("Extract returned error: %v", err)
	}
	if !strings.Contains(got, "loose paragraph") {
		t.Fatalf("断片HTMLからも抽出するはずですがgot %q", got)
	}
}
