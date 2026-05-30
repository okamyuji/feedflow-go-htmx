package service_test

import (
	"testing"
	"time"

	"github.com/okamyuji/feedflow-go-htmx/internal/domain"
	"github.com/okamyuji/feedflow-go-htmx/internal/port"
	"github.com/okamyuji/feedflow-go-htmx/internal/service"
)

var _ port.ItemService = (*service.ItemService)(nil)

func newItemSvcWith(item domain.Item) (*service.ItemService, *fakeRepo) {
	repo := newFakeRepo()
	_ = repo.SaveItems(item.FeedID, []domain.Item{item})
	deps := newDeps(repo, newFakeFetcher(), fakeParser{}, time.Now(), &fakeIDGen{})
	svc := service.NewItemService(deps, service.NewMuteService(deps))
	return svc, repo
}

func first(repo *fakeRepo, feedID string) domain.Item {
	items, _ := repo.Items(feedID)
	return items[0]
}

func TestItemServiceReadLater(t *testing.T) {
	t.Parallel()
	svc, repo := newItemSvcWith(domain.Item{ID: "a1", FeedID: "fA"})
	if err := svc.ReadLater("fA", "a1", true); err != nil {
		t.Fatalf("ReadLater returned error: %v", err)
	}
	if !first(repo, "fA").ReadLater {
		t.Fatalf("item must be read-later")
	}
}

func TestItemServiceSetTags(t *testing.T) {
	t.Parallel()
	svc, repo := newItemSvcWith(domain.Item{ID: "a1", FeedID: "fA", Tags: []string{"old"}})
	if err := svc.SetTags("fA", "a1", []string{"go", "rss"}); err != nil {
		t.Fatalf("SetTags returned error: %v", err)
	}
	got := first(repo, "fA").Tags
	if len(got) != 2 || got[0] != "go" || got[1] != "rss" {
		t.Fatalf("SetTags got %+v want [go rss]", got)
	}
}

func TestItemServiceSetBookmarks(t *testing.T) {
	t.Parallel()
	svc, repo := newItemSvcWith(domain.Item{ID: "a1", FeedID: "fA"})
	if err := svc.SetBookmarks("fA", "a1", []string{"b1"}); err != nil {
		t.Fatalf("SetBookmarks returned error: %v", err)
	}
	got := first(repo, "fA").BookmarkIDs
	if len(got) != 1 || got[0] != "b1" {
		t.Fatalf("SetBookmarks got %+v want [b1]", got)
	}
}

func TestItemServiceSetNote(t *testing.T) {
	t.Parallel()
	svc, repo := newItemSvcWith(domain.Item{ID: "a1", FeedID: "fA"})
	if err := svc.SetNote("fA", "a1", "あとで確認します"); err != nil {
		t.Fatalf("SetNote returned error: %v", err)
	}
	if first(repo, "fA").Note != "あとで確認します" {
		t.Fatalf("SetNote did not persist note")
	}
}

func TestItemServiceAddHighlight(t *testing.T) {
	t.Parallel()
	svc, repo := newItemSvcWith(domain.Item{ID: "a1", FeedID: "fA", Highlights: []string{"一文目"}})
	if err := svc.AddHighlight("fA", "a1", "二文目"); err != nil {
		t.Fatalf("AddHighlight returned error: %v", err)
	}
	got := first(repo, "fA").Highlights
	if len(got) != 2 || got[1] != "二文目" {
		t.Fatalf("AddHighlight got %+v", got)
	}
}

func TestItemServiceActionNotFound(t *testing.T) {
	t.Parallel()
	svc, _ := newItemSvcWith(domain.Item{ID: "a1", FeedID: "fA"})
	if err := svc.SetBookmarks("fA", "missing", []string{"b1"}); err == nil {
		t.Fatalf("SetBookmarks must return error for missing item")
	}
}
