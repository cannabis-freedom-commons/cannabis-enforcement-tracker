// Moves enforcement events from the Cannabis Freedom Index's history (changes.yaml and articles/) into
// reports/, by the routing rule in docs/CFI.md: a single raid, seizure, eradication, arrest,
// prosecution or sentence since 2000 that changed no law comes here; the CFI keeps what changed the
// law or a score, history before 2000, famous and landmark cases, rulings that set a precedent,
// national statistics and anything that is not an event.
//
// Each report keeps the CFI entry's id as its slug and in `cfi_id`, so the CFI can redirect
// /history/<id>/ here and repoint its links. Run it, check the result, then remove the moved entries
// from the CFI (its scripts/move-to-tracker.mjs reads `cfi_id` from these files).
//
//   node scripts/import-cfi-history.mjs [path to the CFI checkout] [--dry]
import fs from "node:fs";
import path from "node:path";
import * as yaml from "js-yaml";
import { codeOf } from "../src/lib/code.js";
import { execSync } from "node:child_process";

const args = process.argv.slice(2);
const DRY = args.includes("--dry");
const cfi = path.resolve(args.find((a) => !a.startsWith("--")) || "../cannabis-freedom-index");
const CFI = "https://cannabisfreedom.fyi";
const TRACKER = "https://tracker.cannabisfreedom.fyi";
const places = new Set(JSON.parse(fs.readFileSync("src/data/countries.json", "utf8")).map((c) => c.id));
const items = yaml.load(fs.readFileSync(path.join(cfi, "changes.yaml"), "utf8"));

