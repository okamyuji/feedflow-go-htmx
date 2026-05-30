package service_test

import (
	"errors"
	"testing"
	"time"

	"github.com/okamyuji/feedflow-go-htmx/internal/domain"
	"github.com/okamyuji/feedflow-go-htmx/internal/port"
	"github.com/okamyuji/feedflow-go-htmx/internal/service"
)

var _ port.BookmarkService = (*service.BookmarkService)(nil)

func newBookmarkSvc(item domain.Item) (*service.BookmarkService, *fakeRepo) {
	repo := newFakeRepo()
	if item.ID != "" {
		_ = repo.SaveItems(item.FeedID, []domain.Item{item})
	}
	deps := newDeps(repo, newFakeFetcher(), fakeParser{}, time.Now(), &fakeIDGen{})
	items := service.NewItemService(deps, service.NewMuteService(deps))
	return service.NewBookmarkService(deps, items), repo
}

func TestBookmarkCreateDedupesByName(t *testing.T) {
	t.Parallel()
	svc, _ := newBookmarkSvc(domain.Item{})
	b1, err := svc.Create("読み物")
	if err != nil {
		t.Fatalf("Create returned error: %v", err)
	}
	b2, err := svc.Create("読み物")
	if err != nil {
		t.Fatalf("Create returned error: %v", err)
	}
	if b1.ID != b2.ID {
		t.Fatalf("同名の作成は既存を返すべき got %q and %q", b1.ID, b2.ID)
	}
}

func TestBookmarkCreateRejectsEmpty(t *testing.T) {
	t.Parallel()
	svc, _ := newBookmarkSvc(domain.Item{})
	if _, err := svc.Create("   "); !errors.Is(err, service.ErrBookmarkNameRequired) {
		t.Fatalf("空名は ErrBookmarkNameRequired を返すべき got %v", err)
	}
}

func TestBookmarkToggleAddsAndRemoves(t *testing.T) {
	t.Parallel()
	svc, repo := newBookmarkSvc(domain.Item{ID: "a1", FeedID: "fA"})
	bm, err := svc.Create("あとで実装する")
	if err != nil {
		t.Fatalf("Create returned error: %v", err)
	}
	if err := svc.Toggle("fA", "a1", bm.ID); err != nil {
		t.Fatalf("Toggle add returned error: %v", err)
	}
	if got := first(repo, "fA").BookmarkIDs; len(got) != 1 || got[0] != bm.ID {
		t.Fatalf("Toggle should add bookmark got %+v", got)
	}
	if err := svc.Toggle("fA", "a1", bm.ID); err != nil {
		t.Fatalf("Toggle remove returned error: %v", err)
	}
	if got := first(repo, "fA").BookmarkIDs; len(got) != 0 {
		t.Fatalf("Toggle should remove bookmark got %+v", got)
	}
}

func TestBookmarkCreateAndAdd(t *testing.T) {
	t.Parallel()
	svc, repo := newBookmarkSvc(domain.Item{ID: "a1", FeedID: "fA"})
	bm, err := svc.CreateAndAdd("fA", "a1", "Go の知見")
	if err != nil {
		t.Fatalf("CreateAndAdd returned error: %v", err)
	}
	got := first(repo, "fA").BookmarkIDs
	if len(got) != 1 || got[0] != bm.ID {
		t.Fatalf("CreateAndAdd should add the bookmark got %+v", got)
	}
}
