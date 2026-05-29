// Package service feedflowの業務ロジックを提供します。
// 各サービスはinternal/portのインターフェースにのみ依存し、具象型に直接依存しません。
// 依存はコンストラクタ注入で受け取ります。設計書のセクション5.2に対応します。
package service

import "github.com/okamyuji/feedflow-go-htmx/internal/port"

// Deps 各サービスが共有する依存をまとめた束です。
// 外部I/Oはすべてこのインターフェース群経由で行い、テストではフェイクを注入します。
type Deps struct {
	Repo  port.Repository // 永続化境界です
	Fetch port.Fetcher    // HTTP取得境界です
	Parse port.FeedParser // フィードのパース境界です
	Clock port.Clock      // 時刻取得境界です
	IDs   port.IDGen      // ID生成境界です
}
