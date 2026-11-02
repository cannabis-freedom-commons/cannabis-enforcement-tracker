import test from "node:test";
import assert from "node:assert/strict";
import { runCost, isPeak, costEstimated, formatUSD, baseModel, planCost, modelPlan, displayedRunCost } from "../src/lib/model-cost.js";

const model = "opencode-go/deepseek-v4.1-flash";
// 26 October 2026 is a Monday; 12:00 UTC is off-peak
const offPeak = Date.UTC(2026, 9, 26, 12) / 1000;
const peak = Date.UTC(2026, 9, 26, 2) / 1000;

test("a million tokens each way cost the published price off-peak", () => {
  assert.equal(runCost({ model, input_tokens: 1_000_000, output_tokens: 1_000_000, started_at: offPeak }), 0.75);
});

test("peak hours cost double", () => {
  assert.equal(runCost({ model, input_tokens: 1_000_000, output_tokens: 1_000_000, started_at: peak }), 1.5);
});

test("peak hours are 01:00-04:00 and 06:00-10:00 UTC on weekdays", () => {
  assert.equal(isPeak(Date.UTC(2026, 9, 26, 1) / 1000), true);
  assert.equal(isPeak(Date.UTC(2026, 9, 26, 3, 59) / 1000), true);
  assert.equal(isPeak(Date.UTC(2026, 9, 26, 4) / 1000), false);
  assert.equal(isPeak(Date.UTC(2026, 9, 26, 6) / 1000), true);
  assert.equal(isPeak(Date.UTC(2026, 9, 26, 10) / 1000), false);
  // Saturday and Sunday are off-peak all day
  assert.equal(isPeak(Date.UTC(2026, 9, 31, 2) / 1000), false);
  assert.equal(isPeak(Date.UTC(2026, 10, 1, 2) / 1000), false);
});

test("a reported cost wins over the token estimate", () => {
  assert.equal(runCost({ model, cost_usd: 0.02, input_tokens: 1_000_000, output_tokens: 1_000_000 }), 0.02);
  assert.equal(costEstimated({ model, cost_usd: 0.02 }), false);
  assert.equal(costEstimated({ model }), true);
});

test("an unknown model or no tokens has no cost", () => {
  assert.equal(runCost({ model: "other/model", input_tokens: 1_000_000, output_tokens: 1_000_000 }), 0);
  assert.equal(runCost({ model }), 0);
  assert.equal(costEstimated({ model: "other/model" }), false);
});

test("a reasoning variant has the same price as its model", () => {
  const variant = `${model}#max`;
  assert.equal(baseModel(variant), model);
  assert.equal(runCost({ model: variant, input_tokens: 1_000_000, output_tokens: 1_000_000, started_at: offPeak }), 0.75);
  assert.equal(costEstimated({ model: variant }), true);
});

test("the example run is a fraction of a cent off-peak", () => {
  const cost = runCost({ model, input_tokens: 39_521, output_tokens: 1_619, started_at: offPeak });
  assert.ok(Math.abs(cost - 0.00689955) < 1e-9);
});

test("costs keep four decimals, and six below a ten-thousandth of a dollar", () => {
  assert.equal(formatUSD(0.0137991), "$0.0138");
  assert.equal(formatUSD(0.00689955), "$0.0069");
  assert.equal(formatUSD(1.23456), "$1.2346");
  assert.equal(formatUSD(0.0001), "$0.0001");
  assert.equal(formatUSD(0.000069), "$0.000069");
  assert.equal(formatUSD(0), "—");
});

for (const [model,input,output] of [
 ["swancloud/muse-spark-1.3-contributor",0.100,0.200],
 ["swancloud/deepseek-v4.1-flash",0.150,0.090],
]) {
 test(`${model} uses the supplied API rates and 15× plan at every hour`,()=>{
  for(const started_at of [peak,offPeak]) {
   const run={model:model+"#max",input_tokens:1e6,output_tokens:1e6,started_at};
   assert(Math.abs(runCost(run)-(input+output))<1e-12);
   assert(Math.abs(displayedRunCost(run,{price_usd:10,allowance_usd:180})-(input+output)/15)<1e-12);
   assert.equal(modelPlan(run.model,null).multiplier,15);
  }
  const run={model,input_tokens:1e6,cache_read_tokens:1e6,cache_write_tokens:1e6,output_tokens:1e6,reasoning_tokens:1e6};
  assert(Math.abs(runCost(run)-(3*input+2*output))<1e-12);
  assert.equal(runCost({...run,cost_usd:3}),3);
  assert.equal(planCost(3,null,model),0.2);
 });
}
test('Swancloud plan does not override historical OpenCode or unknown models',()=>{
 assert.equal(planCost(18,{price_usd:10,allowance_usd:180},model),1);
 assert.equal(modelPlan('other/model',null),null);
});
