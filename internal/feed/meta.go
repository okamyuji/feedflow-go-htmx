package feed

import (
	"bytes"
	"io"
	"strings"

	"golang.org/x/net/html"
	"golang.org/x/net/html/atom"
	"golang.org/x/net/html/charset"
)

// maxTitleRunes 保存するタイトルの最大文字数です。
// 極端に長いタイトルが一覧の描画とJSONの肥大を招くため上限を設けます。
const maxTitleRunes = 256

// PageMeta HTMLページから抽出したメタデータです。
type PageMeta struct {
	Title string // og:titleまたはtitle要素から得たページ名です
}

// ExtractMeta HTMLバイト列とContent-Typeからページのメタデータを抽出します。
// og:titleを優先し、無ければtitle要素を使います。どちらも得られない場合はTitleが空になります。
// 文字コードはContent-Typeとmetaタグから解決します。
// パースに失敗した場合はゼロ値のPageMetaを返します。
func ExtractMeta(body []byte, contentType string) PageMeta {
	var reader io.Reader
	decoded, err := charset.NewReader(bytes.NewReader(body), contentType)
	if err != nil {
		// 文字コードを解決できない場合はバイト列をそのまま読み進めます。
		reader = bytes.NewReader(body)
	} else {
		reader = decoded
	}
	doc, err := html.Parse(reader)
	if err != nil {
		return PageMeta{}
	}
	ogTitle, titleTag := scanTitles(doc)
	title := normalizeTitle(ogTitle)
	if title == "" {
		title = normalizeTitle(titleTag)
	}
	return PageMeta{Title: title}
}

// scanTitles 文書を1度だけ走査してog:titleとtitle要素の内容を返します。
// 同じ要素が複数あれば最初のものを採用します。
func scanTitles(doc *html.Node) (ogTitle, titleTag string) {
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.ElementNode {
			switch n.DataAtom {
			case atom.Meta:
				if ogTitle == "" {
					if v, ok := ogTitleContent(n); ok {
						ogTitle = v
					}
				}
			case atom.Title:
				if titleTag == "" {
					titleTag = textContent(n)
				}
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(doc)
	return ogTitle, titleTag
}

// ogTitleContent meta要素がog:titleならそのcontent属性を返します。
// propertyとnameのどちらで指定されていても拾います。
func ogTitleContent(n *html.Node) (string, bool) {
	isOG := false
	content := ""
	for _, a := range n.Attr {
		switch strings.ToLower(a.Key) {
		case "property", "name":
			if strings.EqualFold(strings.TrimSpace(a.Val), "og:title") {
				isOG = true
			}
		case "content":
			content = a.Val
		}
	}
	if !isOG {
		return "", false
	}
	return content, true
}

// textContent 要素直下のテキストノードを連結して返します。
func textContent(n *html.Node) string {
	var sb strings.Builder
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		if c.Type == html.TextNode {
			sb.WriteString(c.Data)
		}
	}
	return sb.String()
}

// normalizeTitle 前後空白を除去し、連続する空白を1つに畳み、上限文字数で切り詰めます。
func normalizeTitle(s string) string {
	s = strings.Join(strings.Fields(s), " ")
	runes := []rune(s)
	if len(runes) > maxTitleRunes {
		return string(runes[:maxTitleRunes])
	}
	return s
}
