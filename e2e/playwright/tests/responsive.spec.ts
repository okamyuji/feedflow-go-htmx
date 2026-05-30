import { test, expect } from "@playwright/test";
import { addFeed, setupAndLogin, startFeedServer } from "./helpers";

test.describe("レスポンシブ(モバイル)", () => {
  let feed: { url: string; close: () => Promise<void> };

  test.beforeEach(async ({ page }) => {
    feed = await startFeedServer();
    await setupAndLogin(page);
    await addFeed(page, feed.url);
  });

  test.afterEach(async () => {
    await feed.close();
  });

  test("モバイル幅ではハンバーガーでドロワーとスクリムが開閉する", async ({ page }) => {
    await page.setViewportSize({ width: 375, height: 800 });
    await page.reload();
    await expect(page.locator(".app-shell")).toBeVisible();

    const scrim = page.locator(".drawer-scrim");
    // 既定はドロワー閉でスクリムは出ません。
    await expect(scrim).toBeHidden();

    // ハンバーガーでドロワーを開くとスクリムが出ます。
    await page.click('.app-bar .icon-btn[aria-label="サイドバー切替"]');
    await expect(scrim).toBeVisible();
    await expect(page.locator(".app-body.sidebar-open")).toHaveCount(1);

    // スクリムのドロワー外側(右側)をタップするとドロワーが閉じます。
    await scrim.click({ position: { x: 360, y: 400 } });
    await expect(scrim).toBeHidden();
  });

  test("モバイル幅でフィードをタップするとドロワーが自動で閉じる", async ({ page }) => {
    await page.setViewportSize({ width: 375, height: 800 });
    await page.reload();

    await page.click('.app-bar .icon-btn[aria-label="サイドバー切替"]');
    await expect(page.locator(".drawer-scrim")).toBeVisible();

    await page.locator("#tree-pane a.tree-link", { hasText: "E2E Sample Feed" }).last().click();
    await expect(page.locator(".item-list-title")).toContainText("E2E Sample Feed");
    await expect(page.locator(".drawer-scrim")).toBeHidden();
  });
});
