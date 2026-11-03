import { test } from 'node:test';
import assert from 'node:assert/strict';
import { reportTimeline, isReportDay, timelineSelection, bucketRange, brushRange } from '../src/scripts/report-timeline.js';

const rows = [
  { d: '2023-01-01', t: 'raid' },
  { d: '2026-10-27', t: 'seizure', a: ['arrest'] },
  { d: '2026-10-27', t: 'arrest' },
  { d: '2026-10-29', t: 'seizure' },
  { d: '2026-11-01', t: 'raid' },
];
test('daily window excludes older and future reports and preserves empty days', () => {
  const series = reportTimeline(rows, { start: '2026-10-27', end: '2026-10-29', daily: true });
  assert.deepEqual(series, [
    { key: '2026-10-27', total: 2, categories: { seizure: 1, arrest: 1 } },
    { key: '2026-10-28', total: 0, categories: {} },
    { key: '2026-10-29', total: 1, categories: { seizure: 1 } },
  ]);
});
test('monthly stacks count primary categories once across the full history', () => {
  const series = reportTimeline(rows);
  assert.equal(series[0].key, '2023-01');
  assert.equal(series.at(-1).key, '2026-11');
  assert.deepEqual(series.find(b => b.key === '2026-10'), { key: '2026-10', total: 3, categories: { seizure: 2, arrest: 1 } });
  assert.equal(series.reduce((n, b) => n + b.total, 0), rows.length);
});
test('calendar selections intersect the rolling window, including empty selected periods', () => {
  assert.deepEqual(reportTimeline(rows, { start: '2026-10-01', end: '2026-10-29', year: '2023' }), []);
  const series = reportTimeline([], { start: '2026-10-01', end: '2026-10-29', month: '10', daily: true });
  assert.equal(series.length, 29);
  assert.ok(series.every(b => b.total === 0));
  assert.deepEqual(reportTimeline(rows, { year: '2026', month: '11' }), [{ key: '2026-11', total: 1, categories: { raid: 1 } }]);
});
test('All groups the full history into yearly category stacks', () => {
  const series = reportTimeline(rows, { yearly: true });
  assert.equal(series[0].key, '2023');
  assert.equal(series.at(-1).key, '2026');
  assert.deepEqual(series[1], { key: '2024', total: 0, categories: {} });
  assert.deepEqual(series.at(-1), { key: '2026', total: 4, categories: { seizure: 2, arrest: 1, raid: 1 } });
});
test('current year and month end at today, without future empty buckets', () => {
  const options = { year: '2026', end: '2026-10-29' };
  const months = reportTimeline(rows, options);
  assert.equal(months.at(-1).key, '2026-10');
  assert.equal(months.length, 10);
  const days = reportTimeline(rows, { ...options, month: '10', daily: true });
  assert.equal(days.at(-1).key, '2026-10-29');
  assert.equal(days.length, 29);
  assert.deepEqual(reportTimeline(rows, { ...options, month: '11', daily: true }), []);
  assert.deepEqual(reportTimeline(rows, { year: '2027', end: '2026-10-29' }), []);
});

test('day selection preserves the calendar and toggles the exact day', () => {
  const calendar = { year: '2026', month: '10' };
  assert.deepEqual(timelineSelection('2026-10-27', calendar), { ...calendar, date: '2026-10-27' });
  assert.deepEqual(timelineSelection('2026-10-27', { ...calendar, date: '2026-10-27' }), { ...calendar, date: '' });
  assert.deepEqual(timelineSelection('2026-10-28', { ...calendar, date: '2026-10-27' }), { ...calendar, date: '2026-10-28' });
});
test('calendar selection clears a narrower day and rejects invalid dates', () => {
  assert.deepEqual(timelineSelection('2025', { year: '2026', month: '10', date: '2026-10-27' }), { year: '2025', month: '', date: '' });
  assert.deepEqual(timelineSelection('2026-10', { year: '2026', month: '10', date: '2026-10-27' }), { year: '', month: '', date: '' });
  assert.equal(isReportDay('2024-02-29'), true);
  for (const invalid of ['2026-02-29', '2026-10-32', '2026-13-01', '2026-10', '', null]) {
    assert.equal(isReportDay(invalid), false);
    if (invalid && invalid !== "2026-10") assert.equal(timelineSelection(invalid), null);
  }
});

test('dragging across bars selects every day they cover, in either direction', () => {
  assert.deepEqual(bucketRange('2024-02'), { from: '2024-02-01', to: '2024-02-29' });
  assert.deepEqual(bucketRange('2023'), { from: '2023-01-01', to: '2023-12-31' });
  assert.deepEqual(bucketRange('2026-10-27'), { from: '2026-10-27', to: '2026-10-27' });
  assert.equal(bucketRange('2026-13'), null);
  assert.deepEqual(brushRange('2026-04', '2026-02'), { from: '2026-02-01', to: '2026-04-30' });
  assert.deepEqual(brushRange('2026-10-29', '2026-10-27'), { from: '2026-10-27', to: '2026-10-29' });
  assert.equal(brushRange('x', '2026'), null);
});
