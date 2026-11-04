// The figures and series the Statistics page draws, worked out at build time from the reports.
import { TYPES } from "./types.js";
import { weightParts } from "./figures.js";

const compactFormat = new Intl.NumberFormat("en-GB", { notation: "compact", maximumFractionDigits: 1 });
export const fmt = (n) => Math.round(n).toLocaleString("en-GB");
// a big number at a glance: in full up to 9,999, then 12.3K, 5.2M; the exact figure goes beside it
export const compact = (n) => (Math.abs(n) < 10000 ? fmt(n) : compactFormat.format(n).toUpperCase());

// the headline figures: total, how many reports state it, and the unit it is shown in
export function headlineFigures(list) {
  const stated = (k) => list.filter((r) => r.figures?.[k] != null);
  const sum = (k) => stated(k).reduce((s, r) => s + r.figures[k], 0);
  const kg = sum("kg_seized"), deaths = list.filter((r) => r.figures?.deaths != null || r.type === "execution");
  return [
    { id: "arrested", label: "People arrested", value: sum("arrested"), reports: stated("arrested").length },
    { id: "plants", label: "Plants destroyed", value: sum("plants_destroyed"), reports: stated("plants_destroyed").length },
    { id: "seized", label: "Cannabis seized", ...weightParts(kg), reports: stated("kg_seized").length },
    { id: "hectares", label: "Hectares destroyed", value: sum("hectares"), reports: stated("hectares").length },
    { id: "deaths", label: "Deaths", value: deaths.reduce((s, r) => s + (r.figures?.deaths ?? 1), 0), reports: deaths.length },
  ];
}

// reports per period with their types, for a column chart; every period between first and last
export function periodColumns(list, keyOf, periods) {
  const byKey = new Map(periods.map((p) => [p, { key: p, total: 0, types: {} }]));
  for (const r of list) {
    const c = byKey.get(keyOf(r));
    if (!c) continue;
    c.total++;
    c.types[r.type] = (c.types[r.type] ?? 0) + 1;
  }
  return [...byKey.values()];
}

// the tooltip of a column: its total and, in words, the types it is made of
export const typeRows = (types) => Object.keys(TYPES).filter((t) => types[t]).map((t) => [TYPES[t].color, TYPES[t].plural, types[t]]);

// the model that wrote a report, without its provider and reasoning setting:
// "spacexai/grok-4.7#medium" → "grok-4.7"; null when it was not recorded
export function modelName(id) {
  const v = String(id ?? "").trim();
  if (!v || v.startsWith("unknown")) return null;
  const slash = v.match(/^[\w.-]+\/(.+)$/);
  return (slash ? slash[1] : v).split("#")[0].trim();
}

// reports per model, most first, each with the exact identifiers (provider, setting) behind it
export function reportsByModel(list) {
  const models = new Map();
  let unrecorded = 0;
  for (const r of list) {
    const name = modelName(r.created_by_model);
    if (!name) { unrecorded++; continue; }
    const m = models.get(name) ?? { name, count: 0, variants: {} };
    m.count++;
    m.variants[r.created_by_model] = (m.variants[r.created_by_model] ?? 0) + 1;
    models.set(name, m);
  }
  const rows = [...models.values()].sort((a, b) => b.count - a.count || a.name.localeCompare(b.name));
  for (const m of rows) m.variants = Object.entries(m.variants).sort((a, b) => b[1] - a[1]);
  return { rows, unrecorded };
}

// the countries with the most of a figure: [{ id, value }], most first
export function topCountriesBy(list, figure, n = 8) {
  const sums = new Map();
  for (const r of list) {
    const v = r.figures?.[figure];
    if (v) sums.set(r.country, (sums.get(r.country) ?? 0) + v);
  }
  return [...sums].map(([id, value]) => ({ id, value })).sort((a, b) => b.value - a.value).slice(0, n);
}

// A big figure for a tile, short enough to fit any width: 25.6K, 71M, 8,605 t, 3.4 kg. A weight
// comes as { value, unit } (figures.js weightParts), shown to three significant digits under 100.
export function tileNumber({ value, unit }) {
  if (unit && value < 100) return value.toLocaleString("en-GB", { maximumSignificantDigits: 3 });
  return compact(value);
}

