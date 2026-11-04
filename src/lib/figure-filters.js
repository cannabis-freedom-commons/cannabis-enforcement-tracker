// Filters by a report's figures (people arrested, plants destroyed…), its check status and whether
// it has a photo: the "+ Filter" menu of the filter bar (FilterBar.astro, ReportBrowser.astro). The
// numbers come from each report's row of the shared index (`n`, built by figureNumbers), and a
// filter is kept in the address as ?plants=1000-5000 (either end may be open), ?status=verified or
// ?photo=1. A report that does not state a figure is left out while that figure is filtered.
import rates from "../data/exchange-rates.json" with { type: "json" };
import { prisonYears } from "./stats.js";

export const FIGURE_FILTERS = [
  { key: "arrested", field: "ar", label: "People arrested", short: "Arrested" },
  { key: "charged", field: "ch", label: "People charged", short: "Charged" },
  { key: "plants", field: "pl", label: "Plants destroyed", short: "Plants" },
  { key: "hectares", field: "ha", label: "Area destroyed (hectares)", short: "Area", unit: " ha" },
  { key: "kg", field: "kg", label: "Cannabis seized (kg)", short: "Seized", unit: " kg" },
  { key: "deaths", field: "de", label: "Deaths", short: "Deaths" },
  { key: "prison", field: "py", label: "Prison term (years)", short: "Prison", unit: " years", note: "Life and death sentences count as longer than any term." },
  { key: "fine", field: "fu", label: "Fine (in US dollars)", short: "Fine", prefix: "US$", note: `Converted at the exchange rates of ${rates.date}.` },
  { key: "value", field: "vu", label: "Value of the seizure (in US dollars)", short: "Value", prefix: "US$", note: `Converted at the exchange rates of ${rates.date}.` },
];
export const STATUSES = [["verified", "Verified"], ["reported", "Reported"], ["disputed", "Disputed"]];

// A report's figures as plain numbers for its index row: short keys, only those it states.
export function figureNumbers(r) {
  const f = r.figures ?? {}, n = {};
  const usd = (m) => (m && rates.rates[m.currency] ? Math.round(m.amount / rates.rates[m.currency]) : undefined);
  const prison = f.prison ? prisonYears(f.prison) : null;
  const values = {
    ar: f.arrested, ch: f.charged, pl: f.plants_destroyed, ha: f.hectares, kg: f.kg_seized,
    de: f.deaths ?? (r.type === "execution" ? 1 : undefined),
    py: prison === "life" ? "L" : prison === "death" ? "D" : typeof prison === "number" ? Math.round(prison * 100) / 100 : undefined,
    fu: usd(f.fine), vu: usd(f.value),
  };
  for (const [k, v] of Object.entries(values)) if (v != null) n[k] = v;
  return n;
}

// "1000-5000", "1000-", "-5000" ↔ [min, max] (null for an open end); null when neither end is set
export function parseRange(value) {
  const m = /^(\d+(?:\.\d+)?)?-(\d+(?:\.\d+)?)?$/.exec(String(value ?? "").replace(/,/g, ""));
  if (!m || (m[1] == null && m[2] == null)) return null;
  const range = [m[1] == null ? null : Number(m[1]), m[2] == null ? null : Number(m[2])];
  if (range[0] != null && range[1] != null && range[0] > range[1]) range.reverse();
  return range;
}
export const formatRange = ([min, max]) => `${min ?? ""}-${max ?? ""}`;

const num = (v) => v.toLocaleString("en-GB", { maximumFractionDigits: 2 });
// how a filter reads on its chip: "Plants 1,000–5,000", "Arrested ≥ 5", "Fine ≤ US$1,000"
export function rangeLabel(filter, [min, max]) {
  const v = (x) => `${filter.prefix ?? ""}${num(x)}${filter.unit ?? ""}`;
  if (min != null && max != null) return `${filter.short} ${filter.prefix ?? ""}${num(min)}–${num(max)}${filter.unit ?? ""}`;
  return min != null ? `${filter.short} ≥ ${v(min)}` : `${filter.short} ≤ ${v(max)}`;
}

// whether a report's figure lies in the range; life and death sentences are longer than any term
export function inRange(row, filter, [min, max]) {
  const value = row.n?.[filter.field];
  if (value == null) return false;
  const x = value === "L" || value === "D" ? Infinity : value;
  return (min == null || x >= min) && (max == null || x <= max);
}
