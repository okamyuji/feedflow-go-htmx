package service_test

import (
	"testing"
	"time"

	"github.com/okamyuji/feedflow-go-htmx/internal/domain"
	"github.com/okamyuji/feedflow-go-htmx/internal/port"
	"github.com/okamyuji/feedflow-go-htmx/internal/service"
)

var _ port.MuteService = (*service.MuteService)(nil)

func TestMuteServiceAddFilter(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		keyword string
		scope   domain.MuteScope
		feedID  string
		wantErr bool
	}{
		{name: "全体フィルタを追加する", keyword: "広告", scope: domain.MuteScopeGlobal, feedID: "", wantErr: false},
		{name: "フィード限定フィルタを追加する", keyword: "PR", scope: domain.MuteScopeFeed, feedID: "f1", wantErr: false},
		{name: "空キーワードは拒否する", keyword: "", scope: domain.MuteScopeGlobal, feedID: "", wantErr: true},
		{name: "不正な対象範囲は拒否する", keyword: "x", scope: domain.MuteScope("other"), feedID: "", wantErr: true},
		{name: "フィード限定でFeedID空は拒否する", keyword: "x", scope: domain.MuteScopeFeed, feedID: "", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			repo := newFakeRepo()
			svc := service.NewMuteService(newDeps(repo, newFakeFetcher(), fakeParser{}, time.Now(), &fakeIDGen{}))

			f, err := svc.AddFilter(tt.keyword, tt.scope, tt.feedID)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("AddFilter must return error")
				}
				if len(repo.filters) != 0 {
					t.Fatalf("no filter must be saved on error")
				}
				return
			}
			if err != nil {
				t.Fatalf("AddFilter returned error: %v", err)
			}
			if f.ID == "" {
				t.Fatalf("AddFilter must assign an ID")
			}
			if _, ok := repo.filters[f.ID]; !ok {
				t.Fatalf("AddFilter must persist the filter")
			}
		})
	}
}

func TestMuteServiceListFilters(t *testing.T) {
	t.Parallel()
	repo := newFakeRepo()
	repo.filters["x1"] = domain.MuteFilter{ID: "x1", Keyword: "広告", Scope: domain.MuteScopeGlobal}
	svc := service.NewMuteService(newDeps(repo, newFakeFetcher(), fakeParser{}, time.Now(), &fakeIDGen{}))

	got, err := svc.ListFilters()
	if err != nil {
		t.Fatalf("ListFilters returned error: %v", err)
	}
	if len(got) != 1 || got[0].ID != "x1" {
		t.Fatalf("ListFilters got %+v", got)
	}
}

func TestMuteServiceDeleteFilter(t *testing.T) {
	t.Parallel()
	repo := newFakeRepo()
	repo.filters["x1"] = domain.MuteFilter{ID: "x1", Keyword: "広告", Scope: domain.MuteScopeGlobal}
	svc := service.NewMuteService(newDeps(repo, newFakeFetcher(), fakeParser{}, time.Now(), &fakeIDGen{}))

	if err := svc.DeleteFilter("x1"); err != nil {
		t.Fatalf("DeleteFilter returned error: %v", err)
	}
	if _, ok := repo.filters["x1"]; ok {
		t.Fatalf("DeleteFilter must remove the filter")
	}
}

func TestMuteServiceFilter(t *testing.T) {
	t.Parallel()
	repo := newFakeRepo()
	repo.filters["g1"] = domain.MuteFilter{ID: "g1", Keyword: "広告", Scope: domain.MuteScopeGlobal}
	repo.filters["f1"] = domain.MuteFilter{ID: "f1", Keyword: "PR", Scope: domain.MuteScopeFeed, FeedID: "feedA"}
	svc := service.NewMuteService(newDeps(repo, newFakeFetcher(), fakeParser{}, time.Now(), &fakeIDGen{}))

	items := []domain.Item{
		{ID: "i1", FeedID: "feedA", Title: "本日の広告まとめ"}, // 全体フィルタで除外
		{ID: "i2", FeedID: "feedA", Title: "これはPRです"},  // フィード限定で除外
		{ID: "i3", FeedID: "feedB", Title: "これはPRです"},  // 対象外フィードなので残る
		{ID: "i4", FeedID: "feedB", Title: "通常の技術記事"},  // どのフィルタにも一致せず残る
	}
	got, err := svc.Filter(items)
	if err != nil {
		t.Fatalf("Filter returned error: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("Filter kept %d items, want 2: %+v", len(got), got)
	}
	if got[0].ID != "i3" || got[1].ID != "i4" {
		t.Fatalf("Filter kept wrong items: %+v", got)
	}
}

func TestMuteServiceFilterEmptyFilters(t *testing.T) {
	t.Parallel()
	repo := newFakeRepo()
	svc := service.NewMuteService(newDeps(repo, newFakeFetcher(), fakeParser{}, time.Now(), &fakeIDGen{}))

	items := []domain.Item{{ID: "i1", FeedID: "f", Title: "なんでも"}}
	got, err := svc.Filter(items)
	if err != nil {
		t.Fatalf("Filter returned error: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("Filter with no filters must keep all items, got %d", len(got))
	}
}
