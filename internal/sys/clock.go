// Package sys feedflowの本番用の時刻源とID生成の具象実装を提供します。
// port.Clockとport.IDGenを満たし、main.goから各層へコンストラクタ注入します。
// テストではこれらを使わずフェイクを注入するため、この層は薄く保ちます。
package sys

import "time"

// SystemClock 実時刻を返すport.Clockの実装です。
type SystemClock struct{}

// Now 現在時刻を返します。
func (SystemClock) Now() time.Time {
	return time.Now()
}
