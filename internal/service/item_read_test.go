package service_test

import (
	"testing"
	"time"

	"github.com/okamyuji/feedflow-go-htmx/internal/domain"
	"github.com/okamyuji/feedflow-go-htmx/internal/service"
)

// newItemSvc テスト用にフェイク一式を組み、ミュートサービスを注入したItemServiceを返します。
// ListItems がミュート適用済みの記事を返す契約に合わせてMuteServiceを渡します。
func newItemSvc(repo *fakeRepo) *service.ItemService {
	deps := newDeps(repo, newFakeFetcher(), fakeParser{}, time.Now(), &fakeIDGen{})
	return service.NewItemService(deps, service.NewMuteService(deps))
}

func TestItemServiceListItems(t *testing.T) {
	t.Parallel()
	repo := newFakeRepo()
	_ = repo.SaveFeed(domain.Feed{ID: "fA"})
	_ = repo.SaveFeed(domain.Feed{ID: "fB"})
	_ = repo.SaveItems("fA", []domain.Item{{ID: "a1", FeedID: "fA", Title: "A1"}})
	_ = repo.SaveItems("fB", []domain.Item{{ID: "b1", FeedID: "fB", Title: "B1"}})
	svc := newItemSvc(repo)

	t.Run("単一フィードを返す", func(t *testing.T) {
		got, err := svc.ListItems("fA")
		if err != nil {
			t.Fatalf("ListItems returned error: %v", err)
		}
		if len(got) != 1 || got[0].ID != "a1" {
			t.Fatalf("ListItems(fA) got %+v", got)
		}
	})

	t.Run("空指定で全フィードを横断する", func(t *testing.T) {
		got, err := svc.ListItems("")
		if err != nil {
			t.Fatalf("ListItems returned error: %v", err)
		}
		if len(got) != 2 {
			t.Fatalf("ListItems(all) got %d items want 2", len(got))
		}
	})
}

func TestItemServiceMarkRead(t *testing.T) {
	t.Parallel()
	repo := newFakeRepo()
	_ = repo.SaveItems("fA", []domain.Item{
		{ID: "a1", FeedID: "fA", Read: false},
		{ID: "a2", FeedID: "fA", Read: false},
	})
	svc := newItemSvc(repo)

	if err := svc.MarkRead("fA", "a1", true); err != nil {
		t.Fatalf("MarkRead returned error: %v", err)
	}
	items, _ := repo.Items("fA")
	if !items[0].Read {
		t.Fatalf("a1 must be read")
	}
	if items[1].Read {
		t.Fatalf("a2 must stay unread")
	}
}

func TestItemServiceMarkReadNotFound(t *testing.T) {
	t.Parallel()
	repo := newFakeRepo()
	_ = repo.SaveItems("fA", []domain.Item{{ID: "a1", FeedID: "fA"}})
	svc := newItemSvc(repo)

	if err := svc.MarkRead("fA", "missing", true); err == nil {
		t.Fatalf("MarkRead must return error for missing item")
	}
}

func TestItemServiceMarkAllRead(t *testing.T) {
	t.Parallel()
	repo := newFakeRepo()
	_ = repo.SaveFeed(domain.Feed{ID: "fA"})
	_ = repo.SaveFeed(domain.Feed{ID: "fB"})
	_ = repo.SaveItems("fA", []domain.Item{{ID: "a1", FeedID: "fA"}, {ID: "a2", FeedID: "fA"}})
	_ = repo.SaveItems("fB", []domain.Item{{ID: "b1", FeedID: "fB"}})
	svc := newItemSvc(repo)

	t.Run("単一フィードを全既読にする", func(t *testing.T) {
		if err := svc.MarkAllRead("fA"); err != nil {
			t.Fatalf("MarkAllRead returned error: %v", err)
		}
		items, _ := repo.Items("fA")
		for _, it := range items {
			if !it.Read {
				t.Fatalf("item %s must be read", it.ID)
			}
		}
		items, _ = repo.Items("fB")
		if items[0].Read {
			t.Fatalf("fB must stay unread")
		}
	})

	t.Run("空指定で全フィードを全既読にする", func(t *testing.T) {
		if err := svc.MarkAllRead(""); err != nil {
			t.Fatalf("MarkAllRead returned error: %v", err)
		}
		items, _ := repo.Items("fB")
		if !items[0].Read {
			t.Fatalf("fB must be read after all-feeds mark")
		}
	})
}
