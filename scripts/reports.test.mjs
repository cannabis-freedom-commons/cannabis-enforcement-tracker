// Checks every report file before a build (npm test): rules the schema in src/content.config.ts
// cannot see, such as the file's place in reports/<year>/<month>/, unique slugs and citations.
import { test } from "node:test";
import assert from "node:assert/strict";
import fs from "node:fs";
import path from "node:path";
import * as yaml from "js-yaml";

const ROOT = "reports";
const countries = new Set(JSON.parse(fs.readFileSync("src/data/countries.json", "utf8")).map((c) => c.id));
const files = fs.readdirSync(ROOT, { recursive: true }).filter((f) => f.endsWith(".md")).map((f) => path.join(ROOT, f));
const reports = files.map((file) => {
  const text = fs.readFileSync(file, "utf8");
  const m = text.match(/^---\n([\s\S]*?)\n---\n([\s\S]*)$/);
  assert.ok(m, `${file}: no front matter`);
  return { file, data: yaml.load(m[1], { schema: yaml.JSON_SCHEMA }), body: m[2] };
});

test("there are reports", () => assert.ok(reports.length > 0));

for (const { file, data, body } of reports) {
  test(file, () => {
    const slug = path.basename(file, ".md");
    assert.ok(typeof data.created_by_model === "string" && data.created_by_model.trim().length > 0 && data.created_by_model.length <= 300, "created_by_model is required: record the actual model, or unknown for unrecorded historical attribution");
    assert.match(slug, /^[a-z0-9]+(-[a-z0-9]+)*$/, "the file name is the slug: lower case, digits and hyphens");
    const [, year, month] = file.split(path.sep);
    assert.equal(`${year}-${month}`, String(data.date).slice(0, 7), "the file sits in reports/<year>/<month>/ of its date");
    assert.ok(countries.has(data.country), `"${data.country}" is not a country id of the Cannabis Freedom Index (src/data/countries.json)`);
    assert.ok(Array.isArray(data.sources) && data.sources.length > 0, "a report needs sources");
    for (const s of data.sources) assert.ok(/^https?:\/\//.test(s.url ?? "") || s.type, `source "${s.title}" needs a URL, or a type (book, journal, report, law) with its details`);
    assert.equal(typeof data.code, "string", "code is required: run npm run codes -- <report paths> before building");
    assert.match(data.code, /^[0-9A-HJKMNP-TV-Z]{6}$/, "the code is six characters of Crockford base32 (npm run codes writes it)");
    const cited = new Set([...body.matchAll(/\[@([^\]]+)\]/g)].flatMap((m) => m[1].split(",").map((x) => Number(x.trim()))));
    for (const n of cited) assert.ok(n >= 1 && n <= data.sources.length, `[@${n}] names no source`);
    for (let i = 1; i <= data.sources.length; i++) assert.ok(cited.has(i), `source ${i} is never cited ([@${i}])`);
    // a report moved from the CFI history may keep that entry's short description
    assert.ok(body.trim().length > (data.cfi_id ? 60 : 200), "the text is too short");
  });
}

test("codes are unique", () => {
  const seen = new Map();
  for (const { file, data } of reports) {
    assert.ok(!seen.has(data.code), `${file} and ${seen.get(data.code)} share the code ${data.code}`);
    seen.set(data.code, file);
  }
});

test("slugs are unique", () => {
  const seen = new Map();
  for (const { file } of reports) {
    const slug = path.basename(file, ".md");
    assert.ok(!seen.has(slug), `${file} and ${seen.get(slug)} share the slug "${slug}"`);
    seen.set(slug, file);
  }
});
