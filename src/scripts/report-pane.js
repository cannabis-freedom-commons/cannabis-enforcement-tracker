// The home page's report pane: a report opened from the map (its marker) or from the list (its title
// or picture) is read in the side pane instead of leaving the map, the way a route opens in the
// Routes view. The map flies to it and rings it ("reports:zoom", WorldMap). The pane shows the
// report's own page, fetched once and cut down to its article (text, figures, photos, case data,
// sources). "← All reports", Escape or the browser's back button return to the list where it was;
// the address keeps the open report (?open=<id>), so it can be shared or reloaded.
// A click with a modifier key, or the middle button, still opens the full page.
import { initMediaViewers } from "./media-viewer.js";
import "./video-facade.js";

const pane = document.getElementById("report-pane");
const home = pane?.closest(".home-pane");
const list = home?.querySelector(".browser");
if (pane && home && list) start();

function start() {
  const base = document.querySelector(".home-map [data-map]")?.dataset.base ?? "/";
  const body = pane.querySelector("[data-body]"), full = pane.querySelector("[data-full]");
  const cache = new Map();
  let openId = null, listScroll = 0, request = 0;
  // where the map was before the first report was opened: closing goes back there
  const mapOf = () => document.querySelector(".home-map [data-map]")?.__map;
  let camera = null;
  const reduced = matchMedia("(prefers-reduced-motion: reduce)").matches;

  const page = (id) => {
    if (!cache.has(id)) cache.set(id, fetch(`${base}report/${encodeURIComponent(id)}/`).then((r) => {
      if (!r.ok) throw new Error(String(r.status));
      return r.text();
    }).catch((e) => { cache.delete(id); throw e; }));
    return cache.get(id);
  };

  // the article of the report's page, made to sit in the pane
  function article(html) {
    const doc = new DOMParser().parseFromString(html, "text/html");
    const a = doc.getElementById("report-article");
    if (!a) return null;
    a.removeAttribute("id");
    // the page's heading becomes the pane's; the home page keeps its own h1
    const h1 = a.querySelector("h1");
    if (h1) { const h = doc.createElement("h2"); h.className = `${h1.className} rpane-title`; h.innerHTML = h1.innerHTML; h1.replaceWith(h); }
    for (const h of a.querySelectorAll("h2:not(.rpane-title)")) { const h3 = doc.createElement("h3"); h3.id = h.id; h3.innerHTML = h.innerHTML; h.replaceWith(h3); }
    a.querySelector(".report-provenance")?.remove();
    return document.importNode(a, true);
  }

  async function open(id, { push = true, zoom = true } = {}) {
    const mine = ++request;
    if (openId !== id) {
      if (!openId) {
        listScroll = home.scrollTop;
        const map = mapOf();
        if (map && zoom) camera = { center: map.getCenter(), zoom: map.getZoom(), bearing: map.getBearing(), pitch: map.getPitch() };
      }
      openId = id;
      full.href = `${base}report/${id}/`;
      body.innerHTML = `<p class="muted rpane-loading">Loading the report…</p>`;
      pane.hidden = false;
      home.classList.add("reading");
      home.scrollTop = 0;
    }
    if (push) remember(id, true);
    if (zoom) document.dispatchEvent(new CustomEvent("reports:zoom", { detail: id }));
    try {
      const a = article(await page(id));
      if (mine !== request) return;
      if (!a) throw new Error("no article");
      body.replaceChildren(a);
      initMediaViewers(body);
    } catch {
      if (mine !== request) return;
      body.innerHTML = `<p class="muted">The report could not be loaded here. <a href="${base}report/${id}/">Open its page</a>.</p>`;
    }
  }

  function close({ push = true } = {}) {
    if (!openId) return;
    request++;
    openId = null;
    pane.hidden = true;
    body.replaceChildren();
    home.classList.remove("reading");
    home.scrollTop = listScroll;
    const map = mapOf();
    if (map && camera) map.flyTo({ ...camera, duration: reduced ? 0 : 1000 });
    camera = null;
    if (push) remember(null, false);
  }

  // the address: ?open=<id> beside the list's own filters (ReportBrowser keeps it when it rewrites them)
  function remember(id, push) {
    const q = new URLSearchParams(location.search);
    if (id) q.set("open", id); else q.delete("open");
    const url = `${location.pathname}${q.size ? `?${q}` : ""}${location.hash}`;
    if (push && history.state?.reportPane !== id) history.pushState({ reportPane: id }, "", url);
    else history.replaceState({ reportPane: id }, "", url);
  }
  addEventListener("popstate", () => {
    const id = new URLSearchParams(location.search).get("open");
    if (id) open(id, { push: false }); else close({ push: false });
  });

  // a marker on the map
  document.addEventListener("reports:open", (e) => {
    if (doc().dataset.homeView === "routes") return;
    e.preventDefault();
    open(e.detail);
  });
  // a title or picture in the list (not the country link, the zoom button or a tag)
  // the whole item is the target: a click anywhere on it but its other links and buttons (country,
  // zoom, tags), a glossary term, or after selecting some of its text
  const reportOf = (href) => new URL(href, location.href).pathname.match(/\/report\/([^/]+)\/$/)?.[1];
  list.addEventListener("click", (e) => {
    if (e.button !== 0 || e.metaKey || e.ctrlKey || e.shiftKey || e.altKey) return;
    const item = e.target.closest(".report-item");
    if (!item) return;
    const a = e.target.closest("a[href]");
    let id = a && reportOf(a.href);
    if (!id) {
      if (a || e.target.closest("button, input, select, .term, .term-g, [role=button]")) return;
      if (String(getSelection?.() ?? "").trim()) return;
      const title = item.querySelector("h3 a[href]");
      id = title && reportOf(title.href);
    }
    if (!id) return;
    e.preventDefault();
    open(decodeURIComponent(id));
  });
  // inside the pane: links to its own sections scroll the pane (the address's #map= is the map's view)
  pane.addEventListener("click", (e) => {
    if (e.target.closest("[data-close]")) { e.preventDefault(); if (history.state?.reportPane) history.back(); else close(); return; }
    const a = e.target.closest('a[href^="#"]');
    if (!a) return;
    e.preventDefault();
    const target = a.getAttribute("href").length > 1 && body.querySelector(`[id="${CSS.escape(a.getAttribute("href").slice(1))}"]`);
    target?.scrollIntoView({ behavior: matchMedia("(prefers-reduced-motion: reduce)").matches ? "auto" : "smooth", block: "start" });
  });
  addEventListener("keydown", (e) => {
    if (e.key === "Escape" && openId && !document.querySelector("dialog[open]") && !e.target.closest?.("input, textarea, [role=combobox]")) close();
  });

  const doc = () => document.documentElement;
  // a shared link (?open=<id>) opens the report once the map can fly to it
  const first = window.__openReport ?? new URLSearchParams(location.search).get("open");
  if (first && doc().dataset.homeView !== "routes") {
    open(first, { push: false, zoom: false });
    const fly = () => document.dispatchEvent(new CustomEvent("reports:zoom", { detail: first }));
    const box = document.querySelector(".home-map [data-map]");
    // "map:built" fires just before the map listens for "reports:zoom": fly on the next task
    if (box?.__map) fly(); else box?.addEventListener("map:built", () => setTimeout(fly), { once: true });
  }
}
