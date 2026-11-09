// A real Astro build of only the pages a change touches, for UI work: seconds instead of building
// every report page. The output goes to .cache/page-review/, apart from the deployable dist/.
//   npm run build:pages -- / api/index.json api/routes.json      the home page and the data it loads
//   npm run build:pages -- stats report/some-report-id            the Statistics page and one report
//   npm run build:pages -- --no-test /                            skip `npm test` (UI tweaks only)
// A prefix keeps that URL and everything under it ("report" would be every report page). The
// shared assets (/_astro/) are always built. Use `npm run build` for deployment and site-wide checks.
import { spawnSync } from "node:child_process";
import { build } from "astro";

const args = process.argv.slice(2);
const skipTests = args.includes("--no-test");
const prefixes = args.filter((a) => a !== "--no-test").map((s) => `/${s.replace(/^\/+|\/+$/g, "")}`);
if (!prefixes.length || prefixes.some((s) => /[?#*]|\.\./.test(s))) {
  console.error("Usage: npm run build:pages -- [--no-test] <URL prefix> [more prefixes]\nExample: npm run build:pages -- / api/index.json api/routes.json");
  process.exit(1);
}
if (process.env.CI) throw new Error("Selected-page builds are for local review. CI must build the complete site.");
if (!skipTests) {
  const test = spawnSync("npm", ["test"], { stdio: "inherit" });
  if (test.status !== 0) process.exit(test.status ?? 1);
}

const base = (process.env.BASE_PATH || "").replace(/^\/+|\/+$/g, "");
const outDir = "./.cache/page-review/";
await build({
  outDir,
  integrations: [{
    name: "selected-page-review",
    hooks: {
      "astro:build:start": ({ setPrerenderer }) => {
        setPrerenderer((renderer) => ({
          ...renderer,
          async getStaticPaths() {
            const result = await renderer.getStaticPaths();
            const all = Array.isArray(result) ? result : result.paths;
            const matched = new Set();
            const paths = all.filter(({ pathname }) => {
              const url = `/${pathname.replace(/^\/+|\/+$/g, "")}`;
              let keep = false;
              for (const prefix of prefixes) {
                const target = base ? `/${base}${prefix === "/" ? "" : prefix}` : prefix;
                if (url === target || (prefix !== "/" && url.startsWith(`${target}/`))) {
                  matched.add(prefix);
                  keep = true;
                }
              }
              return keep;
            });
            const missing = prefixes.filter((p) => !matched.has(p));
            if (missing.length) throw new Error(`No pages matched: ${missing.join(", ")}`);
            console.log(`Building ${paths.length} of ${all.length} pages.`);
            return Array.isArray(result) ? paths : { ...result, paths };
          },
        }));
      },
    },
  }],
});
console.log(`Pages in ${outDir} (a partial site: use npm run build for deployment and site-wide checks).`);
