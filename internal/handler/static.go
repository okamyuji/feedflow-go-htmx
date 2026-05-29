package handler

import (
	"embed"
	"io/fs"
	"net/http"
)

//go:embed static
var staticFS embed.FS

// staticHandler 埋め込みの静的ファイルを/static配下で配信するハンドラを返します。
// 静的資産は内容が固定のため長期キャッシュ可能なヘッダを付与します。
func staticHandler() http.Handler {
	sub, err := fs.Sub(staticFS, "static")
	if err != nil {
		// embed対象が存在しないビルド構成は想定外のためpanicで早期に検出します。
		panic("failed to create static sub fs: " + err.Error())
	}
	fileServer := http.FileServer(http.FS(sub))
	return http.StripPrefix("/static/", cacheControl(fileServer))
}

// cacheControl 静的ファイルに長期キャッシュのヘッダを付与するミドルウェアです。
func cacheControl(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "public, max-age=86400")
		next.ServeHTTP(w, r)
	})
}
