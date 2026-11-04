import { test } from "node:test";
import assert from "node:assert/strict";
import { sourcesByDomain } from "../src/lib/source-domains.js";

test("source domains combine www, deduplicate URLs and count reports once per domain", () => {
  assert.deepEqual(sourcesByDomain([
    { sources: [{ url: "https://WWW.Example.com/news#one" }, { url: "https://example.com/news#two" }, { url: "https://example.com/other" }] },
    { sources: [{ url: "https://example.com/news" }, { url: "https://another.example/story" }] },
    { sources: [{ url: "not a URL" }, {}, { url: "file:///tmp/source" }] },
  ]), [
    { domain: "example.com", sources: 2, reports: 2 },
    { domain: "another.example", sources: 1, reports: 1 },
  ]);
});
test("source domain ties sort consistently; missing sources are skipped", () => {
  assert.deepEqual(sourcesByDomain([{}, { sources: [{ url: "https://z.example/a" }, { url: "https://a.example/a" }] }]).map(r => r.domain), ["a.example", "z.example"]);
});

test('sources by top-level domain and kind of site', async () => {
  const { sourcesByTld, sourceKind } = await import('../src/lib/source-domains.js');
  assert.equal(sourceKind('ndlea.gov.ng'), 'official');
  assert.equal(sourceKind('polri.go.id'), 'official');
  assert.equal(sourceKind('m.facebook.com'), 'social');
  assert.equal(sourceKind('web.archive.org'), 'archive');
  assert.equal(sourceKind('gov.br'), 'official');
  assert.equal(sourceKind('governor-news.com'), 'news');
  const r = sourcesByTld([{ sources: [{ url: 'https://www.a.com.br/x' }, { url: 'https://a.com.br/x' }, { url: 'https://dea.gov/y' }] }, { sources: [{ url: 'https://b.br/z' }] }]);
  assert.deepEqual(r.tlds.map((t) => [t.tld, t.sources, t.reports]), [['br', 2, 2], ['gov', 1, 1]]);
  assert.deepEqual([r.total, r.kinds.official, r.kinds.news], [3, 1, 2]);
});
