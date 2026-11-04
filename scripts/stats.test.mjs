import test from 'node:test';
import assert from 'node:assert/strict';
import { compact, headlineFigures, modelName, reportsByModel, topCountriesBy, periodColumns } from '../src/lib/stats.js';

test('big figures stay readable as they grow', () => {
  assert.equal(compact(9999), '9,999');
  assert.equal(compact(17437), '17.4K');
  assert.equal(compact(5170718), '5.2M');
});

test('headline figures say how many reports state them; seizures switch to tonnes', () => {
  const list = [{ type: 'seizure', figures: { kg_seized: 1500, arrested: 2 } }, { type: 'arrest', figures: { arrested: 3 } }, { type: 'execution', figures: {} }];
  const f = Object.fromEntries(headlineFigures(list).map((x) => [x.id, x]));
  assert.deepEqual([f.arrested.value, f.arrested.reports], [5, 2]);
  assert.deepEqual([f.seized.value, f.seized.unit], [1.5, 't']);
  assert.deepEqual([f.deaths.value, f.deaths.reports], [1, 1]);
});

test('small cannabis totals use grams and milligrams', () => {
  for (const [kg, value, unit] of [[0.00386, 3.86, 'g'], [0.00047, 470, 'mg']]) {
    const seized = headlineFigures([{ type: 'seizure', figures: { kg_seized: kg } }]).find(f => f.id === 'seized');
    assert.ok(Math.abs(seized.value - value) < 1e-9);
    assert.equal(seized.unit, unit);
  }
});

test('models are grouped without provider and reasoning setting; unknown is counted apart', () => {
  assert.equal(modelName('spacexai/grok-4.7#medium'), 'grok-4.7');
  assert.equal(modelName('Meta Muse Spark (via Muse Code)'), 'Meta Muse Spark (via Muse Code)');
  assert.equal(modelName('unknown # Creator identifier was unavailable'), null);
  const { rows, unrecorded } = reportsByModel([{ created_by_model: 'xai/grok-4.7' }, { created_by_model: 'spacexai/grok-4.7#medium' }, { created_by_model: 'cursor/composer-2.5' }, { created_by_model: 'unknown' }]);
  assert.equal(unrecorded, 1);
  assert.deepEqual(rows.map((r) => [r.name, r.count]), [['grok-4.7', 2], ['composer-2.5', 1]]);
  assert.equal(rows[0].variants.length, 2);
});

test('top countries and period columns', () => {
  const list = [{ country: 'in', type: 'raid', year: 2025, figures: { arrested: 4 } }, { country: 'ke', type: 'arrest', year: 2026, figures: { arrested: 9 } }, { country: 'in', type: 'raid', year: 2026, figures: { arrested: 1 } }];
  assert.deepEqual(topCountriesBy(list, 'arrested'), [{ id: 'ke', value: 9 }, { id: 'in', value: 5 }]);
  const cols = periodColumns(list, (r) => r.year, [2024, 2025, 2026]);
  assert.deepEqual(cols.map((c) => c.total), [0, 1, 2]);
  assert.deepEqual(cols[2].types, { arrest: 1, raid: 1 });
});

