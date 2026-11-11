// The submissions page's activity chart: for the last hour (per minute) or day (per 15 minutes), the
// cases committed and closed without a report per slot, with the sources received as a line (one
// count scale), and under it, on the same time axis, how many cases were open (waiting or running).
// Two strips rather than a second y-axis: the queue is hundreds, the throughput a handful per slot.
// Data: GET /v1/submissions/activity?window=1h|24h (cached by the service for 30 s).
const root = document.querySelector("[data-submission-activity]");
if (root) initialize(root);

function initialize(root) {
  const api = (root.dataset.api || "").replace(/\/$/, "");
  if (!api) return;
  const plot = root.querySelector(".sx-activity-plot"), tip = root.querySelector(".sx-activity-tip");
  const legend = (key) => root.querySelector(`[data-total="${key}"]`);
  const NS = "http://www.w3.org/2000/svg";
  const svg = (tag, attrs = {}) => { const n = document.createElementNS(NS, tag); for (const [k, v] of Object.entries(attrs)) n.setAttribute(k, v); return n; };
  const num = (n) => Number(n).toLocaleString("en-GB");
  const short = (n) => n >= 1e6 ? `${(n / 1e6).toFixed(1)}M` : n >= 1e4 ? `${Math.round(n / 1e3)}k` : n >= 1e3 ? `${(n / 1e3).toFixed(1)}k` : String(Math.round(n));
  const clock = (sec) => new Date(sec * 1000).toLocaleTimeString("en-GB", { hour: "2-digit", minute: "2-digit" });
  let windowName = "1h", data = null, timer = 0, request = null;
  try { if (localStorage.getItem("cet:activity-window") === "24h") windowName = "24h"; } catch { /* no storage */ }

  async function load() {
    clearTimeout(timer);
    if (document.hidden) return;
    request?.abort(); request = new AbortController();
    const asked = windowName;
    try {
      const res = await fetch(`${api}/v1/submissions/activity?window=${asked}`, { credentials: "omit", signal: AbortSignal.any([request.signal, AbortSignal.timeout(15000)]) });
      if (!res.ok) throw new Error();
      const next = await res.json();
      if (asked !== windowName) return;
      data = next; draw(); root.dataset.state = "ready";
    } catch (e) {
      if (e?.name === "AbortError" && request?.signal.aborted) return;
      if (!data) { root.dataset.state = "error"; plot.replaceChildren(); }
    }
    timer = setTimeout(load, 60000);
  }

  function totals() {
    const t = data.totals, slots = data.slots, minutes = (data.to - data.from) / 60;
    legend("committed").textContent = `${num(t.committed)} · ${(t.committed / minutes).toLocaleString("en-GB", { maximumFractionDigits: 2 })}/min`;
    legend("closed").textContent = num(t.closed);
    // a backend from before the split counts failures among the closed
    const failed = root.querySelector('[data-key="failed"]');
    if (failed) { failed.hidden = t.failed == null; legend("failed").textContent = num(t.failed ?? 0); }
    legend("received").textContent = num(t.received);
    legend("open").textContent = `${num(slots[0].open)} → ${num(slots.at(-1).open)}`;
    // tokens per second over the time the window has actually covered (its last slot is still filling)
    const seconds = Math.max(1, Math.min(Date.now() / 1000, data.to) - data.from);
    legend("tokens").textContent = `${short(t.tokens_in / seconds)}/s in · ${short(t.tokens_out / seconds)}/s out`;
    legend("tokens").title = `${num(t.tokens_in)} tokens read (cached reads included) and ${num(t.tokens_out)} written (reasoning included) by the AI runs in this period`;
  }

  // the slot's top segment gets rounded corners; the rest are square, anchored to the baseline
  function bar(x, y, w, h, round, cls) {
    const r = round ? Math.min(3, w / 2, h) : 0;
    const d = r ? `M${x},${y + h}V${y + r}Q${x},${y} ${x + r},${y}H${x + w - r}Q${x + w},${y} ${x + w},${y + r}V${y + h}Z` : `M${x},${y}h${w}v${h}h${-w}Z`;
    return svg("path", { d, class: cls });
  }

  function draw() {
    if (!data) return;
    totals();
    const slots = data.slots, n = slots.length;
    const W = Math.max(280, plot.clientWidth), TOP = 76, GAP = 14, LOW = 34, AXIS = 18, LEFT = 30;
    const H = TOP + GAP + LOW + AXIS, inner = W - LEFT, step = inner / n, bw = Math.max(1, step - 2);
    const out = svg("svg", { viewBox: `0 0 ${W} ${H}`, width: W, height: H, role: "img", "aria-label": `Submission activity, last ${data.window === "1h" ? "hour" : "24 hours"}` });
    // top strip: committed, closed and failed stacked per slot, received as a step line, one count scale
    const peak = Math.max(1, ...slots.map((s) => Math.max(s.committed + s.closed + (s.failed ?? 0), s.received)));
    const y = (v) => TOP - (v / peak) * (TOP - 4);
    for (const v of [0, peak]) {
      out.append(svg("line", { x1: LEFT, x2: W, y1: y(v), y2: y(v), class: "sx-activity-grid" }));
      const label = svg("text", { x: LEFT - 6, y: y(v) + 4, class: "sx-activity-tick", "text-anchor": "end" }); label.textContent = num(v); out.append(label);
    }
    slots.forEach((s, i) => {
      const x = LEFT + i * step + 1, c = s.committed, k = s.closed, f = s.failed ?? 0;
      if (c) out.append(bar(x, y(c), bw, TOP - y(c), !k && !f, "sx-activity-committed"));
      if (k) { const top = y(c + k), h = y(c) - top - (c ? 2 : 0); if (h > 0) out.append(bar(x, top, bw, h, !f, "sx-activity-closed")); }
      if (f) { const top = y(c + k + f), h = y(c + k) - top - (c + k ? 2 : 0); if (h > 0) out.append(bar(x, top, bw, h, true, "sx-activity-failed")); }
    });
    let line = "";
    slots.forEach((s, i) => { const x0 = LEFT + i * step, x1 = x0 + step; line += `${i ? "L" : "M"}${x0},${y(s.received)}H${x1}`; });
    out.append(svg("path", { d: line, class: "sx-activity-received" }));
    // bottom strip: cases open at each slot's end, from zero
    const base = TOP + GAP + LOW, openPeak = Math.max(1, ...slots.map((s) => s.open));
    const yo = (v) => base - (v / openPeak) * (LOW - 2);
    let area = `M${LEFT},${base}`, edge = "";
    slots.forEach((s, i) => { const x = LEFT + (i + 1) * step; area += `L${LEFT + i * step},${yo(s.open)}L${x},${yo(s.open)}`; edge += `${i ? "L" : "M"}${LEFT + i * step},${yo(s.open)}L${x},${yo(s.open)}`; });
    out.append(svg("path", { d: `${area}L${W},${base}Z`, class: "sx-activity-open-fill" }), svg("path", { d: edge, class: "sx-activity-open" }));
    out.append(svg("line", { x1: LEFT, x2: W, y1: base, y2: base, class: "sx-activity-grid" }));
    const openLabel = svg("text", { x: LEFT - 6, y: yo(openPeak) + 4, class: "sx-activity-tick", "text-anchor": "end" }); openLabel.textContent = num(openPeak); out.append(openLabel);
    // time axis: every 15 minutes or every 4 hours
    const every = data.window === "1h" ? 900 : 4 * 3600;
    for (let t = Math.ceil(data.from / every) * every; t < data.to; t += every) {
      const x = LEFT + ((t - data.from) / (data.to - data.from)) * inner;
      out.append(svg("line", { x1: x, x2: x, y1: base, y2: base + 4, class: "sx-activity-grid" }));
      const label = svg("text", { x, y: H - 3, class: "sx-activity-tick", "text-anchor": "middle" }); label.textContent = clock(t); out.append(label);
    }
    // hover: one target per slot over both strips, a marker on the hovered slot
    const marker = svg("rect", { y: 0, height: base, width: step, class: "sx-activity-hover", visibility: "hidden" });
    out.append(marker);
    slots.forEach((s, i) => {
      const hit = svg("rect", { x: LEFT + i * step, y: 0, width: step, height: base, fill: "transparent" });
      hit.addEventListener("pointerenter", () => show(s, i, step, LEFT, marker));
      out.append(hit);
    });
    out.addEventListener("pointerleave", () => { tip.hidden = true; marker.setAttribute("visibility", "hidden"); });
    plot.replaceChildren(out);
  }

  function show(s, i, step, left, marker) {
    marker.setAttribute("x", left + i * step); marker.setAttribute("visibility", "visible");
    const end = Math.min(s.at + data.step, Date.now() / 1000);
    tip.replaceChildren();
    const head = document.createElement("strong"); head.textContent = `${clock(s.at)}–${clock(end)}`; tip.append(head);
    for (const [label, value, key] of [["Committed", s.committed, "committed"], ["Closed, no report", s.closed, "closed"], ...(s.failed != null ? [["Failed", s.failed, "failed"]] : []), ["Received", s.received, "received"], ["Open at the end", s.open, "open"], ["Tokens in / out per s", `${short(s.tokens_in / data.step)} / ${short(s.tokens_out / data.step)}`, "tokens"]]) {
      const row = document.createElement("span"); row.dataset.key = key;
      row.append(Object.assign(document.createElement("i"), { ariaHidden: "true" }), document.createTextNode(`${label} `), Object.assign(document.createElement("b"), { textContent: typeof value === "number" ? num(value) : value }));
      tip.append(row);
    }
    tip.hidden = false;
    const x = left + (i + 0.5) * step, w = tip.offsetWidth, max = plot.clientWidth - w;
    tip.style.left = `${Math.max(0, Math.min(max, x - w / 2))}px`;
  }

  for (const button of root.querySelectorAll("[data-window]")) {
    button.setAttribute("aria-pressed", String(button.dataset.window === windowName));
    button.addEventListener("click", () => {
      windowName = button.dataset.window;
      try { localStorage.setItem("cet:activity-window", windowName); } catch { /* no storage */ }
      for (const b of root.querySelectorAll("[data-window]")) b.setAttribute("aria-pressed", String(b === button));
      data = null; root.dataset.state = "loading"; tip.hidden = true; load();
    });
  }
  new ResizeObserver(() => draw()).observe(plot);
  document.addEventListener("visibilitychange", () => { if (document.hidden) clearTimeout(timer); else load(); });
  load();
}
