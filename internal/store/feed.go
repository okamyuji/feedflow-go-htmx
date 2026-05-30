package store

import (
	"fmt"
	"os"
	"slices"

	"github.com/okamyuji/feedflow-go-htmx/internal/domain"
)

// Feeds登録済みの全フィードを内部状態と共有しないコピーで返します。
func (s *Store) Feeds() ([]domain.Feed, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return slices.Clone(s.feeds), nil
}

// Feed 指定IDのフィードを返します。見つからない場合はErrNotFoundをラップして返します。
func (s *Store) Feed(id string) (domain.Feed, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, f := range s.feeds {
		if f.ID == id {
			return f, nil
		}
	}
	return domain.Feed{}, fmt.Errorf("feed %q: %w", id, ErrNotFound)
}

// SaveFeed フィードを新規追加または更新し、feeds.jsonをアトミックに書き出します。
func (s *Store) SaveFeed(feed domain.Feed) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	prev := slices.Clone(s.feeds)
	idx := slices.IndexFunc(s.feeds, func(f domain.Feed) bool { return f.ID == feed.ID })
	if idx >= 0 {
		s.feeds[idx] = feed
	} else {
		s.feeds = append(s.feeds, feed)
	}

	if err := writeJSONAtomic(s.path(feedsFile), s.feeds); err != nil {
		s.feeds = prev
		return fmt.Errorf("failed to save feed %q: %w", feed.ID, err)
	}
	return nil
}

// DeleteFeed 指定IDのフィードと、それに属する全記事を削除します。
func (s *Store) DeleteFeed(id string) error {
	if !validStoreID(id) {
		return fmt.Errorf("delete feed %q: %w", id, ErrInvalidID)
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	idx := slices.IndexFunc(s.feeds, func(f domain.Feed) bool { return f.ID == id })
	if idx < 0 {
		return fmt.Errorf("feed %q: %w", id, ErrNotFound)
	}

	prev := slices.Clone(s.feeds)
	s.feeds = slices.Delete(s.feeds, idx, idx+1)
	if err := writeJSONAtomic(s.path(feedsFile), s.feeds); err != nil {
		s.feeds = prev
		return fmt.Errorf("failed to delete feed %q: %w", id, err)
	}

	// 記事ファイルとメモリ上の記事も削除します。ファイルが無い場合は許容します。
	if err := os.Remove(s.itemsPath(id)); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to remove items file for feed %q: %w", id, err)
	}
	delete(s.items, id)
	return nil
}

// Categories全カテゴリを内部状態と共有しないコピーで返します。
func (s *Store) Categories() ([]domain.Category, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return slices.Clone(s.categories), nil
}

// SaveCategory カテゴリを新規追加または更新し、categories.jsonをアトミックに書き出します。
func (s *Store) SaveCategory(category domain.Category) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	prev := slices.Clone(s.categories)
	idx := slices.IndexFunc(s.categories, func(c domain.Category) bool { return c.ID == category.ID })
	if idx >= 0 {
		s.categories[idx] = category
	} else {
		s.categories = append(s.categories, category)
	}

	if err := writeJSONAtomic(s.path(categoriesFile), s.categories); err != nil {
		s.categories = prev
		return fmt.Errorf("failed to save category %q: %w", category.ID, err)
	}
	return nil
}

// DeleteCategory 指定IDのカテゴリを削除し、categories.jsonをアトミックに書き出します。
func (s *Store) DeleteCategory(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	idx := slices.IndexFunc(s.categories, func(c domain.Category) bool { return c.ID == id })
	if idx < 0 {
		return fmt.Errorf("category %q: %w", id, ErrNotFound)
	}

	prev := slices.Clone(s.categories)
	s.categories = slices.Delete(s.categories, idx, idx+1)
	if err := writeJSONAtomic(s.path(categoriesFile), s.categories); err != nil {
		s.categories = prev
		return fmt.Errorf("failed to delete category %q: %w", id, err)
	}
	return nil
}
