import { test, expect } from "@playwright/test";
import {
  addSavedURL,
  clearBookmarkLabels,
  clearSavedPages,
  expandBookmarkTree,
  openBookmarkView,
  setupAndLogin,
  startPageServer,
} from "./helpers";

test.describe("任意URLのブックマーク追加", () => {
  let site: { url: string; close: () => Promise<void> };

  test.beforeEach(async ({ page }) => {
    site = await startPageServer();
    await setupAndLogin(page);
    await clearSavedPages(page);
    await clearBookmarkLabels(page);
  });

  test.afterEach(async () => {
    await site.close();
  });

  test("URLを追加するとタイトル付きカードが一覧に現れる", async ({ page }) => {
    await addSavedURL(page, site.url);

    const card = page.locator(".item-list li.item-card");
    await expect(card).toHaveCount(1);
    await expect(card.locator(".item-title")).toHaveText("保存したページの見出し");
    // 本文を持たないため、カードは元URLを新しいタブで開くリンクになります。
    await expect(card.locator("a.item-open")).toHaveAttribute("href", site.url);
    await expect(card.locator("a.item-open")).toHaveAttribute("target", "_blank");
  });

  test("同じURLを2回追加してもカードは増えない", async ({ page }) => {
    await addSavedURL(page, site.url);
    await expect(page.locator(".item-list li.item-card")).toHaveCount(1);

    await addSavedURL(page, `${site.url}/`);
    await expect(page.locator(".item-list li.item-card")).toHaveCount(1);
  });

  test("ラベルを選んで追加するとそのラベルの絞り込みに現れる", async ({ page }) => {
    // 先にラベルなしで1件保存し、そのカードのピッカーからラベルを作ります。
    // ラベル作成は元のカードにも所属を付けるため、そのあと外して切り分けます。
    await addSavedURL(page, site.url);
    const first = page.locator(".item-list li.item-card").first();
    await first.locator(".bookmark-btn").click();
    const input = first.locator(".bookmark-panel .bookmark-create-input");
    await input.fill("資料");
    await input.press("Enter");
    const option = first.locator(".bookmark-panel .bookmark-option", { hasText: "資料" });
    await expect(option).toHaveClass(/is-checked/);
    await option.click();
    await expect(first.locator(".bookmark-panel .bookmark-option", { hasText: "資料" })).not.toHaveClass(
      /is-checked/,
    );

    // 別のURLをそのラベル指定で追加します。
    const other = await startPageServer(
      `<!doctype html><html lang="ja"><head><meta charset="utf-8"><title>ラベル付きの保存ページ</title></head><body></body></html>`,
    );
    try {
      await openBookmarkView(page);
      await addSavedURL(page, other.url, "資料");
      await expect(
        page.locator(".item-list li.item-card .item-title", { hasText: "ラベル付きの保存ページ" }),
      ).toBeVisible();
      await expect(page.locator(".item-list li.item-card")).toHaveCount(2);

      // ラベルで絞り込むと、ラベル指定で追加した1件だけが出ます。
      await expandBookmarkTree(page);
      await page.locator("#tree-pane .tree-sub a.tree-link", { hasText: "資料" }).click();
      await expect(page.locator(".item-list-title")).toContainText("資料");
      await expect(page.locator(".item-list li.item-card")).toHaveCount(1);
      await expect(page.locator(".item-list li.item-card .item-title")).toHaveText(
        "ラベル付きの保存ページ",
      );
    } finally {
      await other.close();
    }
  });

  test("保存したページを解除すると一覧から消え、再表示しても現れない", async ({ page }) => {
    await addSavedURL(page, site.url);
    const card = page.locator(".item-list li.item-card").first();
    await card.locator(".bookmark-btn").click();
    await card
      .locator(".bookmark-panel .bookmark-save-btn", { hasText: "ブックマーク解除" })
      .click();

    await expect(page.locator(".item-list li.item-card")).toHaveCount(0);

    await openBookmarkView(page);
    await expect(page.locator(".item-list li.item-card")).toHaveCount(0);
  });

  test("解除した保存ページは未読ストリームにも現れない", async ({ page }) => {
    await addSavedURL(page, site.url);
    const card = page.locator(".item-list li.item-card").first();
    await card.locator(".bookmark-btn").click();
    await card
      .locator(".bookmark-panel .bookmark-save-btn", { hasText: "ブックマーク解除" })
      .click();
    await expect(page.locator(".item-list li.item-card")).toHaveCount(0);

    await page.locator("#tree-pane a.tree-link", { hasText: "すべて" }).first().click();
    await expect(page.locator(".item-list li.item-card")).toHaveCount(0);
  });

  test("不正なURLはエラー文言が出てカードは増えない", async ({ page }) => {
    // type=urlのブラウザ検証を避けるため、入力欄の型を外してから送信します。
    await page.locator(".add-url-form .add-url-input").evaluate((el) => {
      const input = el as HTMLInputElement;
      input.type = "text";
      input.value = "ftp://example.com/a";
    });
    await page.locator(".add-url-form button[type=submit]").click();

    await expect(page.locator(".add-url-error")).toContainText("URLの形式が正しくありません");
    await expect(page.locator(".item-list li.item-card")).toHaveCount(0);
  });

  test("左ツリーに保存したページが購読フィードとして現れない", async ({ page }) => {
    await addSavedURL(page, site.url);
    await expect(page.locator(".item-list li.item-card")).toHaveCount(1);

    await expect(page.locator("#tree-pane")).not.toContainText("保存したページ");
    await expect(page.locator("#tree-pane .tree-unsubscribe")).toHaveCount(0);
  });

  test("URL追加フォームはブックマークビューにだけ出る", async ({ page }) => {
    await expect(page.locator(".add-url-form")).toBeVisible();

    await page.locator("#tree-pane a.tree-link", { hasText: "すべて" }).first().click();
    await expect(page.locator(".add-url-form")).toHaveCount(0);

    await page.locator("#tree-pane a.tree-link", { hasText: "既読" }).first().click();
    await expect(page.locator(".add-url-form")).toHaveCount(0);
  });
});
