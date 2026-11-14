// Every report against the schema the build uses (src/lib/report-schema.ts), read with Astro's own
// front matter parser: a report the build would refuse ("summary: Too big") fails npm test first.
import { test } from "node:test";
import assert from "node:assert/strict";
import fs from "node:fs";
import path from "node:path";
import { z } from "astro/zod";
import { parseFrontmatter } from "@astrojs/internal-helpers/frontmatter";
import { reportSchema } from "../src/lib/report-schema.ts";

const ROOT = process.env.REPORTS_DIR || "reports";
const files = fs.readdirSync(ROOT, { recursive: true }).filter((f) => f.endsWith(".md")).map((f) => path.join(ROOT, f));

// Astro's image() resolves the cover next to the report file; here it must at least exist there.
const schemaFor = (file) => reportSchema({
  image: () => z.string().min(1).refine((src) => /^https?:\/\//.test(src) || fs.existsSync(path.resolve(path.dirname(file), src)), { message: "the cover image file does not exist" }),
});

test("the schema reads reports", () => assert.ok(files.length > 0));

test("every report matches the content schema", () => {
  const problems = [];
  for (const file of files) {
    const { frontmatter } = parseFrontmatter(fs.readFileSync(file, "utf8"), { frontmatter: "empty-with-spaces" });
    const result = schemaFor(file).safeParse(frontmatter);
    if (!result.success) {
      for (const issue of result.error.issues) problems.push(`${file}: ${issue.path.join(".") || "(report)"}: ${issue.message}`);
    }
  }
  assert.deepEqual(problems, [], "these reports would stop the build (src/lib/report-schema.ts)");
});