// ── what moves ────────────────────────────────────────────────────────────────
// entries the CFI keeps by name: famous or landmark cases and items that are not enforcement events
const KEEP = new Set([
  "russia-griner-sentence-2022", "mauritania-haidalla-drug-case-2016", "philippines-cruz-release-2000",
  "seychelles-ah-kong-suspended-sentence-2016", "papua-new-guinea-gulf-convictions-2024",
  "russia-wikipedia-charas-2015", "ireland-health-referral-scheme-2026", "albania-no-licences-issued-2026",
  "burkina-faso-mainline-sheet-2025", "puerto-rico-family-dispensary-closure-2020", "palau-thc-gummies-2026",
  "equatorial-guinea-open-use-2000", "ethiopia-us-warning-rastafari-2005", "senegal-incb-mission-cultivation-2000",
  "vietnam-amnesty-death-sentences-2025", "kuwait-drug-cases-drop-2026",
]);
const KEEP_TAGS = ["famous", "amnesty", "campaign", "police-powers"];
// entries a cultural atlas entry lists (its `history` or `history_cards`): part of a cultural story
const ATLAS = new Set();
for (const f of fs.readdirSync(path.join(cfi, "culture"), { recursive: true }).filter((f) => f.endsWith(".md"))) {
  const fm = fs.readFileSync(path.join(cfi, "culture", f), "utf8").match(/^---\n([\s\S]*?)\n---/)?.[1];
  const d = fm ? yaml.load(fm) : {};
  for (const id of [...(d.history ?? []), ...(d.history_cards ?? [])]) ATLAS.add(id);
}
// entries the CFI's code names (src/: the did-you-know facts, page texts): chosen by hand, so they stay
const IN_CODE = new Set();
for (const f of fs.readdirSync(path.join(cfi, "src"), { recursive: true }).filter((f) => /\.(js|ts|astro|mjs)$/.test(f))) {
  const text = fs.readFileSync(path.join(cfi, "src", f), "utf8");
  for (const m of text.matchAll(/["'\/:]([a-z0-9]+(?:-[a-z0-9]+)+)["'|\/]/g)) IN_CODE.add(m[1]);
}
// entries linked from a file someone is editing in the CFI right now (uncommitted changes): moving
// them would leave a link that can't be fixed yet, so they wait for a later run
const BUSY_LINKS = new Set();
for (const f of execSync("git status --porcelain", { cwd: cfi, encoding: "utf8" }).split("\n").filter(Boolean).map((l) => l.slice(3))) {
  const file = path.join(cfi, f);
  if (!/\.(md|ya?ml)$/.test(f) || !fs.existsSync(file) || fs.statSync(file).isDirectory()) continue;
  for (const m of fs.readFileSync(file, "utf8").matchAll(/\/history\/([a-z0-9-]+)\//g)) BUSY_LINKS.add(m[1]);
}
// national statistics for a year or a period, and court rulings that set a precedent
const STATISTICS = /\b(reports?|records?|counts?|says?)\b.*\b(in|for|since|during) (19|20)\d\d\b|\bstatistic|\bannual\b|\bfigures\b|more than doubled|cases (fall|drop|rise)|seizures (reach|total|rise|fall|double|top|exceed)|\b(19|20)\d\d\b[^,]*\bseizures\b|\b(in|over) (a|the) (year|half-year|first half)|\bin (nine|six|three|ten|eleven) months\b/i;
const PRECEDENT = /\bcourt (rules|overturns|strikes)|\brules that\b|unconstitutional/i;

function routing(i) {
  const year = +String(i.date).slice(0, 4);
  if (i.category !== "enforcement") return "not enforcement";
  if (year < 2000) return "before 2000";
  if (KEEP.has(i.id)) return "kept by name (famous, landmark or not an event)";
  if (ATLAS.has(i.id)) return "part of a cultural atlas entry";
  if (IN_CODE.has(i.id)) return "named in the CFI's code (a did-you-know fact)";
  if (BUSY_LINKS.has(i.id)) return "linked from a file being edited in the CFI (later)";
  if ((i.tags ?? []).some((t) => KEEP_TAGS.includes(t))) return "famous or policy tag";
  if (i.before != null) return "moves a score";
  if (!i.country) return "international";
  if (STATISTICS.test(i.title)) return "national statistics";
  if (PRECEDENT.test(i.title)) return "a ruling that sets a precedent";
  if (!places.has(placeOf(i))) return `unknown place ${placeOf(i)}`;
  return null;
}
const placeOf = (i) => (i.subunit ? `${i.country}/${i.subunit}` : i.country);

// ── the report's type, from the CFI tags and the title ─────────────────────────
function typeOf(i) {
  const t = i.title.toLowerCase(), tags = i.tags ?? [];
  if (/\bexecut|\bhang(s|ed)\b/.test(t)) return "execution";
  if (/\b(dies|died|killed|shot dead|death in custody)\b/.test(t)) return "death";
  if (tags.includes("eradication") || /\b(destroy|uproot|eradicat|burns? .*(field|plant|crop)|pull up|cut down)/.test(t)) return "eradication";
  if (tags.includes("sentencing") || /\b(jail|prison|sentenc|years? for|life for|fine[sd]?\b|lash|cane|strokes|acquit|community work|suspend)/.test(t)) return "sentence";
  if (tags.includes("court") || /\b(court|charge|prosecut|trial|indict)/.test(t)) return "prosecution";
  if (/\braid/.test(t)) return "raid";
  if (/\b(arrest|detain|held|nab)/.test(t)) return "arrest";
  if (/\b(seiz|find|found|confiscat|intercept|stop|catch|burn|haul|bust)/.test(t)) return "seizure";
  return "seizure";
}

// ── sources: the CFI's {type, title "Publisher — Title", url, locator, …} as the tracker's ─────
function source(s) {
  const [pub, ...rest] = String(s.title).split(" — ");
  const hasPub = rest.length > 0;
  const out = {
    title: hasPub ? rest.join(" — ") : s.title,
    publisher: s.publisher ?? (hasPub ? pub : s.container ?? (s.url ? new URL(s.url).hostname.replace(/^www\./, "") : "Unknown")),
  };
  if (s.url) out.url = s.url;
  if (s.type && s.type !== "web") out.type = s.type;
  for (const k of ["authors", "container", "year", "isbn", "locator"]) if (s[k] != null) out[k] = s[k];
  return out;
}
const hasDetails = (s) => s.url || (s.type && s.type !== "web" && (s.publisher || s.container || s.isbn || s.year));

// a summary of at most 400 characters: whole sentences from the description, without its
// citation markers ([@1], [@1,2], [@key]), which only the report's text can resolve
function summaryOf(text) {
  const clean = String(text).replace(/\s*\[@[^\]]*\]/g, "").replace(/\s+/g, " ").trim();
  if (clean.length <= 400) return clean;
  let out = "";
  // sentences end at . ! or ? followed by a space and a capital, so "211.5 tonnes" is not cut
  for (const s of clean.split(/(?<=[.!?])\s+(?=[A-Z"“‘(])/)) { if ((out + " " + s).trim().length > 400) break; out = `${out} ${s}`.trim(); }
  return (out || clean.slice(0, clean.lastIndexOf(" ", 396)) + "…").trim();
}

// links in an article: a moved entry's history page becomes its report here, any other address of
// the CFI becomes a full link to the CFI
const moved = new Set(items.filter((i) => !routing(i)).map((i) => i.id));
const relink = (body) => body.replace(/\]\((\/[^)\s]*)\)/g, (m, href) => {
  const h = href.match(/^\/history\/([a-z0-9-]+)\/?(#.*)?$/);
  if (h && moved.has(h[1])) return `](/report/${h[1]}/${h[2] ?? ""})`;
  return `](${CFI}${href})`;
});

// the codes already taken, so a new one never repeats them
const usedCodes = new Set(fs.readdirSync("reports", { recursive: true }).filter((f) => f.endsWith(".md"))
  .map((f) => fs.readFileSync(path.join("reports", f), "utf8").match(/^code: ([0-9A-Z]{6})$/m)?.[1]).filter(Boolean));
const existing = new Set(fs.readdirSync("reports", { recursive: true }).filter((f) => f.endsWith(".md")).map((f) => path.basename(f, ".md")));

// ── write ────────────────────────────────────────────────────────────────────
const stats = { moved: 0, types: {}, kept: {} };
for (const i of items) {
  const why = routing(i);
  if (why) { if (why !== "not enforcement") stats.kept[why] = (stats.kept[why] ?? 0) + 1; continue; }
  const date = String(i.date).length === 4 ? `${i.date}-07-01` : String(i.date).length === 7 ? `${i.date}-15` : String(i.date).slice(0, 10);
  const articleFile = path.join(cfi, "articles", `${i.id}.md`);
  let sources, body;
  if (fs.existsSync(articleFile)) {
    const m = fs.readFileSync(articleFile, "utf8").match(/^---\n([\s\S]*?)\n---\n([\s\S]*)$/);
    const fm = yaml.load(m[1]);
    sources = (fm.sources ?? []).map(source);
    body = relink(m[2]).trim();
    if (!/\[@\d/.test(body)) body += ` [@${sources.map((_, k) => k + 1).join(",")}]`;
  } else {
    sources = (i.sources ?? []).filter(hasDetails).map(source);
    body = `${String(i.description).replace(/\s+/g, " ").trim()} [@${sources.map((_, k) => k + 1).join(",")}]`;
  }
  if (!sources.length) { stats.kept["no usable source"] = (stats.kept["no usable source"] ?? 0) + 1; moved.delete(i.id); continue; }
  if (existing.has(i.id)) { stats.kept["already here"] = (stats.kept["already here"] ?? 0) + 1; continue; }
  let code, k = 0;
  do code = codeOf(k ? `${i.id}#${k}` : i.id), k++; while (usedCodes.has(code));
  usedCodes.add(code);
  const type = typeOf(i);
  stats.types[type] = (stats.types[type] ?? 0) + 1;
  const data = {
    code, date, country: placeOf(i), type,
    title: i.title, summary: summaryOf(i.description),
    status: i.checked ? "verified" : "reported",
    created_by_model: i.created_by_model || "unknown", // preserve provenance; never guess a historical author model
    cfi_id: i.id,
    ...(i.tags?.length && { tags: i.tags.filter((t) => /^[a-z0-9-]+$/.test(t)) }),
    sources,
  };
  const file = path.join("reports", date.slice(0, 4), date.slice(5, 7), `${i.id}.md`);
  if (!DRY) {
    fs.mkdirSync(path.dirname(file), { recursive: true });
    fs.writeFileSync(file, `---\n${yaml.dump(data, { lineWidth: 100, quotingType: '"' })}---\n\n${body}\n`);
  }
  stats.moved++;
}
console.log(JSON.stringify(stats, null, 1));
if (DRY) console.log("(dry run: nothing written)");
