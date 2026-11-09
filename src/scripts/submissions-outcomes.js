// The submissions page's statistics (/v1/submissions/outcomes): the row of figures under the
// headline, and what became of every source and where they came from. Every minute while visible.
import { ORIGINS } from "../lib/submission-status.js";

const figures = document.getElementById("register-outcome-figures");
const section = document.querySelector("[data-outcomes]");
const api = (figures?.dataset.api || "").replace(/\/$/, "");

// outcomes as a reader groups them, in this order; the rest is still in progress
const GROUPS = [
  ["Published", "done", ["published"]],
  ["Waiting to publish or deploying", "done-soon", ["committed", "deploying", "merged"]],
  ["Already covered", "covered", ["duplicate"]],
  ["Routed to the CFI", "covered", ["routed_to_cfi"]],
  ["Outside scope", "declined", ["not_relevant"]],
  ["Unable to verify", "declined", ["insufficient_evidence"]],
  ["Source unavailable", "declined", ["source_unavailable"]],
  ["Needs review", "attention", ["needs_review"]],
  ["Failed", "failed", ["failed"]],
];
const REPORTS = new Set(["published", "committed", "deploying", "merged"]);

const num = (n) => Number(n || 0).toLocaleString("en-GB");
const pct = (part, whole) => whole ? `${Math.round((part / whole) * 100)}%` : "—";
const duration = (minutes) => minutes < 60 ? `${minutes} min` : minutes < 48 * 60 ? `${Math.floor(minutes / 60)} h${minutes % 60 ? ` ${minutes % 60} min` : ""}` : `${Math.round(minutes / 1440)} days`;
const set = (name, text) => { const el = figures?.querySelector(`[data-fig="${name}"]`); if (el) el.textContent = text; };

function bars(list, rows) {
  const max = Math.max(1, ...rows.map((r) => r.n));
  const total = rows.reduce((t, r) => t + r.n, 0);
  list.replaceChildren(...rows.map((r) => {
    const li = document.createElement("li");
    li.dataset.tone = r.tone || "";
    const label = document.createElement("span"); label.className = "sx-bar-label"; label.textContent = r.label;
    const track = document.createElement("span"); track.className = "sx-bar-track";
    const fill = document.createElement("i"); fill.style.width = `${Math.max(r.n ? 0.6 : 0, (r.n / max) * 100)}%`; track.append(fill);
    const value = document.createElement("span"); value.className = "sx-bar-value"; value.textContent = num(r.n);
    const share = document.createElement("small"); share.textContent = pct(r.n, total); value.append(" ", share);
    li.append(label, track, value);
    return li;
  }));
}

function render(o) {
  const status = o.by_status || {};
  const all = Object.values(status).reduce((t, n) => t + n, 0);
  const grouped = new Set(GROUPS.flatMap(([, , keys]) => keys));
  const open = Object.entries(status).filter(([k]) => !grouped.has(k)).reduce((t, [, n]) => t + n, 0);
  const reports = Object.entries(status).filter(([k]) => REPORTS.has(k)).reduce((t, [, n]) => t + n, 0);
  const covered = (status.duplicate || 0) + (status.routed_to_cfi || 0);
  const finished = all - open;
  const declined = finished - reports - covered - (status.needs_review || 0) - (status.failed || 0);

  set("received", num(o.received?.day)); set("received-note", `${num(o.received?.week)} in 7 days`);
  set("committed", num(o.committed?.day)); set("committed-note", `${num(o.committed?.week)} in 7 days`);
  set("published", num(o.published?.week)); set("published-note", `${num(o.published?.day)} in the last 24 h`);
  const speed = o.minutes_to_commit;
  set("speed", speed ? duration(speed.median) : "—");
  set("speed-note", speed ? `median · 90% within ${duration(speed.p90)}` : "no reports in 7 days");
  set("rate", pct(reports, finished));
  set("rate-note", finished ? `${pct(covered, finished)} already covered · ${pct(declined, finished)} turned down` : " ");

  if (section) {
    const rows = GROUPS.map(([label, tone, keys]) => ({ label, tone, n: keys.reduce((t, k) => t + (status[k] || 0), 0) })).filter((r) => r.n);
    if (open) rows.push({ label: "Still in progress", tone: "running", n: open });
    bars(section.querySelector('[data-bars="status"]'), rows);
    const origins = Object.entries(o.by_origin || {}).map(([k, n]) => ({ label: k === "rss" ? "News discovery" : ORIGINS[k] || k, n })).sort((a, b) => b.n - a.n);
    bars(section.querySelector('[data-bars="origin"]'), origins);
    section.hidden = false;
  }
}

let poll = 0;
async function load() {
  clearTimeout(poll);
  if (!api || document.hidden) return;
  try {
    const r = await fetch(`${api}/v1/submissions/outcomes`, { credentials: "omit", cache: "no-store" });
    if (r.ok) render(await r.json());
  } catch { /* the figures stay as they are */ }
  poll = window.setTimeout(load, 60000);
}
document.addEventListener("visibilitychange", () => { if (!document.hidden) load(); });
load();
