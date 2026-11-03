// After npm run build and npm run browser-install:
// node scripts/check-report-scroll.mjs
import { chromium } from "playwright";
import fs from "node:fs";
import path from "node:path";
import assert from "node:assert/strict";

const rows = JSON.parse(fs.readFileSync("dist/api/index.json", "utf8")).reports;
const browser = await chromium.launch({ headless: true, channel: "chromium" });
try {
  const page = await browser.newPage({ viewport: { width: 1280, height: 950 } });
  page.setDefaultTimeout(10000);
  const errors = [];
  page.on("pageerror", error => errors.push(error.message));
  await page.route("**/*", async route => {
    const url = new URL(route.request().url());
    if (url.origin !== "https://scroll.test") return route.abort();
    const file = path.join(process.cwd(), "dist", decodeURIComponent(url.pathname), url.pathname.endsWith("/") ? "index.html" : "");
    try {
      await route.fulfill({ body: fs.readFileSync(file), contentType: file.endsWith(".css") ? "text/css" : file.endsWith(".js") ? "application/javascript" : file.endsWith(".json") ? "application/json" : "text/html" });
    } catch { await route.abort(); }
  });
  await page.goto("https://scroll.test/reports/");
  await page.locator("#scroll-status").filter({ hasText: "scroll for more" }).waitFor();
  const items = page.locator("#list .report-item");
  assert.equal(await items.count(), 30);
  assert.equal(await page.locator("#more").isVisible(), false);
  const first = await items.first().elementHandle();
  await page.locator("#report-scroll-sentinel").scrollIntoViewIfNeeded();
  await page.waitForFunction(() => document.querySelectorAll("#list .report-item").length > 30);
  assert.equal(await first.evaluate(el => el.isConnected), true);
  while (await items.count() < rows.length) {
    const previous = await items.count();
    await page.evaluate(() => window.scrollTo({ top: document.body.scrollHeight, behavior: "instant" }));
    await page.waitForFunction(n => document.querySelectorAll("#list .report-item").length > n, previous);
  }
  console.log("Full archive loaded", await items.count());
  assert.deepEqual(await items.evaluateAll(nodes => nodes.map(node => node.dataset.id)), rows.map(row => row.i));
  assert.equal(await page.locator("#report-scroll-sentinel").isVisible(), false);
  assert.ok((await page.locator("#scroll-status").innerText()).startsWith("All "));
  assert.ok(await page.locator("#list .ry-month").evaluateAll(months => months.every(month => [...month.querySelectorAll(".meta time")].every(time => time.dateTime.startsWith(month.id.slice(1))))));

  console.log("Checking filter reset");
  await page.locator("#country").selectOption("to");
  assert.equal(await items.count(), rows.filter(row => row.c === "to" || row.c.startsWith("to/")).length);
  assert.equal(await page.locator("#report-scroll-sentinel").isVisible(), false);
  await page.locator("#country").selectOption("");
  // A reset starts in batches, and may fill the viewport automatically.
  assert.ok(await items.count() >= 30 && await items.count() < rows.length);
  console.log("Checking mobile load", await items.count());
  await page.setViewportSize({ width: 390, height: 850 });
  const beforeMobile = await items.count();
  await page.locator("#report-scroll-sentinel").scrollIntoViewIfNeeded();
  await page.waitForFunction(n => document.querySelectorAll("#list .report-item").length > n, beforeMobile);
  assert.equal(await page.evaluate(() => document.documentElement.scrollWidth), 390);

  await page.setViewportSize({ width: 1280, height: 950 });
  console.log("Checking month navigation");
  await page.goto("https://scroll.test/reports/?year=2026");
  await page.locator('#list .ry-months a[href="#m2026-09"]').click();
  assert.equal(new URL(page.url()).searchParams.get("year"), "2026");
  assert.equal(new URL(page.url()).hash, "#m2026-09");
  assert.equal(await page.locator("#m2026-09").count(), 1);
  await page.goto("https://scroll.test/");
  await page.locator("#months button").first().waitFor();
  assert.equal(await page.locator("#report-scroll-sentinel").count(), 0);
  assert.equal(await page.locator("#more").isVisible(), true);

  console.log("Checking fallback");
  await page.addInitScript(() => { delete window.IntersectionObserver; });
  await page.goto("https://scroll.test/reports/");
  await page.locator("#months button").first().waitFor();
  assert.equal(await page.locator("#more").isVisible(), true);
  await page.locator("#more").click();
  assert.equal(await items.count(), 60);
  assert.deepEqual(errors, []);
  console.log(`Verified automatic scrolling through ${rows.length} reports, stable rows, grouping, filter resets, month links, mobile and manual fallback.`);
} finally {
  await browser.close();
}
