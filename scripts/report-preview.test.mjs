import { test } from "node:test";
import assert from "node:assert/strict";
import { previewMedia, savedReportPath } from "../src/lib/report-preview.js";
import reportAliases from "../src/lib/report-aliases.js";

const selected = { kind: "photo", url: "https://news.example/article", media_url: "https://news.example/selected.jpg" };
const stale = { ...selected, media_url: "https://news.example/stale.jpg" };

test("saved previews resolve the report on the site for published and duplicate submissions", () => {
  for (const status of ["published", "duplicate"]) {
    assert.equal(savedReportPath({ status, report_slug: "saved-report" }, "/tracker/"), "/tracker/report/saved-report/");
  }
  // committed and deploying reports are not on the site yet: their draft is shown
  for (const status of ["drafted", "publishing", "committed", "deploying", "failed"]) assert.equal(savedReportPath({ status, report_slug: "saved-report" }), null);
  for (const slug of ["../secret", "https://elsewhere.example", "", "saved/report"]) assert.equal(savedReportPath({ status: "published", report_slug: slug }), null);
});

test("saved submission links follow every consolidated report to its surviving URL", () => {
  for (const [retired, canonical] of Object.entries(reportAliases.slugs)) {
    for (const status of ["published", "duplicate"]) {
      assert.equal(savedReportPath({ status, report_slug: retired }, "/tracker/"), `/tracker/report/${canonical}/`);
    }
  }
});

test("the prepared file's media selection overrides proposed and captured photos", () => {
  const v = { result: { media: [stale] }, source_check: { images: [stale] }, draft_markdown: `---\nmedia:\n  - kind: photo\n    url: ${selected.url}\n    media_url: ${selected.media_url}\ncover: ../../assets/reports/cover.jpg\n---\nReport.` };
  assert.deepEqual(previewMedia(v), { media: [selected], cover: "../../assets/reports/cover.jpg" });
  for (const draft_markdown of ["---\nmedia: []\n---\nReport.", "---\ntitle: No media\n---\nReport.", "---\nmedia: [invalid\n---\nReport."]) {
    assert.deepEqual(previewMedia({ ...v, draft_markdown }), { media: [], cover: null });
  }
  assert.deepEqual(previewMedia({ ...v, draft_markdown: `---\n${JSON.stringify({ media: [selected] })}\n---\nReport.` }), { media: [selected], cover: null });
});

test("before preparation, only explicitly selected result media is previewed", () => {
  assert.deepEqual(previewMedia({ result: { media: [selected] }, source_check: { images: [stale] } }), { media: [selected], cover: null });
  assert.deepEqual(previewMedia({ source_check: { images: [stale] } }), { media: [], cover: null });
});
