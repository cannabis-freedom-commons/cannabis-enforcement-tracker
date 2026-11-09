// Render outside the scrollable map pane so the tooltip cannot be clipped.
export function timelineTooltip(box, types) {
  const tooltip = document.createElement("div");
  tooltip.id = "report-timeline-tooltip";
  tooltip.className = "report-timeline-tooltip";
  tooltip.setAttribute("role", "tooltip");
  tooltip.hidden = true;
  document.body.append(tooltip);
  let active = null, dismiss;
  const hide = () => {
    clearTimeout(dismiss);
    active?.removeAttribute("aria-describedby");
    active = null;
    tooltip.hidden = true;
  };
  const line = (tag, text, className) => {
    const node = document.createElement(tag);
    node.textContent = text;
    if (className) node.className = className;
    return node;
  };
  const show = (button) => {
    hide();
    active = button;
    const categories = JSON.parse(button.dataset.categories);
    const total = Number(button.dataset.total);
    tooltip.replaceChildren(
      line("strong", button.dataset.periodLabel),
      line("span", `${total.toLocaleString("en-GB")} ${total === 1 ? "report" : "reports"}`, "timeline-tooltip-total"),
    );
    for (const [id, type] of Object.entries(types)) {
      if (!categories[id]) continue;
      const row = document.createElement("div");
      row.className = "timeline-tooltip-row";
      const dot = document.createElement("i");
      dot.style.background = type.color;
      row.append(dot, line("span", type.label), line("b", categories[id].toLocaleString("en-GB")));
      tooltip.append(row);
    }
    tooltip.append(line("span", button.getAttribute("aria-pressed") === "true" ? "Selected · click again to clear" : `Click to filter this ${button.dataset.period}`, "timeline-tooltip-hint"));
    tooltip.hidden = false;
    button.setAttribute("aria-describedby", tooltip.id);
    // under the whole chart, lined up with the bar, so it covers neither the bars nor the page
    // above them; above the chart when the window has no room below it
    const bar = button.getBoundingClientRect(), chart = box.getBoundingClientRect(), tip = tooltip.getBoundingClientRect();
    const left = Math.max(12, Math.min(window.innerWidth - tip.width - 12, bar.left + bar.width / 2 - tip.width / 2));
    // the year labels may hang below the bars' box
    const labels = box.querySelector(".months-axis")?.getBoundingClientRect();
    const below = Math.max(chart.bottom, labels?.bottom ?? 0) + 10, above = chart.top - tip.height - 10;
    const top = below + tip.height <= window.innerHeight - 12 || above < 12 ? Math.min(below, window.innerHeight - tip.height - 12) : above;
    tooltip.style.left = `${left}px`;
    tooltip.style.top = `${top}px`;
  };
  const hideSoon = () => {
    clearTimeout(dismiss);
    dismiss = setTimeout(hide, 150);
  };
  tooltip.addEventListener("pointerenter", () => clearTimeout(dismiss));
  const pointAtBar = (event) => {
    if (event.pointerType === "touch") return;
    const button = event.target.closest("button[data-m]");
    if (!button) return;
    clearTimeout(dismiss);
    if (button !== active || tooltip.hidden) show(button);
  };
  box.addEventListener("pointerover", pointAtBar);
  // Scroll or rerender can dismiss a tooltip without another pointerenter event.
  box.addEventListener("pointermove", pointAtBar);
  box.addEventListener("pointerout", (event) => {
    // Crossing a narrow gap or a stack segment is still inside the chart.
    if (box.contains(event.relatedTarget) || tooltip.contains(event.relatedTarget)) return;
    hideSoon();
  });
  tooltip.addEventListener("pointerleave", (event) => {
    if (!box.contains(event.relatedTarget)) hide();
  });
  box.addEventListener("focusin", (event) => {
    const button = event.target.closest("button[data-m]");
    if (button) show(button);
  });
  box.addEventListener("focusout", hide);
  document.addEventListener("keydown", (event) => { if (event.key === "Escape") hide(); });
  window.addEventListener("scroll", hide, { capture: true, passive: true });
  window.addEventListener("resize", hide);
  return hide;
}
