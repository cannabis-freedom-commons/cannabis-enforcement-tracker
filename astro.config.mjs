import reportAliases from "./src/lib/report-aliases.js";
import { createHash } from "node:crypto";
import { readFileSync } from "node:fs";
import { resolve } from "node:path";
import { defineConfig } from "astro/config";
import { loadEnv } from "vite";
import { reportsDev, devWatchIgnored } from "./scripts/lib/reports-dev.mjs";
import { sitemap } from "./scripts/lib/sitemap.mjs";
import { markdownGlossary } from "./src/lib/glossary.js";
import { unified } from "@astrojs/markdown-remark";

// Fully static output: one HTML file per report and per country, plus the JSON files under /api/.
// Changing definitions must invalidate Astro's cached Markdown, even when report files are unchanged.
// The same for the exchange rates and the weight and money rules behind the tooltips.
const glossaryRevision = createHash("sha256")
  .update(readFileSync(new URL("./src/data/glossary.json", import.meta.url)))
  .update(readFileSync(new URL("./src/data/exchange-rates.json", import.meta.url)))
  .update(readFileSync(new URL("./src/lib/measures.js", import.meta.url)))
  .digest("hex");

const base = (process.env.BASE_PATH || "").replace(/\/+$/, "") + "/";
// The backend the dev server's /__submissions proxy forwards to: the local one, or the server's
// (SUBMISSIONS_DEV_API=https://tracker-api.cannabisfreedom.fyi in .env).
const submissionsDevAPI = loadEnv("development", process.cwd(), "").SUBMISSIONS_DEV_API || "http://127.0.0.1:8080";
const reportsDirectory = resolve(process.env.REPORTS_DIR || "./reports").replaceAll("\\", "/");

// Links in a report's text: one that leaves the site opens in a new tab and is marked `ext`;
// one that starts with "/" is a page of this site and gets the base path in front.
function markdownLinks() {
  const walk = (node) => {
    if (node.tagName === "a" && typeof node.properties?.href === "string") {
      const href = node.properties.href;
      if (/^https?:\/\//.test(href)) {
        node.properties.target = "_blank";
        node.properties.rel = "noopener noreferrer";
        node.properties.className = [...(node.properties.className ?? []), "ext"];
      } else if (href.startsWith("/") && !href.startsWith("//")) {
        node.properties.href = base + href.slice(1);
      }
    }
    node.children?.forEach(walk);
  };
  return walk;
}

// Inline citations, Wikipedia style, as on the Cannabis Freedom Index: [@2] or [@1,3] marks the
// sources, by their place in the front matter's `sources` list, that support what comes before it.
// Each becomes a superscript number linked to the report's list of sources; sources are numbered by
// their first citation, and the order is left in the front matter as `citeOrder`. Every source must
// be cited at least once, and a citation must name a source that exists.
function markdownCitations() {
  const MARK = /\s?\[@([^\]]+)\]/g;
  const sup = (n) => `<sup class="cite"><a href="#ref-${n}" aria-label="Source ${n}">[${n}]</a></sup>`;
  return (tree, file) => {
    const fm = file.data.astro?.frontmatter;
    const count = fm?.sources?.length ?? 0;
    if (!count) return;
    const where = file.history?.[0]?.split("/").slice(-3).join("/") ?? "a report";
    const order = [];
    const numberOf = (i) => {
      if (!Number.isInteger(i) || i < 1 || i > count) throw new Error(`${where}: the citation [@${i}] names no source (it has ${count})`);
      if (!order.includes(i)) order.push(i);
      return order.indexOf(i) + 1;
    };
    const walk = (node) => {
      if (!node.children) return;
      node.children = node.children.flatMap((child) => {
        if (child.type !== "text" || !child.value.includes("[@")) { walk(child); return [child]; }
        const out = [];
        let last = 0;
        for (const m of child.value.matchAll(MARK)) {
          if (m.index > last) out.push({ type: "text", value: child.value.slice(last, m.index) });
          out.push({ type: "html", value: m[1].split(",").map((x) => sup(numberOf(Number(x.trim())))).join("") });
          last = m.index + m[0].length;
        }
        if (last < child.value.length) out.push({ type: "text", value: child.value.slice(last) });
        return out;
      });
    };
    walk(tree);
    const missing = Array.from({ length: count }, (_, i) => i + 1).filter((i) => !order.includes(i));
    if (missing.length) throw new Error(`${where}: never cites ${missing.map((i) => `[@${i}]`).join(", ")}`);
    fm.citeOrder = order;
  };
}

