import { createServer, type Server } from "node:http";
import type { AddressInfo } from "node:net";
import { test, expect, type Page } from "@playwright/test";
import { setupAndLogin } from "./helpers";

// rss は指定タイトルと記事数のRSS2.0を返します。記事数で本文側のスクロール量を調整します。
function rss(title: string, count: number): string {
  let items = "";
  for (let i = 1; i <= count; i++) {
    items += `<item>
      <title>${title} Article ${i}</title>
      <link>http://example.test/${encodeURIComponent(title)}/${i}</link>
      <guid>http://example.test/${encodeURIComponent(title)}/${i}</guid>
      <description>Body ${i} for ${title}.</description>
      <pubDate>Mon, 26 May 2026 09:00:00 +0900</pubDate>
    </item>`;
  }
  return `<?xml version="1.0" encoding="UTF-8"?>
<rss version="2.0">
  <channel>
    <title>${title}</title>
    <link>http://example.test/${encodeURIComponent(title)}</link>
    <description>feedflow scroll e2e</description>
    ${items}
  </channel>
</rss>`;
}

// startScrollFeedServer は多数のフィード(/feed-N.xml)と多記事フィード(/long.xml)を配信するサーバを立てます。
// 左サイドバーと右ペインの双方をスクロール可能な状態にするためのテスト専用フィード源です。
async function startScrollFeedServer(): Promise<{ base: string; close: () => Promise<void> }> {
  const server: Server = createServer((req, res) => {
    const u = req.url || "";
    if (u === "/long.xml") {
      res.writeHead(200, { "Content-Type": "application/rss+xml; charset=utf-8" });
      res.end(rss("Long Scroll Feed", 40));
      return;
    }
    const m = u.match(/^\/feed-(\d+)\.xml$/);
    if (m) {
      res.writeHead(200, { "Content-Type": "application/rss+xml; charset=utf-8" });
      res.end(rss(`Scroll Feed ${m[1]}`, 1));
      return;
    }
    res.writeHead(404, { "Content-Type": "text/plain" });
    res.end("not found");
  });
  await new Promise<void>((resolve) => server.listen(0, "127.0.0.1", resolve));
  const addr = server.address() as AddressInfo;
  const base = `http://127.0.0.1:${addr.port}`;
  const close = () =>
    new Promise<void>((resolve, reject) =>
      server.close((err) => (err ? reject(err) : resolve())),
    );
  return { base, close };
}

// addScrollFeed は購読フォームからフィードを登録し、ツリーに該当ラベルが現れるまで待ちます。
async function addScrollFeed(page: Page, url: string, label: string): Promise<void> {
  await page.fill(".subscribe-form input[name=\"url\"]", url);
  await page.click(".subscribe-form button[type=\"submit\"]");
  await expect(page.locator(`#tree-pane li.tree-feed[data-label="${label}"]`)).toHaveCount(1);
}

// treeScrollTop / mainScrollTop は対象ペインの現在のスクロール量を返します。
const treeScrollTop = (page: Page) =>
  page.locator("#tree-pane").evaluate((el) => el.scrollTop);
const mainScrollTop = (page: Page) =>
  page.locator("#main-pane").evaluate((el) => el.scrollTop);

