package handler

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/okamyuji/feedflow-go-htmx/internal/domain"
)

func newBookmarkHandler(t *testing.T, items *stubItems, bookmarks *stubBookmarks) *Handler {
	t.Helper()
	h, err := New(Deps{
		Subscriptions:     &stubSubscriptions{},
		Items:             items,
		Bookmarks:         bookmarks,
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

func TestBookmarkPickerRendersOptions(t *testing.T) {
	t.Parallel()
	items := &stubItems{items: map[string][]domain.Item{
		"f1": {{ID: "i1", FeedID: "f1", BookmarkIDs: []string{"読み物"}}},
	}}
	bookmarks := &stubBookmarks{list: []domain.Bookmark{{ID: "読み物", Name: "読み物"}, {ID: "Go", Name: "Go"}}}
	h := newBookmarkHandler(t, items, bookmarks)

	req := httptest.NewRequest(http.MethodGet, "/app/items/f1/i1/bookmarks", nil)
	req.SetPathValue("feedID", "f1")
	req.SetPathValue("itemID", "i1")
	req = withSession(req, Session{Username: "owner", CSRFToken: "tok"})
	rec := httptest.NewRecorder()

	h.bookmarkPicker(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status got %d want %d", rec.Code, http.StatusOK)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "読み物") || !strings.Contains(body, "Go") {
		t.Fatalf("picker should list bookmark names: %q", body)
	}
	if !strings.Contains(body, "is-checked") {
		t.Fatalf("所属済みのブックマークはチェック表示されるべき: %q", body)
	}
	if !strings.Contains(body, "保存済み") {
		t.Fatalf("所属があればカードの保存済み表示をOOB更新すべき: %q", body)
	}
}

func TestBookmarkToggle(t *testing.T) {
	t.Parallel()
	items := &stubItems{items: map[string][]domain.Item{"f1": {{ID: "i1", FeedID: "f1"}}}}
	bookmarks := &stubBookmarks{list: []domain.Bookmark{{ID: "b1", Name: "あとで実装する"}}}
	h := newBookmarkHandler(t, items, bookmarks)

	form := url.Values{"bookmark": {"b1"}}
	req := httptest.NewRequest(http.MethodPost, "/app/items/f1/i1/bookmarks/toggle", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.SetPathValue("feedID", "f1")
	req.SetPathValue("itemID", "i1")
	req = withSession(req, Session{Username: "owner", CSRFToken: "tok"})
	rec := httptest.NewRecorder()

	h.bookmarkToggle(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status got %d want %d", rec.Code, http.StatusOK)
	}
	if bookmarks.toggled != "b1" || bookmarks.lastItemID != "i1" {
		t.Fatalf("toggle should record b1/i1 got %q/%q", bookmarks.toggled, bookmarks.lastItemID)
	}
}

func TestBookmarkCreate(t *testing.T) {
	t.Parallel()
	items := &stubItems{items: map[string][]domain.Item{"f1": {{ID: "i1", FeedID: "f1"}}}}
	bookmarks := &stubBookmarks{}
	h := newBookmarkHandler(t, items, bookmarks)

	form := url.Values{"name": {"新ブックマーク"}, "feed": {"f1"}, "item": {"i1"}}
	req := httptest.NewRequest(http.MethodPost, "/app/bookmarks", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req = withSession(req, Session{Username: "owner", CSRFToken: "tok"})
	rec := httptest.NewRecorder()

	h.bookmarkCreate(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status got %d want %d", rec.Code, http.StatusOK)
	}
	if bookmarks.created != "新ブックマーク" {
		t.Fatalf("create should record the name got %q", bookmarks.created)
	}
	if !strings.Contains(rec.Body.String(), "新ブックマーク") {
		t.Fatalf("再描画されたピッカーに新規名称が含まれるべき: %q", rec.Body.String())
	}
}
