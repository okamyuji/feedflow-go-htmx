package store

import (
	"fmt"
	"slices"

	"github.com/okamyuji/feedflow-go-htmx/internal/domain"
)

// Boards全ボードを内部状態と共有しないコピーで返します。
func (s *Store) Boards() ([]domain.Board, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return slices.Clone(s.boards), nil
}

// SaveBoard ボードを新規追加または更新し、boards.jsonをアトミックに書き出します。
func (s *Store) SaveBoard(board domain.Board) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	prev := slices.Clone(s.boards)
	idx := slices.IndexFunc(s.boards, func(b domain.Board) bool { return b.ID == board.ID })
	if idx >= 0 {
		s.boards[idx] = board
	} else {
		s.boards = append(s.boards, board)
	}

	if err := writeJSONAtomic(s.path(boardsFile), s.boards); err != nil {
		s.boards = prev
		return fmt.Errorf("failed to save board %q: %w", board.ID, err)
	}
	return nil
}

// DeleteBoard 指定IDのボードを削除し、boards.jsonをアトミックに書き出します。
func (s *Store) DeleteBoard(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	idx := slices.IndexFunc(s.boards, func(b domain.Board) bool { return b.ID == id })
	if idx < 0 {
		return fmt.Errorf("board %q: %w", id, ErrNotFound)
	}

	prev := slices.Clone(s.boards)
	s.boards = slices.Delete(s.boards, idx, idx+1)
	if err := writeJSONAtomic(s.path(boardsFile), s.boards); err != nil {
		s.boards = prev
		return fmt.Errorf("failed to delete board %q: %w", id, err)
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
