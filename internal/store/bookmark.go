package store

import (
	"fmt"
	"slices"

	"github.com/okamyuji/feedflow-go-htmx/internal/domain"
)

// Bookmarks 全ブックマークを内部状態と共有しないコピーで返します。
func (s *Store) Bookmarks() ([]domain.Bookmark, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return slices.Clone(s.bookmarks), nil
}

// SaveBookmark ブックマークを新規追加または更新し、bookmarks.jsonをアトミックに書き出します。
func (s *Store) SaveBookmark(bookmark domain.Bookmark) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	prev := slices.Clone(s.bookmarks)
	idx := slices.IndexFunc(s.bookmarks, func(b domain.Bookmark) bool { return b.ID == bookmark.ID })
	if idx >= 0 {
		s.bookmarks[idx] = bookmark
	} else {
		s.bookmarks = append(s.bookmarks, bookmark)
	}

	if err := writeJSONAtomic(s.path(bookmarksFile), s.bookmarks); err != nil {
		s.bookmarks = prev
		return fmt.Errorf("failed to save bookmark %q: %w", bookmark.ID, err)
	}
	return nil
}

// DeleteBookmark 指定IDのブックマークを削除し、bookmarks.jsonをアトミックに書き出します。
func (s *Store) DeleteBookmark(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	idx := slices.IndexFunc(s.bookmarks, func(b domain.Bookmark) bool { return b.ID == id })
	if idx < 0 {
		return fmt.Errorf("bookmark %q: %w", id, ErrNotFound)
	}

	prev := slices.Clone(s.bookmarks)
	s.bookmarks = slices.Delete(s.bookmarks, idx, idx+1)
	if err := writeJSONAtomic(s.path(bookmarksFile), s.bookmarks); err != nil {
		s.bookmarks = prev
		return fmt.Errorf("failed to delete bookmark %q: %w", id, err)
	}
	return nil
}

// Filters全ミュートフィルタを内部状態と共有しないコピーで返します。
func (s *Store) Filters() ([]domain.MuteFilter, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return slices.Clone(s.filters), nil
}

// SaveFilter ミュートフィルタを新規追加または更新し、filters.jsonをアトミックに書き出します。
func (s *Store) SaveFilter(filter domain.MuteFilter) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	prev := slices.Clone(s.filters)
	idx := slices.IndexFunc(s.filters, func(m domain.MuteFilter) bool { return m.ID == filter.ID })
	if idx >= 0 {
		s.filters[idx] = filter
	} else {
		s.filters = append(s.filters, filter)
	}

	if err := writeJSONAtomic(s.path(filtersFile), s.filters); err != nil {
		s.filters = prev
		return fmt.Errorf("failed to save filter %q: %w", filter.ID, err)
	}
	return nil
}

// DeleteFilter 指定IDのミュートフィルタを削除し、filters.jsonをアトミックに書き出します。
func (s *Store) DeleteFilter(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	idx := slices.IndexFunc(s.filters, func(m domain.MuteFilter) bool { return m.ID == id })
	if idx < 0 {
		return fmt.Errorf("filter %q: %w", id, ErrNotFound)
	}

	prev := slices.Clone(s.filters)
	s.filters = slices.Delete(s.filters, idx, idx+1)
	if err := writeJSONAtomic(s.path(filtersFile), s.filters); err != nil {
		s.filters = prev
		return fmt.Errorf("failed to delete filter %q: %w", id, err)
	}
	return nil
}
