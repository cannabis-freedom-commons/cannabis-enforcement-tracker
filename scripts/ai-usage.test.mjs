import { test } from 'node:test';
import assert from 'node:assert/strict';
import { summarizeAIUsage } from '../src/lib/ai-usage.js';
import { runCost } from '../src/lib/model-cost.js';
const bucket = (extra={}) => ({model:'opencode-go/deepseek-v4.1-flash#max',started_at:1793275200,runs:2,running:1,input_tokens:1000,output_tokens:200,reasoning_tokens:50,cache_read_tokens:400,cache_write_tokens:10,cost_usd:0,...extra});
test('AI usage aggregates models without double counting cache or reasoning, and keeps reported costs',()=>{
 const a=bucket(),b=bucket({cost_usd:0.25,running:0});
 const c=bucket({model:'unknown-model',runs:1,running:0});
 const {rows,totals}=summarizeAIUsage([a,b,c],{price_usd:10,allowance_usd:100});
 assert.equal(rows.length,2);
 assert.equal(totals.runs,5);
 assert.equal(totals.input,4230);
 assert.equal(totals.output,750);
 assert.equal(totals.cached,1200);
 assert.equal(totals.reasoning,150);
 assert.equal(totals.unpriced,1);
 assert.equal(totals.cost,runCost(a)+0.25);
 assert.equal(totals.planCost,totals.cost/10);
});
test('empty usage has zero totals, and a reported cost works for an unknown model',()=>{
 assert.equal(summarizeAIUsage([],null).totals.runs,0);
 const {totals}=summarizeAIUsage([bucket({model:'other',cost_usd:2})],null);
 assert.equal(totals.cost,2);
 assert.equal(totals.unpriced,0);
 assert.equal(totals.planCost,0);
});

test('mixed providers keep separate plan ratios when aggregating multiple buckets',()=>{
 const old=bucket({input_tokens:1e6,output_tokens:1e6,cache_read_tokens:0,cache_write_tokens:0,reasoning_tokens:0});
 const muse=bucket({model:'swancloud/muse-spark-1.3-contributor#high',cost_usd:0.3});
 const flash=bucket({model:'swancloud/deepseek-v4.1-flash',cost_usd:0.24});
 const {rows,totals}=summarizeAIUsage([old,muse,muse,flash],{price_usd:10,allowance_usd:180});
 assert.equal(totals.unpriced,0);
 assert(Math.abs(rows.find(r=>r.model===muse.model).planCost-0.04)<1e-12);
 assert(Math.abs(totals.planCost-(runCost(old)/18+0.6/15+0.24/15))<1e-12);
 assert.equal(summarizeAIUsage([muse],null).totals.planCost,0.02);
});

test('efficiency per model splits cost and time by outcome', async () => {
  const { summarizeEfficiency, formatDuration, modelParts } = await import('../src/lib/ai-usage.js');
  const b = (category, runs, cost, timed, seconds) => ({ model: 'xai/grok-4.7#low', category, runs, running: 0, input_tokens: 0, output_tokens: 0, reasoning_tokens: 0, cache_read_tokens: 0, cache_write_tokens: 0, cost_usd: cost, timed, duration_seconds: seconds, started_at: 1 });
  const [m] = summarizeEfficiency([b('new', 4, 0.8, 4, 400), b('update', 2, 0.2, 2, 100), b('out_of_scope', 1, 0.05, 1, 20), b('unfinished', 1, 0.15, 0, 0)],
    [{ model: 'xai/grok-4.7#low', category: 'new', runs: 4, median_seconds: 90, p90_seconds: 160 }]);
  assert.equal(m.outcomes.new.avgCost, 0.2);
  assert.equal(m.outcomes.new.avgSeconds, 100);
  assert.equal(m.outcomes.new.median, 90);
  assert.equal(m.outcomes.update.avgCost, 0.1);
  assert.equal(m.perNew, (0.8 + 0.2 + 0.05 + 0.15) / 4);
  assert.deepEqual([m.provider, m.name, m.effort], ['xai', 'grok-4.7', 'low']);
  assert.deepEqual(modelParts('m'), { provider: '', name: 'm', effort: '' });
  assert.deepEqual([formatDuration(45), formatDuration(200), formatDuration(3900)], ['45 s', '3 min 20 s', '1 h 5 min']);
});
