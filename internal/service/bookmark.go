package service

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"mime"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/okamyuji/feedflow-go-htmx/internal/domain"
	"github.com/okamyuji/feedflow-go-htmx/internal/feed"
	"github.com/okamyuji/feedflow-go-htmx/internal/port"
)

// addURLFetchTimeout 保存対象ページのタイトル取得に使う制限時間です。
// HTTPサーバのWriteTimeout(30秒)より短くし、手動取得(20秒)と揃えます。
const addURLFetchTimeout = 20 * time.Second

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
	// addURL 任意URLの追加を直列化します。
	// 重複確認から保存までは複数回リポジトリを触るため、間に別の追加が割り込むと
	// 同じURLが二重に積まれたり、片方の保存が失われたりします。
	addURL sync.Mutex
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
// 対象IDが存在しない場合は ErrBookmarkNotFound をラップして返します。
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

// AddURL 任意のURLをブックマークに追加し、保存された記事を返します。
// 既に同じURLの記事が購読フィードにあればその記事を保存済みにします。
// 無ければ合成フィードに新しい記事を作ります。
// bookmarkIDが空でなければ、その名称コレクションにも所属させます。
// タイトルの取得に失敗しても保存は成功させ、タイトルには入力URLを使います。
func (s *BookmarkService) AddURL(ctx context.Context, rawURL, bookmarkID string) (domain.Item, error) {
	normalized, err := normalizeURL(rawURL)
	if err != nil {
		return domain.Item{}, err
	}
	if bookmarkID != "" {
		if err := s.requireBookmark(bookmarkID); err != nil {
			return domain.Item{}, err
		}
	}

	s.addURL.Lock()
	defer s.addURL.Unlock()

	feeds, err := s.deps.Repo.Feeds()
	if err != nil {
		return domain.Item{}, fmt.Errorf("failed to load feeds: %w", err)
	}
	it, found, err := s.bookmarkExistingItem(feeds, normalized, bookmarkID)
	if err != nil {
		return domain.Item{}, err
	}
	if found {
		return it, nil
	}
	if err := s.ensureSavedPagesFeed(feeds); err != nil {
		return domain.Item{}, err
	}
	return s.appendSavedPage(ctx, normalized, bookmarkID)
}

// requireBookmark 指定IDのラベルが存在することを確かめます。
func (s *BookmarkService) requireBookmark(bookmarkID string) error {
	bms, err := s.deps.Repo.Bookmarks()
	if err != nil {
		return fmt.Errorf("failed to load bookmarks: %w", err)
	}
	for _, b := range bms {
		if b.ID == bookmarkID {
			return nil
		}
	}
	return fmt.Errorf("bookmark %q: %w", bookmarkID, ErrBookmarkNotFound)
}

// bookmarkExistingItem 正規化URLに一致する既存記事を探し、見つかれば保存済みにして返します。
// 合成フィードの記事も探索対象に含むため、同じURLを二度追加しても記事は増えません。
func (s *BookmarkService) bookmarkExistingItem(feeds []domain.Feed, normalized, bookmarkID string) (domain.Item, bool, error) {
	for _, f := range feeds {
		items, err := s.deps.Repo.Items(f.ID)
		if err != nil {
			return domain.Item{}, false, fmt.Errorf("failed to load items for feed %s: %w", f.ID, err)
		}
		for _, it := range items {
			if !sameNormalizedURL(it.Link, normalized) {
				continue
			}
			updated, err := s.markSaved(f.ID, it.ID, bookmarkID)
			if err != nil {
				return domain.Item{}, false, err
			}
			return updated, true, nil
		}
	}
	return domain.Item{}, false, nil
}

// sameNormalizedURL 記事のリンクが対象の正規化URLと同じページを指すかどうかを返します。
// 正規化できないリンクは一致しないものとして扱います。
func sameNormalizedURL(link, normalized string) bool {
	got, err := normalizeURL(link)
	if err != nil {
		return false
	}
	return got == normalized
}

// markSaved 指定記事を保存済みにし、ラベル指定があれば所属させて、更新後の記事を返します。
// ラベル指定があるときはaddBookmarkだけを呼びます。addBookmarkは所属を足すと保存状態も立てるため、
// 書き込みが1回で済み、2回目が失敗してラベルなしで保存だけ残る中途半端な状態を作りません。
func (s *BookmarkService) markSaved(feedID, itemID, bookmarkID string) (domain.Item, error) {
	mutate := func() error { return s.items.SetBookmarked(feedID, itemID, true) }
	if bookmarkID != "" {
		mutate = func() error { return s.items.addBookmark(feedID, itemID, bookmarkID) }
	}
	if err := mutate(); err != nil {
		return domain.Item{}, err
	}
	items, err := s.deps.Repo.Items(feedID)
	if err != nil {
		return domain.Item{}, fmt.Errorf("failed to load items for feed %s: %w", feedID, err)
	}
	for _, it := range items {
		if it.ID == itemID {
			return it, nil
		}
	}
	return domain.Item{}, ErrItemNotFound
}

