import { rankedTags } from "../lib/report-tags.js";

// Tags in the map's search box: typing "#gan" lists matching tags (with how many of the reports
// shown carry them); choosing one filters by it, drops the "#gan" from the text and shows the tag as
// a chip in the box, which clears it when pressed. Arrow keys, Enter and Escape work in the list.
export const tagToken = (value) => {
  const word = value.split(/\s+/).at(-1) || "";
  return word.startsWith("#") ? word.slice(1).toLowerCase() : null;
};

export function suggestTags(ranked, query, limit = 8) {
  const q = query.replace(/^#/, "").trim().toLowerCase();
  const starts = [], within = [];
  for (const t of ranked) (t.tag.startsWith(q) ? starts : t.tag.includes(q) ? within : null)?.push(t);
  return [...starts, ...within].slice(0, limit);
}

export function initTagSuggest(input, list, chip, { selected, onSelect }) {
  let ranked = [], options = [], active = -1;
  const close = () => {
    list.hidden = true; active = -1; options = [];
    input.setAttribute("aria-expanded", "false");
    input.removeAttribute("aria-activedescendant");
  };
  function draw() {
    const query = tagToken(input.value);
    if (query == null || document.activeElement !== input) return close();
    options = suggestTags(ranked, query);
    if (active >= options.length) active = options.length - 1;
    list.replaceChildren(...(options.length ? options.map(({ tag, count }, i) => {
      const li = document.createElement("li");
      li.id = `tag-suggest-${i}`; li.setAttribute("role", "option"); li.dataset.tag = tag;
      li.setAttribute("aria-selected", String(i === active));
      const label = document.createElement("span"); label.textContent = `#${tag}`;
      const n = document.createElement("small"); n.textContent = count.toLocaleString("en-GB");
      li.append(label, n);
      return li;
    }) : [Object.assign(document.createElement("li"), { className: "tag-suggest-empty", textContent: "No tag matches" })]));
    list.hidden = false;
    input.setAttribute("aria-expanded", "true");
    if (active >= 0) input.setAttribute("aria-activedescendant", `tag-suggest-${active}`);
    else input.removeAttribute("aria-activedescendant");
  }
  function choose(tag) {
    input.value = input.value.replace(/\s*#\S*$/, "");
    close();
    onSelect(tag);
    input.focus({ preventScroll: true });
  }
  function drawChip() {
    const tag = selected();
    chip.hidden = !tag;
    chip.querySelector("span").textContent = tag;
    chip.setAttribute("aria-label", `Remove the tag #${tag}`);
  }
  input.addEventListener("input", () => { active = -1; draw(); });
  input.addEventListener("focus", draw);
  input.addEventListener("blur", () => setTimeout(close, 120));
  input.addEventListener("keydown", (e) => {
    if (e.key === "Backspace" && !input.value && selected()) { onSelect(""); return; }
    if (list.hidden || !options.length) return;
    if (e.key === "ArrowDown" || e.key === "ArrowUp") {
      e.preventDefault();
      active = (active + (e.key === "ArrowDown" ? 1 : -1) + options.length) % options.length;
      draw();
    } else if (e.key === "Enter") {
      e.preventDefault();
      choose(options[Math.max(0, active)].tag);
    } else if (e.key === "Escape") {
      e.preventDefault();
      close();
    }
  });
  // pointerdown, not click: before the box loses focus and closes the list
  list.addEventListener("pointerdown", (e) => {
    const li = e.target.closest("[data-tag]");
    if (!li) return;
    e.preventDefault();
    choose(li.dataset.tag);
  });
  chip.addEventListener("click", () => { onSelect(""); input.focus({ preventScroll: true }); });
  drawChip();
  return (rows) => { ranked = rankedTags(rows); drawChip(); draw(); };
}
