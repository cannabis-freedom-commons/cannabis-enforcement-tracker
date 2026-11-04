// The reports added most recently, newest first, as [id, added_at] pairs: enough for a page to count
// and mark the reports a reader has not seen yet (src/scripts/site-updates.js) without the full
// index. The last 30 days of additions, at most 1,000; `from` is the oldest addition the list is
// complete back to (null when it holds every report with an addition time).
import { reports } from "../../lib/reports.js";

export const RECENT_DAYS = 30, RECENT_MAX = 1000;

export function GET() {
  const added = reports.filter((r) => r.added_at).sort((a, b) => b.added_at.localeCompare(a.added_at));
  const cutoff = added.length ? new Date(Date.parse(added[0].added_at) - RECENT_DAYS * 864e5).toISOString() : "";
  const list = added.filter((r) => r.added_at >= cutoff).slice(0, RECENT_MAX);
  const complete = list.length === added.length;
  return new Response(JSON.stringify({ v: 1, from: complete ? null : list.at(-1)?.added_at ?? null, reports: list.map((r) => [r.id, r.added_at]) }), { headers: { "Content-Type": "application/json" } });
}
