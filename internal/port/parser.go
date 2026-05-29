package port

import "time"

// FeedFormat パースで判別したフィードの形式を表します。
type FeedFormat string

// フィード形式の取りうる値です。設計書のセクション8に対応します。
const (
	FormatRSS2 FeedFormat = "rss2" // RSS 2.0です
	FormatAtom FeedFormat = "atom" // Atomです
	FormatRDF  FeedFormat = "rdf"  // RDFつまりRSS 1.0です
)

// ParsedItem パース段階の記事の中間表現です。
// 永続化用のIDやFeedIDの付与はサービス層が担うため、ここには含めません。
type ParsedItem struct {
	GUID        string    // フィード内での記事の一意キーです
	Title       string    // 記事のタイトルです
	Link        string    // 元記事のURLです
	Content     string    // 記事本文です
	Summary     string    // 記事の要約です
	Author      string    // 著者名です
	PublishedAt time.Time // 公開日時です
}

// ParsedFeed パース結果のフィード全体です。
type ParsedFeed struct {
	Format  FeedFormat   // 判別したフィード形式です
	Title   string       // フィードのタイトルです
	SiteURL string       // フィードが指すサイトのURLです
	Items   []ParsedItem // パースした記事群です
}

// FeedParser バイト列を受け取り形式を判別してフィードをパースする抽象です。
// 設計書のセクション8に対応します。
type FeedParser interface {
	// Parse 与えられたバイト列をRSS 2.0とAtomとRDFのいずれかとして判別しパースします。
	// 判別に失敗した場合やパースに失敗した場合はエラーを返します。
	Parse(data []byte) (ParsedFeed, error)
}
