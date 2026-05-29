package poller

import (
	"context"
	"fmt"
	"time"

	"github.com/okamyuji/feedflow-go-htmx/internal/domain"
	"github.com/okamyuji/feedflow-go-htmx/internal/port"
)

// Service フィードの取得反映を担いport.PollServiceを満たします。
// repoとfetcherとparserとclockとidsとmuteとjitterをコンストラクタ注入で受け取ります。
type Service struct {
	repo    port.Repository
	fetcher port.Fetcher
	parser  port.FeedParser
	clock   port.Clock
	ids     port.IDGen
	mute    port.MuteService
	jitter  jitterFunc
}

// NewService 依存を注入してServiceを生成します。
// ジッタは既定の割合ジッタを用います。PollAllの期限判定で取得時刻を散らします。
func NewService(
	repo port.Repository,
	fetcher port.Fetcher,
	parser port.FeedParser,
	clock port.Clock,
	ids port.IDGen,
	mute port.MuteService,
) *Service {
	return &Service{
		repo:    repo,
		fetcher: fetcher,
		parser:  parser,
		clock:   clock,
		ids:     ids,
		mute:    mute,
		jitter:  ratioJitter(defaultJitterRatio),
	}
}

// PollFeed 指定フィードを取得し新着記事を反映して新着件数を返します。
// 取得に失敗した場合は連続エラー数を1増やして保存し、エラーを返します。
// サーバが未更新を示した場合は記事を増やさず最終取得時刻だけ更新します。
func (s *Service) PollFeed(ctx context.Context, feedID string) (int, error) {
	feed, err := s.repo.Feed(feedID)
	if err != nil {
		return 0, fmt.Errorf("failed to load feed %q: %w", feedID, err)
	}

	res, err := s.fetcher.Fetch(ctx, port.FetchRequest{
		URL:          feed.FeedURL,
		ETag:         feed.ETag,
		LastModified: feed.LastModified,
	})
	if err != nil {
		return 0, s.recordFetchError(feed, err)
	}

	now := s.clock.Now()
	if res.NotModified {
		return 0, s.recordNotModified(feed, now)
	}

	parsed, err := s.parser.Parse(res.Body)
	if err != nil {
		return 0, s.recordFetchError(feed, fmt.Errorf("failed to parse feed %q: %w", feedID, err))
	}

	added, err := s.applyParsed(feed, parsed, res, now)
	if err != nil {
		return 0, err
	}
	return added, nil
}

// PollAll 期限の来た全フィードを取得して反映し、処理したフィード数を返します。
// 期限判定は最終取得時刻と間隔から行い、手動のみのフィードは対象外とします。
// 個々のフィードの取得失敗は処理を止めず、処理を試みたフィード数を数えます。
func (s *Service) PollAll(ctx context.Context) (int, error) {
	feeds, err := s.repo.Feeds()
	if err != nil {
		return 0, fmt.Errorf("failed to load feeds: %w", err)
	}
	settings, err := s.repo.Settings()
	if err != nil {
		return 0, fmt.Errorf("failed to load settings: %w", err)
	}

	now := s.clock.Now()
	processed := 0
	for _, feed := range feeds {
		if err := ctx.Err(); err != nil {
			return processed, err
		}
		if !dueForPollWithJitter(feed, settings, now, s.jitter) {
			continue
		}
		processed++
		if _, err := s.PollFeed(ctx, feed.ID); err != nil {
			continue
		}
	}
	return processed, nil
}

// applyParsed パース結果を既存記事と突き合わせ新着を反映してフィードを更新します。
// 既存のGUID集合に無い記事だけを新着としてdomain.Itemへ写し、ID付与とFeedID紐付けを行います。
// 新着にミュートを適用してから既存記事の前に積んで保存します。
// あわせてフィードのタイトルとサイトURLとETagとLast-Modifiedと最終取得時刻を更新し、
// 連続エラー数を0へ戻します。戻り値は保存した新着の件数です。
func (s *Service) applyParsed(feed domain.Feed, parsed port.ParsedFeed, res port.FetchResult, now time.Time) (int, error) {
	existing, err := s.repo.Items(feed.ID)
	if err != nil {
		return 0, fmt.Errorf("failed to load items for feed %q: %w", feed.ID, err)
	}

	seen := make(map[string]struct{}, len(existing))
	for _, it := range existing {
		seen[it.GUID] = struct{}{}
	}

	fresh := make([]domain.Item, 0, len(parsed.Items))
	for _, pi := range parsed.Items {
		if _, ok := seen[pi.GUID]; ok {
			continue
		}
		seen[pi.GUID] = struct{}{}
		fresh = append(fresh, domain.Item{
			ID:          s.ids.NewID(),
			FeedID:      feed.ID,
			GUID:        pi.GUID,
			Title:       pi.Title,
			Link:        pi.Link,
			Content:     pi.Content,
			Summary:     pi.Summary,
			Author:      pi.Author,
			PublishedAt: pi.PublishedAt,
			FetchedAt:   now,
		})
	}

	fresh, err = s.mute.Filter(fresh)
	if err != nil {
		return 0, fmt.Errorf("failed to apply mute for feed %q: %w", feed.ID, err)
	}

	if len(fresh) > 0 {
		merged := make([]domain.Item, 0, len(fresh)+len(existing))
		merged = append(merged, fresh...)
		merged = append(merged, existing...)
		if err := s.repo.SaveItems(feed.ID, merged); err != nil {
			return 0, fmt.Errorf("failed to save items for feed %q: %w", feed.ID, err)
		}
	}

	updated := feed
	if parsed.Title != "" {
		updated.Title = parsed.Title
	}
	if parsed.SiteURL != "" {
		updated.SiteURL = parsed.SiteURL
	}
	updated.ETag = res.ETag
	updated.LastModified = res.LastModified
	updated.LastFetchedAt = now
	updated.ConsecutiveErrors = 0
	if err := s.repo.SaveFeed(updated); err != nil {
		return 0, fmt.Errorf("failed to save feed %q: %w", feed.ID, err)
	}

	return len(fresh), nil
}

// recordFetchError 取得や解析の失敗を連続エラー数へ反映してエラーを返します。
func (s *Service) recordFetchError(feed domain.Feed, cause error) error {
	updated := feed
	updated.ConsecutiveErrors = feed.ConsecutiveErrors + 1
	if saveErr := s.repo.SaveFeed(updated); saveErr != nil {
		return fmt.Errorf("failed to save feed after fetch error (%v): %w", cause, saveErr)
	}
	return fmt.Errorf("failed to poll feed %q: %w", feed.ID, cause)
}

// recordNotModified 未更新応答時に最終取得時刻を更新し連続エラー数を0へ戻します。
func (s *Service) recordNotModified(feed domain.Feed, now time.Time) error {
	updated := feed
	updated.LastFetchedAt = now
	updated.ConsecutiveErrors = 0
	if err := s.repo.SaveFeed(updated); err != nil {
		return fmt.Errorf("failed to save feed %q after not-modified: %w", feed.ID, err)
	}
	return nil
}
