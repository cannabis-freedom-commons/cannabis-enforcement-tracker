// USD per million tokens. Operator-supplied Swancloud rates include no peak or cache
// discount. DeepSeek OpenCode Go rates: https://opencode.ai/v2/docs/console/go (half price
// outside 01:00–04:00 and 06:00–10:00 UTC weekdays). Reasoning is billed as output.
export const MODEL_PRICES = {
  // Swancloud rates and 15× subscription supplied by the operator (USD / million tokens).
  // No separate cached-input rate supplied: estimate it at the standard input price.
  "swancloud/muse-spark-1.3-contributor": { input: 0.100, output: 0.200, planMultiplier: 15 },
  "swancloud/deepseek-v4.1-flash": { input: 0.150, output: 0.090, planMultiplier: 15 },
  // Operator-supplied API rates and 6× plan; no time-of-day adjustment.
  "opencode-go/muse-spark-1.3-contributor": { input: 0.10, output: 0.20, cached: 0.002, planMultiplier: 6 },
  "opencode-go/deepseek-v4.1-flash": { input: 0.15, output: 0.6, cached: 0.003, peakInput: 0.3, peakOutput: 1.2, peakCached: 0.006 },
};

// a model id can carry a reasoning variant ("opencode-go/deepseek-v4.1-flash#max"), which does not
// change the model or its price
export const baseModel = (model) => String(model || "").split("#")[0];

export function isPeak(seconds) {
  const date = new Date((Number(seconds) || 0) * 1000);
  if (Number.isNaN(date.getTime())) return false;
  const day = date.getUTCDay(), hour = date.getUTCHours();
  return day >= 1 && day <= 5 && ((hour >= 1 && hour < 4) || (hour >= 6 && hour < 10));
}

// the run's cost in USD: what OpenCode reported if it did, otherwise its tokens at the price for
// the hour it started; 0 when neither is known
export function runCost(run) {
  const reported = Number(run?.cost_usd || 0);
  if (reported > 0) return reported;
  const price = MODEL_PRICES[baseModel(run?.model)];
  if (!price) return 0;
  const peak = isPeak(run.started_at || run.created_at);
  const input = peak ? (price.peakInput ?? price.input) : price.input;
  const output = peak ? (price.peakOutput ?? price.output) : price.output;
  const cached = (peak ? price.peakCached : undefined) ?? price.cached ?? input;
  const n = (k) => Number(run?.[k] || 0);
  // a cache write is billed as input where the model lists no price of its own for it
  return ((n("input_tokens") + n("cache_write_tokens")) * input + (n("output_tokens") + n("reasoning_tokens")) * output + n("cache_read_tokens") * cached) / 1e6;
}

// Estimated subscription allocation: the model’s fixed plan multiplier takes priority
// over the configured price/allowance ratio. A missing plan returns zero.
export function modelPlan(model, plan) {
  const multiplier = MODEL_PRICES[baseModel(model)]?.planMultiplier;
  if (multiplier > 0) return { multiplier };
  return plan?.multiplier > 0 || (plan?.price_usd > 0 && plan?.allowance_usd > 0) ? plan : null;
}
export function planDescription(plan) {
  return plan?.multiplier > 0 ? `${plan.multiplier}× plan (API value ÷ ${plan.multiplier})`
    : `$${plan?.price_usd} a month plan with $${plan?.allowance_usd} of usage`;
}
export function displayedRunCost(run, plan) {
  const api = runCost(run);
  return planCost(api, plan, run?.model) || api;
}
export function planCost(apiCost, plan, model) {
  plan = modelPlan(model, plan);
  if (plan?.multiplier > 0) return apiCost / plan.multiplier;
  const price = Number(plan?.price_usd || 0), allowance = Number(plan?.allowance_usd || 0);
  return price > 0 && allowance > 0 ? (apiCost * price) / allowance : 0;
}

// true when the cost shown is worked out from tokens rather than reported by OpenCode
export function costEstimated(run) {
  return !(Number(run?.cost_usd || 0) > 0) && Boolean(MODEL_PRICES[baseModel(run?.model)]);
}

// A run costs a fraction of a cent, so keep four decimals ($0.0001) rather than rounding to cents;
// the smallest runs get six so they do not show as $0.0000. 0 (unknown) is shown as a dash.
export function formatUSD(value) {
  const v = Number(value) || 0;
  if (!v) return "—";
  return `$${v.toFixed(v < 0.0001 ? 6 : 4)}`;
}
