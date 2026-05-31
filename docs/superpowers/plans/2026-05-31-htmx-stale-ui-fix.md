# HTMX Stale UI Fix Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Fix stale UI after subscribing to a feed and after creating/toggling bookmark labels.

**Architecture:** Keep HTMX swaps localized: subscribe responses update `#main-pane` with the current item list and include the refreshed tree as an out-of-band swap. Bookmark picker responses still update the clicked picker, but include an out-of-band tree pane so labels and unread counts refresh immediately.

**Tech Stack:** Go `net/http`, `html/template`, HTMX, Playwright E2E, Make quality gates.

---

### Task 1: Lock Subscribe Response Behavior

**Files:**
- Modify: `internal/handler/feed_handler_test.go`
- Modify: `internal/handler/templates/_tree.html`
- Modify: `internal/handler/feed_handler.go`

- [ ] Add a handler test proving `feedSubscribe` returns an item list as the primary response and a tree pane out-of-band swap.
- [ ] Change `.subscribe-form` to target `#main-pane` instead of `#tree-pane`.
- [ ] Change `feedSubscribe` to render `_item_list.html` for the all-items view and append `_tree_pane.html` with `hx-swap-oob="true"`.
- [ ] Run `go test ./internal/handler/ -run 'TestFeedSubscribe|TestParseTemplates' -v`.

### Task 2: Lock Bookmark Tree Refresh Behavior

**Files:**
- Modify: `internal/handler/bookmark_handler_test.go`
- Modify: `internal/handler/bookmark_save_test.go`
- Modify: `internal/handler/bookmark_handler.go`

- [ ] Add tests proving bookmark create, toggle, and save responses include `hx-swap-oob="true"` for `#tree-pane`.
- [ ] Change bookmark picker rendering to append a refreshed tree pane OOB for create/toggle/save flows.
- [ ] Keep bookmark-view removal behavior intact by appending the existing delete OOB fragment when unbookmarking in bookmark views.
- [ ] Run `go test ./internal/handler/ -run 'TestBookmark|TestItemBookmark' -v`.

### Task 3: Browser And Gate Verification

**Files:**
- Modify: `e2e/playwright/tests/subscription.spec.ts`
- Modify: `e2e/playwright/tests/bookmark.spec.ts`

- [ ] Add E2E coverage for subscribe showing article cards without reload.
- [ ] Add E2E coverage for label creation immediately appearing in another article's bookmark picker and left menu.
- [ ] Run `cd e2e/playwright && npx playwright test`.
- [ ] Run `make build`.
- [ ] Run `make quality`.
- [ ] Commit, push branch, and open PR.

### Task 4: Configurable Feed Sorting

**Files:**
- Modify: `internal/domain/types.go`
- Modify: `internal/domain/settings.go`
- Modify: `internal/domain/settings_test.go`
- Modify: `internal/handler/feed_handler.go`
- Modify: `internal/handler/feed_handler_test.go`
- Modify: `internal/handler/settings_handler.go`
- Modify: `internal/handler/settings_handler_test.go`
- Modify: `internal/handler/templates/_settings.html`

- [ ] Add `FeedSortKey` values `title` and `registered`, and `SortDirection` values `asc` and `desc`.
- [ ] Add `FeedSortKey` and `FeedSortDirection` fields to `domain.Settings`, defaulting to title ascending.
- [ ] Parse and render those settings in the settings page.
- [ ] Sort feed nodes by title or registration order according to settings.
- [ ] Run focused domain and handler tests before full gates.
