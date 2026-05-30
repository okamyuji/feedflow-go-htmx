package handler

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/okamyuji/feedflow-go-htmx/internal/domain"
)

// stubSubscriptions SubscriptionServiceのスタブです。
type stubSubscriptions struct {
	feeds         []domain.Feed
	subscribed    domain.Feed
	subscribeErr  error
	unsubscribeID string
}

func (s *stubSubscriptions) Subscribe(_ context.Context, feedURL string, _ []string) (domain.Feed, error) {
	if s.subscribeErr != nil {
		return domain.Feed{}, s.subscribeErr
	}
	f := domain.Feed{ID: "new", FeedURL: feedURL, Title: "新規フィード"}
	s.feeds = append(s.feeds, f)
	s.subscribed = f
	return f, nil
}

func (s *stubSubscriptions) SubscribeFromSite(_ context.Context, siteURL string, _ []string) (domain.Feed, error) {
	if s.subscribeErr != nil {
		return domain.Feed{}, s.subscribeErr
	}
	f := domain.Feed{ID: "new", SiteURL: siteURL, Title: "検出フィード"}
	s.feeds = append(s.feeds, f)
	s.subscribed = f
	return f, nil
}

func (s *stubSubscriptions) Unsubscribe(feedID string) error {
	s.unsubscribeID = feedID
	return nil
}
func (s *stubSubscriptions) ListFeeds() ([]domain.Feed, error)            { return s.feeds, nil }
func (s *stubSubscriptions) Reorder(_ []string) error                     { return nil }
func (s *stubSubscriptions) SetFeedCategories(_ string, _ []string) error { return nil }

// stubItems ItemServiceの最小スタブです。ツリー描画の未読集計に使います。
type stubItems struct {
	items map[string][]domain.Item
}

func (s *stubItems) ListItems(feedID string) ([]domain.Item, error) {
	if feedID == "" {
		var all []domain.Item
		for _, v := range s.items {
			all = append(all, v...)
		}
		return all, nil
	}
	return s.items[feedID], nil
}
func (s *stubItems) MarkRead(_, _ string, _ bool) error         { return nil }
func (s *stubItems) MarkAllRead(_ string) error                 { return nil }
func (s *stubItems) ReadLater(_, _ string, _ bool) error        { return nil }
func (s *stubItems) SetTags(_, _ string, _ []string) error      { return nil }
func (s *stubItems) SetBookmarks(_, _ string, _ []string) error { return nil }
func (s *stubItems) SetNote(_, _, _ string) error               { return nil }
func (s *stubItems) AddHighlight(_, _, _ string) error          { return nil }

// stubBookmarks BookmarkServiceの最小スタブです。一覧と作成と所属操作を記録します。
// IDはテスト簡素化のため名称をそのまま使います。
type stubBookmarks struct {
	list       []domain.Bookmark
	created    string
	toggled    string
	lastFeedID string
	lastItemID string
}

func (s *stubBookmarks) List() ([]domain.Bookmark, error) { return s.list, nil }
func (s *stubBookmarks) Create(name string) (domain.Bookmark, error) {
	for _, b := range s.list {
		if b.Name == name {
			return b, nil
		}
	}
	bm := domain.Bookmark{ID: name, Name: name}
	s.list = append(s.list, bm)
	s.created = name
	return bm, nil
}
func (s *stubBookmarks) Toggle(feedID, itemID, bookmarkID string) error {
	s.toggled = bookmarkID
	s.lastFeedID = feedID
	s.lastItemID = itemID
	return nil
}
func (s *stubBookmarks) CreateAndAdd(feedID, itemID, name string) (domain.Bookmark, error) {
	bm, _ := s.Create(name)
	s.lastFeedID = feedID
	s.lastItemID = itemID
	return bm, nil
}

// stubMutes MuteServiceの最小スタブです。フィルタなしで素通しします。
type stubMutes struct {
	filters []domain.MuteFilter
}

func (s *stubMutes) ListFilters() ([]domain.MuteFilter, error) { return s.filters, nil }
func (s *stubMutes) AddFilter(keyword string, scope domain.MuteScope, feedID string) (domain.MuteFilter, error) {
	f := domain.MuteFilter{ID: "mf", Keyword: keyword, Scope: scope, FeedID: feedID}
	s.filters = append(s.filters, f)
	return f, nil
}
func (s *stubMutes) DeleteFilter(_ string) error                       { return nil }
func (s *stubMutes) Filter(items []domain.Item) ([]domain.Item, error) { return items, nil }

func newAppHandler(t *testing.T, subs *stubSubscriptions, items *stubItems) *Handler {
	t.Helper()
	h, err := New(Deps{
		Subscriptions:     subs,
		Items:             items,
		Bookmarks:         &stubBookmarks{},
		Mutes:             &stubMutes{},
		Sessions:          &stubSessions{username: "owner", ok: true},
		CSRF:              &stubCSRF{ok: true, token: "tok"},
		SessionCookieName: "feedflow_session",
	})
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}
	return h
}

