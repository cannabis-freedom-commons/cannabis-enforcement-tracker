// On every page: marks the reports in its lists that are new to the reader (unread-state.js), counts
// them in the header (a link to just those reports), and checks on load and then once a minute while
// the page is visible whether a newer build of the site is deployed (/version.json), offering to
// reload it. GitHub Pages lets browsers and its CDN keep pages for ten minutes, so once a newer build
// is known every move to another page of the site (Refresh included) carries it as ?_v=, an address
// no cache has; the page that opens drops it from the address again.
import { isNew, since, onUnreadChange, stillHere } from "./unread-state.js";
import { versioned } from "./build-id.js";

const meta = (name) => document.querySelector(`meta[name="${name}"]`)?.getAttribute("content") ?? "";
const base = meta("cet-base") || "/", BUILD = meta("cet-build"), REPORTS = Number(meta("cet-reports")) || 0;
// the cache-passing query a previous page added: not part of the page's address
const here = new URL(location.href);
if (here.searchParams.has("_v")) { here.searchParams.delete("_v"); history.replaceState(history.state, "", here.pathname + here.search + here.hash); }
let newer = ""; // a newer build's id, once one is deployed
const fresh = (href) => { const u = new URL(href, location.href); u.searchParams.set("_v", newer); return u.href; };
// links to the site's own pages: past the caches once a newer build is out
const passCache = (e) => {
  if (!newer) return;
  const a = e.target.closest?.("a[href]");
  if (!a || a.target === "_blank" || a.hasAttribute("download")) return;
  const u = new URL(a.href, location.href);
  if (u.origin !== location.origin || !u.pathname.startsWith(base) || /\.(json|xml|svg|png|jpe?g|webp|pdf)$/i.test(u.pathname)) return;
  if (u.pathname === location.pathname && u.search === location.search && u.hash) return; // a jump within the page
  a.href = fresh(a.href);
};
document.addEventListener("click", passCache, true);
document.addEventListener("auxclick", passCache, true);
const plural = (n, word) => `${n.toLocaleString("en-GB")} ${word}${n === 1 ? "" : "s"}`;

// the reports in lists: a "New" mark on those the reader has not seen
function mark(root = document) {
  for (const item of root.querySelectorAll(".report-item[data-added]")) {
    const unread = isNew(item.dataset.id, item.dataset.added);
    if (item.classList.contains("is-unread") === unread) continue;
    item.classList.toggle("is-unread", unread);
    const badge = item.querySelector(".new-badge");
    if (unread && !badge) item.querySelector(".meta")?.prepend(Object.assign(document.createElement("span"), { className: "new-badge", textContent: "New", title: "Added since your last visit" }));
    if (!unread) badge?.remove();
  }
}
// lists drawn by scripts (the filters, "more"); the observer's callback already comes once per batch
new MutationObserver(() => mark()).observe(document.body, { childList: true, subtree: true });
mark();

// the header's count of new reports, from the list of recent additions
const badge = document.getElementById("new-reports");
let recent = null;
async function loadRecent(fresh = false) { // fresh: the deployed build's, past any cache
  try {
    const r = await fetch(fresh ? `${base}api/recent.json?t=${Date.now()}` : versioned(`${base}api/recent.json`), fresh ? { cache: "no-store" } : {});
    if (r.ok) recent = await r.json();
  } catch { /* offline: no count */ }
  count();
}
function count() {
  if (!badge || !recent) return;
  const n = recent.reports.filter(([id, at]) => isNew(id, at)).length;
  // the list goes back only so far: more may be new than it holds
  const more = recent.from && since() && Date.parse(since()) < Date.parse(recent.from);
  badge.hidden = n === 0;
  badge.querySelector(".new-count").textContent = `${n.toLocaleString("en-GB")}${more ? "+" : ""}`;
  const label = `${more ? "More than " : ""}${plural(n, "report")} new since your last visit`;
  badge.title = label; badge.setAttribute("aria-label", label);
}
onUnreadChange(() => { mark(); count(); });
if (since()) loadRecent(); // a first visit has nothing new

// a newer build: a note with a Refresh button, once per build (until the reader dismisses it)
const DISMISSED = "cet:update-dismissed";
let note = null, offered = "";
function offer(v) {
  if (v.id === offered) return;
  offered = newer = v.id;
  try { if (sessionStorage.getItem(DISMISSED) === v.id) return; } catch { /* no storage */ }
  const added = Math.max(0, (v.reports ?? 0) - REPORTS);
  note?.remove();
  note = document.createElement("div");
  note.className = "update-note";
  note.setAttribute("role", "status");
  note.innerHTML = `<span class="update-dot" aria-hidden="true"></span><span class="update-text"><b>The tracker has been updated</b><span></span></span><button type="button" class="update-reload">Refresh</button><button type="button" class="update-close" aria-label="Not now" title="Not now">×</button>`;
  note.querySelector(".update-text span").textContent = added ? ` · ${plural(added, "new report")}` : "";
  note.querySelector(".update-reload").addEventListener("click", () => location.replace(fresh(location.href)));
  note.querySelector(".update-close").addEventListener("click", () => {
    try { sessionStorage.setItem(DISMISSED, v.id); } catch { /* no storage */ }
    note.remove(); note = null;
  });
  document.body.append(note);
  loadRecent(true); // the header's count takes in the new reports already
}

const EVERY = 60 * 1000;
let lastCheck = Date.now(), timer = 0;
async function check() {
  clearTimeout(timer);
  if (document.hidden) return;
  lastCheck = Date.now();
  stillHere();
  try {
    // a new address each minute, past any cache in between
    const r = await fetch(`${base}version.json?t=${Math.floor(Date.now() / EVERY)}`, { cache: "no-store" });
    if (r.ok) { const v = await r.json(); if (v.id && BUILD && v.id !== BUILD) offer(v); }
  } catch { /* offline: try again later */ }
  timer = window.setTimeout(check, EVERY);
}
if (BUILD && !import.meta.env.DEV) {
  // at once too: a page the browser kept from before a deploy finds out now, not in a minute
  timer = window.setTimeout(check, 1500);
  document.addEventListener("visibilitychange", () => {
    if (document.hidden) clearTimeout(timer);
    else if (Date.now() - lastCheck > EVERY / 2) check();
    else timer = window.setTimeout(check, EVERY);
  });
}
