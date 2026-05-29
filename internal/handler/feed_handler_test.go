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
func (s *stubItems) MarkRead(_, _ string, _ bool) error      { return nil }
func (s *stubItems) MarkAllRead(_ string) error              { return nil }
func (s *stubItems) Star(_, _ string, _ bool) error          { return nil }
func (s *stubItems) ReadLater(_, _ string, _ bool) error     { return nil }
func (s *stubItems) SetTags(_, _ string, _ []string) error   { return nil }
func (s *stubItems) SetBoards(_, _ string, _ []string) error { return nil }
func (s *stubItems) SetNote(_, _, _ string) error            { return nil }
func (s *stubItems) AddHighlight(_, _, _ string) error       { return nil }

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
}
