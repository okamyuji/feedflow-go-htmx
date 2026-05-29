package port

// IDGen 一意な識別子を生成する抽象です。テストでは決定的な連番を返すフェイクを注入します。
type IDGen interface {
	// NewID 一意な識別子の文字列を返します。
	NewID() string
}
