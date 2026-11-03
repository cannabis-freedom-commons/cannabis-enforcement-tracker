import { isReportDay } from "./report-timeline.js";

const presets = new Set(["24h", "7d", "30d", "1y", "all"]);
const MONTHS = ["Jan", "Feb", "Mar", "Apr", "May", "Jun", "Jul", "Aug", "Sep", "Oct", "Nov", "Dec"];
const LONG_MONTHS = ["January", "February", "March", "April", "May", "June", "July", "August", "September", "October", "November", "December"];

// Calendar links take precedence over legacy rolling-window parameters. Never
// intersect a chosen calendar date with an invisible recent-period restriction.
// allowRange: the page has a From–To control (the map), so ?from= and ?to= are read; elsewhere they
// would be a hidden filter and are ignored.
export function readPeriod(params, defaultRange = "all", { allowRelative = true, allowRange = false } = {}) {
  const date = isReportDay(params.get("date")) ? params.get("date") : "";
  let from = allowRange && isReportDay(params.get("from")) ? params.get("from") : "";
  let to = allowRange && isReportDay(params.get("to")) ? params.get("to") : "";
  if (from && to && from > to) [from, to] = [to, from];
  // a day picked inside a From–To period keeps the period around it, not its year and month
  const ranged = !!(from || to);
  const year = ranged ? "" : date ? date.slice(0, 4) : /^\d{4}$/.test(params.get("year") || "") ? params.get("year") : "";
  const month = ranged ? "" : date ? date.slice(5, 7) : /^(0[1-9]|1[0-2])$/.test(params.get("month") || "") ? params.get("month") : "";
  const calendar = !!(year || month || date || ranged || params.get("range") === "calendar");
  const requested = params.get("range");
  const mode = calendar ? "calendar" : !allowRelative ? "all" : presets.has(requested) ? requested : defaultRange;
  return { mode, range: calendar ? "all" : mode, year, month, date, from, to };
}

export function changePeriod(mode, current, defaultYear = "") {
  if (mode === "calendar") {
    const from = current.from || "", to = current.to || "";
    if (from || to) return { mode, range: "all", year: "", month: "", date: current.date || "", from, to };
    return { mode, range: "all", year: current.year || defaultYear, month: current.month || "", date: current.date || "", from: "", to: "" };
  }
  const range = presets.has(mode) ? mode : "all";
  return { mode: range, range, year: "", month: "", date: "", from: "", to: "" };
}

// A From–To period (either end may be open); the ends are swapped if given the wrong way round.
export function rangePeriod(from = "", to = "") {
  from = isReportDay(from) ? from : "";
  to = isReportDay(to) ? to : "";
  if (from && to && from > to) [from, to] = [to, from];
  if (!from && !to) return changePeriod("all", {});
  return { mode: "calendar", range: "all", year: "", month: "", date: "", from, to };
}

// What a calendar period shows on its button: "2023", "Jul 2023", "14 Mar 2024", "12 Mar – 4 Apr 2026".
export function periodLabel({ year = "", month = "", date = "", from = "", to = "" } = {}) {
  const parts = (d) => ({ y: d.slice(0, 4), m: MONTHS[Number(d.slice(5, 7)) - 1], d: String(Number(d.slice(8, 10))) });
  const full = (d) => { const p = parts(d); return `${p.d} ${p.m} ${p.y}`; };
  if (date) return full(date);
  if (from && to) {
    if (from === to) return full(from);
    const a = parts(from), b = parts(to);
    if (a.y !== b.y) return `${full(from)} – ${full(to)}`;
    if (from.slice(5, 7) === to.slice(5, 7)) return `${a.d}–${b.d} ${b.m} ${b.y}`;
    return `${a.d} ${a.m} – ${b.d} ${b.m} ${b.y}`;
  }
  if (from) return `Since ${full(from)}`;
  if (to) return `Until ${full(to)}`;
  if (year && month) return `${MONTHS[Number(month) - 1]} ${year}`;
  if (year) return year;
  if (month) return `${LONG_MONTHS[Number(month) - 1]}, every year`;
  return "Custom";
}
