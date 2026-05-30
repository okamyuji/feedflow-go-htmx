// Package store メモリ常駐とアトミックJSON永続化でport.Repositoryを実装します。
// 起動時にdataディレクトリの全JSONをロードし、sync.RWMutexで保護したメモリ状態を保ちます。
// 変更はtempファイルへ書いてからos.Renameで原子的に反映します。設計書のセクション7に対応します。
package store

import (
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sync"

	"github.com/okamyuji/feedflow-go-htmx/internal/domain"
)

// ErrNotFound 指定IDのエンティティが見つからないことを表すsentinel errorです。
// 呼び出し側はerrors.Is(err, store.ErrNotFound)で判別できます。
var ErrNotFound = errors.New("store: entity not found")

// dirPerm永続化ディレクトリのパーミッションです。所有者のみアクセスを許可します。
const dirPerm = 0o700

// ファイル名の定数です。dataディレクトリ直下に配置します。
const (
	feedsFile        = "feeds.json"
	categoriesFile   = "categories.json"
	bookmarksFile    = "bookmarks.json"
	legacyBoardsFile = "boards.json"
	filtersFile      = "filters.json"
	settingsFile     = "settings.json"
	userFile         = "user.json"
	itemsDir         = "items"
)

// Store メモリ常駐の永続化集約です。全エンティティをメモリに保持しsync.RWMutexで保護します。
// port.Repositoryを実装します。
type Store struct {
	dataDir string
	logger  *slog.Logger

	mu         sync.RWMutex
	feeds      []domain.Feed
	categories []domain.Category
	bookmarks  []domain.Bookmark
	filters    []domain.MuteFilter
	items      map[string][]domain.Item
	settings   domain.Settings
	user       domain.User
}

// New dataDirを永続化ディレクトリとするStoreを生成し、全件をロードします。
// dataDirと配下のitemsディレクトリが無ければ作成します。
func New(dataDir string) (*Store, error) {
	s := &Store{
		dataDir:  dataDir,
		logger:   slog.Default(),
		items:    make(map[string][]domain.Item),
		settings: domain.DefaultSettings(),
	}

	if err := os.MkdirAll(filepath.Join(dataDir, itemsDir), dirPerm); err != nil {
		return nil, fmt.Errorf("failed to create data directories under %s: %w", dataDir, err)
	}
	if err := s.load(); err != nil {
		return nil, fmt.Errorf("failed to load store from %s: %w", dataDir, err)
	}
	return s, nil
}

// path dataディレクトリ直下のファイルへの絶対パスを返します。
func (s *Store) path(name string) string {
	return filepath.Join(s.dataDir, name)
}

// itemsPath指定フィードの記事ファイルへのパスを返します。
func (s *Store) itemsPath(feedID string) string {
	return filepath.Join(s.dataDir, itemsDir, feedID+".json")
}

// load 全エンティティのJSONをメモリに読み込みます。ファイルが無い項目は空または既定値のままにします。
func (s *Store) load() error {
	if err := s.loadSlice(feedsFile, &s.feeds); err != nil {
		return err
	}
	if err := s.loadSlice(categoriesFile, &s.categories); err != nil {
		return err
	}
	if err := s.loadBookmarks(); err != nil {
		return err
	}
	if err := s.loadSlice(filtersFile, &s.filters); err != nil {
		return err
	}
	if err := s.loadSettings(); err != nil {
		return err
	}
	if err := s.loadUser(); err != nil {
		return err
	}
	return s.loadItems()
}

// loadBookmarks bookmarks.jsonを読み込みます。
// bookmarks.jsonが無く旧boards.jsonが存在する場合は、Board(id,name)をBookmarkへ変換して取り込み、
// bookmarks.jsonとして書き出すワンタイムマイグレーションを行います。
// 旧ボードは左メニュー未公開で実データはほぼ無い前提ですが、安全側で移行します。
func (s *Store) loadBookmarks() error {
	if err := s.loadSlice(bookmarksFile, &s.bookmarks); err != nil {
		return err
	}
	if len(s.bookmarks) > 0 {
		return nil
	}
	if _, err := os.Stat(s.path(bookmarksFile)); err == nil {
		return nil // 空のbookmarks.jsonが既にあるなら移行しません
	}
	var legacy []domain.Bookmark
	if err := s.loadSlice(legacyBoardsFile, &legacy); err != nil {
		return err
	}
	if len(legacy) == 0 {
		return nil
	}
	s.bookmarks = legacy
	if err := writeJSONAtomic(s.path(bookmarksFile), s.bookmarks); err != nil {
		return fmt.Errorf("failed to migrate boards to bookmarks: %w", err)
	}
	return nil
}

// loadSlice nameのファイルをdstにデコードします。ファイルが無い場合は何もしません。
func (s *Store) loadSlice(name string, dst any) error {
	err := readJSON(s.path(name), dst)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to load %s: %w", name, err)
	}
	return nil
}

// loadSettings settings.jsonを読み込みます。ファイルが無い場合は既定値を保ちます。
// 既定値を初期値として代入してからUnmarshalするため、JSONに無いキーは既定値が残ります。
// これにより既存のsettings.jsonにauto_read_on_scrollが無くても既定のtrueになります。
func (s *Store) loadSettings() error {
	loaded := domain.DefaultSettings()
	err := readJSON(s.path(settingsFile), &loaded)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("failed to load %s: %w", settingsFile, err)
	}
	s.settings = loaded
	return nil
}

// loadUser user.jsonを読み込みます。ファイルが無い場合はゼロ値のままにします。
func (s *Store) loadUser() error {
	var loaded domain.User
	err := readJSON(s.path(userFile), &loaded)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("failed to load %s: %w", userFile, err)
	}
	s.user = loaded
	return nil
}

// loadItems itemsディレクトリ配下の全JSONを読み込み、ファイル名のフィードIDをキーに保持します。
func (s *Store) loadItems() error {
	dir := filepath.Join(s.dataDir, itemsDir)
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("failed to read items dir %s: %w", dir, err)
	}
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || filepath.Ext(name) != ".json" {
			continue
		}
		feedID := name[:len(name)-len(".json")]
		var items []domain.Item
		if err := readJSON(filepath.Join(dir, name), &items); err != nil {
			return fmt.Errorf("failed to load items file %s: %w", name, err)
		}
		s.items[feedID] = items
	}
	return nil
}
