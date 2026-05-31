package handler

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/okamyuji/feedflow-go-htmx/internal/domain"
)

var errSettingsUnavailable = errors.New("settings unavailable")

func sampleItems() map[string][]domain.Item {
	return map[string][]domain.Item{
		"f1": {
			{ID: "i1", FeedID: "f1", Title: "記事1", Summary: "要約1", PublishedAt: time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC)},
			{ID: "i2", FeedID: "f1", Title: "記事2", Summary: "要約2", Read: true},
		},
	}
}

func TestItemListSingleFeedShowsReadHeadThenUnread(t *testing.T) {
	t.Parallel()
	subs := &stubSubscriptions{feeds: []domain.Feed{{ID: "f1", Title: "f1"}}}
	h := newAppHandler(t, subs, &stubItems{items: sampleItems()})
	req := httptest.NewRequest(http.MethodGet, "/app/items?feed=f1", nil)
	req.Header.Set("HX-Request", "true")
	req = withSession(req, Session{Username: "owner", CSRFToken: "tok"})
	rec := httptest.NewRecorder()

	h.itemList(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status got %d want %d", rec.Code, http.StatusOK)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "記事1") {
		t.Fatalf("単一フィードは未読の記事1を表示すべきです: %q", body)
	}
	if !strings.Contains(body, "記事2") {
		t.Fatalf("単一フィードは既読の記事2を先頭に再表示すべきです: %q", body)
	}
	if !strings.Contains(body, "ここから未読") {
		t.Fatalf("既読先頭群と未読の境界に区切りを出すべきです: %q", body)
	}
}

func TestItemListAllViewHidesRead(t *testing.T) {
	t.Parallel()
	subs := &stubSubscriptions{feeds: []domain.Feed{{ID: "f1", Title: "f1"}}}
	h := newAppHandler(t, subs, &stubItems{items: sampleItems()})
	req := httptest.NewRequest(http.MethodGet, "/app/items", nil)
	req.Header.Set("HX-Request", "true")
	req = withSession(req, Session{Username: "owner", CSRFToken: "tok"})
	rec := httptest.NewRecorder()

	h.itemList(rec, req)

	body := rec.Body.String()
	if !strings.Contains(body, "記事1") {
		t.Fatalf("すべての一覧は未読の記事1を表示すべきです: %q", body)
	}
	if strings.Contains(body, "記事2") {
		t.Fatalf("すべての一覧は既読の記事2を含めてはいけません: %q", body)
	}
}

func TestItemListUsesSavedDefaultView(t *testing.T) {
	t.Parallel()
	subs := &stubSubscriptions{feeds: []domain.Feed{{ID: "f1", Title: "f1"}}}
	items := &stubItems{items: sampleItems()}
	settings := domain.DefaultSettings()
	settings.DefaultView = domain.ViewMagazine
	h := newAppHandlerWithSettings(t, subs, items, &stubSettings{current: settings})
	req := httptest.NewRequest(http.MethodGet, "/app/items", nil)
	req.Header.Set("HX-Request", "true")
	req = withSession(req, Session{Username: "owner", CSRFToken: "tok"})
	rec := httptest.NewRecorder()

	h.itemList(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status got %d want %d", rec.Code, http.StatusOK)
	}
	if !strings.Contains(rec.Body.String(), `data-view="magazine"`) {
		t.Fatalf("item list should use saved default view: %q", rec.Body.String())
	}
}

func TestItemListFallsBackToDefaultViewWhenSettingsFail(t *testing.T) {
	t.Parallel()
	subs := &stubSubscriptions{feeds: []domain.Feed{{ID: "f1", Title: "f1"}}}
	items := &stubItems{items: sampleItems()}
	h := newAppHandlerWithSettings(t, subs, items, &stubSettings{getErr: errSettingsUnavailable})
	req := httptest.NewRequest(http.MethodGet, "/app/items", nil)
	req.Header.Set("HX-Request", "true")
	req = withSession(req, Session{Username: "owner", CSRFToken: "tok"})
	rec := httptest.NewRecorder()

	h.itemList(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status got %d want %d", rec.Code, http.StatusOK)
	}
	if !strings.Contains(rec.Body.String(), `data-view="card"`) {
		t.Fatalf("item list should fall back to default view: %q", rec.Body.String())
	}
}

