// Iconoir icons (https://iconoir.com, MIT) as SVG text, read from the package at build time: for
// Icon.astro and for markup built at build time as strings. Scripts in the browser import the one
// file they need with Vite's `?raw` (e.g. "iconoir/icons/regular/position.svg?raw") and pass it to
// `shapeIcon` (src/lib/icon-shape.js). Build time only (reads files).
import { readFileSync } from "node:fs";
import { createRequire } from "node:module";
import { shapeIcon } from "./icon-shape.js";
export { shapeIcon };

const require = createRequire(import.meta.url);
const cache = new Map();

// the raw file: 24 × 24, stroke 1.5 in currentColor (the solid set is filled)
export function iconFile(name, solid = false) {
  const key = `${solid ? "solid" : "regular"}/${name}`;
  if (!cache.has(key)) cache.set(key, readFileSync(require.resolve(`iconoir/icons/${key}.svg`), "utf8"));
  return cache.get(key);
}

export const iconSvg = (name, options = {}) => shapeIcon(iconFile(name, options.solid), options);
