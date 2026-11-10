// An Iconoir SVG made ready to inline: without its fixed width and height (CSS sizes it), with a
// class, and hidden from screen readers unless titled. Pure, so scripts in the browser can use it
// on an icon they import with Vite's `?raw` (src/lib/icons.js reads them at build time).
export function shapeIcon(svg, { className = "", title = null } = {}) {
  const attrs = `class="icon${className ? ` ${className}` : ""}"${title ? ` role="img" aria-label="${title.replace(/"/g, "&quot;")}"` : ` aria-hidden="true"`} focusable="false"`;
  return svg.trim().replace(/\s(width|height)="24"/g, "").replace("<svg ", `<svg ${attrs} `);
}
