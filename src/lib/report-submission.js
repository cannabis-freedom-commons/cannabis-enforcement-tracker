// The short line about the submission a report came from, on the report page: when it was sent and
// how, how long the AI research took and what it cost. Only what a reader would ask; the request
// page has the rest.
import { ORIGINS, duration } from "./submission-status.js";
import { formatUSD, displayedRunCost } from "./model-cost.js";

export const SUBMISSION_ID = /^[0-9a-f]{32}$/;
export const shortSubmission = (id) => String(id).slice(0, 8);

export function submissionFacts(v, ai) {
  const facts = [];
  if (Number(v?.created_at) > 0) {
    const when = new Date(v.created_at * 1000).toLocaleDateString("en-GB", { day: "numeric", month: "short", year: "numeric", timeZone: "UTC" });
    facts.push(ORIGINS[v.origin] ? `submitted ${when} via ${ORIGINS[v.origin]}` : `submitted ${when}`);
  }
  const runs = (ai?.activity?.runs || []).filter((r) => r.started_at && r.finished_at > r.started_at);
  if (runs.length) {
    const seconds = runs.reduce((t, r) => t + r.finished_at - r.started_at, 0);
    facts.push(`researched in ${duration(seconds)}${runs.length > 1 ? ` over ${runs.length} runs` : ""}`);
    const cost = runs.reduce((t, r) => t + displayedRunCost(r, ai?.processing?.plan), 0);
    if (cost > 0) facts.push(`AI cost ${formatUSD(cost)}`);
  }
  return facts;
}
