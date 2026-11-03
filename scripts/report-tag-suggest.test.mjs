import { test } from 'node:test';
import assert from 'node:assert/strict';
import { tagToken, suggestTags } from '../src/scripts/report-tag-suggest.js';

test('only a word starting with # asks for tags', () => {
  assert.equal(tagToken('police #Gan'), 'gan');
  assert.equal(tagToken('#'), '');
  assert.equal(tagToken('ganja'), null);
  assert.equal(tagToken('#ganja kerala'), null);
});

test('tags starting with the text come before tags containing it, most used first', () => {
  const ranked = [{ tag: 'ndps-act', count: 9 }, { tag: 'ganja', count: 8 }, { tag: 'act', count: 2 }, { tag: 'hashish', count: 1 }];
  assert.deepEqual(suggestTags(ranked, 'act').map((t) => t.tag), ['act', 'ndps-act']);
  assert.deepEqual(suggestTags(ranked, '', 2).map((t) => t.tag), ['ndps-act', 'ganja']);
  assert.deepEqual(suggestTags(ranked, '#zzz'), []);
});
