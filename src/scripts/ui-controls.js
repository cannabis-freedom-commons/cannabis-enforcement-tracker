// The site's dropdowns, built on data-slot's headless components (https://data-slot.com/): a select
// (Select.astro) and a searchable combobox (CountryPicker.astro). Each keeps a hidden input with the
// id a native <select> had, so the code around it is unchanged: reading or setting `.value` reads or
// moves the selection, and choosing an option fires "input" and "change" on that input.
import { createSelect } from "@data-slot/select";
import { createCombobox } from "@data-slot/combobox";

const valueOf = Object.getOwnPropertyDescriptor(HTMLInputElement.prototype, "value");
// the label shown for "nothing chosen" is an option with this value; the input then holds ""
const ALL = "*";

function bind(root, kind) {
  const input = root.querySelector("input[data-ui-value]");
  if (!input || input.dataset.uiBound) return;
  input.dataset.uiBound = "";
  // whether there is an "all" option, read now: once opened, the list may be moved out of `root`
  const hasAll = root.querySelector(`[data-value="${ALL}"]`) != null;
  let quiet = false;
  const setSelection = (v) => {
    quiet = true;
    root.dispatchEvent(new CustomEvent(`${kind}:set`, { detail: { value: v === "" ? (hasAll ? ALL : null) : v } }));
    quiet = false;
  };
  const onValueChange = (v) => {
    const value = v == null || v === ALL ? "" : v;
    valueOf.set.call(input, value);
    if (quiet) return;
    input.dispatchEvent(new Event("input", { bubbles: true }));
    input.dispatchEvent(new Event("change", { bubbles: true }));
  };
  const initial = valueOf.get.call(input);
  const options = { onValueChange, defaultValue: initial || (hasAll ? ALL : undefined) };
  // Opening a list normally locks the page's scrolling, which reserves room for a scrollbar
  // (scrollbar-gutter: stable). The full-window map pages have no scrollbar, so that room would
  // narrow the map and make it redraw (a flicker); there is nothing to scroll there anyway.
  if (root.closest(".app")) options.lockScroll = false;
  if (kind === "combobox") {
    // match the start of any word, ignoring case and accents: "turk" finds Türkiye, "states" the US
    const fold = (s) => s.normalize("NFD").replace(/\p{M}/gu, "").toLowerCase();
    options.filter = (query, value, label) => !query.trim() || fold(label).split(/[\s,()-]+/).some((w) => w.startsWith(fold(query.trim()))) || fold(label).includes(fold(query.trim()));
    options.itemToStringValue = (item) => item?.dataset.display ?? item?.dataset.label ?? "";
    createCombobox(root, options);
  } else createSelect(root, options);
  // code that sets the hidden input's value moves the visible selection with it
  Object.defineProperty(input, "value", {
    configurable: true,
    get: () => valueOf.get.call(input),
    set: (v) => { valueOf.set.call(input, v); setSelection(String(v ?? "")); },
  });
}

export function initControls(scope = document) {
  for (const root of scope.querySelectorAll('[data-slot="select"][data-ui]')) bind(root, "select");
  for (const root of scope.querySelectorAll('[data-slot="combobox"][data-ui]')) bind(root, "combobox");
}
initControls();
