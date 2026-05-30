package service

import (
	"errors"
	"fmt"
	"strings"

	"github.com/okamyuji/feedflow-go-htmx/internal/domain"
)

// ErrBookmarkNameRequired ブックマーク名が空のときに返すエラーです。
var ErrBookmarkNameRequired = errors.New("bookmark name is required")

// BookmarkService 名称付きブックマークの一覧と作成、記事の所属操作を担います。
// port.BookmarkService を満たします。
type BookmarkService struct {
	deps  Deps
	items *ItemService
}

// NewBookmarkService 依存束とItemServiceを受け取りBookmarkServiceを構築します。
// 記事への所属操作はItemServiceの不変更新に委譲します。
func NewBookmarkService(deps Deps, items *ItemService) *BookmarkService {
	return &BookmarkService{deps: deps, items: items}
}

// List 全ブックマークを返します。
func (s *BookmarkService) List() ([]domain.Bookmark, error) {
	bms, err := s.deps.Repo.Bookmarks()
	if err != nil {
		return nil, fmt.Errorf("failed to load bookmarks: %w", err)
	}
	return bms, nil
}

// Create 指定名のブックマークを作成して返します。前後空白は除去します。
// 同名のブックマークが既存ならそれを返し、重複作成しません。空名はエラーを返します。
func (s *BookmarkService) Create(name string) (domain.Bookmark, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return domain.Bookmark{}, ErrBookmarkNameRequired
	}
	bms, err := s.deps.Repo.Bookmarks()
	if err != nil {
		return domain.Bookmark{}, fmt.Errorf("failed to load bookmarks: %w", err)
	}
	for _, b := range bms {
		if b.Name == name {
			return b, nil
		}
	}
	bm := domain.Bookmark{ID: s.deps.IDs.NewID(), Name: name}
	if err := s.deps.Repo.SaveBookmark(bm); err != nil {
		return domain.Bookmark{}, fmt.Errorf("failed to save bookmark: %w", err)
	}
	return bm, nil
}

// Toggle 指定記事のブックマーク所属を切り替えます。
func (s *BookmarkService) Toggle(feedID, itemID, bookmarkID string) error {
	return s.items.toggleBookmark(feedID, itemID, bookmarkID)
}

// CreateAndAdd 指定名のブックマークを用意し、指定記事を所属させて返します。
func (s *BookmarkService) CreateAndAdd(feedID, itemID, name string) (domain.Bookmark, error) {
	bm, err := s.Create(name)
	if err != nil {
		return domain.Bookmark{}, err
	}
	if err := s.items.addBookmark(feedID, itemID, bm.ID); err != nil {
		return domain.Bookmark{}, err
	}
	return bm, nil
}
