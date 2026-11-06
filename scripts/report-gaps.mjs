// Which reports lack what today's writing rules ask for (scripts/lib/report-gaps.mjs), to choose
// work for the enrich-report skill.
//   node scripts/report-gaps.mjs                       counts per gap
//   node scripts/report-gaps.mjs <slug> …               the gaps of these reports
//   node scripts/report-gaps.mjs --gap no-media [--country ng] [--since 2025] [--limit 20]
//                                                       reports with that gap, newest first
//   --json                                              machine-readable output
import { readFileSync, globSync } from "node:fs";
import { basename } from "node:path";
import { load } from "js-yaml";
import { GAPS, reportGaps } from "./lib/report-gaps.mjs";

const args = process.argv.slice(2);
const opt = (name) => { const i = args.indexOf(`--${name}`); return i >= 0 ? args[i + 1] : null; };
const json = args.includes("--json");
const gap = opt("gap"), country = opt("country"), since = opt("since"), limit = Number(opt("limit") ?? 20);
const valued = new Set(["--gap", "--country", "--since", "--limit"].flatMap((f) => { const i = args.indexOf(f); return i >= 0 ? [i, i + 1] : []; }));
const slugs = args.filter((a, i) => !a.startsWith("--") && !valued.has(i));
if (gap && !GAPS[gap]) throw new Error(`unknown gap ${gap}; one of: ${Object.keys(GAPS).join(", ")}`);

const rows = [];
for (const path of globSync("reports/**/*.md")) {
  const slug = basename(path, ".md");
  if (slugs.length && !slugs.includes(slug)) continue;
  const text = readFileSync(path, "utf8");
  const end = text.indexOf("\n---", 4);
  let data;
  try { data = load(text.slice(4, end)); } catch { continue; }
  if (country && data.country !== country && !String(data.country).startsWith(`${country}/`)) continue;
  const date = new Date(data.date).toISOString().slice(0, 10);
  if (since && date < since) continue;
  rows.push({ slug, path, date, country: data.country, title: data.title, gaps: reportGaps(data) });
}
rows.sort((a, b) => b.date.localeCompare(a.date));

if (slugs.length) {
  if (json) console.log(JSON.stringify(rows, null, 2));
  else for (const r of rows) console.log(`${r.path}\n${r.gaps.map((g) => `  - ${g}: ${GAPS[g]}`).join("\n") || "  nothing to add"}\n`);
} else if (gap) {
  const list = rows.filter((r) => r.gaps.includes(gap)).slice(0, limit);
  if (json) console.log(JSON.stringify(list, null, 2));
  else for (const r of list) console.log(`${r.date}  ${r.country.padEnd(6)} ${r.slug}  (${r.gaps.length} gaps)`);
} else {
  const counts = Object.fromEntries(Object.keys(GAPS).map((g) => [g, rows.filter((r) => r.gaps.includes(g)).length]));
  if (json) console.log(JSON.stringify({ reports: rows.length, counts }, null, 2));
  else {
    console.log(`${rows.length} reports`);
    for (const [g, n] of Object.entries(counts).sort((a, b) => b[1] - a[1])) console.log(`  ${String(n).padStart(5)}  ${g.padEnd(18)} ${GAPS[g]}`);
  }
}
