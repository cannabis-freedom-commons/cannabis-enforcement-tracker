import { test } from "node:test";
import assert from "node:assert/strict";
import fs from "node:fs/promises";
import os from "node:os";
import path from "node:path";
import http from "node:http";
import { chromium } from "playwright";
import { captureSource, extractSourcePage } from "./source-browser.mjs";

let browserAvailable = true;
try { await fs.access(chromium.executablePath()); } catch { browserAvailable = false; }

test("source extraction follows the story rather than the first article preview and excludes hidden evidence", { skip: !browserAvailable }, async () => {
  const browser = await chromium.launch({ channel: "chromium", headless: true });
  try {
    const page = await browser.newPage();
    const story = "Police reported a cannabis seizure and arrested two suspects at the farm. ".repeat(5);
    const preview = "An unrelated robbery was reported in another district. ".repeat(12);
    const hidden = "This allegation is hidden behind a paywall.";
    for (const body of [
      `<article>${preview}</article><main><article><h1>Cannabis seizure</h1><p>${story}</p><p hidden>${hidden}</p></article></main>`,
      `<h1>Cannabis seizure</h1><article>${preview}</article><main><app-article-paragraph>${story}</app-article-paragraph><p style="display:none">${hidden}</p></main>`,
      `<h1>Cannabis seizure</h1><article>${preview}</article><section>${story}</section><p hidden>${hidden}</p>`,
    ]) {
      await page.setContent(`<html><head><title>Cannabis seizure</title><meta name="description" content="${hidden}"></head><body>${body}</body></html>`);
      const result = await page.evaluate(extractSourcePage);
      assert.ok(result.text.includes(story.trim()), result.text);
      assert.ok(!result.text.includes(hidden), "hidden content became quotation evidence");
      if (body.includes("<main>")) assert.ok(!result.text.includes(preview.trim()), "selected the related-story card");
    }
  } finally { await browser.close(); }
});

test("Playwright captures JavaScript text, SEO metadata, redirects and screenshots through the proxy", { skip: !browserAvailable, timeout: 45000 }, async () => {
  const dir = await fs.mkdtemp(path.join(os.tmpdir(), "tracker-source-fixture-"));
  const requests = [];
  const text = "Police reported a cannabis seizure after stopping a truck. The opened article describes the incident and the subsequent arrest. ".repeat(4);
  const app = `setTimeout(async () => {
    const data = await fetch('/api/story').then(response => response.json());
    document.querySelector('main').textContent = data.text;
    document.title = 'Rendered cannabis report';
  }, 100);`;
  const html = `<!doctype html><html lang="en"><head><title>Loading story</title>
    <meta name="csrf-token" content="must-not-export"><meta name="description" content="Description of the opened report"><meta property="og:site_name" content="Fixture News">
    <meta property="og:image" content="https://images.example/cover.jpg"><meta name="twitter:card" content="summary_large_image">
    <link rel="canonical" href="/canonical-story"><script type="application/ld+json">{"@graph":[{"@type":"NewsArticle","author":{"@type":"Person","name":"Fixture Author"},"datePublished":"2026-10-29T08:00:00Z"}]}</script>
    <script src="/app.js" defer></script></head><body><main>Loading…</main></body></html>`;
  // the loader's proxy, playing the publisher too: plain HTTP arrives in absolute form, and only
  // with the capture's credentials, which Chromium sends once asked (407)
  const auth = "Basic " + Buffer.from("capture:fixture-token").toString("base64");
  let challenged = false;
  const agents = new Set();
  const bridge = http.createServer((req, res) => {
    if (req.headers["proxy-authorization"] !== auth) { challenged = true; res.writeHead(407, { "Proxy-Authenticate": 'Basic realm="capture"' }); res.end(); return; }
    requests.push(req.url); agents.add(req.headers["user-agent"]);
    const pathname = new URL(req.url).pathname;
    let status = 200, headers = { "content-type": "text/html" }, body = html;
    if (pathname === "/start") { status = 302; headers.location = "http://news.example/story"; body = ""; }
    if (pathname === "/app.js") { headers["content-type"] = "application/javascript"; body = app; }
    if (pathname === "/api/story") { headers["content-type"] = "application/json"; body = JSON.stringify({ text }); }
    res.writeHead(status, headers); res.end(body);
  });
  await new Promise(resolve => bridge.listen(0, "127.0.0.1", resolve));
  try {
    const result = await captureSource({ url: "http://news.example/start", dir, proxy: `http://127.0.0.1:${bridge.address().port}`, token: "fixture-token" });
    assert.equal(result.error, ""); assert.equal(result.evidence.final_url, "http://news.example/story");
    assert.ok(challenged, "the proxy's password was never asked for");
    assert.ok([...agents].every(a => /Chrome\/\d+/.test(a) && !/Headless/.test(a)), [...agents].join(" | "));
    assert.equal(result.evidence.text, text.trim()); assert.equal(result.evidence.rendered, true);
    assert.equal(result.evidence.metadata.title, "Rendered cannabis report");
    assert.equal(result.evidence.metadata.description, "Description of the opened report");
    assert.equal(result.evidence.metadata.meta["csrf-token"], undefined);
    assert.equal(result.evidence.metadata.canonical, "http://news.example/canonical-story");
    assert.deepEqual(result.evidence.metadata.authors, ["Fixture Author"]);
    assert.equal(result.evidence.metadata.published_at, "2026-10-29T08:00:00Z");
    assert.equal(result.evidence.metadata.open_graph["og:image"], "https://images.example/cover.jpg");
    assert.equal(result.evidence.metadata.twitter["twitter:card"], "summary_large_image");
    assert.equal(result.evidence.metadata.json_ld.length, 1);
    assert.ok(requests.some(url => url.endsWith("/app.js"))); assert.ok(requests.some(url => url.endsWith("/api/story")));
    const screenshot = await fs.readFile(path.join(dir, "page.png"));
    assert.deepEqual([...screenshot.subarray(0, 8)], [137,80,78,71,13,10,26,10]);
    assert.equal((await fs.stat(path.join(dir, "page.png"))).mode & 0o777, 0o600);
    assert.match(await fs.readFile(path.join(dir, "metadata.json"), "utf8"), /Fixture Author/);
  } finally { await new Promise(resolve => bridge.close(resolve)); await fs.rm(dir, { recursive: true, force: true }); }
});
