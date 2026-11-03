import test from 'node:test';
import assert from 'node:assert/strict';
import { readPeriod, changePeriod, rangePeriod, periodLabel, recentPeriodBounds } from '../src/scripts/report-period.js';

test('calendar links override recent presets, including older shared links', () => {
  assert.deepEqual(readPeriod(new URLSearchParams('range=30d&year=2023&month=07'), '30d'), {
    mode: 'calendar', range: 'all', year: '2023', month: '07', date: '', from: '', to: '',
  });
  assert.deepEqual(readPeriod(new URLSearchParams('range=7d&date=2024-02-29'), '30d'), {
    mode: 'calendar', range: 'all', year: '2024', month: '02', date: '2024-02-29', from: '', to: '',
  });
});
test('recent and all-time modes clear every calendar restriction', () => {
  for (const mode of ['24h', '7d', '30d', '1y', 'all']) {
    assert.deepEqual(changePeriod(mode, { year: '2023', month: '07', date: '2023-07-01' }), {
      mode, range: mode, year: '', month: '', date: '', from: '', to: '',
    });
  }
  assert.deepEqual(changePeriod('calendar', {}, '2026'), { mode: 'calendar', range: 'all', year: '2026', month: '', date: '', from: '', to: '' });
});
test('page defaults and invalid date parameters never create hidden calendar filters', () => {
  assert.equal(readPeriod(new URLSearchParams(), '30d').range, '30d');
  assert.equal(readPeriod(new URLSearchParams(), 'all').range, 'all');
  assert.equal(readPeriod(new URLSearchParams('range=7d'), 'all').range, '7d');
  assert.equal(readPeriod(new URLSearchParams('range=24h'), '30d').range, '24h');
  assert.deepEqual(readPeriod(new URLSearchParams('year=bad&month=99&date=2026-02-30'), '30d'), {
    mode: '30d', range: '30d', year: '', month: '', date: '', from: '', to: '',
  });
});

test('Reports page ignores relative windows while preserving calendar selections', () => {
  for (const range of ['24h', '7d', '30d', '1y']) {
    assert.equal(readPeriod(new URLSearchParams({ range }), 'all', { allowRelative: false }).mode, 'all');
  }
  assert.deepEqual(readPeriod(new URLSearchParams('range=7d&year=2023'), 'all', { allowRelative: false }), {
    mode: 'calendar', range: 'all', year: '2023', month: '', date: '', from: '', to: '',
  });
});

test('From–To ranges are read only where the page offers them, and override year and month', () => {
  const params = new URLSearchParams('from=2026-04-04&to=2026-03-12&year=2025&range=30d');
  assert.deepEqual(readPeriod(params, '30d', { allowRange: true }), {
    mode: 'calendar', range: 'all', year: '', month: '', date: '', from: '2026-03-12', to: '2026-04-04',
  });
  assert.deepEqual(readPeriod(new URLSearchParams('from=2026-03-12'), '30d'), {
    mode: '30d', range: '30d', year: '', month: '', date: '', from: '', to: '',
  });
  assert.equal(readPeriod(new URLSearchParams('from=2026-02-30'), '30d', { allowRange: true }).mode, '30d');
  assert.deepEqual(rangePeriod('2026-05-01', '2026-01-01'), {
    mode: 'calendar', range: 'all', year: '', month: '', date: '', from: '2026-01-01', to: '2026-05-01',
  });
  assert.equal(rangePeriod('', '').mode, 'all');
  assert.deepEqual(readPeriod(new URLSearchParams('from=2026-03-01&to=2026-03-31&date=2026-03-14'), '30d', { allowRange: true }), {
    mode: 'calendar', range: 'all', year: '', month: '', date: '2026-03-14', from: '2026-03-01', to: '2026-03-31',
  });
  assert.deepEqual(changePeriod('calendar', { from: '2026-01-01', to: '', year: '2020' }), {
    mode: 'calendar', range: 'all', year: '', month: '', date: '', from: '2026-01-01', to: '',
  });
});

test('calendar periods are labelled for the time switch', () => {
  assert.equal(periodLabel({ from: '2026-03-12', to: '2026-04-04' }), '12 Mar – 4 Apr 2026');
  assert.equal(periodLabel({ from: '2026-03-02', to: '2026-03-09' }), '2–9 Mar 2026');
  assert.equal(periodLabel({ from: '2025-12-30', to: '2026-01-02' }), '30 Dec 2025 – 2 Jan 2026');
  assert.equal(periodLabel({ from: '2026-01-05', to: '2026-01-05' }), '5 Jan 2026');
  assert.equal(periodLabel({ from: '2024-01-01' }), 'Since 1 Jan 2024');
  assert.equal(periodLabel({ to: '2010-06-30' }), 'Until 30 Jun 2010');
  assert.equal(periodLabel({ date: '2024-02-29', year: '2024', month: '02' }), '29 Feb 2024');
  assert.equal(periodLabel({ year: '2023', month: '07' }), 'Jul 2023');
  assert.equal(periodLabel({ year: '2023' }), '2023');
  assert.equal(periodLabel({ month: '07' }), 'July, every year');
  assert.equal(periodLabel({}), 'Custom');
});


test('recent windows include valid dates ahead of UTC without admitting days still future worldwide', () => {
  const now = Date.parse('2026-11-03T19:53:47Z');
  for (const days of [1, 7, 30, 365]) {
    const { start, end } = recentPeriodBounds(days, now);
    assert.ok('2026-11-04' >= start && '2026-11-04' <= end, 'Gurugram local date must remain visible');
    assert.ok('2026-11-05' > end);
  }
  assert.equal(recentPeriodBounds(30, Date.parse('2026-11-03T09:59:59Z')).end, '2026-11-03');
  assert.equal(recentPeriodBounds(30, Date.parse('2026-11-03T10:00:00Z')).end, '2026-11-04');
  assert.equal(recentPeriodBounds(30, Date.parse('2026-12-31T19:00:00Z')).end, '2027-01-01');
});
