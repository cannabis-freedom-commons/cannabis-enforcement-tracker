import test from 'node:test';
import assert from 'node:assert/strict';
import { sourceKey, findDuplicateReportCandidates } from './lib/duplicate-reports.mjs';
const report = (file, values = {}) => ({ file, country: 'gb', date: '2026-11-01', title: 'Cannabis seizure', sources: [], ...values });
test('source variants nominate the same article without dropping meaningful query parameters', () => {
  assert.equal(sourceKey('http://www.example.org/news/case/amp/?utm_source=rss#top'), sourceKey('https://example.org/news/case/'));
  assert.notEqual(sourceKey('https://example.org/article?id=1'), sourceKey('https://example.org/article?id=2'));
});
test('a shared roundup remains a candidate with its evidence; candidates do not merge or mutate reports', () => {
  const input = [report('a', { title: 'Four held at a warehouse', sources: [{ url: 'https://news.example/roundup' }] }), report('b', { title: 'Garden uprooted at a different town', sources: [{ url: 'https://news.example/roundup' }] })];
  const before = structuredClone(input), pairs = findDuplicateReportCandidates(input);
  assert.deepEqual(input, before);
  assert.equal(pairs.length, 1);
  assert.deepEqual(pairs[0].sharedSources, ['news.example/roundup']);
});
test('case names connect differently worded court stages many years apart', () => {
  const pairs = findDuplicateReportCandidates([
    report('arrest', { date: '2001-01-01', title: 'Eleanor Sample arrested with cannabis', summary: 'Eleanor Sample was arrested at the border.' }),
    report('appeal', { date: '2026-01-01', title: 'Court frees Eleanor Sample on appeal', summary: 'Eleanor Sample won her appeal.' }),
  ]);
  assert.equal(pairs.length, 1);
  assert.ok(pairs[0].sharedNames.includes('Eleanor Sample'));
  assert.ok(pairs[0].days > 8000);
});
test('a national report can match its state report, while unrelated countries do not match by a number alone', () => {
  const a = report('national', { country: 'us', title: 'Somerset warehouse: 931 plants seized', figures: { plants_destroyed: 931 } });
  const b = report('state', { ...a, file: 'state', country: 'us/ca' });
  const c = report('other', { ...a, file: 'other', country: 'ca' });
  assert.deepEqual(findDuplicateReportCandidates([a, b, c]).map(p => [p.a, p.b]), [['national', 'state']]);
});
test('empty or generic titles and common small arrest counts are not sufficient evidence', () => {
  const a = report('a', { title: 'Cannabis seized', figures: { arrested: 1 } });
  const b = report('b', { title: 'Cannabis seized', figures: { arrested: 1 } });
  assert.deepEqual(findDuplicateReportCandidates([a, b]), []);
});
