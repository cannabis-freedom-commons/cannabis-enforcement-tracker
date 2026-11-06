import { test } from "node:test";
import assert from "node:assert/strict";
import { detailValues, hasDetail, DETAIL_FILTERS, figureNumbers } from "../src/lib/figure-filters.js";
import { backfillDetails, citedSentences, detailsYAML, insertDetails } from "./lib/backfill-details.mjs";
import { scoreIn } from "../src/lib/legal-context-core.js";

test("index rows carry registry agencies and listed terms only", () => {
  const x = detailValues({ agencies: ["ng-ndlea", "Kano State Police Command"], products: [{ product: "hashish", kg: 2 }, { product: "hashish" }], setting: ["field"], case_stage: "charged" });
  assert.deepEqual(x, { ag: ["ng-ndlea"], pd: ["hashish"], se: ["field"], cs: ["charged"] });
  const agency = DETAIL_FILTERS.find((f) => f.key === "agency");
  assert.ok(hasDetail({ x }, agency, "ng-ndlea"));
  assert.ok(!hasDetail({ x: {} }, agency, "ng-ndlea"));
  assert.deepEqual(figureNumbers({ figures: { convicted: 2, injured: 1 } }), { cv: 2, in: 1 });
});

test("the backfill adds only values a cited sentence bears out", () => {
  const body = "Officers of the NDLEA seized 40 kg of hashish at the airport. [@1]\n\nThe suspect was charged. [@2]\n\nAn uncited sentence about a farm.";
  assert.deepEqual(citedSentences(body).map((s) => s.cites), [[1], [2]]);
  const found = backfillDetails({ country: "ng", tags: ["hashish", "airport", "farm", "indoor-grow"] }, body, 2);
  assert.deepEqual(found.details, { agencies: ["ng-ndlea"], agency_types: ["anti-narcotics"], setting: ["airport"], products: [{ product: "hashish" }] });
  assert.deepEqual(found.data_sources["details.agencies"], [1]);
  // an agency of another country is not matched, and a report with details is left alone
  assert.equal(backfillDetails({ country: "us", tags: [] }, "The NDLEA said so. [@1]", 1), null);
  assert.equal(backfillDetails({ country: "ng", tags: ["hashish"], details: {} }, body, 2), null);
  const text = "---\ncode: ABC123\ntitle: T\nstatus: verified\nsources: []\n---\n\nBody\n";
  const next = insertDetails(text, detailsYAML(found));
  assert.match(next, /title: T\ndetails:\n {2}agencies: \[ng-ndlea\][\s\S]*data_sources:\n[\s\S]*\nstatus: verified/);
  assert.equal(insertDetails(next, detailsYAML(found)), null);
});

test("the CFI score in a year follows the last change before it", () => {
  const trend = [[1935, 19], [1979, 5.5], [2022, 63.5], [2025, 26.5]];
  assert.equal(scoreIn(trend, 1990), 5.5);
  assert.equal(scoreIn(trend, 2022), 63.5);
  assert.equal(scoreIn(trend, 1900), null);
  assert.equal(scoreIn(null, 2000), null);
});
