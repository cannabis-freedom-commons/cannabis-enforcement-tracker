// This page's build (Base.astro's cet-build meta) as a query to add to the site's own data files:
// GitHub Pages lets browsers and its CDN keep files for ten minutes, so a page from a new build
// could load a previous build's index. With the build in the address each build's files are new
// addresses, fetched fresh once and cached after that.
export const BUILD = document.querySelector('meta[name="cet-build"]')?.getAttribute("content") ?? "";
export const versioned = (url) => (BUILD ? `${url}${url.includes("?") ? "&" : "?"}b=${encodeURIComponent(BUILD)}` : url);
