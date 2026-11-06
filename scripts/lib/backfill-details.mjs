// Fills a report's structured details from what it already says: its tags propose a value, and
// a cited sentence of its own text that names the value supports it (its citations become the
// value's data_sources). Agencies are found by name in cited sentences, in the report's country.
// Nothing is guessed: a tag no sentence bears out, or a sentence without a citation, adds nothing.
import vocabularies from "../../src/data/vocabularies.json" with { type: "json" };
import agencies from "../../src/data/agencies.json" with { type: "json" };

const escape = (s) => s.replace(/[.*+?^${}()|[\]\\]/g, "\\$&");
// "indoor-grow" matches "indoor grow" or "indoor-grow"; whole words only
const wordPattern = (word, caseSensitive = false) => new RegExp(`(?<![\\p{L}\\p{N}])${escape(word).replace(/\\?-/g, "[-\\s]")}(?![\\p{L}\\p{N}])`, caseSensitive ? "u" : "iu");

// a report's body as sentences with the sources they cite
export function citedSentences(body) {
  const out = [];
  for (const paragraph of body.split(/\n\s*\n/)) {
    for (const chunk of paragraph.replace(/\s+/g, " ").split(/(?<=\])\s+|(?<=[.!?])\s+(?=[A-Z])/)) {
      const cites = [...chunk.matchAll(/\[@(\d+)(?:\s*,\s*@?(\d+))*\]/g)].flatMap((m) => [...m[0].matchAll(/\d+/g)].map((d) => Number(d[0])));
      if (cites.length) out.push({ text: chunk.replace(/\[@[^\]]*\]/g, "").trim(), cites });
    }
  }
  return out;
}

const support = (sentences, words, caseSensitive = false) => {
  const patterns = words.filter((w) => w.length >= 3).map((w) => wordPattern(w, caseSensitive));
  return [...new Set(sentences.filter((s) => patterns.some((p) => p.test(s.text))).flatMap((s) => s.cites))].sort((a, b) => a - b);
};

const countryMatches = (agency, country) => !agency.country || agency.country === country || country.startsWith(`${agency.country}/`);

// the details and data_sources to add, or null when nothing is supported
export function backfillDetails({ country, tags = [], details, figures }, body, sourceCount) {
  if (details) return null;
  const sentences = citedSentences(body).map((s) => ({ ...s, cites: s.cites.filter((n) => n >= 1 && n <= sourceCount) })).filter((s) => s.cites.length);
  const tagSet = new Set(tags);
  const out = {}, sources = {};
  const add = (key, value, cites) => { out[key] = value; sources[`details.${key}`] = cites; };

  // agencies: by name or acronym in a cited sentence (acronyms case-sensitive)
  const found = [], agencyCites = new Set();
  for (const a of agencies) {
    if (!countryMatches(a, country)) continue;
    const names = [a.name.replace(/\s*\(.*\)$/, ""), ...a.aliases];
    const cites = [...support(sentences, names.filter((n) => n !== n.toUpperCase())), ...support(sentences, names.filter((n) => n === n.toUpperCase()), true)];
    if (cites.length || (a.tags.some((t) => tagSet.has(t)) && support(sentences, a.tags).length)) {
      found.push(a);
      for (const n of cites.length ? cites : support(sentences, a.tags)) agencyCites.add(n);
    }
  }
  if (found.length) {
    add("agencies", found.slice(0, 10).map((a) => a.id), [...agencyCites].sort((x, y) => x - y));
    const types = [...new Set(found.map((a) => a.type))];
    add("agency_types", types, [...agencyCites].sort((x, y) => x - y));
  }

  // listed terms: a tag of the term proposes it, a cited sentence naming it supports it
  for (const [key, list] of [["activity", "activity"], ["setting", "setting"], ["environment", "environment"]]) {
    const values = [], cites = new Set();
    for (const term of vocabularies[list]) {
      const termTags = term.tags ?? [];
      if (!termTags.some((t) => tagSet.has(t))) continue;
      const support_ = support(sentences, [...termTags, term.id, term.label]);
      if (support_.length) { values.push(term.id); support_.forEach((n) => cites.add(n)); }
    }
    if (values.length) add(key, values, [...cites].sort((x, y) => x - y));
  }
  // products: as above
  const products = [], productCites = new Set();
  for (const term of vocabularies.product) {
    const termTags = term.tags ?? [];
    if (!termTags.some((t) => tagSet.has(t))) continue;
    const support_ = support(sentences, [...termTags, term.label]);
    if (support_.length) { products.push(term.id); support_.forEach((n) => productCites.add(n)); }
  }
  // no quantity: the report's total weight does not show how much of each product it was
  if (products.length) add("products", products.map((p) => ({ product: p })), [...productCites].sort((x, y) => x - y));
  if (!Object.keys(out).length) return null;
  return { details: out, data_sources: sources };
}

// YAML text for the frontmatter, in the style the service writes
export function detailsYAML({ details, data_sources }) {
  const lines = ["details:"];
  const flowList = (v) => `[${v.join(", ")}]`;
  for (const [key, value] of Object.entries(details)) {
    if (key === "products") {
      lines.push("  products:");
      for (const p of value) lines.push(`    - {product: ${p.product}${p.kg != null ? `, kg: ${p.kg}` : ""}}`);
    } else lines.push(`  ${key}: ${flowList(value.map((v) => (/^[a-z0-9-]+$/.test(v) ? v : JSON.stringify(v))))}`);
  }
  lines.push("data_sources:");
  for (const [field, cites] of Object.entries(data_sources).sort()) lines.push(`  ${field}: ${flowList(cites)}`);
  return lines.join("\n") + "\n";
}

// the report text with the block inserted before `status:` (every report has one)
export function insertDetails(text, block) {
  if (!text.startsWith("---\n")) return null;
  const end = text.indexOf("\n---", 4);
  const front = text.slice(4, end + 1);
  if (/^details:/m.test(front) || /^data_sources:/m.test(front)) return null;
  const at = front.search(/^status:/m);
  if (at < 0) return null;
  return "---\n" + front.slice(0, at) + block + front.slice(at) + text.slice(end + 1);
}
