import { test, expect } from "@playwright/test";
import { addFeed, setupAndLogin, startFeedServer } from "./helpers";

test.describe("既読化", () => {
  let feed: { url: string; close: () => Promise<void> };

  test.beforeEach(async ({ page }) => {
    feed = await startFeedServer();
    await setupAndLogin(page);
    await addFeed(page, feed.url);
    await page.locator('#tree-pane a.tree-link', { hasText: "E2E Sample Feed" }).last().click();
    await expect(page.locator(".item-list li.item-card")).toHaveCount(2);
  });

  test.afterEach(async () => {
    await feed.close();
  });

  test("個別の既読ボタンで記事が既読になる", async ({ page }) => {
    const first = page.locator(".item-list li.item-card").first();
    await expect(first).not.toHaveClass(/is-read/);
    await first.locator(".item-quick button.quick-btn", { hasText: "既読" }).click();
    await expect(page.locator(".item-list li.item-card").first()).toHaveClass(/is-read/);
  });

  test("このフィードを既読で表示中フィードの記事がすべて既読になる", async ({ page }) => {
    await page.click(".item-list-bar .markread-primary");
    const unread = page.locator(".item-list li.item-card:not(.is-read)");
    await expect(unread).toHaveCount(0);
  });

  test("すべてのフィードを既読のメニューは確認のうえ実行できる", async ({ page }) => {
    // 破壊的操作のためhx-confirmのダイアログを受理します。
    page.on("dialog", (dialog) => dialog.accept());
    await page.click(".item-list-bar .markread-caret");
    await expect(page.locator(".item-list-bar .markread-menu")).toBeVisible();
    await page.click(".item-list-bar .markread-menu .markread-menu-item");
    await expect(page.locator(".item-list li.item-card:not(.is-read)")).toHaveCount(0);
  });

  test("既読状態は再読込後も維持される", async ({ page }) => {
    await page.click(".item-list-bar .markread-primary");
    await expect(page.locator(".item-list li.item-card:not(.is-read)")).toHaveCount(0);
    await page.reload();
    await page.locator('#tree-pane a.tree-link', { hasText: "E2E Sample Feed" }).last().click();
    await expect(page.locator(".item-list li.item-card:not(.is-read)")).toHaveCount(0);
  });
});
