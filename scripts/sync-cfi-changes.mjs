// Copies the dated law changes of the Cannabis Freedom Index (changes.yaml) into
// src/data/cfi-changes.json, compact, for the legal context of each report: what changed in the
// place's law before and after the event (src/lib/legal-context.js). Only the date, kind, place,
// title and whether it moved a score: the CFI's own pages hold the rest.
//
//   node scripts/sync-cfi-changes.mjs [path to the CFI checkout]   (default ../cannabis-freedom-index)
import fs from "node:fs";
import path from "node:path";
import * as yaml from "js-yaml";

const cfi = path.resolve(process.argv[2] || process.env.CFI_DIR || "../cannabis-freedom-index");
const changes = yaml.load(fs.readFileSync(path.join(cfi, "changes.yaml"), "utf8"));
const places = new Set(JSON.parse(fs.readFileSync("src/data/countries.json", "utf8")).map((c) => c.id));
const out = [];
for (const c of changes) {
  if (!c.country) continue; // international and undated-place entries
  const place = c.subunit ? `${c.country}/${c.subunit}` : c.country;
  if (!places.has(place)) continue;
  out.push([String(c.date).slice(0, 10), place, c.category, c.title.trim(), c.unscored ? 0 : 1, c.id]);
}
out.sort((a, b) => a[0].localeCompare(b[0]) || a[5].localeCompare(b[5]));
fs.writeFileSync("src/data/cfi-changes.json", JSON.stringify({ fields: ["date", "place", "category", "title", "scored", "id"], changes: out }) + "\n");
console.log(`${out.length} dated law changes for ${new Set(out.map((c) => c[1])).size} places`);
