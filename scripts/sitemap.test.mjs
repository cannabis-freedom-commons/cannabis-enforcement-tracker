import { test } from "node:test";
import assert from "node:assert/strict";
import fs from "node:fs/promises";
import os from "node:os";
import path from "node:path";
import { pageEntry, writeSitemap } from "./lib/sitemap.mjs";

const page = (head) => `<!doctype html><html><head>${head}</head><body><p>noindex in the body changes nothing</p></body></html>`;

test("sitemap lists indexable pages with their last change, never redirects or noindex pages", () => {
  assert.deepEqual(pageEntry(page(`<meta name="robots" content="index, follow"><link rel="canonical" href="https://t.example/report/a/"><meta property="article:published_time" content="2026-11-01T10:00:00Z"><meta property="article:modified_time" content="2026-11-02T10:00:00Z">`)),
    { loc: "https://t.example/report/a/", lastmod: "2026-11-02T10:00:00Z" });
  assert.deepEqual(pageEntry(page(`<link rel="canonical" href="https://t.example/country/de/">`)), { loc: "https://t.example/country/de/" });
  assert.equal(pageEntry(page(`<meta name="robots" content="noindex, follow"><link rel="canonical" href="https://t.example/submissions/">`)), null);
  assert.equal(pageEntry(page(`<meta http-equiv="refresh" content="0;url=/report/a/"><link rel="canonical" href="https://t.example/report/a/">`)), null);
  assert.equal(pageEntry(page("<title>no canonical</title>")), null);
});

test("sitemap.xml and robots.txt are written under the base path", async () => {
  const dir = await fs.mkdtemp(path.join(os.tmpdir(), "tracker-sitemap-"));
  try {
    await fs.mkdir(path.join(dir, "report", "a"), { recursive: true });
    await fs.writeFile(path.join(dir, "report", "a", "index.html"), page(`<link rel="canonical" href="https://t.example/cet/report/a/">`));
    await fs.writeFile(path.join(dir, "index.html"), page(`<link rel="canonical" href="https://t.example/cet/">`));
    await fs.writeFile(path.join(dir, "404.html"), page(`<link rel="canonical" href="https://t.example/cet/404/">`));
    assert.equal(await writeSitemap(dir, new URL("https://t.example/cet/")), 2);
    const xml = await fs.readFile(path.join(dir, "sitemap.xml"), "utf8");
    assert.match(xml, /<loc>https:\/\/t\.example\/cet\/report\/a\/<\/loc>/);
    assert.doesNotMatch(xml, /404/);
    assert.match(await fs.readFile(path.join(dir, "robots.txt"), "utf8"), /^Sitemap: https:\/\/t\.example\/cet\/sitemap\.xml$/m);
  } finally { await fs.rm(dir, { recursive: true, force: true }); }
});
