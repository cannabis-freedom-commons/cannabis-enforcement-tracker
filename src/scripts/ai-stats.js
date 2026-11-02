import { summarizeAIUsage } from '../lib/ai-usage.js';
import { formatUSD, modelPlan } from '../lib/model-cost.js';
import { watchSubmissionUpdates } from '../lib/ai-activity.js';

const root = document.querySelector('[data-ai-stats]');
if (root?.dataset.api) initialize(root);
function initialize(root) {
  const api = root.dataset.api.replace(/\/$/, '');
  const $ = id => document.getElementById(id);
  const period = $('ai-stats-period');
  const fmt = n => n.toLocaleString('en-GB');
  const money = n => n ? formatUSD(n) : '$0.0000';
  const node = (tag, text, cls) => { const el = document.createElement(tag); el.textContent = text; if (cls) el.className = cls; return el; };
  let busy = false, pending = false;
  async function refresh() {
    if (document.hidden) return;
    if (busy) { pending = true; return; }
    busy = true;
    const selected = period.value;
    try {
      const response = await fetch(`${api}/v1/stats/ai?period=${selected}`, { credentials:'omit', signal:AbortSignal.timeout(15000) });
      if (!response.ok) throw new Error('unavailable');
      const data = await response.json();
      if (selected !== period.value) { pending = true; return; }
      const {rows, totals} = summarizeAIUsage(data.usage, data.plan);
      const hasPlan = Boolean(modelPlan("", data.plan)) || rows.some(row => modelPlan(row.model, data.plan));
      const tiles = [
        [fmt(totals.runs), 'Research runs'], [fmt(totals.running), 'Running now'],
        [fmt(totals.input), 'Input tokens, including cache'], [fmt(totals.output), 'Output tokens, including reasoning'],
        [money(totals.cost), 'API cost / estimated value'],
        ...(hasPlan ? [[money(totals.planCost), 'Estimated plan share']] : []),
      ];
      $('ai-stats-tiles').replaceChildren(...tiles.map(([value,label]) => {
        const tile = node('div', '', 'tile'); tile.append(node('div',value,'big'),node('div',label,'label')); return tile;
      }));
      $('ai-stats-models').replaceChildren(...rows.map(row => {
        const tr = document.createElement('tr');
        tr.append(node('td',row.model));
        for (const value of [fmt(row.runs),fmt(row.input),fmt(row.output),row.unpriced === row.runs ? '—' : money(row.cost)]) tr.append(node('td',value,'num'));
        if (hasPlan) tr.append(node('td', row.unpriced === row.runs ? '—' : money(row.planCost), 'num'));
        return tr;
      }));
      root.querySelector('[data-plan-cost]').hidden = !hasPlan;
      $('ai-stats-status').textContent = `${rows.length ? 'Updated' : 'No research runs recorded for this period. Updated'} ${new Date(data.updated_at * 1000).toLocaleTimeString()}.`;
      $('ai-stats-note').textContent = `${fmt(totals.cached)} cached input tokens · ${fmt(totals.reasoning)} reasoning output tokens. Costs use OpenCode telemetry where available, otherwise the configured model pricing, including peak/off-peak start time where applicable. These are usage estimates, separate from budget reservations.${hasPlan ? ' Plan share uses the model’s plan multiplier, or the configured subscription price and allowance; it is not an additional bill.' : ''}${totals.unpriced ? ` Cost unavailable for ${fmt(totals.unpriced)} ${totals.unpriced === 1 ? "run" : "runs"}; those runs are excluded from cost totals.` : ''} Startup connectivity probes are excluded. Running and interrupted runs may have incomplete telemetry.`;
      $('ai-stats-content').hidden = false;
    } catch {
      if (selected === period.value) $('ai-stats-status').textContent = 'AI usage is unavailable. Start or update the backend; this page retries automatically.';
    } finally {
      busy = false;
      if (pending) { pending = false; refresh(); }
    }
  }
  period.addEventListener('change', () => { $('ai-stats-content').hidden = true; $('ai-stats-status').textContent = 'Loading AI usage…'; refresh(); });
  watchSubmissionUpdates(api, refresh);
  const timer = setInterval(refresh, 30000);
  window.addEventListener('pagehide', () => clearInterval(timer), {once:true});
  refresh();
}
