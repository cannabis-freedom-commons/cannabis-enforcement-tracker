// Adds structured details to existing reports from their tags and cited sentences
// (scripts/lib/backfill-details.mjs). Dry run by default; --write changes the files.
//   node scripts/backfill-details.mjs [--write] [--limit N]
import { readFileSync, writeFileSync, globSync } from "node:fs";
import { load as loadYAML } from "js-yaml";
import { backfillDetails, detailsYAML, insertDetails } from "./lib/backfill-details.mjs";

const write = process.argv.includes("--write");
const limitAt = process.argv.indexOf("--limit");
const limit = limitAt > 0 ? Number(process.argv[limitAt + 1]) : Infinity;
const counts = {};
let changed = 0, seen = 0;
for (const path of globSync("reports/**/*.md").sort()) {
  if (changed >= limit) break;
  const text = readFileSync(path, "utf8");
  const end = text.indexOf("\n---", 4);
  if (!text.startsWith("---\n") || end < 0) continue;
  seen++;
  let data;
  try { data = loadYAML(text.slice(4, end)); } catch { continue; }
  const found = backfillDetails(data, text.slice(end + 4), data.sources?.length ?? 0);
  if (!found) continue;
  const next = insertDetails(text, detailsYAML(found));
  if (!next) continue;
  for (const key of Object.keys(found.details)) counts[key] = (counts[key] ?? 0) + 1;
  changed++;
  if (write) writeFileSync(path, next);
}
console.log(`${write ? "Updated" : "Would update"} ${changed} of ${seen} reports.`);
console.log(Object.entries(counts).sort((a, b) => b[1] - a[1]).map(([k, n]) => `  ${k}: ${n}`).join("\n"));
if (!write) console.log("Dry run: add --write to change the files.");
