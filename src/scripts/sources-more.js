// A report's list of sources shows its first few; "Show all" opens the rest, and pressing it again
// folds them. A source a citation links to (#ref-12) shows even while folded (report.css). It
// listens on the document, so it also works for a report read in the map's side pane.
document.addEventListener("click", (e) => {
  const button = e.target.closest?.("[data-sources-more]");
  if (!button) return;
  const list = button.previousElementSibling?.matches("[data-sources]") ? button.previousElementSibling : null;
  if (!list) return;
  const open = list.classList.toggle("all");
  button.textContent = open ? "Show fewer" : button.dataset.label;
  button.setAttribute("aria-expanded", String(open));
  if (!open) list.scrollIntoView({ block: "nearest" });
});