// Each cited stretch of text (from the previous citation, or the start of its paragraph, up to its
// own citations) is wrapped in <span class="cited" data-refs="1 3">. Without scripts it changes
// nothing; with them, the report page hides the [n] numbers and shows a sentence's sources when it
// is pointed at, focused or tapped (src/scripts/report-citations.js).
function markdownCitedText() {
  const isCite = (n) => n?.type === "raw" && n.value.startsWith('<sup class="cite">');
  const hasText = (n) => n.type === "text" ? n.value.trim() !== "" : n.type !== "comment";
  const walk = (node) => {
    if (!node.children) return;
    if (node.type === "element" && node.children.some(isCite)) {
      const out = [];
      let run = [];
      for (let i = 0; i < node.children.length; i++) {
        if (!isCite(node.children[i])) { run.push(node.children[i]); continue; }
        const group = [node.children[i]];
        while (isCite(node.children[i + 1])) group.push(node.children[++i]);
        // the space after the previous citation stays outside, so the highlight starts at the text
        const lead = run[0]?.type === "text" ? run[0].value.match(/^\s+/)?.[0] : "";
        if (lead) { out.push({ type: "text", value: lead }); run[0] = { type: "text", value: run[0].value.slice(lead.length) }; }
        const refs = group.flatMap((g) => [...g.value.matchAll(/#ref-(\d+)/g)].map((m) => m[1]));
        if (run.some(hasText)) out.push({ type: "element", tagName: "span", properties: { className: ["cited"], dataRefs: refs.join(" ") }, children: run });
        else out.push(...run);
        out.push(...group);
        run = [];
      }
      node.children = [...out, ...run];
    }
    node.children.forEach(walk);
  };
  return walk;
}

export default defineConfig({
  redirects: Object.fromEntries([
    ...Object.entries(reportAliases.slugs).map(([slug, target]) => [`/report/${slug}/`, { destination: `${base}report/${target}/`, status: 301 }]),
    ...Object.entries(reportAliases.codes).map(([code, target]) => [`/r/${code}/`, { destination: `${base}report/${target}/`, status: 301 }]),
  ]),
  site: process.env.SITE || "https://tracker.cannabisfreedom.fyi",
  base,
  integrations: [reportsDev(), sitemap()],
  vite: {
    server: {
      // the Go backend's code and job storage, and reports with their photo manifest: refreshed only
      // from the dev header button (scripts/lib/reports-dev.mjs)
      watch: { ignored: devWatchIgnored(reportsDirectory) },
      proxy: {
        "/__submissions": {
          target: submissionsDevAPI,
          changeOrigin: true,
          ws: true,
          rewrite: (path) => path.replace(/^\/__submissions/, ""),
          configure: (proxy) => {
            // The dev server makes server-to-server requests. Do not forward its
            // browser origin: Astro may select a different port when 4321 is busy.
            proxy.on("proxyReq", (request) => request.removeHeader("origin"));
            proxy.on("proxyReqWs", (request) => request.removeHeader("origin"));
          },
        },
      },
    },
  },
  markdown: {
    processor: unified({ remarkPlugins: [markdownCitations], rehypePlugins: [markdownLinks, markdownCitedText, [markdownGlossary, { revision: glossaryRevision }]] }),
  },
});
