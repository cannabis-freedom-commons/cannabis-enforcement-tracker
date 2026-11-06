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
  // a smuggling route: each end its own filter (?from=ma&to=es)
  const route = detailValues({ route: { from: ["ma"], to: ["es", "fr"] } });
  assert.deepEqual(route, { rf: ["ma"], rt: ["es", "fr"] });
  const [from, to] = ["from", "to"].map((k) => DETAIL_FILTERS.find((f) => f.key === k));
  assert.ok(hasDetail({ x: route }, from, "ma") && hasDetail({ x: route }, to, "fr") && !hasDetail({ x: route }, to, "ma"));
  assert.ok(from.options.some(([id]) => id === "ma"));
  // a life or death sentence the figures state is a sentence type too
  assert.deepEqual(detailValues(undefined, { prison: "life" }), { st: ["life"] });
  assert.deepEqual(detailValues({ sentence_types: ["death"] }, { prison: "death" }), { st: ["death"] });
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

import { reportGaps } from "./lib/report-gaps.mjs";
test("the gap audit names what today's rules ask for", () => {
  const old = { status: "reported", figures: { arrested: 2 }, sources: [{ url: "https://a.example", title: "A", publisher: "A" }] };
  assert.deepEqual(reportGaps(old), ["no-coordinates", "single-source", "unarchived-source", "undated-source", "no-media", "no-details", "unsourced-figure", "not-verified", "no-open-questions", "no-tags"]);
  const done = {
    status: "verified", coordinates: [1, 2], precision: "locality", tags: ["x"], open_questions: ["q"],
    figures: { arrested: 2 }, details: { setting: ["farm"], region: "Kano" }, data_sources: { "figures.arrested": [1], "details.setting": [2] },
    media: [{ url: "https://a.example", media_url: "https://a.example/p.jpg", archived: "https://web.archive.org/x" }],
    sources: [{ url: "https://a.example", archived: "x", date: new Date() }, { url: "https://b.example", archived: "y", date: new Date() }],
  };
  assert.deepEqual(reportGaps(done), []);
});