// ensureSavedPagesFeed 合成フィードが無ければ作成します。
// 購読フィードではないため、ポーリング間隔は手動のみにします。
// 存在の判定には読み込み済みのフィード一覧を使います。
// Feed(id)の戻り値で判定すると、読み込み失敗と不在を区別できず、
// 一時的な読み込み失敗のたびに既存の合成フィードを作り直して上書きしてしまいます。
func (s *BookmarkService) ensureSavedPagesFeed(feeds []domain.Feed) error {
	for _, f := range feeds {
		if domain.IsSavedPagesFeed(f.ID) {
			return nil
		}
	}
	f := domain.Feed{
		ID:           domain.SavedPagesFeedID,
		Title:        domain.SavedPagesFeedTitle,
		PollInterval: domain.PollManualOnly,
	}
	if err := s.deps.Repo.SaveFeed(f); err != nil {
		return fmt.Errorf("failed to create saved pages feed: %w", err)
	}
	return nil
}

// appendSavedPage 合成フィードの先頭に保存ページの記事を1件積みます。
// タイトル取得には最大addURLFetchTimeoutかかるため、取得の前後で重複を確かめます。
// 取得中に同じURLが別のリクエストで保存されていた場合は、新しく積まずにその記事を返します。
func (s *BookmarkService) appendSavedPage(ctx context.Context, normalized, bookmarkID string) (domain.Item, error) {
	title := s.fetchTitle(ctx, normalized)

	existing, err := s.deps.Repo.Items(domain.SavedPagesFeedID)
	if err != nil {
		return domain.Item{}, fmt.Errorf("failed to load saved pages items: %w", err)
	}
	for _, it := range existing {
		if sameNormalizedURL(it.Link, normalized) {
			return s.markSaved(domain.SavedPagesFeedID, it.ID, bookmarkID)
		}
	}

	now := s.deps.Clock.Now()
	item := domain.Item{
		ID:          s.deps.IDs.NewID(),
		FeedID:      domain.SavedPagesFeedID,
		GUID:        normalized,
		Title:       title,
		Link:        normalized,
		PublishedAt: now,
		FetchedAt:   now,
		Bookmarked:  true,
	}
	if bookmarkID != "" {
		item.BookmarkIDs = []string{bookmarkID}
	}
	next := append([]domain.Item{item}, existing...)
	if err := s.deps.Repo.SaveItems(domain.SavedPagesFeedID, next); err != nil {
		return domain.Item{}, fmt.Errorf("failed to save saved pages items: %w", err)
	}
	return item, nil
}

// fetchTitle 対象ページを取得してタイトルを返します。
// 取得できない場合やHTMLでない場合やタイトルが空の場合はURLをそのまま返します。
// 保存自体は成功させたいため、エラーは返さずログにだけ残します。
func (s *BookmarkService) fetchTitle(ctx context.Context, pageURL string) string {
	ctx, cancel := context.WithTimeout(ctx, addURLFetchTimeout)
	defer cancel()
	res, err := s.deps.Fetch.Fetch(ctx, port.FetchRequest{URL: pageURL})
	if err != nil {
		// クエリ文字列に資格情報が混じり得るため、URL全体は残さずホスト名だけにします。
		slog.Warn("failed to fetch page for title", "host", hostOf(pageURL), "error", err)
		return pageURL
	}
	if !isHTMLContentType(res.ContentType) {
		return pageURL
	}
	if title := feed.ExtractMeta(res.Body, res.ContentType).Title; title != "" {
		return title
	}
	return pageURL
}

// isHTMLContentType Content-TypeがHTMLを表すかどうかを返します。
// パラメータ付き(text/html; charset=utf-8)にも対応します。
func isHTMLContentType(contentType string) bool {
	mediaType, _, err := mime.ParseMediaType(contentType)
	if err != nil {
		return false
	}
	return mediaType == "text/html" || mediaType == "application/xhtml+xml"
}

// hostOf ログに出しても差し支えのないホスト名を返します。解析できない場合は空文字を返します。
func hostOf(rawURL string) string {
	u, err := url.Parse(rawURL)
	if err != nil {
		return ""
	}
	return u.Hostname()
}
