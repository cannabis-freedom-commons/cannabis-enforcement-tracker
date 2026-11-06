import { test } from "node:test";
import assert from "node:assert/strict";
import sharp from "sharp";
import { rgbaToThumbHash } from "thumbhash";
import { placeholderOf, thumbUrlOf, photoOf } from "../src/lib/media-manifest.js";

test("a ThumbHash becomes a small PNG placeholder", async () => {
  const { data, info } = await sharp({ create: { width: 40, height: 30, channels: 4, background: "#3a7d2c" } }).raw().toBuffer({ resolveWithObject: true });
  const hash = Buffer.from(rgbaToThumbHash(info.width, info.height, data)).toString("base64");
  assert.ok(hash.length <= 48, `hash of ${hash.length} characters`);
  const url = placeholderOf(hash);
  assert.match(url, /^data:image\/png;base64,/);
  assert.ok(url.length < 2000);
  assert.equal(placeholderOf(null), null);
});

test("without MEDIA_BASE photos load through the proxy, as before", () => {
  assert.equal(thumbUrlOf("ab".repeat(32)), null);
  const p = photoOf("https://unknown.example/a.jpg");
  assert.match(p.src, /^https:\/\/external-content\.duckduckgo\.com\//);
  assert.equal(p.src, p.full);
  assert.equal(p.placeholder, null);
});
