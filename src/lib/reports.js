// All reports, sorted newest first, with what the pages and the JSON files need worked out once:
// the country's name, where the dot goes on the map, the year, and the figures as short labels.
import { getCollection } from "astro:content";
import { TYPES } from "./types.js";
import { countryName, countryOf } from "./countries.js";
import { centreOf } from "./map.js";
import { figureRows } from "./figures.js";

export const formatDate = (d) => d.toLocaleDateString("en-GB", { day: "numeric", month: "long", year: "numeric", timeZone: "UTC" });
export const isoDate = (d) => d.toISOString().slice(0, 10);
export { figureRows };

const entries = await getCollection("reports");
export const reports = entries.map(({ id, data, body }) => {
  const coordinates = data.coordinates ?? centreOf(data.country);
  return {
    id, ...data, body,
    url: `report/${id}/`,
    countryName: countryName(data.country),
    parent: countryOf(data.country)?.parent ?? null,
    year: data.date.getUTCFullYear(),
    month: isoDate(data.date).slice(0, 7), // "2026-08"
    typeLabel: TYPES[data.type].label,
    color: TYPES[data.type].color,
    coordinates,
    precision: data.coordinates ? data.precision : coordinates ? "country" : null,
    figureRows: figureRows(data.figures),
    // the first photo with a direct address, shown beside the report in lists (loaded from its
    // publisher, never copied); a report's own cover image comes first where there is one
    photo: data.media?.find((m) => m.kind === "photo" && /^https?:\/\//.test(m.media_url ?? ""))?.media_url ?? null,
  };
}).sort((a, b) => b.date - a.date || a.title.localeCompare(b.title));

// Indexes, so pages stay quick with thousands of reports: by country (a country's list includes
// its states': "us" also lists "us/ca"), by type, and by month ("2026-08"), each newest first.
const group = (key) => {
  const m = new Map();
  for (const r of reports) for (const k of [key(r)].flat().filter(Boolean)) (m.get(k) ?? m.set(k, []).get(k)).push(r);
  return m;
};
const byCountry = group((r) => [r.country, r.parent]);
const byType = group((r) => r.type);
export const byMonth = group((r) => r.month);
export const reportsOf = (id) => byCountry.get(id) ?? [];
export const reportsOfMonth = (month) => byMonth.get(month) ?? [];
export const years = [...new Set(reports.map((r) => r.year))].sort((a, b) => b - a);
export const monthsOf = (year) => [...byMonth.keys()].filter((m) => m.startsWith(`${year}-`)).sort().reverse();
// every month from the first report to the last, with how many reports: for timelines
export function monthSeries(list = reports) {
  if (!list.length) return [];
  const counts = new Map();
  for (const r of list) counts.set(r.month, (counts.get(r.month) ?? 0) + 1);
  const months = [...counts.keys()].sort();
  const out = [];
  for (let [y, m] = months[0].split("-").map(Number); `${y}-${String(m).padStart(2, "0")}` <= months.at(-1); m === 12 ? (y++, m = 1) : m++) {
    const k = `${y}-${String(m).padStart(2, "0")}`;
    out.push({ month: k, count: counts.get(k) ?? 0 });
  }
  return out;
}
export const monthName = (month, style = "long") => new Date(`${month}-01T00:00:00Z`).toLocaleDateString("en-GB", { month: style, year: "numeric", timeZone: "UTC" });

// totals of the figures over a set of reports
export function totals(list) {
  const sum = (k) => list.reduce((s, r) => s + (r.figures?.[k] ?? 0), 0);
  return {
    reports: list.length,
    arrested: sum("arrested"),
    plants_destroyed: sum("plants_destroyed"),
    hectares: +sum("hectares").toFixed(2),
    kg_seized: Number(sum("kg_seized").toPrecision(15)),
    deaths: sum("deaths") + list.filter((r) => r.type === "execution" && r.figures?.deaths == null).length,
  };
}

export const countByType = (list) => Object.fromEntries(Object.keys(TYPES).map((t) => [t, list.filter((r) => r.type === t).length]));

// the countries that have reports, with their count, most first
export const countriesWithReports = () => {
  const m = new Map();
  for (const r of reports) m.set(r.country, (m.get(r.country) ?? 0) + 1);
  return [...m].map(([id, n]) => ({ id, name: countryName(id), count: n })).sort((a, b) => b.count - a.count || a.name.localeCompare(b.name));
};

// reports like this one: same country and type first, then same country, then same type; nearest
// in time first. Looks only at the report's country and type, not at every report.
export function related(r, n = 4) {
  const sameCountry = reportsOf(r.parent ?? r.country);
  const pool = new Set([...sameCountry, ...(byType.get(r.type) ?? []).slice(0, 200)]);
  pool.delete(r);
  const score = (o) => (sameCountry.includes(o) ? 2 : 0) + (o.type === r.type ? 1 : 0);
  return [...pool].sort((a, b) => score(b) - score(a) || Math.abs(a.date - r.date) - Math.abs(b.date - r.date)).slice(0, n);
}

// One report as a row of the shared index (/api/index.json) that the map and the home page's
// filters load: short keys, only what a list item and a map dot need.
export function indexRow(r) {
  return {
    i: r.id, d: isoDate(r.date), c: r.country, t: r.type, a: r.also ?? [],
    ti: r.title, s: r.summary, p: r.place ?? null,
    ll: r.coordinates ? [r.coordinates[1], r.coordinates[0]] : null, // [lon, lat], as GeoJSON
    pr: r.precision ?? null, // how precise ll is (types.js PRECISION)
    f: r.figureRows.filter((f) => f.key !== "value").slice(0, 4).map((f) => [f.label, f.value]),
    k: [`cet-${r.code}`, r.code, ...(r.tags ?? [])].join(" ").toLowerCase(),
    ...(r.tags?.length ? { g: r.tags } : {}), // exact tags for selection, separate from search text
    ...(r.cover || r.photo ? { im: r.cover?.image.src ?? r.photo } : {}), // the list thumbnail
  };
}

// the compact form used by the JSON files (see README: "JSON API")
export function compact(r, site) {
  return {
    id: r.id,
    code: `CET-${r.code}`,
    url: new URL(r.url, site).href,
    short_url: new URL(`r/${r.code}/`, site).href,
    date: isoDate(r.date),
    country: r.country,
    country_name: r.countryName,
    place: r.place ?? null,
    coordinates: r.coordinates ?? null,
    precision: r.precision,
    type: r.type,
    also: r.also ?? [],
    title: r.title,
    summary: r.summary,
    status: r.status,
    figures: r.figures ?? {},
    tags: r.tags ?? [],
    cover: r.cover ? { url: new URL(r.cover.image.src, site).href, alt: r.cover.alt, credit: r.cover.credit, licence: r.cover.licence, source: r.cover.url, ...(r.cover.media_url ? { media_url: r.cover.media_url } : {}) } : null,
    media: r.media ?? [],
    cfi_id: r.cfi_id ?? null,
    created_by_model: r.created_by_model,
    open_questions: r.open_questions ?? [],
    sources: r.sources.map((s) => ({ title: s.title, url: s.url ?? null, publisher: s.publisher, date: s.date ? isoDate(s.date) : null, archived: s.archived ?? null })),
  };
}
