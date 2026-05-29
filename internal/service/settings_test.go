package service_test

import (
	"testing"
	"time"

	"github.com/okamyuji/feedflow-go-htmx/internal/domain"
	"github.com/okamyuji/feedflow-go-htmx/internal/port"
	"github.com/okamyuji/feedflow-go-htmx/internal/service"
)

var _ port.SettingsService = (*service.SettingsService)(nil)

func TestSettingsServiceGet(t *testing.T) {
	t.Parallel()
	repo := newFakeRepo()
	want := domain.DefaultSettings()
	want.MaxItems = 123
	repo.settings = want
	svc := service.NewSettingsService(newDeps(repo, newFakeFetcher(), fakeParser{}, time.Now(), &fakeIDGen{}))

	got, err := svc.Get()
	if err != nil {
		t.Fatalf("Get returned error: %v", err)
	}
	if got.MaxItems != 123 {
		t.Fatalf("MaxItems got %d want 123", got.MaxItems)
	}
}

func TestSettingsServiceGetRepoError(t *testing.T) {
	t.Parallel()
	repo := newFakeRepo()
	repo.failOn["Settings"] = errNotFound
	svc := service.NewSettingsService(newDeps(repo, newFakeFetcher(), fakeParser{}, time.Now(), &fakeIDGen{}))

	if _, err := svc.Get(); err == nil {
		t.Fatalf("Get must return error when repo fails")
	}
}

func TestSettingsServiceUpdate(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name      string
		settings  domain.Settings
		wantErr   bool
		wantSaved bool
	}{
		{
			name:      "妥当な設定は保存する",
			settings:  domain.DefaultSettings(),
			wantErr:   false,
			wantSaved: true,
		},
		{
			name: "件数0は不正で保存しない",
			settings: func() domain.Settings {
				s := domain.DefaultSettings()
				s.MaxItems = 0
				return s
			}(),
			wantErr:   true,
			wantSaved: false,
		},
		{
			name: "テーマ 不正値は保存しない",
			settings: func() domain.Settings {
				s := domain.DefaultSettings()
				s.Theme = "neon"
				return s
			}(),
			wantErr:   true,
			wantSaved: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			repo := newFakeRepo()
			repo.settings = domain.Settings{} // 保存有無を見分けるためゼロ値から始めます
			svc := service.NewSettingsService(newDeps(repo, newFakeFetcher(), fakeParser{}, time.Now(), &fakeIDGen{}))

			err := svc.Update(tt.settings)
			if tt.wantErr && err == nil {
				t.Fatalf("Update must return error")
			}
			if !tt.wantErr && err != nil {
				t.Fatalf("Update returned error: %v", err)
			}
			saved := repo.settings != domain.Settings{}
			if saved != tt.wantSaved {
				t.Fatalf("saved got %v want %v", saved, tt.wantSaved)
			}
		})
	}
}
