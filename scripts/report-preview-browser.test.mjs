import { test } from "node:test";
import assert from "node:assert/strict";
import fs from "node:fs/promises";
import { chromium } from "playwright";

let available = true;
try { await fs.access(chromium.executablePath()); } catch { available = false; }

test("preview photos follow prepared media and saved report edits, with a working gallery", { skip: !available, timeout: 30000 }, async () => {
  const browser = await chromium.launch({ headless: true, executablePath: chromium.executablePath() });
  try {
    const page = await browser.newPage();
    const errors = [];
    page.on("pageerror", error => errors.push(error.message));
    let saved = "current", fail = false;
    await page.route("**/*", async route => {
      const url = new URL(route.request().url());
      if (url.hostname === "external-content.duckduckgo.com") {
        return route.fulfill({ contentType: "image/png", body: Buffer.from("iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8/x8AAwMCAO+jS1sAAAAASUVORK5CYII=", "base64") });
      }
      if (url.pathname.startsWith("/report/")) {
        if (fail) return route.fulfill({ status: 404, body: "Not built yet" });
        return route.fulfill({ contentType: "text/html", body: `<article id="report-article"><header><h1>${saved} saved title</h1></header><figure class="cover"><img alt="Saved cover" src="https://external-content.duckduckgo.com/cover.png"></figure><p>The actual merged report.</p><h2>Photos and video</h2><div data-media-grid class="media-grid">${[1, 2].map(n => `<figure class="media-tile"><button class="media-open"><img alt="${saved} ${n}" src="https://external-content.duckduckgo.com/${saved}-${n}.png"></button><figcaption>${saved} ${n}<span class="media-source"><a href="https://news.example/story">Publisher</a></span></figcaption></figure>`).join("")}</div></article>` });
      }
      if (url.pathname === "/") return route.fulfill({ contentType: "text/html", body: `<script type="importmap">{"imports":{"js-yaml":"/node_modules/js-yaml/dist/js-yaml.mjs"}}</script><main id="preview"></main><script type="module">import {renderDraft} from '/src/scripts/draft-report.js'; window.draw=(v)=>renderDraft(document.querySelector('main'),v,{compact:true});</script>` });
      if (/^\/(src\/|node_modules\/js-yaml\/dist\/)/.test(url.pathname)) {
        return route.fulfill({ contentType: "application/javascript", body: await fs.readFile(new URL(`..${url.pathname}`, import.meta.url)) });
      }
      return route.abort();
    });
    await page.goto("http://tracker.test/");
    await page.waitForFunction(() => window.draw);
    const proposal = { title: "Old proposed title", facts: [{ text: "Proposed fact.", source: 1 }], sources: [{ url: "https://news.example/story" }], media: [{ kind: "photo", caption: "Unselected proposal", url: "https://news.example/story", media_url: "https://news.example/stale.jpg" }] };
    const prepared = { result: proposal, source_check: { images: [{ src: "https://news.example/research-only.jpg" }] }, draft_markdown: "---\nmedia:\n  - kind: photo\n    caption: Selected photo\n    url: https://news.example/story\n    media_url: https://news.example/selected.jpg\n---\nReport." };
    await page.evaluate(v => window.draw(v), prepared);
    assert.deepEqual(await page.locator(".media-open img").evaluateAll(imgs => imgs.map(img => img.alt)), ["Selected photo"]);
    assert.equal(await page.locator('img[src*="source/image"], img[src*="research-only"], img[src*="stale"]').count(), 0);
    await page.evaluate(v => window.draw({ ...v, draft_markdown: "---\nmedia: []\n---\nReport." }), prepared);
    assert.equal(await page.locator("main img").count(), 0);
    // a published report is drawn from its page on the site
    const committed = { ...prepared, status: "published", report_slug: "saved-report" };
    await page.evaluate(v => window.draw(v), committed);
    await page.getByText("current saved title", { exact: true }).waitFor();
    assert.equal(await page.locator("main h1").count(), 0);
    assert.equal(await page.locator(".cover img").getAttribute("alt"), "Saved cover");
    assert.deepEqual(await page.locator(".media-open img").evaluateAll(imgs => imgs.map(img => img.alt)), ["current 1", "current 2"]);
    await page.locator(".media-open").first().click();
    assert.equal(await page.locator("dialog[open] img").getAttribute("alt"), "current 1");
    await page.keyboard.press("ArrowRight");
    assert.equal(await page.locator("dialog[open] img").getAttribute("alt"), "current 2");
    await page.keyboard.press("Escape");
    saved = "edited";
    await page.evaluate(v => window.draw(v), committed);
    await page.getByText("edited saved title", { exact: true }).waitFor();
    assert.equal(await page.locator("dialog").count(), 1);
    assert.equal(await page.getByText("Old proposed title", { exact: true }).count(), 0);
    await page.locator(".media-open img").first().dispatchEvent("error");
    assert.equal(await page.locator("ul.media a").textContent(), "edited 1");
    fail = true;
    await page.evaluate(v => window.draw(v), committed);
    await page.getByText("The saved report is not available on this site yet.", { exact: false }).waitFor();
    assert.equal(await page.locator("main img, dialog").count(), 0);
    // A response belonging to an earlier refresh must not overwrite a newer draft.
    fail = false;
    await page.evaluate(({ committed, prepared }) => { window.draw(committed); window.draw(prepared); }, { committed, prepared });
    await page.waitForLoadState("networkidle");
    assert.deepEqual(await page.locator(".media-open img").evaluateAll(imgs => imgs.map(img => img.alt)), ["Selected photo"]);
    assert.deepEqual(errors, []);
  } finally { await browser.close(); }
});
