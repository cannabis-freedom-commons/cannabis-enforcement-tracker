export const RECEIPT_KEY = "tracker.submissions.v1";
export const ID_PATTERN = /^[a-f0-9]{32}$/;
export const TERMINAL = new Set(["published", "duplicate", "not_relevant", "insufficient_evidence", "routed_to_cfi", "needs_review", "failed", "source_unavailable"]);
export const STATUS_LABELS = {
  queued: "Queued", received: "Queued", fetching: "Researching", classifying: "Researching",
  researching: "Researching", verifying: "Checking the answer", drafted: "Drafted", awaiting_review: "Awaiting review",
  committed: "Waiting to publish",
  budget_wait: "Budget paused",
  publishing: "Publishing", merged: "Deploying", deploying: "Deploying", published: "Published", duplicate: "Already covered",
  not_relevant: "Outside scope", routed_to_cfi: "Routed to CFI", insufficient_evidence: "Unable to verify",
  needs_review: "Needs review", failed: "Failed", source_unavailable: "Source unavailable",
};
export function parseReceipts(value) {
  if (!value || value.version !== 1 || !Array.isArray(value.receipts) || value.receipts.length > 500) throw new Error("Use a version 1 receipt export with at most 500 receipts.");
  const seen = new Set();
  return value.receipts.map(r => {
    if (!r || !ID_PATTERN.test(r.submission_id) || !Number.isSafeInteger(r.created_at) || r.created_at < 0 || r.created_at > 4102444800) throw new Error("The file contains an invalid receipt.");
    return { submission_id: r.submission_id, created_at: r.created_at };
  }).filter(r => !seen.has(r.submission_id) && seen.add(r.submission_id));
}
export function mergeReceipts(a, b) {
  return parseReceipts({ version: 1, receipts: [...new Map([...a, ...b].map(r => [r.submission_id, r])).values()] }).sort((a, b) => b.created_at - a.created_at);
}
export function safeHTTP(value) { try { const u = new URL(value); return ["https:", "http:"].includes(u.protocol) && !u.username && !u.password ? u.href : null; } catch { return null; } }
