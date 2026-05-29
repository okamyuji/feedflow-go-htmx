package handler

import (
	"bytes"
	"context"
	"errors"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/okamyuji/feedflow-go-htmx/internal/domain"
)

// stubSettings SettingsServiceのスタブです。
type stubSettings struct {
	current   domain.Settings
	updateErr error
	updated   domain.Settings
}

func (s *stubSettings) Get() (domain.Settings, error) { return s.current, nil }
func (s *stubSettings) Update(settings domain.Settings) error {
	if s.updateErr != nil {
		return s.updateErr
	}
	s.updated = settings
	s.current = settings
	return nil
}

// stubOPML OPMLServiceのスタブです。
type stubOPML struct {
	imported  int
	exportOut []byte
}

func (s *stubOPML) Import(_ context.Context, _ []byte) (int, error) { return s.imported, nil }
func (s *stubOPML) Export() ([]byte, error)                         { return s.exportOut, nil }

func newSettingsHandler(t *testing.T, st *stubSettings, op *stubOPML) *Handler {
	t.Helper()
	h, err := New(Deps{
		Settings:          st,
		OPML:              op,
		Sessions:          &stubSessions{username: "owner", ok: true},
		CSRF:              &stubCSRF{ok: true, token: "tok"},
		SessionCookieName: "feedflow_session",
	})
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}
	return h
}

func TestSettingsPageRenders(t *testing.T) {
	t.Parallel()
	st := &stubSettings{current: domain.DefaultSettings()}
	h := newSettingsHandler(t, st, &stubOPML{})
	req := httptest.NewRequest(http.MethodGet, "/app/settings", nil)
	req = withSession(req, Session{Username: "owner", CSRFToken: "tok"})
	rec := httptest.NewRecorder()

	h.settingsPage(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status got %d want %d", rec.Code, http.StatusOK)
	}
	if !strings.Contains(rec.Body.String(), "設定") {
		t.Fatalf("body should render settings: %q", rec.Body.String())
	}
}

func TestSettingsUpdateSuccess(t *testing.T) {
	t.Parallel()
	st := &stubSettings{current: domain.DefaultSettings()}
	h := newSettingsHandler(t, st, &stubOPML{})
	form := url.Values{
		"poll_interval":       {"1h"},
		"max_items":           {"100"},
		"read_retention_days": {"14"},
		"theme":               {"light"},
		"default_view":        {"magazine"},
	}
	req := httptest.NewRequest(http.MethodPost, "/app/settings", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req = withSession(req, Session{Username: "owner", CSRFToken: "tok"})
	rec := httptest.NewRecorder()

	h.settingsUpdate(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status got %d want %d", rec.Code, http.StatusOK)
	}
	if st.updated.MaxItems != 100 {
		t.Fatalf("updated MaxItems got %d want 100", st.updated.MaxItems)
	}
	if st.updated.Theme != domain.ThemeLight {
		t.Fatalf("updated Theme got %q want %q", st.updated.Theme, domain.ThemeLight)
	}
}

func TestSettingsUpdateInvalidShowsError(t *testing.T) {
	t.Parallel()
	st := &stubSettings{current: domain.DefaultSettings(), updateErr: errors.New("invalid settings")}
	h := newSettingsHandler(t, st, &stubOPML{})
	form := url.Values{
		"poll_interval":       {"30m"},
		"max_items":           {"0"},
		"read_retention_days": {"30"},
		"theme":               {"dark"},
		"default_view":        {"card"},
	}
	req := httptest.NewRequest(http.MethodPost, "/app/settings", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req = withSession(req, Session{Username: "owner", CSRFToken: "tok"})
	rec := httptest.NewRecorder()

	h.settingsUpdate(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status got %d want %d", rec.Code, http.StatusBadRequest)
	}
	if !strings.Contains(rec.Body.String(), "設定を保存できませんでした") {
		t.Fatalf("body should contain error: %q", rec.Body.String())
	}
}

func TestOPMLExport(t *testing.T) {
	t.Parallel()
	op := &stubOPML{exportOut: []byte(`<opml version="2.0"></opml>`)}
	h := newSettingsHandler(t, &stubSettings{current: domain.DefaultSettings()}, op)
	req := httptest.NewRequest(http.MethodGet, "/app/opml/export", nil)
	req = withSession(req, Session{Username: "owner", CSRFToken: "tok"})
	rec := httptest.NewRecorder()

	h.opmlExport(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status got %d want %d", rec.Code, http.StatusOK)
	}
	if ct := rec.Header().Get("Content-Type"); !strings.Contains(ct, "xml") {
		t.Fatalf("Content-Type got %q want xml", ct)
	}
	if !strings.Contains(rec.Body.String(), "opml") {
		t.Fatalf("body should contain opml: %q", rec.Body.String())
	}
}

func TestOPMLImport(t *testing.T) {
	t.Parallel()
	op := &stubOPML{imported: 3}
	h := newSettingsHandler(t, &stubSettings{current: domain.DefaultSettings()}, op)

	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
	part, err := mw.CreateFormFile("opml", "feeds.opml")
	if err != nil {
		t.Fatalf("CreateFormFile returned error: %v", err)
	}
	if _, err := part.Write([]byte(`<opml version="2.0"></opml>`)); err != nil {
		t.Fatalf("write part returned error: %v", err)
	}
	if err := mw.Close(); err != nil {
		t.Fatalf("close writer returned error: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/app/opml/import", &body)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	req = withSession(req, Session{Username: "owner", CSRFToken: "tok"})
	rec := httptest.NewRecorder()

	h.opmlImport(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status got %d want %d", rec.Code, http.StatusOK)
	}
	if !strings.Contains(rec.Body.String(), "3") {
		t.Fatalf("body should report import count: %q", rec.Body.String())
	}
}
