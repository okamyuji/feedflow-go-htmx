package handler

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/okamyuji/feedflow-go-htmx/internal/domain"
)

func newBoardHandler(t *testing.T, items *stubItems) *Handler {
	t.Helper()
	subs := &stubSubscriptions{}
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

// boardItems ボード割り当てを記録するItemServiceスタブです。
type boardItems struct {
	stubItems
	lastFeed   string
	lastItem   string
	lastBoards []string
}

func (b *boardItems) SetBoards(feedID, itemID string, boardIDs []string) error {
	b.lastFeed = feedID
	b.lastItem = itemID
	b.lastBoards = boardIDs
	return nil
}

func TestItemSetBoards(t *testing.T) {
	t.Parallel()
	items := &boardItems{stubItems: stubItems{items: map[string][]domain.Item{
		"f1": {{ID: "i1", FeedID: "f1", Title: "記事1"}},
	}}}
	h := newBoardHandler(t, &items.stubItems)
	h.deps.Items = items
	form := url.Values{"board_ids": {"b1", "b2"}}
	req := httptest.NewRequest(http.MethodPost, "/app/items/f1/i1/boards", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.SetPathValue("feedID", "f1")
	req.SetPathValue("itemID", "i1")
	req = withSession(req, Session{Username: "owner", CSRFToken: "tok"})
	rec := httptest.NewRecorder()

	h.itemSetBoards(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("status got %d want %d", rec.Code, http.StatusNoContent)
	}
	if items.lastItem != "i1" {
		t.Fatalf("lastItem got %q want %q", items.lastItem, "i1")
	}
	if len(items.lastBoards) != 2 {
		t.Fatalf("lastBoards len got %d want 2", len(items.lastBoards))
	}
}
