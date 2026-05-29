package handler

import (
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"io"
	"io/fs"
	"net/http"
)

//go:embed static
var staticFS embed.FS

// staticHandler 埋め込みの静的ファイルを/static配下で配信するハンドラを返します。
// 各ファイルのコンテンツハッシュをETagに用い、内容が変わると再取得され、
// 未変更ならIf-None-Matchで304を返します。デプロイ後の資産更新が確実に反映されます。
func staticHandler() http.Handler {
	sub, err := fs.Sub(staticFS, "static")
	if err != nil {
		// embed対象が存在しないビルド構成は想定外のためpanicで早期に検出します。
		panic("failed to create static sub fs: " + err.Error())
	}
	etags := computeETags(sub)
	fileServer := http.FileServer(http.FS(sub))
	return http.StripPrefix("/static/", cacheControl(etags, fileServer))
}

// computeETags 配信対象の各ファイルのSHA256ハッシュからETag値のマップを作ります。
// キーはStripPrefix後のリクエストパス(先頭スラッシュなし)に合わせます。
// 起動時に1度だけ計算します。個別ファイルの読み取り失敗はETagなしで配信を続けます。
func computeETags(fsys fs.FS) map[string]string {
	etags := make(map[string]string)
	_ = fs.WalkDir(fsys, ".", func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil //nolint:nilerr // 個別の読み取り失敗は致命的でないため配信を継続します
		}
		f, openErr := fsys.Open(path)
		if openErr != nil {
			return nil
		}
		defer func() { _ = f.Close() }()
		h := sha256.New()
		if _, copyErr := io.Copy(h, f); copyErr != nil {
			return nil
		}
		etags[path] = `"` + hex.EncodeToString(h.Sum(nil))[:16] + `"`
		return nil
	})
	return etags
}

// cacheControl ETagによる再検証を行うミドルウェアです。
// 内容が変わると新しいETagになり再取得され、未変更ならIf-None-Matchで304を返します。
func cacheControl(etags map[string]string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if etag, ok := etags[r.URL.Path]; ok {
			w.Header().Set("ETag", etag)
			w.Header().Set("Cache-Control", "no-cache")
			if r.Header.Get("If-None-Match") == etag {
				w.WriteHeader(http.StatusNotModified)
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}
