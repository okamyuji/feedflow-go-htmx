package store_test

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/okamyuji/feedflow-go-htmx/internal/domain"
	"github.com/okamyuji/feedflow-go-htmx/internal/port"
	"github.com/okamyuji/feedflow-go-htmx/internal/store"
)

// assertRepository Storeがport.Repositoryを満たすことをコンパイル時に確認します。
var _ port.Repository = (*store.Store)(nil)

func TestNewCreatesDataDirAndItemsDir(t *testing.T) {
	t.Parallel()

	root := filepath.Join(t.TempDir(), "data")
	s, err := store.New(root)
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}
	if s == nil {
		t.Fatal("New returned nil Store")
	}

	if info, err := os.Stat(root); err != nil || !info.IsDir() {
		t.Fatalf("data dir not created: stat err %v", err)
	}
	if info, err := os.Stat(filepath.Join(root, "items")); err != nil || !info.IsDir() {
		t.Fatalf("items dir not created: stat err %v", err)
	}
}

func TestNewOnEmptyDirReturnsDefaults(t *testing.T) {
	t.Parallel()

	s, err := store.New(filepath.Join(t.TempDir(), "data"))
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}

	feeds, err := s.Feeds()
	if err != nil {
		t.Fatalf("Feeds returned error: %v", err)
	}
	if len(feeds) != 0 {
		t.Fatalf("feeds got %d want 0 on empty dir", len(feeds))
	}

	settings, err := s.Settings()
	if err != nil {
		t.Fatalf("Settings returned error: %v", err)
	}
	if settings != domain.DefaultSettings() {
		t.Fatalf("settings got %+v want DefaultSettings on empty dir", settings)
	}

	user, err := s.User()
	if err != nil {
		t.Fatalf("User returned error: %v", err)
	}
	if user.IsRegistered() {
		t.Fatalf("user got registered %+v want zero value on empty dir", user)
	}
}

func TestNewLoadsExistingFeeds(t *testing.T) {
	t.Parallel()

	root := filepath.Join(t.TempDir(), "data")

	// 1つ目のStoreで1件保存し、2つ目のStoreでロードできることを確認します。
	s1, err := store.New(root)
	if err != nil {
		t.Fatalf("first New returned error: %v", err)
	}
	want := domain.Feed{ID: "f1", FeedURL: "https://example.com/rss", Title: "Example"}
	if err := s1.SaveFeed(want); err != nil {
		t.Fatalf("SaveFeed returned error: %v", err)
	}

	s2, err := store.New(root)
	if err != nil {
		t.Fatalf("second New returned error: %v", err)
	}
	got, err := s2.Feed("f1")
	if err != nil {
		t.Fatalf("Feed returned error: %v", err)
	}
	if got.FeedURL != want.FeedURL || got.Title != want.Title {
		t.Fatalf("loaded feed got %+v want %+v", got, want)
	}
}

func TestSaveFeedAddsAndUpdates(t *testing.T) {
	t.Parallel()

	s, err := store.New(filepath.Join(t.TempDir(), "data"))
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}

	if err := s.SaveFeed(domain.Feed{ID: "f1", Title: "First"}); err != nil {
		t.Fatalf("SaveFeed add returned error: %v", err)
	}
	if err := s.SaveFeed(domain.Feed{ID: "f2", Title: "Second"}); err != nil {
		t.Fatalf("SaveFeed add returned error: %v", err)
	}
	if err := s.SaveFeed(domain.Feed{ID: "f1", Title: "First Updated"}); err != nil {
		t.Fatalf("SaveFeed update returned error: %v", err)
	}

	feeds, err := s.Feeds()
	if err != nil {
		t.Fatalf("Feeds returned error: %v", err)
	}
	if len(feeds) != 2 {
		t.Fatalf("feeds got %d want 2 (update must not duplicate)", len(feeds))
	}

	got, err := s.Feed("f1")
	if err != nil {
		t.Fatalf("Feed returned error: %v", err)
	}
	if got.Title != "First Updated" {
		t.Fatalf("feed title got %q want First Updated", got.Title)
	}
}

