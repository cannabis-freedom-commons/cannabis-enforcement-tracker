import { test } from 'node:test';
import assert from 'node:assert/strict';
import { figureNumbers, parseRange, formatRange, rangeLabel, inRange, FIGURE_FILTERS } from '../src/lib/figure-filters.js';

const filter = (key) => FIGURE_FILTERS.find((f) => f.key === key);

test('a report\'s figures become plain numbers for the index, only those it states', () => {
  assert.deepEqual(figureNumbers({ type: 'arrest', figures: { arrested: 3, kg_seized: 0.2 } }), { ar: 3, kg: 0.2 });
  assert.deepEqual(figureNumbers({ type: 'sentence', figures: { prison: '7 years rigorous imprisonment' } }), { py: 7 });
  assert.deepEqual(figureNumbers({ type: 'sentence', figures: { prison: 'life imprisonment' } }), { py: 'L' });
  assert.deepEqual(figureNumbers({ type: 'execution', figures: {} }), { de: 1 });
  assert.equal(figureNumbers({ type: 'seizure', figures: { value: { amount: 100, currency: 'USD' } } }).vu, 100);
  assert.deepEqual(figureNumbers({ type: 'raid' }), {});
});

test('ranges are read from and written to the address, with open ends', () => {
  assert.deepEqual(parseRange('1000-5000'), [1000, 5000]);
  assert.deepEqual(parseRange('1000-'), [1000, null]);
  assert.deepEqual(parseRange('-2.5'), [null, 2.5]);
  assert.deepEqual(parseRange('5000-1000'), [1000, 5000]);
  assert.equal(parseRange('-'), null);
  assert.equal(parseRange('abc'), null);
  assert.equal(formatRange([1000, null]), '1000-');
});

test('a range keeps only reports that state the figure, and reads on its chip', () => {
  const plants = filter('plants'), prison = filter('prison');
  assert.equal(inRange({ n: { pl: 1200 } }, plants, [1000, 5000]), true);
  assert.equal(inRange({ n: { pl: 800 } }, plants, [1000, null]), false);
  assert.equal(inRange({ n: {} }, plants, [null, 5000]), false);
  assert.equal(inRange({ n: { py: 'L' } }, prison, [10, null]), true);
  assert.equal(inRange({ n: { py: 'D' } }, prison, [null, 20]), false);
  assert.equal(rangeLabel(plants, [1000, 5000]), 'Plants 1,000–5,000');
  assert.equal(rangeLabel(filter('arrested'), [5, null]), 'Arrested ≥ 5');
  assert.equal(rangeLabel(filter('fine'), [null, 1000]), 'Fine ≤ US$1,000');
  assert.equal(rangeLabel(filter('kg'), [1, 10]), 'Seized 1–10 kg');
});
