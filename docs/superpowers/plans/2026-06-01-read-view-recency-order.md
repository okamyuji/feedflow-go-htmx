# Read View Recency Order Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 左メニュー「既読」で表示する記事を、公開日時の最新降順で安定して表示する。

**Architecture:** 既読ビューの表示モデル生成時だけ、フィルタ後の記事列を公開日時降順に並べ替える。永続化層、取得層、単一フィードの既読先頭表示、未読ストリーム、ブックマーク、あとで読むの挙動は変更しない。

**Tech Stack:** Go standard library, `net/http/httptest`, `html/template`, HTMX templates, existing handler tests.

---

## Root Cause

`Handler.listItemsFor` の `view=read` 分岐は `it.Read && !isBookmarked(it)` で絞り込むだけで、公開日時降順に並べていない。

`ItemService.ListItems("")` はフィード一覧順に各フィードの記事を連結する。各フィード内では `poller.Service.applyParsed` が新着を前に保存するが、全フィード横断では「フィードAの全記事、フィードBの全記事」の順になるため、フィードBにより新しい既読記事があっても後ろに出る。

## MECE Logic Tree

問題: 「既読」ビューの記事が古く見える

- 入力データ
  - RSSの `PublishedAt` が古い: パーサやフィード内容の問題。今回の症状は既読ビュー限定で、単一フィードの `withReadHead` は `PublishedAt` 降順を実装済み。
  - `FetchedAt` が古い: 同一 `PublishedAt` の二次キーには使うが、主キーは期待仕様の「最新記事」= 公開日時。
- 保存順
  - 各フィード内: `applyParsed` が新着を前に積むため大きな破綻はない。
  - 全フィード横断: `ListItems("")` がフィード順に連結するため、公開日時降順にはならない。
- 表示時フィルタ
  - 未読ストリーム: 既存仕様どおり未読のみ。今回の対象外。
  - 単一フィード既定表示: `withReadHead` が既読先頭群だけ公開日時降順。今回の対象外。
  - 既読ビュー: 既読抽出後にソートがない。今回の根本原因。
- テンプレート
  - `_item_list.html` は渡された `.Items` を順に描画するだけなので原因ではない。

## Regression Risk Control

- 変更範囲を `view=read` 分岐に限定する。
- ソートはコピー済み/フィルタ済みのローカルスライスだけに適用し、ストア保存順へ副作用を出さない。
- 既存 `withReadHead` と同じ比較規則を使う: `PublishedAt` 降順、同時刻なら `FetchedAt` 降順。
- ブックマーク済み既読を既読ビューから除外する既存仕様は維持する。
- 品質ゲートで Go 標準ライブラリの脆弱性が検出された場合は、アプリ挙動変更と分離して Go patch version 指定だけを修正版へ上げる。

## Task 1: Regression Test

**Files:**
- Modify: `internal/handler/item_handler_test.go`

- [ ] **Step 1: Add a failing test for read-view global recency order**

Add this test after `TestItemListReadViewShowsOnlyRead`:

```go
func TestItemListReadViewOrdersByPublishedAtDescending(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)
	subs := &stubSubscriptions{feeds: []domain.Feed{
		{ID: "older-feed", Title: "older"},
		{ID: "newer-feed", Title: "newer"},
	}}
	h := newAppHandler(t, subs, &stubItems{items: map[string][]domain.Item{
		"older-feed": {
			{
				ID:          "old",
				FeedID:      "older-feed",
				Title:       "古い既読",
				Read:        true,
				PublishedAt: now.Add(-24 * time.Hour),
				FetchedAt:   now.Add(-23 * time.Hour),
			},
		},
		"newer-feed": {
			{
				ID:          "new",
				FeedID:      "newer-feed",
				Title:       "新しい既読",
				Read:        true,
				PublishedAt: now,
				FetchedAt:   now,
			},
		},
	}})
	req := httptest.NewRequest(http.MethodGet, "/app/items?view=read", nil)
	req.Header.Set("HX-Request", "true")
	req = withSession(req, Session{Username: "owner", CSRFToken: "tok"})
	rec := httptest.NewRecorder()

	h.itemList(rec, req)

	body := rec.Body.String()
	newerIndex := strings.Index(body, "新しい既読")
	olderIndex := strings.Index(body, "古い既読")
	if newerIndex == -1 || olderIndex == -1 {
		t.Fatalf("既読ビューは新旧どちらの記事も表示すべきです: %q", body)
	}
	if newerIndex > olderIndex {
		t.Fatalf("既読ビューは公開日時の新しい順に表示すべきです: %q", body)
	}
}
```