func TestFeedNotFound(t *testing.T) {
	t.Parallel()

	s, err := store.New(filepath.Join(t.TempDir(), "data"))
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}

	_, err = s.Feed("missing")
	if !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("Feed error got %v want errors.Is ErrNotFound", err)
	}
}

func TestFeedsReturnsCopy(t *testing.T) {
	t.Parallel()

	s, err := store.New(filepath.Join(t.TempDir(), "data"))
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}
	if err := s.SaveFeed(domain.Feed{ID: "f1", Title: "Original"}); err != nil {
		t.Fatalf("SaveFeed returned error: %v", err)
	}

	feeds, err := s.Feeds()
	if err != nil {
		t.Fatalf("Feeds returned error: %v", err)
	}
	feeds[0].Title = "Mutated"

	got, err := s.Feed("f1")
	if err != nil {
		t.Fatalf("Feed returned error: %v", err)
	}
	if got.Title != "Original" {
		t.Fatalf("internal state mutated via returned slice: got %q want Original", got.Title)
	}
}

func TestDeleteFeedRemovesFeedAndItems(t *testing.T) {
	t.Parallel()

	root := filepath.Join(t.TempDir(), "data")
	s, err := store.New(root)
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}
	if err := s.SaveFeed(domain.Feed{ID: "f1", Title: "First"}); err != nil {
		t.Fatalf("SaveFeed returned error: %v", err)
	}
	if err := s.SaveItems("f1", []domain.Item{{ID: "i1", FeedID: "f1", Title: "Article"}}); err != nil {
		t.Fatalf("SaveItems returned error: %v", err)
	}

	if err := s.DeleteFeed("f1"); err != nil {
		t.Fatalf("DeleteFeed returned error: %v", err)
	}

	if _, err := s.Feed("f1"); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("Feed after delete error got %v want ErrNotFound", err)
	}
	items, err := s.Items("f1")
	if err != nil {
		t.Fatalf("Items returned error: %v", err)
	}
	if len(items) != 0 {
		t.Fatalf("items after delete got %d want 0", len(items))
	}
	if _, statErr := os.Stat(filepath.Join(root, "items", "f1.json")); !os.IsNotExist(statErr) {
		t.Fatalf("items file still present after DeleteFeed: stat err %v", statErr)
	}
}

func TestDeleteFeedMissingReturnsNotFound(t *testing.T) {
	t.Parallel()

	s, err := store.New(filepath.Join(t.TempDir(), "data"))
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}
	if err := s.DeleteFeed("missing"); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("DeleteFeed error got %v want ErrNotFound", err)
	}
}

func TestSaveAndDeleteCategory(t *testing.T) {
	t.Parallel()

	s, err := store.New(filepath.Join(t.TempDir(), "data"))
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}
	if err := s.SaveCategory(domain.Category{ID: "c1", Name: "Tech", Order: 1}); err != nil {
		t.Fatalf("SaveCategory returned error: %v", err)
	}
	if err := s.SaveCategory(domain.Category{ID: "c1", Name: "Technology", Order: 1}); err != nil {
		t.Fatalf("SaveCategory update returned error: %v", err)
	}

	cats, err := s.Categories()
	if err != nil {
		t.Fatalf("Categories returned error: %v", err)
	}
	if len(cats) != 1 || cats[0].Name != "Technology" {
		t.Fatalf("categories got %+v want one Technology", cats)
	}

	if err := s.DeleteCategory("c1"); err != nil {
		t.Fatalf("DeleteCategory returned error: %v", err)
	}
	cats, err = s.Categories()
	if err != nil {
		t.Fatalf("Categories returned error: %v", err)
	}
	if len(cats) != 0 {
		t.Fatalf("categories after delete got %d want 0", len(cats))
	}
}

