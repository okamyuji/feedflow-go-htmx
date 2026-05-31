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

  test("フィードURLを登録すると再読込なしで記事一覧に反映される", async ({ page }) => {
    await setupAndLogin(page);
    await addFeed(page, feed.url);
    const items = page.locator(".item-list li.item-card");
    await expect(items).toHaveCount(2);
    await expect(page.locator(".item-list")).toContainText("First E2E Article");
    await expect(page.locator(".item-list")).toContainText("Second E2E Article");
  });

  test("購読したフィードを開くと記事が一覧に並ぶ", async ({ page }) => {
    await setupAndLogin(page);
    await addFeed(page, feed.url);
    await page.locator('#tree-pane a.tree-link', { hasText: "E2E Sample Feed" }).last().click();
    const items = page.locator(".item-list li.item-card");
    await expect(items).toHaveCount(2);
    await expect(page.locator(".item-list")).toContainText("First E2E Article");
    await expect(page.locator(".item-list")).toContainText("Second E2E Article");
  });

  test("登録直後の記事はすべて未読である", async ({ page }) => {
    await setupAndLogin(page);
    await addFeed(page, feed.url);
    await page.locator('#tree-pane a.tree-link', { hasText: "E2E Sample Feed" }).last().click();
    const unread = page.locator(".item-list li.item-card:not(.is-read)");
    await expect(unread).toHaveCount(2);
  });

  test("設定でフィード順をAlphabet昇順から登録降順へ変更できる", async ({ page }) => {
    const alpha = await startFeedServer("Alpha Feed");
    const zulu = await startFeedServer("Zulu Feed");
    try {
      await setupAndLogin(page);

      await page.fill('.subscribe-form input[name="url"]', alpha.url);
      await page.click('.subscribe-form button[type="submit"]');
      await expect(page.locator("#tree-pane")).toContainText("Alpha Feed");

      await page.fill('.subscribe-form input[name="url"]', zulu.url);
      await page.click('.subscribe-form button[type="submit"]');
      await expect(page.locator("#tree-pane")).toContainText("Zulu Feed");

      await expect(page.locator("#tree-pane .tree-feeds .tree-label")).toHaveText([
        "Alpha Feed",
        "Zulu Feed",
      ]);

      await page.getByRole("link", { name: "設定" }).click();
      await page.locator('select[name="feed_sort_key"]').selectOption("registered");
      await page.locator('select[name="feed_sort_direction"]').selectOption("desc");
      await page.locator(".settings-form button[type='submit']").click();

      await expect(page.locator("#tree-pane .tree-feeds .tree-label")).toHaveText([
        "Zulu Feed",
        "Alpha Feed",
      ]);
    } finally {
      await alpha.close();
      await zulu.close();
    }
  });

  test("設定した既定の表示形式が記事一覧と再読込後に反映される", async ({ page }) => {
    await setupAndLogin(page);
    await addFeed(page, feed.url);

    for (const view of ["title", "card", "magazine", "article"]) {
      await page.getByRole("link", { name: "設定" }).click();
      await page.locator('select[name="default_view"]').selectOption(view);
      await page.locator(".settings-form button[type='submit']").click();

      await page.locator(".tree-all .tree-link").click();
      await expect(page.locator(".item-list")).toHaveAttribute("data-view", view);

      await page.reload();
      await expect(page.locator(".item-list")).toHaveAttribute("data-view", view);
    }
  });
});
