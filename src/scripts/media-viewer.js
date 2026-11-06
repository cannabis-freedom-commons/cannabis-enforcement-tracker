// The report page's photo grid: a photo opens large in a dialog, with its caption and credit, and
// the arrow keys move between photos. A photo its publisher no longer serves leaves the grid and
// is listed as a link instead, so a broken image is never shown. Moving to another photo shows it at
// once as what is already here (its ThumbHash placeholder, and the tile's small picture over it)
// with a loading note, and the full photo fades in when it has arrived.
const initialized = new WeakSet();

export function initMediaViewers(root = document) {
  for (const grid of root.querySelectorAll("[data-media-grid]")) {
    if (initialized.has(grid)) continue;
    initialized.add(grid);
    initGrid(grid);
  }
}

function initGrid(grid) {
  const tiles = [...grid.querySelectorAll(".media-tile")].filter((t) => t.querySelector(".media-open"));
  if (!tiles.length) return;
  const broken = (tile) => {
    tile.hidden = true;
    const a = tile.querySelector(".media-source a");
    let list = grid.nextElementSibling;
    if (!list?.matches("ul.media")) { list = Object.assign(document.createElement("ul"), { className: "media" }); grid.after(list); }
    const li = document.createElement("li"), link = Object.assign(document.createElement("a"), { href: a.href, className: "ext", target: "_blank", rel: "noopener noreferrer", textContent: tile.querySelector("img").alt });
    li.append(link, Object.assign(document.createElement("span"), { className: "muted", textContent: ` Photos: ${a.textContent}` }));
    list.append(li);
    if (![...grid.children].some((t) => !t.hidden)) grid.hidden = true;
  };
  for (const tile of tiles) {
    const img = tile.querySelector("img");
    if (img.complete && !img.naturalWidth) broken(tile);
    else img.addEventListener("error", () => broken(tile), { once: true });
  }

  const dialog = document.createElement("dialog");
  dialog.className = "media-viewer";
  dialog.innerHTML = `<figure><div class="mv-frame"><img alt="" referrerpolicy="no-referrer"><span class="mv-loading" role="status">Loading the full photo…</span></div><figcaption></figcaption></figure>
    <button type="button" class="mv-close" aria-label="Close" autofocus>×</button>
    <button type="button" class="mv-prev" aria-label="Previous photo">‹</button>
    <button type="button" class="mv-next" aria-label="Next photo">›</button>`;
  grid.parentElement.append(dialog);
  const big = dialog.querySelector(".mv-frame img"), frame = dialog.querySelector(".mv-frame"), caption = dialog.querySelector("figcaption");
  const note = dialog.querySelector(".mv-loading");
  let loading = 0; // which show() the pending load belongs to, so a slow earlier photo never replaces a later one
  let at = 0;
  const visible = () => tiles.filter((t) => !t.hidden);
  const show = (tile) => {
    const list = visible(); at = list.indexOf(tile);
    const small = tile.querySelector("img"), full = small.dataset.full || small.src;
    // at once: the placeholder, the tile's picture over it (cached: already on the page), in the photo's shape
    const placeholder = tile.querySelector(".media-open").style.backgroundImage;
    const preview = small.complete && small.naturalWidth ? `url("${small.currentSrc || small.src}")` : "";
    frame.style.backgroundImage = [preview, placeholder].filter(Boolean).join(", ");
    const w = Number(small.getAttribute("width")) || small.naturalWidth, h = Number(small.getAttribute("height")) || small.naturalHeight;
    frame.style.aspectRatio = w && h ? `${w} / ${h}` : "";
    big.alt = small.alt;
    if (big.src !== full) {
      const ticket = ++loading;
      big.classList.remove("is-loaded");
      frame.classList.remove("is-failed");
      frame.classList.add("is-loading");
      note.textContent = "Loading the full photo…";
      big.onload = () => { if (ticket === loading) { big.classList.add("is-loaded"); frame.classList.remove("is-loading"); } };
      big.onerror = () => { if (ticket === loading) { note.textContent = "The full photo could not be loaded; this is a small copy."; frame.classList.add("is-failed"); } };
      big.src = full;
      if (big.complete && big.naturalWidth) big.onload();
    }
    caption.replaceChildren(...[...tile.querySelector("figcaption").childNodes].map((n) => n.cloneNode(true)));
    for (const b of caption.querySelectorAll(".photo-info-btn")) b.setAttribute("aria-expanded", "false");
    dialog.querySelector(".mv-prev").hidden = dialog.querySelector(".mv-next").hidden = list.length < 2;
  };
  const step = (d) => { const list = visible(); if (list.length > 1) show(list[(at + d + list.length) % list.length]); };
  grid.addEventListener("click", (e) => {
    const tile = e.target.closest(".media-open")?.closest(".media-tile");
    if (tile) { show(tile); dialog.showModal(); }
  });
  dialog.querySelector(".mv-close").addEventListener("click", () => dialog.close());
  dialog.querySelector(".mv-prev").addEventListener("click", () => step(-1));
  dialog.querySelector(".mv-next").addEventListener("click", () => step(1));
  dialog.addEventListener("click", (e) => { if (e.target === dialog) dialog.close(); });
  dialog.addEventListener("keydown", (e) => { if (e.key === "ArrowLeft") step(-1); if (e.key === "ArrowRight") step(1); });
}

// a tap on a photo's info icon pins its details (hover and focus show them without a script)
document.addEventListener("click", (e) => {
  const btn = e.target.closest?.(".photo-info-btn");
  for (const open of document.querySelectorAll('.photo-info-btn[aria-expanded="true"]')) if (open !== btn) open.setAttribute("aria-expanded", "false");
  if (btn) { e.stopPropagation(); btn.setAttribute("aria-expanded", String(btn.getAttribute("aria-expanded") !== "true")); }
}, true);

initMediaViewers();
