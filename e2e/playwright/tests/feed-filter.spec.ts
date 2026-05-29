import { test, expect } from "@playwright/test";
import { addFeed, setupAndLogin, startFeedServer } from "./helpers";

test.describe("フィード絞り込み", () => {
  let feed: { url: string; close: () => Promise<void> };

  test.beforeEach(async ({ page }) => {
    feed = await startFeedServer();
    await setupAndLogin(page);
    await addFeed(page, feed.url);
  });

  test.afterEach(async () => {
    await feed.close();
  });

  const FEED = '.tree-feeds .tree-feed:has-text("E2E Sample Feed")';

  test("部分一致で絞り込み、空にすると全表示へ戻る", async ({ page }) => {
    // 一致する文字ではフィードが残ります。
    await page.fill(".feed-filter-input", "Sample");
    await expect(page.locator(FEED).first()).toBeVisible();

    // 一致しない文字ではフィードが隠れ、案内が出ます。
    await page.fill(".feed-filter-input", "zzzznomatch");
    await expect(page.locator(FEED).first()).toBeHidden();
    await expect(page.locator(".tree-feed-empty")).toBeVisible();

    // 空にすると自動で全表示へ戻ります。
    await page.fill(".feed-filter-input", "");
    await expect(page.locator(FEED).first()).toBeVisible();
    await expect(page.locator(".tree-feed-empty")).toBeHidden();
  });

  test("Escapeキーで絞り込みを解除する", async ({ page }) => {
    await page.fill(".feed-filter-input", "zzzznomatch");
    await expect(page.locator(FEED).first()).toBeHidden();
    await page.locator(".feed-filter-input").press("Escape");
    await expect(page.locator(FEED).first()).toBeVisible();
    await expect(page.locator(".feed-filter-input")).toHaveValue("");
  });

  test("固定ナビは絞り込みの対象にならない", async ({ page }) => {
    await page.fill(".feed-filter-input", "zzzznomatch");
    await expect(page.locator('.tree-nav .tree-all .tree-label')).toBeVisible();
    await expect(page.locator('.tree-nav .tree-readlater .tree-label')).toBeVisible();
  });
});
