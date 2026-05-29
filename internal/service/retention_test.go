package service_test

import (
	"testing"
	"time"

	"github.com/okamyuji/feedflow-go-htmx/internal/domain"
	"github.com/okamyuji/feedflow-go-htmx/internal/port"
	"github.com/okamyuji/feedflow-go-htmx/internal/service"
)

var _ port.RetentionService = (*service.RetentionService)(nil)

func TestRetentionServiceApplyFeed(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 5, 29, 12, 0, 0, 0, time.UTC)
	old := now.AddDate(0, 0, -40) // M=30を超過します
	recent := now.AddDate(0, 0, -5)

	repo := newFakeRepo()
	settings := domain.DefaultSettings()
	settings.MaxItems = 3
	settings.ReadRetentionDays = 30
	repo.settings = settings
	_ = repo.SaveFeed(domain.Feed{ID: "fA"})

	// 公開日時の新しい順に並ぶよう日時をずらします
	items := []domain.Item{
		{ID: "keep-recent-unread", FeedID: "fA", Read: false, PublishedAt: now.Add(-1 * time.Hour), FetchedAt: recent},
		{ID: "keep-recent-read", FeedID: "fA", Read: true, PublishedAt: now.Add(-2 * time.Hour), FetchedAt: recent},
		{ID: "drop-old-read", FeedID: "fA", Read: true, PublishedAt: now.Add(-3 * time.Hour), FetchedAt: old},
		{ID: "drop-over-limit", FeedID: "fA", Read: false, PublishedAt: now.Add(-4 * time.Hour), FetchedAt: recent},
		{ID: "keep-actioned", FeedID: "fA", Read: true, Starred: true, PublishedAt: now.Add(-5 * time.Hour), FetchedAt: old},
	}
	_ = repo.SaveItems("fA", items)

	svc := service.NewRetentionService(newDeps(repo, newFakeFetcher(), fakeParser{}, now, &fakeIDGen{}))

	removed, err := svc.ApplyFeed("fA")
	if err != nil {
		t.Fatalf("ApplyFeed returned error: %v", err)
	}
	// drop-old-readは既読かつM日超過で削除、drop-over-limitは順位3でN=3上限超過で削除
	if removed != 2 {
		t.Fatalf("removed got %d want 2", removed)
	}
	got, _ := repo.Items("fA")
	kept := map[string]bool{}
	for _, it := range got {
		kept[it.ID] = true
	}
	for _, id := range []string{"keep-recent-unread", "keep-recent-read", "keep-actioned"} {
		if !kept[id] {
			t.Fatalf("item %s must be kept", id)
		}
	}
	for _, id := range []string{"drop-old-read", "drop-over-limit"} {
		if kept[id] {
			t.Fatalf("item %s must be removed", id)
		}
	}
}

func TestRetentionServiceApplyAll(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 5, 29, 12, 0, 0, 0, time.UTC)
	old := now.AddDate(0, 0, -40)

	repo := newFakeRepo()
	settings := domain.DefaultSettings()
	settings.MaxItems = 100
	settings.ReadRetentionDays = 30
	repo.settings = settings
	_ = repo.SaveFeed(domain.Feed{ID: "fA"})
	_ = repo.SaveFeed(domain.Feed{ID: "fB"})
	_ = repo.SaveItems("fA", []domain.Item{
		{ID: "a-old", FeedID: "fA", Read: true, PublishedAt: now, FetchedAt: old},
	})
	_ = repo.SaveItems("fB", []domain.Item{
		{ID: "b-old", FeedID: "fB", Read: true, PublishedAt: now, FetchedAt: old},
		{ID: "b-keep", FeedID: "fB", Read: false, PublishedAt: now, FetchedAt: now},
	})

	svc := service.NewRetentionService(newDeps(repo, newFakeFetcher(), fakeParser{}, now, &fakeIDGen{}))

	removed, err := svc.Apply()
	if err != nil {
		t.Fatalf("Apply returned error: %v", err)
	}
	if removed != 2 {
		t.Fatalf("Apply removed got %d want 2", removed)
	}
	a, _ := repo.Items("fA")
	if len(a) != 0 {
		t.Fatalf("fA must be empty, got %d", len(a))
	}
	b, _ := repo.Items("fB")
	if len(b) != 1 || b[0].ID != "b-keep" {
		t.Fatalf("fB must keep only b-keep, got %+v", b)
	}
}

func TestRetentionServiceApplyFeedNoDelete(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 5, 29, 12, 0, 0, 0, time.UTC)
	repo := newFakeRepo()
	settings := domain.DefaultSettings()
	repo.settings = settings
	_ = repo.SaveFeed(domain.Feed{ID: "fA"})
	_ = repo.SaveItems("fA", []domain.Item{
		{ID: "a1", FeedID: "fA", Read: false, PublishedAt: now, FetchedAt: now},
	})
	svc := service.NewRetentionService(newDeps(repo, newFakeFetcher(), fakeParser{}, now, &fakeIDGen{}))

	removed, err := svc.ApplyFeed("fA")
	if err != nil {
		t.Fatalf("ApplyFeed returned error: %v", err)
	}
	if removed != 0 {
		t.Fatalf("removed got %d want 0", removed)
	}
}
