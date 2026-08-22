import { createServer, type Server } from "node:http";
import type { AddressInfo } from "node:net";
import { type Page, expect } from "@playwright/test";

// USERNAME はE2Eで登録するユーザー名です。
export const USERNAME = "owner";
// PASSWORD はE2Eで登録するパスワードです。
export const PASSWORD = "correct-horse-battery-staple";

// SAMPLE_RSS はテスト用フィードサーバが返すRSS2.0の本文です。
export const SAMPLE_RSS = `<?xml version="1.0" encoding="UTF-8"?>
<rss version="2.0">
  <channel>
    <title>E2E Sample Feed</title>
    <link>http://example.test/site</link>
    <description>feedflow e2e sample</description>
    <item>
      <title>First E2E Article</title>
      <link>http://example.test/articles/1</link>
      <guid>http://example.test/articles/1</guid>
      <description>Body of the first article for e2e.</description>
      <pubDate>Mon, 26 May 2026 09:00:00 +0900</pubDate>
    </item>
    <item>
      <title>Second E2E Article</title>
      <link>http://example.test/articles/2</link>
      <guid>http://example.test/articles/2</guid>
      <description>Body of the second article for e2e.</description>
      <pubDate>Mon, 26 May 2026 10:00:00 +0900</pubDate>
    </item>
  </channel>
</rss>`;

// escapeXML はテスト用RSSへ埋め込む可変テキストをXMLエスケープします。
function escapeXML(value: string): string {
  return value
    .replaceAll("&", "&amp;")
    .replaceAll("<", "&lt;")
    .replaceAll(">", "&gt;")
    .replaceAll('"', "&quot;")
    .replaceAll("'", "&apos;");
}

// startFeedServer はテスト用のフィード配信HTTPサーバを起動してURLを返します。
export async function startFeedServer(
  title = "E2E Sample Feed",
): Promise<{ url: string; close: () => Promise<void> }> {
  const rss = SAMPLE_RSS.replace(
    "<title>E2E Sample Feed</title>",
    `<title>${escapeXML(title)}</title>`,
  );
  const server: Server = createServer((req, res) => {
    if (req.url === "/feed.xml") {
      res.writeHead(200, { "Content-Type": "application/rss+xml; charset=utf-8" });
      res.end(rss);
      return;
    }
    res.writeHead(404, { "Content-Type": "text/plain" });
    res.end("not found");
  });
  await new Promise<void>((resolve) => server.listen(0, "127.0.0.1", resolve));
  const addr = server.address() as AddressInfo;
  const url = `http://127.0.0.1:${addr.port}/feed.xml`;
  const close = () =>
    new Promise<void>((resolve, reject) =>
      server.close((err) => (err ? reject(err) : resolve())),
    );
  return { url, close };
}

// SAMPLE_PAGE_HTML は任意URL保存の検証に使うページ本文です。og:titleとtitleの両方を持ちます。
export const SAMPLE_PAGE_HTML = `<!doctype html><html lang="ja"><head>
<meta charset="utf-8">
<meta property="og:title" content="保存したページの見出し">
<title>タイトル要素</title>
</head><body><p>本文</p></body></html>`;

// startPageServer はテスト用のHTMLページ配信HTTPサーバを起動してURLを返します。
// 任意URL保存のタイトル取得を検証するために使います。
export async function startPageServer(
  html = SAMPLE_PAGE_HTML,
): Promise<{ url: string; close: () => Promise<void> }> {
  const server: Server = createServer((_req, res) => {
    res.writeHead(200, { "Content-Type": "text/html; charset=utf-8" });
    res.end(html);
  });
  await new Promise<void>((resolve) => server.listen(0, "127.0.0.1", resolve));
  const addr = server.address() as AddressInfo;
  const url = `http://127.0.0.1:${addr.port}/page`;
  const close = () =>
    new Promise<void>((resolve, reject) =>
      server.close((err) => (err ? reject(err) : resolve())),
    );
  return { url, close };
}

