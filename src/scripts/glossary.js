import { glossaryParts, GLOSSARY_SKIP } from "../lib/glossary.js";

const main = document.querySelector("main");
if (main) {
  let tip, current, hideTimer, scanTimer, showTimer;
  // a pointer resting on a term for a second opens its explanation (not one passing over the text);
  // while one is open, the next term shows at once
  const SHOW_DELAY = 1000;
  let touchFocus = false;
  const pending = new Set();
  function hide() {
    clearTimeout(hideTimer); clearTimeout(showTimer);
    current?.removeAttribute("aria-describedby"); current = null;
    if (tip) tip.hidden = true;
  }
  function show(term) {
    clearTimeout(hideTimer);
    if (!tip) {
      tip = document.createElement("div"); tip.id = "glossary-tooltip";
      tip.className = "glossary-tooltip"; tip.setAttribute("role", "tooltip");
      tip.addEventListener("pointerenter", () => clearTimeout(hideTimer));
      tip.addEventListener("pointerleave", () => { hideTimer = setTimeout(hide, 120); });
      document.body.append(tip);
    }
    if (current !== term) current?.removeAttribute("aria-describedby");
    current = term; term.setAttribute("aria-describedby", tip.id);
    const eyebrow = document.createElement("p"); eyebrow.className = "glossary-eyebrow"; eyebrow.textContent = term.dataset.eyebrow || "Explained";
    const title = document.createElement("b"); title.className = "glossary-title"; title.textContent = term.dataset.title;
    const definition = document.createElement("p"); definition.className = "glossary-definition"; definition.textContent = term.dataset.text;
    // a weight or a sum of money: its conversions as a small table, with a note under it
    let rows = null;
    try { rows = term.dataset.rows ? JSON.parse(term.dataset.rows) : null; } catch {}
    if (rows?.length) {
      const table = document.createElement("table"); table.className = "glossary-table";
      for (const [label, value] of rows) {
        const tr = table.insertRow(), th = document.createElement("th"); th.scope = "row"; th.textContent = label;
        const td = document.createElement("td"); td.textContent = value; tr.append(th, td);
      }
      const note = document.createElement("p"); note.className = "glossary-note"; note.textContent = term.dataset.note ?? "";
      tip.replaceChildren(eyebrow, title, table, ...(note.textContent ? [note] : []));
    } else tip.replaceChildren(eyebrow, title, definition);
    tip.hidden = false;
    const rect = term.getBoundingClientRect(), margin = 10, width = tip.offsetWidth, height = tip.offsetHeight;
    const below = rect.bottom + 8 + height < innerHeight || rect.top - 8 - height < 0;
    tip.style.left = `${Math.max(margin, Math.min(rect.left, innerWidth - width - margin))}px`;
    tip.style.top = `${Math.max(margin, below ? rect.bottom + 8 : rect.top - 8 - height)}px`;
  }
  const termOf = event => event.target instanceof Element ? event.target.closest(".term-g") : null;
  document.addEventListener("pointerover", event => {
    const term = termOf(event);
    if (!term || event.pointerType === "touch") return;
    clearTimeout(showTimer);
    if (tip && !tip.hidden) show(term);
    else showTimer = setTimeout(() => show(term), SHOW_DELAY);
  });
  document.addEventListener("pointerout", event => {
    const term = termOf(event);
    if (!term || term.contains(event.relatedTarget) || tip?.contains(event.relatedTarget)) return;
    clearTimeout(showTimer);
    hideTimer = setTimeout(hide, 120);
  });
  document.addEventListener("pointerdown", event => { touchFocus = event.pointerType === "touch"; }, { capture: true });
  document.addEventListener("focusin", event => { const term = termOf(event); if (term && !touchFocus) show(term); });
  document.addEventListener("focusout", event => { if (termOf(event)) hide(); });
  document.addEventListener("click", event => {
    const term = termOf(event);
    if (term instanceof HTMLAnchorElement) {
      // Atlas terms remain links. On touch, the first tap explains; a second opens CFI.
      if (event.pointerType === "touch" && (current !== term || tip?.hidden)) { event.preventDefault(); show(term); }
      return;
    }
    if (term) { if (current === term && !tip?.hidden) hide(); else show(term); }
    else if (!tip?.contains(event.target)) hide();
  });
  document.addEventListener("keydown", event => {
    touchFocus = false;
    if (event.key === "Escape") hide();
    else if (["Enter", " "].includes(event.key)) { const term = termOf(event); if (term && !(term instanceof HTMLAnchorElement)) { event.preventDefault(); if (current === term && !tip?.hidden) hide(); else show(term); } }
  });
  window.addEventListener("scroll", hide, { passive: true, capture: true });
  window.addEventListener("resize", hide);

  function mark(root, seen) {
    if (!root.isConnected || root.closest(GLOSSARY_SKIP)) return;
    const walker = document.createTreeWalker(root, NodeFilter.SHOW_TEXT, {
      acceptNode: node => node.parentElement?.closest(GLOSSARY_SKIP) || !node.textContent.trim() ? NodeFilter.FILTER_REJECT : NodeFilter.FILTER_ACCEPT,
    });
    const texts = []; while (walker.nextNode()) texts.push(walker.currentNode);
    for (const node of texts) {
      // "$" or "Rs" is the currency of the report the text belongs to
      const parts = glossaryParts(node.textContent, seen, { country: node.parentElement?.closest("[data-country]")?.dataset.country });
      if (!parts.some(part => part.entry)) continue;
      const fragment = document.createDocumentFragment();
      for (const part of parts) {
        if (!part.entry) { fragment.append(document.createTextNode(part.text)); continue; }
        const term = document.createElement(part.entry.url ? "a" : "span"); term.className = `term term-g${part.entry.url ? " term-l" : ""}`;
        if (part.entry.url) { term.href = part.entry.url; term.target = "_blank"; term.rel = "noopener noreferrer"; }
        term.tabIndex = 0; if (!part.entry.url) term.setAttribute("role", "button");
        term.dataset.termId = part.entry.id; term.dataset.title = part.entry.title ?? part.entry.id; term.dataset.text = part.entry.text;
        if (part.entry.eyebrow) term.dataset.eyebrow = part.entry.eyebrow;
        if (part.entry.rows) { term.dataset.rows = JSON.stringify(part.entry.rows); term.dataset.note = part.entry.note; }
        term.textContent = part.text; fragment.append(term);
      }
      node.replaceWith(fragment);
    }
  }
  const options = { childList: true, subtree: true, characterData: true };
  function scan() {
    observer.disconnect();
    if (current && !current.isConnected) hide();
    const seen = new Set([...main.querySelectorAll(".term-g[data-term-id]")].map(term => term.dataset.termId));
    const roots = [...pending].filter(root => root.isConnected);
    pending.clear();
    for (const root of roots) if (!roots.some(other => other !== root && other.contains(root))) mark(root, seen);
    observer.observe(main, options);
  }
  const observer = new MutationObserver(records => {
    for (const record of records) {
      const candidates = record.type === "characterData" ? [record.target] : [...record.addedNodes];
      for (const node of candidates) {
        const root = node instanceof Element ? node : node.parentElement;
        if (root && !root.closest(GLOSSARY_SKIP)) pending.add(root);
      }
    }
    if (pending.size) { clearTimeout(scanTimer); scanTimer = setTimeout(scan, 80); }
    if (current && !current.isConnected) hide();
  });
  pending.add(main); scan();
  window.addEventListener("pagehide", () => { clearTimeout(scanTimer); observer.disconnect(); hide(); });
  window.addEventListener("pageshow", event => { if (event.persisted) { pending.add(main); scan(); } });
}
