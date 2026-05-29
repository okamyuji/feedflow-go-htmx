package service

import (
	"errors"
	"fmt"

	"github.com/okamyuji/feedflow-go-htmx/internal/domain"
)

// ErrInvalidSettings 設定値が妥当でないときに返すエラーです。
var ErrInvalidSettings = errors.New("invalid settings")

// SettingsService 設定の取得と更新を担います。port.SettingsServiceを満たします。
type SettingsService struct {
	deps Deps
}

// NewSettingsService 依存束を受け取りSettingsServiceを構築します。
func NewSettingsService(deps Deps) *SettingsService {
	return &SettingsService{deps: deps}
}

// Get 現在の設定を返します。
func (s *SettingsService) Get() (domain.Settings, error) {
	settings, err := s.deps.Repo.Settings()
	if err != nil {
		return domain.Settings{}, fmt.Errorf("failed to load settings: %w", err)
	}
	return settings, nil
}

// Update 設定を検証してから保存します。妥当でない場合は保存せずエラーを返します。
func (s *SettingsService) Update(settings domain.Settings) error {
	if !settings.Valid() {
		return ErrInvalidSettings
	}
	if err := s.deps.Repo.SaveSettings(settings); err != nil {
		return fmt.Errorf("failed to save settings: %w", err)
	}
	return nil
}
