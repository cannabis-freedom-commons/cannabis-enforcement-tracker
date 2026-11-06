// The agencies reports name that are not in the registry (src/data/agencies.json), most frequent
// first: candidates to add there. With --apply, a name that matches a registry agency (its name,
// or an alias, in the report's country) is rewritten to the agency's id, so every report uses ids.
//   node scripts/agency-names.mjs [--apply] [--min N]
import { readFileSync, writeFileSync, globSync } from "node:fs";
import { load, dump } from "js-yaml";
import agencies from "../src/data/agencies.json" with { type: "json" };

const apply = process.argv.includes("--apply");
const minAt = process.argv.indexOf("--min");
const min = minAt > 0 ? Number(process.argv[minAt + 1]) : 1;
const ids = new Set(agencies.map((a) => a.id));
const norm = (s) => s.toLowerCase().replace(/\s*\([^)]*\)\s*$/, "").replace(/[^\p{L}\p{N}]+/gu, " ").trim();
const inCountry = (a, country) => !a.country || a.country === country || country.startsWith(`${a.country}/`);
const match = (name, country) => agencies.find((a) => inCountry(a, country) && [a.name, ...a.aliases].some((n) => norm(n) === norm(name)));

const unknown = new Map();
let rewritten = 0;
for (const path of globSync("reports/**/*.md")) {
  const text = readFileSync(path, "utf8");
  const end = text.indexOf("\n---", 4);
  if (!text.startsWith("---\n") || end < 0 || !/^ {2}agencies:/m.test(text.slice(0, end))) continue;
  const data = load(text.slice(4, end));
  const list = data.details?.agencies ?? [];
  let changed = false;
  const next = list.map((name) => {
    if (ids.has(name)) return name;
    const found = match(name, data.country);
    if (found) { changed = true; return found.id; }
    const key = `${data.country}\t${name}`;
    unknown.set(key, (unknown.get(key) ?? 0) + 1);
    return name;
  });
  if (changed && apply) {
    const line = `  agencies: ${dump([...new Set(next)], { flowLevel: 0 }).trim()}`;
    const front = text.slice(0, end).replace(/^ {2}agencies:.*$/m, line);
    writeFileSync(path, front + text.slice(end));
    rewritten++;
  } else if (changed) rewritten++;
}
console.log(`${apply ? "Rewrote" : "Would rewrite"} agency names to ids in ${rewritten} reports.`);
const rows = [...unknown].filter(([, n]) => n >= min).sort((a, b) => b[1] - a[1]);
console.log(`${rows.length} names not in the registry${min > 1 ? ` (named at least ${min} times)` : ""}:`);
for (const [key, n] of rows.slice(0, 100)) { const [country, name] = key.split("\t"); console.log(`  ${String(n).padStart(4)}  ${country}  ${name}`); }
