// Reclassify unchanged backend reports from saved service validation, without AI or network calls.
// Dry run: node scripts/recheck-report-status.mjs backend/data/tracker.sqlite
// Apply:   node scripts/recheck-report-status.mjs backend/data/tracker.sqlite --write
import fs from "node:fs";
import path from "node:path";
import { DatabaseSync } from "node:sqlite";
import { recheckReportStatus } from "./lib/report-verification.mjs";

const args = process.argv.slice(2), write = args.includes("--write");
const database = args.filter((arg) => arg !== "--write");
if (database.length !== 1) throw new Error("Usage: node scripts/recheck-report-status.mjs <database> [--write]");
const db = new DatabaseSync(database[0], { readOnly: true });
let records;
try {
  records = db.prepare("SELECT report_slug,ai_result,validation,draft_markdown FROM cases WHERE status IN ('committed','published') AND report_slug<>'' AND draft_markdown<>''").all();
} finally { db.close(); }
const bySlug = new Map();
for (const file of fs.readdirSync("reports", { recursive: true }).filter((f) => f.endsWith(".md"))) {
  const slug = path.basename(file, ".md");
  if (bySlug.has(slug)) throw new Error(`duplicate report slug: ${slug}`);
  bySlug.set(slug, path.join("reports", file));
}
const proposed = new Map(), skipped = new Map();
for (const record of records) {
  const file = bySlug.get(record.report_slug);
  if (!file) { skipped.set("report missing", (skipped.get("report missing") ?? 0) + 1); continue; }
  const original = fs.readFileSync(file, "utf8");
  const result = recheckReportStatus(original, record);
  if (!result.eligible) { skipped.set(result.reason, (skipped.get(result.reason) ?? 0) + 1); continue; }
  proposed.set(file, { original, content: result.content });
}
for (const [file, { original, content }] of proposed) {
  if (write) {
    if (fs.readFileSync(file, "utf8") !== original) throw new Error(`report changed during recheck: ${file}`);
    fs.writeFileSync(file, content);
  }
  console.log(`${write ? "Verified" : "Eligible"}: ${file}`);
}
console.log(JSON.stringify({ mode: write ? "write" : "dry-run", checked_records: records.length, eligible_reports: proposed.size, skipped: Object.fromEntries(skipped) }));
