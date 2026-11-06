// The report photos' manifest (src/data/media.json): for every photo a report links with a direct
// `media_url`, the SHA-256 of the original file, its size and type, a ThumbHash placeholder (a
// blurred preview of about 25 bytes, shown while the photo loads) and how it was obtained.
// The original goes to the private archive, content-addressed (archive/media/<sha256>.<ext>), and a
// 320px AVIF thumbnail to .cache/media/<ab>/<cd>/<sha256>.avif, served only where MEDIA_BASE says
// (src/lib/media.js). Neither is committed or published: the photos belong to their publishers.
//
//   node scripts/media-manifest.mjs              photos not in the manifest yet
//   node scripts/media-manifest.mjs --retry      also try again those that failed
//   node scripts/media-manifest.mjs --thumbs     (re)make missing thumbnails from archived originals
import fs from "node:fs";
import path from "node:path";
import crypto from "node:crypto";
import sharp from "sharp";
import { load } from "js-yaml";
import { rgbaToThumbHash } from "thumbhash";

const MANIFEST = "src/data/media.json", ORIGINALS = "archive/media", THUMBS = ".cache/media";
// the same browser identity as archive-media.mjs: many news sites refuse unknown clients
const UA = "Mozilla/5.0 (Macintosh; Intel Mac OS X 14_0) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/126 Safari/537.36";
const retry = process.argv.includes("--retry"), thumbsOnly = process.argv.includes("--thumbs");
const manifest = fs.existsSync(MANIFEST) ? JSON.parse(fs.readFileSync(MANIFEST, "utf8")) : {};

// every photo the reports link, with the page it was published on
const wanted = new Map();
for (const p of fs.globSync("reports/**/*.md")) {
  const t = fs.readFileSync(p, "utf8"), e = t.indexOf("\n---", 4);
  let d; try { d = load(t.slice(4, e)); } catch { continue; }
  for (const m of d.media ?? []) if (m.kind === "photo" && /^https?:\/\//.test(m.media_url ?? "")) wanted.set(m.media_url, m.url);
}
// what archive-media.mjs already keeps, by image address
const archived = new Map();
for (const f of fs.globSync("archive/*/manifest.json")) {
  try { for (const x of JSON.parse(fs.readFileSync(f, "utf8"))) archived.set(x.image, path.join(path.dirname(f), x.file)); } catch {}
}

const thumbPath = (sha) => path.join(THUMBS, sha.slice(0, 2), sha.slice(2, 4), `${sha}.avif`);
const originalOf = (sha) => fs.globSync(`${ORIGINALS}/${sha}.*`)[0];

async function describe(buf) {
  const img = sharp(buf, { failOn: "none", animated: false });
  const meta = await img.metadata();
  if (!meta.width || !meta.height) throw new Error("not an image");
  const small = await sharp(buf, { failOn: "none" }).rotate().resize(100, 100, { fit: "inside" }).ensureAlpha().raw().toBuffer({ resolveWithObject: true });
  const hash = Buffer.from(rgbaToThumbHash(small.info.width, small.info.height, small.data)).toString("base64");
  const upright = (meta.orientation ?? 1) >= 5;
  return { width: upright ? meta.height : meta.width, height: upright ? meta.width : meta.height, type: `image/${meta.format}`, thumbhash: hash };
}
async function thumbnail(buf, sha) {
  const out = thumbPath(sha);
  if (fs.existsSync(out)) return;
  fs.mkdirSync(path.dirname(out), { recursive: true });
  await sharp(buf, { failOn: "none" }).rotate().resize(320, 320, { fit: "inside", withoutEnlargement: true }).avif({ quality: 50, effort: 4 }).toFile(out);
}
async function download(url, page) {
  const res = await fetch(url, { headers: { "User-Agent": UA, ...(page ? { Referer: page } : {}) }, signal: AbortSignal.timeout(25000), redirect: "follow" });
  if (!res.ok) throw new Error(`HTTP ${res.status}`);
  const type = res.headers.get("content-type") ?? "";
  if (type && !/^image\//.test(type) && !/octet-stream/.test(type)) throw new Error(`not an image (${type.split(";")[0]})`);
  const buf = Buffer.from(await res.arrayBuffer());
  if (buf.length > 25 * 1024 * 1024) throw new Error("larger than 25 MB");
  return buf;
}

async function one(url, page) {
  const old = manifest[url];
  if (thumbsOnly) {
    if (old?.sha256 && !fs.existsSync(thumbPath(old.sha256))) { const f = originalOf(old.sha256); if (f) await thumbnail(fs.readFileSync(f), old.sha256); }
    return;
  }
  if (old && (old.status === "ok" || !retry)) return;
  try {
    const local = archived.get(url);
    const buf = local && fs.existsSync(local) ? fs.readFileSync(local) : await download(url, page);
    const sha256 = crypto.createHash("sha256").update(buf).digest("hex");
    const info = await describe(buf);
    const ext = info.type.split("/")[1].replace("jpeg", "jpg");
    fs.mkdirSync(ORIGINALS, { recursive: true });
    if (!originalOf(sha256)) fs.writeFileSync(path.join(ORIGINALS, `${sha256}.${ext}`), buf);
    await thumbnail(buf, sha256);
    manifest[url] = { status: "ok", sha256, ...info, bytes: buf.length, fetched: new Date().toISOString(), via: local ? "archive" : "download" };
  } catch (e) {
    manifest[url] = { status: "unavailable", error: String(e.message ?? e).slice(0, 160), fetched: new Date().toISOString() };
  }
}

// the reports' own cover images (src/assets/reports/): only a placeholder, keyed "cover:<name>"
for (const f of fs.globSync("src/assets/reports/*.{jpg,jpeg,png,webp,avif}")) {
  const key = `cover:${path.basename(f).replace(/\.[^.]+$/, "")}`;
  if (manifest[key]?.status === "ok" && !retry) continue;
  const buf = fs.readFileSync(f);
  try { manifest[key] = { status: "ok", sha256: crypto.createHash("sha256").update(buf).digest("hex"), ...(await describe(buf)), bytes: buf.length, via: "local" }; } catch {}
}
const save = () => {
  const sorted = Object.fromEntries(Object.entries(manifest).filter(([u]) => wanted.has(u) || u.startsWith("cover:")).sort(([a], [b]) => a.localeCompare(b)));
  fs.writeFileSync(MANIFEST, JSON.stringify(sorted, null, 0).replace(/},"/g, '},\n"') + "\n");
};
const queue = [...wanted];
let done = 0;
await Promise.all(Array.from({ length: 8 }, async () => {
  while (queue.length) {
    const [url, page] = queue.shift();
    await one(url, page);
    if (++done % 200 === 0) { save(); console.log(`${done}/${wanted.size}`); }
  }
}));
save();
const all = Object.values(manifest), ok = all.filter((m) => m.status === "ok").length;
console.log(`${wanted.size} photos: ${ok} described, ${all.length - ok} unavailable. Manifest: ${MANIFEST}`);
