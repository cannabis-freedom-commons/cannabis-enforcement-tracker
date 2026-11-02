import { runCost, planCost, MODEL_PRICES, baseModel } from './model-cost.js';

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
