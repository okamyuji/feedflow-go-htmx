package feed

import (
	"bytes"
	"fmt"
	"strings"

	"golang.org/x/net/html"
	"golang.org/x/net/html/atom"
)

// excludedTags 本文抽出で内容を無視する要素です。
var excludedTags = map[atom.Atom]struct{}{
	atom.Script:   {},
	atom.Style:    {},
	atom.Nav:      {},
	atom.Header:   {},
	atom.Footer:   {},
	atom.Aside:    {},
	atom.Form:     {},
	atom.Noscript: {},
}

// Extract 記事HTMLから本文テキストを段落区切りで抽出します。
// articleまたはmain要素があれば優先し、無ければ本文テキスト量が最大の要素を選びます。
// scriptやstyleやnavやheaderやfooterやasideの内容は除外します。
func Extract(data []byte) (string, error) {
	doc, err := html.Parse(bytes.NewReader(data))
	if err != nil {
		return "", fmt.Errorf("failed to parse html: %w", err)
	}

	if candidate := findPreferred(doc); candidate != nil {
		return collectText(candidate), nil
	}

	best := pickBestByTextLength(doc)
	if best == nil {
		return "", nil
	}
	return collectText(best), nil
}

// findPreferred articleまたはmain要素を深さ優先で探して返します。見つからなければnilを返します。
func findPreferred(n *html.Node) *html.Node {
	if n.Type == html.ElementNode && (n.DataAtom == atom.Article || n.DataAtom == atom.Main) {
		return n
	}
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		if found := findPreferred(c); found != nil {
			return found
		}
	}
	return nil
}

// pickBestByTextLength divやsectionやbodyのうち、除外要素を差し引いた本文テキスト量が最大の要素を返します。
func pickBestByTextLength(root *html.Node) *html.Node {
	var best *html.Node
	bestLen := 0
	var walk func(n *html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.ElementNode {
			switch n.DataAtom {
			case atom.Div, atom.Section, atom.Body, atom.Td, atom.Li:
				if l := len(collectText(n)); l > bestLen {
					bestLen = l
					best = n
				}
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(root)
	return best
}

// collectText 要素配下のテキストを、ブロック要素の境界で段落区切りにしながら連結します。
// 除外要素の内容は取り込みません。
func collectText(n *html.Node) string {
	var b strings.Builder
	var walk func(n *html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.ElementNode {
			if _, skip := excludedTags[n.DataAtom]; skip {
				return
			}
		}
		if n.Type == html.TextNode {
			text := strings.TrimSpace(n.Data)
			if text != "" {
				b.WriteString(text)
				b.WriteString(" ")
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
		if n.Type == html.ElementNode && isBlockElement(n.DataAtom) {
			b.WriteString("\n\n")
		}
	}
	walk(n)
	return normalizeWhitespace(b.String())
}

// isBlockElement 段落区切りを入れるブロック要素かどうかを返します。
func isBlockElement(a atom.Atom) bool {
	switch a {
	case atom.P, atom.Div, atom.Section, atom.Article, atom.H1, atom.H2, atom.H3,
		atom.H4, atom.H5, atom.H6, atom.Ul, atom.Ol, atom.Li, atom.Blockquote, atom.Pre:
		return true
	default:
		return false
	}
}

// normalizeWhitespace 連続する空白と過剰な改行を整理します。
func normalizeWhitespace(s string) string {
	lines := strings.Split(s, "\n")
	out := make([]string, 0, len(lines))
	for _, line := range lines {
		fields := strings.Fields(line)
		if len(fields) == 0 {
			continue
		}
		out = append(out, strings.Join(fields, " "))
	}
	return strings.Join(out, "\n\n")
}
