package handler

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/okamyuji/feedflow-go-htmx/internal/domain"
	"github.com/okamyuji/feedflow-go-htmx/internal/service"
)

// stubSubscriptions SubscriptionServiceのスタブです。
type stubSubscriptions struct {
	feeds          []domain.Feed
	subscribed     domain.Feed
	subscribeErr   error
	unsubscribeErr error
	unsubscribeID  string
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
	if s.unsubscribeErr != nil {
		return s.unsubscribeErr
	}
	s.unsubscribeID = feedID
	return nil
}
func (s *stubSubscriptions) ListFeeds() ([]domain.Feed, error)            { return s.feeds, nil }
func (s *stubSubscriptions) Reorder(_ []string) error                     { return nil }
func (s *stubSubscriptions) SetFeedCategories(_ string, _ []string) error { return nil }

// stubItems ItemServiceの最小スタブです。ツリー描画の未読集計に使います。
type stubItems struct {
	items         map[string][]domain.Item
	deletedFeedID string
	deletedItemID string
	deleteErr     error
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
func (s *stubItems) MarkRead(feedID, itemID string, read bool) error {
	items := s.items[feedID]
	for i, it := range items {
		if it.ID == itemID {
			items[i].Read = read
			s.items[feedID] = items
			return nil
		}
	}
	return nil
}
func (s *stubItems) MarkAllRead(_ string) error                 { return nil }
func (s *stubItems) ReadLater(_, _ string, _ bool) error        { return nil }
func (s *stubItems) SetTags(_, _ string, _ []string) error      { return nil }
func (s *stubItems) SetBookmarks(_, _ string, _ []string) error { return nil }
func (s *stubItems) SetBookmarked(_, _ string, _ bool) error    { return nil }
func (s *stubItems) SetNote(_, _, _ string) error               { return nil }
func (s *stubItems) AddHighlight(_, _, _ string) error          { return nil }

// DeleteItem 合成フィードから指定記事を取り除きます。呼び出しの記録も残します。
func (s *stubItems) DeleteItem(feedID, itemID string) error {
	if !domain.IsSavedPagesFeed(feedID) {
		return service.ErrNotSavedPagesFeed
	}
	if s.deleteErr != nil {
		return s.deleteErr
	}
	items := s.items[feedID]
	kept := make([]domain.Item, 0, len(items))
	found := false
	for _, it := range items {
		if it.ID == itemID {
			found = true
			s.deletedFeedID = feedID
			s.deletedItemID = itemID
			continue
		}
		kept = append(kept, it)
	}
	if !found {
		return service.ErrItemNotFound
	}
	s.items[feedID] = kept
	return nil
}

// stubBookmarks BookmarkServiceの最小スタブです。一覧と作成と所属操作を記録します。
// IDはテスト簡素化のため名称をそのまま使います。
type stubBookmarks struct {
	list        []domain.Bookmark
	created     string
	toggled     string
	lastFeedID  string
	lastItemID  string
	addedURL    string
	addedLabel  string
	addedItem   domain.Item
	addURLError error
	listErr     error
}

func (s *stubBookmarks) List() ([]domain.Bookmark, error) { return s.list, s.listErr }

// AddURL 入力を記録し、仕込まれた結果を返します。
func (s *stubBookmarks) AddURL(_ context.Context, rawURL, bookmarkID string) (domain.Item, error) {
	s.addedURL = rawURL
	s.addedLabel = bookmarkID
	if s.addURLError != nil {
		return domain.Item{}, s.addURLError
	}
	return s.addedItem, nil
}
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
func (s *stubBookmarks) Rename(id, name string) error {
	for i, b := range s.list {
		if b.ID == id {
			s.list[i].Name = name
			return nil
		}
	}
	return nil
}
func (s *stubBookmarks) Delete(id string) error {
	next := s.list[:0]
	for _, b := range s.list {
		if b.ID != id {
			next = append(next, b)
		}
	}
	s.list = next
	return nil
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

type stubPoll struct {
	polledFeedID       string
	polledAll          bool
	pollAllHadDeadline bool
	err                error
}

func (s *stubPoll) PollFeed(_ context.Context, feedID string) (int, error) {
	s.polledFeedID = feedID
	if s.err != nil {
		return 0, s.err
	}
	return 1, nil
}

func (s *stubPoll) PollAll(_ context.Context) (int, error) {
	return 0, nil
}

func (s *stubPoll) PollAllNow(ctx context.Context) (int, error) {
	s.polledAll = true
	_, s.pollAllHadDeadline = ctx.Deadline()
	if s.err != nil {
		return 0, s.err
	}
	return 1, nil
}

func newAppHandler(t *testing.T, subs *stubSubscriptions, items *stubItems) *Handler {
	t.Helper()
	return newAppHandlerWithSettings(t, subs, items, nil)
}

func newAppHandlerWithSettings(t *testing.T, subs *stubSubscriptions, items *stubItems, settings *stubSettings) *Handler {
	t.Helper()
	deps := Deps{
		Subscriptions:     subs,
		Items:             items,
		Bookmarks:         &stubBookmarks{},
		Mutes:             &stubMutes{},
		Poll:              &stubPoll{},
		Sessions:          &stubSessions{username: "owner", ok: true},
		CSRF:              &stubCSRF{ok: true, token: "tok"},
		SessionCookieName: "feedflow_session",
	}
	if settings != nil {
		deps.Settings = settings
	}
	h, err := New(deps)
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}
	return h
}

func TestFeedSubscribeWithFeedURL(t *testing.T) {
	t.Parallel()
	subs := &stubSubscriptions{}
	h := newAppHandler(t, subs, &stubItems{items: map[string][]domain.Item{
		"new": {{ID: "i1", FeedID: "new", Title: "購読直後の記事"}},
	}})
	form := url.Values{"url": {"https://example.com/feed.xml"}}
	req := httptest.NewRequest(http.MethodPost, "/app/feeds", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("HX-Request", "true")
	req = withSession(req, Session{Username: "owner", CSRFToken: "tok"})
	rec := httptest.NewRecorder()

	h.feedSubscribe(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status got %d want %d", rec.Code, http.StatusOK)
	}
	if subs.subscribed.FeedURL != "https://example.com/feed.xml" {
		t.Fatalf("subscribed FeedURL got %q", subs.subscribed.FeedURL)
	}
	body := rec.Body.String()
	if !strings.Contains(body, `class="item-list"`) || !strings.Contains(body, "購読直後の記事") {
		t.Fatalf("subscribe should render the refreshed item list as the primary response: %q", body)
	}
	if !strings.Contains(body, `id="tree-pane"`) || !strings.Contains(body, `hx-swap-oob="true"`) {
		t.Fatalf("subscribe should include a tree pane out-of-band refresh: %q", body)
	}
}

func TestFeedSubscribeWithSiteURLFallback(t *testing.T) {
	t.Parallel()
	subs := &stubSubscriptions{}
	h := newAppHandler(t, subs, &stubItems{items: map[string][]domain.Item{}})
	form := url.Values{"url": {"https://example.com/"}, "from_site": {"true"}}
	req := httptest.NewRequest(http.MethodPost, "/app/feeds", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("HX-Request", "true")
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

func TestTreeRendersPollButtonForFeeds(t *testing.T) {
	t.Parallel()
	subs := &stubSubscriptions{feeds: []domain.Feed{{ID: "f1", Title: "f1"}}}
	h := newAppHandler(t, subs, &stubItems{items: map[string][]domain.Item{}})
	req := httptest.NewRequest(http.MethodDelete, "/app/feeds/missing", nil)
	req.SetPathValue("feedID", "missing")
	req = withSession(req, Session{Username: "owner", CSRFToken: "tok"})
	rec := httptest.NewRecorder()

	h.feedUnsubscribe(rec, req)

	body := rec.Body.String()
	if !strings.Contains(body, `hx-post="/app/feeds/f1/poll?feed=f1"`) {
		t.Fatalf("feed node should expose a manual poll control: %q", body)
	}
	if !strings.Contains(body, `hx-push-url="/app/items?feed=f1"`) {
		t.Fatalf("feed poll should move the browser URL to the refreshed feed: %q", body)
	}
	if !strings.Contains(body, "tree-refresh") {
		t.Fatalf("feed node should render the manual poll button: %q", body)
	}
	if !strings.Contains(body, `hx-disabled-elt="this"`) {
		t.Fatalf("feed poll button should disable itself while loading: %q", body)
	}
}

func TestFeedPollRefreshesSelectedFeed(t *testing.T) {
	t.Parallel()
	poll := &stubPoll{}
	subs := &stubSubscriptions{feeds: []domain.Feed{{ID: "f1", Title: "f1"}}}
	items := &stubItems{items: map[string][]domain.Item{
		"f1": {{ID: "i1", FeedID: "f1", Title: "更新後の記事"}},
	}}
	h := newAppHandler(t, subs, items)
	h.deps.Poll = poll
	req := httptest.NewRequest(http.MethodPost, "/app/feeds/f1/poll?feed=f1", nil)
	req.SetPathValue("feedID", "f1")
	req.Header.Set("HX-Request", "true")
	req = withSession(req, Session{Username: "owner", CSRFToken: "tok"})
	rec := httptest.NewRecorder()

	h.feedPoll(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status got %d want %d", rec.Code, http.StatusOK)
	}
	if poll.polledFeedID != "f1" {
		t.Fatalf("polled feed got %q want %q", poll.polledFeedID, "f1")
	}
	body := rec.Body.String()
	if !strings.Contains(body, `class="item-list"`) || !strings.Contains(body, "更新後の記事") {
		t.Fatalf("poll should render the refreshed feed item list: %q", body)
	}
	if !strings.Contains(body, `id="tree-pane"`) || !strings.Contains(body, `hx-swap-oob="true"`) {
		t.Fatalf("poll should include a tree pane out-of-band refresh: %q", body)
	}
}

func TestItemListRendersManualPollButtonForCurrentView(t *testing.T) {
	t.Parallel()
	subs := &stubSubscriptions{feeds: []domain.Feed{{ID: "f1", Title: "f1"}}}
	h := newAppHandler(t, subs, &stubItems{items: map[string][]domain.Item{}})
	req := httptest.NewRequest(http.MethodGet, "/app/items?feed=f1", nil)
	req.Header.Set("HX-Request", "true")
	req = withSession(req, Session{Username: "owner", CSRFToken: "tok"})
	rec := httptest.NewRecorder()

	h.itemList(rec, req)

	body := rec.Body.String()
	if !strings.Contains(body, `hx-post="/app/feeds/poll?feed=f1"`) {
		t.Fatalf("item list should render a manual poll button for the current feed: %q", body)
	}
	if !strings.Contains(body, "最新記事を取得") {
		t.Fatalf("manual poll button label should be visible: %q", body)
	}
	if !strings.Contains(body, `hx-disabled-elt="find button"`) {
		t.Fatalf("manual poll form should disable its button while loading: %q", body)
	}
	if !strings.Contains(body, `class="poll-spinner"`) {
		t.Fatalf("manual poll button should include a loading spinner: %q", body)
	}
}

func TestFeedPollAllRefreshesCurrentView(t *testing.T) {
	t.Parallel()
	poll := &stubPoll{}
	subs := &stubSubscriptions{feeds: []domain.Feed{{ID: "f1", Title: "f1"}}}
	items := &stubItems{items: map[string][]domain.Item{
		"f1": {{ID: "i1", FeedID: "f1", Title: "全体更新後の記事"}},
	}}
	h := newAppHandler(t, subs, items)
	h.deps.Poll = poll
	req := httptest.NewRequest(http.MethodPost, "/app/feeds/poll", nil)
	req.Header.Set("HX-Request", "true")
	req = withSession(req, Session{Username: "owner", CSRFToken: "tok"})
	rec := httptest.NewRecorder()

	h.feedPoll(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status got %d want %d", rec.Code, http.StatusOK)
	}
	if !poll.polledAll {
		t.Fatalf("manual poll without a feed should poll all feeds")
	}
	if !strings.Contains(rec.Body.String(), "全体更新後の記事") {
		t.Fatalf("poll should render the current item list: %q", rec.Body.String())
	}
}

func TestFeedPollAllUsesBoundedContext(t *testing.T) {
	t.Parallel()
	poll := &stubPoll{}
	h := newAppHandler(t, &stubSubscriptions{}, &stubItems{items: map[string][]domain.Item{}})
	h.deps.Poll = poll
	req := httptest.NewRequest(http.MethodPost, "/app/feeds/poll", nil)
	req.Header.Set("HX-Request", "true")
	req = withSession(req, Session{Username: "owner", CSRFToken: "tok"})
	rec := httptest.NewRecorder()

	h.feedPoll(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status got %d want %d", rec.Code, http.StatusOK)
	}
	if !poll.pollAllHadDeadline {
		t.Fatalf("manual poll all should run with a deadline to avoid gateway timeouts")
	}
}

func TestFeedPollRendersCurrentViewWhenPollFails(t *testing.T) {
	t.Parallel()
	poll := &stubPoll{err: errors.New("fetch failed")}
	subs := &stubSubscriptions{feeds: []domain.Feed{{ID: "f1", Title: "f1"}}}
	items := &stubItems{items: map[string][]domain.Item{
		"f1": {{ID: "i1", FeedID: "f1", Title: "既存の記事"}},
	}}
	h := newAppHandler(t, subs, items)
	h.deps.Poll = poll
	req := httptest.NewRequest(http.MethodPost, "/app/feeds/f1/poll?feed=f1", nil)
	req.SetPathValue("feedID", "f1")
	req.Header.Set("HX-Request", "true")
	req = withSession(req, Session{Username: "owner", CSRFToken: "tok"})
	rec := httptest.NewRecorder()

	h.feedPoll(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status got %d want %d", rec.Code, http.StatusOK)
	}
	if !strings.Contains(rec.Body.String(), "既存の記事") {
		t.Fatalf("poll failure should keep the current item list visible: %q", rec.Body.String())
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

func TestOrderFeedNodesUsesConfiguredSort(t *testing.T) {
	t.Parallel()
	feeds := []domain.Feed{
		{ID: "f1", Title: "Zulu"},
		{ID: "f2", Title: "alpha"},
		{ID: "f3", Title: "Beta"},
	}
	unread := map[string]int{"f2": 3}

	tests := []struct {
		name     string
		settings domain.Settings
		want     []string
	}{
		{
			name:     "default title asc",
			settings: domain.DefaultSettings(),
			want:     []string{"alpha", "Beta", "Zulu"},
		},
		{
			name: "title desc",
			settings: func() domain.Settings {
				s := domain.DefaultSettings()
				s.FeedSortDirection = domain.SortDesc
				return s
			}(),
			want: []string{"Zulu", "Beta", "alpha"},
		},
		{
			name: "registered desc",
			settings: func() domain.Settings {
				s := domain.DefaultSettings()
				s.FeedSortKey = domain.FeedSortRegistered
				s.FeedSortDirection = domain.SortDesc
				return s
			}(),
			want: []string{"Beta", "alpha", "Zulu"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			nodes := orderFeedNodes(feeds, unread, tt.settings)
			got := make([]string, 0, len(nodes))
			for _, n := range nodes {
				got = append(got, n.Label)
			}
			if strings.Join(got, ",") != strings.Join(tt.want, ",") {
				t.Fatalf("order got %v want %v", got, tt.want)
			}
			for _, n := range nodes {
				if n.ID == "f2" && n.UnreadCount != 3 {
					t.Fatalf("unread count for f2 got %d want 3", n.UnreadCount)
				}
			}
		})
	}
}
