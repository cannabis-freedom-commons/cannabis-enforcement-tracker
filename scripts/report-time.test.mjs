import test from "node:test";
import assert from "node:assert/strict";
import { reportDay, compareReports, addedAtLabel, addedAtTitle } from "../src/lib/report-time.js";
import { reportTimeline } from "../src/scripts/report-timeline.js";

const rows = [
  { i: "early", d: "2026-11-03", at: "2026-11-03T08:00:00Z", ti: "A title", t: "raid" },
  { i: "late", d: "2026-11-03", at: "2026-11-03T12:00:00Z", ti: "Z title", t: "arrest" },
  { i: "archive", d: "2010-01-01", at: "2026-11-03T13:00:00Z", t: "sentence" },
  { i: "unknown", d: "2026-11-03", t: "seizure" },
];
test("latest incidents use first-added time within the day, independent of titles", () => {
  assert.deepEqual([...rows].sort(compareReports).map(r => r.i), ["late", "early", "unknown", "archive"]);
  assert.equal(compareReports({ id: "a", date: new Date("2026-11-03"), added_at: rows[0].at }, { id: "b", date: new Date("2026-11-03"), added_at: rows[1].at }) > 0, true);
});
test("listing order and timelines use incident dates, including recently added historical cases", () => {
  const series = reportTimeline(rows);
  assert.equal(series[0].key, "2010-01");
  assert.equal(series.at(-1).key, "2026-11");
  assert.equal(series.at(-1).total, 3);
  assert.equal(reportDay(rows[2]), "2010-01-01");
});
test("addition and update labels stay relative, with exact UTC timestamps on hover", () => {
  const now = Date.parse("2026-11-03T12:05:00Z");
  assert.equal(addedAtLabel(rows[1].at, now), "Added 5 minutes ago");
  assert.equal(addedAtLabel(rows[0].at, now), "Added 4 hours ago");
  assert.equal(addedAtLabel("2026-11-03T12:04:00Z", now, "Updated"), "Updated 1 minute ago");
  assert.equal(addedAtLabel("2026-11-01T12:05:00Z", now), "Added 2 days ago");
  assert.match(addedAtTitle(rows[1].at), /12:00:00 UTC$/);
  assert.doesNotMatch(addedAtTitle(rows[1].at), /estimated/i);
});
