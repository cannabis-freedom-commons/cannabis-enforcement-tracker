// Caption-signal scan for stock / AI / file / illustration media already linked on reports.
// Used by the unlisted review page at /review/media-stock/.
import { reports, isoDate } from "./reports.js";
import { photoOf } from "./media-manifest.js";

const PATTERNS = [
  [/\bAI[- ]generated\b/i, "ai-generated"],
  [/\bartificial intelligence\b/i, "ai-generated"],
  [/\bshutterstock\b/i, "shutterstock"],
  [/\bgetty images\b/i, "getty"],
  [/\bistock\b/i, "istock"],
  [/\bpixabay\b/i, "pixabay"],
  [/\bunsplash\b/i, "unsplash"],
  [/\brawpixel\b/i, "rawpixel"],
  [/\bdepositphotos\b/i, "depositphotos"],
  [/\badobe stock\b/i, "adobe-stock"],
  [/\bfreepik\b/i, "freepik"],
  [/\bsymbolbild\b/i, "symbolbild"],
  [/\bstock\b/i, "stock"],
  [/\bphoto illustration\b/i, "photo-illustration"],
  [/\bphoto d['']illustration\b/i, "illustration"],
  [/\billustration\b/i, "illustration"],
  [/\billustrative\b/i, "illustrative"],
  [/\bfile photo\b/i, "file-photo"],
  [/\barchive photo\b/i, "archive-photo"],
  [/\barchivo\b/i, "archivo"],
  [/\brepresentational\b/i, "representative"],
  [/\brepresentative\b/i, "representative"],
  [/\bgeneric\b/i, "generic"],
  [/\bnot established to show\b/i, "not-established"],
  [/\bnot a (photo|picture|photograph) of this\b/i, "not-of-this"],
  [/\bimage library\b/i, "image-library"],
];

const HARD = new Set([
  "ai-generated", "shutterstock", "getty", "istock", "pixabay", "unsplash", "rawpixel",
  "depositphotos", "adobe-stock", "freepik", "symbolbild", "stock", "photo-illustration",
  "illustration", "illustrative", "file-photo", "archive-photo", "archivo", "representative",
  "generic", "not-of-this", "image-library",
]);

function flagsOf(text) {
  const out = [];
  for (const [re, name] of PATTERNS) if (re.test(text) && !out.includes(name)) out.push(name);
  return out;
}

function strength(flags) {
  if (flags.some((f) => ["ai-generated", "shutterstock", "getty", "istock", "pixabay", "unsplash", "rawpixel", "depositphotos", "adobe-stock", "freepik"].includes(f))) return "hard";
  if (flags.some((f) => HARD.has(f))) return "strong";
  return "soft";
}

export function mediaReviewCandidates() {
  const items = [];
  for (const r of reports) {
    const media = Array.isArray(r.media) ? r.media : [];
    media.forEach((m, index) => {
      if (m.kind && m.kind !== "photo") return;
      const text = [m.caption, m.credit, m.publisher, m.url, m.media_url].filter(Boolean).join(" | ");
      const flags = flagsOf(text);
      if (!flags.length) return;
      const url = m.media_url || "";
      const photo = /^https?:\/\//.test(url) ? photoOf(url) : null;
      items.push({
        id: `${r.id}::${index}`,
        slug: r.id,
        reportUrl: r.url,
        title: r.title,
        date: isoDate(r.date),
        country: r.country,
        countryName: r.countryName,
        index,
        flags,
        strength: strength(flags),
        caption: m.caption || "",
        credit: m.credit || "",
        publisher: m.publisher || "",
        pageUrl: m.url || "",
        mediaUrl: url,
        thumb: photo?.src || null,
        full: photo?.full || null,
        placeholder: photo?.placeholder || null,
        width: photo?.width || null,
        height: photo?.height || null,
      });
    });
  }
  const rank = { hard: 0, strong: 1, soft: 2 };
  return items.sort((a, b) =>
    rank[a.strength] - rank[b.strength]
    || String(b.date).localeCompare(String(a.date))
    || a.slug.localeCompare(b.slug)
    || a.index - b.index
  );
}
