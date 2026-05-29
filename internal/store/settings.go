package store

import (
	"fmt"

	"github.com/okamyuji/feedflow-go-htmx/internal/domain"
)

// Settings 現在の設定を返します。settings.jsonが未保存のときは既定値を返します。
func (s *Store) Settings() (domain.Settings, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.settings, nil
}

// SaveSettings 設定を保存し、settings.jsonをアトミックに書き出します。
func (s *Store) SaveSettings(settings domain.Settings) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	prev := s.settings
	s.settings = settings
	if err := writeJSONAtomic(s.path(settingsFile), s.settings); err != nil {
		s.settings = prev
		return fmt.Errorf("failed to save settings: %w", err)
	}
	return nil
}

// User 所有者ユーザーを返します。未登録の場合はゼロ値のUserを返します。
func (s *Store) User() (domain.User, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.user, nil
}

// SaveUser 所有者ユーザーを保存し、user.jsonをアトミックに書き出します。
func (s *Store) SaveUser(user domain.User) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	prev := s.user
	s.user = user
	if err := writeJSONAtomic(s.path(userFile), s.user); err != nil {
		s.user = prev
		return fmt.Errorf("failed to save user: %w", err)
	}
	return nil
}
