// What the media manifest (src/data/media.json, scripts/media-manifest.mjs) knows about a report
// photo: its size, so the page reserves the space and nothing jumps while it loads, a ThumbHash
// placeholder shown until then, and our own small thumbnail (public/media/) where there is one.
// Build time only: the manifest is not sent to readers (lists get a row's hash, see reports.js).
import manifest from "../data/media.json" with { type: "json" };
import { thumbHashToRGBA } from "thumbhash";
import { deflateSync, crc32 } from "node:zlib";
import { existsSync } from "node:fs";
import { imageProxy } from "./media.js";

// the 320px AVIF thumbnails: public/media/<ab>/<cd>/<sha256>.avif, served by the site at /media/
// (MEDIA_BASE moves them elsewhere). A photo without one loads from its publisher through the proxy.
const MEDIA_BASE = (import.meta.env?.MEDIA_BASE ?? process.env.MEDIA_BASE ?? "").replace(/\/$/, "");
const BASE = (import.meta.env?.BASE_URL ?? "/").replace(/\/?$/, "/");

// the placeholder as a small compressed PNG (a few hundred bytes; ThumbHash's own data URL is
// uncompressed, about 5 KB, too much to inline for every photo)
const chunk = (type, data) => {
  const head = Buffer.alloc(8); head.writeUInt32BE(data.length); head.write(type, 4, "ascii");
  const crc = Buffer.alloc(4); crc.writeUInt32BE(crc32(Buffer.concat([head.subarray(4), data])) >>> 0);
  return Buffer.concat([head, data, crc]);
};
function png(w, h, rgba) {
  const raw = Buffer.alloc((w * 4 + 1) * h);
  for (let y = 0; y < h; y++) Buffer.from(rgba.buffer, rgba.byteOffset + y * w * 4, w * 4).copy(raw, y * (w * 4 + 1) + 1);
  const ihdr = Buffer.alloc(13); ihdr.writeUInt32BE(w); ihdr.writeUInt32BE(h, 4); ihdr[8] = 8; ihdr[9] = 6;
  return Buffer.concat([Buffer.from([137, 80, 78, 71, 13, 10, 26, 10]), chunk("IHDR", ihdr), chunk("IDAT", deflateSync(raw, { level: 9 })), chunk("IEND", Buffer.alloc(0))]);
}
const placeholders = new Map();
export const placeholderOf = (hash) => {
  if (!hash) return null;
  if (!placeholders.has(hash)) {
    const { w, h, rgba } = thumbHashToRGBA(Uint8Array.from(Buffer.from(hash, "base64")));
    // at most 10 pixels across: the browser scales it up smoothly, which is the blur
    const scale = Math.min(1, 10 / Math.max(w, h)), sw = Math.max(1, Math.round(w * scale)), sh = Math.max(1, Math.round(h * scale));
    const small = new Uint8Array(sw * sh * 4);
    for (let y = 0; y < sh; y++) for (let x = 0; x < sw; x++) {
      const from = (Math.min(h - 1, Math.floor((y + 0.5) / scale)) * w + Math.min(w - 1, Math.floor((x + 0.5) / scale))) * 4;
      small.set(rgba.subarray(from, from + 4), (y * sw + x) * 4);
    }
    placeholders.set(hash, `data:image/png;base64,${png(sw, sh, small).toString("base64")}`);
  }
  return placeholders.get(hash);
};
const thumbFile = (sha) => `${sha.slice(0, 2)}/${sha.slice(2, 4)}/${sha}.avif`;
export const thumbUrlOf = (sha) => {
  if (!sha) return null;
  if (MEDIA_BASE) return `${MEDIA_BASE}/${thumbFile(sha)}`;
  return existsSync(`public/media/${thumbFile(sha)}`) ? `${BASE}media/${thumbFile(sha)}` : null;
};

// a cover image's placeholder, by its file name (Astro serves it as /_astro/<name>.<hash>.<ext>)
export const coverPlaceholderOf = (src) => {
  const name = String(src ?? "").split("/").pop().split(".")[0];
  return placeholderOf(manifest[`cover:${name}`]?.thumbhash);
};

// a photo as the page shows it: { src (small), full (to enlarge), width, height, placeholder, hash }
export function photoOf(url) {
  const m = manifest[url];
  const full = imageProxy(url);
  if (m?.status !== "ok") return { src: full, full, width: null, height: null, placeholder: null, hash: null };
  return { src: thumbUrlOf(m.sha256) ?? full, full, width: m.width, height: m.height, placeholder: placeholderOf(m.thumbhash), hash: m.thumbhash, thumb: thumbUrlOf(m.sha256) };
}
