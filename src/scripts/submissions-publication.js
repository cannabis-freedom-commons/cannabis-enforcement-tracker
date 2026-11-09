// The publication strip on /submissions/: committed reports waiting for the next batch to main,
// reports on main waiting for the site's build, and the last batch (backend: worker/batch.go).
// From /v1/status, every 30 seconds while the page is visible.
const box = document.getElementById("register-publication");
if (box) {
  const api = (box.dataset.api || "").replace(/\/$/, "");
  const $ = (name) => box.querySelector(`[data-pub="${name}"]`);
  const num = (n) => Number(n || 0).toLocaleString("en-GB");
  const clock = (unix) => new Date(unix * 1000).toLocaleTimeString("en-GB", { hour: "2-digit", minute: "2-digit" });
  const minutes = (seconds) => {
    const m = Math.round(Math.abs(seconds) / 60);
    return m < 1 ? "less than a minute" : m < 90 ? `${m} min` : `${Math.round(m / 60)} h`;
  };
  // the site is built at :13 and :43 (.github/workflows/deploy.yml)
  const nextBuild = (from) => {
    const d = new Date(from * 1000);
    for (let i = 0; i < 120; i++, d.setUTCMinutes(d.getUTCMinutes() + 1)) {
      if (d.getUTCMinutes() === 13 || d.getUTCMinutes() === 43) { d.setUTCSeconds(0, 0); return d.getTime() / 1000; }
    }
    return from;
  };
  let data = null, poll = 0;
  const render = () => {
    const p = data?.publication;
    if (!p) return;
    box.hidden = false;
    const now = Date.now() / 1000;
    $("waiting").textContent = num(p.waiting);
    $("deploying").textContent = num(p.deploying);
    box.querySelector('[data-step="waiting"]').dataset.active = p.waiting > 0 ? "true" : "false";
    box.querySelector('[data-step="deploying"]').dataset.active = p.deploying > 0 ? "true" : "false";
    if (p.batched && p.next_batch_at) {
      const at = p.next_batch_at;
      $("next").textContent = p.waiting > 0 ? `Next batch at ${clock(at)}, in ${minutes(at - now)}` : `Next batch at ${clock(at)}`;
    } else $("next").textContent = "Each report goes to main at once";
    const build = nextBuild(now);
    $("build").textContent = `Next site build about ${clock(build)}, in ${minutes(build - now)}`;
    const last = p.last_batch;
    if (last) {
      $("last-reports").textContent = num(last.reports);
      const when = `${clock(last.merged_at)} (${minutes(now - last.merged_at)} ago)`;
      $("last").textContent = last.deployed_at ? `Pushed ${when}, on the site since ${clock(last.deployed_at)}` : `Pushed ${when}, waiting for the site`;
    } else {
      $("last-reports").textContent = "—";
      $("last").textContent = "No batch yet";
    }
    $("note").textContent = p.batched
      ? `Reports are committed to the ${p.branch} branch and go to main together every ${minutes(p.interval_seconds)}, shortly before the site is rebuilt. A report is published once the site has it.`
      : "Each report is pushed to main when it is committed, and published once the site has been rebuilt.";
  };
  const load = async () => {
    clearTimeout(poll);
    if (document.hidden) return;
    try {
      const r = await fetch(`${api}/v1/status`, { credentials: "omit", cache: "no-store" });
      if (r.ok) { data = await r.json(); render(); }
    } catch { /* keeps the last figures */ }
    poll = window.setTimeout(load, 30000);
  };
  // the countdowns move on between loads
  setInterval(() => { if (!document.hidden) render(); }, 15000);
  document.addEventListener("visibilitychange", () => { if (!document.hidden) load(); });
  load();
}
