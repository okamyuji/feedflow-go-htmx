package store

import (
	"fmt"
	"slices"

	"github.com/okamyuji/feedflow-go-htmx/internal/domain"
)

// Items指定フィードの全記事を内部状態と共有しないコピーで返します。
// 未登録のフィードには空スライスを返します。
func (s *Store) Items(feedID string) ([]domain.Item, error) {
	if !validStoreID(feedID) {
		return nil, fmt.Errorf("items for feed %q: %w", feedID, ErrInvalidID)
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	items, ok := s.items[feedID]
	if !ok {
		return []domain.Item{}, nil
	}
	return slices.Clone(items), nil
}

// SaveItems指定フィードの記事群をまとめて保存し、既存の記事群を置き換えます。
// 対応するitems/フィードID.jsonをアトミックに書き出します。
func (s *Store) SaveItems(feedID string, items []domain.Item) error {
	if !validStoreID(feedID) {
		return fmt.Errorf("save items for feed %q: %w", feedID, ErrInvalidID)
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	prev, had := s.items[feedID]
	s.items[feedID] = slices.Clone(items)

	if err := writeJSONAtomic(s.itemsPath(feedID), s.items[feedID]); err != nil {
		if had {
			s.items[feedID] = prev
		} else {
			delete(s.items, feedID)
		}
		return fmt.Errorf("failed to save items for feed %q: %w", feedID, err)
	}
	return nil
}
