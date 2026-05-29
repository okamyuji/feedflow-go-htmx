package domain

import "testing"

func TestUserIsRegistered(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		user User
		want bool
	}{
		{name: "未登録は空", user: User{}, want: false},
		{name: "名前のみでハッシュなしは未登録", user: User{Username: "owner"}, want: false},
		{name: "ハッシュのみで名前なしは未登録", user: User{PasswordHash: "abc"}, want: false},
		{name: "名前とハッシュありで登録済み", user: User{Username: "owner", PasswordHash: "abc"}, want: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := tt.user.IsRegistered(); got != tt.want {
				t.Fatalf("IsRegistered() got %v want %v", got, tt.want)
			}
		})
	}
}
