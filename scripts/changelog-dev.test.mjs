import test from "node:test";
import assert from "node:assert/strict";
import { copyFileSync, mkdtempSync, mkdirSync, writeFileSync, symlinkSync, rmSync } from "node:fs";
import { tmpdir } from "node:os";
import { join, resolve } from "node:path";
import { spawn, execFileSync } from "node:child_process";
import { once } from "node:events";
import { setTimeout as delay } from "node:timers/promises";
import { pathToFileURL } from "node:url";
import { codeOf } from "../src/lib/code.js";

// Exercise Astro's real dev route cache: commits change no watched source files.
test("dev commits refresh changelog, report indexes and cached report routes", { timeout: 60000 }, async () => {
  const root = mkdtempSync(join(tmpdir(), "tracker-changelog-dev-"));
  const git = (...args) => execFileSync("git", args, { cwd: root, stdio: "pipe" });
  let child, socket, logs = "";
  const until = async (action) => {
    for (let i = 0; i < 150; i++) {
      if (child.exitCode !== null) throw new Error(`Astro exited: ${logs}`);
      try { const value = await action(); if (value) return value; } catch { /* startup/reload in progress */ }
      await delay(100);
    }
    throw new Error(`Timed out waiting for dev changelog: ${logs}`);
  };
  try {
    mkdirSync(join(root, "src/pages/changelog"), { recursive: true });
    for (const dir of ["src/lib", "src/data", "src/pages/api/country", "src/pages/report", "src/pages/r", "reports"]) mkdirSync(join(root, dir), { recursive: true });
    for (const name of ["reports", "types", "countries", "map", "code", "figures", "atlas-countries", "report-time", "report-history", "changelog"]) copyFileSync(resolve(`src/lib/${name}.js`), join(root, `src/lib/${name}.js`));
    copyFileSync(resolve("src/data/countries.json"), join(root, "src/data/countries.json"));
    copyFileSync(resolve("src/pages/api/index.json.js"), join(root, "src/pages/api/index.json.js"));
    copyFileSync(resolve("src/pages/api/country/[...id].json.js"), join(root, "src/pages/api/country/[...id].json.js"));
    copyFileSync(resolve("src/pages/r/[code].astro"), join(root, "src/pages/r/[code].astro"));
    writeFileSync(join(root, "src/content.config.ts"), `import { defineCollection, z } from 'astro:content';
import { glob } from 'astro/loaders';
export const collections = { reports: defineCollection({ loader: glob({ pattern: '**/*.md', base: './reports' }), schema: z.object({date:z.coerce.date(),code:z.string()}).passthrough() }) };`);
    const report = (title, slug = "initial") => `---\ncode: "${codeOf(slug)}"\ndate: 2026-10-30\ncountry: cz\ntype: seizure\ntitle: ${title}\nsummary: Fixture report\nfigures:\n  kg_seized: 0.00386\nstatus: reported\ncreated_by_model: unknown\nsources: []\n---\nFixture body.\n`;
    writeFileSync(join(root, "reports/initial.md"), report("Initial report"));
    writeFileSync(join(root, "src/pages/report/[slug].astro"), `---
import { reports } from '../../lib/reports.js';
export const getStaticPaths = () => reports.map(r => ({params:{slug:r.id},props:{r}}));
---
<h1>{Astro.props.r.title}</h1>`);
    symlinkSync(resolve("node_modules"), join(root, "node_modules"), "dir");
    writeFileSync(join(root, "package.json"), '{"type":"module"}');
    const plugin = pathToFileURL(resolve("scripts/lib/changelog-dev.mjs")).href;
    writeFileSync(join(root, "astro.config.mjs"), `import { defineConfig } from 'astro/config';\nimport { changelogDev } from ${JSON.stringify(plugin)};\nexport default defineConfig({ site: 'https://tracker.example', devToolbar: { enabled: false }, cacheDir: './.astro-cache', vite: { cacheDir: './.vite-cache', plugins: [changelogDev({ interval: 100 })] } });`);
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
    git("add", "package.json", "reports"); git("commit", "-m", "Initial history");
    // Seed the content store before exercising route caches. Otherwise a request
    // racing the initial dev sync can cache an empty collection for the whole test.
    execFileSync(process.execPath, [resolve("node_modules/astro/bin/astro.mjs"), "sync", "--root", root], { cwd: root, env: { ...process.env, ASTRO_TELEMETRY_DISABLED: "1", NO_COLOR: "1" }, stdio: "pipe" });
    child = spawn(process.execPath, [resolve("node_modules/astro/bin/astro.mjs"), "dev", "--ignore-lock", "--root", root, "--host", "127.0.0.1", "--port", "0"], { cwd: root, env: { ...process.env, ASTRO_TELEMETRY_DISABLED: "1", NO_COLOR: "1" }, stdio: ["ignore", "pipe", "pipe"] });
    const record = (chunk) => { logs += chunk.toString(); };
    child.stdout.on("data", record); child.stderr.on("data", record);
    const address = await until(() => logs.match(/http:\/\/127\.0\.0\.1:\d+/)?.[0]);
    const read = async (path) => { const response = await fetch(address + path); return { status: response.status, text: await response.text() }; };
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
    git("commit", "--allow-empty", "-m", "New commit without source edits");
    await until(() => notifications.some((message) => message.event === "tracker:git-changed"));
    assert.ok(notifications.some(message => message.type === "full-reload"), "Git changes reload all open pages, including the report browser and map's cached index");
    await until(async () => (await read("/changelog/")).text.includes("<h1>New commit without source edits</h1>"));
    const older = await read("/changelog/2/");
    assert.equal(older.status, 200);
    assert.ok(older.text.includes("<h1>Initial history</h1>"));
    writeFileSync(join(root, "reports/new-report.md"), report("Newly committed report", "new-report"));
    writeFileSync(join(root, "reports/initial.md"), report("Updated report"));
    // Let Astro persist the content-loader update before testing Git-only cache invalidation.
    // The store debounces disk writes; an immediate commit can otherwise reload its old snapshot.
    await until(async () => (await read("/api/index.json")).text.includes("Newly committed report"));
    git("add", "reports"); git("commit", "-m", "Add and update reports");
    await until(async () => (await read("/changelog/")).text.includes("Add and update reports"));
    await until(async () => (await read("/api/index.json")).text.includes("Newly committed report"));
    await until(async () => (await read("/report/new-report/")).text.includes("<h1>Newly committed report</h1>"));
    await until(async () => (await read("/report/initial/")).text.includes("<h1>Updated report</h1>"));
    assert.ok((await read("/changelog/")).text.includes('href="/report/new-report/"'), "Changelog links include newly committed reports");
    const index = JSON.parse((await read("/api/index.json")).text);
    assert.ok(index.reports.some(r => r.i === "new-report"));
    // Request without following the short URL redirect, then exercise its target above.
    assert.equal((await fetch(address + `/r/${codeOf("new-report")}/`, { redirect: "manual" })).status, 301);
    // Branch/revision changes also invalidate cached paths, including removed pages.
    git("checkout", "--detach", "HEAD~2");
    await until(async () => (await read("/changelog/")).text.includes("<h1>Initial history</h1>"));
    assert.equal((await read("/changelog/2/")).status, 404);
    await until(async () => !(await read("/api/index.json")).text.includes("Newly committed report"));
    assert.equal((await read("/report/new-report/")).status, 404);
    assert.ok((await read("/report/initial/")).text.includes("<h1>Initial report</h1>"));
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
