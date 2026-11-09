// The home page's Routes view (?view=routes): the smuggling routes drawn on the home page's own map
// (WorldMap, no second map), with the years, filters, totals and the panel in the pane beside it
// (RoutesPane). The report markers are hidden while it is open and come back as they were.
//
// Kept light so it stays smooth with hundreds of lines:
// - the routes are fetched (/api/routes.json) only when the view first opens;
// - the moving dots are drawn on a plain 2D canvas over the map, a few hundred points a frame, not
//   as a map source re-sent to MapLibre's worker every frame; they stop when the view is closed,
//   the tab is hidden or the reader prefers less motion;
// - picking or pointing at a route changes one paint expression or one filter, not the state of
//   every line;
// - dragging the years redraws at most once a frame.
// What is chosen stays in the address (?view=routes&years=2020-2026&route=th-in…).
import { corridors, placeTotals, arc, along, MODES, PRODUCTS } from "../lib/routes.js";
import { formatWeight } from "../lib/figures.js";
import { versioned } from "./build-id.js";

const doc = document.documentElement;
const pane = document.getElementById("routes-pane");
const mapBox = document.querySelector(".home-map [data-map]");
const switches = document.querySelectorAll(".view-switch [data-view]");
if (pane && mapBox) start();

function start() {
  const base = mapBox.dataset.base ?? "/";
  const first = +pane.dataset.first, last = +pane.dataset.last;
  const setRange = (k, v) => timeline.style.setProperty(k, v);
  const bar = document.getElementById("routes-bar"), timeline = document.getElementById("routes-timeline");
  const $ = (s) => bar.querySelector(s) ?? timeline.querySelector(s) ?? pane.querySelector(s);
  const cols = [...timeline.querySelectorAll(".rt-col")];
  const fromInput = $('[data-year="from"]'), toInput = $('[data-year="to"]');
  const metricSel = $("[data-metric]"), productSel = $("[data-product]"), modeSel = $("[data-mode]");
  const panel = $("[data-panel]"), span = $("[data-span]"), playBtn = $("[data-play]");
  const reduced = matchMedia("(prefers-reduced-motion: reduce)").matches;
  const theme = () => (doc.dataset.theme === "light" ? "light" : "dark");
  const INK = {
    dark: { line: "#e9b25f", lineDim: "rgba(233,178,95,0.08)", hi: "#fff6e3", send: "#e9a04a", receive: "#6fa8e8", both: "#cfc6b0", halo: "#0c0c0b" },
    light: { line: "#b46a12", lineDim: "rgba(180,106,18,0.10)", hi: "#1b1a16", send: "#c7721a", receive: "#2f65a3", both: "#7d7766", halo: "#f4f2eb" },
  };
  const esc = (s = "") => String(s).replace(/[&<>"]/g, (c) => ({ "&": "&amp;", "<": "&lt;", ">": "&gt;", '"': "&quot;" })[c]);
  const fmt = (n) => Math.round(n).toLocaleString("en-GB");
  const fmtDate = (d) => new Date(`${d}T00:00:00Z`).toLocaleDateString("en-GB", { day: "numeric", month: "short", year: "numeric", timeZone: "UTC" });
  const plural = (n, one, many = `${one}s`) => `${fmt(n)} ${n === 1 ? one : many}`;
  const weight = (kg) => formatWeight(kg, 3);
  const reportsUrl = (q) => `${base}reports/?${new URLSearchParams(q)}`;

  // ---- what is shown ----
  const state = { from: first, to: last, metric: "reports", product: "", mode: "", pick: null, hover: null };
  const products = PRODUCTS, modes = MODES.map(([id, label]) => [id, label]);
  const colorOf = Object.fromEntries(products.map(([id, , c]) => [id, c]));
  let data = null;
  let shown = [], lines = [], domestic = [], byKey = new Map(), totals = {};
  const geometry = new Map();
  const place = (id) => `<span class="fl">${data.nodes[id]?.fl ?? ""}</span>${esc(data.nodes[id]?.n ?? id)}`;

  function readAddress() {
    const q = new URLSearchParams(location.search);
    const yrs = /^(\d{4})-(\d{4})$/.exec(q.get("years") ?? "");
    if (yrs) { state.from = Math.max(first, Math.min(last, +yrs[1])); state.to = Math.max(state.from, Math.min(last, +yrs[2])); }
    if (q.get("by") === "kg") state.metric = "kg";
    if (["hashish", "flower", "other"].includes(q.get("cannabis"))) state.product = q.get("cannabis");
    if (["air", "road", "sea", "mail"].includes(q.get("way"))) state.mode = q.get("way");
    const r = /^([a-z/]+)-([a-z/]+)$/.exec(q.get("route") ?? "");
    if (r) state.pick = { line: `${r[1]}>${r[2]}` };
    else if (q.get("place")) state.pick = { place: q.get("place") };
    fromInput.value = state.from; toInput.value = state.to;
    metricSel.value = state.metric; productSel.value = state.product; modeSel.value = state.mode;
  }
  function remember() {
    if (!active) return;
    const q = new URLSearchParams({ view: "routes" });
    if (state.from !== first || state.to !== last) q.set("years", `${state.from}-${state.to}`);
    if (state.metric === "kg") q.set("by", "kg");
    if (state.product) q.set("cannabis", state.product);
    if (state.mode) q.set("way", state.mode);
    if (state.pick?.line) q.set("route", state.pick.line.replace(">", "-"));
    if (state.pick?.place) q.set("place", state.pick.place);
    history.replaceState(null, "", `${location.pathname}?${q}${location.hash}`);
  }

  function compute() {
    shown = data.records.filter((x) => x.y >= state.from && x.y <= state.to && (!state.product || x.p === state.product) && (!state.mode || x.m.includes(state.mode)));
    ({ lines, domestic } = corridors(shown, data.nodes));
    byKey = new Map(lines.map((c) => [c.key, c]));
    totals = placeTotals(lines);
    for (const c of lines) if (!geometry.has(c.key)) geometry.set(c.key, arc(data.nodes[c.from].ll, data.nodes[c.to].ll));
  }
  const value = (c) => (state.metric === "kg" ? c.kg : c.reports);
  const picked = () => {
    if (state.pick?.line) return new Set(byKey.has(state.pick.line) ? [state.pick.line] : []);
    if (state.pick?.place) return new Set(lines.filter((c) => c.from === state.pick.place || c.to === state.pick.place).map((c) => c.key));
    return null;
  };

  // ---- the map: the home page's own, handed over by WorldMap on every new basemap ----
  let map = null, active = doc.dataset.homeView === "routes";
  const OWN = ["route-line", "route-hi", "route-hit", "route-places"];
  // the report page's layers, hidden while the routes are shown and put back as they were
  const OTHERS = ["reports", "focus", "cluster-count", "clusters", "accuracy-fill", "accuracy-line", "landrace-region-fill", "landrace-region-line", "landrace-area-fill", "landrace-area-line", "landrace-area-dot", "landrace-area-label", "landrace-accession", "spot-region-fill", "spot-region-line", "spot-area-fill", "spot-area-line", "spot-area-dot"];
  let saved = {};

  const routeData = () => {
    const max = Math.max(1, ...lines.map(value));
    return {
      type: "FeatureCollection",
      features: [...lines].sort((a, b) => value(a) - value(b)).map((c) => ({
        type: "Feature", properties: { key: c.key, w: Math.sqrt(value(c) / max) },
        geometry: { type: "LineString", coordinates: geometry.get(c.key) },
      })),
    };
  };
  const placeData = () => {
    const max = Math.max(1, ...Object.values(totals).map((t) => t.in + t.out));
    return {
      type: "FeatureCollection",
      features: Object.entries(totals).map(([id, t]) => ({
        type: "Feature",
        properties: { id, r: 3 + 9 * Math.sqrt((t.in + t.out) / max), kind: t.out > t.in * 1.5 ? "send" : t.in > t.out * 1.5 ? "receive" : "both" },
        geometry: { type: "Point", coordinates: data.nodes[id].ll },
      })),
    };
  };
  const EMPTY = { type: "FeatureCollection", features: [] };

  function addLayers() {
    if (!map || map.getSource("routes")) return;
    const c = INK[theme()];
    map.addSource("routes", { type: "geojson", data: data ? routeData() : EMPTY, lineMetrics: true });
    map.addSource("route-places", { type: "geojson", data: data ? placeData() : EMPTY });
    const width = (lo, hi) => ["interpolate", ["linear"], ["zoom"], 1, ["+", lo, ["*", ["get", "w"], hi]], 5, ["+", lo * 1.6, ["*", ["get", "w"], hi * 1.8]]];
    const vis = { visibility: active ? "visible" : "none" };
    map.addLayer({ id: "route-line", type: "line", source: "routes", layout: { "line-cap": "round", ...vis },
      paint: { "line-width": width(0.6, 5), "line-gradient": ["interpolate", ["linear"], ["line-progress"], 0, c.lineDim, 0.6, c.line, 1, c.line], "line-opacity": BASE_OPACITY } });
    map.addLayer({ id: "route-hi", type: "line", source: "routes", filter: ["in", ["get", "key"], ["literal", []]], layout: { "line-cap": "round", ...vis },
      paint: { "line-color": c.hi, "line-width": width(1.2, 3), "line-opacity": 0.9 } });
    map.addLayer({ id: "route-hit", type: "line", source: "routes", layout: vis, paint: { "line-color": "#000", "line-opacity": 0, "line-width": 14 } });
    map.addLayer({ id: "route-places", type: "circle", source: "route-places", layout: vis,
      paint: { "circle-radius": ["get", "r"], "circle-color": ["match", ["get", "kind"], "send", c.send, "receive", c.receive, c.both],
        "circle-opacity": 0.85, "circle-stroke-color": c.halo, "circle-stroke-width": 1.2 } });
    paintPick();
  }
  const BASE_OPACITY = ["+", 0.35, ["*", 0.6, ["get", "w"]]];

  // which layers show: the routes or the reports
  function showLayers() {
    if (!map || !map.getLayer("route-line")) return;
    for (const id of OWN) map.setLayoutProperty(id, "visibility", active ? "visible" : "none");
    for (const id of OTHERS) {
      if (!map.getLayer(id)) continue;
      if (active) {
        if (!(id in saved)) saved[id] = map.getLayoutProperty(id, "visibility") ?? "visible";
        map.setLayoutProperty(id, "visibility", "none");
      } else if (id in saved) map.setLayoutProperty(id, "visibility", saved[id]);
    }
    if (!active) saved = {};
  }

  // the picked route(s), and the one pointed at, bright; the others dim
  function paintPick() {
    if (!map?.getLayer("route-line")) return;
    const only = picked();
    if (only) {
      map.setPaintProperty("route-line", "line-opacity", ["case", ["in", ["get", "key"], ["literal", [...only]]], 0.95, 0.08]);
      const on = new Set(lines.filter((c) => only.has(c.key)).flatMap((c) => [c.from, c.to]));
      map.setPaintProperty("route-places", "circle-opacity", ["case", ["in", ["get", "id"], ["literal", [...on]]], 0.9, 0.2]);
    } else {
      map.setPaintProperty("route-line", "line-opacity", BASE_OPACITY);
      map.setPaintProperty("route-places", "circle-opacity", 0.85);
    }
    paintHover();
  }
  let lastBright = "";
  function paintHover() {
    if (!map?.getLayer("route-hi")) return;
    const bright = [...(picked() ?? []), ...(state.hover && byKey.has(state.hover) ? [state.hover] : [])];
    const key = bright.join("|");
    if (key === lastBright) return;
    lastBright = key;
    map.setFilter("route-hi", ["in", ["get", "key"], ["literal", bright]]);
  }

  // ---- the moving dots, on a canvas of their own ----
  let canvas = null, ctx = null, particles = [], frameId = 0, dpr = 1;
  function seed() {
    const max = Math.max(1, ...lines.map(value));
    particles = lines.flatMap((c) => {
      const line = geometry.get(c.key), n = 1 + Math.round(3 * Math.sqrt(value(c) / max));
      let len = 0;
      for (let i = 1; i < line.length; i++) len += Math.hypot(line[i][0] - line[i - 1][0], line[i][1] - line[i - 1][1]);
      const mix = Object.entries(c.products).sort((a, b) => b[1] - a[1]).flatMap(([p, k]) => Array(k).fill(colorOf[p]));
      return Array.from({ length: n }, (_, i) => ({ key: c.key, line, len: Math.max(len, 4), offset: i / n, color: mix[Math.floor((i / n) * mix.length)] ?? colorOf[""] }));
    });
  }
  function sizeCanvas() {
    if (!canvas) return;
    const gl = map.getCanvas();
    dpr = Math.min(2, devicePixelRatio || 1);
    canvas.width = Math.round(gl.clientWidth * dpr); canvas.height = Math.round(gl.clientHeight * dpr);
    canvas.style.width = `${gl.clientWidth}px`; canvas.style.height = `${gl.clientHeight}px`;
  }
  function draw(now) {
    if (!ctx) return;
    ctx.setTransform(dpr, 0, 0, dpr, 0, 0);
    ctx.clearRect(0, 0, canvas.width, canvas.height);
    if (!active) return;
    const only = picked(), w = canvas.width / dpr, h = canvas.height / dpr;
    const r = Math.max(2, Math.min(4.2, 1.6 + map.getZoom() * 0.55));
    const groups = new Map();
    for (const p of particles) {
      if (only && !only.has(p.key)) continue;
      const t = reduced ? 0.5 + p.offset / 4 : ((now / 1000) * 9 / p.len + p.offset) % 1;
      const pt = map.project(along(p.line, t));
      if (pt.x < -5 || pt.y < -5 || pt.x > w + 5 || pt.y > h + 5) continue;
      let g = groups.get(p.color);
      if (!g) groups.set(p.color, (g = []));
      g.push(pt.x, pt.y);
    }
    ctx.lineWidth = 1;
    ctx.strokeStyle = INK[theme()].halo;
    for (const [color, pts] of groups) {
      ctx.beginPath();
      for (let i = 0; i < pts.length; i += 2) { ctx.moveTo(pts[i] + r, pts[i + 1]); ctx.arc(pts[i], pts[i + 1], r, 0, Math.PI * 2); }
      ctx.fillStyle = color;
      ctx.fill();
      ctx.stroke();
    }
  }
  const tick = (now) => {
    frameId = 0;
    if (!active || document.hidden) return;
    draw(now);
    frameId = requestAnimationFrame(tick);
  };
  function animate() {
    if (!map || !ctx) return;
    if (reduced) { draw(0); return; }
    if (active && !frameId) frameId = requestAnimationFrame(tick);
  }
  document.addEventListener("visibilitychange", animate);

  // ---- the panel ----
  const barOf = (n, max) => `<span class="rp-bar"><i style="width:${Math.max(2, (n / Math.max(1, max)) * 100)}%"></i></span>`;
  const metricText = (c) => (state.metric === "kg" ? (c.kg ? weight(c.kg) : "–") : fmt(c.reports));
  const lineRow = (c, max, label = `${place(c.from)} <span class="arrow">→</span> ${place(c.to)}`) =>
    `<li><button type="button" class="rp-row" data-line="${esc(c.key)}"><span class="rp-name">${label}</span>${barOf(value(c), max)}<span class="rp-num">${metricText(c)}</span></button></li>`;
  const yearsText = () => (state.from === state.to ? `in ${state.from}` : `${state.from}–${state.to}`);
  const yearBars = (list) => {
    const counts = Array.from({ length: last - first + 1 }, (_, i) => list.filter((x) => x.y === first + i).length), max = Math.max(1, ...counts);
    return `<div class="rp-years" aria-hidden="true">${counts.map((n, i) => `<i class="${first + i >= state.from && first + i <= state.to ? "in" : ""}" style="height:${n ? Math.max(8, (n / max) * 100) : 0}%" title="${first + i}: ${n}"></i>`).join("")}</div><div class="rp-years-axis"><span>${first}</span><span>${last}</span></div>`;
  };
  const counted = (obj, labels) => Object.entries(obj).filter(([k]) => labels.some(([id]) => id === k)).sort((a, b) => b[1] - a[1])
    .map(([k, n]) => `<li><span>${esc(labels.find(([id]) => id === k)[1])}</span><b>${fmt(n)}</b></li>`).join("");

  function renderTiles() {
    const set = (k, v) => { pane.querySelector(`[data-t="${k}"]`).textContent = v; };
    const kg = shown.reduce((n, r) => n + (r.kg ?? 0), 0);
    set("reports", fmt(shown.length)); set("routes", fmt(lines.length));
    set("places", fmt(Object.keys(totals).length)); set("kg", kg ? weight(kg) : "–");
  }
  function renderPanel() {
    if (state.pick?.line) return renderLine(state.pick.line);
    if (state.pick?.place) return renderPlace(state.pick.place);
    const top = [...lines].sort((a, b) => value(b) - value(a) || b.reports - a.reports).slice(0, 15);
    const max = Math.max(1, ...top.map(value));
    const home = domestic.slice(0, 6);
    panel.innerHTML = `
      <h2>Busiest routes <span class="rp-kicker">${yearsText()}${state.metric === "kg" ? " · by kilograms" : ""}</span></h2>
      ${top.length ? `<ol class="rp-list">${top.map((c) => lineRow(c, max)).join("")}</ol>` : `<p class="muted">No routes match these years and filters.</p>`}
      <p class="rp-hint">Pick a line or a country on the map to see its reports.</p>
      ${home.length ? `<h3>Within one country</h3><ul class="rp-counts">${home.map((c) => `<li><a href="${reportsUrl({ from: c.from, to: c.to })}">${place(c.from)}</a><b>${fmt(c.reports)}</b></li>`).join("")}</ul>` : ""}`;
  }
  function renderLine(key) {
    const [from, to] = key.split(">");
    const back = `<button type="button" class="rp-back" data-clear>← All routes</button>`;
    if (!data.nodes[from] || !data.nodes[to]) { state.pick = null; return renderPanel(); }
    const c = byKey.get(key);
    if (!c) { panel.innerHTML = `${back}<h2>${place(from)} <span class="arrow">→</span> ${place(to)}</h2><p class="muted">No reports on this route ${yearsText()} with these filters.</p>`; return; }
    const list = shown.filter((x) => x.f.includes(from) && x.t.includes(to));
    const all = data.records.filter((x) => x.f.includes(from) && x.t.includes(to));
    const other = byKey.get(`${to}>${from}`);
    const crossings = [...new Set(list.map((x) => x.x).filter(Boolean))];
    panel.innerHTML = `
      ${back}
      <p class="rp-kicker">Route · ${yearsText()}</p>
      <h2>${place(from)} <span class="arrow">→</span> ${place(to)}</h2>
      <dl class="rp-facts">
        <div><dt>reports</dt><dd>${fmt(c.reports)}</dd></div>
        <div><dt>seized</dt><dd>${c.kg ? weight(c.kg) : "–"}</dd></div>
        <div><dt>years</dt><dd>${c.first === c.last ? c.first : `${c.first}–${String(c.last).slice(2)}`}</dd></div>
      </dl>
      ${yearBars(all)}
      ${Object.keys(c.modes).length ? `<h3>How it travelled</h3><ul class="rp-counts">${counted(c.modes, modes)}</ul>` : ""}
      ${Object.keys(c.products).some((k) => k) ? `<h3>What was carried</h3><ul class="rp-counts">${counted(c.products, products.filter(([id]) => id))}</ul>` : ""}
      ${crossings.length ? `<h3>Crossings named</h3><p class="rp-crossings">${crossings.map(esc).join(" · ")}</p>` : ""}
      <h3>Latest reports</h3>
      <ol class="rp-reports">${list.slice(0, 6).map((x) => `<li><a href="${base}${esc(x.u)}"><time>${fmtDate(x.d)}</time>${esc(x.ti)}</a>${x.kg ? `<span class="rp-kg">${weight(x.kg)}</span>` : ""}</li>`).join("")}</ol>
      <a class="rp-more" href="${reportsUrl({ from, to })}">All ${plural(all.length, "report")} on this route →</a>
      ${other ? `<p class="rp-also">The other way: <button type="button" class="rp-link" data-line="${esc(other.key)}">${place(to)} → ${place(from)}</button> (${plural(other.reports, "report")})</p>` : ""}
      <p class="rp-also"><button type="button" class="rp-link" data-place="${esc(from)}">All routes of ${esc(data.nodes[from].n)}</button> · <button type="button" class="rp-link" data-place="${esc(to)}">of ${esc(data.nodes[to].n)}</button></p>`;
  }
  function renderPlace(id) {
    if (!data.nodes[id]) { state.pick = null; return renderPanel(); }
    const out = lines.filter((c) => c.from === id), inn = lines.filter((c) => c.to === id);
    const max = Math.max(1, ...[...out, ...inn].map(value));
    const sum = (l) => l.reduce((n, c) => n + c.reports, 0);
    panel.innerHTML = `
      <button type="button" class="rp-back" data-clear>← All routes</button>
      <p class="rp-kicker">Country · ${yearsText()}</p>
      <h2>${place(id)}</h2>
      <dl class="rp-facts">
        <div><dt>reports out</dt><dd>${fmt(sum(out))}</dd></div>
        <div><dt>reports in</dt><dd>${fmt(sum(inn))}</dd></div>
        <div><dt>partners</dt><dd>${fmt(new Set([...out.map((c) => c.to), ...inn.map((c) => c.from)]).size)}</dd></div>
      </dl>
      ${out.length ? `<h3>Going out</h3><ol class="rp-list">${out.slice(0, 10).map((c) => lineRow(c, max, `<span class="arrow">→</span> ${place(c.to)}`)).join("")}</ol>
        <a class="rp-more" href="${reportsUrl({ from: id })}">All reports from ${esc(data.nodes[id].n)} →</a>` : ""}
      ${inn.length ? `<h3>Coming in</h3><ol class="rp-list">${inn.slice(0, 10).map((c) => lineRow(c, max, `${place(c.from)} <span class="arrow">→</span>`)).join("")}</ol>
        <a class="rp-more" href="${reportsUrl({ to: id })}">All reports to ${esc(data.nodes[id].n)} →</a>` : ""}
      ${!out.length && !inn.length ? `<p class="muted">No routes ${yearsText()} with these filters.</p>` : ""}`;
  }

  // ---- applying a change: at most once a frame ----
  let pending = 0, refit = false;
  function refresh({ fit = false } = {}) {
    refit ||= fit;
    if (pending || !data) return;
    pending = requestAnimationFrame(() => {
      pending = 0;
      compute(); seed();
      span.textContent = state.from === state.to ? String(state.from) : `${state.from}–${state.to}`;
      for (const c of cols) c.classList.toggle("out", +c.dataset.year < state.from || +c.dataset.year > state.to);
      setRange("--from", String((state.from - first) / Math.max(1, last - first)));
      setRange("--to", String((state.to - first) / Math.max(1, last - first)));
      if (map?.getSource("routes")) {
        map.getSource("routes").setData(routeData());
        map.getSource("route-places").setData(placeData());
        lastBright = "";
        paintPick();
      }
      renderTiles(); renderPanel(); remember();
      if (reduced) draw(0);
      if (refit) { refit = false; frame(); }
    });
  }
  function frame() {
    const only = picked();
    if (!map || !only?.size) return;
    const pts = [...only].flatMap((k) => geometry.get(k) ?? []);
    const xs = pts.map((p) => p[0]), ys = pts.map((p) => p[1]);
    map.fitBounds([[Math.min(...xs), Math.min(...ys)], [Math.max(...xs), Math.max(...ys)]], { padding: 70, maxZoom: 5, duration: reduced ? 0 : 900 });
  }
  const choose = (pick) => {
    state.pick = pick; state.hover = null; tip?.remove();
    renderPanel(); pane.parentElement.scrollTop = 0;
    paintPick(); remember(); frame();
    if (reduced) draw(0);
  };

  panel.addEventListener("click", (e) => {
    const t = e.target.closest("[data-line],[data-place],[data-clear]");
    if (!t) return;
    if (t.dataset.line) choose({ line: t.dataset.line });
    else if (t.dataset.place) choose({ place: t.dataset.place });
    else choose(null);
  });
  panel.addEventListener("pointerover", (e) => { const t = e.target.closest("[data-line]"); state.hover = t?.dataset.line ?? null; paintHover(); });
  panel.addEventListener("pointerleave", () => { state.hover = null; paintHover(); });
  addEventListener("keydown", (e) => { if (e.key === "Escape" && active && state.pick) choose(null); });

  const setYears = (which) => {
    let a = +fromInput.value, b = +toInput.value;
    if (a > b) { if (which === "from") b = a; else a = b; fromInput.value = a; toInput.value = b; }
    state.from = a; state.to = b; refresh();
  };
  fromInput.addEventListener("input", () => { stop(); setYears("from"); });
  toInput.addEventListener("input", () => { stop(); setYears("to"); });
  timeline.addEventListener("click", (e) => {
    const col = e.target.closest(".rt-col");
    if (!col) return;
    stop();
    const y = +col.dataset.year;
    if (state.from === y && state.to === y) { state.from = first; state.to = last; }
    else { state.from = y; state.to = y; }
    fromInput.value = state.from; toInput.value = state.to;
    refresh();
  });
  metricSel.addEventListener("change", () => { state.metric = metricSel.value; refresh(); });
  productSel.addEventListener("change", () => { state.product = productSel.value; refresh(); });
  modeSel.addEventListener("change", () => { state.mode = modeSel.value; refresh(); });
  let timer = 0;
  function stop() { clearInterval(timer); timer = 0; playBtn.classList.remove("on"); playBtn.firstElementChild.textContent = "▶"; playBtn.setAttribute("aria-label", "Play the years one by one"); }
  playBtn.addEventListener("click", () => {
    if (timer) return stop();
    playBtn.classList.add("on"); playBtn.firstElementChild.textContent = "❚❚"; playBtn.setAttribute("aria-label", "Pause");
    if (state.to >= last) state.to = state.from;
    toInput.value = state.to; refresh();
    timer = setInterval(() => {
      if (state.to >= last || !active) return stop();
      state.to++; toInput.value = state.to; refresh();
    }, 750);
  });

  // ---- pointing and clicking on the map ----
  let tip = null;
  function wireMap(maplibregl) {
    tip = new maplibregl.Popup({ closeButton: false, closeOnClick: false, offset: 10, className: "routes-tip", maxWidth: "260px" });
    let hoverKey = "";
    map.on("mousemove", (e) => {
      if (!active || !map.getLayer("route-hit")) return;
      const hit = map.queryRenderedFeatures([[e.point.x - 3, e.point.y - 3], [e.point.x + 3, e.point.y + 3]], { layers: ["route-places", "route-hit"] });
      const pl = hit.find((f) => f.layer.id === "route-places"), ln = hit.find((f) => f.layer.id === "route-hit");
      const key = pl ? `p:${pl.properties.id}` : ln ? `l:${ln.properties.key}` : "";
      map.getCanvas().style.cursor = key ? "pointer" : "";
      if (key === hoverKey) { if (ln && !pl) tip.setLngLat(e.lngLat); return; }
      hoverKey = key;
      state.hover = ln && !pl ? ln.properties.key : null;
      paintHover();
      if (pl) {
        const id = pl.properties.id, t = totals[id];
        tip.setLngLat(data.nodes[id].ll).setHTML(`<strong>${place(id)}</strong><span>${fmt(t?.out ?? 0)} out · ${fmt(t?.in ?? 0)} in</span>`).addTo(map);
      } else if (ln) {
        const c = byKey.get(ln.properties.key);
        if (c) tip.setLngLat(e.lngLat).setHTML(`<strong>${place(c.from)} → ${place(c.to)}</strong><span>${plural(c.reports, "report")}${c.kg ? ` · ${weight(c.kg)} seized` : ""}</span><span class="muted">${c.first === c.last ? c.first : `${c.first}–${c.last}`}</span>`).addTo(map);
      } else tip.remove();
    });
    map.getCanvas().addEventListener("mouseleave", () => { hoverKey = ""; state.hover = null; paintHover(); tip.remove(); });
    map.on("click", (e) => {
      if (!active || !map.getLayer("route-hit")) return;
      const hit = map.queryRenderedFeatures([[e.point.x - 3, e.point.y - 3], [e.point.x + 3, e.point.y + 3]], { layers: ["route-places", "route-hit"] });
      const pl = hit.find((f) => f.layer.id === "route-places"), ln = hit.find((f) => f.layer.id === "route-hit");
      if (pl) choose({ place: pl.properties.id });
      else if (ln) choose({ line: ln.properties.key });
      else if (state.pick) choose(null);
    });
    map.on("resize", () => { sizeCanvas(); if (reduced) draw(0); });
    if (reduced) map.on("move", () => draw(0));
  }

  let wired = false;
  const onBuilt = async (built) => {
    map = built;
    if (!wired) {
      wired = true;
      wireMap(await import("maplibre-gl"));
      canvas = document.createElement("canvas");
      canvas.className = "routes-dots";
      canvas.setAttribute("aria-hidden", "true");
      map.getCanvasContainer().appendChild(canvas);
      ctx = canvas.getContext("2d");
      sizeCanvas();
    }
    saved = {}; // a new basemap: the report layers are new too
    if (active) await load();
    addLayers();
    showLayers();
    animate();
  };
  mapBox.addEventListener("map:built", (e) => onBuilt(e.detail));
  if (mapBox.__map) onBuilt(mapBox.__map);

  // ---- loading the routes, once ----
  let loading = null;
  function load() {
    loading ??= fetch(versioned(`${base}api/routes.json`)).then((r) => r.json()).then((json) => {
      data = json;
      readAddress();
      if (state.pick?.line && !state.pick.line.split(">").every((id) => data.nodes[id])) state.pick = null;
      compute(); seed();
      refresh({ fit: true });
    }).catch(() => { panel.innerHTML = `<p class="muted">The routes could not be loaded. Please reload to try again.</p>`; });
    return loading;
  }

  // ---- the switch between the two views ----
  let reportsQuery = active ? "" : location.search;
  async function setView(view) {
    const on = view === "routes";
    if (on === active) return;
    if (on) reportsQuery = location.search;
    active = on;
    doc.dataset.homeView = view;
    for (const b of switches) b.setAttribute("aria-pressed", String(b.dataset.view === view));
    if (on) {
      await load();
      addLayers();
      showLayers();
      refresh();
      sizeCanvas();
      animate();
    } else {
      stop();
      tip?.remove();
      showLayers();
      draw(0);
      history.replaceState(null, "", `${location.pathname}${reportsQuery}${location.hash}`);
    }
    map?.resize();
  }
  for (const b of switches) {
    b.setAttribute("aria-pressed", String(b.dataset.view === (active ? "routes" : "reports")));
    b.addEventListener("click", () => setView(b.dataset.view));
  }
  if (active) load();
}
