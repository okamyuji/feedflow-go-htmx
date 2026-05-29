// Package main feedflowのエントリポイントを提供します。
package main

import (
	"log"
	"net/http"
	"time"
)

// versionビルド時に-ldflagsで埋め込むバージョン文字列です。
var version = "dev"

func main() {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", healthz)

	srv := &http.Server{
		Addr:              ":8080",
		Handler:           mux,
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

// healthz死活監視用のエンドポイントを返します。
func healthz(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	_, _ = w.Write([]byte("ok"))
}
