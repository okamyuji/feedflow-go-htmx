package store

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

type sample struct {
	Name  string `json:"name"`
	Count int    `json:"count"`
}

func TestWriteJSONAtomicAndReadJSON(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "sample.json")

	want := sample{Name: "feedflow", Count: 42}
	if err := writeJSONAtomic(path, want); err != nil {
		t.Fatalf("writeJSONAtomic returned error: %v", err)
	}

	var got sample
	if err := readJSON(path, &got); err != nil {
		t.Fatalf("readJSON returned error: %v", err)
	}
	if got != want {
		t.Fatalf("round trip got %+v want %+v", got, want)
	}
}

func TestWriteJSONAtomicLeavesNoTempFile(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "sample.json")

	if err := writeJSONAtomic(path, sample{Name: "a", Count: 1}); err != nil {
		t.Fatalf("writeJSONAtomic returned error: %v", err)
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("ReadDir returned error: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("dir entries got %d want 1 (no leftover temp file)", len(entries))
	}
	if entries[0].Name() != "sample.json" {
		t.Fatalf("entry got %q want sample.json", entries[0].Name())
	}
}

func TestWriteJSONAtomicOverwrites(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "sample.json")

	if err := writeJSONAtomic(path, sample{Name: "old", Count: 1}); err != nil {
		t.Fatalf("first write returned error: %v", err)
	}
	if err := writeJSONAtomic(path, sample{Name: "new", Count: 2}); err != nil {
		t.Fatalf("second write returned error: %v", err)
	}

	var got sample
	if err := readJSON(path, &got); err != nil {
		t.Fatalf("readJSON returned error: %v", err)
	}
	if (got != sample{Name: "new", Count: 2}) {
		t.Fatalf("overwrite got %+v want {new 2}", got)
	}
}

func TestWriteJSONAtomicProducesIndentedJSON(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "sample.json")

	if err := writeJSONAtomic(path, sample{Name: "a", Count: 1}); err != nil {
		t.Fatalf("writeJSONAtomic returned error: %v", err)
	}

	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile returned error: %v", err)
	}
	if !json.Valid(raw) {
		t.Fatalf("written bytes are not valid JSON: %q", raw)
	}
	if raw[len(raw)-1] != '\n' {
		t.Fatalf("written JSON does not end with newline: %q", raw)
	}
}

func TestReadJSONMissingFileReturnsNotExist(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "missing.json")

	var got sample
	err := readJSON(path, &got)
	if err == nil {
		t.Fatal("readJSON got nil error want non-nil for missing file")
	}
	if !os.IsNotExist(err) {
		t.Fatalf("readJSON error got %v want os.IsNotExist to be true", err)
	}
}
