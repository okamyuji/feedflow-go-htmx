package handler

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/okamyuji/feedflow-go-htmx/internal/domain"
)

// bookmarkExclusionItems 未読・既読・ブックマーク所属の記事を1件ずつ持つテストデータを返します。
// ブックマーク済みの記事は未読(read=false)にして、未読ストリームから外れることを検証できるようにします。
func bookmarkExclusionItems() map[string][]domain.Item {
	return map[string][]domain.Item{
		"f1": {
			{ID: "unread", FeedID: "f1", Title: "未読記事"},
			{ID: "read", FeedID: "f1", Title: "既読記事", Read: true},
			{ID: "marked", FeedID: "f1", Title: "ブックマーク記事", BookmarkIDs: []string{"b1"}},
		},
	}
}

// TestItemListSingleFeedExcludesBookmarked 単一フィードの既定表示でもブックマーク済みを除外します。
// 未読バッジ(unreadByFeed)がブックマーク済みを数えないため、一覧側も除外して件数の整合を保ちます。
func TestItemListSingleFeedExcludesBookmarked(t *testing.T) {
	t.Parallel()
	subs := &stubSubscriptions{feeds: []domain.Feed{{ID: "f1", Title: "f1"}}}
	items := map[string][]domain.Item{
		"f1": {
			{ID: "unread", FeedID: "f1", Title: "未読記事"},
			{ID: "marked", FeedID: "f1", Title: "ブックマーク記事", BookmarkIDs: []string{"b1"}},
		},
	}
	h := newAppHandler(t, subs, &stubItems{items: items})
	req := httptest.NewRequest(http.MethodGet, "/app/items?feed=f1", nil)
	req.Header.Set("HX-Request", "true")
	req = withSession(req, Session{Username: "owner", CSRFToken: "tok"})
	rec := httptest.NewRecorder()

	h.itemList(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status got %d want %d", rec.Code, http.StatusOK)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "未読記事") {
		t.Fatalf("単一フィードは未読記事を表示すべきです: %q", body)
	}
	if strings.Contains(body, "ブックマーク記事") {
		t.Fatalf("単一フィードはブックマーク済み記事を含めてはいけません: %q", body)
	}
}

// TestItemListAllViewExcludesBookmarked すべて(未読ストリーム)はブックマーク済みを保管済みとして除外します。
func TestItemListAllViewExcludesBookmarked(t *testing.T) {
	t.Parallel()
	subs := &stubSubscriptions{feeds: []domain.Feed{{ID: "f1", Title: "f1"}}}
	h := newAppHandler(t, subs, &stubItems{items: bookmarkExclusionItems()})
	req := httptest.NewRequest(http.MethodGet, "/app/items", nil)
	req.Header.Set("HX-Request", "true")
	req = withSession(req, Session{Username: "owner", CSRFToken: "tok"})
	rec := httptest.NewRecorder()

	h.itemList(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status got %d want %d", rec.Code, http.StatusOK)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "未読記事") {
		t.Fatalf("すべては未読記事を表示すべきです: %q", body)
	}
	if strings.Contains(body, "ブックマーク記事") {
		t.Fatalf("すべてはブックマーク済み記事を含めてはいけません: %q", body)
	}
	if strings.Contains(body, "既読記事") {
		t.Fatalf("すべては既読記事を含めてはいけません: %q", body)
	}
}

// TestItemListReadViewExcludesBookmarked 既読ビューはブックマーク済みを除外します。
func TestItemListReadViewExcludesBookmarked(t *testing.T) {
	t.Parallel()
	subs := &stubSubscriptions{feeds: []domain.Feed{{ID: "f1", Title: "f1"}}}
	items := map[string][]domain.Item{
		"f1": {
			{ID: "read", FeedID: "f1", Title: "既読記事", Read: true},
			{ID: "markedread", FeedID: "f1", Title: "既読かつブックマーク記事", Read: true, BookmarkIDs: []string{"b1"}},
		},
	}
	h := newAppHandler(t, subs, &stubItems{items: items})
	req := httptest.NewRequest(http.MethodGet, "/app/items?view=read", nil)
	req.Header.Set("HX-Request", "true")
	req = withSession(req, Session{Username: "owner", CSRFToken: "tok"})
	rec := httptest.NewRecorder()

	h.itemList(rec, req)

	body := rec.Body.String()
	if !strings.Contains(body, "既読記事") {
		t.Fatalf("既読ビューは既読記事を表示すべきです: %q", body)
	}
	if strings.Contains(body, "既読かつブックマーク記事") {
		t.Fatalf("既読ビューはブックマーク済みを除外すべきです: %q", body)
	}
}

// TestItemListBookmarkViewStillShowsBookmarked ブックマークビューでは引き続きブックマーク済みを表示します。
func TestItemListBookmarkViewStillShowsBookmarked(t *testing.T) {
	t.Parallel()
	subs := &stubSubscriptions{feeds: []domain.Feed{{ID: "f1", Title: "f1"}}}
	h := newAppHandler(t, subs, &stubItems{items: bookmarkExclusionItems()})
	req := httptest.NewRequest(http.MethodGet, "/app/items?bookmark=b1", nil)
	req.Header.Set("HX-Request", "true")
	req = withSession(req, Session{Username: "owner", CSRFToken: "tok"})
	rec := httptest.NewRecorder()

	h.itemList(rec, req)

	body := rec.Body.String()
	if !strings.Contains(body, "ブックマーク記事") {
		t.Fatalf("ブックマークビューはブックマーク所属記事を表示すべきです: %q", body)
	}
}

// TestBuildTreeExcludesBookmarkedFromUnread 未読カウントはブックマーク済みを除外します。
func TestBuildTreeExcludesBookmarkedFromUnread(t *testing.T) {
	t.Parallel()
	subs := &stubSubscriptions{feeds: []domain.Feed{{ID: "f1", Title: "f1"}}}
	items := &stubItems{items: map[string][]domain.Item{
		"f1": {
			{ID: "u1", FeedID: "f1"},                              // 未読
			{ID: "u2", FeedID: "f1"},                              // 未読
			{ID: "m1", FeedID: "f1", BookmarkIDs: []string{"b1"}}, // 未読だがブックマーク済み→カウント外
		},
	}}
	h := newAppHandler(t, subs, items)

	nodes, err := h.buildTree()
	if err != nil {
		t.Fatalf("buildTree returned error: %v", err)
	}
	var all, feed feedTreeNode
	for _, n := range nodes {
		switch n.Kind {
		case "all":
			all = n
		case "feed":
			feed = n
		}
	}
	if all.UnreadCount != 2 {
		t.Fatalf("すべての未読数はブックマーク済みを除いた2であるべきです、got %d", all.UnreadCount)
	}
	if feed.UnreadCount != 2 {
		t.Fatalf("フィードの未読バッジはブックマーク済みを除いた2であるべきです、got %d", feed.UnreadCount)
	}
}