import { tileNumber, prisonYears, prisonBands, seizureBands, typeMixByRegion } from '../src/lib/stats.js';
test('tile numbers stay short as the totals grow', () => {
  assert.equal(tileNumber({ value: 25623 }), '25.6K');
  assert.equal(tileNumber({ value: 70989763 }), '71M');
  assert.equal(tileNumber({ value: 8604.7, unit: 't' }), '8,605');
  assert.equal(tileNumber({ value: 3.4567, unit: 'kg' }), '3.46');
  assert.equal(tileNumber({ value: 155 }), '155');
});
test('prison terms are read from the stated text, or left out', () => {
  assert.equal(prisonYears('7 years rigorous imprisonment'), 7);
  assert.equal(prisonYears('3 years and 6 months'), 3.5);
  assert.equal(prisonYears('18 months'), 1.5);
  assert.ok(Math.abs(prisonYears('15 days') - 15 / 365) < 1e-9);
  assert.equal(prisonYears('six years (reported as three years on each of two counts)'), 6);
  assert.equal(prisonYears('3 to 7 years'), 7);
  assert.equal(prisonYears('9.5 to 10.5 years'), 10.5);
  assert.equal(prisonYears('12 and 18 months'), 1.5);
  assert.equal(prisonYears('life imprisonment and 12 strokes'), 'life');
  assert.equal(prisonYears('death by hanging'), 'death');
  assert.equal(prisonYears('more than 30 years (combined)'), null);
  assert.equal(prisonYears('time served (29 March 2007 to 9 July 2008)'), null);
  assert.equal(prisonYears('16 weeks (time served)'), 16 / 52);
  const b = prisonBands([{ figures: { prison: '7 years' } }, { figures: { prison: 'life' } }, { figures: { prison: 'over 24 years combined' } }, { figures: {} }]);
  assert.deepEqual(b.bands.filter((x) => x.count).map((x) => x.label), ['5–10 years', 'Life']);
  assert.equal(b.unread, 1);
  assert.equal(b.median, 7);
});
test('seizures are banded by weight and the type mix sums to one per region', () => {
  const s = seizureBands([{ figures: { kg_seized: 0.5 } }, { figures: { kg_seized: 120 } }, { figures: { kg_seized: 12000 } }, { figures: {} }]);
  assert.deepEqual(s.bands.map((x) => x.count), [1, 0, 0, 1, 0, 1]);
  assert.equal(s.median, 120);
  const mix = typeMixByRegion([{ type: 'raid', c: 'A' }, { type: 'arrest', c: 'A' }, { type: 'raid', c: 'B' }], (r) => r.c);
  assert.equal(mix[0].region, 'A');
  assert.equal(mix[0].shares.raid + mix[0].shares.arrest, 1);
});

test('calendar charts count weekdays from Monday and spread each type over the months', async () => {
  const { weekdayCounts, seasonality } = await import('../src/lib/stats.js');
  const list = [{ type: 'eradication', date: new Date('2026-11-02') }, { type: 'eradication', date: new Date('2026-07-05') }, { type: 'seizure', date: new Date('2026-07-06') }];
  const w = weekdayCounts(list);
  assert.deepEqual([w[0].label, w[0].count, w[6].count], ['Monday', 2, 1]);
  const s = seasonality(list);
  assert.equal(s[0].type, 'eradication');
  assert.deepEqual([s[0].counts[6], s[0].shares[10]], [1, 0.5]);
});

test('tag cloud keeps tags used more than once, sized on a log scale, in alphabetical order', async () => {
  const { tagCloud } = await import('../src/lib/stats.js');
  const list = [...Array(9)].map(() => ({ tags: ['hashish'] })).concat([{ tags: ['airport', 'once'] }, { tags: ['airport'] }]);
  assert.deepEqual(tagCloud(list).map((t) => [t.tag, t.step]), [['airport', 1], ['hashish', 6]]);
});

test('sources per report and reports added per day', async () => {
  const { sourceCountBands, addedPerDay } = await import('../src/lib/stats.js');
  const b = sourceCountBands([{ sources: [1] }, { sources: [1, 2] }, { sources: Array(12) }]);
  assert.deepEqual(b.bands.map((x) => x.count), [1, 1, 0, 0, 0, 0, 1]);
  const days = addedPerDay([{ type: 'arrest', added_at: '2026-11-01T10:00:00Z' }, { type: 'arrest', added_at: '2026-11-03T01:00:00Z' }], new Date('2026-11-03T12:00:00Z'));
  assert.deepEqual(days.map((d) => [d.key, d.total, d.cumulative]), [['2026-11-01', 1, 1], ['2026-11-02', 0, 1], ['2026-11-03', 1, 2]]);
});
