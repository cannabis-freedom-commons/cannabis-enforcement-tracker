// After npm run build: verify incident-first ordering everywhere and detail-only tracker times.
import { chromium } from "playwright";
import fs from "node:fs";
import path from "node:path";
import assert from "node:assert/strict";

const index = JSON.parse(fs.readFileSync("dist/api/index.json", "utf8"));
const row = (i, d, at) => ({ i, d, ...(at ? { at } : {}), c: "cz", t: "raid", a: [], ti: i, s: "An example event.", p: "Prague", ll: [14.42, 50.08], pr: "locality", f: [], k: "" });
const rows = [
  row("event-early", "2026-11-03", "2026-11-03T08:00:00Z"),
  row("event-late", "2026-11-03", "2026-11-03T12:00:00Z"),
  row("historical", "2010-01-01", "2026-11-03T13:00:00Z"),
  row("previous", "2000-01-01", "2026-11-02T23:59:59Z"),
  row("unknown", "2026-11-03"),
];
const browser = await chromium.launch({ headless: true, channel: "chromium" });
try {
  const page = await browser.newPage({ viewport: { width: 1440, height: 1100 }, timezoneId: "Europe/Prague", colorScheme: "dark" });
  const errors = [];
  page.on("pageerror", e => errors.push(e.message));
  await page.clock.setFixedTime(new Date("2026-11-03T13:45:00Z"));
  let synthetic = true;
  const serve = async route => {
    const url = new URL(route.request().url());
    if (url.origin !== "https://time.test") return route.abort();
    if (synthetic && url.pathname === "/api/index.json") return route.fulfill({ json: { v: 1, countries: { cz: "Czechia" }, reports: rows } });
    const file = path.join(process.cwd(), "dist", decodeURIComponent(url.pathname), url.pathname.endsWith("/") ? "index.html" : "");
    try { await route.fulfill({ body: fs.readFileSync(file), contentType: file.endsWith(".css") ? "text/css" : file.endsWith(".js") ? "application/javascript" : file.endsWith(".json") ? "application/json" : "text/html" }); }
    catch { await route.abort(); }
  };
  await page.route("**/*", serve);
  const ids = () => page.locator("#list .report-item").evaluateAll(items => items.map(el => el.dataset.id));
  const ready = () => page.waitForFunction(() => document.querySelector("#months").getAttribute("aria-busy") !== "true");
  await page.goto("https://time.test/?view=added"); // retired parameters must not change the event clock
  await ready();
  assert.deepEqual(await ids(), ["event-late", "event-early", "unknown"]);
  assert.equal(await page.locator("[data-report-view]").count(), 0);
  assert.equal(await page.locator("#list .report-times").count(), 0);
  assert.equal(await page.locator("#period-label").textContent(), "Incident date");
  assert.equal(new URL(page.url()).searchParams.has("view"), false);
  assert.equal(await page.locator('#months button[data-m="2026-11-03"]').getAttribute("data-total"), "3");
  assert.deepEqual(await page.evaluate(() => [...window.__reportsShown]), ["event-late", "event-early", "unknown"]);
  await page.locator('#months button[data-m="2026-11-03"]').click();
  await page.reload();
  await ready();
  assert.deepEqual(await ids(), ["event-late", "event-early", "unknown"]);
  await page.locator("#period").selectOption("all");
  assert.deepEqual(await ids(), ["event-late", "event-early", "unknown", "historical", "previous"]);
  for (const url of ["/reports/", "/country/cz/"]) {
    await page.goto(`https://time.test${url}`);
    await ready();
    assert.deepEqual(await ids(), ["event-late", "event-early", "unknown", "historical", "previous"]);
    assert.equal(await page.locator("#list .report-times").count(), 0);
  }
  synthetic = false;
  await page.goto("https://time.test/");
  await ready();
  fs.mkdirSync(".cache", { recursive: true });
  await page.screenshot({ path: ".cache/report-time-desktop.png" });
  await page.setViewportSize({ width: 390, height: 844 });
  await page.locator("#list").scrollIntoViewIfNeeded();
  await page.screenshot({ path: ".cache/report-time-mobile.png" });
  assert.equal(await page.evaluate(() => document.documentElement.scrollWidth > innerWidth), false);
  await page.goto("https://time.test/2026/11/");
  assert.equal(await page.locator(".report-times").count(), 0);
  assert.deepEqual(await page.locator(".reports .report-item").evaluateAll(items => items.map(el => el.dataset.id)), index.reports.filter(r => r.d.startsWith("2026-11")).map(r => r.i));
  await page.goto(`https://time.test/report/${index.reports.find(r => r.at).i}/`);
  assert.match(await page.locator(".report-head .report-added").textContent(), /Added .*(ago|just now)/);
  assert.match(await page.locator(".report-head .report-added").getAttribute("title"), /\d{2}:\d{2}:\d{2} UTC/);
  assert.equal(await page.locator(".report-item .report-times").count(), 0);
  assert.equal(await page.locator(".report-times").filter({ hasText: /estimated/i }).count(), 0);
  assert.match(await page.getByRole("link", { name: "View report source" }).getAttribute("href"), /\/blob\/main\/reports\//);
  const noJS = await browser.newPage({ javaScriptEnabled: false });
  await noJS.route("**/*", serve);
  await noJS.goto("https://time.test/");
  assert.deepEqual(await noJS.locator("#list .report-item").evaluateAll(items => items.map(el => el.dataset.id)), index.reports.slice(0, 30).map(r => r.i));
  assert.equal(await noJS.locator("#list .report-times").count(), 0);
  assert.deepEqual(errors, []);
  console.log("Verified incident/addition ordering on homepage, Reports, country and month listings; incident filters/map; detail-only relative timestamps; main-branch source links; mobile and no-JavaScript rendering.");
} finally { await browser.close(); }
