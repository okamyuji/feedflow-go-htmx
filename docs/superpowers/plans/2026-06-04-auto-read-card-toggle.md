# Auto Read Card Toggle Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 自動既読でカードが灰色化したとき、カード内の既読ボタンも必ず `未読に戻す` へ更新されるようにする。

**Architecture:** 既読状態の真実はサーバーの `domain.Item.Read` に置き、既読化レスポンスが更新済みカードHTMLを返す契約をユニットテストで固定する。オーバーレイ表示で既読化する経路は、本文HTMLを主レスポンスとして返しつつ、対象カードをHTMX out-of-bandで差し替える。

**Tech Stack:** Go `testing` / `httptest`, `html/template`, HTMX out-of-band swap, Alpine.js.

---

## MECE Logic Tree

```text
現象: 記事が灰色(is-read)になってもカード内ボタンが「既読」のまま
├── A. サーバー描画契約
│   ├── A1. _item_card.html は .Read=true なら「未読に戻す」を描画する
│   ├── A2. itemMarkRead は MarkRead 後の最新Itemでカードを再描画する必要がある
│   └── A3. itemOverlay は MarkRead 後にカードOOBを同梱する必要がある
├── B. クライアントDOM更新契約
│   ├── B1. onListScroll はカード差し替えレスポンスを受け取る
│   ├── B2. openOverlay は灰色化だけで終わらせず、サーバーレスポンスのOOB更新へ任せる
│   └── B3. OOBカードは overlay target へのレスポンス内でもHTMXが処理できる
├── C. 表示ビュー横断
│   ├── C1. すべて(view未指定)でも同じカードOOBが効く
│   ├── C2. フィード選択でも同じカードOOBが効く
│   └── C3. カテゴリや他ビューでもカードIDが存在すれば同じOOBが効く
└── D. 回帰防止
    ├── D1. 既読POST単体が更新済みカードを返すことをユニットテストで固定する
    ├── D2. オーバーレイ既読化が本文に加えて更新済みカードOOBを返すことをユニットテストで固定する
    └── D3. 既存のツリー未読数OOBを壊さないことを既存テストで確認する
```

## File Structure

- Modify: `internal/handler/feed_handler_test.go`
  - `stubItems.MarkRead` を実状態更新するテストダブルに変える。
  - 既読POST後のHTML文言をテスト可能にする。
- Modify: `internal/handler/item_handler_test.go`
  - `itemMarkRead` レスポンスが `未読に戻す` と `read=false` の `hx-vals` を返すことを固定する。
  - `itemOverlay` が未読記事を既読化した場合、本文レスポンスに対象カードのOOB差し替えと `未読に戻す` を含めることを固定する。
- Modify: `internal/handler/render.go`
  - `itemView` に `CardOOB bool` を追加する。
- Modify: `internal/handler/templates/_item_card.html`
  - `CardOOB` が真のとき、カード `<li>` に `hx-swap-oob="outerHTML"` を付ける。
- Modify: `internal/handler/item_handler.go`
  - オーバーレイ既読化後、更新済みカードOOBとツリーOOBを同梱する描画関数を追加する。
  - `itemOverlay` は `MarkRead` 後の状態をカードOOBへ反映する。
- Modify: `internal/handler/static/app.js`
  - `openOverlay()` の `card.classList.add("is-read")` を削除し、灰色化とボタン文言更新をサーバーOOBに一本化する。

---

### Task 1: Failing Unit Tests

**Files:**
- Modify: `internal/handler/feed_handler_test.go:66`
- Modify: `internal/handler/item_handler_test.go:221`
- Test: `internal/handler/item_handler_test.go`

- [ ] **Step 1: Make the test stub persist MarkRead**

Change `stubItems.MarkRead` in `internal/handler/feed_handler_test.go` from:

```go
func (s *stubItems) MarkRead(_, _ string, _ bool) error         { return nil }
```

to:

```go
func (s *stubItems) MarkRead(feedID, itemID string, read bool) error {
	items := s.items[feedID]
	for i, it := range items {
		if it.ID == itemID {
			items[i].Read = read
			s.items[feedID] = items
			return nil
		}
	}
	return nil
}
```

- [ ] **Step 2: Add a failing assertion for direct read POST**

Append these assertions to `TestItemMarkRead` in `internal/handler/item_handler_test.go` after the existing OOB assertion:

