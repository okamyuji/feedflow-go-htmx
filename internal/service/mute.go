package service

import (
	"errors"
	"fmt"

	"github.com/okamyuji/feedflow-go-htmx/internal/domain"
)

// ErrEmptyKeyword ミュートフィルタのキーワードが空のときに返すエラーです。
var ErrEmptyKeyword = errors.New("mute filter keyword must not be empty")

// ErrInvalidMuteScope ミュートフィルタの対象範囲が不正なときに返すエラーです。
var ErrInvalidMuteScope = errors.New("invalid mute scope")

// ErrMissingFeedID フィード限定フィルタで対象フィードIDが空のときに返すエラーです。
var ErrMissingFeedID = errors.New("feed-scoped filter requires a feed id")

// MuteService ミュートフィルタの管理と適用を担います。port.MuteServiceを満たします。
type MuteService struct {
	deps Deps
}

// NewMuteService 依存束を受け取りMuteServiceを構築します。
func NewMuteService(deps Deps) *MuteService {
	return &MuteService{deps: deps}
}

// ListFilters 全ミュートフィルタを返します。
func (s *MuteService) ListFilters() ([]domain.MuteFilter, error) {
	filters, err := s.deps.Repo.Filters()
	if err != nil {
		return nil, fmt.Errorf("failed to load filters: %w", err)
	}
	return filters, nil
}

// AddFilter ミュートフィルタを検証してから採番し保存します。追加後のフィルタを返します。
func (s *MuteService) AddFilter(keyword string, scope domain.MuteScope, feedID string) (domain.MuteFilter, error) {
	if keyword == "" {
		return domain.MuteFilter{}, ErrEmptyKeyword
	}
	if !scope.Valid() {
		return domain.MuteFilter{}, ErrInvalidMuteScope
	}
	if scope == domain.MuteScopeFeed && feedID == "" {
		return domain.MuteFilter{}, ErrMissingFeedID
	}
	filter := domain.MuteFilter{
		ID:      s.deps.IDs.NewID(),
		Keyword: keyword,
		Scope:   scope,
		FeedID:  feedID,
	}
	if err := s.deps.Repo.SaveFilter(filter); err != nil {
		return domain.MuteFilter{}, fmt.Errorf("failed to save filter: %w", err)
	}
	return filter, nil
}

// DeleteFilter 指定IDのミュートフィルタを削除します。
func (s *MuteService) DeleteFilter(id string) error {
	if err := s.deps.Repo.DeleteFilter(id); err != nil {
		return fmt.Errorf("failed to delete filter: %w", err)
	}
	return nil
}

// Filter 与えた記事群からミュート対象を除いた記事群を返します。
// いずれかのフィルタにタイトルが一致した記事を除外します。
func (s *MuteService) Filter(items []domain.Item) ([]domain.Item, error) {
	filters, err := s.deps.Repo.Filters()
	if err != nil {
		return nil, fmt.Errorf("failed to load filters: %w", err)
	}
	if len(filters) == 0 {
		return items, nil
	}
	out := make([]domain.Item, 0, len(items))
	for _, item := range items {
		if muted(filters, item) {
			continue
		}
		out = append(out, item)
	}
	return out, nil
}

// muted いずれかのフィルタが記事に一致するかどうかを返します。
func muted(filters []domain.MuteFilter, item domain.Item) bool {
	for _, f := range filters {
		if f.Matches(item.Title, item.FeedID) {
			return true
		}
	}
	return false
}
