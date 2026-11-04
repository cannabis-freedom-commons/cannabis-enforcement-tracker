import { test } from 'node:test';
import assert from 'node:assert/strict';
import { nextVisit, isUnread, withRead, markAllRead, VISIT_GAP, READ_MAX } from '../src/lib/unread.js';

const T = Date.parse('2026-11-04T10:00:00Z');

test('a first visit has nothing new', () => {
  const s = nextVisit(null, T, '2026-11-04T09:00:00Z');
  assert.equal(isUnread(s, 'a', '2026-11-04T09:00:00Z'), false);
});

test('reports added after the last visit are new, until read or a later visit', () => {
  let s = nextVisit(null, T, '2026-11-01T00:00:00Z');
  // back a day later, with two newer reports
  s = nextVisit(s, T + 864e5, '2026-11-04T12:00:00Z');
  assert.equal(isUnread(s, 'a', '2026-11-03T00:00:00Z'), true);
  assert.equal(isUnread(s, 'old', '2026-10-30T00:00:00Z'), false);
  assert.equal(isUnread(s, 'a', '2026-11-03T00:00:00Z', new Set(['a'])), false);
  // more pages in the same visit keep them new; one added meanwhile is new too
  s = nextVisit(s, T + 864e5 + 60000, '2026-11-04T13:00:00Z');
  assert.equal(isUnread(s, 'a', '2026-11-03T00:00:00Z'), true);
  assert.equal(isUnread(s, 'b', '2026-11-04T13:00:00Z'), true);
  // the next visit: what was seen is no longer new
  s = nextVisit(s, T + 864e5 + 60000 + VISIT_GAP + 1, '2026-11-04T13:00:00Z');
  assert.equal(isUnread(s, 'b', '2026-11-04T13:00:00Z'), false);
});

test('marking all read, and the read list stays bounded', () => {
  let s = nextVisit(nextVisit(null, T, '2026-11-01T00:00:00Z'), T + 864e5, '2026-11-04T00:00:00Z');
  s = markAllRead(s, '2026-11-04T00:00:00Z');
  assert.equal(isUnread(s, 'a', '2026-11-03T00:00:00Z'), false);
  assert.equal(isUnread(s, 'n', '2026-11-05T00:00:00Z'), true);
  let ids = [];
  for (let i = 0; i < READ_MAX + 5; i++) ids = withRead(ids, `r${i}`);
  assert.equal(ids.length, READ_MAX);
  assert.deepEqual(withRead(['a', 'b'], 'a'), ['b', 'a']);
});

test('times with and without fractions of a second compare as times', () => {
  const s = { since: '2026-11-04T19:01:49.157881Z', seen: '2026-11-04T19:01:49.157881Z', last: T };
  assert.equal(isUnread(s, 'a', '2026-11-04T19:01:49Z'), false);
  assert.equal(isUnread(s, 'b', '2026-11-04T19:01:50.000Z'), true);
});