func TestItemListReadViewShowsOnlyRead(t *testing.T) {
	t.Parallel()
	subs := &stubSubscriptions{feeds: []domain.Feed{{ID: "f1", Title: "f1"}}}
	h := newAppHandler(t, subs, &stubItems{items: sampleItems()})
	req := httptest.NewRequest(http.MethodGet, "/app/items?view=read", nil)
	req.Header.Set("HX-Request", "true")
	req = withSession(req, Session{Username: "owner", CSRFToken: "tok"})
	rec := httptest.NewRecorder()

	h.itemList(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status got %d want %d", rec.Code, http.StatusOK)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "記事2") {
		t.Fatalf("既読ビューは既読の記事2を表示すべきです: %q", body)
	}
	if strings.Contains(body, "記事1") {
		t.Fatalf("既読ビューに未読の記事1を含めてはいけません: %q", body)
	}
}

func TestItemListReadViewOrdersByPublishedAtDescending(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)
	subs := &stubSubscriptions{feeds: []domain.Feed{
		{ID: "older-feed", Title: "older"},
		{ID: "newer-feed", Title: "newer"},
	}}
	h := newAppHandler(t, subs, &stubItems{items: map[string][]domain.Item{
		"older-feed": {
			{
				ID:          "old",
				FeedID:      "older-feed",
				Title:       "古い既読",
				Read:        true,
				PublishedAt: now.Add(-24 * time.Hour),
				FetchedAt:   now.Add(-23 * time.Hour),
			},
		},
		"newer-feed": {
			{
				ID:          "new",
				FeedID:      "newer-feed",
				Title:       "新しい既読",
				Read:        true,
				PublishedAt: now,
				FetchedAt:   now,
			},
		},
	}})
	req := httptest.NewRequest(http.MethodGet, "/app/items?view=read", nil)
	req.Header.Set("HX-Request", "true")
	req = withSession(req, Session{Username: "owner", CSRFToken: "tok"})
	rec := httptest.NewRecorder()

	h.itemList(rec, req)

	body := rec.Body.String()
	newerIndex := strings.Index(body, "新しい既読")
	olderIndex := strings.Index(body, "古い既読")
	if newerIndex == -1 || olderIndex == -1 {
		t.Fatalf("既読ビューは新旧どちらの記事も表示すべきです: %q", body)
	}
	if newerIndex > olderIndex {
		t.Fatalf("既読ビューは公開日時の新しい順に表示すべきです: %q", body)
	}
}

func TestItemOverlayRendersContent(t *testing.T) {
	t.Parallel()
	subs := &stubSubscriptions{feeds: []domain.Feed{{ID: "f1", Title: "f1"}}}
	h := newAppHandler(t, subs, &stubItems{items: sampleItems()})
	req := httptest.NewRequest(http.MethodGet, "/app/items/f1/i1", nil)
	req.SetPathValue("feedID", "f1")
	req.SetPathValue("itemID", "i1")
	req = withSession(req, Session{Username: "owner", CSRFToken: "tok"})
	rec := httptest.NewRecorder()

	h.itemOverlay(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status got %d want %d", rec.Code, http.StatusOK)
	}
	if !strings.Contains(rec.Body.String(), "記事1") {
		t.Fatalf("overlay should render item title: %q", rec.Body.String())
	}
}

func TestItemOverlayNotFound(t *testing.T) {
	t.Parallel()
	subs := &stubSubscriptions{}
	h := newAppHandler(t, subs, &stubItems{items: sampleItems()})
	req := httptest.NewRequest(http.MethodGet, "/app/items/f1/missing", nil)
	req.SetPathValue("feedID", "f1")
	req.SetPathValue("itemID", "missing")
	req = withSession(req, Session{Username: "owner", CSRFToken: "tok"})
	rec := httptest.NewRecorder()

	h.itemOverlay(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status got %d want %d", rec.Code, http.StatusNotFound)
	}
}

