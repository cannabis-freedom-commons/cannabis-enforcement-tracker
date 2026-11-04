// The Statistics page's hover and focus tooltip: any element with data-tip inside a [data-chart]
// shows its title, total and rows ([colour, label, value: a number, or text shown as it is]) in the timeline tooltip's style.
const tip = document.createElement("div");
tip.className = "report-timeline-tooltip";
tip.setAttribute("role", "tooltip");
tip.id = "chart-tooltip";
tip.hidden = true;
document.body.append(tip);
const el = (tag, text, className) => Object.assign(document.createElement(tag), { textContent: text, ...(className && { className }) });
let active = null;
const hide = () => { active?.removeAttribute("aria-describedby"); active = null; tip.hidden = true; };
function show(target) {
  if (target === active) return;
  let data;
  try { data = JSON.parse(target.dataset.tip); } catch { return; }
  active = target;
  tip.replaceChildren(el("strong", data.t), el("span", data.s, "timeline-tooltip-total"));
  for (const [color, label, value] of data.rows || []) {
    const row = el("div", "", "timeline-tooltip-row"), dot = el("i");
    if (color) dot.style.background = color;
    row.append(dot, el("span", label), el("b", typeof value === "number" ? value.toLocaleString("en-GB") : String(value)));
    tip.append(row);
  }
  tip.hidden = false;
  target.setAttribute("aria-describedby", tip.id);
  const box = target.getBoundingClientRect(), t = tip.getBoundingClientRect();
  const left = Math.max(12, Math.min(innerWidth - t.width - 12, box.left + box.width / 2 - t.width / 2));
  const above = box.top - t.height - 10;
  tip.style.left = `${left}px`;
  tip.style.top = `${Math.max(12, above >= 12 ? above : box.bottom + 10)}px`;
}
for (const chart of document.querySelectorAll("[data-chart]")) {
  const at = (e) => e.target.closest?.("[data-tip]");
  chart.addEventListener("pointerover", (e) => { if (e.pointerType !== "touch" && at(e)) show(at(e)); });
  chart.addEventListener("pointerout", (e) => { if (!chart.contains(e.relatedTarget) || !e.relatedTarget?.closest?.("[data-tip]")) hide(); });
  chart.addEventListener("focusin", (e) => at(e) && show(at(e)));
  chart.addEventListener("focusout", hide);
}
addEventListener("scroll", hide, { capture: true, passive: true });
addEventListener("keydown", (e) => e.key === "Escape" && hide());
