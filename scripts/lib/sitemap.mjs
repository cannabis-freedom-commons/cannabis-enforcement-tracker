// sitemap.xml and robots.txt, written after the build from the pages it actually produced: every
// HTML page except redirects and pages marked noindex (submissions, drafts, countries without
// reports). A page's <meta property="article:modified_time"> (or published_time) is its lastmod.
import { readdir, readFile, writeFile } from "node:fs/promises";
import { join, relative, sep } from "node:path";
import { fileURLToPath } from "node:url";

const MAX_URLS = 50000; // per sitemap file, the protocol's limit

async function* htmlFiles(dir) {
  for (const entry of await readdir(dir, { withFileTypes: true })) {
    const path = join(dir, entry.name);
    if (entry.isDirectory()) yield* htmlFiles(path);
    else if (entry.name.endsWith(".html")) yield path;
  }
}

const escapeXml = (s) => s.replace(/&/g, "&amp;").replace(/</g, "&lt;").replace(/>/g, "&gt;");

// What the sitemap says about one built page, or null when it does not belong there.
export function pageEntry(html) {
  const head = html.slice(0, html.indexOf("</head>") + 1 || 20000);
  if (/<meta[^>]+http-equiv=["']?refresh/i.test(head)) return null;
  if (/<meta[^>]+name=["']robots["'][^>]+content=["'][^"']*noindex/i.test(head)) return null;
  const canonical = head.match(/<link[^>]+rel=["']canonical["'][^>]+href=["']([^"']+)/i)?.[1];
  if (!canonical) return null;
  const time = head.match(/<meta[^>]+property=["']article:modified_time["'][^>]+content=["']([^"']+)/i)?.[1]
    ?? head.match(/<meta[^>]+property=["']article:published_time["'][^>]+content=["']([^"']+)/i)?.[1];
  return { loc: canonical, ...(time ? { lastmod: time } : {}) };
}

export async function writeSitemap(dir, site) {
  const entries = [];
  for await (const file of htmlFiles(dir)) {
    const name = relative(dir, file).split(sep).join("/");
    if (name === "404.html") continue;
    const entry = pageEntry(await readFile(file, "utf8"));
    if (entry) entries.push(entry);
  }
  entries.sort((a, b) => a.loc.localeCompare(b.loc));
  const chunks = [];
  for (let i = 0; i < entries.length; i += MAX_URLS) chunks.push(entries.slice(i, i + MAX_URLS));
  const urlset = (list) => `<?xml version="1.0" encoding="UTF-8"?>\n<urlset xmlns="http://www.sitemaps.org/schemas/sitemap/0.9">\n${list.map((e) =>
    `<url><loc>${escapeXml(e.loc)}</loc>${e.lastmod ? `<lastmod>${escapeXml(e.lastmod)}</lastmod>` : ""}</url>`).join("\n")}\n</urlset>\n`;
  const root = new URL(".", site);
  let index;
  if (chunks.length <= 1) {
    await writeFile(join(dir, "sitemap.xml"), urlset(entries));
    index = new URL("sitemap.xml", root).href;
  } else {
    for (const [i, list] of chunks.entries()) await writeFile(join(dir, `sitemap-${i + 1}.xml`), urlset(list));
    await writeFile(join(dir, "sitemap.xml"), `<?xml version="1.0" encoding="UTF-8"?>\n<sitemapindex xmlns="http://www.sitemaps.org/schemas/sitemap/0.9">\n${chunks.map((_, i) =>
      `<sitemap><loc>${escapeXml(new URL(`sitemap-${i + 1}.xml`, root).href)}</loc></sitemap>`).join("\n")}\n</sitemapindex>\n`);
    index = new URL("sitemap.xml", root).href;
  }
  // Nothing is disallowed: pages kept out of search say so with noindex, which a crawler must be
  // allowed to read.
  await writeFile(join(dir, "robots.txt"), `User-agent: *\nAllow: /\n\nSitemap: ${index}\n`);
  return entries.length;
}

export function sitemap() {
  let site;
  return {
    name: "tracker:sitemap",
    hooks: {
      "astro:config:done"({ config }) { site = new URL(config.base, config.site); },
      async "astro:build:done"({ dir, logger }) {
        if (!site) return;
        const n = await writeSitemap(fileURLToPath(dir), site);
        logger.info(`sitemap.xml: ${n} pages`);
      },
    },
  };
}
