import { test } from 'node:test';
import assert from 'node:assert/strict';
import { measureMatches } from '../src/lib/measures.js';
import { glossaryParts } from '../src/lib/glossary.js';

const found = (text, country) => measureMatches(text, { country }).map((m) => m.text);
const tip = (text, country) => measureMatches(text, { country })[0]?.entry.text ?? '';
const rows = (text, country) => measureMatches(text, { country })[0]?.entry.rows ?? [];

test('weights both ways: metric in pounds and ounces, others in grams and kilograms', () => {
  assert.deepEqual(found('120.960 kg, 500-gram packets, 562 pounds, 20 oz and 37 quintals'), ['120.960 kg', '500-gram', '562 pounds', '20 oz', '37 quintals']);
  assert.match(tip('120.960 kg'), /Pounds: ≈ 267 lb/);
  assert.match(tip('500 g'), /Pounds: ≈ 1\.1 lb; Ounces: ≈ 17\.6 oz/);
  assert.match(tip('562 pounds'), /Kilograms: ≈ 255 kg/);
  assert.match(tip('37 quintals'), /Tonnes: ≈ 3\.7 t/);
  assert.match(tip('5 tons', 'us'), /^If US tons \(2,000 lb\): ≈ 4\.54 t/);
  assert.match(tip('5 tons', 'ph'), /^If metric tonnes: ≈ 5 t/);
});

test('networks, distances and words are not weights or money', () => {
  assert.deepEqual(found('a 4G network, 5G, 15 m from the road, all 3 men, 2 TOP officers'), []);
});

test('money is read in the country\'s currency and converted to dollars and euros', () => {
  assert.deepEqual(found('valued at Rs 60.48 lakh, over ₹1.5 crore and HK$1.2m', 'in'), ['Rs 60.48 lakh', '₹1.5 crore', 'HK$1.2m']);
  assert.match(tip('Rs 60.48 lakh', 'in'), /^Indian Rupee: ₹6,048,000; US dollar: ≈ US\$[\d,]+; Euro: ≈ €[\d,]+; British pound: ≈ £[\d,]+\. A lakh is 100,000\. At the exchange rates of/);
  assert.match(tip('Rs 5,000', 'pk'), /Pakistani Rupee/);
  assert.match(tip('$2.5 million', 'us'), /Euro: ≈ €[\d.]+ million/);
  assert.match(tip('$2.5 million', 'au'), /Australian Dollar/);
  assert.match(tip('5,000 euros'), /US dollar: ≈ US\$/);
  assert.match(tip('USD 300'), /Euro: ≈ €/);
  assert.equal(found('R2,000 fine', 'gb').length, 0);
  assert.match(tip('R2,000 fine', 'za'), /South African Rand/);
});

test('pounds of money are told from pounds of weight', () => {
  assert.match(tip('a 10,000-pound fine', 'eg'), /Egyptian Pound/);
  assert.match(tip('fined 500 pounds', 'gb'), /British Pound/);
  assert.match(tip('500 pounds of cannabis', 'us'), /kg/);
});

test('the glossary keeps its terms around amounts, and explains every amount where it is read', () => {
  const parts = glossaryParts('valued at Rs 60.48 lakh and Rs 60.48 lakh', new Set(), { country: 'in' });
  assert.equal(parts.filter((p) => p.entry).length, 2);
  assert.ok(parts.filter((p) => p.entry).every((p) => p.entry.id.startsWith('money:')));
  assert.equal(parts.map((p) => p.text).join(''), 'valued at Rs 60.48 lakh and Rs 60.48 lakh');
  assert.equal(parts.find((p) => p.entry).entry.eyebrow, 'Converted');
});

test('tooltips list the other units as rows, leaving out the one written', () => {
  assert.deepEqual(rows('500 g').map(([l]) => l), ['Pounds', 'Ounces']);
  assert.deepEqual(rows('37 quintals').map(([l]) => l), ['Tonnes', 'Kilograms', 'Pounds', 'US tons']);
  assert.deepEqual(rows('11,800 lira', 'tr').map(([l]) => l), ['Turkish Lira', 'US dollar', 'Euro', 'British pound']);
  assert.deepEqual(rows('$300', 'us').map(([l]) => l), ['US Dollar', 'Euro', 'British pound']);
});

test('a currency named with its nationality is read as that currency', () => {
  assert.deepEqual(found('seized 11,800 Turkish lira in cash', 'tr'), ['11,800 Turkish lira']);
  assert.equal(rows('500 Australian dollars', 'nz')[0][0], 'Australian Dollar');
  assert.equal(rows('5,000 Indian rupees', 'pk')[0][0], 'Indian Rupee');
  assert.deepEqual(found('Police said 300 dollars', 'ca'), ['300 dollars']);
});
