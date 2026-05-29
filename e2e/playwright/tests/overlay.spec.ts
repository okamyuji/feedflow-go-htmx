import { test, expect } from "@playwright/test";
import { addFeed, setupAndLogin, startFeedServer } from "./helpers";

test.describe("本文オーバーレイ表示", () => {
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

  test("記事タイトルを押すとオーバーレイが開き本文が表示される", async ({ page }) => {
    await expect(page.locator("#reading-overlay")).toBeHidden();
    await page.click(".item-list li.item-card a.item-open >> nth=0");
    await expect(page.locator("#reading-overlay")).toBeVisible();
    await expect(page.locator("#reading-overlay .reading-body")).toContainText("Body of the first article");
  });

  test("閉じるボタンでオーバーレイが閉じる", async ({ page }) => {
    await page.click(".item-list li.item-card a.item-open >> nth=0");
    await expect(page.locator("#reading-overlay")).toBeVisible();
    await page.click('#reading-overlay button.icon-btn[aria-label="閉じる"]');
    await expect(page.locator("#reading-overlay")).toBeHidden();
  });

  test("Escキーでオーバーレイが閉じる", async ({ page }) => {
    await page.click(".item-list li.item-card a.item-open >> nth=0");
    await expect(page.locator("#reading-overlay")).toBeVisible();
    await page.keyboard.press("Escape");
    await expect(page.locator("#reading-overlay")).toBeHidden();
  });
});
