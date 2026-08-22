package service_test

import (
	"errors"
	"testing"

	"github.com/okamyuji/feedflow-go-htmx/internal/domain"
	"github.com/okamyuji/feedflow-go-htmx/internal/service"
)

// newItemSvcWithRepo テスト用にItemServiceとフェイクリポジトリを組んで返します。
func newItemSvcWithRepo() (*service.ItemService, *fakeRepo) {
	repo := newFakeRepo()
	return newItemSvc(repo), repo
}

func TestDeleteItemRemovesFromSavedPagesFeed(t *testing.T) {
	t.Parallel()
	svc, repo := newItemSvcWithRepo()
	if err := repo.SaveItems(domain.SavedPagesFeedID, []domain.Item{
		{ID: "i1", FeedID: domain.SavedPagesFeedID, Link: "https://example.com/1", Bookmarked: true},
		{ID: "i2", FeedID: domain.SavedPagesFeedID, Link: "https://example.com/2", Bookmarked: true},
	}); err != nil {
		t.Fatalf("SaveItems returned error: %v", err)
	}

	if err := svc.DeleteItem(domain.SavedPagesFeedID, "i1"); err != nil {
		t.Fatalf("DeleteItem returned error: %v", err)
	}

	got, err := repo.Items(domain.SavedPagesFeedID)
	if err != nil {
		t.Fatalf("Items returned error: %v", err)
	}
	if len(got) != 1 || got[0].ID != "i2" {
		t.Errorf("items after delete = %+v, want only i2", got)
	}
}

func TestDeleteItemRemovesLastItem(t *testing.T) {
	t.Parallel()
	svc, repo := newItemSvcWithRepo()
	if err := repo.SaveItems(domain.SavedPagesFeedID, []domain.Item{
		{ID: "i1", FeedID: domain.SavedPagesFeedID, Bookmarked: true},
	}); err != nil {
		t.Fatalf("SaveItems returned error: %v", err)
	}

	if err := svc.DeleteItem(domain.SavedPagesFeedID, "i1"); err != nil {
		t.Fatalf("DeleteItem returned error: %v", err)
	}

	got, err := repo.Items(domain.SavedPagesFeedID)
	if err != nil {
		t.Fatalf("Items returned error: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("items after delete = %+v, want empty", got)
	}
}

func TestDeleteItemRejectsNonSavedPagesFeed(t *testing.T) {
	t.Parallel()
	svc, repo := newItemSvcWithRepo()
	if err := repo.SaveItems("f1", []domain.Item{{ID: "i1", FeedID: "f1"}}); err != nil {
		t.Fatalf("SaveItems returned error: %v", err)
	}

	if err := svc.DeleteItem("f1", "i1"); !errors.Is(err, service.ErrNotSavedPagesFeed) {
		t.Fatalf("DeleteItem error = %v, want ErrNotSavedPagesFeed", err)
	}

	got, err := repo.Items("f1")
	if err != nil {
		t.Fatalf("Items returned error: %v", err)
	}
	if len(got) != 1 {
		t.Errorf("items were modified despite the rejection: %+v", got)
	}
}

func TestDeleteItemRejectsEmptyFeedID(t *testing.T) {
	t.Parallel()
	svc, _ := newItemSvcWithRepo()
	if err := svc.DeleteItem("", "i1"); !errors.Is(err, service.ErrNotSavedPagesFeed) {
		t.Errorf("DeleteItem error = %v, want ErrNotSavedPagesFeed", err)
	}
}

func TestDeleteItemReturnsNotFoundForUnknownItem(t *testing.T) {
	t.Parallel()
	svc, repo := newItemSvcWithRepo()
	if err := repo.SaveItems(domain.SavedPagesFeedID, []domain.Item{
		{ID: "i1", FeedID: domain.SavedPagesFeedID},
	}); err != nil {
		t.Fatalf("SaveItems returned error: %v", err)
	}

	if err := svc.DeleteItem(domain.SavedPagesFeedID, "missing"); !errors.Is(err, service.ErrItemNotFound) {
		t.Fatalf("DeleteItem error = %v, want ErrItemNotFound", err)
	}
}

func TestDeleteItemPropagatesLoadError(t *testing.T) {
	t.Parallel()
	svc, repo := newItemSvcWithRepo()
	loadErr := errors.New("boom")
	repo.failOn["Items"] = loadErr

	if err := svc.DeleteItem(domain.SavedPagesFeedID, "i1"); !errors.Is(err, loadErr) {
		t.Errorf("DeleteItem error = %v, want the load error", err)
	}
}

func TestDeleteItemPropagatesSaveError(t *testing.T) {
	t.Parallel()
	svc, repo := newItemSvcWithRepo()
	if err := repo.SaveItems(domain.SavedPagesFeedID, []domain.Item{
		{ID: "i1", FeedID: domain.SavedPagesFeedID},
	}); err != nil {
		t.Fatalf("SaveItems returned error: %v", err)
	}
	saveErr := errors.New("disk full")
	repo.failOn["SaveItems"] = saveErr

	if err := svc.DeleteItem(domain.SavedPagesFeedID, "i1"); !errors.Is(err, saveErr) {
		t.Errorf("DeleteItem error = %v, want the save error", err)
	}
}
