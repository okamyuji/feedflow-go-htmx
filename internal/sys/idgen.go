package sys

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
)

// idBytes 生成するIDの乱数バイト数です。16バイトで128ビットの一意性を確保します。
const idBytes = 16

// RandomIDGen 暗号論的乱数から一意なIDを生成するport.IDGenの実装です。
type RandomIDGen struct{}

// NewRandomIDGen RandomIDGenを生成します。
func NewRandomIDGen() RandomIDGen {
	return RandomIDGen{}
}

// NewID 16バイトの乱数を16進文字列にしたIDを返します。
// crypto/randの読み取りはOSのエントロピー枯渇など致命的な状況でのみ失敗します。
// port.IDGenはerrorを返さない契約のため、その稀な失敗はpanicで顕在化させ、握り潰しません。
func (RandomIDGen) NewID() string {
	b := make([]byte, idBytes)
	if _, err := rand.Read(b); err != nil {
		panic(fmt.Sprintf("sys: failed to read crypto/rand for id: %v", err))
	}
	return hex.EncodeToString(b)
}
