package handler

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/okamyuji/feedflow-go-htmx/internal/domain"
)

// TestItemBookmarkSaveRendersPicker 保存(bookmarked=true)はピッカーを再描画し、解除ボタン(=保存済み状態)を出します。
// 「保存済み」のテキスト表示は廃止したため、保存状態はピッカーのボタン表示で確認します。
func TestItemBookmarkSaveRendersPicker(t *testing.T) {
	t.Parallel()
	subs := &stubSubscriptions{feeds: []domain.Feed{{ID: "f1", Title: "f1"}}}
	// SetBookmarked後に findItem で読み直すため、Bookmarked=true の記事を返すstubにします。
	items := &stubItems{items: map[string][]domain.Item{
		"f1": {{ID: "i1", FeedID: "f1", Title: "記事1", Bookmarked: true}},
	}}
	h := newAppHandler(t, subs, items)
	form := url.Values{"bookmarked": {"true"}, "surface": {"picker"}}
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
	if !strings.Contains(body, "ブックマーク解除") {
		t.Fatalf("保存時はピッカーに解除ボタン(=保存済み状態)を出すべき: %q", body)
	}
	if strings.Contains(body, "保存済み") {
		t.Fatalf("「保存済み」のテキスト表示は廃止したため出してはいけません: %q", body)
	}
}

// TestItemBookmarkUnsetRemovesCardInBookmarkView ブックマークビューでの解除はピッカー更新に加え、
// 当該記事カードを一覧から取り除くOOB断片(hx-swap-oob="delete")を返します。
func TestItemBookmarkUnsetRemovesCardInBookmarkView(t *testing.T) {
	t.Parallel()
	subs := &stubSubscriptions{feeds: []domain.Feed{{ID: "f1", Title: "f1"}}}
	items := &stubItems{items: map[string][]domain.Item{
		"f1": {{ID: "i1", FeedID: "f1", Title: "記事1"}},
	}}
	h := newAppHandler(t, subs, items)
	form := url.Values{"bookmarked": {"false"}, "surface": {"picker"}}
	req := httptest.NewRequest(http.MethodPost, "/app/items/f1/i1/bookmark", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("HX-Request", "true")
	// 現在表示中ページがブックマークビューであることをHX-Current-URLで伝えます。
	req.Header.Set("HX-Current-URL", "http://example.test/app/items?view=bookmark")
	req.SetPathValue("feedID", "f1")
	req.SetPathValue("itemID", "i1")
	req = withSession(req, Session{Username: "owner", CSRFToken: "tok"})
	rec := httptest.NewRecorder()

	h.itemBookmark(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status got %d want %d", rec.Code, http.StatusOK)
	}
	body := rec.Body.String()
	if !strings.Contains(body, `hx-swap-oob="delete"`) || !strings.Contains(body, `id="item-i1"`) {
		t.Fatalf("ブックマークビューでの解除は当該カードを除去するOOBを返すべき: %q", body)
	}
}

// TestItemBookmarkUnsetKeepsCardOutsideBookmarkView ブックマークビュー以外での解除はカード除去OOBを返しません。
func TestItemBookmarkUnsetKeepsCardOutsideBookmarkView(t *testing.T) {
	t.Parallel()
	subs := &stubSubscriptions{feeds: []domain.Feed{{ID: "f1", Title: "f1"}}}
	items := &stubItems{items: map[string][]domain.Item{
		"f1": {{ID: "i1", FeedID: "f1", Title: "記事1"}},
	}}
	h := newAppHandler(t, subs, items)
	form := url.Values{"bookmarked": {"false"}, "surface": {"picker"}}
	req := httptest.NewRequest(http.MethodPost, "/app/items/f1/i1/bookmark", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("HX-Request", "true")
	// 通常のフィード表示中はカードを除去しません。
	req.Header.Set("HX-Current-URL", "http://example.test/app/items?feed=f1")
	req.SetPathValue("feedID", "f1")
	req.SetPathValue("itemID", "i1")
	req = withSession(req, Session{Username: "owner", CSRFToken: "tok"})
	rec := httptest.NewRecorder()

	h.itemBookmark(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status got %d want %d", rec.Code, http.StatusOK)
	}
	if strings.Contains(rec.Body.String(), `hx-swap-oob="delete"`) {
		t.Fatalf("ブックマークビュー以外ではカード除去OOBを返してはいけません: %q", rec.Body.String())
	}
}

// TestItemCardSecondButtonIsReadInBookmarkView ブックマークビューでも記事カードの2つ目ボタンは既読トグルで、
// 記事内の「ブックマーク解除」ボタンは廃止済みです(ブックマークボタンの解除で代用)。
func TestItemCardSecondButtonIsReadInBookmarkView(t *testing.T) {
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
	if strings.Contains(body, "ブックマーク解除") {
		t.Fatalf("記事カードに解除ボタンを出してはいけません(廃止済み): %q", body)
	}
	if !strings.Contains(body, "/read") {
		t.Fatalf("ブックマークビューでもカードは既読トグルを出すべき: %q", body)
	}
}

// TestItemCardSecondButtonIsReadInNormalView 通常ビューでも2つ目ボタンは既読トグルです。
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