// A stated prison term in years, from the free text a report gives ("7 years rigorous imprisonment",
// "3 years and 6 months", "18 months"): "life", "death" (a death sentence), a number of years, or
// null when it cannot be read as one term (combined totals of several people, time served…). A
// range ("3 to 7 years") counts by its upper end, as written last.
const WORDS = { one: 1, two: 2, three: 3, four: 4, five: 5, six: 6, seven: 7, eight: 8, nine: 9, ten: 10, eleven: 11, twelve: 12, fifteen: 15, twenty: 20, thirty: 30 };
const UNIT = { year: 1, month: 1 / 12, week: 1 / 52, day: 1 / 365 };
export function prisonYears(text) {
  const s = String(text ?? "").toLowerCase();
  if (/\bdeath\b/.test(s)) return "death";
  if (/^\s*life\b/.test(s)) return "life";
  if (/combined/.test(s)) return null;
  const clause = s.split(/[;(]| for /)[0];
  if (/time served/.test(clause)) return null;
  const parts = [...clause.matchAll(/(\d+(?:[.,]\d+)?|[a-z]+)\s*(year|month|week|day)s?\b/g)];
  if (!parts.length) return null;
  // "3 years and 6 months": the units add up; "3 to 7 years": only the last term counts
  const terms = [];
  for (const [, n, unit] of parts) {
    const v = /\d/.test(n) ? Number(n.replace(",", ".")) : WORDS[n];
    if (v == null) return null;
    terms.push(v * UNIT[unit]);
  }
  const ranged = /\d\s*(?:to|–|-)\s*\d/.test(clause) || /\b(?:and|or)\b/.test(clause) && parts.every(([, , u]) => u === parts[0][2]);
  return ranged ? terms.at(-1) : terms.reduce((a, b) => a + b, 0);
}

// how many sentences fall in each band of length; `unread` are those no term could be read from
// [label, short label for an axis, from, below]
export const PRISON_BANDS = [["Under 1 year", "<1 yr", 0, 1], ["1–3 years", "1–3", 1, 3], ["3–5 years", "3–5", 3, 5], ["5–10 years", "5–10", 5, 10], ["10–20 years", "10–20", 10, 20], ["20 years or more", "20+", 20, Infinity]];
export function prisonBands(list) {
  const bands = PRISON_BANDS.map(([label, short]) => ({ label, short, count: 0 }));
  const extra = { life: { label: "Life", short: "Life", count: 0 }, death: { label: "Death sentence", short: "Death", count: 0 } };
  let unread = 0;
  const years = [];
  for (const r of list) {
    if (r.figures?.prison == null) continue;
    const y = prisonYears(r.figures.prison);
    if (y == null) { unread++; continue; }
    if (typeof y === "string") { extra[y].count++; continue; }
    years.push(y);
    bands[PRISON_BANDS.findIndex(([, , lo, hi]) => y >= lo && y < hi)].count++;
  }
  years.sort((a, b) => a - b);
  return { bands: [...bands, extra.life, extra.death], unread, median: years.length ? years[Math.floor((years.length - 1) / 2)] : null };
}

// how many seizures fall in each band of weight (kg), and the median seizure
export const SEIZURE_BANDS = [["Under 1 kg", "<1 kg", 0, 1], ["1–10 kg", "1–10 kg", 1, 10], ["10–100 kg", "10–100 kg", 10, 100], ["100 kg–1 t", "0.1–1 t", 100, 1000], ["1–10 t", "1–10 t", 1000, 10000], ["10 t or more", "10 t+", 10000, Infinity]];
export function seizureBands(list) {
  const kg = list.map((r) => r.figures?.kg_seized).filter((v) => v > 0).sort((a, b) => a - b);
  return {
    bands: SEIZURE_BANDS.map(([label, short, lo, hi]) => ({ label, short, count: kg.filter((v) => v >= lo && v < hi).length })),
    median: kg.length ? kg[Math.floor((kg.length - 1) / 2)] : null,
    total: kg.length,
  };
}

// each region's reports split by primary type, as shares of that region: [{ region, total, shares: { type: 0.4 } }]
export function typeMixByRegion(list, regionOf) {
  const rows = new Map();
  for (const r of list) {
    const k = regionOf(r);
    const row = rows.get(k) ?? { region: k, total: 0, counts: {} };
    row.total++;
    row.counts[r.type] = (row.counts[r.type] ?? 0) + 1;
    rows.set(k, row);
  }
  return [...rows.values()].sort((a, b) => b.total - a.total)
    .map((row) => ({ ...row, shares: Object.fromEntries(Object.entries(row.counts).map(([t, n]) => [t, n / row.total])) }));
}

// how many reports cite one source, two, three…: [{ label, short, count }]
export const SOURCE_COUNT_BANDS = [["1 source", "1", 1, 2], ["2 sources", "2", 2, 3], ["3 sources", "3", 3, 4], ["4 sources", "4", 4, 5], ["5 sources", "5", 5, 6], ["6–9 sources", "6–9", 6, 10], ["10 or more sources", "10+", 10, Infinity]];
export function sourceCountBands(list) {
  const n = list.map((r) => r.sources?.length ?? 0).filter((v) => v > 0).sort((a, b) => a - b);
  return {
    bands: SOURCE_COUNT_BANDS.map(([label, short, lo, hi]) => ({ label, short, count: n.filter((v) => v >= lo && v < hi).length })),
    mean: n.length ? n.reduce((a, b) => a + b, 0) / n.length : null,
  };
}

// reports by the day of the week of their date, Monday first
export const WEEKDAYS = ["Monday", "Tuesday", "Wednesday", "Thursday", "Friday", "Saturday", "Sunday"];
export function weekdayCounts(list) {
  const counts = WEEKDAYS.map((label) => ({ label, short: label.slice(0, 3), count: 0 }));
  for (const r of list) counts[(r.date.getUTCDay() + 6) % 7].count++;
  return counts;
}

// each type's reports by calendar month (January…December), as shares of that type's year, so a
// type's season shows whatever its size: [{ type, total, counts: [12], shares: [12] }], most first
export function seasonality(list) {
  const rows = new Map();
  for (const r of list) {
    const row = rows.get(r.type) ?? rows.set(r.type, { type: r.type, total: 0, counts: Array(12).fill(0) }).get(r.type);
    row.total++;
    row.counts[r.date.getUTCMonth()]++;
  }
  return [...rows.values()].sort((a, b) => b.total - a.total).map((row) => ({ ...row, shares: row.counts.map((n) => n / row.total) }));
}

// the most used tags with a size step 1–6 by how often they are used (on a log scale), in
// alphabetical order for a cloud: [{ tag, count, step }]
export function tagCloud(list, n = 90) {
  const counts = new Map();
  for (const r of list) for (const t of new Set(r.tags ?? [])) counts.set(t, (counts.get(t) ?? 0) + 1);
  const top = [...counts].filter(([, c]) => c > 1).sort((a, b) => b[1] - a[1] || a[0].localeCompare(b[0])).slice(0, n);
  if (!top.length) return [];
  const hi = Math.log(top[0][1]), lo = Math.log(top.at(-1)[1]);
  return top.map(([tag, count]) => ({ tag, count, step: 1 + Math.round(hi === lo ? 5 : ((Math.log(count) - lo) / (hi - lo)) * 5) }))
    .sort((a, b) => a.tag.localeCompare(b.tag, "en"));
}

// reports added to the tracker per day (UTC), every day from the first addition to `until`
export function addedPerDay(list, until = new Date()) {
  const byDay = new Map();
  for (const r of list) {
    if (!r.added_at) continue;
    const k = String(r.added_at).slice(0, 10), day = byDay.get(k) ?? byDay.set(k, { total: 0, types: {} }).get(k);
    day.total++;
    day.types[r.type] = (day.types[r.type] ?? 0) + 1;
  }
  if (!byDay.size) return [];
  const first = [...byDay.keys()].sort()[0], last = until.toISOString().slice(0, 10);
  const out = [];
  let cumulative = 0;
  for (let d = new Date(`${first}T00:00:00Z`); d.toISOString().slice(0, 10) <= last; d.setUTCDate(d.getUTCDate() + 1)) {
    const key = d.toISOString().slice(0, 10), day = byDay.get(key) ?? { total: 0, types: {} };
    cumulative += day.total;
    out.push({ key, ...day, cumulative });
  }
  return out;
}