func TestDeleteCategoryMissingReturnsNotFound(t *testing.T) {
	t.Parallel()

	s, err := store.New(filepath.Join(t.TempDir(), "data"))
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}
	if err := s.DeleteCategory("missing"); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("DeleteCategory error got %v want ErrNotFound", err)
	}
}

func TestSaveItemsAndItems(t *testing.T) {
	t.Parallel()

	root := filepath.Join(t.TempDir(), "data")
	s, err := store.New(root)
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}

	want := []domain.Item{
		{ID: "i1", FeedID: "f1", Title: "First"},
		{ID: "i2", FeedID: "f1", Title: "Second"},
	}
	if err := s.SaveItems("f1", want); err != nil {
		t.Fatalf("SaveItems returned error: %v", err)
	}

	got, err := s.Items("f1")
	if err != nil {
		t.Fatalf("Items returned error: %v", err)
	}
	if len(got) != 2 || got[0].ID != "i1" || got[1].ID != "i2" {
		t.Fatalf("items got %+v want i1,i2", got)
	}

	// ファイルがitemsディレクトリに作られていることを確認します。
	if _, statErr := os.Stat(filepath.Join(root, "items", "f1.json")); statErr != nil {
		t.Fatalf("items file not created: %v", statErr)
	}
}

func TestSaveItemsReplaces(t *testing.T) {
	t.Parallel()

	s, err := store.New(filepath.Join(t.TempDir(), "data"))
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}
	if err := s.SaveItems("f1", []domain.Item{{ID: "i1", FeedID: "f1"}, {ID: "i2", FeedID: "f1"}}); err != nil {
		t.Fatalf("first SaveItems returned error: %v", err)
	}
	if err := s.SaveItems("f1", []domain.Item{{ID: "i3", FeedID: "f1"}}); err != nil {
		t.Fatalf("second SaveItems returned error: %v", err)
	}

	got, err := s.Items("f1")
	if err != nil {
		t.Fatalf("Items returned error: %v", err)
	}
	if len(got) != 1 || got[0].ID != "i3" {
		t.Fatalf("items after replace got %+v want only i3", got)
	}
}

func TestItemsUnknownFeedReturnsEmpty(t *testing.T) {
	t.Parallel()

	s, err := store.New(filepath.Join(t.TempDir(), "data"))
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}
	got, err := s.Items("nope")
	if err != nil {
		t.Fatalf("Items returned error: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("items got %d want 0 for unknown feed", len(got))
	}
}

func TestItemsReturnsCopy(t *testing.T) {
	t.Parallel()

	s, err := store.New(filepath.Join(t.TempDir(), "data"))
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}
	if err := s.SaveItems("f1", []domain.Item{{ID: "i1", FeedID: "f1", Title: "Original"}}); err != nil {
		t.Fatalf("SaveItems returned error: %v", err)
	}

	got, err := s.Items("f1")
	if err != nil {
		t.Fatalf("Items returned error: %v", err)
	}
	got[0].Title = "Mutated"

	again, err := s.Items("f1")
	if err != nil {
		t.Fatalf("Items returned error: %v", err)
	}
	if again[0].Title != "Original" {
		t.Fatalf("internal state mutated via returned slice: got %q want Original", again[0].Title)
	}
}

func TestItemsPersistAcrossReload(t *testing.T) {
	t.Parallel()

	root := filepath.Join(t.TempDir(), "data")
	s1, err := store.New(root)
	if err != nil {
		t.Fatalf("first New returned error: %v", err)
	}
	if err := s1.SaveItems("f1", []domain.Item{{ID: "i1", FeedID: "f1", Title: "Persisted"}}); err != nil {
		t.Fatalf("SaveItems returned error: %v", err)
	}

	s2, err := store.New(root)
	if err != nil {
		t.Fatalf("second New returned error: %v", err)
	}
	got, err := s2.Items("f1")
	if err != nil {
		t.Fatalf("Items returned error: %v", err)
	}
	if len(got) != 1 || got[0].Title != "Persisted" {
		t.Fatalf("reloaded items got %+v want one Persisted", got)
	}
}

