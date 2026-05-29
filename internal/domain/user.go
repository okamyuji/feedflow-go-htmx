package domain

// User アプリの所有者を表します。単一ユーザーの運用です。設計書のセクション6と9に対応します。
type User struct {
	Username     string `json:"username"`      // ログインに使うユーザー名です
	PasswordHash string `json:"password_hash"` // scryptで生成したパスワードハッシュです
}

// IsRegistered 所有者が登録済みかどうかを返します。
// ユーザー名とパスワードハッシュの両方が設定されているときに登録済みとみなします。
// 初回セットアップの可否判定の基礎になります。設計書のセクション9.3に対応します。
func (u User) IsRegistered() bool {
	return u.Username != "" && u.PasswordHash != ""
}
