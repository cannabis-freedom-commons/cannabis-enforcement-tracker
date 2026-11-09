// The submissions list (/submissions/): one row per request, most recently updated first, live over a WebSocket.
// A row says what is happening (a pulsing status while running, its stage and attempt) or how it
// ended and why, and links to the request's own page.
import { modelLabel, renderProcessingStatus } from "../lib/ai-activity.js";
import { runCost, planCost, formatUSD, displayedRunCost } from "../lib/model-cost.js";
import { ID_PATTERN } from "../lib/submission-receipts.js";
import { queueEstimate } from "../lib/queue-estimate.js";
import { statusMeta, isRunning, STAGES, ORIGINS, relativeTime, fullTime, sourceParts } from "../lib/submission-status.js";

const root = document.querySelector("[data-submission-register]");
if (root) initialize(root);

function initialize(root) {
  const api = root.dataset.api.replace(/\/$/, "");
  if (!api) return;
  const base = root.dataset.base;
  const $ = (id) => document.getElementById(`register-${id}`);
  const el = (tag, text, className) => { const n = document.createElement(tag); if (text !== undefined) n.textContent = text; if (className) n.className = className; return n; };
  let cursors = [""], nextCursor = "", revision = "", rows = [], status = "", plan = null, sort = "updated", order = "desc";
  const sortDefault = { received: "desc", updated: "desc", id: "asc", source: "asc", status: "asc", tokens: "desc", runs: "desc", cost: "desc" };
  let socket, retryTimer, refreshTimer, tick, failures = 0, request, pending = false, disposed = false, lastRefresh = 0;
  const rowCaches = new WeakMap();
  let processingSignature = "";

  function connection(text, state = "offline") { const c = $("connection"); c.dataset.state = state; c.querySelector("span").textContent = text; }
  function controls() {
    $("previous").disabled = cursors.length === 1; $("newest").disabled = cursors.length === 1; $("next").disabled = !nextCursor;
    $("page").textContent = `Page ${cursors.length}`;
    root.querySelector(".sx-pager").hidden = cursors.length === 1 && !nextCursor;
  }
  async function get(path, signal) {
    const res = await fetch(`${api}${path}`, { credentials: "omit", signal: AbortSignal.any([signal, AbortSignal.timeout(15000)]) });
    if (!res.ok) throw new Error("Submissions could not be refreshed.");
    return res.json();
  }

  // a status pill: a pulsing dot while running, a mark for how it ended
  function pill(s) {
    const meta = statusMeta(s), p = el("span", undefined, "sx-pill"); p.dataset.tone = meta.tone;
    p.append(el("i", undefined, "sx-dot"), document.createTextNode(meta.label)); return p;
  }
  // One row, in the order a reader scans: when it came in, when it last changed (the full register
  // only), its short id, the source, where it is (status and the pipeline, the step being worked on
  // pulsing), tokens read and written, AI runs and how long the request has taken, and what it cost
  // (the plan's share when there is a plan).
  const compact = (n) => { n = Number(n) || 0; return n >= 1e6 ? `${(n / 1e6).toFixed(n >= 1e7 ? 0 : 1)}M` : n >= 1e3 ? `${(n / 1e3).toFixed(n >= 1e4 ? 0 : 1)}k` : String(n); };
  const span = (sec) => { sec = Math.max(0, Math.round(sec)); return sec < 60 ? `${sec}s` : sec < 3600 ? `${Math.floor(sec / 60)}m ${String(sec % 60).padStart(2, "0")}s` : `${Math.floor(sec / 3600)}h ${String(Math.round((sec % 3600) / 60)).padStart(2, "0")}m`; };
  const stamp = (sec) => { const d = new Date(sec * 1000), p = (x) => String(x).padStart(2, "0"); return { date: `${p(d.getMonth() + 1)}-${p(d.getDate())}`, time: `${p(d.getHours())}:${p(d.getMinutes())}` }; };
  const timeCell = (sec, title) => {
    const { date, time } = stamp(sec);
    const when = el("time", undefined, "sx-row-time"); when.dateTime = new Date(sec * 1000).toISOString(); when.title = title;
    when.append(el("span", date, "sx-row-date"), el("span", time));
    return when;
  };
  function row(v, showUpdated = false) {
    const meta = statusMeta(v.status), running = isRunning(v.status), now = Date.now() / 1000;
    const li = el("li", undefined, "sx-row"); li.dataset.tone = meta.tone;
    const a = el("a", undefined, "sx-row-link"); a.href = `${base}submission/?id=${v.id}`; a.title = v.explanation;
    const when = timeCell(v.created_at, `Received ${fullTime(v.created_at)}`);
    const updated = timeCell(Math.max(v.created_at, v.updated_at || 0), `Updated ${fullTime(Math.max(v.created_at, v.updated_at || 0))}`);
    const id = el("span", v.id.slice(0, 7), "sx-row-id"); id.title = `Receipt ${v.id}`;
    // the source
    const { host, path } = sourceParts(v.url);
    const src = el("span", undefined, "sx-row-source"); src.append(el("strong", host), el("span", path, "sx-path")); src.title = `${v.url}\n${ORIGINS[v.origin] || v.origin}${v.note ? ` · ${v.note}` : ""}`;
    // where it is: the status, and under it the pipeline
    const state = el("span", undefined, "sx-row-state");
    const label = el("span", undefined, "sx-row-label"); label.append(el("i", undefined, "sx-dot"), document.createTextNode(meta.label));
    const progress = el("span", undefined, "sx-row-progress");
    const lastStage = meta.stage === 5 && !["published", "duplicate", "routed_to_cfi"].includes(v.status) && v.last_stage ? statusMeta(v.last_stage).stage : meta.stage;
    progress.setAttribute("role", "progressbar"); progress.setAttribute("aria-label", "Processing stage");
    progress.setAttribute("aria-valuemin", "0"); progress.setAttribute("aria-valuemax", "5"); progress.setAttribute("aria-valuenow", String(lastStage));
    progress.setAttribute("aria-valuetext", meta.label); progress.title = `${STAGES[lastStage]} · ${meta.label}${v.resume_at ? ` · resumes ${fullTime(v.resume_at)}` : ""}`;
    for (let i = 0; i < STAGES.length; i++) { const part = el("i"); part.dataset.state = i < lastStage ? "done" : i === lastStage ? (running ? "active" : "current") : "todo"; progress.append(part); }
    state.append(label, progress);
    // tokens read and written
    const tokens = el("span", undefined, "sx-row-tokens");
    tokens.append(el("span", compact(v.input_tokens)), el("span", "/", "sx-row-sep"), el("span", compact(v.output_tokens)));
    tokens.title = `${Number(v.input_tokens || 0).toLocaleString()} tokens read (new and cached) · ${Number(v.output_tokens || 0).toLocaleString()} written (answer and reasoning)`;
    // AI runs and how long the request has taken (until now while it runs)
    const list = Array.isArray(v.runs) ? v.runs : [];
    const runTime = list.reduce((t, r) => t + (r.started_at ? (r.finished_at || (r.status === "running" ? now : r.started_at)) - r.started_at : 0), 0);
    const total = (running ? now : Math.max(v.updated_at || 0, v.created_at)) - v.created_at;
    const runs = el("span", undefined, "sx-row-runs");
    runs.append(el("span", String(v.run_count || 0), "sx-row-count"), el("span", "·", "sx-row-sep"), el("span", span(total)));
    runs.title = `${v.run_count || 0} AI ${v.run_count === 1 ? "run" : "runs"} (${span(runTime)} of research) · ${span(total)} from receipt${running ? " so far" : ""}`;
    // cost: the plan's share, or the API price without a plan
    const apiCost = list.reduce((t, r) => t + runCost(r), 0), share = list.reduce((t, r) => t + planCost(runCost(r), plan, r.model), 0), shownCost = list.reduce((t, r) => t + displayedRunCost(r, plan), 0);
    const cost = el("span", apiCost ? formatUSD(shownCost) : "—", "sx-row-cost");
    cost.title = apiCost ? `${share ? `${formatUSD(share)} of the plan · ` : ""}${formatUSD(apiCost)} at API prices` : "No AI run yet";
    const model = el("span", list[0]?.model ? modelLabel(list[0].model) : "Awaiting research", "sx-row-model");
    model.title = list[0]?.model || "No model run recorded yet";
    a.append(when, ...(showUpdated ? [updated] : []), id, src, state, ...(!showUpdated ? [model] : []), tokens, runs, cost);
    li.append(a); return li;
  }
  // Keep unchanged links in place while live work updates. Rebuilding every row
  // loses keyboard focus and forces layout and accessibility updates for the whole table.
  function renderRows(list, values, showUpdated, empty) {
    const previous = rowCaches.get(list) || new Map(), next = new Map();
    const planKey = JSON.stringify(plan), clock = Math.floor(Date.now() / 30000);
    for (const [index, value] of values.entries()) {
      const signature = JSON.stringify(value) + planKey + (isRunning(value.status) ? clock : "");
      const cached = previous.get(value.id);
      const entry = cached?.signature === signature ? cached : { signature, node: row(value, showUpdated) };
      next.set(value.id, entry);
      if (cached && cached !== entry && cached.node.parentNode === list) cached.node.replaceWith(entry.node);
      if (list.children[index] !== entry.node) list.insertBefore(entry.node, list.children[index] || null);
    }
    if (!values.length) {
      if (list.children.length !== 1 || list.firstElementChild?.textContent !== empty) list.replaceChildren(el("li", empty, "sx-empty"));
    } else {
      while (list.children.length > values.length) list.lastElementChild.remove();
    }
    rowCaches.set(list, next);
  }
  function render() {
    const list = $("rows"); list.setAttribute("aria-busy", "false");
    renderRows(list, rows, true, status || $("origin").value ? "No submissions match this filter." : "No submissions yet.");
    controls();
  }
  async function refresh() {
    if (disposed || document.hidden) return;
    if (request) { pending = true; return; }
    const params = new URLSearchParams();
    if (cursors.at(-1)) params.set("cursor", cursors.at(-1));
    if (status) params.set("status", status);
    if ($("origin").value) params.set("origin", $("origin").value);
    params.set("sort", sort); params.set("order", order);
    clearTimeout(refreshTimer); refreshTimer = null;
    lastRefresh = Date.now();
    request = new AbortController(); const active = request;
    try {
      const data = await get(`/v1/submissions?${params}`, active.signal);
      if (disposed || active.signal.aborted) return;
      const nextProcessing = JSON.stringify(data.processing);
      if (nextProcessing !== processingSignature) {
        renderProcessingStatus($("processing"), data.processing, { compact: true });
        processingSignature = nextProcessing;
      }
      plan = data.processing?.plan || null;
      const working = (data.working || []).filter(v => ID_PATTERN.test(v.id));
      renderRows($("working"), working, false, "No submissions are running right now.");
      const limit = data.processing?.max_running > 0 ? `/${data.processing.max_running}` : "";
      $("working-count").textContent = Number.isFinite(data.summary?.running) ? `· ${data.summary.running.toLocaleString()}${limit}` : "";
      rows = data.submissions.filter((v) => ID_PATTERN.test(v.id));
      nextCursor = data.next_cursor; revision = data.revision; render();
      for (const key of ["total", "running", "queued"]) {
        $(key).textContent = Number.isFinite(data.summary?.[key]) ? data.summary[key].toLocaleString() : "—";
      }
      // running against the worker's limit: "1/2"
      const committed = data.summary?.committed_last_hour;
      $("commit-rate").textContent = Number.isFinite(committed) ? (committed / 60).toLocaleString("en-GB", { maximumFractionDigits: 2 }) : "—";
      $("commit-rate").title = Number.isFinite(committed) ? `${committed.toLocaleString()} articles committed in the last 60 minutes` : "Commit rate unavailable";
      $("queue-estimate").textContent = `Est. to clear: ${queueEstimate(data.summary, data.processing)}`;
      if (Number.isFinite(data.summary?.running) && limit) $("running").textContent = `${data.summary.running}${limit}`;
      const running = rows.filter((v) => isRunning(v.status)).length;
      $("count").textContent = `${Number(data.total).toLocaleString()} matching${running ? ` · ${running} in progress` : ""}`;
      $("message").textContent = cursors.length > 1 ? "Showing a later page. Choose First to return to the start." : "";
    } catch {
      if (!active.signal.aborted) { $("message").textContent = "Submissions could not be refreshed; the last loaded list is kept."; if (!rows.length) $("rows").replaceChildren(el("li", "Submissions could not be loaded.", "sx-empty")); }
    } finally { request = null; if (pending) { pending = false; scheduleRefresh(); } }
  }
  // Coalesce stream bursts without postponing updates indefinitely. Explicit navigation
  // still refreshes immediately; background updates run at most once every five seconds (with
  // several requests running, the stream announces a change about every second).
  function scheduleRefresh() {
    if (refreshTimer || disposed || document.hidden) return;
    refreshTimer = setTimeout(() => { refreshTimer = null; refresh(); }, Math.max(250, 5000 - (Date.now() - lastRefresh)));
  }
  function changePage(action) { clearTimeout(refreshTimer); refreshTimer = null; lastRefresh = 0; request?.abort(); action(); pending = true; if (!request) { pending = false; refresh(); } }

  for (const chip of root.querySelectorAll("[data-filter]")) chip.addEventListener("click", () => {
    status = chip.dataset.filter;
    for (const c of root.querySelectorAll("[data-filter]")) c.setAttribute("aria-pressed", String(c === chip));
    changePage(() => { cursors = [""]; });
  });
  function markSort() {
    for (const button of root.querySelectorAll(".sx-table-updated [data-sort]")) {
      button.setAttribute("aria-sort", button.dataset.sort === sort ? (order === "asc" ? "ascending" : "descending") : "none");
    }
  }
  for (const button of root.querySelectorAll(".sx-table-updated [data-sort]")) button.addEventListener("click", () => {
    if (sort === button.dataset.sort) order = order === "desc" ? "asc" : "desc";
    else { sort = button.dataset.sort; order = sortDefault[sort] || "desc"; }
    markSort();
    changePage(() => { cursors = [""]; });
  });
  $("origin").addEventListener("change", () => changePage(() => { cursors = [""]; }));
  $("next").addEventListener("click", () => { if (nextCursor) changePage(() => cursors.push(nextCursor)); });
  $("previous").addEventListener("click", () => changePage(() => { if (cursors.length > 1) cursors.pop(); }));
  $("newest").addEventListener("click", () => changePage(() => { cursors = [""]; }));
  window.addEventListener("submission-created", () => {
    status = "";
    for (const chip of root.querySelectorAll("[data-filter]")) chip.setAttribute("aria-pressed", String(chip.dataset.filter === ""));
    changePage(() => { cursors = [""]; $("origin").value = ""; });
  });
  // "5 min ago" stays true without reloading
  tick = setInterval(() => {
    if (document.hidden) return;
    for (const t of root.querySelectorAll(".sx-list time")) if (!t.childElementCount) t.textContent = relativeTime(Date.parse(t.dateTime) / 1000);
    // Refresh the UTC daily budget even when no WebSocket events occur overnight.
    refresh();
  }, 30000);

  function disconnect() { clearTimeout(retryTimer); const old = socket; socket = null; old?.close(); }
  function connect() {
    if (disposed || document.hidden || socket) return;
    if (!navigator.onLine) { connection("Offline"); return; }
    if (!window.WebSocket) { connection("No live updates"); return; }
    try {
      const url = new URL(`${api}/v1/submissions/stream`, location.href); url.protocol = url.protocol === "https:" ? "wss:" : "ws:";
      connection("Connecting…"); let first = true; const current = new WebSocket(url); socket = current;
      current.onmessage = (event) => {
        if (socket !== current) return;
        try { const data = JSON.parse(event.data); if (data.type === "submissions_changed" && typeof data.revision === "string") { failures = 0; connection("Live", "live"); if (first || data.revision !== revision) scheduleRefresh(); first = false; } } catch { /* unknown message */ }
      };
      current.onclose = () => { if (socket !== current) return; socket = null; reconnect(); };
      current.onerror = () => { if (socket === current) connection("Reconnecting…"); };
    } catch { socket = null; reconnect(); }
  }
  function reconnect() { if (disposed || document.hidden) return; connection("Reconnecting…"); clearTimeout(retryTimer); retryTimer = setTimeout(connect, Math.min(30000, 1000 * 2 ** Math.min(failures++, 5)) + Math.random() * 500); }
  document.addEventListener("visibilitychange", () => { if (document.hidden) { disconnect(); clearTimeout(refreshTimer); refreshTimer = null; connection("Paused"); } else { refresh(); connect(); } });
  window.addEventListener("online", () => { disconnect(); connect(); refresh(); });
  window.addEventListener("offline", () => { disconnect(); connection("Offline"); });
  window.addEventListener("pagehide", () => { disposed = true; disconnect(); request?.abort(); clearTimeout(refreshTimer); refreshTimer = null; clearInterval(tick); });
  window.addEventListener("pageshow", (event) => { if (event.persisted) { disposed = false; connect(); refresh(); } });
  refresh(); connect();
}
