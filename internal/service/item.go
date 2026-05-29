package service

import (
	"errors"
	"fmt"

	"github.com/okamyuji/feedflow-go-htmx/internal/domain"
)

// ErrItemNotFound 指定IDの記事が見つからないときに返すエラーです。
var ErrItemNotFound = errors.New("item not found")

// ItemService 記事の既読やスターやあとで読むやタグやボードやメモの操作を担います。
// port.ItemService を満たします。
type ItemService struct {
	deps Deps
	mute *MuteService
}

// NewItemService 依存束とミュートサービスを受け取りItemServiceを構築します。
// ListItems がミュート適用済みの記事を返すためMuteServiceを必須の依存とします。
func NewItemService(deps Deps, mute *MuteService) *ItemService {
	return &ItemService{deps: deps, mute: mute}
}

// ListItems 指定フィードの記事をミュート適用済みで返します。feedIDが空なら全フィード横断で返します。
// port.ItemService.ListItems の契約どおりMuteService.Filterを通してミュート対象を除外します。
func (s *ItemService) ListItems(feedID string) ([]domain.Item, error) {
	var raw []domain.Item
	if feedID != "" {
		items, err := s.deps.Repo.Items(feedID)
		if err != nil {
			return nil, fmt.Errorf("failed to load items for feed %s: %w", feedID, err)
		}
		raw = items
	} else {
		feeds, err := s.deps.Repo.Feeds()
		if err != nil {
			return nil, fmt.Errorf("failed to load feeds: %w", err)
		}
		for _, f := range feeds {
			items, err := s.deps.Repo.Items(f.ID)
			if err != nil {
				return nil, fmt.Errorf("failed to load items for feed %s: %w", f.ID, err)
			}
			raw = append(raw, items...)
		}
	}
	filtered, err := s.mute.Filter(raw)
	if err != nil {
		return nil, fmt.Errorf("failed to apply mute filters: %w", err)
	}
	return filtered, nil
}

// MarkRead 指定記事の既読状態を設定します。
func (s *ItemService) MarkRead(feedID, itemID string, read bool) error {
	return s.mutateItem(feedID, itemID, func(item domain.Item) domain.Item {
		item.Read = read
		return item
	})
}

// MarkAllRead 指定フィードの全記事を既読にします。feedIDが空なら全フィードを対象にします。
func (s *ItemService) MarkAllRead(feedID string) error {
	if feedID != "" {
		return s.markAllReadOne(feedID)
	}
	feeds, err := s.deps.Repo.Feeds()
	if err != nil {
		return fmt.Errorf("failed to load feeds: %w", err)
	}
	for _, f := range feeds {
		if err := s.markAllReadOne(f.ID); err != nil {
			return err
		}
	}
	return nil
}

// markAllReadOne 単一フィードの全記事を既読にします。
func (s *ItemService) markAllReadOne(feedID string) error {
	items, err := s.deps.Repo.Items(feedID)
	if err != nil {
		return fmt.Errorf("failed to load items for feed %s: %w", feedID, err)
	}
	updated := make([]domain.Item, len(items))
	for i, item := range items {
		item.Read = true
		updated[i] = item
	}
	if err := s.deps.Repo.SaveItems(feedID, updated); err != nil {
		return fmt.Errorf("failed to save items for feed %s: %w", feedID, err)
	}
	return nil
}

// mutateItem 指定フィード内の指定記事を不変更新で差し替えて保存します。
// 対象が見つからない場合はErrItemNotFoundを返します。
func (s *ItemService) mutateItem(feedID, itemID string, fn func(domain.Item) domain.Item) error {
	items, err := s.deps.Repo.Items(feedID)
	if err != nil {
		return fmt.Errorf("failed to load items for feed %s: %w", feedID, err)
	}
	updated := make([]domain.Item, len(items))
	copy(updated, items)
	found := false
	for i, item := range updated {
		if item.ID == itemID {
			updated[i] = fn(item)
			found = true
			break
		}
	}
	if !found {
		return ErrItemNotFound
	}
	if err := s.deps.Repo.SaveItems(feedID, updated); err != nil {
		return fmt.Errorf("failed to save items for feed %s: %w", feedID, err)
	}
	return nil
}

// Star 指定記事のスター状態を設定します。
func (s *ItemService) Star(feedID, itemID string, starred bool) error {
	return s.mutateItem(feedID, itemID, func(item domain.Item) domain.Item {
		item.Starred = starred
		return item
	})
}

// ReadLater 指定記事のあとで読む状態を設定します。
func (s *ItemService) ReadLater(feedID, itemID string, readLater bool) error {
	return s.mutateItem(feedID, itemID, func(item domain.Item) domain.Item {
		item.ReadLater = readLater
		return item
	})
}

// SetTags 指定記事のタグを与えた内容で置き換えます。
func (s *ItemService) SetTags(feedID, itemID string, tags []string) error {
	return s.mutateItem(feedID, itemID, func(item domain.Item) domain.Item {
		next := make([]string, len(tags))
		copy(next, tags)
		item.Tags = next
		return item
	})
}

// SetBoards 指定記事の保存先ボードを与えた内容で置き換えます。
func (s *ItemService) SetBoards(feedID, itemID string, boardIDs []string) error {
	return s.mutateItem(feedID, itemID, func(item domain.Item) domain.Item {
		next := make([]string, len(boardIDs))
		copy(next, boardIDs)
		item.BoardIDs = next
		return item
	})
}

// SetNote 指定記事のメモを更新します。
func (s *ItemService) SetNote(feedID, itemID, note string) error {
	return s.mutateItem(feedID, itemID, func(item domain.Item) domain.Item {
		item.Note = note
		return item
	})
}

// AddHighlight 指定記事のハイライト群へ末尾追記します。
func (s *ItemService) AddHighlight(feedID, itemID, highlight string) error {
	return s.mutateItem(feedID, itemID, func(item domain.Item) domain.Item {
		next := make([]string, 0, len(item.Highlights)+1)
		next = append(next, item.Highlights...)
		next = append(next, highlight)
		item.Highlights = next
		return item
	})
}
