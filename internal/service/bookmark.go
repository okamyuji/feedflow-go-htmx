package service

import (
	"errors"
	"fmt"
	"strings"

	"github.com/okamyuji/feedflow-go-htmx/internal/domain"
)

// ErrBookmarkNameRequired ブックマーク名が空のときに返すエラーです。
var ErrBookmarkNameRequired = errors.New("bookmark name is required")

// ErrBookmarkNameTaken 別のブックマークが同名で既に存在するときに返すエラーです。リネームの重複を防ぎます。
var ErrBookmarkNameTaken = errors.New("bookmark name already exists")

// ErrBookmarkNotFound 指定IDのブックマークが存在しないときに返すエラーです。リネーム対象の不在判定に使います。
var ErrBookmarkNotFound = errors.New("bookmark not found")

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

// Rename 指定IDのラベル名を変更します。前後空白は除去します。
// 空名は ErrBookmarkNameRequired を返します。別のラベルが同名なら ErrBookmarkNameTaken を返します。
// 対象IDが存在しない場合は store.ErrNotFound を透過します。
func (s *BookmarkService) Rename(id, name string) error {
	name = strings.TrimSpace(name)
	if name == "" {
		return ErrBookmarkNameRequired
	}
	bms, err := s.deps.Repo.Bookmarks()
	if err != nil {
		return fmt.Errorf("failed to load bookmarks: %w", err)
	}
	found := false
	for _, b := range bms {
		if b.ID != id && b.Name == name {
			return ErrBookmarkNameTaken
		}
		if b.ID == id {
			found = true
		}
	}
	if !found {
		return fmt.Errorf("bookmark %q: %w", id, ErrBookmarkNotFound)
	}
	if err := s.deps.Repo.SaveBookmark(domain.Bookmark{ID: id, Name: name}); err != nil {
		return fmt.Errorf("failed to rename bookmark: %w", err)
	}
	return nil
}

// Delete 指定IDのラベルを削除します。
// 先に全記事から当該ラベルの所属を取り除いて孤児参照を防ぎ、その後にラベル自体を削除します。
// 記事の保存状態(Bookmarked)は維持されるため、ラベルを消しても保存した記事はブックマークに残ります。
func (s *BookmarkService) Delete(id string) error {
	if err := s.items.RemoveBookmarkIDFromAll(id); err != nil {
		return fmt.Errorf("failed to detach bookmark %q from items: %w", id, err)
	}
	if err := s.deps.Repo.DeleteBookmark(id); err != nil {
		return fmt.Errorf("failed to delete bookmark %q: %w", id, err)
	}
	return nil
}
