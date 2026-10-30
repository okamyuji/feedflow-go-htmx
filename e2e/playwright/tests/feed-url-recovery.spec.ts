import { createServer, type Server } from "node:http";
import type { AddressInfo } from "node:net";
import { test, expect, type Page } from "@playwright/test";
import { SAMPLE_RSS, addFeed, setupAndLogin } from "./helpers";

const ERROR_THRESHOLD = 5;

// startMovingFeedServer は、旧URLが途中から500を返し、サイトのトップから新URLを検出できるサーバを起動します。
// 移転したサイトの旧フィードURLが壊れた状態を再現します。
async function startMovingFeedServer() {
  let broken = false;
  const movedRSS = SAMPLE_RSS.replace("<title>E2E Sample Feed</title>", "<title>Moved Feed</title>")
    .replace("First E2E Article", "Moved First Article")
    .replaceAll("example.test/articles/", "example.test/moved/");
  const server: Server = createServer((req, res) => {
    if (req.url === "/old/feed.xml") {
      if (broken) {
        res.writeHead(500, { "Content-Type": "application/json" });
        res.end('{"message": "Internal server error"}');
        return;
      }
      res.writeHead(200, { "Content-Type": "application/rss+xml; charset=utf-8" });
      res.end(SAMPLE_RSS);
      return;
    }
    if (req.url === "/site/") {
      res.writeHead(200, { "Content-Type": "text/html; charset=utf-8" });
      res.end('<!doctype html><html><head><link rel="alternate" type="application/rss+xml" href="/new/index.xml"></head><body>moved</body></html>');
      return;
    }
    if (req.url === "/plain/") {
      res.writeHead(200, { "Content-Type": "text/html; charset=utf-8" });
      res.end("<!doctype html><html><head><title>no feed</title></head><body></body></html>");
      return;
    }
    if (req.url === "/new/index.xml") {
      res.writeHead(200, { "Content-Type": "application/rss+xml; charset=utf-8" });
      res.end(movedRSS);
      return;
    }
    res.writeHead(404, { "Content-Type": "text/plain" });
    res.end("not found");
  });
  await new Promise<void>((resolve) => server.listen(0, "127.0.0.1", resolve));
  const base = `http://127.0.0.1:${(server.address() as AddressInfo).port}`;
  return {
    base,
    breakOldURL: () => {
      broken = true;
    },
    close: () =>
      new Promise<void>((resolve, reject) => server.close((err) => (err ? reject(err) : resolve()))),
  };
}

// pollUntilError は選択中フィードの手動取得をしきい値の回数だけ繰り返し、エラー状態にします。
async function pollUntilError(page: Page) {
  for (let i = 0; i < ERROR_THRESHOLD; i++) {
    const done = page.waitForResponse((res) => res.url().includes("/app/feeds/poll") && res.ok());
    await page.locator(".item-list-actions button", { hasText: "最新記事を取得" }).click();
    await done;
  }
}

test.describe("取得エラーのフィードのURL変更", () => {
  let server: Awaited<ReturnType<typeof startMovingFeedServer>>;

  test.beforeEach(async () => {
    server = await startMovingFeedServer();
  });

  test.afterEach(async () => {
    await server.close();
  });

  test("左ペインでサイトURLを入れると、削除せずに新しいフィードへ切り替わる", async ({ page }) => {
    await setupAndLogin(page);
    await addFeed(page, `${server.base}/old/feed.xml`);
    const feedRow = page.locator("#tree-pane li.tree-feed", { hasText: "E2E Sample Feed" });
    await feedRow.locator("a.tree-link").click();
    await expect(page.locator(".item-list li.item-card")).toHaveCount(2);

    server.breakOldURL();
    await pollUntilError(page);

    const errorRow = page.locator("#tree-pane li.tree-feed.tree-error", { hasText: "E2E Sample Feed" });
    await expect(errorRow.locator(".tree-error-mark")).toBeVisible();
    const fixButton = errorRow.getByRole("button", { name: "URL修正" });
    await expect(fixButton).toBeVisible();
    await fixButton.click();

    const form = errorRow.locator("form.tree-feed-url-form");
    await expect(form).toBeVisible();
    await expect(form).toContainText("HTTP 500");
    const input = form.getByLabel("新しいフィードURLまたはサイトURL");
    await expect(input).toHaveValue(`${server.base}/old/feed.xml`);

    await input.fill(`${server.base}/plain/`);
    await form.getByRole("button", { name: "URLを変更" }).click();
    await expect(form.locator(".add-url-error")).toContainText("フィードが見つかりませんでした");
    await expect(input).toHaveValue(`${server.base}/plain/`);

    await input.fill(`${server.base}/site/`);
    await form.getByRole("button", { name: "URLを変更" }).click();

    const movedRow = page.locator("#tree-pane li.tree-feed", { hasText: "Moved Feed" });
    await expect(movedRow).toBeVisible();
    await expect(movedRow).not.toHaveClass(/tree-error/);
    await expect(movedRow.getByRole("button", { name: "URL修正" })).toHaveCount(0);
    await expect(page.locator(".item-list")).toContainText("Moved First Article");
    await expect(page).toHaveURL(/\/app\/items\?feed=/);
  });
});
