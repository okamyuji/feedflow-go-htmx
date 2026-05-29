// Package main feedflowのエントリポイントを提供します。
package main

import (
	"log"
	"net/http"
	"time"

	"github.com/okamyuji/feedflow-go-htmx/internal/handler"
)

// version ビルド時に-ldflagsで埋め込むバージョン文字列です。
var version = "dev"

func main() {
	srvHandler, err := buildHandler()
	if err != nil {
		log.Fatalf("failed to build handler: %v", err)
	}

	srv := &http.Server{
		Addr:              ":8080",
		Handler:           srvHandler,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	log.Printf("feedflow %s listening on %s", version, srv.Addr)
	if err := srv.ListenAndServe(); err != nil {
		log.Fatalf("server stopped: %v", err)
	}
}

// buildHandler 依存を組み立ててルーティング済みハンドラを返します。
// 各依存の具象生成はPhase2からPhase6で確定し、ここでDepsへ注入します。
// 現時点では結線の骨組みとしてhandler.Newを呼び、Routesを返します。
func buildHandler() (http.Handler, error) {
	deps := handler.Deps{}
	h, err := handler.New(deps)
	if err != nil {
		return nil, err
	}
	return h.Routes(), nil
}
