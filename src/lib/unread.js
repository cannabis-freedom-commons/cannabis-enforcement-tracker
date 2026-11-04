// "New since your last visit": which reports a reader has not seen yet, kept in their browser only.
// A visit is a stretch of use without a gap of more than VISIT_GAP; the reports added after the
// newest one the reader had seen by the end of their previous visit are new to them, until they open
// one (it is then read) or mark them all as read. On a first visit nothing is new.
//   state: { since, seen, last }: since: the newest addition seen before this visit (new = added
//     after it); seen: the newest addition seen so far; last: when the reader was last here (ms)
export const VISIT_GAP = 30 * 60 * 1000;
export const READ_MAX = 1000;

// times are ISO strings with or without fractions of a second ("…49Z", "…49.157881Z"), so they are
// compared as times, not as text
const ms = (t) => Date.parse(t ?? "") || 0;
const later = (a, b) => (!a ? b ?? null : !b ? a : ms(a) >= ms(b) ? a : b);

// the state for a page shown at `now` whose newest report was added at `latest`
export function nextVisit(state, now, latest) {
  if (!state?.last) return { since: latest ?? null, seen: latest ?? null, last: now };
  const fresh = now - state.last > VISIT_GAP;
  return { since: fresh ? later(state.seen, state.since) : state.since, seen: later(state.seen, latest), last: now };
}

// whether a report added at `addedAt` is new to the reader
export const isUnread = (state, id, addedAt, read = new Set()) => !!(state?.since && addedAt && ms(addedAt) > ms(state.since) && !read.has(id));

// the read list with `id` added, newest last, at most READ_MAX long
export const withRead = (ids, id) => [...ids.filter((x) => x !== id), id].slice(-READ_MAX);

// everything seen so far is read: nothing stays new until something newer is added
export const markAllRead = (state, latest) => ({ ...state, since: later(state?.seen, latest), seen: later(state?.seen, latest) });
