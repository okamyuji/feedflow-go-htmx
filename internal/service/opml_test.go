package service_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/okamyuji/feedflow-go-htmx/internal/domain"
	"github.com/okamyuji/feedflow-go-htmx/internal/port"
	"github.com/okamyuji/feedflow-go-htmx/internal/service"
)

var _ port.OPMLService = (*service.OPMLService)(nil)

func newOPMLDeps(repo *fakeRepo, fetch *fakeFetcher, parse fakeParser, now time.Time) service.Deps {
	return newDeps(repo, fetch, parse, now, &fakeIDGen{})
}

func TestOPMLServiceImport(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 5, 29, 12, 0, 0, 0, time.UTC)
	repo := newFakeRepo()
	fetch := newFakeFetcher()
	fetch.results["https://a.example/feed"] = port.FetchResult{StatusCode: 200, Body: []byte("<rss></rss>")}
	fetch.results["https://b.example/feed"] = port.FetchResult{StatusCode: 200, Body: []byte("<rss></rss>")}
	parse := fakeParser{parsed: port.ParsedFeed{Format: port.FormatRSS2, Title: "imported"}}
	deps := newOPMLDeps(repo, fetch, parse, now)

	sub := service.NewSubscriptionService(deps)
	svc := service.NewOPMLService(deps, sub)

	opml := `<?xml version="1.0" encoding="UTF-8"?>
<opml version="2.0">
  <head><title>subs</title></head>
  <body>
    <outline text="cat1">
      <outline type="rss" text="A" xmlUrl="https://a.example/feed"/>
      <outline type="rss" text="B" xmlUrl="https://b.example/feed"/>
    </outline>
  </body>
</opml>`

	count, err := svc.Import(context.Background(), []byte(opml))
	if err != nil {
		t.Fatalf("Import returned error: %v", err)
	}
	if count != 2 {
		t.Fatalf("imported count got %d want 2", count)
	}
	feeds, _ := repo.Feeds()
	if len(feeds) != 2 {
		t.Fatalf("repo feeds got %d want 2", len(feeds))
	}
}

func TestOPMLServiceImportSkipsDuplicates(t *testing.T) {
	t.Parallel()
	now := time.Now()
	repo := newFakeRepo()
	_ = repo.SaveFeed(domain.Feed{ID: "f0", FeedURL: "https://a.example/feed"})
	fetch := newFakeFetcher()
	fetch.results["https://b.example/feed"] = port.FetchResult{StatusCode: 200, Body: []byte("<rss></rss>")}
	parse := fakeParser{parsed: port.ParsedFeed{Format: port.FormatRSS2, Title: "imported"}}
	deps := newOPMLDeps(repo, fetch, parse, now)

	sub := service.NewSubscriptionService(deps)
	svc := service.NewOPMLService(deps, sub)

	opml := `<opml version="2.0"><body>
    <outline type="rss" xmlUrl="https://a.example/feed"/>
    <outline type="rss" xmlUrl="https://b.example/feed"/>
  </body></opml>`

	count, err := svc.Import(context.Background(), []byte(opml))
	if err != nil {
		t.Fatalf("Import returned error: %v", err)
	}
	if count != 1 {
		t.Fatalf("imported count got %d want 1 (duplicate skipped)", count)
	}
}

func TestOPMLServiceImportInvalidXML(t *testing.T) {
	t.Parallel()
	now := time.Now()
	repo := newFakeRepo()
	deps := newOPMLDeps(repo, newFakeFetcher(), fakeParser{}, now)
	sub := service.NewSubscriptionService(deps)
	svc := service.NewOPMLService(deps, sub)

	if _, err := svc.Import(context.Background(), []byte("not xml at all <<<")); err == nil {
		t.Fatalf("Import must return error for invalid xml")
	}
}

