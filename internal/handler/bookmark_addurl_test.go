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

// errAddURLStub 想定外のエラー経路を検証するためのスタブエラーです。
var errAddURLStub = errors.New("stub failure")

// newAddURLHandler URL追加フォームの検証用にHandlerとブックマークスタブを組んで返します。
func newAddURLHandler(t *testing.T, bms []domain.Bookmark) (*Handler, *stubItems, *stubBookmarks) {
	t.Helper()
	subs := &stubSubscriptions{feeds: []domain.Feed{{ID: "f1", Title: "通常フィード"}}}
	items := &stubItems{items: map[string][]domain.Item{}}
	bookmarks := &stubBookmarks{list: bms}
	h, err := New(Deps{
		Subscriptions:     subs,
		Items:             items,
		Bookmarks:         bookmarks,
		Mutes:             &stubMutes{},
		Poll:              &stubPoll{},
		Sessions:          &stubSessions{username: "owner", ok: true},
		CSRF:              &stubCSRF{ok: true, token: "tok"},
		SessionCookieName: "feedflow_session",
	})
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}
	return h, items, bookmarks
}

// postAddURL URL追加フォームのPOSTリクエストを組み立ててハンドラを直接呼びます。
func postAddURL(t *testing.T, h *Handler, query string, form url.Values) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/app/bookmarks/add-url"+query, strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("HX-Request", "true")
	req = withSession(req, Session{Username: "owner", CSRFToken: "tok"})
	rec := httptest.NewRecorder()
	h.bookmarkAddURL(rec, req)
	return rec
}

func TestBookmarkAddURLPassesInputToService(t *testing.T) {
	t.Parallel()
	h, _, bookmarks := newAddURLHandler(t, []domain.Bookmark{{ID: "b1", Name: "あとで"}})
	bookmarks.addedItem = domain.Item{ID: "s1", FeedID: domain.SavedPagesFeedID, Title: "保存したページ"}

	rec := postAddURL(t, h, "?view=bookmark", url.Values{
		"url":         {"https://example.com/a"},
		"bookmark_id": {"b1"},
	})

	if rec.Code != http.StatusOK {
		t.Fatalf("status got %d want %d", rec.Code, http.StatusOK)
	}
	if bookmarks.addedURL != "https://example.com/a" {
		t.Errorf("AddURLへ渡したURL got %q want %q", bookmarks.addedURL, "https://example.com/a")
	}
	if bookmarks.addedLabel != "b1" {
		t.Errorf("AddURLへ渡したラベル got %q want %q", bookmarks.addedLabel, "b1")
	}
}

func TestBookmarkAddURLRendersItemListOnSuccess(t *testing.T) {
	t.Parallel()
	h, items, bookmarks := newAddURLHandler(t, nil)
	items.items[domain.SavedPagesFeedID] = []domain.Item{
		{ID: "s1", FeedID: domain.SavedPagesFeedID, Title: "保存したページの見出し", Bookmarked: true},
	}
	bookmarks.addedItem = items.items[domain.SavedPagesFeedID][0]

	rec := postAddURL(t, h, "?view=bookmark", url.Values{"url": {"https://example.com/a"}})

	body := rec.Body.String()
	if !strings.Contains(body, "保存したページの見出し") {
		t.Errorf("追加後の一覧に保存したページが出るべき: %q", body)
	}
	if !strings.Contains(body, `class="item-list"`) {
		t.Errorf("成功時は記事一覧を返すべき: %q", body)
	}
}

func TestBookmarkAddURLShowsErrorMessages(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		err     error
		wantMsg string
	}{
		{name: "URL不正", err: service.ErrInvalidURL, wantMsg: "URLの形式が正しくありません"},
		{name: "ラベル不在", err: service.ErrBookmarkNotFound, wantMsg: "指定のラベルが見つかりません"},
		{name: "その他", err: errAddURLStub, wantMsg: "保存できませんでした"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			h, items, bookmarks := newAddURLHandler(t, nil)
			bookmarks.addURLError = tt.err

			rec := postAddURL(t, h, "?view=bookmark", url.Values{"url": {"https://example.com/a"}})

			if rec.Code != http.StatusOK {
				t.Fatalf("status got %d want %d", rec.Code, http.StatusOK)
			}
			body := rec.Body.String()
			if !strings.Contains(body, tt.wantMsg) {
				t.Errorf("エラー文言が出ていません want %q got %q", tt.wantMsg, body)
			}
			if !strings.Contains(body, `class="add-url-form"`) {
				t.Errorf("失敗時は入力フォームを返すべき: %q", body)
			}
			if len(items.items[domain.SavedPagesFeedID]) != 0 {
				t.Error("失敗時に記事が作られています")
			}
		})
	}
}

