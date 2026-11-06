// A report's structured details as labelled rows for its infobox, in reading order. Ids are named
// from src/data/vocabularies.json and src/data/agencies.json; a name as published stays as it is.
import vocabularies from "../data/vocabularies.json" with { type: "json" };
import agencies from "../data/agencies.json" with { type: "json" };
import { formatWeight } from "./figures.js";
import { flagOf } from "./countries.js";

const label = (list, id) => vocabularies[list]?.find((t) => t.id === id)?.label ?? id;
export const agencyName = (id) => agencies.find((a) => a.id === id)?.name ?? id;
// an agency as a link target: its registry id (it has a page at /agency/<id>/), or null for a name as published
export const agencyId = (id) => (agencies.some((a) => a.id === id) ? id : null);
// the flag of a registry agency's country (none for international bodies and names as published)
const agencyFlag = (id) => { const c = agencies.find((a) => a.id === id)?.country; return c ? flagOf(c) : null; };
const fmtNum = (n) => n.toLocaleString("en-GB", { maximumFractionDigits: 1 });
const dateText = (d) => new Date(d).toLocaleDateString("en-GB", { day: "numeric", month: "short", year: "numeric", timeZone: "UTC" });

export function detailRows(d = {}, countryName = (id) => id) {
  const rows = [];
  const push = (key, labelText, value) => value && rows.push({ key, label: labelText, value });
  const list = (key, labelText, vocab) => d[key]?.length && push(key, labelText, d[key].map((id) => label(vocab, id)).join(", "));
  if (d.agencies?.length) rows.push({ key: "agencies", label: d.agencies.length > 1 ? "Agencies" : "Agency", value: d.agencies.map(agencyName).join("; "), parts: d.agencies.map((a) => ({ text: agencyName(a), agency: agencyId(a), flag: agencyFlag(a) })), sep: "; " });
  list("agency_types", "Agency type", "agency_type");
  if (d.products?.length) push("products", "Cannabis", d.products.map((p) => {
    const amount = [p.kg != null && formatWeight(p.kg), p.plants != null && `${fmtNum(p.plants)} plants`].filter(Boolean).join(", ");
    return amount ? `${label("product", p.product)} (${amount})` : label("product", p.product);
  }).join("; "));
  list("activity", "Activity", "activity");
  list("setting", "Setting", "setting");
  list("environment", "Grown", "environment");
  if (d.charges?.length) push("charges", d.charges.length > 1 ? "Charges" : "Charge", d.charges.join("; "));
  if (d.case_stage) push("case_stage", "Case stage", label("case_stage", d.case_stage));
  list("sentence_types", "Sentence", "sentence_type");
  list("force", "Force", "force");
  if (d.period) push("period", "Operation", `${dateText(d.period.start)} – ${dateText(d.period.end)}`);
  if (d.region) push("region", "Region", d.region);
  if (d.route) {
    const ends = [d.route.from?.map(countryName).join(", "), d.route.to?.map(countryName).join(", ")].filter(Boolean).join(" → ");
    push("route", "Route", [ends, d.route.crossing && `via ${d.route.crossing}`].filter(Boolean).join(" "));
  }
  if (d.date_precision && d.date_precision !== "exact") push("date_precision", "Date", label("date_precision", d.date_precision));
  return rows;
}

// an agency in few words, for the summary at the top: its acronym where it has one ("NDLEA"),
// else its name; a name as published is cut at its first comma
export const agencyShort = (id) => {
  const a = agencies.find((x) => x.id === id);
  if (!a) return id.split(",")[0].trim();
  return a.name.match(/\(([A-Z][A-Za-z0-9.-]{1,11})\)$/)?.[1] ?? a.name;
};

// the few details a reader wants first, for the summary under the title: who acted, what was
// taken and how far the case went. The full list, with sources, is at the end of the page.
export function glanceFacts(d = {}) {
  const facts = [];
  if (d.agencies?.length) {
    const names = d.agencies.slice(0, 2);
    facts.push({ key: "agencies", label: d.agencies.length > 1 ? "Agencies" : "Agency", value: names.map(agencyShort).join(", "),
      parts: names.map((a) => ({ text: agencyShort(a), agency: agencyId(a), title: agencyName(a), flag: agencyFlag(a) })), sep: ", ",
      more: d.agencies.length > 2 ? `+${d.agencies.length - 2}` : null });
  }
  if (d.products?.length) facts.push({ key: "products", label: "Cannabis", value: [...new Set(d.products.map((p) => label("product", p.product)))].join(", ") });
  if (d.case_stage) facts.push({ key: "case_stage", label: "Case stage", value: label("case_stage", d.case_stage) });
  return facts;
}
