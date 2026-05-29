import { test, expect } from "@playwright/test";
import { completeSetup, login, USERNAME } from "./helpers";

test.describe("認証と初回セットアップ", () => {
  test("未認証でメイン画面へアクセスするとログインへ誘導される", async ({ page }) => {
    const res = await page.goto("/app");
    expect(res?.status()).toBe(200);
    await expect(page).toHaveURL(/\/(login|setup)$/);
  });

  test("初回セットアップでユーザーを登録しログインできる", async ({ page }) => {
    await completeSetup(page);
    await login(page);
    await expect(page.locator(".app-shell")).toBeVisible();
  });

  test("誤ったパスワードではログインできない", async ({ page }) => {
    await completeSetup(page);
    await page.goto("/login");
    await page.fill('form.auth-form input[name="username"]', USERNAME);
    await page.fill('form.auth-form input[name="password"]', "wrong-password");
    await page.click('form.auth-form button[type="submit"]');
    await expect(page).toHaveURL(/\/login/);
    await expect(page.locator(".app-shell")).toHaveCount(0);
  });

  test("登録後は初回セットアップ画面へ到達できずログインへ送られる", async ({ page }) => {
    await completeSetup(page);
    await page.goto("/setup");
    await expect(page).toHaveURL(/\/login$/);
    await expect(page.locator("form.auth-form input[name='password'][minlength='8']")).toHaveCount(0);
  });

  test("ログイン後にログアウトすると再びログインが必要になる", async ({ page }) => {
    await completeSetup(page);
    await login(page);
    await page.click('form[action="/logout"] button[type="submit"]');
    await expect(page).toHaveURL(/\/login$/);
    await page.goto("/app");
    await expect(page).toHaveURL(/\/login$/);
  });
});
