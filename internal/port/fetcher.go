package port

import "context"

// FetchRequest 条件付き取得のための入力です。
// 前回保存したETagとLast-Modifiedを渡すと、フェッチャはそれらを使い未更新かどうかを問い合わせます。
type FetchRequest struct {
	URL          string // 取得対象のURLです
	ETag         string // 前回取得時のETagです。空なら条件付けしません
	LastModified string // 前回取得時のLast-Modifiedです。空なら条件付けしません
}

// FetchResult 取得結果です。
// NotModifiedが真のときはBodyを持たず、サーバが未更新を示したことを表します。
type FetchResult struct {
	StatusCode   int    // HTTPのステータスコードです
	NotModified  bool   // サーバが304で未更新を示したかどうかです
	Body         []byte // 取得した本文です。NotModifiedが真のときは空です
	ContentType  string // レスポンスのContent-Typeです
	ETag         string // レスポンスのETagです
	LastModified string // レスポンスのLast-Modifiedです
}

// Fetcher URLの内容をETagとLast-Modifiedを考慮して取得する抽象です。
// SSRF対策やサイズ上限やタイムアウトは実装側で担います。設計書のセクション8に対応します。
type Fetcher interface {
	// Fetch 指定したリクエストに従い内容を取得します。
	// contextのキャンセルとタイムアウトを尊重します。
	Fetch(ctx context.Context, req FetchRequest) (FetchResult, error)
}
