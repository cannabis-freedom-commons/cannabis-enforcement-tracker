// Event dates and tracker addition times are separate clocks. Unknown addition times stay unknown.
export const reportDay = (r) => (r.d ?? (r.date instanceof Date ? r.date.toISOString() : r.date) ?? "").slice(0, 10);
const addedTime = (r) => Date.parse(r.at ?? r.added_at ?? "") || 0;
export function compareReports(a, b) {
  return reportDay(b).localeCompare(reportDay(a))
    || addedTime(b) - addedTime(a)
    || (a.i ?? a.id ?? "").localeCompare(b.i ?? b.id ?? "");
}

export function addedAtTitle(value) {
  return `${new Intl.DateTimeFormat("en-GB", { day: "numeric", month: "short", year: "numeric",
    hour: "2-digit", minute: "2-digit", second: "2-digit", timeZone: "UTC" }).format(new Date(value))} UTC`;
}
export function addedAtLabel(value, now = Date.now(), verb = "Added") {
  const seconds = Math.max(0, Math.floor((now - Date.parse(value)) / 1000));
  if (seconds < 60) return `${verb} just now`;
  const [amount, unit] = seconds < 3600 ? [Math.floor(seconds / 60), "minute"]
    : seconds < 86400 ? [Math.floor(seconds / 3600), "hour"]
    : seconds < 30 * 86400 ? [Math.floor(seconds / 86400), "day"]
    : seconds < 365 * 86400 ? [Math.floor(seconds / (30 * 86400)), "month"]
    : [Math.floor(seconds / (365 * 86400)), "year"];
  return `${verb} ${amount} ${unit}${amount === 1 ? "" : "s"} ago`;
}
