// The law where and when a report's event happened, from the Cannabis Freedom Index: the CFI score
// in that year (the CFI's own year-by-year series), the law changes just before the event and the
// reforms since. Worked out at build time from dated CFI data (scripts/sync-countries.mjs,
// scripts/sync-cfi-changes.mjs), never stored in reports, so a later change in the law never
// rewrites what the law was at the time.
import data from "../data/cfi-changes.json" with { type: "json" };
import { cfiOf, tierOf, CFI } from "./cfi.js";
import { countryOf } from "./countries.js";
import { scoreIn } from "./legal-context-core.js";
export { scoreIn };

const byPlace = new Map();
for (const [date, place, category, title, scored, id] of data.changes) {
  (byPlace.get(place) ?? byPlace.set(place, []).get(place)).push({ date, place, category, title, scored: scored === 1, id, url: `${CFI}/history/${id}/` });
}

const DAY = 86400000;
// the CFI change kinds that are changes in the law or its enforcement (not debate, culture,
// international treaties or other events)
const LAW = new Set(["easing", "tightening", "legalisation", "ban", "death", "medical", "hemp", "enforcement"]);
// the kinds that change what the law allows or punishes
const REFORM = new Set(["easing", "tightening", "legalisation", "ban", "death"]);
export function legalContext(country, date, { before = 3, after = 3 } = {}) {
  const iso = date.toISOString().slice(0, 10);
  const year = date.getUTCFullYear();
  const parent = countryOf(country)?.parent;
  const changes = [...(byPlace.get(country) ?? []), ...(parent ? byPlace.get(parent) ?? [] : [])]
    .filter((c) => LAW.has(c.category)).sort((a, b) => a.date.localeCompare(b.date));
  // a change dated only by year or month counts as before an event in that year or month
  const earlier = changes.filter((c) => c.date <= iso || iso.startsWith(c.date));
  const later = changes.filter((c) => !earlier.includes(c));
  const cfi = cfiOf(country) ?? (parent ? cfiOf(parent) : null);
  const trend = cfi?.trend ?? (parent ? cfiOf(parent)?.trend : null);
  const then = scoreIn(trend, year);
  // a scored law change in the year before the event: the law was new
  const recent = [...earlier].reverse().find((c) => c.scored && REFORM.has(c.category) && c.date.length === 10 && date - new Date(`${c.date}T00:00:00Z`) <= 365 * DAY);
  return {
    year,
    scoreThen: then,
    tierThen: then != null ? tierOf(then).name : null,
    scoreNow: cfi?.score ?? null,
    before: earlier.slice(-before).reverse(),
    after: later.filter((c) => c.scored).slice(0, after),
    recentReform: recent ?? null,
  };
}
