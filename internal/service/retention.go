package service

import (
	"fmt"
	"sort"

	"github.com/okamyuji/feedflow-go-htmx/internal/domain"
)

// RetentionService 保持ポリシーの適用を担います。port.RetentionServiceを満たします。
type RetentionService struct {
	deps Deps
}

// NewRetentionService 依存束を受け取りRetentionServiceを構築します。
func NewRetentionService(deps Deps) *RetentionService {
	return &RetentionService{deps: deps}
}

// Apply 全フィードに保持ポリシーを適用し、削除した記事の総数を返します。
func (s *RetentionService) Apply() (int, error) {
	feeds, err := s.deps.Repo.Feeds()
	if err != nil {
		return 0, fmt.Errorf("failed to load feeds: %w", err)
	}
	total := 0
	for _, f := range feeds {
		removed, err := s.ApplyFeed(f.ID)
		if err != nil {
			return total, err
		}
		total += removed
	}
	return total, nil
}

// ApplyFeed 指定フィードに保持ポリシーを適用し、削除した記事数を返します。
// 記事を公開日時の新しい順へ並べ、順位とともにdomain.Item.ShouldRetainで保持判定します。
func (s *RetentionService) ApplyFeed(feedID string) (int, error) {
	settings, err := s.deps.Repo.Settings()
	if err != nil {
		return 0, fmt.Errorf("failed to load settings: %w", err)
	}
	items, err := s.deps.Repo.Items(feedID)
	if err != nil {
		return 0, fmt.Errorf("failed to load items for feed %s: %w", feedID, err)
	}

	ordered := make([]domain.Item, len(items))
	copy(ordered, items)
	sortByRecency(ordered)

	now := s.deps.Clock.Now()
	kept := make([]domain.Item, 0, len(ordered))
	removed := 0
	for rank, item := range ordered {
		if item.ShouldRetain(now, rank, settings.MaxItems, settings.ReadRetentionDays) {
			kept = append(kept, item)
			continue
		}
		removed++
	}
	if removed == 0 {
		return 0, nil
	}
	if err := s.deps.Repo.SaveItems(feedID, kept); err != nil {
		return 0, fmt.Errorf("failed to save items for feed %s: %w", feedID, err)
	}
	return removed, nil
}

// sortByRecency 記事を公開日時の新しい順へ並べ替えます。
// 公開日時が等しいときは取得日時の新しい順を二次キーにします。
func sortByRecency(items []domain.Item) {
	sort.SliceStable(items, func(i, j int) bool {
		if items[i].PublishedAt.Equal(items[j].PublishedAt) {
			return items[i].FetchedAt.After(items[j].FetchedAt)
		}
		return items[i].PublishedAt.After(items[j].PublishedAt)
	})
}
