// Count each report once by its primary category, within the map's date window.
export function reportTimeline(rows, { start = "", end = "", year = "", month = "", daily = false, yearly = false, dateOf = (r) => r.d } = {}) {
  const dates = rows.map(dateOf).filter(Boolean).sort();
  let first = start || (year ? `${year}-01-01` : dates[0]);
  let last = end || (year ? `${year}-12-31` : dates.at(-1));
  if (year) {
    first = first > `${year}-01-01` ? first : `${year}-01-01`;
    last = last < `${year}-12-31` ? last : `${year}-12-31`;
  }
  if (!first || !last || first > last) return [];
  const buckets = new Map();
  const date = new Date(`${yearly ? `${first.slice(0, 4)}-01-01` : daily ? first : `${first.slice(0, 7)}-01`}T00:00:00Z`);
  while (date.toISOString().slice(0, 10) <= last) {
    const key = date.toISOString().slice(0, yearly ? 4 : daily ? 10 : 7);
    if (yearly || !month || key.slice(5, 7) === month) buckets.set(key, { key, total: 0, categories: {} });
    if (yearly) date.setUTCFullYear(date.getUTCFullYear() + 1);
    else if (daily) date.setUTCDate(date.getUTCDate() + 1);
    else date.setUTCMonth(date.getUTCMonth() + 1);
  }
  for (const row of rows) {
    const day = dateOf(row);
    if (!day || day < first || day > last || (month && day.slice(5, 7) !== month)) continue;
    const bucket = buckets.get(day.slice(0, yearly ? 4 : daily ? 10 : 7));
    if (!bucket) continue;
    bucket.total++;
    bucket.categories[row.t] = (bucket.categories[row.t] || 0) + 1;
  }
  return [...buckets.values()];
}

export function isReportDay(value) {
  return typeof value === "string" && /^\d{4}-\d{2}-\d{2}$/.test(value)
    && !Number.isNaN(Date.parse(`${value}T00:00:00Z`))
    && new Date(`${value}T00:00:00Z`).toISOString().slice(0, 10) === value;
}

// Daily bars narrow only the day, keeping the surrounding calendar/window intact.
export function timelineSelection(key, { year = "", month = "", date = "" } = {}) {
  if (isReportDay(key)) return { year, month, date: date === key ? "" : key };
  if (!/^\d{4}(?:-(?:0[1-9]|1[0-2]))?$/.test(key)) return null;
  const [y, m = ""] = key.split("-");
  const same = year === y && month === m;
  return { year: same ? "" : y, month: same ? "" : m, date: "" };
}

// The x axis labels of a timeline: which bars get one and what it says, as many as fit the width
// (about 56 px apart), at round steps aligned to the calendar. keys: "2026", "2026-03" or
// "2026-03-14"; scale: "year", "month" or "day". Returns [{ index, label }].
const MONTH_NAMES = ["Jan", "Feb", "Mar", "Apr", "May", "Jun", "Jul", "Aug", "Sep", "Oct", "Nov", "Dec"];
export function axisTicks(keys, scale, width, gap = 56) {
  const n = keys.length;
  if (!n) return [];
  const per = width / n; // pixels per bar
  const steps = { year: [1, 2, 5, 10, 20, 50, 100], month: [1, 2, 3, 6, 12, 24, 60, 120], day: [1, 2, 7, 14, 31] }[scale];
  const step = steps.find((s) => s * per >= gap) ?? steps.at(-1);
  const ticks = [];
  keys.forEach((key, index) => {
    const year = Number(key.slice(0, 4)), month = Number(key.slice(5, 7)) - 1, day = Number(key.slice(8, 10));
    if (scale === "year") {
      if (year % step === 0) ticks.push({ index, label: String(year) });
    } else if (scale === "month") {
      if (step >= 12) { if (month === 0 && year % (step / 12) === 0) ticks.push({ index, label: String(year) }); }
      else if (month % step === 0) ticks.push({ index, label: month === 0 ? String(year) : MONTH_NAMES[month] });
    } else if (step >= 31) {
      if (day === 1) ticks.push({ index, label: MONTH_NAMES[month] });
    } else if (day === 1 || (step === 14 ? day === 15 : step === 7 ? [8, 15, 22].includes(day) : (day - 1) % step === 0 && day < 30)) {
      ticks.push({ index, label: day === 1 ? `${day} ${MONTH_NAMES[month]}` : String(day) });
    }
  });
  // too few round ticks (a short range): label the ends instead
  if (ticks.length < 2 && n > 1) {
    const label = (k) => scale === "year" ? k : scale === "month" ? `${MONTH_NAMES[Number(k.slice(5, 7)) - 1]} ${k.slice(0, 4)}` : `${Number(k.slice(8, 10))} ${MONTH_NAMES[Number(k.slice(5, 7)) - 1]}`;
    return [{ index: 0, label: label(keys[0]) }, { index: n - 1, label: label(keys[n - 1]) }];
  }
  return ticks;
}
