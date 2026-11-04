// The reader's "new since your last visit" state in this browser (src/lib/unread.js), shared by every
// script on the page: whether a report is new to them, marking reports read, and telling the page
// when that changes (in this tab or another).
import { nextVisit, isUnread, withRead, markAllRead } from "../lib/unread.js";

const VISIT = "cet:visit", READ = "cet:read";
const meta = (name) => document.querySelector(`meta[name="${name}"]`)?.getAttribute("content") || null;
const load = (key) => { try { return JSON.parse(localStorage.getItem(key) ?? "null"); } catch { return null; } };
const save = (key, value) => { try { localStorage.setItem(key, JSON.stringify(value)); } catch { /* private window: this page only */ } };

export const latestAdded = meta("cet-latest");
let state = nextVisit(load(VISIT), Date.now(), latestAdded);
save(VISIT, state);
let read = new Set(load(READ) ?? []);
const listeners = new Set();
const changed = () => listeners.forEach((fn) => fn());

// a report page: the reader has now seen that report
const own = meta("cet-report");
if (own && !read.has(own)) { read = new Set(withRead([...read], own)); save(READ, [...read]); }

export const isNew = (id, addedAt) => isUnread(state, id, addedAt, read);
export const since = () => state.since;
export function onUnreadChange(fn) { listeners.add(fn); return () => listeners.delete(fn); }
// `latest`: the newest addition the reader now knows of (a newer build's, once it is announced)
export function markAllAsRead(latest = latestAdded) {
  state = markAllRead(state, latest);
  save(VISIT, state);
  changed();
}
// still here: keeps the visit going while the page stays open
export function stillHere() { state = { ...state, last: Date.now() }; save(VISIT, state); }

// another tab marked reports read, or opened one
addEventListener("storage", (e) => {
  if (e.key === VISIT) { const next = load(VISIT); if (next) state = next; changed(); }
  if (e.key === READ) { read = new Set(load(READ) ?? []); changed(); }
});
