import { test, expect } from "@playwright/test";
import { addFeed, setupAndLogin, startFeedServer } from "./helpers";

test.describe("購読追加と記事一覧", () => {
  let feed: { url: string; close: () => Promise<void> };

  test.beforeEach(async () => {
    feed = await startFeedServer();
  });

  test.afterEach(async () => {
    await feed.close();
  });

  test("フィードURLを登録すると購読ツリーに表示される", async ({ page }) => {
    await setupAndLogin(page);
    await addFeed(page, feed.url);
    await expect(page.locator("#tree-pane")).toContainText("E2E Sample Feed");
  });

  test("購読したフィードを開くと記事が一覧に並ぶ", async ({ page }) => {
    await setupAndLogin(page);
    await addFeed(page, feed.url);
    await page.click('#tree-pane a.tree-link:has-text("E2E Sample Feed")');
    const items = page.locator(".item-list li.item-card");
    await expect(items).toHaveCount(2);
    await expect(page.locator(".item-list")).toContainText("First E2E Article");
    await expect(page.locator(".item-list")).toContainText("Second E2E Article");
  });

  test("登録直後の記事はすべて未読である", async ({ page }) => {
    await setupAndLogin(page);
    await addFeed(page, feed.url);
    await page.click('#tree-pane a.tree-link:has-text("E2E Sample Feed")');
    const unread = page.locator(".item-list li.item-card:not(.is-read)");
    await expect(unread).toHaveCount(2);
  });
});
