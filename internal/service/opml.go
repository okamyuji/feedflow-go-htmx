package service

import (
	"context"
	"encoding/xml"
	"errors"
	"fmt"

	"github.com/okamyuji/feedflow-go-htmx/internal/port"
)

// opmlDocument OPML文書全体のXMLマッピングです。
type opmlDocument struct {
	XMLName xml.Name `xml:"opml"`
	Version string   `xml:"version,attr"`
	Head    opmlHead `xml:"head"`
	Body    opmlBody `xml:"body"`
}

// opmlHead OPMLのヘッダ部です。
type opmlHead struct {
	Title string `xml:"title"`
}

// opmlBody OPMLの本体部です。outlineを入れ子で持ちます。
type opmlBody struct {
	Outlines []opmlOutline `xml:"outline"`
}

// opmlOutline OPMLのoutline要素です。カテゴリ表現として入れ子になりえます。
type opmlOutline struct {
	Text     string        `xml:"text,attr"`
	Title    string        `xml:"title,attr"`
	Type     string        `xml:"type,attr"`
	XMLURL   string        `xml:"xmlUrl,attr"`
	HTMLURL  string        `xml:"htmlUrl,attr"`
	Outlines []opmlOutline `xml:"outline"`
}

// OPMLService OPMLの入出力を担います。購読追加はSubscriptionServiceに委譲します。
// port.OPMLService を満たします。
type OPMLService struct {
	deps Deps
	subs port.SubscriptionService
}

// NewOPMLService 依存束と購読サービスを受け取りOPMLServiceを構築します。
func NewOPMLService(deps Deps, subs port.SubscriptionService) *OPMLService {
	return &OPMLService{deps: deps, subs: subs}
}

// Import OPMLのバイト列を読み込み、各outlineのxmlUrlを購読に追加します。
// 新規に購読したフィード数を返します。すでに購読済みのURLはスキップしてカウントしません。
func (s *OPMLService) Import(ctx context.Context, data []byte) (int, error) {
	var doc opmlDocument
	if err := xml.Unmarshal(data, &doc); err != nil {
		return 0, fmt.Errorf("failed to parse opml: %w", err)
	}
	urls := collectFeedURLs(doc.Body.Outlines)
	count := 0
	for _, u := range urls {
		_, err := s.subs.Subscribe(ctx, u, nil)
		if err != nil {
			if errors.Is(err, ErrDuplicateFeed) {
				continue
			}
			return count, fmt.Errorf("failed to subscribe %s during opml import: %w", u, err)
		}
		count++
	}
	return count, nil
}

// collectFeedURLs outlineを再帰的に走査し、空でないxmlUrlを順序を保って集めます。
func collectFeedURLs(outlines []opmlOutline) []string {
	var urls []string
	for _, o := range outlines {
		if o.XMLURL != "" {
			urls = append(urls, o.XMLURL)
		}
		urls = append(urls, collectFeedURLs(o.Outlines)...)
	}
	return urls
}

// Export 現在の購読をOPMLのバイト列として返します。
func (s *OPMLService) Export() ([]byte, error) {
	feeds, err := s.deps.Repo.Feeds()
	if err != nil {
		return nil, fmt.Errorf("failed to load feeds: %w", err)
	}
	outlines := make([]opmlOutline, 0, len(feeds))
	for _, f := range feeds {
		outlines = append(outlines, opmlOutline{
			Text:    f.Title,
			Title:   f.Title,
			Type:    "rss",
			XMLURL:  f.FeedURL,
			HTMLURL: f.SiteURL,
		})
	}
	doc := opmlDocument{
		Version: "2.0",
		Head:    opmlHead{Title: "feedflow subscriptions"},
		Body:    opmlBody{Outlines: outlines},
	}
	body, err := xml.MarshalIndent(doc, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("failed to marshal opml: %w", err)
	}
	out := append([]byte(xml.Header), body...)
	return out, nil
}
