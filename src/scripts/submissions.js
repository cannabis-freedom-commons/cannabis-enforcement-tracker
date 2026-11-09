import { watchSubmissionUpdates } from "../lib/ai-activity.js";
import { renderSubmissionDetail } from "./submission-detail.js";
import { RECEIPT_KEY, ID_PATTERN, TERMINAL, STATUS_LABELS, parseReceipts, mergeReceipts, safeHTTP } from "../lib/submission-receipts.js";

const root = document.querySelector("[data-submissions], [data-submission-status]");
if (root) initialize(root);
function initialize(root) {
  const api = root.dataset.api.replace(/\/$/, "");
  const base = root.dataset.base;
  const $ = id => document.getElementById(id);
  const message = (id, text) => { $(id).textContent = text; };
  const node = (tag, text, className) => { const el = document.createElement(tag); if (text) el.textContent = text; if (className) el.className = className; return el; };
  const link = (label, href) => { const a = node("a", label); a.href = href; return a; };
  let receipts = [], page = 0, timer, polls = 0, busy = false, pollActive = true;
  let storageError = "";
  try { const raw = localStorage.getItem(RECEIPT_KEY); receipts = raw ? parseReceipts(JSON.parse(raw)) : []; }
  catch { storageError = "Browser receipt storage could not be read. Keep the receipt link or export a copy before leaving."; }
  function save() { try { localStorage.setItem(RECEIPT_KEY, JSON.stringify({ version: 1, receipts })); } catch { storageError = "This browser cannot save receipts. Export a copy or keep the receipt link before leaving."; } }
  async function request(path, options = {}) {
    const response = await fetch(api + path, { ...options, signal: AbortSignal.timeout(15000), credentials: "omit", headers: { "Content-Type": "application/json", ...options.headers } });
    const data = await response.json();
    if (!response.ok) throw new Error(data.error || "The service could not complete this request.");
    return data;
  }
  function card(v, receipt) {
    const el = node("article", "", "receipt");
    el.append(node("p", STATUS_LABELS[v?.status] || "Status unavailable", "state"));
    if (v) {
      const heading = node("h3"); const source = safeHTTP(v.url); heading.append(source ? link(v.url, source) : node("span", v.url)); el.append(heading);
      el.append(node("p", v.explanation));
      if (v.note) el.append(node("p", `Public note: ${v.note}`, "hint"));
      if (["published", "duplicate"].includes(v.status) && /^[a-z0-9]+(?:-[a-z0-9]+)*$/.test(v.report_slug || "")) el.append(link("Read report →", `${base}report/${v.report_slug}/`));
      if (v.pr_url && /^https:\/\/github\.com\/[^/]+\/[^/]+\/pull\/\d+$/.test(v.pr_url)) { const p = node("p"); p.append(link("Review proposal on GitHub ↗", v.pr_url)); el.append(p); }
      for (const [key, label] of [["nostr_event_id", "Submission on Nostr ↗"], ["reply_event_id", "Resolution on Nostr ↗"]]) {
        if (/^[a-f0-9]{64}$/.test(v[key] || "")) { const p = node("p"); p.append(link(label, `https://njump.me/${v[key]}`)); el.append(p); }
      }
    }
    const time = node("time", new Date(receipt.created_at * 1000).toLocaleString()); time.dateTime = new Date(receipt.created_at * 1000).toISOString(); el.append(time);
    el.append(link(`Receipt ${receipt.submission_id.slice(0, 12)}…`, `${base}submission/?id=${receipt.submission_id}`));
    return el;
  }
  function renderList(data = []) {
    const byID = new Map(data.map(v => [v.id, v])); const list = $("receipts-list"); list.replaceChildren();
    for (const r of receipts.slice(page * 50, (page + 1) * 50)) {
      const el = card(byID.get(r.submission_id), r); const remove = node("button", "Remove from this browser"); remove.type = "button";
      remove.addEventListener("click", () => { receipts = receipts.filter(v => v.submission_id !== r.submission_id); page = Math.min(page, Math.max(0, Math.ceil(receipts.length / 50) - 1)); save(); renderList(); refresh(); });
      const p = node("p"); p.append(remove); el.append(p); list.append(el);
    }
    $("receipts-prev").hidden = page === 0; $("receipts-next").hidden = (page + 1) * 50 >= receipts.length;
  }
  async function refresh() {
    if (busy || document.hidden || !api) return;
    busy = true;
    try {
      if (root.hasAttribute("data-submission-status")) {
        const id = new URLSearchParams(location.search).get("id"); if (!ID_PATTERN.test(id || "")) { message("status-message", "This link needs a valid receipt ID."); pollActive = false; return; }
        const [v,ai]=await Promise.all([request(`/v1/submissions/${id}`),request(`/v1/submissions/${id}/ai`).catch(() => null)]);
        renderSubmissionDetail(v, ai, { api, id });
        message("status-message", ""); pollActive = !TERMINAL.has(v.status);
      } else {
        if (!receipts.length) { renderList(); message("receipts-message", storageError || "No receipts saved in this browser yet."); pollActive = false; return; }
        const ids = receipts.slice(page * 50, (page + 1) * 50).map(r => r.submission_id).join(",");
        const data = await request(`/v1/submissions?ids=${ids}`); renderList(data.submissions);
        const missing = data.submissions.length !== Math.min(50, receipts.length - page * 50);
        message("receipts-message", storageError || (missing ? "Some receipts could not be found by this service. Their IDs remain saved here." : "Receipts updated."));
        pollActive = data.submissions.some(v => !TERMINAL.has(v.status));
      }
    } catch (err) { message(root.hasAttribute("data-submission-status") ? "status-message" : "receipts-message", `Status could not be refreshed. Saved receipts are kept. ${err.message}`); }
    finally { busy = false; schedule(); }
  }
  function schedule() { clearTimeout(timer); if (pollActive && !document.hidden && polls < 60) timer = setTimeout(() => { polls++; refresh(); }, 30000); }
  document.addEventListener("visibilitychange", () => { clearTimeout(timer); if (!document.hidden) { polls = 0; refresh(); } });
  window.addEventListener("pagehide", () => clearTimeout(timer));
  window.addEventListener("submission-restarted", () => { polls = 0; pollActive = true; refresh(); });
  for (const id of ["refresh-status", "refresh-submissions"]) $(id)?.addEventListener("click", () => { polls = 0; refresh(); });
  if (root.hasAttribute("data-submissions")) {
    renderList(); if (storageError) message("receipts-message", storageError); else if (!receipts.length) message("receipts-message", "No receipts saved in this browser yet.");
    $("submission-form").addEventListener("submit", async event => {
      event.preventDefault(); if (!api) return; const button = event.currentTarget.querySelector("button[type=submit]"); button.disabled = true;
      message("submit-message", "Sending source…");
      try {
        const v = await request("/v1/submissions", { method: "POST", body: JSON.stringify({ url: $("source-url").value.trim(), note: $("public-note").value.trim() }) });
        if (!ID_PATTERN.test(v.id)) throw new Error("The service returned an invalid receipt.");
        // Show the durable receipt even when local storage is full or the browser has 500 receipts.
        if (receipts.length >= 500) { storageError = "Receipt storage is full. Keep this receipt link and export your saved receipts."; }
        else { receipts = mergeReceipts(receipts, [{ submission_id: v.id, created_at: Math.floor(Date.now() / 1000) }]); save(); }
        // straight to the request's own page, where it can be followed as it is worked on; its
        // address is the receipt link
        if (!storageError) { location.assign(`${base}submission/?id=${v.id}`); return; }
        const receiptLink = link("Open your receipt →", `${base}submission/?id=${v.id}`); $("submit-message").replaceChildren(node("p", "Source accepted for processing. Keep this receipt link."), receiptLink);
        if (storageError) $("submit-message").append(node("p", storageError)); $("submission-form").reset(); window.dispatchEvent(new Event("submission-created")); page = 0; pollActive = true; polls = 0; await refresh();
      } catch (err) { message("submit-message", `The submission could not be confirmed. ${err.message}`); }
      finally { button.disabled = false; }
    });
    $("export-receipts").addEventListener("click", () => { const blob = new Blob([JSON.stringify({ version: 1, receipts }, null, 2)], { type: "application/json" }); const url = URL.createObjectURL(blob); const a = link("", url); a.download = "tracker-receipts.json"; a.click(); setTimeout(() => URL.revokeObjectURL(url), 1000); });
    $("import-receipts").addEventListener("change", async event => { const file = event.target.files[0]; if (!file) return; try { if (file.size > 100000) throw new Error("Receipt files must be under 100 KB."); receipts = mergeReceipts(receipts, parseReceipts(JSON.parse(await file.text()))); save(); page = 0; renderList(); await refresh(); if (!api) message("receipts-message", storageError || "Receipts imported. Status is temporarily unavailable."); } catch (err) { message("receipts-message", err.message); } event.target.value = ""; });
    for (const [id, delta] of [["receipts-prev", -1], ["receipts-next", 1]]) $(id).addEventListener("click", () => { page += delta; renderList(); refresh(); });
  }
  if (api && root.hasAttribute("data-submission-status")) watchSubmissionUpdates(api,refresh);
  refresh();
}
