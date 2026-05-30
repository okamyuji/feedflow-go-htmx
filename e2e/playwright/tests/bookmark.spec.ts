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
    // 保存状態はピッカーの解除ボタン(is-saved)で確認します(「保存済み」テキスト表示は廃止)。
    await expect(panel.locator(".bookmark-save-btn")).toHaveClass(/is-saved/);
    await expect(panel.locator(".bookmark-save-btn")).toHaveText("ブックマーク解除");
  });

  test("作成したブックマークが左メニューに出て絞り込める", async ({ page }) => {
    const first = page.locator(".item-list li.item-card").first();
    await first.locator(".bookmark-btn").click();
    const input = first.locator(".bookmark-panel .bookmark-create-input");
    await input.fill("Go の知見");
    await input.press("Enter");
    await expect(first.locator(".bookmark-panel .bookmark-save-btn")).toHaveClass(/is-saved/);

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

  test("既存ラベルへのトグルで所属を切り替えられる(保存は維持)", async ({ page }) => {
    const first = page.locator(".item-list li.item-card").first();
    await first.locator(".bookmark-btn").click();
    const input = first.locator(".bookmark-panel .bookmark-create-input");
    await input.fill("読み物");
    await input.press("Enter");
    const option = first.locator(".bookmark-panel .bookmark-option", { hasText: "読み物" });
    await expect(option).toHaveClass(/is-checked/);

    // もう一度トグルするとラベルのチェックは外れます。
    await option.click();
    await expect(option).not.toHaveClass(/is-checked/);
    // ただし保存(ブックマーク)状態は維持されます(保存とラベルの分離)。
    await expect(first.locator(".bookmark-panel .bookmark-save-btn")).toHaveClass(/is-saved/);
  });

  test("ラベルを付けずにブックマーク保存できる", async ({ page }) => {
    const first = page.locator(".item-list li.item-card").first();
    await first.locator(".bookmark-btn").click();
    const panel = first.locator(".bookmark-panel");
    // ラベルを作らず、保存トグルだけで保存します。
    await expect(panel.locator(".bookmark-save-btn")).toHaveText("ブックマークに保存");
    await panel.locator(".bookmark-save-btn").click();
    await expect(panel.locator(".bookmark-save-btn")).toHaveClass(/is-saved/);

    // ラベル0件でも view=bookmark に保存記事が出ます。
    await page.reload();
    await page.locator(".tree-bookmark .tree-link", { hasText: "ブックマーク" }).first().click();
    await expect(page.locator(".item-list-title")).toContainText("ブックマーク");
    await expect(page.locator(".item-list li.item-card")).toHaveCount(1);
  });

  test("ブックマークビューで解除すると一覧から消える", async ({ page }) => {
    const first = page.locator(".item-list li.item-card").first();
    const savedTitle = await first.locator(".item-title").innerText();
    await first.locator(".bookmark-btn").click();
    await first.locator(".bookmark-panel .bookmark-save-btn").click();
    await expect(first.locator(".bookmark-panel .bookmark-save-btn")).toHaveClass(/is-saved/);

    // ブックマークビューへ遷移すると保存記事が1件出ます。
    await page.reload();
    await page.locator(".tree-bookmark .tree-link", { hasText: "ブックマーク" }).first().click();
    await expect(page.locator(".item-list li.item-card")).toHaveCount(1);

    // ブックマークボタンの解除で、その記事がビューから消えます(記事内の解除ボタンを代用)。
    const card = page.locator(".item-list li.item-card", { hasText: savedTitle });
    await card.locator(".bookmark-btn").click();
    await card.locator(".bookmark-panel .bookmark-save-btn", { hasText: "ブックマーク解除" }).click();
    await expect(page.locator(".item-list li.item-card", { hasText: savedTitle })).toHaveCount(0);
  });

  test("ラベルを左メニューから名前変更できる", async ({ page }) => {
    const first = page.locator(".item-list li.item-card").first();
    await first.locator(".bookmark-btn").click();
    const input = first.locator(".bookmark-panel .bookmark-create-input");
    await input.fill("リネーム前");
    await input.press("Enter");
    await expect(first.locator(".bookmark-panel .bookmark-save-btn")).toHaveClass(/is-saved/);

    await page.reload();
    await page.locator(".tree-bookmark .tree-disclosure").click();
    const childLi = page.locator(".tree-sub li.tree-bookmark-item", { hasText: "リネーム前" });
    await childLi.locator(".tree-rename").click();
    const renameInput = childLi.locator(".tree-rename-input");
    await expect(renameInput).toBeVisible();
    await renameInput.fill("リネーム後");
    await renameInput.press("Enter");

    // ツリーが再描画され、新しい名称になります。
    await expect(page.locator("#tree-pane")).toContainText("リネーム後");
    await expect(page.locator("#tree-pane")).not.toContainText("リネーム前");
  });

  test("ラベルを左メニューから削除しても保存記事は残る", async ({ page }) => {
    page.on("dialog", (d) => d.accept());

    const first = page.locator(".item-list li.item-card").first();
    const savedTitle = await first.locator(".item-title").innerText();
    await first.locator(".bookmark-btn").click();
    const input = first.locator(".bookmark-panel .bookmark-create-input");
    await input.fill("削除するラベル");
    await input.press("Enter");
    await expect(first.locator(".bookmark-panel .bookmark-save-btn")).toHaveClass(/is-saved/);

    await page.reload();
    await page.locator(".tree-bookmark .tree-disclosure").click();
    const childLi = page.locator(".tree-sub li.tree-bookmark-item", { hasText: "削除するラベル" });
    // ラベル削除は専用クラスtree-label-delete(購読解除のtree-unsubscribeとは別)です。
    await childLi.locator(".tree-label-delete").click();

    // ラベルはツリーから消えます。
    await expect(page.locator("#tree-pane")).not.toContainText("削除するラベル");

    // ただし保存した記事自体は view=bookmark に残ります。
    await page.locator(".tree-bookmark .tree-link", { hasText: "ブックマーク" }).first().click();
    await expect(page.locator(".item-list li.item-card", { hasText: savedTitle })).toHaveCount(1);
  });
});