func TestItemMarkRead(t *testing.T) {
	t.Parallel()
	subs := &stubSubscriptions{}
	items := &stubItems{items: sampleItems()}
	h := newAppHandler(t, subs, items)
	form := url.Values{"read": {"true"}}
	req := httptest.NewRequest(http.MethodPost, "/app/items/f1/i1/read", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.SetPathValue("feedID", "f1")
	req.SetPathValue("itemID", "i1")
	req = withSession(req, Session{Username: "owner", CSRFToken: "tok"})
	rec := httptest.NewRecorder()

	h.itemMarkRead(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status got %d want %d", rec.Code, http.StatusOK)
	}
	if !strings.Contains(rec.Body.String(), "item-card") {
		t.Fatalf("body should re-render the card: %q", rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `hx-swap-oob="true"`) {
		t.Fatalf("body should include the out-of-band tree swap: %q", rec.Body.String())
	}
}

func TestItemMarkReadUpdatesTreeUnreadOOB(t *testing.T) {
	t.Parallel()
	subs := &stubSubscriptions{feeds: []domain.Feed{{ID: "f1", Title: "f1"}}}
	items := &stubItems{items: map[string][]domain.Item{
		"f1": {
			{ID: "i1", FeedID: "f1"},
			{ID: "i2", FeedID: "f1"},
		},
	}}
	h := newAppHandler(t, subs, items)
	form := url.Values{"read": {"true"}}
	req := httptest.NewRequest(http.MethodPost, "/app/items/f1/i1/read", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.SetPathValue("feedID", "f1")
	req.SetPathValue("itemID", "i1")
	req = withSession(req, Session{Username: "owner", CSRFToken: "tok"})
	rec := httptest.NewRecorder()

	h.itemMarkRead(rec, req)

	body := rec.Body.String()
	if !strings.Contains(body, `id="tree-pane"`) || !strings.Contains(body, `hx-swap-oob="true"`) {
		t.Fatalf("body should swap the tree pane out of band: %q", body)
	}
}

func TestItemMarkAllUpdatesTreeOOB(t *testing.T) {
	t.Parallel()
	subs := &stubSubscriptions{feeds: []domain.Feed{{ID: "f1", Title: "f1"}}}
	items := &stubItems{items: sampleItems()}
	h := newAppHandler(t, subs, items)
	req := httptest.NewRequest(http.MethodPost, "/app/items/markall", nil)
	req.Header.Set("HX-Request", "true")
	req = withSession(req, Session{Username: "owner", CSRFToken: "tok"})
	rec := httptest.NewRecorder()

	h.itemMarkAll(rec, req)

	if !strings.Contains(rec.Body.String(), `hx-swap-oob="true"`) {
		t.Fatalf("mark all should include the out-of-band tree swap: %q", rec.Body.String())
	}
}

func TestItemMarkAll(t *testing.T) {
	t.Parallel()
	subs := &stubSubscriptions{feeds: []domain.Feed{{ID: "f1", Title: "f1"}}}
	items := &stubItems{items: sampleItems()}
	h := newAppHandler(t, subs, items)
	req := httptest.NewRequest(http.MethodPost, "/app/items/markall", nil)
	req.Header.Set("HX-Request", "true")
	req = withSession(req, Session{Username: "owner", CSRFToken: "tok"})
	rec := httptest.NewRecorder()

	h.itemMarkAll(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status got %d want %d", rec.Code, http.StatusOK)
	}
}

func TestBulkReadContext(t *testing.T) {
	t.Parallel()
	subs := &stubSubscriptions{feeds: []domain.Feed{{ID: "f1", Title: "フィード1"}}}
	h := newAppHandler(t, subs, &stubItems{items: sampleItems()})

	cases := []struct {
		name      string
		target    string
		wantScope string
		wantFeed  string
		wantTitle string
	}{
		{"特定フィード", "/app/items?feed=f1", "feed", "f1", "フィード1"},
		{"既定の未読ストリーム", "/app/items", "all", "", ""},
		{"すべてビュー", "/app/items?view=all", "all", "", ""},
		{"既読ビュー", "/app/items?view=read", "none", "", ""},
		{"ブックマークビュー", "/app/items?view=bookmark", "none", "", ""},
		{"あとで読むビュー", "/app/items?view=readlater", "none", "", ""},
		{"カテゴリ", "/app/items?category=c1", "none", "", ""},
		{"ブックマーク絞り込み", "/app/items?bookmark=b1", "none", "", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, tc.target, nil)
			scope, feed, title := h.bulkReadContext(req)
			if scope != tc.wantScope || feed != tc.wantFeed || title != tc.wantTitle {
				t.Fatalf("got (%q,%q,%q) want (%q,%q,%q)", scope, feed, title, tc.wantScope, tc.wantFeed, tc.wantTitle)
			}
		})
	}
}

func TestItemListFeedScopeShowsPerFeedMarkRead(t *testing.T) {
	t.Parallel()
	subs := &stubSubscriptions{feeds: []domain.Feed{{ID: "f1", Title: "フィード1"}}}
	h := newAppHandler(t, subs, &stubItems{items: sampleItems()})
	req := httptest.NewRequest(http.MethodGet, "/app/items?feed=f1", nil)
	req.Header.Set("HX-Request", "true")
	req = withSession(req, Session{Username: "owner", CSRFToken: "tok"})
	rec := httptest.NewRecorder()

	h.itemList(rec, req)

	body := rec.Body.String()
	if !strings.Contains(body, "このフィードを既読") {
		t.Fatalf("フィード選択時は主ボタンがこのフィードを既読であるべきです: %q", body)
	}
	if !strings.Contains(body, "markall?feed=f1") {
		t.Fatalf("主ボタンは表示中フィードを対象に送るべきです: %q", body)
	}
	if !strings.Contains(body, "すべてのフィードを既読") {
		t.Fatalf("メニューにすべてのフィードを既読があるべきです: %q", body)
	}
}

func TestItemListMarksActiveFeedAndShowsLabel(t *testing.T) {
	t.Parallel()
	subs := &stubSubscriptions{feeds: []domain.Feed{{ID: "f1", Title: "フィード1"}}}
	h := newAppHandler(t, subs, &stubItems{items: sampleItems()})
	req := httptest.NewRequest(http.MethodGet, "/app/items?feed=f1", nil)
	req.Header.Set("HX-Request", "true")
	req = withSession(req, Session{Username: "owner", CSRFToken: "tok"})
	rec := httptest.NewRecorder()

	h.itemList(rec, req)

	body := rec.Body.String()
	if !strings.Contains(body, "tree-active") {
		t.Fatalf("選択中フィードはtree-activeで強調されるべきです: %q", body)
	}
	if !strings.Contains(body, `class="item-list-title">フィード1`) {
		t.Fatalf("右ペインに選択中フィード名が出るべきです: %q", body)
	}
}

func TestItemListReadViewShowsReadLabel(t *testing.T) {
	t.Parallel()
	subs := &stubSubscriptions{feeds: []domain.Feed{{ID: "f1", Title: "フィード1"}}}
	h := newAppHandler(t, subs, &stubItems{items: sampleItems()})
	req := httptest.NewRequest(http.MethodGet, "/app/items?view=read", nil)
	req.Header.Set("HX-Request", "true")
	req = withSession(req, Session{Username: "owner", CSRFToken: "tok"})
	rec := httptest.NewRecorder()

	h.itemList(rec, req)

	if !strings.Contains(rec.Body.String(), `class="item-list-title">既読`) {
		t.Fatalf("既読ビューでは選択中名称が既読であるべきです: %q", rec.Body.String())
	}
}

func TestItemListBookmarkViewHidesBulkRead(t *testing.T) {
	t.Parallel()
	subs := &stubSubscriptions{feeds: []domain.Feed{{ID: "f1", Title: "フィード1"}}}
	h := newAppHandler(t, subs, &stubItems{items: sampleItems()})
	req := httptest.NewRequest(http.MethodGet, "/app/items?view=bookmark", nil)
	req.Header.Set("HX-Request", "true")
	req = withSession(req, Session{Username: "owner", CSRFToken: "tok"})
	rec := httptest.NewRecorder()

	h.itemList(rec, req)

	body := rec.Body.String()
	if strings.Contains(body, "このフィードを既読") || strings.Contains(body, "表示中をすべて既読") {
		t.Fatalf("ブックマークビューでは一括既読コントロールを出してはいけません: %q", body)
	}
}
