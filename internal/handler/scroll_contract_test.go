package handler

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/okamyuji/feedflow-go-htmx/internal/domain"
)

// app.jsのスクロール保持(右ペインを最上部へ・左ペインの位置を維持)は、以下のDOM契約に依存します。
// idやOOB属性が変わるとJS側のセレクタが無言で壊れるため、サーバが返すマークアップを回帰防止としてロックします。
//   - 右ペインのスクロール対象 ……… id="main-pane"
//   - 左ペインのスクロールコンテナ …… id="tree-pane"
//   - フィード選択・購読解除時に左ペイン(tree-pane)が丸ごと差し替わること(=スクロールが戻る原因。JSで補正する)

// TestShellExposesScrollContainers フルページに右ペインと左ペインのスクロール対象/コンテナが存在することを保証します。
func TestShellExposesScrollContainers(t *testing.T) {
	t.Parallel()
	h := newFullHandler(t, true)
	srv := h.Routes()
	req := httptest.NewRequest(http.MethodGet, "/app", nil)
	rec := httptest.NewRecorder()

	srv.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status got %d want %d", rec.Code, http.StatusOK)
	}
	body := rec.Body.String()
	if !strings.Contains(body, `id="main-pane"`) {
		t.Fatalf("右ペインのスクロール対象 id=\"main-pane\" が存在すべきです: %q", body)
	}
	if !strings.Contains(body, `id="tree-pane"`) {
		t.Fatalf("左ペインのスクロールコンテナ id=\"tree-pane\" が存在すべきです: %q", body)
	}
}

// TestFeedSelectSwapsTreePaneOOB フィード選択(HTMXのitemList)が左ペインをOOBで丸ごと差し替えることを保証します。
// これがスクロールが先頭へ戻る原因であり、JSはこのスワップをまたいでscrollTopを復元します。
func TestFeedSelectSwapsTreePaneOOB(t *testing.T) {
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
	if !strings.Contains(body, `id="tree-pane"`) || !strings.Contains(body, `hx-swap-oob="true"`) {
		t.Fatalf("フィード選択は左ペインをOOBで差し替えるべきです(JSのスクロール復元の前提): %q", body)
	}
}

// TestUnsubscribeReplacesTreePane 購読解除が左ペイン(tree-pane)を差し替えることを保証します。
// 削除後もJSがscrollTopを復元できるよう、差し替え対象のidがtree-paneであることをロックします。
func TestUnsubscribeReplacesTreePane(t *testing.T) {
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
	if !strings.Contains(rec.Body.String(), `id="tree-pane"`) {
		t.Fatalf("購読解除は左ペイン id=\"tree-pane\" を差し替えるべきです: %q", rec.Body.String())
	}
}
