// Package main feedflowのエントリポイントを提供します。
// 設定を環境変数から読み、各層の具象を生成してインターフェース経由で注入し、
// HTTPサーバとバックグラウンドのポーラーを起動します。
package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/okamyuji/feedflow-go-htmx/internal/auth"
	"github.com/okamyuji/feedflow-go-htmx/internal/feed"
	"github.com/okamyuji/feedflow-go-htmx/internal/handler"
	"github.com/okamyuji/feedflow-go-htmx/internal/poller"
	"github.com/okamyuji/feedflow-go-htmx/internal/service"
	"github.com/okamyuji/feedflow-go-htmx/internal/store"
	"github.com/okamyuji/feedflow-go-htmx/internal/sys"
)

// version ビルド時に-ldflagsで埋め込むバージョン文字列です。
var version = "dev"

// sessionCookieName セッションIDを載せるCookie名です。
const sessionCookieName = "feedflow_session"

// sessionTTL セッションの有効期間です。個人利用のため長めに取ります。
const sessionTTL = 30 * 24 * time.Hour

// loginBurst ログイン試行で同時に許す回数です。
const loginBurst = 5

// loginRefill ログイン試行トークンを1個補充する間隔です。
const loginRefill = time.Minute

func main() {
	if err := run(); err != nil {
		log.Fatalf("feedflow exited with error: %v", err)
	}
}

// run アプリを組み立ててサーバとポーラーを起動し、終了シグナルまで動かします。
func run() error {
	addr := envOr("FEEDFLOW_ADDR", ":8080")

	routes, runner, err := buildApp()
	if err != nil {
		return err
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	go runner.Run(ctx)

	srv := &http.Server{
		Addr:              addr,
		Handler:           routes,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	errCh := make(chan error, 1)
	go func() {
		log.Printf("feedflow %s listening on %s", version, addr)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
		}
	}()

	select {
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := srv.Shutdown(shutdownCtx); err != nil {
			return fmt.Errorf("failed to shut down server: %w", err)
		}
		return nil
	case err := <-errCh:
		return fmt.Errorf("server error: %w", err)
	}
}

// buildApp 設定から全依存を組み立て、ルーティング済みハンドラとポーラーを返します。
// 各層はインターフェース経由で注入し、具象はこの関数だけが知ります。
func buildApp() (http.Handler, *poller.Runner, error) {
	dataDir := envOr("FEEDFLOW_DATA_DIR", "./data")
	baseURL := envOr("FEEDFLOW_BASE_URL", "")
	isHTTPS := strings.HasPrefix(baseURL, "https://")

	repo, err := store.New(dataDir)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to open store at %s: %w", dataDir, err)
	}

	clock := sys.SystemClock{}
	ids := sys.NewRandomIDGen()
	// 既定ではSSRF対策でプライベートやループバック宛の取得を拒否します。
	// ローカルのE2Eはループバックのテスト用フィードサーバを使うため、FEEDFLOW_ALLOW_PRIVATE_FETCHで許可します。
	allowPrivateFetch := envOr("FEEDFLOW_ALLOW_PRIVATE_FETCH", "") != ""
	fetcher := feed.NewHTTPFetcher(feed.WithAllowPrivateAddresses(allowPrivateFetch))
	parser := feed.NewXMLParser()

	sdeps := service.Deps{Repo: repo, Fetch: fetcher, Parse: parser, Clock: clock, IDs: ids}
	mute := service.NewMuteService(sdeps)
	subs := service.NewSubscriptionService(sdeps)
	items := service.NewItemService(sdeps, mute)
	bookmarks := service.NewBookmarkService(sdeps, items)
	retention := service.NewRetentionService(sdeps)
	opml := service.NewOPMLService(sdeps, subs)
	settings := service.NewSettingsService(sdeps)
	pollSvc := poller.NewService(repo, fetcher, parser, clock, ids, mute)
	runner := poller.NewRunner(pollSvc, repo, clock, poller.DefaultConfig())

	sessions := auth.NewSessionStore(auth.SessionConfig{
		Clock:      clock,
		TTL:        sessionTTL,
		CookieName: sessionCookieName,
		Secure:     isHTTPS,
	})
	csrf := auth.NewCSRFStore()
	// ログイン試行のレート制限はenvで上書きできます。E2Eのように短時間で多数ログインする環境では
	// FEEDFLOW_LOGIN_BURSTを大きくして枯渇を防ぎます。既定は本番向けの控えめな値です。
	limiter := auth.NewRateLimiter(auth.RateLimitConfig{
		Clock:       clock,
		Burst:       envIntOr("FEEDFLOW_LOGIN_BURST", loginBurst),
		RefillEvery: loginRefill,
	})
	manager := auth.NewManager(repo, auth.DefaultParams())

	h, err := handler.New(handler.Deps{
		Subscriptions:     subs,
		Items:             items,
		Bookmarks:         bookmarks,
		Retention:         retention,
		Mutes:             mute,
		OPML:              opml,
		Settings:          settings,
		Poll:              pollSvc,
		Sessions:          sessions,
		CSRF:              csrf,
		LoginLimiter:      limiter,
		Setup:             manager,
		SessionCookieName: sessionCookieName,
		IsHTTPS:           isHTTPS,
	})
	if err != nil {
		return nil, nil, fmt.Errorf("failed to build handler: %w", err)
	}

	return h.Routes(), runner, nil
}

// envOr 環境変数keyの値を返します。未設定や空のときはdefを返します。
func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

// envIntOr 環境変数keyを正の整数として返します。未設定や空や不正値のときはdefを返します。
func envIntOr(key string, def int) int {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			return n
		}
	}
	return def
}