func TestSaveAndDeleteBoard(t *testing.T) {
	t.Parallel()

	s, err := store.New(filepath.Join(t.TempDir(), "data"))
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}
	if err := s.SaveBoard(domain.Board{ID: "b1", Name: "Reading", Description: "later"}); err != nil {
		t.Fatalf("SaveBoard returned error: %v", err)
	}
	if err := s.SaveBoard(domain.Board{ID: "b1", Name: "Reading List", Description: "later"}); err != nil {
		t.Fatalf("SaveBoard update returned error: %v", err)
	}

	boards, err := s.Boards()
	if err != nil {
		t.Fatalf("Boards returned error: %v", err)
	}
	if len(boards) != 1 || boards[0].Name != "Reading List" {
		t.Fatalf("boards got %+v want one Reading List", boards)
	}

	if err := s.DeleteBoard("b1"); err != nil {
		t.Fatalf("DeleteBoard returned error: %v", err)
	}
	boards, err = s.Boards()
	if err != nil {
		t.Fatalf("Boards returned error: %v", err)
	}
	if len(boards) != 0 {
		t.Fatalf("boards after delete got %d want 0", len(boards))
	}
}

func TestDeleteBoardMissingReturnsNotFound(t *testing.T) {
	t.Parallel()

	s, err := store.New(filepath.Join(t.TempDir(), "data"))
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}
	if err := s.DeleteBoard("missing"); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("DeleteBoard error got %v want ErrNotFound", err)
	}
}

func TestSaveAndDeleteFilter(t *testing.T) {
	t.Parallel()

	s, err := store.New(filepath.Join(t.TempDir(), "data"))
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}
	if err := s.SaveFilter(domain.MuteFilter{ID: "m1", Keyword: "spam", Scope: domain.MuteScopeGlobal}); err != nil {
		t.Fatalf("SaveFilter returned error: %v", err)
	}
	if err := s.SaveFilter(domain.MuteFilter{ID: "m1", Keyword: "promo", Scope: domain.MuteScopeGlobal}); err != nil {
		t.Fatalf("SaveFilter update returned error: %v", err)
	}

	filters, err := s.Filters()
	if err != nil {
		t.Fatalf("Filters returned error: %v", err)
	}
	if len(filters) != 1 || filters[0].Keyword != "promo" {
		t.Fatalf("filters got %+v want one promo", filters)
	}

	if err := s.DeleteFilter("m1"); err != nil {
		t.Fatalf("DeleteFilter returned error: %v", err)
	}
	filters, err = s.Filters()
	if err != nil {
		t.Fatalf("Filters returned error: %v", err)
	}
	if len(filters) != 0 {
		t.Fatalf("filters after delete got %d want 0", len(filters))
	}
}

func TestDeleteFilterMissingReturnsNotFound(t *testing.T) {
	t.Parallel()

	s, err := store.New(filepath.Join(t.TempDir(), "data"))
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}
	if err := s.DeleteFilter("missing"); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("DeleteFilter error got %v want ErrNotFound", err)
	}
}

func TestSaveAndGetSettings(t *testing.T) {
	t.Parallel()

	root := filepath.Join(t.TempDir(), "data")
	s, err := store.New(root)
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}

	want := domain.Settings{
		PollInterval:      domain.Poll1Hour,
		MaxItems:          500,
		ReadRetentionDays: 60,
		Theme:             domain.ThemeLight,
		DefaultView:       domain.ViewMagazine,
	}
	if err := s.SaveSettings(want); err != nil {
		t.Fatalf("SaveSettings returned error: %v", err)
	}

	got, err := s.Settings()
	if err != nil {
		t.Fatalf("Settings returned error: %v", err)
	}
	if got != want {
		t.Fatalf("settings got %+v want %+v", got, want)
	}

	// 再ロードして永続化を確認します。
	s2, err := store.New(root)
	if err != nil {
		t.Fatalf("reload New returned error: %v", err)
	}
	reloaded, err := s2.Settings()
	if err != nil {
		t.Fatalf("reload Settings returned error: %v", err)
	}
	if reloaded != want {
		t.Fatalf("reloaded settings got %+v want %+v", reloaded, want)
	}
}

