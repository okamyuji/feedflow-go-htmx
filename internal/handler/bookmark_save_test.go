package handler

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/okamyuji/feedflow-go-htmx/internal/domain"
)

// TestItemBookmarkSaveRendersSaved 保存(bookmarked=true)はカードを再描画し「保存済み」表示にします。
func TestItemBookmarkSaveRendersSaved(t *testing.T) {
	t.Parallel()
	subs := &stubSubscriptions{feeds: []domain.Feed{{ID: "f1", Title: "f1"}}}
	// SetBookmarked後に findItem で読み直すため、Bookmarked=true の記事を返すstubにします。
	items := &stubItems{items: map[string][]domain.Item{
		"f1": {{ID: "i1", FeedID: "f1", Title: "記事1", Bookmarked: true}},
	}}
	h := newAppHandler(t, subs, items)
	form := url.Values{"bookmarked": {"true"}}
	req := httptest.NewRequest(http.MethodPost, "/app/items/f1/i1/bookmark", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.SetPathValue("feedID", "f1")
	req.SetPathValue("itemID", "i1")
	req = withSession(req, Session{Username: "owner", CSRFToken: "tok"})
	rec := httptest.NewRecorder()

	h.itemBookmark(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status got %d want %d", rec.Code, http.StatusOK)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "保存済み") {
		t.Fatalf("保存時はカードに保存済み表示を出すべき: %q", body)
	}
}

// TestItemBookmarkUnsetReRendersList 解除(bookmarked=false)は一覧全体を再描画してビューから外します。
func TestItemBookmarkUnsetReRendersList(t *testing.T) {
	t.Parallel()
	subs := &stubSubscriptions{feeds: []domain.Feed{{ID: "f1", Title: "f1"}}}
	items := &stubItems{items: map[string][]domain.Item{
		"f1": {{ID: "i1", FeedID: "f1", Title: "記事1"}},
	}}
	h := newAppHandler(t, subs, items)
	form := url.Values{"bookmarked": {"false"}}
	req := httptest.NewRequest(http.MethodPost, "/app/items/f1/i1/bookmark?bookmark=b1", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("HX-Request", "true")
	req.SetPathValue("feedID", "f1")
	req.SetPathValue("itemID", "i1")
	req = withSession(req, Session{Username: "owner", CSRFToken: "tok"})
	rec := httptest.NewRecorder()

	h.itemBookmark(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status got %d want %d", rec.Code, http.StatusOK)
	}
	// 一覧テンプレート(item-list)とOOBツリーが返ることを確認します。
	body := rec.Body.String()
	if !strings.Contains(body, "item-list") || !strings.Contains(body, `hx-swap-oob="true"`) {
		t.Fatalf("解除時は一覧全体を再描画すべき: %q", body)
	}
}

// TestItemCardSecondButtonIsUnbookmarkInBookmarkView ブックマークビューでは記事カードの2つ目ボタンが「ブックマーク解除」になります。
func TestItemCardSecondButtonIsUnbookmarkInBookmarkView(t *testing.T) {
	t.Parallel()
	subs := &stubSubscriptions{feeds: []domain.Feed{{ID: "f1", Title: "f1"}}}
	items := &stubItems{items: map[string][]domain.Item{
		"f1": {{ID: "i1", FeedID: "f1", Title: "保存記事", Bookmarked: true}},
	}}
	h := newBookmarkTreeHandler(t, subs, items, []domain.Bookmark{{ID: "b1", Name: "ArgoCD"}})
	req := httptest.NewRequest(http.MethodGet, "/app/items?view=bookmark", nil)
	req.Header.Set("HX-Request", "true")
	req = withSession(req, Session{Username: "owner", CSRFToken: "tok"})
	rec := httptest.NewRecorder()

	h.itemList(rec, req)

	body := rec.Body.String()
	if !strings.Contains(body, "ブックマーク解除") {
		t.Fatalf("ブックマークビューの記事カードは解除ボタンを出すべき: %q", body)
	}
}

// TestItemCardSecondButtonIsReadInNormalView 通常ビューでは2つ目ボタンが既読トグルのままです。
func TestItemCardSecondButtonIsReadInNormalView(t *testing.T) {
	t.Parallel()
	subs := &stubSubscriptions{feeds: []domain.Feed{{ID: "f1", Title: "f1"}}}
	items := &stubItems{items: map[string][]domain.Item{
		"f1": {{ID: "i1", FeedID: "f1", Title: "通常記事"}},
	}}
	h := newAppHandler(t, subs, items)
	req := httptest.NewRequest(http.MethodGet, "/app/items?feed=f1", nil)
	req.Header.Set("HX-Request", "true")
	req = withSession(req, Session{Username: "owner", CSRFToken: "tok"})
	rec := httptest.NewRecorder()

	h.itemList(rec, req)

	body := rec.Body.String()
	if strings.Contains(body, "ブックマーク解除") {
		t.Fatalf("通常ビューでは解除ボタンを出してはいけません: %q", body)
	}
	if !strings.Contains(body, "/read") {
		t.Fatalf("通常ビューでは既読トグルを出すべき: %q", body)
	}
}

// TestBookmarkViewUsesBookmarkedFlag ブックマークビューはラベル0件でも保存済み記事を表示します。
func TestBookmarkViewUsesBookmarkedFlag(t *testing.T) {
	t.Parallel()
	subs := &stubSubscriptions{feeds: []domain.Feed{{ID: "f1", Title: "f1"}}}
	items := &stubItems{items: map[string][]domain.Item{
		"f1": {
			{ID: "saved", FeedID: "f1", Title: "ラベル無し保存記事", Bookmarked: true},
			{ID: "plain", FeedID: "f1", Title: "通常記事"},
		},
	}}
	h := newAppHandler(t, subs, items)
	req := httptest.NewRequest(http.MethodGet, "/app/items?view=bookmark", nil)
	req.Header.Set("HX-Request", "true")
	req = withSession(req, Session{Username: "owner", CSRFToken: "tok"})
	rec := httptest.NewRecorder()

	h.itemList(rec, req)

	body := rec.Body.String()
	if !strings.Contains(body, "ラベル無し保存記事") {
		t.Fatalf("ブックマークビューはラベル0件でも保存済み記事を出すべき: %q", body)
	}
	if strings.Contains(body, "通常記事") {
		t.Fatalf("ブックマークビューに未保存記事を含めてはいけません: %q", body)
	}
}

// TestBookmarkRenameRoute リネームルートはツリーペインを再描画します。
func TestBookmarkRenameRoute(t *testing.T) {
	t.Parallel()
	subs := &stubSubscriptions{feeds: []domain.Feed{{ID: "f1", Title: "f1"}}}
	items := &stubItems{items: map[string][]domain.Item{}}
	h := newBookmarkTreeHandler(t, subs, items, []domain.Bookmark{{ID: "b1", Name: "旧名"}})
	form := url.Values{"name": {"新名"}}
	req := httptest.NewRequest(http.MethodPost, "/app/bookmarks/b1/rename", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.SetPathValue("id", "b1")
	req = withSession(req, Session{Username: "owner", CSRFToken: "tok"})
	rec := httptest.NewRecorder()

	h.bookmarkRename(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status got %d want %d", rec.Code, http.StatusOK)
	}
	if !strings.Contains(rec.Body.String(), `id="tree-pane"`) {
		t.Fatalf("リネーム後はツリーペインを再描画すべき: %q", rec.Body.String())
	}
}

// TestBookmarkDeleteRoute 削除ルートはツリーペインを再描画します。
func TestBookmarkDeleteRoute(t *testing.T) {
	t.Parallel()
	subs := &stubSubscriptions{feeds: []domain.Feed{{ID: "f1", Title: "f1"}}}
	items := &stubItems{items: map[string][]domain.Item{}}
	h := newBookmarkTreeHandler(t, subs, items, []domain.Bookmark{{ID: "b1", Name: "ArgoCD"}})
	req := httptest.NewRequest(http.MethodDelete, "/app/bookmarks/b1", nil)
	req.SetPathValue("id", "b1")
	req = withSession(req, Session{Username: "owner", CSRFToken: "tok"})
	rec := httptest.NewRecorder()

	h.bookmarkDelete(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status got %d want %d", rec.Code, http.StatusOK)
	}
	if !strings.Contains(rec.Body.String(), `id="tree-pane"`) {
		t.Fatalf("削除後はツリーペインを再描画すべき: %q", rec.Body.String())
	}
}
