package feed

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"strings"
	"time"

	"github.com/okamyuji/feedflow-go-htmx/internal/port"
)

// XMLParser encoding/xmlを用いたport.FeedParserの実装です。
// RSS2.0とAtomとRDFを判別してパースしport.ParsedFeedへ正規化します。
type XMLParser struct{}

// NewXMLParser XMLParserを構築します。状態を持たないため設定はありません。
func NewXMLParser() *XMLParser {
	return &XMLParser{}
}

// Parse バイト列の形式を判別し、対応するパーサでport.ParsedFeedを返します。
// 実在のフィードにはXML1.0で不正な制御文字が紛れ込むことがあるため、判別とデコードの前に除去します。
func (p *XMLParser) Parse(data []byte) (port.ParsedFeed, error) {
	data = sanitizeXMLChars(data)
	format, err := detectFormat(data)
	if err != nil {
		return port.ParsedFeed{}, fmt.Errorf("failed to detect feed format: %w", err)
	}
	switch format {
	case port.FormatRSS2:
		return parseRSS2(data)
	case port.FormatAtom:
		return parseAtom(data)
	case port.FormatRDF:
		return parseRDF(data)
	default:
		return port.ParsedFeed{}, fmt.Errorf("feed: unsupported format %q", format)
	}
}

// sanitizeXMLChars XML1.0のChar生成規則で許可されない文字を取り除きます。
// 許可されるのはタブ(0x09)と改行(0x0A)と復帰(0x0D)、および0x20以上の通常文字
// (サロゲートと0xFFFE/0xFFFFを除く)です。encoding/xmlはStrict=falseでも不正文字を
// 拒否するため、現実のフィードを購読できるよう事前に除去します。
// 不正文字が無い場合は元のバイト列を新規確保せずに返します。
func sanitizeXMLChars(data []byte) []byte {
	cleaned := strings.Map(func(r rune) rune {
		if isValidXMLChar(r) {
			return r
		}
		return -1
	}, string(data))
	return []byte(cleaned)
}

// isValidXMLChar rがXML1.0のChar生成規則に適合するかを返します。
func isValidXMLChar(r rune) bool {
	switch {
	case r == 0x09 || r == 0x0A || r == 0x0D:
		return true
	case r >= 0x20 && r <= 0xD7FF:
		return true
	case r >= 0xE000 && r <= 0xFFFD:
		return true
	case r >= 0x10000 && r <= 0x10FFFF:
		return true
	default:
		return false
	}
}

// rss2Document RSS2.0のデコード用構造体です。
type rss2Document struct {
	XMLName xml.Name    `xml:"rss"`
	Channel rss2Channel `xml:"channel"`
}

type rss2Channel struct {
	Title string     `xml:"title"`
	Link  string     `xml:"link"`
	Items []rss2Item `xml:"item"`
}

type rss2Item struct {
	Title       string `xml:"title"`
	Link        string `xml:"link"`
	GUID        string `xml:"guid"`
	Description string `xml:"description"`
	Encoded     string `xml:"http://purl.org/rss/1.0/modules/content/ encoded"`
	Author      string `xml:"author"`
	Creator     string `xml:"http://purl.org/dc/elements/1.1/ creator"`
	PubDate     string `xml:"pubDate"`
	DCDate      string `xml:"http://purl.org/dc/elements/1.1/ date"`
}

// parseRSS2 RSS 2.0のバイト列をport.ParsedFeedへ正規化します。
func parseRSS2(data []byte) (port.ParsedFeed, error) {
	var doc rss2Document
	dec := xml.NewDecoder(bytes.NewReader(data))
	dec.Strict = false
	if err := dec.Decode(&doc); err != nil {
		return port.ParsedFeed{}, fmt.Errorf("failed to decode rss2: %w", err)
	}
	feed := port.ParsedFeed{
		Format:  port.FormatRSS2,
		Title:   strings.TrimSpace(doc.Channel.Title),
		SiteURL: strings.TrimSpace(doc.Channel.Link),
		Items:   make([]port.ParsedItem, 0, len(doc.Channel.Items)),
	}
	for _, it := range doc.Channel.Items {
		link := strings.TrimSpace(it.Link)
		guid := strings.TrimSpace(it.GUID)
		if guid == "" {
			guid = link
		}
		author := strings.TrimSpace(it.Author)
		if author == "" {
			author = strings.TrimSpace(it.Creator)
		}
		content := strings.TrimSpace(it.Encoded)
		if content == "" {
			content = strings.TrimSpace(it.Description)
		}
		dateStr := it.PubDate
		if strings.TrimSpace(dateStr) == "" {
			dateStr = it.DCDate
		}
		feed.Items = append(feed.Items, port.ParsedItem{
			GUID:        guid,
			Title:       strings.TrimSpace(it.Title),
			Link:        link,
			Content:     content,
			Summary:     strings.TrimSpace(it.Description),
			Author:      author,
			PublishedAt: parseTime(dateStr),
		})
	}
	return feed, nil
}

// timeLayouts 公開日時の解析で順に試すレイアウトです。
var timeLayouts = []string{
	time.RFC1123Z,
	time.RFC1123,
	time.RFC822Z,
	time.RFC822,
	time.RFC3339,
	"Mon, 2 Jan 2006 15:04:05 -0700",
	"2006-01-02T15:04:05Z07:00",
	"2006-01-02 15:04:05",
	"2006-01-02",
}

