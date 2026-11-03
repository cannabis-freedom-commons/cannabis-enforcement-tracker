// A report's sources one gesture away: the [n] numbers are hidden (html.js, report.css) and each
// cited stretch of text (a <span class="cited" data-refs="1 3">, see astro.config.mjs) shows its
// sources in a card, copied from the report's list of sources, when it is pointed at or focused;
// a tap or click pins the card until the reader taps elsewhere or presses Escape. Links and
// glossary terms inside the text keep working as before.
const body = document.querySelector(".article-body");
const spans = body ? [...body.querySelectorAll(".cited[data-refs]")] : [];
if (spans.length) {
  // the hidden numbers stay for screen readers, out of the tab order (the text itself is focusable)
  for (const a of body.querySelectorAll("sup.cite a")) a.tabIndex = -1;
  for (const span of spans) span.tabIndex = 0;

  const card = document.createElement("div");
  card.id = "cite-card";
  card.className = "cite-card";
  card.setAttribute("role", "tooltip");
  card.hidden = true;
  document.body.append(card);
  let current = null, pinned = false, showTimer, hideTimer;

  const fill = (span) => {
    const refs = span.dataset.refs.split(" ").filter(Boolean);
    const eyebrow = document.createElement("p");
    eyebrow.className = "cite-card-eyebrow";
    eyebrow.textContent = refs.length === 1 ? "Source" : `${refs.length} sources`;
    const list = document.createElement("ol");
    list.className = "cite-card-list";
    for (const n of refs) {
      const entry = document.getElementById(`ref-${n}`);
      if (!entry) continue;
      const li = document.createElement("li");
      li.value = Number(n);
      li.append(...[...entry.childNodes].map((node) => node.cloneNode(true)));
      list.append(li);
    }
    card.replaceChildren(eyebrow, list);
  };
  // under the line the pointer is on (or the last line), above it when there is no room below
  const place = (span, x, y) => {
    const rects = [...span.getClientRects()];
    const line = (y != null && rects.find((r) => y >= r.top - 2 && y <= r.bottom + 2)) || rects.at(-1);
    if (!line) return;
    const margin = 10, w = card.offsetWidth, h = card.offsetHeight;
    card.style.left = `${Math.max(margin, Math.min(x ?? line.left, innerWidth - w - margin))}px`;
    card.style.top = `${line.bottom + 8 + h <= innerHeight - margin ? line.bottom + 8 : Math.max(margin, line.top - 8 - h)}px`;
  };
  const show = (span, x, y) => {
    clearTimeout(hideTimer);
    if (current !== span) {
      current?.classList.remove("is-active");
      current?.removeAttribute("aria-describedby");
      current = span;
      span.classList.add("is-active");
      span.setAttribute("aria-describedby", card.id);
      fill(span);
    }
    card.hidden = false;
    place(span, x, y);
  };
  const hide = () => {
    clearTimeout(showTimer);
    clearTimeout(hideTimer);
    pinned = false;
    current?.classList.remove("is-active");
    current?.removeAttribute("aria-describedby");
    current = null;
    card.hidden = true;
  };
  const later = () => { clearTimeout(hideTimer); if (!pinned) hideTimer = setTimeout(hide, 160); };
  const spanOf = (target) => target instanceof Element ? target.closest(".cited[data-refs]") : null;
  // a glossary term or a link has its own behaviour: the card stays out of its way
  const ownTarget = (target) => target instanceof Element && target.closest(".term-g, a");

  body.addEventListener("pointerover", (e) => {
    if (e.pointerType === "touch" || pinned) return;
    const span = spanOf(e.target);
    if (!span || ownTarget(e.target)) { if (current) later(); return; }
    clearTimeout(showTimer);
    clearTimeout(hideTimer);
    if (span === current) return;
    showTimer = setTimeout(() => show(span, e.clientX, e.clientY), current ? 0 : 90);
  });
  body.addEventListener("pointerout", (e) => {
    if (e.pointerType === "touch" || pinned) return;
    if (spanOf(e.relatedTarget) === current && !ownTarget(e.relatedTarget)) return;
    clearTimeout(showTimer);
    if (!card.contains(e.relatedTarget)) later();
  });
  card.addEventListener("pointerenter", () => clearTimeout(hideTimer));
  card.addEventListener("pointerleave", later);
  body.addEventListener("click", (e) => {
    const span = spanOf(e.target);
    if (!span || ownTarget(e.target) || getSelection()?.toString()) return;
    if (pinned && current === span) { hide(); return; }
    show(span, e.clientX, e.clientY);
    pinned = true;
  });
  body.addEventListener("focusin", (e) => { const span = spanOf(e.target); if (span === e.target) show(span); });
  body.addEventListener("focusout", (e) => { if (!pinned && !card.contains(e.relatedTarget)) hide(); });
  document.addEventListener("pointerdown", (e) => {
    if (pinned && !card.contains(e.target) && !spanOf(e.target)) hide();
  });
  document.addEventListener("keydown", (e) => {
    if (e.key === "Escape" && current) { const span = current; hide(); span.focus({ preventScroll: true }); }
    else if ((e.key === "Enter" || e.key === " ") && spanOf(e.target) === e.target) {
      e.preventDefault();
      show(e.target);
      pinned = true;
    }
  });
  addEventListener("scroll", () => { if (!current) return; if (pinned) place(current); else hide(); }, { passive: true });
  addEventListener("resize", () => current && place(current));
}
