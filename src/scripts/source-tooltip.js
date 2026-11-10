// A report's source, pointed at in the list of sources: a card with its full title, publisher, site,
// date and language, whether there is an archived copy, and the screenshot the tracker took of the
// page when it read it (GET /v1/sources/screenshot on the submissions API; none for a page it never
// opened, or when no API is configured). It opens after the pointer rests on the line for a moment,
// stays while the pointer is on the card, and also works for a report read in the map's side pane.
const DELAY = 600;
let card = null, current = null, showTimer = 0, hideTimer = 0;

function build(li) {
  const d = li.dataset, list = li.closest("[data-sources]");
  const el = (tag, cls, text) => { const e = document.createElement(tag); if (cls) e.className = cls; if (text) e.textContent = text; return e; };
  const parts = [el("p", "srct-eyebrow", `Source ${d.n}`), el("b", "srct-pub", d.publisher || d.domain || "")];
  if (d.title) { const t = el("p", "srct-title", d.title); t.dir = "auto"; parts.push(t); }
  parts.push(el("p", "srct-meta", [d.domain, d.date, d.lang].filter(Boolean).join(" · ")));
  if (d.archived) parts.push(el("p", "srct-meta", "An archived copy is kept"));
  const api = list?.dataset.api;
  if (api && d.url) {
    const shot = el("a", "srct-shot");
    shot.href = `${api}/v1/sources/screenshot?url=${encodeURIComponent(d.url)}`;
    shot.target = "_blank"; shot.rel = "noopener noreferrer";
    const img = el("img"); img.alt = `The page as the tracker read it`; img.loading = "eager"; img.decoding = "async";
    img.addEventListener("load", () => { shot.classList.add("is-loaded"); place(); });
    img.addEventListener("error", () => shot.remove());
    img.src = shot.href;
    shot.append(img, el("span", "srct-caption", "Screenshot when the tracker read it"));
    parts.push(shot);
  }
  card.replaceChildren(...parts);
}

function place() {
  if (!card || !current) return;
  const r = current.getBoundingClientRect(), m = 10, w = card.offsetWidth, h = card.offsetHeight;
  card.style.left = `${Math.max(m, Math.min(r.left + 40, innerWidth - w - m))}px`;
  card.style.top = `${r.bottom + 6 + h <= innerHeight - m ? r.bottom + 6 : Math.max(m, r.top - 6 - h)}px`;
}

function show(li) {
  clearTimeout(hideTimer);
  if (!card) {
    card = document.createElement("div");
    card.className = "source-tip"; card.setAttribute("role", "tooltip"); card.id = "source-tip";
    card.addEventListener("pointerenter", () => clearTimeout(hideTimer));
    card.addEventListener("pointerleave", () => { hideTimer = setTimeout(hide, 150); });
    document.body.append(card);
  }
  if (current !== li) { current?.removeAttribute("aria-describedby"); current = li; build(li); }
  li.setAttribute("aria-describedby", card.id);
  card.hidden = false;
  place();
}
function hide() { clearTimeout(showTimer); if (card) card.hidden = true; current?.removeAttribute("aria-describedby"); current = null; }

document.addEventListener("pointerover", (e) => {
  const li = e.target.closest?.("[data-sources] .src");
  if (!li || e.pointerType === "touch") return;
  clearTimeout(hideTimer); clearTimeout(showTimer);
  if (card && !card.hidden) show(li); else showTimer = setTimeout(() => show(li), DELAY);
});
document.addEventListener("pointerout", (e) => {
  const li = e.target.closest?.("[data-sources] .src");
  if (!li || li.contains(e.relatedTarget) || card?.contains(e.relatedTarget)) return;
  clearTimeout(showTimer);
  hideTimer = setTimeout(hide, 150);
});
addEventListener("keydown", (e) => { if (e.key === "Escape") hide(); });
addEventListener("scroll", () => { if (card && !card.hidden) hide(); }, { passive: true, capture: true });