func TestBookmarkAddURLKeepsCurrentQueryInPostTarget(t *testing.T) {
	t.Parallel()
	h, _, bookmarks := newAddURLHandler(t, nil)
	bookmarks.addURLError = service.ErrInvalidURL

	rec := postAddURL(t, h, "?bookmark=b1", url.Values{"url": {"bad"}})

	if !strings.Contains(rec.Body.String(), `hx-post="/app/bookmarks/add-url?bookmark=b1"`) {
		t.Errorf("再描画したフォームは表示条件を引き継ぐべき: %q", rec.Body.String())
	}
}

func TestItemListShowsAddURLFormOnlyInBookmarkViews(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name  string
		query string
		want  bool
	}{
		{name: "ブックマークビュー", query: "?view=bookmark", want: true},
		{name: "ラベル絞り込み", query: "?bookmark=b1", want: true},
		{name: "すべて", query: "", want: false},
		{name: "既読", query: "?view=read", want: false},
		{name: "あとで読む", query: "?view=readlater", want: false},
		{name: "単一フィード", query: "?feed=f1", want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			h, _, _ := newAddURLHandler(t, nil)
			req := httptest.NewRequest(http.MethodGet, "/app/items"+tt.query, nil)
			req.Header.Set("HX-Request", "true")
			req = withSession(req, Session{Username: "owner", CSRFToken: "tok"})
			rec := httptest.NewRecorder()

			h.itemList(rec, req)

			got := strings.Contains(rec.Body.String(), `class="add-url-form"`)
			if got != tt.want {
				t.Errorf("URL追加フォームの表示 got %v want %v", got, tt.want)
			}
		})
	}
}

func TestAddURLFormListsExistingLabels(t *testing.T) {
	t.Parallel()
	h, _, _ := newAddURLHandler(t, []domain.Bookmark{{ID: "b1", Name: "あとで"}, {ID: "b2", Name: "資料"}})
	req := httptest.NewRequest(http.MethodGet, "/app/items?view=bookmark", nil)
	req.Header.Set("HX-Request", "true")
	req = withSession(req, Session{Username: "owner", CSRFToken: "tok"})
	rec := httptest.NewRecorder()

	h.itemList(rec, req)

	body := rec.Body.String()
	for _, want := range []string{`value="b1"`, "あとで", `value="b2"`, "資料", "ラベルなし", `name="bookmark_id"`} {
		if !strings.Contains(body, want) {
			t.Errorf("ラベル選択肢に %q が含まれていません", want)
		}
	}
}

func TestAddURLFormOmitsSelectWhenNoLabels(t *testing.T) {
	t.Parallel()
	h, _, _ := newAddURLHandler(t, nil)
	req := httptest.NewRequest(http.MethodGet, "/app/items?view=bookmark", nil)
	req.Header.Set("HX-Request", "true")
	req = withSession(req, Session{Username: "owner", CSRFToken: "tok"})
	rec := httptest.NewRecorder()

	h.itemList(rec, req)

	if strings.Contains(rec.Body.String(), `name="bookmark_id"`) {
		t.Error("ラベルが1件も無いときはセレクトを出してはいけません")
	}
}

func TestAddURLFormCarriesCSRFToken(t *testing.T) {
	t.Parallel()
	h, _, _ := newAddURLHandler(t, nil)
	req := httptest.NewRequest(http.MethodGet, "/app/items?view=bookmark", nil)
	req.Header.Set("HX-Request", "true")
	req = withSession(req, Session{Username: "owner", CSRFToken: "tok"})
	rec := httptest.NewRecorder()

	h.itemList(rec, req)

	if !strings.Contains(rec.Body.String(), `name="csrf_token" value="tok"`) {
		t.Errorf("フォームにCSRFトークンが埋まっていません: %q", rec.Body.String())
	}
}
