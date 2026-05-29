// Package port feedflowの各層境界となるインターフェースを定義します。
// このパッケージはinternal/domainにのみ依存し、具体的な実装には依存しません。
package port

import "time"

// Clock 現在時刻を返す抽象です。テストでは固定時刻を返すフェイクを注入します。
type Clock interface {
	// Now 現在時刻を返します。
	Now() time.Time
}