func TestOPMLServiceExport(t *testing.T) {
	t.Parallel()
	now := time.Now()
	repo := newFakeRepo()
	_ = repo.SaveFeed(domain.Feed{ID: "f1", Title: "Feed One", FeedURL: "https://a.example/feed", SiteURL: "https://a.example"})
	_ = repo.SaveFeed(domain.Feed{ID: "f2", Title: "Feed Two", FeedURL: "https://b.example/feed", SiteURL: "https://b.example"})
	deps := newOPMLDeps(repo, newFakeFetcher(), fakeParser{}, now)
	sub := service.NewSubscriptionService(deps)
	svc := service.NewOPMLService(deps, sub)

	data, err := svc.Export()
	if err != nil {
		t.Fatalf("Export returned error: %v", err)
	}
	out := string(data)
	if !strings.Contains(out, `xmlUrl="https://a.example/feed"`) {
		t.Fatalf("export missing feed A: %s", out)
	}
	if !strings.Contains(out, `xmlUrl="https://b.example/feed"`) {
		t.Fatalf("export missing feed B: %s", out)
	}
	if !strings.Contains(out, `text="Feed One"`) {
		t.Fatalf("export missing title: %s", out)
	}
	if !strings.HasPrefix(out, "<?xml") {
		t.Fatalf("export must start with xml declaration: %s", out)
	}
}

func TestOPMLServiceExportImportRoundTrip(t *testing.T) {
	t.Parallel()
	now := time.Now()

	srcRepo := newFakeRepo()
	_ = srcRepo.SaveFeed(domain.Feed{ID: "f1", Title: "Feed One", FeedURL: "https://a.example/feed"})
	srcDeps := newOPMLDeps(srcRepo, newFakeFetcher(), fakeParser{}, now)
	srcSvc := service.NewOPMLService(srcDeps, service.NewSubscriptionService(srcDeps))
	data, err := srcSvc.Export()
	if err != nil {
		t.Fatalf("Export returned error: %v", err)
	}

	dstRepo := newFakeRepo()
	fetch := newFakeFetcher()
	fetch.results["https://a.example/feed"] = port.FetchResult{StatusCode: 200, Body: []byte("<rss></rss>")}
	dstDeps := newOPMLDeps(dstRepo, fetch, fakeParser{parsed: port.ParsedFeed{Title: "Feed One"}}, now)
	dstSvc := service.NewOPMLService(dstDeps, service.NewSubscriptionService(dstDeps))

	count, err := dstSvc.Import(context.Background(), data)
	if err != nil {
		t.Fatalf("Import returned error: %v", err)
	}
	if count != 1 {
		t.Fatalf("round-trip import count got %d want 1", count)
	}
}

func TestOPMLServiceImportContinuesOnFailure(t *testing.T) {
	t.Parallel()
	now := time.Now()
	repo := newFakeRepo()
	fetch := newFakeFetcher()
	fetch.results["https://a.example/feed"] = port.FetchResult{StatusCode: 200, Body: []byte("<rss></rss>")}
	parse := fakeParser{parsed: port.ParsedFeed{Format: port.FormatRSS2, Title: "imported"}}
	deps := newOPMLDeps(repo, fetch, parse, now)
	sub := service.NewSubscriptionService(deps)
	svc := service.NewOPMLService(deps, sub)

	opml := `<opml version="2.0"><body>
	    <outline type="rss" xmlUrl="https://a.example/feed"/>
	    <outline type="rss" xmlUrl="https://b.example/unreachable"/>
	    <outline type="rss" xmlUrl="https://c.example/unreachable"/>
	  </body></opml>`

	count, err := svc.Import(context.Background(), []byte(opml))
	if err != nil {
		t.Fatalf("Import は個別フィードの失敗で全体をエラーにせず継続すべきですが error: %v", err)
	}
	if count != 1 {
		t.Fatalf("imported count got %d want 1 (到達可能な a のみ成功)", count)
	}
	feeds, _ := repo.Feeds()
	if len(feeds) != 1 {
		t.Fatalf("repo feeds got %d want 1", len(feeds))
	}
}