```go
	body := rec.Body.String()
	if !strings.Contains(body, `>未読に戻す</button>`) {
		t.Fatalf("read response should render unread toggle after marking read: %q", body)
	}
	if !strings.Contains(body, `hx-vals='{"read": "false"}'`) {
		t.Fatalf("read response should make the next toggle mark unread: %q", body)
	}
```

- [ ] **Step 3: Add a failing test for overlay auto-read card OOB**

Add this test to `internal/handler/item_handler_test.go`:

```go
func TestItemOverlayMarkReadIncludesUpdatedCardOOB(t *testing.T) {
	t.Parallel()
	subs := &stubSubscriptions{feeds: []domain.Feed{{ID: "f1", Title: "f1"}}}
	items := &stubItems{items: map[string][]domain.Item{
		"f1": {
			{
				ID:      "i1",
				FeedID:  "f1",
				Title:   "記事1",
				Summary: "要約1",
				Content: "本文1",
			},
		},
	}}
	h := newAppHandler(t, subs, items)
	req := httptest.NewRequest(http.MethodGet, "/app/items/f1/i1", nil)
	req.Header.Set("HX-Request", "true")
	req.SetPathValue("feedID", "f1")
	req.SetPathValue("itemID", "i1")
	req = withSession(req, Session{Username: "owner", CSRFToken: "tok"})
	rec := httptest.NewRecorder()

	h.itemOverlay(rec, req)

	body := rec.Body.String()
	if !strings.Contains(body, `class="reading-article"`) {
		t.Fatalf("overlay response should still render the reading article: %q", body)
	}
	if !strings.Contains(body, `id="item-i1"`) || !strings.Contains(body, `hx-swap-oob="outerHTML"`) {
		t.Fatalf("overlay read response should include updated card OOB: %q", body)
	}
	if !strings.Contains(body, `>未読に戻す</button>`) {
		t.Fatalf("overlay read response should render unread toggle in updated card: %q", body)
	}
}
```

- [ ] **Step 4: Run tests to verify RED**

Run:

```bash
rtk go test ./internal/handler
```

Expected: FAIL. The direct read test may pass after the stub update, but `TestItemOverlayMarkReadIncludesUpdatedCardOOB` must fail because the overlay response does not yet include `id="item-i1"` with `hx-swap-oob="outerHTML"`.

---

### Task 2: Minimal Server-Side Fix

**Files:**
- Modify: `internal/handler/render.go`
- Modify: `internal/handler/templates/_item_card.html`
- Modify: `internal/handler/item_handler.go`
- Test: `internal/handler/item_handler_test.go`

- [ ] **Step 1: Add CardOOB to itemView**

Add this field to `itemView` in `internal/handler/render.go`:

```go
	CardOOB     bool          // HTMX out-of-bandでカードを差し替えるかどうかです
```

- [ ] **Step 2: Render card OOB when requested**

Change the opening `<li>` in `internal/handler/templates/_item_card.html` to:

```html
<li
  class="item-card{{ if .Read }} is-read{{ end }}"
  id="item-{{ .ID }}"
  data-feed="{{ .FeedID }}"
  data-item="{{ .ID }}"
  {{ if .CardOOB }}hx-swap-oob="outerHTML"{{ end }}
  x-data="{ bmOpen: false }"
>
```

- [ ] **Step 3: Add a renderer for overlay plus updated card plus tree**

Add this function after `renderCard` in `internal/handler/item_handler.go`:

```go
func (h *Handler) renderOverlayWithCardOOB(w http.ResponseWriter, r *http.Request, overlay itemView, feedID, itemID string) {
	var buf bytes.Buffer
	if err := h.templates.ExecuteTemplate(&buf, "_item_overlay.html", overlay); err != nil {
		slog.Error("failed to execute template", "template", "_item_overlay.html", "error", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	it, ok, err := h.findItem(feedID, itemID)
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	if !ok {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	card := toItemView(it)
	card.CardOOB = true
	if err := h.templates.ExecuteTemplate(&buf, "_item_card.html", card); err != nil {
		slog.Error("failed to execute template", "template", "_item_card.html", "error", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	tree, err := h.treeData(r)
	if err != nil {
		slog.Error("failed to build tree for oob swap", "error", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	tree.TreeOOB = true
	if err := h.templates.ExecuteTemplate(&buf, "_tree_pane.html", tree); err != nil {
		slog.Error("failed to execute template", "template", "_tree_pane.html", "error", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	if _, err := buf.WriteTo(w); err != nil {
		slog.Error("failed to write overlay with card oob", "error", err)
	}
}
```

