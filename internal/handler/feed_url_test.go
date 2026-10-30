package handler

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/okamyuji/feedflow-go-htmx/internal/domain"
	"github.com/okamyuji/feedflow-go-htmx/internal/service"
)

func brokenFeed() domain.Feed {
	return domain.Feed{
		ID: "f1", Title: "DeNA", FeedURL: "http://old.example.com/atom.xml", SiteURL: "https://new.example.com/",
		ConsecutiveErrors: domain.ErrorThreshold, LastError: "failed to fetch: HTTP 500",
	}
}

func newFeedURLRequest(feedID, rawURL string) *http.Request {
	form := url.Values{"url": {rawURL}}
	req := httptest.NewRequest(http.MethodPost, "/app/feeds/"+feedID+"/url?feed="+feedID, strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("HX-Request", "true")
	req.SetPathValue("feedID", feedID)
	return withSession(req, Session{Username: "owner", CSRFToken: "tok"})
}

func TestTreeRendersURLFormOnlyForErrorFeeds(t *testing.T) {
	t.Parallel()
	healthy := domain.Feed{ID: "f2", Title: "正常", FeedURL: "https://ok.example.com/feed"}
	subs := &stubSubscriptions{feeds: []domain.Feed{brokenFeed(), healthy}}
	h := newAppHandler(t, subs, &stubItems{items: map[string][]domain.Item{}})

	data, err := h.treeData(httptest.NewRequest(http.MethodGet, "/app", nil))
	if err != nil {
		t.Fatalf("treeData returned error: %v", err)
	}
	rec := httptest.NewRecorder()
	h.renderPartial(rec, http.StatusOK, "_tree_pane.html", data)
	body := rec.Body.String()

	for _, want := range []string{
		`id="feed-url-form-f1"`,
		`hx-post="/app/feeds/f1/url?feed=f1"`,
		`value="http://old.example.com/atom.xml"`,
		"failed to fetch: HTTP 500",
		"https://new.example.com/",
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("error feed must render %q in the tree: %s", want, body)
		}
	}
	if strings.Contains(body, "/app/feeds/f2/url") {
		t.Fatalf("healthy feed must not render the url form")
	}
}

func TestFeedUpdateURLSwitchesSourceAndPolls(t *testing.T) {
	t.Parallel()
	subs := &stubSubscriptions{feeds: []domain.Feed{brokenFeed()}}
	items := &stubItems{items: map[string][]domain.Item{"f1": {{ID: "i1", FeedID: "f1", Title: "新しい記事"}}}}
	h := newAppHandler(t, subs, items)
	poll := &stubPoll{}
	h.deps.Poll = poll
	rec := httptest.NewRecorder()

	h.feedUpdateURL(rec, newFeedURLRequest("f1", "https://new.example.com/"))

	if rec.Code != http.StatusOK {
		t.Fatalf("status got %d want 200", rec.Code)
	}
	if subs.updatedID != "f1" || subs.updatedURL != "https://new.example.com/" {
		t.Fatalf("UpdateFeedURL got id=%q url=%q", subs.updatedID, subs.updatedURL)
	}
	if poll.polledFeedID != "f1" {
		t.Fatalf("the feed must be polled right after the switch, got %q", poll.polledFeedID)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "新しい記事") || !strings.Contains(body, `hx-swap-oob="true"`) {
		t.Fatalf("response must render the feed list with a tree refresh: %s", body)
	}
	if strings.Contains(body, `id="feed-url-form-f1"`) {
		t.Fatalf("recovered feed must not keep the url form: %s", body)
	}
}

func TestFeedUpdateURLRendersFormWithMessageOnFailure(t *testing.T) {
	t.Parallel()
	tests := []struct {
		err  error
		want string
	}{
		{service.ErrInvalidURL, "URLの形式が正しくありません"},
		{service.ErrFeedNotDiscovered, "フィードが見つかりませんでした"},
		{service.ErrDuplicateFeed, "すでに購読しています"},
		{service.ErrFeedNotEditable, "このフィードはURLを変更できません"},
		{errors.New("failed to fetch: HTTP 404"), "フィードを取得できませんでした"},
	}
	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			t.Parallel()
			subs := &stubSubscriptions{feeds: []domain.Feed{brokenFeed()}, updateErr: tt.err}
			h := newAppHandler(t, subs, &stubItems{items: map[string][]domain.Item{}})
			poll := &stubPoll{}
			h.deps.Poll = poll
			rec := httptest.NewRecorder()

			h.feedUpdateURL(rec, newFeedURLRequest("f1", "https://typed.example.com/"))

			if rec.Code != http.StatusOK {
				t.Fatalf("status got %d want 200", rec.Code)
			}
			if got := rec.Header().Get("HX-Retarget"); got != "#feed-url-form-f1" {
				t.Fatalf("HX-Retarget got %q", got)
			}
			if got := rec.Header().Get("HX-Reswap"); got != "outerHTML" {
				t.Fatalf("HX-Reswap got %q", got)
			}
			if got := rec.Header().Get("HX-Push-Url"); got != "false" {
				t.Fatalf("HX-Push-Url got %q want false", got)
			}
			body := rec.Body.String()
			if !strings.Contains(body, tt.want) || !strings.Contains(body, `value="https://typed.example.com/"`) {
				t.Fatalf("form must show the message and keep the input: %s", body)
			}
			if strings.Contains(body, "HTTP 404") {
				t.Fatalf("the raw error must not be shown in the form message: %s", body)
			}
			if poll.polledFeedID != "" {
				t.Fatalf("poll must not run when the switch failed")
			}
		})
	}
}

func TestFeedUpdateURLRejectsMissingFeedID(t *testing.T) {
	t.Parallel()
	h := newAppHandler(t, &stubSubscriptions{}, &stubItems{items: map[string][]domain.Item{}})
	rec := httptest.NewRecorder()

	h.feedUpdateURL(rec, newFeedURLRequest("", "https://new.example.com/"))

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status got %d want 400", rec.Code)
	}
}