- [ ] **Step 2: Run the focused test and confirm it fails**

Run:

```bash
rtk go test ./internal/handler -run TestItemListReadViewOrdersByPublishedAtDescending -count=1
```

Expected: FAIL with the new assertion because the older feed appears before the newer feed.

## Task 2: Read View Sorting

**Files:**
- Modify: `internal/handler/item_handler.go`

- [ ] **Step 1: Add a shared recency sorter in handler**

Add this helper near `withReadHead`:

```go
// sortItemsByRecency 記事を公開日時の新しい順へ並べ替えます。
// 公開日時が等しいときは取得日時の新しい順を二次キーにします。
func sortItemsByRecency(items []domain.Item) {
	sort.SliceStable(items, func(i, j int) bool {
		if items[i].PublishedAt.Equal(items[j].PublishedAt) {
			return items[i].FetchedAt.After(items[j].FetchedAt)
		}
		return items[i].PublishedAt.After(items[j].PublishedAt)
	})
}
```

- [ ] **Step 2: Reuse the helper in `withReadHead`**

Replace the inline `sort.SliceStable(read, ...)` block in `withReadHead` with:

```go
sortItemsByRecency(read)
```

- [ ] **Step 3: Sort only the read view**

In `listItemsFor`, change the `case "read"` branch to:

```go
case "read":
	// ブックマーク済みは保管済みとして既読/未読管理の対象外にするため、既読ビューにも出しません。
	items = keepItems(items, func(it domain.Item) bool { return it.Read && !isBookmarked(it) })
	sortItemsByRecency(items)
```

- [ ] **Step 4: Format the touched files**

Run:

```bash
rtk gofmt -w internal/handler/item_handler.go internal/handler/item_handler_test.go
```

## Task 3: Verification

- [ ] **Step 1: Confirm focused regression passes**

Run:

```bash
rtk go test ./internal/handler -run TestItemListReadViewOrdersByPublishedAtDescending -count=1
```

Expected: PASS.

- [ ] **Step 2: Confirm related handler tests pass**

Run:

```bash
rtk go test ./internal/handler -count=1
```

Expected: PASS.

- [ ] **Step 3: Run the full quality gate**

Run:

```bash
rtk ./scripts/quality-gate.sh
```

Expected: all quality checks passed.

## Task 4: Quality Gate Toolchain Patch

**Files:**
- Modify: `go.mod`
- Modify: `.github/workflows/ci.yml`
- Modify: `Dockerfile`

- [ ] **Step 1: If `govulncheck` fails only because the Go standard library version is vulnerable, pin Go to 1.25.10**

Update:

```go
go 1.25.10
```

Update CI `go-version` values to:

```yaml
go-version: "1.25.10"
```

Update Docker build image to:

```dockerfile
FROM golang:1.25.10-bookworm AS build
```

- [ ] **Step 2: Re-run the full quality gate**

Run:

```bash
rtk bash -lc './scripts/quality-gate.sh'
```

Expected: all quality checks passed.

## Self-Review

- Spec coverage: The left-menu `view=read` path gets explicit latest-first ordering. Existing exclusions for bookmarked items remain intact.
- Placeholder scan: No TODO/TBD/placeholder steps remain.
- Type consistency: Helper takes `[]domain.Item`, matching `listItemsFor` and `withReadHead`.
- Regression control: No repository save path changes; no template changes; no changes to unread, bookmark, read-later, category, or single-feed default branches. Go patch version updates only address quality gate vulnerabilities and do not change application logic.
