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

test("source extraction reads articles rendered in shadow DOM without the next feed story", { skip: !browserAvailable }, async () => {
  const browser = await chromium.launch({ channel: "chromium", headless: true });
  try {
    const page = await browser.newPage();
    const story = "Investigators searched several flats and found an illegal cannabis grow and weapons. ".repeat(6);
    const next = "An unrelated story about a football match follows in the endless feed. ".repeat(10);
    const banner = "We and our partners use cookies to store and access personal data on your device. ".repeat(4);
    // MSN-like: the headline sits in one component, the body in a nested article component, and
    // the next story is appended below; only a cookie dialog is in the light DOM.
    await page.setContent(`<html><head><title>Big cannabis raid in the district</title></head><body>
      <div id="banner"><h1>Your privacy</h1><p>${banner}</p></div><feed-page></feed-page></body></html>`);
    await page.evaluate(([story, next]) => {
      const attach = (host, html) => { host.attachShadow({ mode: "open" }).innerHTML = html; return host.shadowRoot; };
      const feed = attach(document.querySelector("feed-page"), `<div class="story" id="one"><views-header></views-header><article-body></article-body></div><div class="story" id="two"><views-header></views-header><article-body></article-body></div>`);
      attach(feed.querySelector("#one views-header"), "<h1>Big cannabis raid in the district</h1><span>2 days ago</span>");
      attach(feed.querySelector("#one article-body"), `<article><slot></slot><p>${story}</p><p hidden>Paywalled detail</p></article>`);
      attach(feed.querySelector("#two views-header"), "<h1>Football result</h1>");
      attach(feed.querySelector("#two article-body"), `<article><p>${next}</p></article>`);
    }, [story, next]);
    const result = await page.evaluate(extractSourcePage);
    assert.ok(result.text.includes("Big cannabis raid in the district"), result.text);
    assert.ok(result.text.includes(story.trim()), result.text);
    assert.ok(!result.text.includes("endless feed"), "mixed in the next feed story");
    assert.ok(!result.text.includes("cookies"), "kept the cookie banner instead of the article");
    assert.ok(!result.text.includes("Paywalled detail"), "hidden shadow content became evidence");
    // with the banner refused, as in a real capture
    await page.evaluate(() => document.querySelector("#banner").remove());
    assert.ok((await page.evaluate(extractSourcePage)).text.includes(story.trim()));
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
    <script src="/app.js" defer></script></head><body><main>Loading…</main><img src="/photo.svg" width="320" height="180"></body></html>`;
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
    if (pathname === "/photo.svg") { headers["content-type"] = "image/svg+xml"; body = '<svg xmlns="http://www.w3.org/2000/svg" width="320" height="180"><rect width="320" height="180" fill="blue"/></svg>'; }
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
    assert.equal(result.evidence.screenshot, "page.webp");
    assert.equal(result.evidence.metadata.images.length, 1);
    assert.ok(result.evidence.metadata.images[0].src.startsWith("http") && !("file" in result.evidence.metadata.images[0]));
    await assert.rejects(fs.stat(path.join(dir, "images")), { code: "ENOENT" });
    for (const name of [result.evidence.screenshot]) {
      const file = path.join(dir, name);
      const image = await fs.readFile(file);
      assert.equal(image.toString("ascii", 0, 4), "RIFF");
      assert.equal(image.toString("ascii", 8, 12), "WEBP");
      await assert.rejects(fs.stat(file.replace(/\.webp$/, ".png")), { code: "ENOENT" });
      assert.equal((await fs.stat(file)).mode & 0o777, 0o600);
    }
    assert.deepEqual(JSON.parse(await fs.readFile(path.join(dir, "metadata.json"), "utf8")), result.evidence.metadata);
  } finally { await new Promise(resolve => bridge.close(resolve)); await fs.rm(dir, { recursive: true, force: true }); }
});
