import test from "node:test";
import assert from "node:assert/strict";
import { copyFileSync, mkdtempSync, mkdirSync, readFileSync, writeFileSync, symlinkSync, rmSync } from "node:fs";
import { tmpdir } from "node:os";
import { join, resolve } from "node:path";
import { spawn, execFileSync } from "node:child_process";
import { once } from "node:events";
import { setTimeout as delay } from "node:timers/promises";
import { pathToFileURL } from "node:url";
import { codeOf } from "../src/lib/code.js";

// Exercise manual refresh against Astro's real content store, route cache and HMR socket.
// npm test runs every test file at once: Astro's first, cold compile of a page can then take far
// longer than the few seconds it takes alone, so each wait has a minute and the test five.
test("dev reports and commits stay quiet until manual refresh; code still hot reloads", { timeout: 300000 }, async () => {
  const root = mkdtempSync(join(tmpdir(), "tracker-changelog-dev-"));
  const git = (...args) => execFileSync("git", args, { cwd: root, stdio: "pipe" });
  let child, socket, logs = "";
  // waits for action to return something, polling while Astro starts or reloads; a wait is
  // limited in time, not in tries, as one try can take seconds under load
  const until = async (action, what = action.toString()) => {
    const deadline = Date.now() + 60000;
    while (Date.now() < deadline) {
      if (child.exitCode !== null) throw new Error(`Astro exited: ${logs}`);
      try { const value = await action(); if (value) return value; } catch { /* startup/reload in progress */ }
      await delay(100);
    }
    throw new Error(`Timed out waiting for ${what}: ${logs}`);
  };
  try {
    mkdirSync(join(root, "src/pages/changelog"), { recursive: true });
    for (const dir of ["src/lib", "src/data", "src/pages/api/country", "src/pages/report", "src/pages/reports", "src/pages/r", "reports"]) mkdirSync(join(root, dir), { recursive: true });
    for (const name of ["reports", "types", "countries", "map", "code", "figures", "details", "media", "media-manifest", "figure-filters", "stats", "atlas-countries", "report-time", "report-history", "changelog", "git-blobs"]) copyFileSync(resolve(`src/lib/${name}.js`), join(root, `src/lib/${name}.js`));
    copyFileSync(resolve("src/data/countries.json"), join(root, "src/data/countries.json"));
    copyFileSync(resolve("src/data/exchange-rates.json"), join(root, "src/data/exchange-rates.json"));
    for (const name of ["vocabularies", "agencies", "media"]) copyFileSync(resolve(`src/data/${name}.json`), join(root, `src/data/${name}.json`));
    copyFileSync(resolve("src/pages/api/index.json.js"), join(root, "src/pages/api/index.json.js"));
    copyFileSync(resolve("src/pages/api/country/[...id].json.js"), join(root, "src/pages/api/country/[...id].json.js"));
    copyFileSync(resolve("src/pages/r/[code].astro"), join(root, "src/pages/r/[code].astro"));
    writeFileSync(join(root, "src/content.config.ts"), `import { defineCollection, z } from 'astro:content';
import { reportLoader } from ${JSON.stringify(pathToFileURL(resolve('scripts/lib/report-loader.mjs')).href)};
export const collections = { reports: defineCollection({ loader: reportLoader({ pattern: '**/*.md', base: './reports' }), schema: z.object({date:z.coerce.date(),code:z.string()}).passthrough() }) };`);
    const report = (title, slug = "initial") => `---\ncode: "${codeOf(slug)}"\ndate: 2026-10-30\ncountry: cz\ntype: seizure\ntitle: ${title}\nsummary: Fixture report\nfigures:\n  kg_seized: 0.00386\nstatus: reported\ncreated_by_model: unknown\nsources: []\n---\nFixture body.\n`;
    writeFileSync(join(root, "reports/initial.md"), report("Initial report"));
    writeFileSync(join(root, "src/pages/report/[slug].astro"), `---
import { reports } from '../../lib/reports.js';
export const getStaticPaths = () => reports.map(r => ({params:{slug:r.id},props:{r}}));
---
<h1>{Astro.props.r.title}</h1>`);
    writeFileSync(join(root, "src/pages/reports/index.astro"), "<h1>Initial reports page</h1>");
    symlinkSync(resolve("node_modules"), join(root, "node_modules"), "dir");
    writeFileSync(join(root, "package.json"), '{"type":"module"}');
    const plugin = pathToFileURL(resolve("scripts/lib/reports-dev.mjs")).href;
    writeFileSync(join(root, "astro.config.mjs"), `import { defineConfig } from 'astro/config';\nimport { reportsDev, devWatchIgnored } from ${JSON.stringify(plugin)};\nexport default defineConfig({ site: 'https://tracker.example', devToolbar: { enabled: false }, cacheDir: './.astro-cache', integrations: [reportsDev()], vite: { cacheDir: './.vite-cache', server: { watch: { ignored: devWatchIgnored(${JSON.stringify(root + '/reports')}) } } } });`);
    writeFileSync(join(root, "src/pages/changelog/[...page].astro"), `---
import {execFileSync} from 'node:child_process';
import { reports } from '../../lib/reports.js';
export function getStaticPaths() {
 const subjects=execFileSync('git',['log','--format=%s'],{encoding:'utf8'}).trim().split('\\n');
 return subjects.map((subject,i)=>({params:{page:i===0?undefined:String(i+1)},props:{subject}}));
}
---
<h1>{Astro.props.subject}</h1>
{reports.map(r => <a href={'/' + r.url}>{r.title}</a>)}`);
    git("init"); git("config", "user.name", "Test"); git("config", "user.email", "test@example.org");
    git("add", "package.json", "reports", "src/data/media.json"); git("commit", "-m", "Initial history");
    // Start with a fresh content cache so dev loads its own content configuration
    // and completes the initial sync before routes are requested.
    child = spawn(process.execPath, [resolve("node_modules/astro/bin/astro.mjs"), "dev", "--ignore-lock", "--root", root, "--host", "127.0.0.1", "--port", "0"], { cwd: root, env: { ...process.env, ASTRO_TELEMETRY_DISABLED: "1", NO_COLOR: "1" }, stdio: ["ignore", "pipe", "pipe"] });
    const record = (chunk) => { logs += chunk.toString(); };
    child.stdout.on("data", record); child.stderr.on("data", record);
    const address = await until(() => logs.match(/http:\/\/127\.0\.0\.1:\d+/)?.[0]);
    const read = async (path) => { const response = await fetch(address + path, { signal: AbortSignal.timeout(30000) }); return { status: response.status, text: await response.text() }; };
    await until(async () => (await read("/changelog/")).text.includes("<h1>Initial history</h1>"));
    assert.equal((await read("/changelog/2/")).status, 404);
    await until(async () => (await read("/api/index.json")).text.includes("Initial report"));
    assert.ok((await read("/api/index.json")).text.includes("3.86 g"));
    assert.equal(JSON.parse((await read("/api/country/cz.json")).text).totals.kg_seized, 0.00386);
    assert.equal((await read("/report/new-report/")).status, 404);
    assert.ok((await read("/report/initial/")).text.includes("Initial report"));
    assert.equal((await fetch(address + `/r/${codeOf("initial")}/`, { redirect: "manual" })).status, 301);
    const client = await (await fetch(address + "/@vite/client")).text();
    const token = client.match(/const wsToken = "([^"]+)"/)?.[1];
    assert.ok(token, "Vite exposes a token for its development WebSocket");
    socket = new WebSocket(address.replace("http:", "ws:") + "/?token=" + token, "vite-hmr");
    const notifications = [];
    socket.addEventListener("message", (event) => notifications.push(JSON.parse(event.data)));
    await once(socket, "open");
    const refresh = async () => {
      const response = await fetch(address + "/__dev/refresh-reports", { method: "POST" });
      assert.equal(response.status, 204, await response.text());
    };
    assert.equal((await fetch(address + "/__dev/refresh-reports")).status, 405);
    assert.equal((await fetch(address + "/__dev/refresh-reports", { method: "POST", headers: { Origin: "https://elsewhere.example" } })).status, 403);
    await delay(1500); // Let startup-generated modules finish notifying the watcher.
    notifications.length = 0;
    git("commit", "--allow-empty", "-m", "New commit without source edits");
    writeFileSync(join(root, "reports/new-report.md"), report("Newly committed report", "new-report"));
    writeFileSync(join(root, "reports/initial.md"), report("Updated report"));
    git("add", "reports"); git("commit", "-m", "Add and update reports");
    // a thumbnail commit rewrites the photo manifest, which reports.js imports: quiet too
    writeFileSync(join(root, "src/data/media.json"), readFileSync(join(root, "src/data/media.json"), "utf8").replace(/\n$/, "") + "\n\n");
    git("add", "src/data/media.json"); git("commit", "-m", "Add photo thumbnails: fixture");
    await delay(1500);
    assert.equal(notifications.filter(m => m.type === "full-reload" || m.type === "update").length, 0, `Report writes and commits must not trigger HMR: ${JSON.stringify(notifications)}\n${logs}`);
    assert.ok(!(await read("/api/index.json")).text.includes("Newly committed report"));
    assert.ok((await read("/report/initial/")).text.includes("Initial report"));
    assert.equal((await read("/report/new-report/")).status, 404);
    assert.ok((await read("/changelog/")).text.includes("<h1>Initial history</h1>"));
    await refresh();
    await until(async () => (await read("/changelog/")).text.includes("Add photo thumbnails: fixture"));
    await until(async () => (await read("/api/index.json")).text.includes("Newly committed report"));
    await until(async () => (await read("/report/new-report/")).text.includes("<h1>Newly committed report</h1>"));
    await until(async () => (await read("/report/initial/")).text.includes("<h1>Updated report</h1>"));
    assert.ok((await read("/changelog/")).text.includes('href="/report/new-report/"'), "Changelog links include newly committed reports");
    const index = JSON.parse((await read("/api/index.json")).text);
    assert.ok(index.reports.some(r => r.i === "new-report"));
    // Request without following the short URL redirect, then exercise its target above.
    assert.equal((await fetch(address + `/r/${codeOf("new-report")}/`, { redirect: "manual" })).status, 301);
    // Branch/revision changes also invalidate cached paths, including removed pages.
    git("checkout", "--detach", "HEAD~3");
    await refresh();
    await until(async () => (await read("/changelog/")).text.includes("<h1>Initial history</h1>"));
    assert.equal((await read("/changelog/2/")).status, 404);
    await until(async () => !(await read("/api/index.json")).text.includes("Newly committed report"));
    assert.equal((await read("/report/new-report/")).status, 404);
    assert.ok((await read("/report/initial/")).text.includes("<h1>Initial report</h1>"));
    notifications.length = 0;
    assert.ok((await read("/reports/")).text.includes("Initial reports page"));
    writeFileSync(join(root, "src/pages/reports/index.astro"), "<h1>Reports code</h1>");
    await until(() => notifications.some(m => m.type === "full-reload" || m.type === "update"));
    assert.ok((await read("/reports/")).text.includes("Reports code"));
    notifications.length = 0;
    writeFileSync(join(root, "src/pages/report/[slug].astro"), readFileSync(join(root, "src/pages/report/[slug].astro"), "utf8") + "\n<p>Code hot reload works</p>");
    await until(() => notifications.some(m => m.type === "full-reload" || m.type === "update"));
    assert.ok((await read("/report/initial/")).text.includes("Code hot reload works"));
  } finally {
    socket?.close();
    if (child && child.exitCode === null) {
      const exited = once(child, "exit");
      child.kill("SIGTERM");
      await exited;
    }
    rmSync(root, { recursive: true, force: true });
  }
});
