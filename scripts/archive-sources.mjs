// Gives every source and media page of the reports an archived copy in the Wayback Machine, and
// writes its address into the report as `archived:`, so the page can still be read if it changes or
// disappears.
//
//   node scripts/archive-sources.mjs              look up existing snapshots and fill them in
//   node scripts/archive-sources.mjs --save       also ask the Wayback Machine to save pages it lacks
//   node scripts/archive-sources.mjs <slug> …     only these reports (with or without --save)
//   node scripts/archive-sources.mjs --dry        show what would change, write nothing
import fs from "node:fs";
import path from "node:path";
import * as yaml from "js-yaml";

const args = process.argv.slice(2);
const SAVE = args.includes("--save");
const DRY = args.includes("--dry");
const only = args.filter((a) => !a.startsWith("--"));
const UA = "CannabisEnforcementTracker/0.1 (+https://tracker.cannabisfreedom.fyi)";
const sleep = (ms) => new Promise((r) => setTimeout(r, ms));

// the newest snapshot of a page, or null
async function snapshot(url) {
  const res = await fetch(`https://archive.org/wayback/available?url=${encodeURIComponent(url)}`, { headers: { "User-Agent": UA } });
  if (!res.ok) return null;
  const s = (await res.json()).archived_snapshots?.closest;
  return s?.available && s.status?.startsWith("2") ? s.url.replace(/^http:/, "https:") : null;
}

// ask the Wayback Machine to save a page; the snapshot's address, or null
async function save(url) {
  const res = await fetch(`https://web.archive.org/save/${url}`, { headers: { "User-Agent": UA }, redirect: "manual" });
  const loc = res.headers.get("location") ?? res.headers.get("content-location");
  if (loc) return new URL(loc, "https://web.archive.org").href;
  if (res.ok) { await sleep(5000); return snapshot(url); }
  console.log(`  save refused (${res.status}): ${url}`);
  return null;
}

// adds `archived: <snapshot>` after the `url:` line of each list item with that URL that has none
// yet, keeping the rest of the file as written
function addArchived(text, url, archived) {
  const lines = text.split("\n");
  for (let i = lines.length - 1; i >= 0; i--) {
    if (!/^\s*-?\s*url:/.test(lines[i]) || lines[i].replace(/^\s*-?\s*url:\s*/, "").trim() !== url) continue;
    const indent = lines[i].match(/^\s*/)[0] + (lines[i].trimStart().startsWith("-") ? "  " : "");
    let has = false;
    for (let j = i + 1; j < lines.length && lines[j].startsWith(indent) && !lines[j].trimStart().startsWith("-"); j++) {
      if (lines[j].trimStart().startsWith("archived:")) has = true;
    }
    if (!has) lines.splice(i + 1, 0, `${indent}archived: ${archived}`);
  }
  return lines.join("\n");
}

const files = fs.readdirSync("reports", { recursive: true }).filter((f) => f.endsWith(".md")).map((f) => path.join("reports", f));
let added = 0, missing = 0;
for (const file of files) {
  const slug = path.basename(file, ".md");
  if (only.length && !only.includes(slug)) continue;
  let text = fs.readFileSync(file, "utf8");
  const data = yaml.load(text.match(/^---\n([\s\S]*?)\n---/)[1], { schema: yaml.JSON_SCHEMA });
  const urls = [...new Set([...data.sources, ...(data.media ?? [])].filter((s) => !s.archived).map((s) => s.url))];
  for (const url of urls) {
    let archived = await snapshot(url);
    if (!archived && SAVE) { archived = await save(url); await sleep(4000); }
    if (archived) {
      console.log(`${slug}: ${url}\n  → ${archived}`);
      text = addArchived(text, url, archived);
      added++;
    } else {
      console.log(`${slug}: no snapshot yet: ${url}`);
      missing++;
    }
  }
  if (!DRY) fs.writeFileSync(file, text);
}
console.log(`\n${added} archived copies added, ${missing} pages without one${missing && !SAVE ? " (run with --save to create them)" : ""}`);
