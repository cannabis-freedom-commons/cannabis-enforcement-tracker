// Builds the site from N synthetic reports, to see that pages, the index and the build time stay
// reasonable as the tracker grows. Nothing here touches reports/: the fake reports go to
// .cache/load-test/reports and the site to .cache/load-test/dist (both ignored by git).
//
//   node scripts/load-test.mjs [count]     default 5000
import fs from "node:fs";
import path from "node:path";
import { execSync } from "node:child_process";
import { codeOf } from "../src/lib/code.js";

const N = Number(process.argv[2] || 5000);
const root = ".cache/load-test", dir = path.join(root, "reports"), out = path.join(root, "dist");
fs.rmSync(root, { recursive: true, force: true });
const countries = JSON.parse(fs.readFileSync("src/data/countries.json", "utf8")).filter((c) => c.iso_numeric != null || c.parent);
const types = ["eradication", "eradication", "seizure", "seizure", "raid", "arrest", "arrest", "prosecution", "sentence", "death"];
let seed = 42;
const rnd = () => ((seed = (seed * 1103515245 + 12345) % 2 ** 31) / 2 ** 31);
const pick = (a) => a[Math.floor(rnd() * a.length)];
const start = Date.UTC(2016, 0, 1), span = Date.UTC(2026, 9, 1) - start;
for (let i = 0; i < N; i++) {
  const date = new Date(start + rnd() * span).toISOString().slice(0, 10);
  const c = pick(countries), type = pick(types);
  const d = path.join(dir, date.slice(0, 4), date.slice(5, 7));
  fs.mkdirSync(d, { recursive: true });
  const coords = rnd() < 0.7 ? `coordinates: [${(rnd() * 120 - 50).toFixed(2)}, ${(rnd() * 340 - 170).toFixed(2)}]\nprecision: district\n` : "";
  fs.writeFileSync(path.join(d, `test-${i}.md`), `---
code: "${codeOf(`test-${i}`)}"
date: ${date}
added_at: "${new Date(Date.UTC(2026, 9, 1) + i * 1000).toISOString()}"
country: ${c.id}
place: Test place ${i}
${coords}type: ${type}
title: Synthetic ${type} report number ${i} in ${c.name}
summary: A synthetic report for load testing, about the length of a real two-sentence summary of an event and its consequences for people.
figures:
  arrested: ${Math.floor(rnd() * 20)}
  hectares: ${(rnd() * 50).toFixed(1)}
status: reported
created_by_model: unknown
sources:
  - title: Test source ${i}
    url: https://example.org/${i}
    publisher: Example
---

${"A paragraph of synthetic text that stands in for a real report, cited to its source. [@1] ".repeat(6)}
`);
}
console.log(`${N} synthetic reports in ${dir}`);
const t0 = Date.now();
execSync(`npx astro build --outDir ${out}`, { stdio: "inherit", env: { ...process.env, REPORTS_DIR: dir } });
const secs = ((Date.now() - t0) / 1000).toFixed(1);
const size = (f) => fs.existsSync(f) ? `${(fs.statSync(f).size / 1024).toFixed(0)} KB` : "–";
const count = (d) => fs.readdirSync(d, { recursive: true }).filter((f) => f.endsWith(".html")).length;
console.log(`\nbuilt in ${secs} s: ${count(out)} pages`);
for (const f of ["index.html", "api/index.json", "api/reports.json", "api/latest.json", "stats/index.html", "country/in/index.html", "report/test-1/index.html", "2026/index.html"]) console.log(`  ${f.padEnd(28)} ${size(path.join(out, f))}`);
