import { test, expect } from "@playwright/test";
import { addFeed, setupAndLogin, startFeedServer } from "./helpers";

test.describe("ブックマーク", () => {
  let feed: { url: string; close: () => Promise<void> };

  test.beforeEach(async ({ page }) => {
    feed = await startFeedServer();
    await setupAndLogin(page);
    await addFeed(page, feed.url);
    await page.locator("#tree-pane a.tree-link", { hasText: "E2E Sample Feed" }).last().click();
    await expect(page.locator(".item-list li.item-card")).toHaveCount(2);
  });

  test.afterEach(async () => {
    await feed.close();
  });

  test("新規名称を作成して記事をブックマークできる", async ({ page }) => {
    const first = page.locator(".item-list li.item-card").first();
    await first.locator(".bookmark-btn").click();

    const panel = first.locator(".bookmark-panel");
    const input = panel.locator(".bookmark-create-input");
    await expect(input).toBeVisible();
    await input.fill("あとで実装する");
    await input.press("Enter");

    // 作成後はピッカーに名称が出てチェック済みになります。
    await expect(panel.locator(".bookmark-option", { hasText: "あとで実装する" })).toHaveClass(/is-checked/);
    // カードに保存済み表示が出ます。
    await expect(first.locator(".item-bookmark")).toContainText("保存済み");
  });

  test("作成したブックマークが左メニューに出て絞り込める", async ({ page }) => {
    const first = page.locator(".item-list li.item-card").first();
    await first.locator(".bookmark-btn").click();
    const input = first.locator(".bookmark-panel .bookmark-create-input");
    await input.fill("Go の知見");
    await input.press("Enter");
    await expect(first.locator(".item-bookmark")).toContainText("保存済み");

    // 左メニューは次の遷移でブックマークを反映するため再読込します。
    await page.reload();

    // ブックマークを展開すると名称が子ノードに出ます。
    await page.locator(".tree-bookmark .tree-disclosure").click();
    const child = page.locator(".tree-sub a.tree-link", { hasText: "Go の知見" });
    await expect(child).toBeVisible();

    // 名称で絞り込むと保存した記事が一覧に出ます。
    await child.click();
    await expect(page.locator(".item-list-title")).toContainText("Go の知見");
    await expect(page.locator(".item-list li.item-card")).toHaveCount(1);
  });

  test("オーバーレイでブックマークするとボタン表示が即時同期する", async ({ page }) => {
    await page.click(".item-list li.item-card a.item-open >> nth=0");
    await expect(page.locator("#reading-overlay")).toBeVisible();

    const actions = page.locator(".reading-actions");
    await actions.getByRole("button", { name: /ブックマーク/ }).click();
    const input = actions.locator(".bookmark-create-input");
    await input.fill("保存リスト");
    await input.press("Enter");

    // ピッカーのx-initがイベントを発火し、上部ボタンがブックマーク済みへ同期します。
    await expect(actions.locator("button.is-active")).toContainText("ブックマーク済み");
  });

  test("既存ブックマークへのトグルで所属を切り替えられる", async ({ page }) => {
    const first = page.locator(".item-list li.item-card").first();
    await first.locator(".bookmark-btn").click();
    const input = first.locator(".bookmark-panel .bookmark-create-input");
    await input.fill("読み物");
    await input.press("Enter");
    const option = first.locator(".bookmark-panel .bookmark-option", { hasText: "読み物" });
    await expect(option).toHaveClass(/is-checked/);

    // もう一度トグルすると外れ、保存済み表示が消えます。
    await option.click();
    await expect(option).not.toHaveClass(/is-checked/);
    await expect(first.locator(".item-bookmark")).not.toContainText("保存済み");
  });
});
