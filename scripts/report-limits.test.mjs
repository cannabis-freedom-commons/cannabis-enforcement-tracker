// Reports stay readable as sources are added (docs/WRITING.md §2 and §9): at most 12 open
// questions (the backend enforces it; the build fails past 20, a few over 12 are listed to rewrite), and a text of about 900 words at most (reported
// here as a list to rewrite, not a failure: an automatic update cannot shorten a text). A report grows with
// the case, not with each source; the backend refuses updates that would pile up questions
// (backend/internal/research/updates.go). Reports that grew before that are listed in
// report-limits-allow.json until they are rewritten; the list may only shrink, so a report that is
// fixed must leave it (node scripts/report-limits.test.mjs --update rewrites it).
import { test } from "node:test";
import assert from "node:assert/strict";
import fs from "node:fs";
import path from "node:path";
import * as yaml from "js-yaml";

export const MAX_QUESTIONS = 12;
// The build fails only past this ceiling: a report a few over 12 (an older backend still running,
// a service note) is listed below to rewrite, and must never stop a deployment.
export const HARD_MAX_QUESTIONS = 20;
export const MAX_WORDS = 900;
const ALLOW = "scripts/report-limits-allow.json";

const ROOT = "reports";
const files = fs.readdirSync(ROOT, { recursive: true }).filter((f) => f.endsWith(".md")).map((f) => path.join(ROOT, f));
const over = [], long = [], notText = [];
for (const file of files) {
  const m = fs.readFileSync(file, "utf8").match(/^---\n([\s\S]*?)\n---\n([\s\S]*)$/);
  if (!m) continue;
  const data = yaml.load(m[1], { schema: yaml.JSON_SCHEMA });
  const questions = data.open_questions?.length ?? 0;
  // a question with ": " in it, unquoted, is read as a key and value and breaks the site build
  if ((data.open_questions ?? []).some((q) => typeof q !== "string")) notText.push(path.basename(file, ".md"));
  const words = m[2].replace(/\[@[^\]]*\]/g, "").split(/\s+/).filter(Boolean).length;
  if (questions > MAX_QUESTIONS) over.push({ slug: path.basename(file, ".md"), questions, words });
  else if (words > MAX_WORDS) long.push({ slug: path.basename(file, ".md"), words });
}
over.sort((a, b) => a.slug.localeCompare(b.slug));

if (process.argv.includes("--update")) {
  fs.writeFileSync(ALLOW, JSON.stringify(over.map((o) => o.slug), null, 1) + "\n");
  console.log(`${ALLOW}: ${over.length} reports over the limits`);
}
const allowed = new Set(JSON.parse(fs.readFileSync(ALLOW, "utf8")));

test("every open question is plain text", () => {
  assert.deepEqual(notText, [], "quote these open questions: one with \": \" in it is read as a key and value");
});

test(`no report has more than ${HARD_MAX_QUESTIONS} open questions, outside the list being rewritten`, () => {
  const fresh = over.filter((o) => !allowed.has(o.slug) && o.questions > HARD_MAX_QUESTIONS);
  assert.deepEqual(fresh.map((o) => `${o.slug}: ${o.questions} open questions`), [],
    "rewrite these reports once, every fact and source kept, the open questions down to the real gaps (docs/WRITING.md)");
});

test(`reports over ${MAX_QUESTIONS} open questions, to rewrite`, (t) => {
  for (const o of over.filter((o) => !allowed.has(o.slug) && o.questions <= HARD_MAX_QUESTIONS)) t.diagnostic(`${o.slug}: ${o.questions} open questions`);
});

// Not a failure: the backend fixes listed reports by itself when sources are added (it then
// sends the revised open questions), and that must not stop a deployment.
test("reports on the list that are within the limits now", (t) => {
  const fixed = [...allowed].filter((slug) => !over.some((o) => o.slug === slug));
  if (fixed.length) t.diagnostic(`${fixed.length} can leave ${ALLOW}: node scripts/report-limits.test.mjs --update`);
});

test(`reports over ${MAX_WORDS} words, to rewrite`, (t) => {
  for (const o of long.sort((a, b) => b.words - a.words)) t.diagnostic(`${o.slug}: ${o.words} words`);
});