// parseTime 与えられた日時文字列を既知のレイアウトで順に解析します。
// いずれにも一致しない場合はゼロ値のtime.Timeを返します。
func parseTime(s string) time.Time {
	s = strings.TrimSpace(s)
	if s == "" {
		return time.Time{}
	}
	for _, layout := range timeLayouts {
		if t, err := time.Parse(layout, s); err == nil {
			return t
		}
	}
	return time.Time{}
}

// atomDocument Atomのデコード用構造体です。
type atomDocument struct {
	XMLName xml.Name    `xml:"feed"`
	Title   string      `xml:"title"`
	Links   []atomLink  `xml:"link"`
	Entries []atomEntry `xml:"entry"`
}

type atomLink struct {
	Href string `xml:"href,attr"`
	Rel  string `xml:"rel,attr"`
}

type atomEntry struct {
	Title     string     `xml:"title"`
	ID        string     `xml:"id"`
	Links     []atomLink `xml:"link"`
	Summary   string     `xml:"summary"`
	Content   string     `xml:"content"`
	Author    atomAuthor `xml:"author"`
	Published string     `xml:"published"`
	Updated   string     `xml:"updated"`
}

type atomAuthor struct {
	Name string `xml:"name"`
}

// pickAtomLink relがalternateまたは未指定のhrefを優先して返します。
// 該当が無い場合は最初のhrefを返します。
func pickAtomLink(links []atomLink) string {
	for _, l := range links {
		rel := strings.ToLower(strings.TrimSpace(l.Rel))
		if rel == "alternate" || rel == "" {
			if strings.TrimSpace(l.Href) != "" {
				return strings.TrimSpace(l.Href)
			}
		}
	}
	for _, l := range links {
		if strings.TrimSpace(l.Href) != "" {
			return strings.TrimSpace(l.Href)
		}
	}
	return ""
}

// parseAtom Atomのバイト列をport.ParsedFeedへ正規化します。
func parseAtom(data []byte) (port.ParsedFeed, error) {
	var doc atomDocument
	dec := xml.NewDecoder(bytes.NewReader(data))
	dec.Strict = false
	if err := dec.Decode(&doc); err != nil {
		return port.ParsedFeed{}, fmt.Errorf("failed to decode atom: %w", err)
	}
	feed := port.ParsedFeed{
		Format:  port.FormatAtom,
		Title:   strings.TrimSpace(doc.Title),
		SiteURL: pickAtomLink(doc.Links),
		Items:   make([]port.ParsedItem, 0, len(doc.Entries)),
	}
	for _, e := range doc.Entries {
		summary := strings.TrimSpace(e.Summary)
		content := strings.TrimSpace(e.Content)
		if content == "" {
			content = summary
		}
		dateStr := e.Updated
		if strings.TrimSpace(dateStr) == "" {
			dateStr = e.Published
		}
		feed.Items = append(feed.Items, port.ParsedItem{
			GUID:        strings.TrimSpace(e.ID),
			Title:       strings.TrimSpace(e.Title),
			Link:        pickAtomLink(e.Links),
			Content:     content,
			Summary:     summary,
			Author:      strings.TrimSpace(e.Author.Name),
			PublishedAt: parseTime(dateStr),
		})
	}
	return feed, nil
}

// rdfDocument RDFつまりRSS1.0のデコード用構造体です。
type rdfDocument struct {
	XMLName xml.Name   `xml:"RDF"`
	Channel rdfChannel `xml:"channel"`
	Items   []rdfItem  `xml:"item"`
}

type rdfChannel struct {
	Title string `xml:"title"`
	Link  string `xml:"link"`
}

type rdfItem struct {
	About       string `xml:"http://www.w3.org/1999/02/22-rdf-syntax-ns# about,attr"`
	Title       string `xml:"title"`
	Link        string `xml:"link"`
	Description string `xml:"description"`
	Creator     string `xml:"http://purl.org/dc/elements/1.1/ creator"`
	Date        string `xml:"http://purl.org/dc/elements/1.1/ date"`
}

// parseRDF RDFつまりRSS1.0のバイト列をport.ParsedFeedへ正規化します。
func parseRDF(data []byte) (port.ParsedFeed, error) {
	var doc rdfDocument
	dec := xml.NewDecoder(bytes.NewReader(data))
	dec.Strict = false
	if err := dec.Decode(&doc); err != nil {
		return port.ParsedFeed{}, fmt.Errorf("failed to decode rdf: %w", err)
	}
	feed := port.ParsedFeed{
		Format:  port.FormatRDF,
		Title:   strings.TrimSpace(doc.Channel.Title),
		SiteURL: strings.TrimSpace(doc.Channel.Link),
		Items:   make([]port.ParsedItem, 0, len(doc.Items)),
	}
	for _, it := range doc.Items {
		link := strings.TrimSpace(it.Link)
		guid := strings.TrimSpace(it.About)
		if guid == "" {
			guid = link
		}
		feed.Items = append(feed.Items, port.ParsedItem{
			GUID:        guid,
			Title:       strings.TrimSpace(it.Title),
			Link:        link,
			Content:     strings.TrimSpace(it.Description),
			Summary:     strings.TrimSpace(it.Description),
			Author:      strings.TrimSpace(it.Creator),
			PublishedAt: parseTime(it.Date),
		})
	}
	return feed, nil
}