test.describe("スクロール挙動(フィード選択・削除)", () => {
  let feed: { base: string; close: () => Promise<void> };
  const SHORT_FEED_COUNT = 10;

  test.beforeEach(async ({ page }) => {
    test.setTimeout(90_000);
    feed = await startScrollFeedServer();
    await setupAndLogin(page);
    // デスクトップ幅かつ低めの高さにして、両ペインを確実にスクロール可能にします。
    await page.setViewportSize({ width: 1100, height: 500 });
    await addScrollFeed(page, `${feed.base}/long.xml`, "Long Scroll Feed");
    for (let i = 1; i <= SHORT_FEED_COUNT; i++) {
      await addScrollFeed(page, `${feed.base}/feed-${i}.xml`, `Scroll Feed ${i}`);
    }
  });

  test.afterEach(async () => {
    await feed.close();
  });

  test("フィードを選ぶと右ペインが最上部から表示される", async ({ page }) => {
    await page.click('#tree-pane li.tree-feed[data-label="Long Scroll Feed"] a.tree-link');
    await expect(page.locator(".item-list-title")).toHaveText("Long Scroll Feed");
    await expect(page.locator(".item-list li.item-card")).toHaveCount(40);

    // 右ペインを下方へスクロールしておきます。
    await page.locator("#main-pane").evaluate((el) => {
      el.scrollTop = 400;
    });
    expect(await mainScrollTop(page)).toBeGreaterThan(100);

    // 別フィードを選ぶと、記事の先頭から表示される(scrollTopが0へ戻る)べきです。
    await page.click('#tree-pane li.tree-feed[data-label="Scroll Feed 1"] a.tree-link');
    await expect(page.locator(".item-list-title")).toHaveText("Scroll Feed 1");
    await expect.poll(() => mainScrollTop(page)).toBe(0);
  });

  test("フィードを選んでも左ペインのスクロール位置が保持される", async ({ page }) => {
    // 左サイドバーを下方へスクロールします。
    await page.locator("#tree-pane").evaluate((el) => {
      el.scrollTop = 200;
    });
    const before = await treeScrollTop(page);
    expect(before).toBeGreaterThan(50);

    // 中ほどのフィードを選択します(選択でtree-paneがOOBで差し替わる)。
    await page.click('#tree-pane li.tree-feed[data-label="Scroll Feed 6"] a.tree-link');
    await expect(page.locator(".item-list-title")).toHaveText("Scroll Feed 6");

    // スクロール位置が先頭(0)に戻らず保持されるべきです。
    await expect.poll(() => treeScrollTop(page)).toBeGreaterThan(50);
  });

  test("フィードを削除しても左ペインのスクロール位置が保持される", async ({ page }) => {
    // 購読解除はhx-confirmのダイアログを伴うため受理します。
    page.on("dialog", (dialog) => dialog.accept());

    await page.locator("#tree-pane").evaluate((el) => {
      el.scrollTop = 150;
    });
    expect(await treeScrollTop(page)).toBeGreaterThan(50);

    // 中ほどのフィードを購読解除します(tree-paneがouterHTMLで差し替わる)。
    await page
      .locator('#tree-pane li.tree-feed[data-label="Scroll Feed 5"] .tree-unsubscribe')
      .click();
    await expect(
      page.locator('#tree-pane li.tree-feed[data-label="Scroll Feed 5"]'),
    ).toHaveCount(0);

    // 削除後もスクロール位置が先頭に戻らず、削除した位置の近辺が保たれるべきです。
    await expect.poll(() => treeScrollTop(page)).toBeGreaterThan(50);
  });

  test("自動既読中も右ペインのスクロール位置が先頭へ飛ばない", async ({ page }) => {
    await page.click('#tree-pane li.tree-feed[data-label="Long Scroll Feed"] a.tree-link');
    await expect(page.locator(".item-list li.item-card")).toHaveCount(40);

    // 下方へスクロールすると上端を越えた未読カードが自動既読になり、カード単体がスワップされます。
    // そのスワップイベントがmain-paneへ伝播しても、右ペインが先頭へ戻ってはいけません(デグレ防止)。
    await page.locator("#main-pane").evaluate((el) => {
      el.scrollTop = 500;
    });
    await page.waitForTimeout(800);

    expect(await mainScrollTop(page)).toBeGreaterThan(100);
  });

  test("スクロール自動既読はカードのボタンも未読に戻すへ更新する", async ({ page }) => {
    await page.click('#tree-pane li.tree-feed[data-label="Long Scroll Feed"] a.tree-link');
    await expect(page.locator(".item-list li.item-card")).toHaveCount(40);

    await page.locator("#main-pane").evaluate((el) => {
      el.scrollTop = 500;
    });
    await page.waitForTimeout(1200);

    await expect
      .poll(async () =>
        page.locator(".item-card").evaluateAll((cards) => {
          const barBottom = document.querySelector(".app-bar")?.getBoundingClientRect().bottom ?? 0;
          return cards
            .filter((card) => card.getBoundingClientRect().bottom <= barBottom)
            .map((card) => ({
              title: card.querySelector(".item-title")?.textContent?.trim(),
              isRead: card.classList.contains("is-read"),
              hasUnreadButton: Array.from(card.querySelectorAll("button")).some(
                (button) => button.textContent?.trim() === "未読に戻す",
              ),
            }));
        }),
      )
      .toEqual(
        expect.arrayContaining([
          expect.objectContaining({ isRead: true, hasUnreadButton: true }),
        ]),
      );
  });
});
