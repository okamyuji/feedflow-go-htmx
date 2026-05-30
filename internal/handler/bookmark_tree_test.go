package handler

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/okamyuji/feedflow-go-htmx/internal/domain"
)

// newBookmarkTreeHandler フィードとブックマーク一覧を差し込んだHandlerを構築します。
// ツリー描画とブックマークビューの検証に使うため、購読フィードとブックマークの両方を注入します。
func newBookmarkTreeHandler(t *testing.T, subs *stubSubscriptions, items *stubItems, bms []domain.Bookmark) *Handler {
	t.Helper()
	h, err := New(Deps{
		Subscriptions:     subs,
		Items:             items,
		Bookmarks:         &stubBookmarks{list: bms},
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

// TestBookmarkChildSelectedKeepsParentOpen 子(名称コレクション)を選んだとき、親ブックマークが開いた状態で描画されることを保証します。
// ツリーはOOB再生成されるため、x-dataのopen初期値が子の選択を反映しないと折りたたまれてしまいます。
func TestBookmarkChildSelectedKeepsParentOpen(t *testing.T) {
	t.Parallel()
	subs := &stubSubscriptions{feeds: []domain.Feed{{ID: "f1", Title: "f1"}}}
	items := &stubItems{items: map[string][]domain.Item{
		"f1": {{ID: "i1", FeedID: "f1", Title: "記事1", BookmarkIDs: []string{"b1"}}},
	}}
	h := newBookmarkTreeHandler(t, subs, items, []domain.Bookmark{{ID: "b1", Name: "ArgoCD"}})
	req := httptest.NewRequest(http.MethodGet, "/app/items?bookmark=b1", nil)
	req.Header.Set("HX-Request", "true")
	req = withSession(req, Session{Username: "owner", CSRFToken: "tok"})
	rec := httptest.NewRecorder()

	h.itemList(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status got %d want %d", rec.Code, http.StatusOK)
	}
	body := rec.Body.String()
	if !strings.Contains(body, `x-data="{ open: true }"`) {
		t.Fatalf("子選択時は親ブックマークを開いた状態で描画すべきです: %q", body)
	}
}

// TestBookmarkChildIsNotMarkedActiveWhenParentSelected 親(view=bookmark)選択時に子ノードへtree-activeが付かないことを保証します。
// 子へtree-activeが付くと、選択していない子まで強調されます(子の強調はCSSの直接結合子で親選択時に巻き込まないようにしています)。
func TestBookmarkChildIsNotMarkedActiveWhenParentSelected(t *testing.T) {
	t.Parallel()
	subs := &stubSubscriptions{feeds: []domain.Feed{{ID: "f1", Title: "f1"}}}
	items := &stubItems{items: map[string][]domain.Item{
		"f1": {{ID: "i1", FeedID: "f1", BookmarkIDs: []string{"b1"}}},
	}}
	h := newBookmarkTreeHandler(t, subs, items, []domain.Bookmark{{ID: "b1", Name: "ArgoCD"}})
	req := httptest.NewRequest(http.MethodGet, "/app/items?view=bookmark", nil)
	req.Header.Set("HX-Request", "true")
	req = withSession(req, Session{Username: "owner", CSRFToken: "tok"})
	rec := httptest.NewRecorder()

	h.itemList(rec, req)

	body := rec.Body.String()
	// 子ノードのli(tree-bookmark-item)にtree-activeが付いていないこと。
	if strings.Contains(body, "tree-bookmark-item tree-active") {
		t.Fatalf("親選択時に子ノードへtree-activeを付けてはいけません: %q", body)
	}
}

// TestItemCardWithoutContentOpensOriginalDirectly 本文の無い記事(外部リンク)はオーバーレイを開かず元記事を新規タブで開くことを保証します。
func TestItemCardWithoutContentOpensOriginalDirectly(t *testing.T) {
	t.Parallel()
	subs := &stubSubscriptions{feeds: []domain.Feed{{ID: "f1", Title: "f1"}}}
	items := &stubItems{items: map[string][]domain.Item{
		"f1": {
			{ID: "ext", FeedID: "f1", Title: "外部リンク", Link: "https://example.com/a", Content: "", BookmarkIDs: []string{"b1"}},
		},
	}}
	h := newBookmarkTreeHandler(t, subs, items, []domain.Bookmark{{ID: "b1", Name: "ArgoCD"}})
	req := httptest.NewRequest(http.MethodGet, "/app/items?bookmark=b1", nil)
	req.Header.Set("HX-Request", "true")
	req = withSession(req, Session{Username: "owner", CSRFToken: "tok"})
	rec := httptest.NewRecorder()

	h.itemList(rec, req)

	body := rec.Body.String()
	if !strings.Contains(body, `target="_blank"`) {
		t.Fatalf("本文の無い記事は元記事を新規タブで開くべきです: %q", body)
	}
	if strings.Contains(body, `hx-get="/app/items/f1/ext"`) {
		t.Fatalf("本文の無い記事はオーバーレイ取得のhx-getを出してはいけません: %q", body)
	}
}

// TestItemCardWithContentOpensOverlay 本文のある記事は従来どおりオーバーレイを開くことを保証します。
func TestItemCardWithContentOpensOverlay(t *testing.T) {
	t.Parallel()
	subs := &stubSubscriptions{feeds: []domain.Feed{{ID: "f1", Title: "f1"}}}
	items := &stubItems{items: map[string][]domain.Item{
		"f1": {
			{ID: "rich", FeedID: "f1", Title: "本文あり", Link: "https://example.com/b", Content: "<p>本文</p>"},
		},
	}}
	h := newBookmarkTreeHandler(t, subs, items, nil)
	req := httptest.NewRequest(http.MethodGet, "/app/items?feed=f1", nil)
	req.Header.Set("HX-Request", "true")
	req = withSession(req, Session{Username: "owner", CSRFToken: "tok"})
	rec := httptest.NewRecorder()

	h.itemList(rec, req)

	body := rec.Body.String()
	if !strings.Contains(body, `hx-get="/app/items/f1/rich"`) {
		t.Fatalf("本文のある記事はオーバーレイ取得のhx-getを出すべきです: %q", body)
	}
}