func TestFeedSubscribeWithFeedURL(t *testing.T) {
	t.Parallel()
	subs := &stubSubscriptions{}
	h := newAppHandler(t, subs, &stubItems{items: map[string][]domain.Item{}})
	form := url.Values{"url": {"https://example.com/feed.xml"}}
	req := httptest.NewRequest(http.MethodPost, "/app/feeds", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req = withSession(req, Session{Username: "owner", CSRFToken: "tok"})
	rec := httptest.NewRecorder()

	h.feedSubscribe(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status got %d want %d", rec.Code, http.StatusOK)
	}
	if subs.subscribed.FeedURL != "https://example.com/feed.xml" {
		t.Fatalf("subscribed FeedURL got %q", subs.subscribed.FeedURL)
	}
	if !strings.Contains(rec.Body.String(), "tree") {
		t.Fatalf("body should render tree partial: %q", rec.Body.String())
	}
}

func TestFeedSubscribeWithSiteURLFallback(t *testing.T) {
	t.Parallel()
	subs := &stubSubscriptions{}
	h := newAppHandler(t, subs, &stubItems{items: map[string][]domain.Item{}})
	form := url.Values{"url": {"https://example.com/"}, "from_site": {"true"}}
	req := httptest.NewRequest(http.MethodPost, "/app/feeds", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req = withSession(req, Session{Username: "owner", CSRFToken: "tok"})
	rec := httptest.NewRecorder()

	h.feedSubscribe(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status got %d want %d", rec.Code, http.StatusOK)
	}
	if subs.subscribed.SiteURL != "https://example.com/" {
		t.Fatalf("subscribed SiteURL got %q", subs.subscribed.SiteURL)
	}
}

func TestFeedUnsubscribe(t *testing.T) {
	t.Parallel()
	subs := &stubSubscriptions{feeds: []domain.Feed{{ID: "f1", Title: "f1"}}}
	h := newAppHandler(t, subs, &stubItems{items: map[string][]domain.Item{}})
	req := httptest.NewRequest(http.MethodDelete, "/app/feeds/f1", nil)
	req.SetPathValue("feedID", "f1")
	req = withSession(req, Session{Username: "owner", CSRFToken: "tok"})
	rec := httptest.NewRecorder()

	h.feedUnsubscribe(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status got %d want %d", rec.Code, http.StatusOK)
	}
	if subs.unsubscribeID != "f1" {
		t.Fatalf("unsubscribed id got %q want %q", subs.unsubscribeID, "f1")
	}
	if !strings.Contains(rec.Body.String(), `id="tree-pane"`) {
		t.Fatalf("body should render the tree pane wrapper: %q", rec.Body.String())
	}
}

func TestTreeRendersUnsubscribeButtonForFeeds(t *testing.T) {
	t.Parallel()
	subs := &stubSubscriptions{feeds: []domain.Feed{{ID: "f1", Title: "f1"}}}
	h := newAppHandler(t, subs, &stubItems{items: map[string][]domain.Item{}})
	req := httptest.NewRequest(http.MethodDelete, "/app/feeds/missing", nil)
	req.SetPathValue("feedID", "missing")
	req = withSession(req, Session{Username: "owner", CSRFToken: "tok"})
	rec := httptest.NewRecorder()

	h.feedUnsubscribe(rec, req)

	body := rec.Body.String()
	if !strings.Contains(body, `hx-delete="/app/feeds/f1"`) {
		t.Fatalf("feed node should expose an unsubscribe control: %q", body)
	}
	if !strings.Contains(body, "tree-unsubscribe") {
		t.Fatalf("feed node should render the unsubscribe button: %q", body)
	}
}

func TestBuildTreeCountsUnreadStream(t *testing.T) {
	t.Parallel()
	subs := &stubSubscriptions{feeds: []domain.Feed{{ID: "f1", Title: "f1"}}}
	items := &stubItems{items: map[string][]domain.Item{
		"f1": {
			{ID: "i1", FeedID: "f1"},
			{ID: "i2", FeedID: "f1", Read: true},
			{ID: "i3", FeedID: "f1"},
		},
	}}
	h := newAppHandler(t, subs, items)

	nodes, err := h.buildTree()
	if err != nil {
		t.Fatalf("buildTree returned error: %v", err)
	}
	var all, read, feed feedTreeNode
	hasRead := false
	for _, n := range nodes {
		switch n.Kind {
		case "all":
			all = n
		case "read":
			read = n
			hasRead = true
		case "feed":
			feed = n
		}
	}
	if all.Label != "すべて" || all.UnreadCount != 2 {
		t.Fatalf("all node should be the unread stream with 2 unread, got label=%q count=%d", all.Label, all.UnreadCount)
	}
	if !hasRead || read.Label != "既読" {
		t.Fatalf("tree should contain a 既読 node, got %+v", read)
	}
	if read.UnreadCount != 0 {
		t.Fatalf("既読 node should not carry an unread badge, got %d", read.UnreadCount)
	}
	if feed.UnreadCount != 2 {
		t.Fatalf("feed node unread count got %d want 2", feed.UnreadCount)
	}
}
