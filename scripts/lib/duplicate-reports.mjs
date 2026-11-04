// Advisory candidate generation. Scores are review priorities, never merge decisions.
const STOP = new Set(`a an and are as at by cannabis marijuana marihuana ganja hashish hash hemp drug drugs police of on in over for from to the with after during more than about nearly allegedly alleged seized seizes seize arrest arrests arrested held jailed jail sentenced sentence sentencing kg kilograms kilogram g grams gram tonne tonnes t lb pounds million worth plants plant destroys destroyed destroy found accused man men woman women raid raids raided smuggling smuggled smuggles trafficking trafficker traffickers possession prison years year months month officers case court operation caught following amid says say said`.split(' '));
export function sourceKey(value) {
  try {
    const url = new URL(value);
    url.hostname = url.hostname.replace(/^www\./, '');
    for (const key of [...url.searchParams.keys()]) if (/^(utm_|fbclid$|gclid$)/i.test(key)) url.searchParams.delete(key);
    // These variants only nominate a pair. A reader must still confirm the same article/event.
    return decodeURI(url.host + url.pathname.replace(/\/amp\/?$/, '').replace(/\/$/, '') + url.search);
  } catch { return value; }
}
const tokens = value => new Set((String(value ?? '').normalize('NFKD').replace(/[\u0300-\u036f]/g, '').toLowerCase().match(/[\p{L}\p{N}]+/gu) ?? []).filter(w => w.length > 1 && !STOP.has(w)));
const compatibleCountry = (a, b) => a === b || a.startsWith(b + '/') || b.startsWith(a + '/');
const equalFigures = (a, b) => Object.entries(a.figures ?? {}).filter(([k, v]) => typeof v === 'number' && v > 0 && v === b.figures?.[k] && !['arrested', 'charged', 'deaths'].includes(k)).map(([k]) => k);
function shingles(report) {
  const words = `${report.summary ?? ''} ${report.body ?? ''}`.toLowerCase().replace(/\[@[^\]]*\]/g, '').match(/[\p{L}\p{N}]+/gu) ?? [];
  return new Set(words.slice(3).map((_, i) => words.slice(i, i + 4).join(' ')));
}

export function findDuplicateReportCandidates(reports) {
  const titleTokens = reports.map(r => tokens(r.title));
  const frequencies = new Map();
  for (const set of titleTokens) for (const token of set) frequencies.set(token, (frequencies.get(token) ?? 0) + 1);
  const weight = token => Math.log(2 + reports.length / (frequencies.get(token) ?? 1));
  const similarity = (a, b) => {
    const shared = [...a].filter(w => b.has(w));
    const sum = set => [...set].reduce((n, w) => n + weight(w), 0);
    return { shared, score: shared.reduce((n, w) => n + weight(w), 0) / Math.max(sum(a), sum(b), 1) };
  };
  const sources = reports.map(r => new Set((r.sources ?? []).filter(s => s.url).map(s => sourceKey(s.url))));
  const places = reports.map(r => tokens(r.place));
  const bodyShingles = reports.map(shingles);
  const names = reports.map(r => new Set([...`${r.title} ${r.summary ?? ''} ${r.body ?? ''}`.matchAll(/\b([A-Z][\p{L}’'-]{2,})\s+([A-Z][\p{L}’'-]{2,})\b/gu)].map(m => m[0])));
  const nameFrequency = new Map();
  for (const set of names) for (const name of set) nameFrequency.set(name, (nameFrequency.get(name) ?? 0) + 1);
  const candidates = [];
  for (let i = 0; i < reports.length; i++) for (let j = i + 1; j < reports.length; j++) {
    const a = reports[i], b = reports[j];
    const sharedSources = [...sources[i]].filter(url => sources[j].has(url));
    if (!compatibleCountry(a.country, b.country) && !sharedSources.length) continue;
    const days = Math.abs(Date.parse(a.date) - Date.parse(b.date)) / 86400000;
    const title = similarity(titleTokens[i], titleTokens[j]);
    const figures = equalFigures(a, b);
    const place = similarity(places[i], places[j]);
    const rareNames = [...names[i]].filter(n => names[j].has(n) && nameFrequency.get(n) <= 6);
    let overlap = 0;
    if (rareNames.length) overlap = [...bodyShingles[i]].filter(s => bodyShingles[j].has(s)).length / Math.max(1, Math.min(bodyShingles[i].size, bodyShingles[j].size));
    const nameInTitles = rareNames.some(n => a.title.includes(n) && b.title.includes(n));
    const sameSentence = a.figures?.prison && a.figures.prison === b.figures?.prison;
    const titleMatch = title.score >= 0.48 && title.shared.length >= 2;
    const nearbyMatch = days <= 8 && title.score >= 0.28 && (figures.length || place.score >= 0.65);
    const caseMatch = rareNames.length && (figures.length || nameInTitles || overlap >= 0.15 || (sameSentence && days <= 60));
    if (!sharedSources.length && !titleMatch && !nearbyMatch && !caseMatch) continue;
    let score = title.score * 50 + Math.min(sharedSources.length, 3) * 12 + (days <= 7 ? 6 : days <= 60 ? 2 : 0) + figures.length * 7;
    if (sharedSources.length && title.score < 0.2) score -= 25;
    if (caseMatch) score += Math.min(overlap * 30, 10);
    candidates.push({ a: a.file, b: b.file, titles: [a.title, b.title], countries: [a.country, b.country], dates: [a.date, b.date], days, score: +score.toFixed(1), titleSimilarity: +title.score.toFixed(2), sharedTitleTokens: title.shared, sharedSources, equalFigures: figures, ...(caseMatch ? { sharedNames: rareNames, bodyOverlap: +overlap.toFixed(2) } : {}) });
  }
  return candidates.sort((a, b) => b.score - a.score || a.a.localeCompare(b.a) || a.b.localeCompare(b.b));
}
