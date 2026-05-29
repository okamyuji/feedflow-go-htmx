import { test, expect } from "@playwright/test";
import { setupAndLogin } from "./helpers";

const THEME_TOGGLE = 'button.icon-btn[aria-label="テーマ切替"]';

test.describe("テーマ切替", () => {
  test("既定テーマはダークで初回の切替でライトへ変わる", async ({ page }) => {
    await setupAndLogin(page);
    await page.click(THEME_TOGGLE);
    await expect(page.locator("html")).toHaveAttribute("data-theme", "light");
  });

  test("切替を二度押すとダークへ戻る", async ({ page }) => {
    await setupAndLogin(page);
    await page.click(THEME_TOGGLE);
    await expect(page.locator("html")).toHaveAttribute("data-theme", "light");
    await page.click(THEME_TOGGLE);
    await expect(page.locator("html")).toHaveAttribute("data-theme", "dark");
  });

  test("切替の結果はlocalStorageへ保存される", async ({ page }) => {
    await setupAndLogin(page);
    await page.click(THEME_TOGGLE);
    await expect(page.locator("html")).toHaveAttribute("data-theme", "light");
    const saved = await page.evaluate(() => localStorage.getItem("feedflow-theme"));
    expect(saved).toBe("light");
  });

  test("変更したテーマは再読込後も維持される", async ({ page }) => {
    await setupAndLogin(page);
    await page.click(THEME_TOGGLE);
    await expect(page.locator("html")).toHaveAttribute("data-theme", "light");
    await page.reload();
    await expect(page.locator("html")).toHaveAttribute("data-theme", "light");
  });
});
