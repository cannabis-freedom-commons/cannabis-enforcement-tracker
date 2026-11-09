// What a submission's status means, for the list (/submissions/) and the request page
// (/submission/?id=): a short label, the stage of the pipeline it is in, whether it is still
// running, and how it ended. Shared so both pages say the same thing.
import { STATUS_LABELS } from "./submission-receipts.js";

// the pipeline, in order; a request moves left to right and ends in one outcome
export const STAGES = ["Queued", "Source check", "Research", "Validate", "Publish", "Outcome"];

// tone: running (still being worked on), done (published), covered (already in the tracker or
// sent to the CFI), declined (not published, with a reason), attention (a person has to look), failed
const META = {
  budget_wait: { stage: 2, tone: "paused", label: "Budget paused" },
  queued: { stage: 0, tone: "running", label: "Queued" },
  received: { stage: 0, tone: "running", label: "Queued" },
  fetching: { stage: 1, tone: "running", label: "Checking the source" },
  classifying: { stage: 2, tone: "running", label: "Source checked" },
  researching: { stage: 2, tone: "running", label: "Researching" },
  verifying: { stage: 3, tone: "running", label: "Checking the answer" },
  drafted: { stage: 3, tone: "running", label: "Drafted" },
  awaiting_review: { stage: 3, tone: "running", label: "Awaiting review" },
  publishing: { stage: 4, tone: "running", label: "Publishing" },
  // committed: waiting for the next batch to main; deploying: on main, waiting for the site's build
  committed: { stage: 4, tone: "running", label: "Waiting to publish" },
  merged: { stage: 4, tone: "running", label: "Deploying" },
  deploying: { stage: 4, tone: "running", label: "Deploying" },
  published: { stage: 5, tone: "done", label: "Published" },
  duplicate: { stage: 5, tone: "covered", label: "Already covered" },
  routed_to_cfi: { stage: 5, tone: "covered", label: "Routed to the CFI" },
  source_unavailable: { stage: 5, tone: "declined", label: "Source unavailable" },
  not_relevant: { stage: 5, tone: "declined", label: "Outside scope" },
  insufficient_evidence: { stage: 5, tone: "declined", label: "Unable to verify" },
  needs_review: { stage: 5, tone: "attention", label: "Needs review" },
  failed: { stage: 5, tone: "failed", label: "Failed" },
};
export const statusMeta = (status) => META[status] ?? { stage: 0, tone: "running", label: STATUS_LABELS[status] ?? status };
export const isRunning = (status) => statusMeta(status).tone === "running";

// what an outcome means for the person who sent the source, beside the worker's own explanation
export const OUTCOME_HELP = {
  budget_wait: "This request is waiting for the daily AI budget to reset. No new research run has started.",
  committed: "A cited report was written and committed. It goes to the site with the next batch (every 30 minutes) and is published once the site is rebuilt.",
  deploying: "The report is on main and appears on the site with its next build (every 30 minutes, at :13 and :43).",
  published: "A cited report was written and published from this source.",
  duplicate: "The tracker already has a report on this event, so no new one was made.",
  routed_to_cfi: "This is about a change in the law, not an enforcement event, so it belongs in the Cannabis Freedom Index history.",
  source_unavailable: "The submitted page could not be loaded or read, so it was not researched. A report rests on the source that was sent.",
  not_relevant: "The source is not about a cannabis raid, seizure, eradication, arrest, prosecution or sentence.",
  insufficient_evidence: "The facts could not be confirmed well enough from the sources to publish a report.",
  needs_review: "The automatic checks stopped short of publishing; an editor will look at it.",
  failed: "Processing stopped because of an error. It may be retried.",
};

// the filter choices in the list, grouped by what a reader is looking for
export const FILTERS = [
  ["", "All"],
  ["researching", "Researching"],
  ["queued", "Queued"],
  ["committed", "Waiting to publish"],
  ["deploying", "Deploying"],
  ["published", "Published"],
  ["duplicate", "Already covered"],
  ["insufficient_evidence", "Unable to verify"],
  ["source_unavailable", "Source unavailable"],
  ["not_relevant", "Outside scope"],
  ["routed_to_cfi", "Routed to CFI"],
  ["needs_review", "Needs review"],
  ["failed", "Failed"],
];
export const ORIGINS = { web: "Website", nostr: "Nostr", rss: "RSS", gdelt: "GDELT", api: "API" };

// The history repeats "opening the source → checking scope → researching" for every retry; an
// attempt starts each time the source is opened again. Returns [{ entries, start, end, status }].
export function attempts(history = []) {
  const out = [];
  for (const h of history) {
    const opensAgain = h.status === "fetching" && out.length && out.at(-1).entries.some((e) => e.status !== "queued");
    if (!out.length || opensAgain) out.push({ entries: [] });
    out.at(-1).entries.push(h);
  }
  return out.map((a) => ({ ...a, start: a.entries[0].created_at, end: a.entries.at(-1).created_at, status: a.entries.at(-1).status }));
}

// "5 min ago", "yesterday", or a date
export function relativeTime(seconds, now = Date.now() / 1000) {
  const d = Math.max(0, now - seconds);
  if (d < 45) return "just now";
  if (d < 3600) return `${Math.round(d / 60)} min ago`;
  if (d < 86400) return `${Math.round(d / 3600)} h ago`;
  if (d < 172800) return "yesterday";
  return new Date(seconds * 1000).toLocaleDateString(undefined, { day: "numeric", month: "short", year: new Date(seconds * 1000).getFullYear() === new Date().getFullYear() ? undefined : "numeric" });
}
export const fullTime = (seconds) => new Date(seconds * 1000).toLocaleString(undefined, { day: "numeric", month: "short", year: "numeric", hour: "2-digit", minute: "2-digit" });

// how long something took: "45 s", "3 min", "1 h 20 min"
export function duration(seconds) {
  const s = Math.max(0, Math.round(seconds));
  if (s < 60) return `${s} s`;
  if (s < 3600) return `${Math.round(s / 60)} min`;
  return `${Math.floor(s / 3600)} h ${Math.round((s % 3600) / 60)} min`;
}

// a source address as host and path, for a compact line
export function sourceParts(url) {
  try {
    const u = new URL(url);
    const path = decodeURIComponent(u.pathname + u.search).replace(/\/$/, "");
    return { host: u.hostname.replace(/^www\./, ""), path: path || "/" };
  } catch { return { host: url, path: "" }; }
}
