package handler

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/okamyuji/feedflow-go-htmx/internal/domain"
)

func sampleItems() map[string][]domain.Item {
	return map[string][]domain.Item{
		"f1": {
			{ID: "i1", FeedID: "f1", Title: "記事1", Summary: "要約1", PublishedAt: time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC)},
			{ID: "i2", FeedID: "f1", Title: "記事2", Summary: "要約2", Read: true},
		},
	}
}

func TestItemListRendersCards(t *testing.T) {
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
	if !strings.Contains(body, "記事1") || !strings.Contains(body, "記事2") {
		t.Fatalf("body should list both items: %q", body)
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
}

func TestItemStar(t *testing.T) {
	t.Parallel()
	subs := &stubSubscriptions{}
	items := &stubItems{items: sampleItems()}
	h := newAppHandler(t, subs, items)
	form := url.Values{"starred": {"true"}}
	req := httptest.NewRequest(http.MethodPost, "/app/items/f1/i1/star", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.SetPathValue("feedID", "f1")
	req.SetPathValue("itemID", "i1")
	req = withSession(req, Session{Username: "owner", CSRFToken: "tok"})
	rec := httptest.NewRecorder()

	h.itemStar(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status got %d want %d", rec.Code, http.StatusOK)
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