Also add imports to `internal/handler/item_handler.go`:

```go
	"bytes"
	"log/slog"
```

- [ ] **Step 4: Use the renderer after overlay mark-read**

Change the marked-read branch in `itemOverlay` from:

```go
	if markedRead {
		h.renderWithTreeOOB(w, r, http.StatusOK, "_item_overlay.html", view)
		return
	}
```

to:

```go
	if markedRead {
		h.renderOverlayWithCardOOB(w, r, view, feedID, itemID)
		return
	}
```

- [ ] **Step 5: Run tests to verify GREEN**

Run:

```bash
rtk go test ./internal/handler
```

Expected: PASS.

---

### Task 3: Minimal Client-Side Alignment

**Files:**
- Modify: `internal/handler/static/app.js`
- Test: `internal/handler/item_handler_test.go`

- [ ] **Step 1: Remove local-only gray state from openOverlay**

Change `openOverlay()` in `internal/handler/static/app.js` from:

```js
    openOverlay(event) {
      const { feedID, itemID, card } = cardActionData(event.currentTarget);
      this.activeFeed = feedID;
      this.activeItem = itemID;
      this.overlayOpen = true;
      if (card) {
        card.classList.add("is-read");
      }
    },
```

to:

```js
    openOverlay(event) {
      const { feedID, itemID } = cardActionData(event.currentTarget);
      this.activeFeed = feedID;
      this.activeItem = itemID;
      this.overlayOpen = true;
    },
```

- [ ] **Step 2: Run focused tests**

Run:

```bash
rtk go test ./internal/handler
```

Expected: PASS.

---

### Task 4: Regression Verification

**Files:**
- Test: `internal/handler`
- Test: project Go packages

- [ ] **Step 1: Run handler tests**

Run:

```bash
rtk go test ./internal/handler
```

Expected: PASS.

- [ ] **Step 2: Run all Go tests**

Run:

```bash
rtk go test ./...
```

Expected: PASS.

---

## Self-Review

### Spec Coverage

- スクロール自動既読で灰色化したカードのボタンが `未読に戻す` になる: Task 1 direct read POST assertion and existing `onListScroll` card swap contract cover the server response that HTMX applies.
- `すべて` 選択時など、記事表示だけで既読化される経路でもボタンが `未読に戻す` になる: Task 1 overlay OOB test and Task 2 overlay card OOB implementation cover this path.
- 他の左メニュー選択でも同じ現象が起き得る: fix is view-agnostic because OOB targets `id="item-{itemID}"` whenever that card exists in the current DOM.
- ツリー未読数更新: Task 2 keeps `_tree_pane.html` OOB in overlay read responses, and existing tests keep direct read tree OOB locked.

### Placeholder Scan

No TBD, TODO, "similar to", or unspecified implementation steps remain. Code snippets include exact file paths, commands, and expected outcomes.

### Type Consistency

- `itemView.CardOOB` is read only by `_item_card.html`.
- `renderOverlayWithCardOOB` receives `itemView`, `feedID`, and `itemID`, and uses existing `findItem`, `toItemView`, `treeData`, and templates.
- Imports `bytes` and `log/slog` are needed only in `item_handler.go` for the new renderer.

### Regression / Backward-Compatibility Review

- Direct `itemMarkRead` behavior remains unchanged: primary response is still the card itself plus tree OOB.
- Overlay response still returns `_item_overlay.html` as the primary content for `#reading-overlay`; adding OOB fragments is an HTMX-compatible extension.
- Removing `card.classList.add("is-read")` from `openOverlay` avoids the stale half-updated UI. The card will update when the HTMX response settles. If the network request fails, the UI will no longer falsely show a read state that the server did not persist.
- Existing scroll-position preservation watches for `id="tree-pane"` in responses. Overlay mark-read already returned tree OOB before; keeping tree OOB preserves that behavior.
- The plan intentionally does not change the product behavior that opening an article marks it read. It only makes the card UI consistent with the existing server-side read transition.
