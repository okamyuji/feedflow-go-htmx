package service

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net/url"
	"strings"

	"golang.org/x/net/html"

	"github.com/okamyuji/feedflow-go-htmx/internal/domain"
	"github.com/okamyuji/feedflow-go-htmx/internal/port"
)

// ErrDuplicateFeed 同じフィードURLがすでに購読済みのときに返すエラーです。
var ErrDuplicateFeed = errors.New("feed url already subscribed")

// ErrFeedNotDiscovered サイトURLからフィードを検出できなかったときに返すエラーです。
var ErrFeedNotDiscovered = errors.New("no feed link discovered on site")

// ErrNotUnsubscribable 購読解除できないフィードを指定されたときに返すエラーです。
// 任意URLの保存に使う合成フィードが対象です。
var ErrNotUnsubscribable = errors.New("feed cannot be unsubscribed")

// ErrFeedNotEditable 取得元URLを変更できないフィードを指定されたときに返すエラーです。
// 合成フィードは取得元URLを持たないため対象外です。
var ErrFeedNotEditable = errors.New("feed url cannot be edited")

// errNotFoundCategory Reorderで未知のカテゴリIDを受け取ったときの基底エラーです。
var errNotFoundCategory = errors.New("category not found")

// SubscriptionService 購読の追加と削除と一覧と整理を担います。port.SubscriptionServiceを満たします。
type SubscriptionService struct {
	deps Deps
}

// NewSubscriptionService 依存束を受け取りSubscriptionServiceを構築します。
func NewSubscriptionService(deps Deps) *SubscriptionService {
	return &SubscriptionService{deps: deps}
}

// ListFeeds 購読中の全フィードを返します。
func (s *SubscriptionService) ListFeeds() ([]domain.Feed, error) {
	feeds, err := s.deps.Repo.Feeds()
	if err != nil {
		return nil, fmt.Errorf("failed to load feeds: %w", err)
	}
	return feeds, nil
}

// Subscribe フィードURLを取得しパースして購読に追加します。追加後のフィードを返します。
// 既存の購読URLと重複する場合は追加せずエラーを返します。
func (s *SubscriptionService) Subscribe(ctx context.Context, feedURL string, categoryIDs []string) (domain.Feed, error) {
	duplicate, err := s.feedURLExists(feedURL)
	if err != nil {
		return domain.Feed{}, err
	}
	if duplicate {
		return domain.Feed{}, ErrDuplicateFeed
	}

	res, err := s.deps.Fetch.Fetch(ctx, port.FetchRequest{URL: feedURL})
	if err != nil {
		return domain.Feed{}, fmt.Errorf("failed to fetch feed %s: %w", feedURL, err)
	}
	parsed, err := s.deps.Parse.Parse(res.Body)
	if err != nil {
		return domain.Feed{}, fmt.Errorf("failed to parse feed %s: %w", feedURL, err)
	}

	return s.createFeed(feedURL, categoryIDs, res, parsed)
}

// createFeed パース結果からフィードと記事を構築して保存し、保存したフィードを返します。
func (s *SubscriptionService) createFeed(feedURL string, categoryIDs []string, res port.FetchResult, parsed port.ParsedFeed) (domain.Feed, error) {
	now := s.deps.Clock.Now()
	cats := make([]string, len(categoryIDs))
	copy(cats, categoryIDs)

	feed := domain.Feed{
		ID:            s.deps.IDs.NewID(),
		FeedURL:       feedURL,
		SiteURL:       parsed.SiteURL,
		Title:         parsed.Title,
		CategoryIDs:   cats,
		PollInterval:  domain.PollDefault,
		ETag:          res.ETag,
		LastModified:  res.LastModified,
		LastFetchedAt: now,
	}
	if err := s.deps.Repo.SaveFeed(feed); err != nil {
		return domain.Feed{}, fmt.Errorf("failed to save feed: %w", err)
	}

	items := make([]domain.Item, 0, len(parsed.Items))
	for _, p := range parsed.Items {
		items = append(items, domain.Item{
			ID:          s.deps.IDs.NewID(),
			FeedID:      feed.ID,
			GUID:        p.GUID,
			Title:       p.Title,
			Link:        p.Link,
			Content:     p.Content,
			Summary:     p.Summary,
			Author:      p.Author,
			PublishedAt: p.PublishedAt,
			FetchedAt:   now,
		})
	}
	if err := s.deps.Repo.SaveItems(feed.ID, items); err != nil {
		return domain.Feed{}, fmt.Errorf("failed to save items for feed %s: %w", feed.ID, err)
	}
	return feed, nil
}

