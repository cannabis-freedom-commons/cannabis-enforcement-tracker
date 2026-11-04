// Read-only scan of every report type and year. Does not delete, hide or merge reports.
import fs from 'node:fs';
import path from 'node:path';
import { load, JSON_SCHEMA } from 'js-yaml';
import { findDuplicateReportCandidates } from './lib/duplicate-reports.mjs';
const args = process.argv.slice(2);
const json = args.includes('--json');
const country = args.find(a => a.startsWith('--country='))?.slice(10);
const files = args.filter(a => !a.startsWith('--'));
for (const arg of args) if (arg.startsWith('--') && arg !== '--json' && !arg.startsWith('--country=')) throw new Error(`Unknown option: ${arg}`);
const paths = files.length ? files : fs.readdirSync('reports', { recursive: true }).filter(f => f.endsWith('.md')).map(f => path.join('reports', f));
const reports = paths.map(file => {
  const front = fs.readFileSync(file, 'utf8').match(/^---\r?\n([\s\S]*?)\r?\n---\r?\n([\s\S]*)$/);
  if (!front) throw new Error(`${file}: missing front matter`);
  return { ...load(front[1], { schema: JSON_SCHEMA }), file, body: front[2] };
}).filter(r => !country || r.country === country || r.country.startsWith(country + '/'));
const candidates = findDuplicateReportCandidates(reports);
const result = { reportsScanned: reports.length, candidatePairs: candidates.length, notes: ['Review aid only; a shared source, person, number or score does not establish the same event.', 'Includes all years and report types by default; court stages may be years apart.', 'Distinct raids in roundups and campaign totals can legitimately share sources or quantities.', 'An empty candidate list is not proof that every event is unique.'], candidates };
if (json) console.log(JSON.stringify(result, null, 2));
else {
  console.log(`${reports.length} reports screened; ${candidates.length} candidate pairs for review (no files changed).`);
  for (const [i, p] of candidates.entries()) console.log(`\n${i + 1}. score ${p.score}; ${p.days} days apart; ${p.sharedSources.length} shared sources\n   ${p.a}: ${p.titles[0]}\n   ${p.b}: ${p.titles[1]}`);
}
