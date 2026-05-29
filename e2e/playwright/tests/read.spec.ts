import { test, expect } from "@playwright/test";
import { addFeed, setupAndLogin, startFeedServer } from "./helpers";

test.describe("既読化", () => {
  let feed: { url: string; close: () => Promise<void> };

  test.beforeEach(async ({ page }) => {
    feed = await startFeedServer();
    await setupAndLogin(page);
    await addFeed(page, feed.url);
    await page.click('#tree-pane a.tree-link:has-text("E2E Sample Feed")');
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

  test("全既読ボタンですべての記事が既読になる", async ({ page }) => {
    await page.click('.item-list-bar form[action="/app/items/markall"] button.btn-ghost');
    const unread = page.locator(".item-list li.item-card:not(.is-read)");
    await expect(unread).toHaveCount(0);
  });

  test("既読状態は再読込後も維持される", async ({ page }) => {
    await page.click('.item-list-bar form[action="/app/items/markall"] button.btn-ghost');
    await expect(page.locator(".item-list li.item-card:not(.is-read)")).toHaveCount(0);
    await page.reload();
    await page.click('#tree-pane a.tree-link:has-text("E2E Sample Feed")');
    await expect(page.locator(".item-list li.item-card:not(.is-read)")).toHaveCount(0);
  });
});
