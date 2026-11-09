import test from 'node:test';
import assert from 'node:assert/strict';
import { routeRecords, routeNodes, corridors, placeTotals, arc, along, productOf, modesOf } from '../src/lib/routes.js';

const report = (id, date, route, extra = {}) => ({ id, url: `report/${id}/`, title: id, date: new Date(`${date}T00:00:00Z`), details: { route, ...extra.details }, figures: extra.figures });
const centres = { ma: [32, -6], es: [40, -4], fr: [46, 2], us: [39, -98], 'us/ca': [39, -98], nz: [-41, 174], fj: [-17, 178], ws: [-13, -172] };
const env = { centreOf: (id) => centres[id] ?? null, countryName: (id) => id.toUpperCase(), flagOf: () => '' };

test('only reports naming both ends become route records, newest first', () => {
  const list = [
    report('a', '2020-01-01', { from: ['ma'], to: ['es'] }, { details: { products: [{ product: 'hashish' }], setting: ['port', 'vehicle'] }, figures: { kg_seized: 500 } }),
    report('b', '2024-05-01', { from: ['ma'], to: ['es', 'fr'], crossing: 'Algeciras' }),
    report('c', '2024-06-01', { from: ['ma'] }),
    { id: 'd', date: new Date(), details: {} },
  ];
  const recs = routeRecords(list);
  assert.deepEqual(recs.map((r) => r.i), ['b', 'a']);
  assert.deepEqual([recs[1].kg, recs[1].p, recs[1].m], [500, 'hashish', ['road', 'sea']]);
  assert.equal(recs[0].x, 'Algeciras');
});

test('corridors count each from × to pair; routes inside one place are domestic', () => {
  const recs = routeRecords([
    report('a', '2020-01-01', { from: ['ma'], to: ['es'] }, { figures: { kg_seized: 500 } }),
    report('b', '2024-05-01', { from: ['ma'], to: ['es', 'fr'] }, { figures: { kg_seized: 10 } }),
    report('c', '2024-05-01', { from: ['us/ca'], to: ['us'] }),
    report('d', '2024-05-01', { from: ['xx'], to: ['es'] }),
  ]);
  const nodes = routeNodes(recs, env);
  assert.equal(nodes.xx, undefined);
  assert.deepEqual(nodes.ma.ll, [-6, 32]);
  const { lines, domestic } = corridors(recs, nodes);
  assert.deepEqual(lines.map((c) => [c.key, c.reports, c.kg, c.first, c.last]), [['ma>es', 2, 510, 2020, 2024], ['ma>fr', 1, 10, 2024, 2024]]);
  assert.deepEqual(domestic.map((c) => [c.key, c.reports]), [['us', 1]]);
  assert.deepEqual(placeTotals(lines), { ma: { out: 3, in: 0 }, es: { out: 0, in: 2 }, fr: { out: 0, in: 1 } });
});

test('the line takes the short way across the date line and bends left of travel', () => {
  const line = arc([174, -41], [-172, -13]); // New Zealand to Samoa
  assert.deepEqual(line[0], [174, -41]);
  assert.deepEqual(line.at(-1), [188, -13]);
  assert(line.every(([x]) => x >= 170 && x <= 195));
  const there = arc([0, 0], [10, 0]), back = arc([10, 0], [0, 0]);
  assert(there[20][1] > 0 && back[20][1] < 0, 'the two directions are two lines');
  assert.deepEqual(along([[0, 0], [10, 0], [10, 10]], 0.75), [10, 5]);
  assert.deepEqual(along([[0, 0], [10, 0]], 2), [10, 0]);
});

test('products and ways of travel group the vocabulary', () => {
  assert.equal(productOf(['flower', 'hashish']), 'hashish');
  assert.equal(productOf(['plants']), 'flower');
  assert.equal(productOf(['oil']), 'other');
  assert.equal(productOf([]), '');
  assert.deepEqual(modesOf(['airport', 'mail', 'home']), ['air', 'mail']);
});
