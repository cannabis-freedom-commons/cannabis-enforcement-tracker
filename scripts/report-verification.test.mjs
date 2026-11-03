import test from "node:test";
import assert from "node:assert/strict";
import { recheckReportStatus } from "./lib/report-verification.mjs";

const report = `---
code: ABC123
date: 2026-11-03
status: reported
created_by_model: original/model#high
sources:
  - url: https://news.example/story
---

Police arrested two people. [@1]
`;
const record = () => ({
  report_slug: "example",
  draft_markdown: report.replace("code: ABC123\n", ""),
  ai_result: JSON.stringify({ outcome: "new_event", proposed_slug: "example", sources: [{ url: "https://news.example/story" }], facts: [{ text: "Police arrested two people.", source: 1 }] }),
  validation: JSON.stringify({ passed: true, confirmed: 1, sources: [{ url: "https://news.example/story", opened: true, confirmed: 1 }] }),
});

test("saved complete checks promote only status, preserving code, attribution and text", () => {
  const got = recheckReportStatus(report, record());
  assert.equal(got.eligible, true);
  assert.equal(got.content, report.replace("status: reported", "status: verified"));
});

test("edits, disputes, incomplete checks and update-only evidence cannot promote a report", () => {
  for (const text of [report.replace("two people", "three people"), report.replace("status: reported", "status: disputed"), report.replace("original/model#high", "another/model")]) assert.equal(recheckReportStatus(text, record()).eligible, false);
  for (const change of [
    (r) => { r.validation = ""; },
    (r) => { r.draft_markdown = ""; },
    (r) => { const v = JSON.parse(r.validation); v.passed = false; r.validation = JSON.stringify(v); },
    (r) => { const v = JSON.parse(r.validation); v.confirmed = 0; r.validation = JSON.stringify(v); },
    (r) => { const v = JSON.parse(r.validation); v.sources[0].opened = false; r.validation = JSON.stringify(v); },
    (r) => { const v = JSON.parse(r.validation); v.sources[0].confirmed = 0; r.validation = JSON.stringify(v); },
    (r) => { const v = JSON.parse(r.ai_result); v.outcome = "existing_case"; r.ai_result = JSON.stringify(v); },
    (r) => { const v = JSON.parse(r.ai_result); v.facts.push({ text: "Another claim.", source: 1 }); r.ai_result = JSON.stringify(v); },
    (r) => { r.report_slug = "other"; },
    (r) => { const v = JSON.parse(r.ai_result); v.facts[0].text = "Another checked claim."; r.ai_result = JSON.stringify(v); },
    (r) => { r.draft_markdown = report.replace("ABC123", "XYZ789"); },
  ]) { const r = record(); change(r); assert.equal(recheckReportStatus(report, r).eligible, false); }
});

test("a dropped unsupported claim does not block verification of checked retained text", () => {
  const r = record(), check = JSON.parse(r.validation);
  Object.assign(check, { facts: 2, checked: 2, left_out: ["An unsupported claim."] });
  r.validation = JSON.stringify(check);
  assert.equal(recheckReportStatus(report, r).eligible, true);
});
