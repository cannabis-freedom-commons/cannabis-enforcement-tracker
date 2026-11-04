import { runCost, planCost, modelPlan, MODEL_PRICES, baseModel } from './model-cost.js';

export function summarizeAIUsage(buckets, plan) {
  const models = new Map();
  for (const b of buckets) {
    const row = models.get(b.model) || { model: b.model || 'Not recorded', runs: 0, running: 0, input: 0, output: 0, cached: 0, reasoning: 0, cost: 0, planCost: 0, unpriced: 0 };
    row.runs += b.runs;
    row.running += b.running;
    row.input += b.input_tokens + b.cache_read_tokens + b.cache_write_tokens;
    row.output += b.output_tokens + b.reasoning_tokens;
    row.cached += b.cache_read_tokens;
    row.reasoning += b.reasoning_tokens;
    row.cost += runCost(b);
    if (!(b.cost_usd > 0) && !MODEL_PRICES[baseModel(b.model)]) row.unpriced += b.runs;
    row.planCost = planCost(row.cost, plan, b.model);
    models.set(b.model, row);
  }
  const rows = [...models.values()].sort((a,b) => b.cost-a.cost || b.runs-a.runs || a.model.localeCompare(b.model));
  const totals = rows.reduce((sum, row) => {
    for (const key of ['runs','running','input','output','cached','reasoning','cost','planCost','unpriced']) sum[key] += row[key];
    return sum;
  }, { runs:0, running:0, input:0, output:0, cached:0, reasoning:0, cost:0, planCost:0, unpriced:0 });
  return { rows, totals };
}

// What each kind of work costs and takes per model: the outcome categories the service groups runs
// by (backend store.categorySQL); new reports, updates and out-of-scope submissions are compared.
export const OUTCOMES = [
  ['new', 'New report', 'A report about an event the tracker did not have'],
  ['update', 'Update', 'Another source or detail for a report it had'],
  ['out_of_scope', 'Outside scope', 'Not cannabis enforcement, or the Index’s matter'],
];
export const OTHER_OUTCOMES = [['other', 'Other outcomes'], ['unfinished', 'Failed or unfinished']];

// "provider/model#effort" → its parts, for a short label
export function modelParts(id) {
  const [path, effort = ''] = String(id || '').split('#');
  const slash = path.indexOf('/');
  return { provider: slash > 0 ? path.slice(0, slash) : '', name: slash > 0 ? path.slice(slash + 1) : path || 'Not recorded', effort };
}

// per model: for each outcome, the runs, cost (as summarizeAIUsage works it out), the average cost
// and time per run, and the median and p90 times; `perNew` is the cost of everything the model ran
// (failures and rejections included) for each new report it wrote. Costs are API prices; the
// `plan…` twins are the plan share where a plan covers the model (`onPlan`), else the same price.
export function summarizeEfficiency(buckets, durations = [], plan = null) {
  const models = new Map();
  const blank = () => ({ runs: 0, cost: 0, priced: 0, timed: 0, seconds: 0, median: null, p90: null });
  for (const b of buckets) {
    const m = models.get(b.model) || { model: b.model || '', runs: 0, cost: 0, outcomes: {} };
    const o = m.outcomes[b.category || 'unfinished'] ||= blank();
    const cost = runCost(b), priced = b.cost_usd > 0 || Boolean(MODEL_PRICES[baseModel(b.model)]);
    o.runs += b.runs; o.timed += b.timed || 0; o.seconds += b.duration_seconds || 0;
    if (priced) { o.cost += cost; o.priced += b.runs; }
    m.runs += b.runs; if (priced) m.cost += cost;
    models.set(b.model, m);
  }
  for (const d of durations) {
    const o = models.get(d.model)?.outcomes[d.category];
    if (o) { o.median = d.median_seconds; o.p90 = d.p90_seconds; }
  }
  return [...models.values()].map((m) => {
    for (const o of Object.values(m.outcomes)) {
      o.avgCost = o.priced ? o.cost / o.priced : null;
      o.avgSeconds = o.timed ? o.seconds / o.timed : null;
      // what the run takes of a subscription where one covers the model, else its API price
      o.planAvgCost = o.avgCost == null ? null : planCost(o.avgCost, plan, m.model) || o.avgCost;
    }
    const made = m.outcomes.new?.runs || 0, perNew = made && m.cost ? m.cost / made : null;
    return { ...m, ...modelParts(m.model), perNew, planPerNew: perNew == null ? null : planCost(perNew, plan, m.model) || perNew, onPlan: Boolean(modelPlan(m.model, plan)) };
  }).sort((a, b) => b.runs - a.runs || a.model.localeCompare(b.model));
}

// "45 s", "3 min 20 s", "1 h 5 min"
export function formatDuration(seconds) {
  if (seconds == null) return '—';
  const s = Math.round(seconds);
  if (s < 60) return `${s} s`;
  if (s < 3600) return `${Math.floor(s / 60)} min${s % 60 ? ` ${s % 60} s` : ''}`;
  return `${Math.floor(s / 3600)} h${Math.round((s % 3600) / 60) ? ` ${Math.round((s % 3600) / 60)} min` : ''}`;
}
