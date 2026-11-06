// A report's structured details as labelled rows for its infobox, in reading order. Ids are named
// from src/data/vocabularies.json and src/data/agencies.json; a name as published stays as it is.
import vocabularies from "../data/vocabularies.json" with { type: "json" };
import agencies from "../data/agencies.json" with { type: "json" };
import { formatWeight } from "./figures.js";

const label = (list, id) => vocabularies[list]?.find((t) => t.id === id)?.label ?? id;
export const agencyName = (id) => agencies.find((a) => a.id === id)?.name ?? id;
const fmtNum = (n) => n.toLocaleString("en-GB", { maximumFractionDigits: 1 });
const dateText = (d) => new Date(d).toLocaleDateString("en-GB", { day: "numeric", month: "short", year: "numeric", timeZone: "UTC" });

export function detailRows(d = {}, countryName = (id) => id) {
  const rows = [];
  const push = (key, labelText, value) => value && rows.push({ key, label: labelText, value });
  const list = (key, labelText, vocab) => d[key]?.length && push(key, labelText, d[key].map((id) => label(vocab, id)).join(", "));
  if (d.agencies?.length) push("agencies", d.agencies.length > 1 ? "Agencies" : "Agency", d.agencies.map(agencyName).join("; "));
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