// feedURLExists 指定URLがすでに購読済みかどうかを返します。
func (s *SubscriptionService) feedURLExists(feedURL string) (bool, error) {
	feeds, err := s.deps.Repo.Feeds()
	if err != nil {
		return false, fmt.Errorf("failed to load feeds: %w", err)
	}
	for _, f := range feeds {
		if f.FeedURL == feedURL {
			return true, nil
		}
	}
	return false, nil
}

// Unsubscribe 指定フィードの購読を解除し、属する記事も削除します。
// 合成フィードは購読フィードではないため拒否します。
// 左ツリーに解除ボタンを出していなくてもルート自体は到達可能で、
// 通してしまうと保存したページが記事ごと一括で失われるためです。
func (s *SubscriptionService) Unsubscribe(feedID string) error {
	if domain.IsSavedPagesFeed(feedID) {
		return ErrNotUnsubscribable
	}
	if err := s.deps.Repo.DeleteFeed(feedID); err != nil {
		return fmt.Errorf("failed to delete feed %s: %w", feedID, err)
	}
	return nil
}

// UpdateFeedURL 指定フィードの取得元URLを差し替え、更新後のフィードを返します。
// rawURLはフィードURLでもサイトURLでもよく、フィードとして解析できなければHTMLからフィードを検出します。
// 取得と解析に成功したときだけ保存し、IDと記事とカテゴリを保ったままエラー状態を解除します。
// 削除して購読し直すと既読やブックマークが失われるため、URLだけを入れ替えます。
// 検証で取得した記事は取り込まず、ETagとLast-Modifiedを空にして次の取得で全件を取り込ませます。
func (s *SubscriptionService) UpdateFeedURL(ctx context.Context, feedID, rawURL string) (domain.Feed, error) {
	if domain.IsSavedPagesFeed(feedID) {
		return domain.Feed{}, ErrFeedNotEditable
	}
	// 末尾スラッシュの有無で別のページを返すサーバがあるため、ブックマーク用の正規化はせず入力どおりに取得します。
	u, err := parseHTTPURL(rawURL)
	if err != nil {
		return domain.Feed{}, err
	}
	feed, err := s.deps.Repo.Feed(feedID)
	if err != nil {
		return domain.Feed{}, fmt.Errorf("failed to load feed %s: %w", feedID, err)
	}
	feedURL, parsed, err := s.fetchFeedOrDiscover(ctx, u.String())
	if err != nil {
		return domain.Feed{}, err
	}
	feeds, err := s.deps.Repo.Feeds()
	if err != nil {
		return domain.Feed{}, fmt.Errorf("failed to load feeds: %w", err)
	}
	for _, f := range feeds {
		if f.ID != feedID && f.FeedURL == feedURL {
			return domain.Feed{}, ErrDuplicateFeed
		}
	}

	updated := feed
	updated.FeedURL = feedURL
	if parsed.Title != "" {
		updated.Title = parsed.Title
	}
	if parsed.SiteURL != "" {
		updated.SiteURL = parsed.SiteURL
	}
	updated.ETag = ""
	updated.LastModified = ""
	updated.LastFetchedAt = s.deps.Clock.Now()
	updated.ConsecutiveErrors = 0
	updated.LastError = ""
	if err := s.deps.Repo.SaveFeed(updated); err != nil {
		return domain.Feed{}, fmt.Errorf("failed to save feed %s: %w", feedID, err)
	}
	return updated, nil
}

// fetchFeedOrDiscover URLをフィードとして取得し、解析できなければHTMLから検出したフィードURLで取り直します。
func (s *SubscriptionService) fetchFeedOrDiscover(ctx context.Context, rawURL string) (string, port.ParsedFeed, error) {
	res, err := s.deps.Fetch.Fetch(ctx, port.FetchRequest{URL: rawURL})
	if err != nil {
		return "", port.ParsedFeed{}, fmt.Errorf("failed to fetch %s: %w", rawURL, err)
	}
	if parsed, parseErr := s.deps.Parse.Parse(res.Body); parseErr == nil {
		return rawURL, parsed, nil
	}
	feedURL, err := discoverFeedURL(rawURL, res.Body)
	if err != nil {
		return "", port.ParsedFeed{}, err
	}
	res, err = s.deps.Fetch.Fetch(ctx, port.FetchRequest{URL: feedURL})
	if err != nil {
		return "", port.ParsedFeed{}, fmt.Errorf("failed to fetch feed %s: %w", feedURL, err)
	}
	parsed, err := s.deps.Parse.Parse(res.Body)
	if err != nil {
		return "", port.ParsedFeed{}, fmt.Errorf("failed to parse feed %s: %w", feedURL, err)
	}
	return feedURL, parsed, nil
}

