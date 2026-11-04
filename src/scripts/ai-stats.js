import { summarizeAIUsage, summarizeEfficiency, OUTCOMES, formatDuration } from '../lib/ai-usage.js';
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
      lastEfficiency = summarizeEfficiency(data.usage, data.durations ?? [], data.plan);
      drawEfficiency(lastEfficiency);
      $('ai-stats-content').hidden = false;
    } catch {
      if (selected === period.value) $('ai-stats-status').textContent = 'AI usage is unavailable. Start or update the backend; this page retries automatically.';
    } finally {
      busy = false;
      if (pending) { pending = false; refresh(); }
    }
  }
  // per model, what a new report, an update and an out-of-scope submission cost and take
  const COLORS = { new: '#6cb0f5', update: '#8fd18a', out_of_scope: '#c9a35a' };
  // costs as API prices or as the plan share (where a plan covers the model); the reader's choice
  let lastEfficiency = null, mode = 'api';
  try { if (localStorage.getItem('ai-cost-mode') === 'plan') mode = 'plan'; } catch { /* no storage */ }
  const avg = (o) => (mode === 'plan' ? o?.planAvgCost : o?.avgCost);
  const perNew = (m) => (mode === 'plan' ? m.planPerNew : m.perNew);
  $('ai-eff-mode').addEventListener('click', (e) => {
    const next = e.target.closest('[data-mode]')?.dataset.mode;
    if (!next || next === mode) return;
    mode = next;
    try { localStorage.setItem('ai-cost-mode', mode); } catch { /* no storage */ }
    if (lastEfficiency) drawEfficiency(lastEfficiency);
  });
  const MIN_RUNS = 3;
  const label = (m) => { const el = node('span', '', 'eff-label'); el.append(node('span', m.name), node('small', [m.provider, m.effort].filter(Boolean).join(' · ') || ' ')); el.title = m.model; return el; };
  const tip = (title, total, rows) => JSON.stringify({ t: title, s: total, rows });
  function panel(key, title, help, rows, value, text, extra, costs = false) {
    const box = node('div', '', 'eff-panel');
    const h = node('h4', ''); const dot = node('i', ''); dot.style.setProperty('--c', COLORS[key]); h.append(dot, title);
    box.append(h, node('p', help));
    const list = rows.filter((m) => (m.outcomes[key]?.runs ?? 0) >= MIN_RUNS && value(m) != null).sort((a, b) => value(a) - value(b));
    if (!list.length) { box.append(node('p', 'Not enough runs yet.', 'eff-empty')); return box; }
    const max = Math.max(...list.map((m) => extra?.(m) ?? value(m))) || 1;
    const ol = node('ol', '', 'eff-bars');
    list.forEach((m, i) => {
      const o = m.outcomes[key], li = node('li', '');
      if (i === 0 && list.length > 1) li.className = 'eff-best';
      li.tabIndex = 0;
      li.style.setProperty('--c', COLORS[key]);
      li.dataset.tip = tip(`${m.name} · ${title}`, `${fmt(o.runs)} runs${m.provider ? ` via ${m.provider}` : ''}${m.effort ? `, ${m.effort}` : ''}`, [
        ['', 'Average API price', o.avgCost == null ? '—' : money(o.avgCost)],
        ...(m.onPlan ? [['', 'Average plan share', money(o.planAvgCost)]] : []), ['', 'Average time', formatDuration(o.avgSeconds)],
        ['', 'Median time', formatDuration(o.median)], ['', 'Slowest tenth from', formatDuration(o.p90)]]);
      const track = node('span', '', 'eff-track'), bar = node('i', '');
      bar.style.width = `${(value(m) / max) * 100}%`;
      track.append(bar);
      if (extra?.(m) != null) { const tick = node('b', ''); tick.style.left = `${(extra(m) / max) * 100}%`; track.append(tick); }
      const num = node('span', text(m), 'eff-num');
      if (costs && mode === 'plan' && m.onPlan) num.append(node('span', 'plan', 'eff-plan'));
      li.append(label(m), num, track);
      ol.append(li);
    });
    box.append(ol);
    return box;
  }
  function drawEfficiency(models) {
    const box = $('ai-efficiency');
    box.hidden = !models.some((m) => OUTCOMES.some(([k]) => (m.outcomes[k]?.runs ?? 0) >= MIN_RUNS));
    if (box.hidden) return;
    const anyPlan = models.some((m) => m.onPlan);
    $('ai-eff-mode').hidden = !anyPlan;
    if (!anyPlan) mode = 'api';
    for (const b of $('ai-eff-mode').querySelectorAll('[data-mode]')) b.setAttribute('aria-pressed', String(b.dataset.mode === mode));
    $('ai-eff-mode-note').textContent = mode === 'plan' ? 'As plan share: what the runs take of a subscription where a plan covers the model (marked “plan”); other models at API price.' : 'At API prices (or the cost OpenCode reports), whatever plan the model runs on.';
    $('ai-eff-cost').replaceChildren(...OUTCOMES.map(([key, title, help]) => panel(key, title, help, models, (m) => avg(m.outcomes[key]), (m) => money(avg(m.outcomes[key])), null, true)));
    $('ai-eff-time').replaceChildren(...OUTCOMES.map(([key, title, help]) => panel(key, title, help, models, (m) => m.outcomes[key]?.median, (m) => formatDuration(m.outcomes[key].median), (m) => m.outcomes[key]?.p90)));
    const cell = (o) => {
      const td = node('td', o?.runs ? (avg(o) == null ? '—' : money(avg(o))) : '–', 'num');
      if (o?.runs) td.append(node('small', `${fmt(o.runs)} · ${formatDuration(o.median ?? o.avgSeconds)}`));
      return td;
    };
    $('ai-eff-rows').replaceChildren(...models.map((m) => {
      const tr = document.createElement('tr'), name = node('td', m.name);
      name.append(node('small', [m.provider, m.effort].filter(Boolean).join(' · ')));
      tr.append(name, node('td', fmt(m.runs), 'num'), node('td', perNew(m) == null ? '—' : `${money(perNew(m))}${mode === 'plan' && m.onPlan ? ' plan' : ''}`, 'num'),
        cell(m.outcomes.new), cell(m.outcomes.update), cell(m.outcomes.out_of_scope), cell(m.outcomes.unfinished));
      return tr;
    }));
    $('ai-eff-note').textContent = 'Each cell: average cost per run, then runs · median time. "All in" divides everything the model ran, failed and rejected runs included, by the new reports it wrote. Duplicates, too little evidence and reviews are counted in the runs but not shown apart. Costs follow the switch above the charts: API prices (reported, or estimated from tokens) or plan share.';
  }
  period.addEventListener('change', () => { $('ai-stats-content').hidden = true; $('ai-stats-status').textContent = 'Loading AI usage…'; refresh(); });
  watchSubmissionUpdates(api, refresh);
  const timer = setInterval(refresh, 30000);
  window.addEventListener('pagehide', () => clearInterval(timer), {once:true});
  refresh();
}
