package sys

import (
	"testing"
	"time"
)

func TestSystemClock_Now(t *testing.T) {
	before := time.Now()
	got := SystemClock{}.Now()
	after := time.Now()

	if got.Before(before) || got.After(after) {
		t.Fatalf("Now() %v は [%v, %v] の範囲外です", got, before, after)
	}
}

func TestRandomIDGen_NewID_NonEmptyAndLength(t *testing.T) {
	g := NewRandomIDGen()
	id := g.NewID()

	if id == "" {
		t.Fatal("NewID() が空文字列を返しました")
	}
	if len(id) != idBytes*2 {
		t.Fatalf("NewID() の長さ got %d want %d", len(id), idBytes*2)
	}
}

func TestRandomIDGen_NewID_Unique(t *testing.T) {
	g := NewRandomIDGen()
	const n = 1000
	seen := make(map[string]struct{}, n)
	for range n {
		id := g.NewID()
		if _, dup := seen[id]; dup {
			t.Fatalf("NewID() が重複を返しました id=%s", id)
		}
		seen[id] = struct{}{}
	}
}
