// Persist permanent CET identifiers before building/committing reports.
// npm run codes [-- reports/<year>/<month>/<slug>.md ...]
// Without paths, fill missing codes across the repository. Explicit paths change only those files.
import fs from "node:fs";
import path from "node:path";
import { fileURLToPath } from "node:url";
import * as yaml from "js-yaml";
import { codeOf } from "../src/lib/code.js";

const pattern = /^[0-9A-HJKMNP-TV-Z]{6}$/;

export function assignCodes(root = "reports", targets) {
  const files = fs.readdirSync(root, { recursive: true }).filter((f) => f.endsWith(".md")).sort().map((f) => path.resolve(root, f));
  const selected = new Set(targets ? targets.map((f) => path.resolve(f)) : files);
  for (const file of selected) if (!files.includes(file)) throw new Error(`not a report in ${root}: ${file}`);
  const used = new Map(), pending = [];
  for (const file of files) {
    const text = fs.readFileSync(file, "utf8");
    const header = text.match(/^---\r?\n([\s\S]*?)\r?\n---(?:\r?\n|$)/);
    if (!header) throw new Error(`${file}: no front matter`);
    const data = yaml.load(header[1], { schema: yaml.JSON_SCHEMA });
    let code = data.code;
    // Older codes were written unquoted. Preserve the six original characters, including zeros,
    // when YAML interpreted them as numbers (or scientific notation).
    const bare = header[1].match(/^code: ([0-9A-HJKMNP-TV-Z]{6})\s*$/m)?.[1];
    if (typeof code !== "string" && bare) code = bare;
    if (Object.hasOwn(data, "code")) {
      if (typeof code !== "string" || !pattern.test(code)) throw new Error(`${file}: invalid code`);
      if (used.has(code)) throw new Error(`${file} and ${used.get(code)} share code ${code}`);
      used.set(code, file);
    }
    if (selected.has(file) && (code == null || typeof data.code !== "string")) pending.push({ file, text, data, header, code });
  }
  const changed = [];
  for (let { file, text, data, header, code } of pending) {
    if (code == null) {
      const slug = path.basename(file, ".md");
      let k = 0;
      do code = codeOf(k ? `${slug}#${k}` : slug), k++; while (used.has(code));
      used.set(code, file);
    }
    if (header[1].trimStart().startsWith("{")) {
      text = text.replace(header[1], yaml.dump({ ...data, code }, { schema: yaml.JSON_SCHEMA }).trimEnd());
    } else if (data.code != null) {
      text = text.replace(/^code: .*$/m, `code: "${code}"`);
    } else {
      text = text.replace(/^---\r?\n/, (start) => `${start}code: "${code}"\n`);
    }
    fs.writeFileSync(file, text);
    changed.push({ file, code });
  }
  return changed;
}

if (process.argv[1] && path.resolve(process.argv[1]) === fileURLToPath(import.meta.url)) {
  const changed = assignCodes("reports", process.argv.length > 2 ? process.argv.slice(2) : undefined);
  for (const { file, code } of changed) console.log(`${path.relative(process.cwd(), file)}: CET-${code}`);
  if (!changed.length) console.log("all selected reports have a code");
}