func TestSaveAndGetUser(t *testing.T) {
	t.Parallel()

	root := filepath.Join(t.TempDir(), "data")
	s, err := store.New(root)
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}

	want := domain.User{Username: "owner", PasswordHash: "scrypt$hash"}
	if err := s.SaveUser(want); err != nil {
		t.Fatalf("SaveUser returned error: %v", err)
	}

	got, err := s.User()
	if err != nil {
		t.Fatalf("User returned error: %v", err)
	}
	if got != want {
		t.Fatalf("user got %+v want %+v", got, want)
	}
	if !got.IsRegistered() {
		t.Fatal("user got not registered want registered")
	}

	s2, err := store.New(root)
	if err != nil {
		t.Fatalf("reload New returned error: %v", err)
	}
	reloaded, err := s2.User()
	if err != nil {
		t.Fatalf("reload User returned error: %v", err)
	}
	if reloaded != want {
		t.Fatalf("reloaded user got %+v want %+v", reloaded, want)
	}
}

func TestConcurrentSaveAndRead(t *testing.T) {
	t.Parallel()

	s, err := store.New(filepath.Join(t.TempDir(), "data"))
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}

	const workers = 8
	const perWorker = 20

	var wg sync.WaitGroup
	wg.Add(workers * 2)

	for w := 0; w < workers; w++ {
		go func(w int) {
			defer wg.Done()
			for i := 0; i < perWorker; i++ {
				id := fmt.Sprintf("f-%d-%d", w, i)
				if err := s.SaveFeed(domain.Feed{ID: id, Title: id}); err != nil {
					t.Errorf("SaveFeed returned error: %v", err)
					return
				}
			}
		}(w)
		go func() {
			defer wg.Done()
			for i := 0; i < perWorker; i++ {
				if _, err := s.Feeds(); err != nil {
					t.Errorf("Feeds returned error: %v", err)
					return
				}
			}
		}()
	}
	wg.Wait()

	feeds, err := s.Feeds()
	if err != nil {
		t.Fatalf("Feeds returned error: %v", err)
	}
	if len(feeds) != workers*perWorker {
		t.Fatalf("feeds got %d want %d", len(feeds), workers*perWorker)
	}
}

func TestNewWithCorruptFeedsFileReturnsError(t *testing.T) {
	t.Parallel()

	root := filepath.Join(t.TempDir(), "data")
	if err := os.MkdirAll(filepath.Join(root, "items"), 0o700); err != nil {
		t.Fatalf("MkdirAll returned error: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "feeds.json"), []byte("{not json"), 0o600); err != nil {
		t.Fatalf("WriteFile returned error: %v", err)
	}

	_, err := store.New(root)
	if err == nil {
		t.Fatal("New got nil error want error for corrupt feeds.json")
	}
	if !strings.Contains(err.Error(), "feeds.json") {
		t.Fatalf("error got %v want it to mention feeds.json", err)
	}
}

func TestNewWithCorruptItemsFileReturnsError(t *testing.T) {
	t.Parallel()

	root := filepath.Join(t.TempDir(), "data")
	if err := os.MkdirAll(filepath.Join(root, "items"), 0o700); err != nil {
		t.Fatalf("MkdirAll returned error: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "items", "f1.json"), []byte("[broken"), 0o600); err != nil {
		t.Fatalf("WriteFile returned error: %v", err)
	}

	_, err := store.New(root)
	if err == nil {
		t.Fatal("New got nil error want error for corrupt items file")
	}
	if !strings.Contains(err.Error(), "f1.json") {
		t.Fatalf("error got %v want it to mention f1.json", err)
	}
}