// SubscribeFromSite サイトURLを取得しHTMLからフィードリンクを検出して購読に追加します。
// 検出した最初のRSSまたはAtomのフィードURLでSubscribeを呼びます。
func (s *SubscriptionService) SubscribeFromSite(ctx context.Context, siteURL string, categoryIDs []string) (domain.Feed, error) {
	res, err := s.deps.Fetch.Fetch(ctx, port.FetchRequest{URL: siteURL})
	if err != nil {
		return domain.Feed{}, fmt.Errorf("failed to fetch site %s: %w", siteURL, err)
	}
	feedURL, err := discoverFeedURL(siteURL, res.Body)
	if err != nil {
		return domain.Feed{}, err
	}
	return s.Subscribe(ctx, feedURL, categoryIDs)
}

// Reorder カテゴリの並び順を指定したID順に更新します。
// 指定IDの先頭から0, 1, 2とOrderを振り直します。未知のIDがあればエラーを返します。
func (s *SubscriptionService) Reorder(categoryIDs []string) error {
	categories, err := s.deps.Repo.Categories()
	if err != nil {
		return fmt.Errorf("failed to load categories: %w", err)
	}
	byID := make(map[string]domain.Category, len(categories))
	for _, c := range categories {
		byID[c.ID] = c
	}
	for order, id := range categoryIDs {
		c, ok := byID[id]
		if !ok {
			return fmt.Errorf("unknown category id %s: %w", id, errNotFoundCategory)
		}
		c.Order = order
		if err := s.deps.Repo.SaveCategory(c); err != nil {
			return fmt.Errorf("failed to save category %s: %w", id, err)
		}
	}
	return nil
}

// SetFeedCategories 指定フィードの所属カテゴリを与えた内容で置き換えます。
func (s *SubscriptionService) SetFeedCategories(feedID string, categoryIDs []string) error {
	feed, err := s.deps.Repo.Feed(feedID)
	if err != nil {
		return fmt.Errorf("failed to load feed %s: %w", feedID, err)
	}
	next := make([]string, len(categoryIDs))
	copy(next, categoryIDs)
	feed.CategoryIDs = next
	if err := s.deps.Repo.SaveFeed(feed); err != nil {
		return fmt.Errorf("failed to save feed %s: %w", feedID, err)
	}
	return nil
}

// discoverFeedURL HTMLのバイト列から最初のフィードリンクの絶対URLを返します。
// link 要素のrelがalternateでtypeがRSSまたはAtomのhrefを対象にします。
// href が相対URLの場合は基準URLで絶対化します。
func discoverFeedURL(baseURL string, htmlBytes []byte) (string, error) {
	base, err := url.Parse(baseURL)
	if err != nil {
		return "", fmt.Errorf("failed to parse base url %s: %w", baseURL, err)
	}
	doc, err := html.Parse(bytes.NewReader(htmlBytes))
	if err != nil {
		return "", fmt.Errorf("failed to parse site html: %w", err)
	}
	href := findFeedHref(doc)
	if href == "" {
		return "", ErrFeedNotDiscovered
	}
	ref, err := url.Parse(href)
	if err != nil {
		return "", fmt.Errorf("failed to parse feed href %s: %w", href, err)
	}
	return base.ResolveReference(ref).String(), nil
}

// findFeedHref ノードを深さ優先で走査し、最初のフィードリンクのhrefを返します。
func findFeedHref(n *html.Node) string {
	if n.Type == html.ElementNode && n.Data == "link" {
		if href, ok := feedLinkHref(n); ok {
			return href
		}
	}
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		if href := findFeedHref(c); href != "" {
			return href
		}
	}
	return ""
}

// feedLinkHref link要素がフィードリンクならhrefを返します。
// type 属性がapplication/rss+xmlまたはapplication/atom+xmlのときに該当とみなします。
func feedLinkHref(n *html.Node) (string, bool) {
	var typ, href string
	for _, a := range n.Attr {
		switch strings.ToLower(a.Key) {
		case "type":
			typ = strings.ToLower(strings.TrimSpace(a.Val))
		case "href":
			href = strings.TrimSpace(a.Val)
		}
	}
	if href == "" {
		return "", false
	}
	if typ == "application/rss+xml" || typ == "application/atom+xml" {
		return href, true
	}
	return "", false
}
