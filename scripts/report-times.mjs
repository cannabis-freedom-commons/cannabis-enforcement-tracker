// Persist tracker times independently of incident dates. Git backfill never uses migration time.
// `npm run codes -- <paths>` also calls this; metadata-only changes do not count as report edits.
import fs from "node:fs";
import path from "node:path";
import { fileURLToPath } from "node:url";
import { execFileSync } from "node:child_process";
import * as yaml from "js-yaml";
import { readReportHistory } from "../src/lib/report-history.js";

const parse = (text) => {
  const header = text.match(/^---\r?\n([\s\S]*?)\r?\n---(?:\r?\n|$)/);
  if (!header) throw new Error("no front matter");
  return { header, data: yaml.load(header[1], { schema: yaml.JSON_SCHEMA }), body: text.slice(header[0].length).trim() };
};
export function assignReportTimes(root = "reports", targets, cwd = process.cwd(), now = new Date()) {
  const files = fs.readdirSync(root, { recursive: true }).filter(f => f.endsWith(".md")).map(f => path.resolve(root, f));
  const selected = targets ? targets.map(f => path.resolve(f)) : files;
  for (const file of selected) if (!files.includes(file)) throw new Error(`not a report in ${root}: ${file}`);
  const history = readReportHistory(cwd), changed = [];
  const byName = new Map([...history].map(([name, value]) => [path.basename(name), { ...value, path: name }]));
  for (const file of selected) {
    const original = fs.readFileSync(file, "utf8");
    let text = original;
    const { header, data } = parse(text);
    const rel = path.relative(cwd, file).replaceAll("\\", "/");
    let previous = history.get(rel) ?? byName.get(path.basename(file));
    if (!previous && data.code) {
      try {
        const names = execFileSync("git", ["grep", "-l", "-E", `^code: ["']?${data.code}["']?$`, "HEAD", "--", "reports"], { cwd, encoding: "utf8", stdio: ["ignore", "pipe", "ignore"] }).trim().split("\n");
        const origin = names[0]?.replace(/^HEAD:/, "");
        previous = origin ? { ...(history.get(origin) ?? { addedAt: null }), path: origin } : undefined;
      } catch { /* new report */ }
    }
    if (!previous) {
      try {
        execFileSync("git", ["cat-file", "-e", `HEAD:${rel}`], { cwd, stdio: "ignore" });
        previous = { addedAt: null }; // tracked report with unavailable history
      } catch { /* new report */ }
    }
    const fields = {};
    if (!Object.hasOwn(data, "added_at") && (!previous || previous.addedAt)) {
      fields.added_at = previous?.addedAt ?? now.toISOString();
      if (previous) fields.added_at_source = "git";
    }
    if (!Object.keys(fields).length) continue;
    if (header[1].trimStart().startsWith("{")) text = text.replace(header[1], yaml.dump({ ...data, ...fields }, { schema: yaml.JSON_SCHEMA }).trimEnd());
    else {
      for (const [key, value] of Object.entries(fields)) {
        const line = `${key}: ${key === "added_at_source" ? value : JSON.stringify(value)}`;
        if (Object.hasOwn(data, key)) text = text.replace(new RegExp(`^${key}:.*$`, "m"), line);
        else text = text.replace(/^---\r?\n/, start => `${start}${line}\n`);
      }
    }
    if (fs.readFileSync(file, "utf8") !== original) throw new Error(`${file}: changed while recording timestamps`);
    fs.writeFileSync(file, text);
    changed.push({ file, ...fields });
  }
  return changed;
}

if (process.argv[1] && path.resolve(process.argv[1]) === fileURLToPath(import.meta.url)) {
  const changed = assignReportTimes("reports", process.argv.length > 2 ? process.argv.slice(2) : undefined);
  console.log(`Recorded timestamps in ${changed.length} reports.`);
}
