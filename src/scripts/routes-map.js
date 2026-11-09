// The Routes page (src/pages/routes.astro): the corridors as curved lines on a MapLibre map, dots
// running along them in the direction of travel, the years and filters above, and the panel beside
// it (the busiest routes; a route's reports; a country's routes in and out). What is chosen is kept
// in the address (?years=2020-2026&route=ma-es, ?place=th…), so a view can be shared.
import * as maplibregl from "maplibre-gl";
import "./maplibre-worker.js";
import "maplibre-gl/dist/maplibre-gl.css";
import { corridors, placeTotals, arc, along } from "../lib/routes.js";
import { formatWeight } from "../lib/figures.js";

const box = document.querySelector("[data-routes-map]");
const root = document.getElementById("routes");
const data = JSON.parse(document.getElementById("routes-data")?.textContent ?? "null");
if (box && root && data) start();

function start() {
  const { records, nodes, first, last, styles, base, products, modes } = data;
  const $ = (s) => root.querySelector(s);
  const fromInput = $('[data-year="from"]'), toInput = $('[data-year="to"]');
  const metricSel = $("[data-metric]"), productSel = $("[data-product]"), modeSel = $("[data-mode]");
  const panel = $("[data-panel]"), span = $("[data-span]"), playBtn = $("[data-play]"), empty = box.querySelector("[data-empty]");
  const reduced = matchMedia("(prefers-reduced-motion: reduce)").matches;
  const theme = () => (document.documentElement.dataset.theme === "light" ? "light" : "dark");
  const INK = {
    dark: { line: "#e9b25f", lineDim: "rgba(233,178,95,0.08)", hi: "#fff6e3", send: "#e9a04a", receive: "#6fa8e8", both: "#cfc6b0", halo: "#0c0c0b" },
    light: { line: "#b46a12", lineDim: "rgba(180,106,18,0.10)", hi: "#1b1a16", send: "#c7721a", receive: "#2f65a3", both: "#7d7766", halo: "#f4f2eb" },
  };
  const colorOf = Object.fromEntries(products.map(([id, , c]) => [id, c]));
  const esc = (s = "") => String(s).replace(/[&<>"]/g, (c) => ({ "&": "&amp;", "<": "&lt;", ">": "&gt;", '"': "&quot;" })[c]);
  const fmt = (n) => Math.round(n).toLocaleString("en-GB");
  const fmtDate = (d) => new Date(`${d}T00:00:00Z`).toLocaleDateString("en-GB", { day: "numeric", month: "short", year: "numeric", timeZone: "UTC" });
  const plural = (n, one, many = `${one}s`) => `${fmt(n)} ${n === 1 ? one : many}`;
  const weight = (kg) => formatWeight(kg, 3);
  const place = (id) => `<span class="fl">${nodes[id]?.fl ?? ""}</span>${esc(nodes[id]?.n ?? id)}`;
  const reportsUrl = (q) => `${base}reports/?${new URLSearchParams(q)}`;

  // what is shown: the years, the filters, and the route or country picked
  const params = new URLSearchParams(location.search);
  const state = { from: first, to: last, metric: "reports", product: "", mode: "", pick: null, hover: null };
  const yrs = /^(\d{4})-(\d{4})$/.exec(params.get("years") ?? "");
  if (yrs) { state.from = Math.max(first, Math.min(last, +yrs[1])); state.to = Math.max(state.from, Math.min(last, +yrs[2])); }
  if (params.get("by") === "kg") state.metric = "kg";
  if (products.some(([id]) => id && id === params.get("cannabis"))) state.product = params.get("cannabis");
  if (modes.some(([id]) => id === params.get("way"))) state.mode = params.get("way");
  const r = /^([a-z/]+)-([a-z/]+)$/.exec(params.get("route") ?? "");
  if (r && nodes[r[1]] && nodes[r[2]]) state.pick = { line: `${r[1]}>${r[2]}` };
  else if (nodes[params.get("place") ?? ""]) state.pick = { place: params.get("place") };
  fromInput.value = state.from; toInput.value = state.to;
  metricSel.value = state.metric; productSel.value = state.product; modeSel.value = state.mode;

  const remember = () => {
    const q = new URLSearchParams();
    if (state.from !== first || state.to !== last) q.set("years", `${state.from}-${state.to}`);
    if (state.metric === "kg") q.set("by", "kg");
    if (state.product) q.set("cannabis", state.product);
    if (state.mode) q.set("way", state.mode);
    if (state.pick?.line) q.set("route", state.pick.line.replace(">", "-"));
    if (state.pick?.place) q.set("place", state.pick.place);
    const s = q.toString();
    history.replaceState(null, "", `${location.pathname}${s ? `?${s}` : ""}`);
  };

  // the records and corridors the years and filters let through
  let shown = [], lines = [], byKey = new Map(), totals = {}, geometry = new Map();
  function compute() {
    shown = records.filter((x) => x.y >= state.from && x.y <= state.to && (!state.product || x.p === state.product) && (!state.mode || x.m.includes(state.mode)));
    lines = corridors(shown, nodes).lines;
    byKey = new Map(lines.map((c) => [c.key, c]));
    totals = placeTotals(lines);
    for (const c of lines) if (!geometry.has(c.key)) geometry.set(c.key, arc(nodes[c.from].ll, nodes[c.to].ll));
  }
  const value = (c) => (state.metric === "kg" ? c.kg : c.reports);
  const picked = () => {
    if (state.pick?.line) return new Set(byKey.has(state.pick.line) ? [state.pick.line] : []);
    if (state.pick?.place) return new Set(lines.filter((c) => c.from === state.pick.place || c.to === state.pick.place).map((c) => c.key));
    return null;
  };

  const routeData = () => {
    const max = Math.max(1, ...lines.map(value));
    return {
      type: "FeatureCollection",
      // the thin ones first, so the thick ones are drawn over them
      features: [...lines].sort((a, b) => value(a) - value(b)).map((c) => ({
        type: "Feature", id: undefined,
        properties: { key: c.key, w: Math.sqrt(value(c) / max) },
        geometry: { type: "LineString", coordinates: geometry.get(c.key) },
      })),
    };
  };
  const nodeData = () => {
    const max = Math.max(1, ...Object.values(totals).map((t) => t.in + t.out));
    return {
      type: "FeatureCollection",
      features: Object.entries(totals).map(([id, t]) => ({
        type: "Feature",
        properties: { id, r: 3 + 9 * Math.sqrt((t.in + t.out) / max), kind: t.out > t.in * 1.5 ? "send" : t.in > t.out * 1.5 ? "receive" : "both" },
        geometry: { type: "Point", coordinates: nodes[id].ll },
      })),
    };
  };

  // the dots: a few per line (more on the busy ones), each the colour of what one of its reports
  // carried, moving at the same speed on every line, so a long route takes longer to cross
  let particles = [];
  function seed() {
    const max = Math.max(1, ...lines.map(value));
    particles = lines.flatMap((c) => {
      const line = geometry.get(c.key), n = 1 + Math.round(4 * Math.sqrt(value(c) / max));
      let len = 0;
      for (let i = 1; i < line.length; i++) len += Math.hypot(line[i][0] - line[i - 1][0], line[i][1] - line[i - 1][1]);
      const mix = Object.entries(c.products).sort((a, b) => b[1] - a[1]).flatMap(([p, k]) => Array(k).fill(colorOf[p]));
      return Array.from({ length: n }, (_, i) => ({ key: c.key, line, len: Math.max(len, 4), offset: i / n, color: mix[Math.floor((i / n) * mix.length)] ?? colorOf[""] }));
    });
  }
  const dotData = (now) => {
    const only = picked();
    return {
      type: "FeatureCollection",
      features: particles.filter((p) => !only || only.has(p.key)).map((p) => ({
        type: "Feature",
        properties: { c: p.color },
        geometry: { type: "Point", coordinates: along(p.line, reduced ? 0.5 + p.offset / 4 : ((now / 1000) * 9 / p.len + p.offset) % 1) },
      })),
    };
  };

  const map = new maplibregl.Map({
    container: box, style: styles[theme()], bounds: [[-125, -42], [150, 62]], minZoom: 0.5, maxZoom: 8, fadeDuration: 0,
    renderWorldCopies: true, attributionControl: { compact: true }, cooperativeGestures: matchMedia("(pointer: coarse)").matches,
  });
  map.addControl(new maplibregl.NavigationControl({ showCompass: false }), "top-right");
  const tip = new maplibregl.Popup({ closeButton: false, closeOnClick: false, offset: 10, className: "routes-tip", maxWidth: "260px" });

  function build() {
    const c = INK[theme()];
    map.addSource("routes", { type: "geojson", data: routeData(), lineMetrics: true, promoteId: "key" });
    map.addSource("places", { type: "geojson", data: nodeData(), promoteId: "id" });
    map.addSource("dots", { type: "geojson", data: dotData(performance.now()) });
    const width = (lo, hi) => ["interpolate", ["linear"], ["zoom"], 1, ["+", lo, ["*", ["get", "w"], hi]], 5, ["+", lo * 1.6, ["*", ["get", "w"], hi * 1.8]]];
    const dim = ["boolean", ["feature-state", "dim"], false];
    map.addLayer({ id: "route-glow", type: "line", source: "routes", layout: { "line-cap": "round" },
      paint: { "line-color": c.line, "line-width": width(3, 10), "line-blur": 6, "line-opacity": ["case", dim, 0, ["*", 0.3, ["get", "w"]]] } });
    // each line fades in from where it starts, so the direction shows even without the dots
    map.addLayer({ id: "route-line", type: "line", source: "routes", layout: { "line-cap": "round" },
      paint: { "line-width": width(0.6, 5), "line-gradient": ["interpolate", ["linear"], ["line-progress"], 0, c.lineDim, 0.6, c.line, 1, c.line],
        "line-opacity": ["case", dim, 0.1, ["+", 0.35, ["*", 0.6, ["get", "w"]]]] } });
    map.addLayer({ id: "route-hi", type: "line", source: "routes", filter: ["in", ["get", "key"], ["literal", []]], layout: { "line-cap": "round" },
      paint: { "line-color": c.hi, "line-width": width(1.2, 3), "line-opacity": 0.9 } });
    map.addLayer({ id: "route-hit", type: "line", source: "routes", paint: { "line-color": "#000", "line-opacity": 0, "line-width": 14 } });
    map.addLayer({ id: "route-dots", type: "circle", source: "dots",
      paint: { "circle-radius": ["interpolate", ["linear"], ["zoom"], 1, 2.6, 5, 4.2], "circle-color": ["get", "c"], "circle-stroke-color": c.halo, "circle-stroke-width": 0.8 } });
    map.addLayer({ id: "place-dots", type: "circle", source: "places",
      paint: { "circle-radius": ["get", "r"], "circle-color": ["match", ["get", "kind"], "send", c.send, "receive", c.receive, c.both],
        "circle-opacity": ["case", ["boolean", ["feature-state", "dim"], false], 0.25, 0.85], "circle-stroke-color": c.halo, "circle-stroke-width": 1.2 } });
    paintState();
  }

  // the picked or pointed-at route(s) drawn bright over the others, which dim
  let dimmed = { routes: new Set(), places: new Set() };
  function paintState() {
    if (!map.getSource("routes")) return;
    const only = picked();
    const hover = state.hover && byKey.has(state.hover) ? state.hover : null;
    const bright = new Set([...(only ?? []), ...(hover ? [hover] : [])]);
    map.setFilter("route-hi", ["in", ["get", "key"], ["literal", [...bright]]]);
    const dimRoutes = new Set(only ? lines.filter((c) => !only.has(c.key)).map((c) => c.key) : []);
    const onPlaces = only ? new Set(lines.filter((c) => only.has(c.key)).flatMap((c) => [c.from, c.to])) : null;
    const dimPlaces = new Set(onPlaces ? Object.keys(totals).filter((id) => !onPlaces.has(id)) : []);
    for (const k of dimmed.routes) if (!dimRoutes.has(k)) map.setFeatureState({ source: "routes", id: k }, { dim: false });
    for (const k of dimRoutes) map.setFeatureState({ source: "routes", id: k }, { dim: true });
    for (const k of dimmed.places) if (!dimPlaces.has(k)) map.setFeatureState({ source: "places", id: k }, { dim: false });
    for (const k of dimPlaces) map.setFeatureState({ source: "places", id: k }, { dim: true });
    dimmed = { routes: dimRoutes, places: dimPlaces };
  }

  function refresh({ fit = false } = {}) {
    compute();
    seed();
    span.textContent = state.from === state.to ? String(state.from) : `${state.from}–${state.to}`;
    root.style.setProperty("--from", String((state.from - first) / Math.max(1, last - first)));
    root.style.setProperty("--to", String((state.to - first) / Math.max(1, last - first)));
    empty.hidden = lines.length > 0;
    if (map.getSource("routes")) {
      map.removeFeatureState({ source: "routes" });
      map.removeFeatureState({ source: "places" });
      dimmed = { routes: new Set(), places: new Set() };
      map.getSource("routes").setData(routeData());
      map.getSource("places").setData(nodeData());
      paintState();
    }
    renderPanel();
    remember();
    if (fit) frame();
  }

  // the map moves to what was picked
  function frame() {
    const only = picked();
    if (!only?.size) return;
    const pts = [...only].flatMap((k) => geometry.get(k) ?? []);
    if (!pts.length) return;
    const xs = pts.map((p) => p[0]), ys = pts.map((p) => p[1]);
    map.fitBounds([[Math.min(...xs), Math.min(...ys)], [Math.max(...xs), Math.max(...ys)]], { padding: 70, maxZoom: 5, duration: reduced ? 0 : 900 });
  }

  // ---- the panel ----
  const bar = (n, max) => `<span class="rp-bar"><i style="width:${Math.max(2, (n / Math.max(1, max)) * 100)}%"></i></span>`;
  const metricText = (c) => (state.metric === "kg" ? (c.kg ? weight(c.kg) : "–") : fmt(c.reports));
  const lineRow = (c, max, label = `${place(c.from)} <span class="arrow">→</span> ${place(c.to)}`) =>
    `<li><button type="button" class="rp-row" data-line="${esc(c.key)}"><span class="rp-name">${label}</span>${bar(value(c), max)}<span class="rp-num">${metricText(c)}</span></button></li>`;
  const yearsText = () => (state.from === state.to ? `in ${state.from}` : `${state.from}–${state.to}`);
  const yearBars = (list) => {
    const counts = Array.from({ length: last - first + 1 }, (_, i) => list.filter((x) => x.y === first + i).length), max = Math.max(1, ...counts);
    return `<div class="rp-years" aria-hidden="true">${counts.map((n, i) => `<i class="${first + i >= state.from && first + i <= state.to ? "in" : ""}" style="height:${n ? Math.max(8, (n / max) * 100) : 0}%" title="${first + i}: ${n}"></i>`).join("")}</div><div class="rp-years-axis"><span>${first}</span><span>${last}</span></div>`;
  };
  const counted = (obj, labels) => Object.entries(obj).filter(([k]) => labels.some(([id]) => id === k)).sort((a, b) => b[1] - a[1])
    .map(([k, n]) => `<li><span>${esc(labels.find(([id]) => id === k)[1])}</span><b>${fmt(n)}</b></li>`).join("");

  function renderPanel() {
    const pick = state.pick;
    if (pick?.line) return renderLine(pick.line);
    if (pick?.place) return renderPlace(pick.place);
    const top = [...lines].sort((a, b) => value(b) - value(a) || b.reports - a.reports).slice(0, 14);
    const max = Math.max(1, ...top.map(value));
    panel.innerHTML = `
      <p class="rp-kicker">${yearsText()}</p>
      <h2>Busiest routes</h2>
      <p class="rp-sum">${plural(shown.length, "report")} on ${plural(lines.length, "route")}${state.metric === "kg" ? ", by kilograms seized" : ""}.</p>
      ${top.length ? `<ol class="rp-list">${top.map((c) => lineRow(c, max)).join("")}</ol>` : `<p class="muted">Nothing in these years.</p>`}
      <p class="rp-hint">Pick a line or a country on the map to see its reports.</p>`;
  }

  function renderLine(key) {
    const c = byKey.get(key);
    const [from, to] = key.split(">");
    const back = `<button type="button" class="rp-back" data-clear>← All routes</button>`;
    if (!c) {
      panel.innerHTML = `${back}<h2>${place(from)} <span class="arrow">→</span> ${place(to)}</h2><p class="muted">No reports on this route ${yearsText()} with these filters.</p>`;
      return;
    }
    const list = shown.filter((x) => x.f.includes(from) && x.t.includes(to));
    const all = records.filter((x) => x.f.includes(from) && x.t.includes(to));
    const back_ = byKey.get(`${to}>${from}`);
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
      ${back_ ? `<p class="rp-also">The other way: <button type="button" class="rp-link" data-line="${esc(back_.key)}">${place(to)} → ${place(from)}</button> (${plural(back_.reports, "report")})</p>` : ""}
      <p class="rp-also"><button type="button" class="rp-link" data-place="${esc(from)}">All routes of ${esc(nodes[from].n)}</button> · <button type="button" class="rp-link" data-place="${esc(to)}">of ${esc(nodes[to].n)}</button></p>`;
  }

  function renderPlace(id) {
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
        <a class="rp-more" href="${reportsUrl({ from: id })}">All reports from ${esc(nodes[id].n)} →</a>` : ""}
      ${inn.length ? `<h3>Coming in</h3><ol class="rp-list">${inn.slice(0, 10).map((c) => lineRow(c, max, `${place(c.from)} <span class="arrow">→</span>`)).join("")}</ol>
        <a class="rp-more" href="${reportsUrl({ to: id })}">All reports to ${esc(nodes[id].n)} →</a>` : ""}
      ${!out.length && !inn.length ? `<p class="muted">No routes ${yearsText()} with these filters.</p>` : ""}`;
  }

  const choose = (pick) => { state.pick = pick; state.hover = null; tip.remove(); paintState(); renderPanel(); panel.scrollTop = 0; remember(); frame(); };
  panel.addEventListener("click", (e) => {
    const t = e.target.closest("[data-line],[data-place],[data-clear]");
    if (!t) return;
    if (t.dataset.line) choose({ line: t.dataset.line });
    else if (t.dataset.place) choose({ place: t.dataset.place });
    else { choose(null); map.flyTo({ center: [12, 18], zoom: map.getZoom() > 2 ? 1.4 : map.getZoom(), duration: reduced ? 0 : 900 }); }
  });
  panel.addEventListener("pointerover", (e) => { const t = e.target.closest("[data-line]"); state.hover = t?.dataset.line ?? null; paintState(); });
  panel.addEventListener("pointerleave", () => { state.hover = null; paintState(); });
  addEventListener("keydown", (e) => { if (e.key === "Escape" && state.pick) choose(null); });

  // ---- the controls ----
  const setYears = (which) => {
    let a = +fromInput.value, b = +toInput.value;
    if (a > b) { if (which === "from") b = a; else a = b; fromInput.value = a; toInput.value = b; }
    state.from = a; state.to = b; refresh();
  };
  fromInput.addEventListener("input", () => { stop(); setYears("from"); });
  toInput.addEventListener("input", () => { stop(); setYears("to"); });
  metricSel.addEventListener("change", () => { state.metric = metricSel.value; refresh(); });
  productSel.addEventListener("change", () => { state.product = productSel.value; refresh(); });
  modeSel.addEventListener("change", () => { state.mode = modeSel.value; refresh(); });
  // play: from the first year, one more year at a time, up to the last
  let timer = 0;
  function stop() { clearInterval(timer); timer = 0; playBtn.classList.remove("on"); playBtn.firstElementChild.textContent = "▶"; playBtn.setAttribute("aria-label", "Play the years one by one"); }
  playBtn.addEventListener("click", () => {
    if (timer) return stop();
    playBtn.classList.add("on"); playBtn.firstElementChild.textContent = "❚❚"; playBtn.setAttribute("aria-label", "Pause");
    if (state.to >= last) { state.to = state.from; }
    toInput.value = state.to; fromInput.value = state.from; refresh();
    timer = setInterval(() => {
      if (state.to >= last) return stop();
      state.to++; toInput.value = state.to; refresh();
    }, 750);
  });

  // ---- full screen: the whole view (years, filters, map and panel), not only the map. The browser's
  // own full screen where it has one; otherwise (iPhone Safari) the view just covers the page.
  const fullBtn = $("[data-full]");
  const isFull = () => root.classList.contains("full");
  function setFull(on) {
    root.classList.toggle("full", on);
    document.documentElement.classList.toggle("routes-full-page", on);
    fullBtn.setAttribute("aria-pressed", String(on));
    fullBtn.lastElementChild.textContent = on ? "Exit full screen" : "Full screen";
    requestAnimationFrame(() => map.resize());
  }
  fullBtn.addEventListener("click", async () => {
    if (isFull()) {
      if (document.fullscreenElement) await document.exitFullscreen().catch(() => {});
      setFull(false);
      return;
    }
    setFull(true);
    if (root.requestFullscreen) await root.requestFullscreen().catch(() => {});
  });
  document.addEventListener("fullscreenchange", () => { if (!document.fullscreenElement && isFull()) setFull(false); });
  addEventListener("keydown", (e) => {
    if (e.key === "Escape" && isFull() && !document.fullscreenElement && !state.pick) setFull(false);
    if ((e.key === "f" || e.key === "F") && !e.metaKey && !e.ctrlKey && !e.altKey && !e.target.closest?.("input,select,textarea")) fullBtn.click();
  });

  // ---- the map's own pointer ----
  map.on("mousemove", "route-hit", (e) => {
    const key = e.features?.[0]?.properties?.key, c = byKey.get(key);
    if (!c) return;
    map.getCanvas().style.cursor = "pointer";
    if (state.hover !== key) { state.hover = key; paintState(); }
    tip.setLngLat(e.lngLat).setHTML(`<strong>${place(c.from)} → ${place(c.to)}</strong><span>${plural(c.reports, "report")}${c.kg ? ` · ${weight(c.kg)} seized` : ""}</span><span class="muted">${c.first === c.last ? c.first : `${c.first}–${c.last}`}</span>`).addTo(map);
  });
  map.on("mouseleave", "route-hit", () => { map.getCanvas().style.cursor = ""; state.hover = null; paintState(); tip.remove(); });
  map.on("mousemove", "place-dots", (e) => {
    const id = e.features?.[0]?.properties?.id, t = totals[id];
    if (!t) return;
    map.getCanvas().style.cursor = "pointer";
    tip.setLngLat(nodes[id].ll).setHTML(`<strong>${place(id)}</strong><span>${fmt(t.out)} out · ${fmt(t.in)} in</span>`).addTo(map);
  });
  map.on("mouseleave", "place-dots", () => { map.getCanvas().style.cursor = ""; tip.remove(); });
  map.on("click", (e) => {
    const hit = map.queryRenderedFeatures(e.point, { layers: ["place-dots", "route-hit"] });
    const placeHit = hit.find((f) => f.layer.id === "place-dots"), lineHit = hit.find((f) => f.layer.id === "route-hit");
    if (placeHit) choose({ place: placeHit.properties.id });
    else if (lineHit) choose({ line: lineHit.properties.key });
    else if (state.pick) choose(null);
  });

  // ---- the moving dots ----
  let lastDraw = 0;
  const tick = (now) => {
    requestAnimationFrame(tick);
    if (now - lastDraw < 33 || document.hidden) return; // about 30 frames a second is smooth enough
    lastDraw = now;
    map.getSource("dots")?.setData(dotData(now));
  };

  compute(); seed(); renderPanel(); refresh();
  map.on("load", () => {
    build();
    frame();
    if (!reduced) requestAnimationFrame(tick);
  });
  document.addEventListener("theme:change", () => {
    map.setStyle(styles[theme()], { diff: false });
    map.once("style.load", build);
  });
}