// completeSetup は初回セットアップ画面でユーザーを登録します。
// テストサーバはスイート全体で共有されるため、登録済みなら/setupは/loginへ誘導されます。
// その場合は登録を飛ばし、初回だけ登録する冪等な実装にします。
export async function completeSetup(page: Page): Promise<void> {
  await page.goto("/setup");
  if (!page.url().endsWith("/setup")) {
    return;
  }
  await expect(page.locator("form.auth-form")).toBeVisible();
  await page.fill('form.auth-form input[name="username"]', USERNAME);
  await page.fill('form.auth-form input[name="password"]', PASSWORD);
  await page.click('form.auth-form button[type="submit"]');
  await expect(page).toHaveURL(/\/login$/);
}

// login はログイン画面から認証してメイン画面へ遷移します。
export async function login(page: Page): Promise<void> {
  await page.goto("/login");
  await expect(page.locator("form.auth-form")).toBeVisible();
  await page.fill('form.auth-form input[name="username"]', USERNAME);
  await page.fill('form.auth-form input[name="password"]', PASSWORD);
  await page.click('form.auth-form button[type="submit"]');
  await expect(page.locator(".app-shell")).toBeVisible();
}

// clearAllFeeds は既存の購読フィードをすべて解除し、各テストを順序非依存のクリーンな状態から始めます。
// E2Eサーバはスイート全体でデータを共有するため、前のテストが残したフィードを取り除きます。
export async function clearAllFeeds(page: Page): Promise<void> {
  // 購読解除はhx-confirmのダイアログを挟むため、この処理の間だけ受理ハンドラを付けます。
  // 各テストが独自のダイアログ処理を持てるよう、終了時にハンドラを必ず外します。
  const acceptDialog = (dialog: import("@playwright/test").Dialog) => dialog.accept();
  page.on("dialog", acceptDialog);
  try {
    if (!page.url().includes("/app")) {
      await page.goto("/app");
    }
    for (;;) {
      const buttons = page.locator("#tree-pane .tree-unsubscribe");
      const remaining = await buttons.count();
      if (remaining === 0) {
        break;
      }
      await buttons.first().click();
      await expect(page.locator("#tree-pane .tree-unsubscribe")).toHaveCount(remaining - 1);
    }
  } finally {
    page.off("dialog", acceptDialog);
  }
}

// setupAndLogin は初回セットアップとログインを連続して行い、購読フィードを空に整えます。
export async function setupAndLogin(page: Page): Promise<void> {
  await completeSetup(page);
  await login(page);
  await clearAllFeeds(page);
}

// addFeed は購読追加フォームにフィードURLを入れて登録します。
export async function addFeed(page: Page, feedURL: string): Promise<void> {
  await page.fill('.subscribe-form input[name="url"]', feedURL);
  await page.click('.subscribe-form button[type="submit"]');
  await expect(page.locator("#tree-pane")).toContainText("E2E Sample Feed");
}

// openBookmarkView はブックマークビュー(全保存記事)を開きます。
export async function openBookmarkView(page: Page): Promise<void> {
  if (!page.url().includes("/app")) {
    await page.goto("/app");
  }
  await page.locator(".tree-bookmark .tree-link", { hasText: "ブックマーク" }).first().click();
  await expect(page.locator(".add-url-form")).toBeVisible();
}

// clearSavedPages はブックマークビューの保存記事をすべて解除して空にします。
// E2Eはスイート全体で1つのサーバを使うため、テスト間の持ち越しをここで断ちます。
export async function clearSavedPages(page: Page): Promise<void> {
  await openBookmarkView(page);
  for (;;) {
    const cards = page.locator(".item-list li.item-card");
    const remaining = await cards.count();
    if (remaining === 0) {
      break;
    }
    const card = cards.first();
    await card.locator(".bookmark-btn").click();
    await card.locator(".bookmark-panel .bookmark-save-btn", { hasText: "ブックマーク解除" }).click();
    await expect(page.locator(".item-list li.item-card")).toHaveCount(remaining - 1);
  }
}

// addSavedURL はブックマークビューのフォームからURLを保存します。
// labelを渡すとそのラベルを選んで追加します。
export async function addSavedURL(page: Page, pageURL: string, label?: string): Promise<void> {
  await page.locator(".add-url-form .add-url-input").fill(pageURL);
  if (label !== undefined) {
    await page.locator(".add-url-form .add-url-select").selectOption({ label });
  }
  await page.locator(".add-url-form button[type=submit]").click();
}
