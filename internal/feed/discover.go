package feed

import (
	"bytes"
	"fmt"
	"net/url"
	"strings"

	"golang.org/x/net/html"
)

// feedLinkTypes link要素のtype属性のうちフィードとみなす値です。
var feedLinkTypes = map[string]struct{}{
	"application/rss+xml":  {},
	"application/atom+xml": {},
	"application/rdf+xml":  {},
	"application/xml":      {},
	"text/xml":             {},
}

// Discover HTMLのlink要素からフィードのURLを抽出しbaseURLで絶対URL化して返します。
// relがalternateでtypeがフィードを示すlinkを対象にします。出現順を保ち重複を除きます。
func Discover(data []byte, baseURL string) ([]string, error) {
	base, err := url.Parse(baseURL)
	if err != nil {
		return nil, fmt.Errorf("failed to parse base url %q: %w", baseURL, err)
	}
	if base.Scheme == "" || base.Host == "" {
		return nil, fmt.Errorf("feed: base url %q must be absolute", baseURL)
	}

	doc, err := html.Parse(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("failed to parse html: %w", err)
	}

	var found []string
	seen := make(map[string]struct{})
	var walk func(n *html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.ElementNode && strings.EqualFold(n.Data, "link") {
			if href, ok := feedHref(n); ok {
				ref, perr := url.Parse(href)
				if perr == nil {
					abs := base.ResolveReference(ref).String()
					if _, dup := seen[abs]; !dup {
						seen[abs] = struct{}{}
						found = append(found, abs)
					}
				}
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(doc)
	return found, nil
}

// feedHref link要素がフィードを指すならhrefを返します。
// relにalternateを含みtypeがフィード種別でhrefが空でないことを条件にします。
func feedHref(n *html.Node) (string, bool) {
	var rel, typ, href string
	for _, attr := range n.Attr {
		switch strings.ToLower(attr.Key) {
		case "rel":
			rel = strings.ToLower(attr.Val)
		case "type":
			typ = strings.ToLower(strings.TrimSpace(attr.Val))
		case "href":
			href = strings.TrimSpace(attr.Val)
		}
	}
	if !strings.Contains(rel, "alternate") {
		return "", false
	}
	if _, ok := feedLinkTypes[typ]; !ok {
		return "", false
	}
	if href == "" {
		return "", false
	}
	return href, true
}
