// Read Git blobs in batches. Every version of every report together is hundreds of megabytes,
// more than one child-process buffer may hold, so each batch is parsed as it arrives and only
// what `parse` returns is kept.
import { execFileSync } from "node:child_process";
import fs from "node:fs";
import path from "node:path";

export function readBlobs(cwd, hashes, parse, batch = 1000) {
  const out = new Map();
  const list = [...hashes];
  for (let start = 0; start < list.length; start += batch) {
    const data = execFileSync("git", ["cat-file", "--batch"], {
      cwd, input: list.slice(start, start + batch).join("\n") + "\n",
      maxBuffer: 256 * 1024 * 1024, stdio: ["pipe", "pipe", "ignore"],
    });
    let offset = 0;
    while (offset < data.length) {
      const end = data.indexOf(10, offset);
      const [hash, type, length] = data.subarray(offset, end).toString().split(" ");
      if (type !== "blob" || !Number.isFinite(Number(length))) throw new Error(`unavailable blob ${hash}`);
      const size = Number(length);
      out.set(hash, parse(data.subarray(end + 1, end + 1 + size).toString()));
      offset = end + size + 2;
    }
  }
  return out;
}

// A JSON cache in .cache/ for results derived from immutable Git objects (commits, blob pairs).
// Bump the name when what is stored changes. A missing or unreadable cache is only slower.
export function gitCache(cwd, name) {
  const file = path.join(cwd, ".cache", `${name}.json`);
  let entries = {};
  try { entries = JSON.parse(fs.readFileSync(file, "utf8")); } catch { /* first build */ }
  let changed = false;
  return {
    get: (key) => entries[key],
    set(key, value) { entries[key] = value; changed = true; },
    // keep only the keys still in use, so rewritten history does not grow it forever
    save(keys) {
      if (keys) {
        const kept = {};
        for (const key of keys) if (key in entries) kept[key] = entries[key];
        if (Object.keys(kept).length !== Object.keys(entries).length) changed = true;
        entries = kept;
      }
      if (!changed) return;
      try {
        fs.mkdirSync(path.dirname(file), { recursive: true });
        const tmp = `${file}.${process.pid}.tmp`;
        fs.writeFileSync(tmp, JSON.stringify(entries));
        fs.renameSync(tmp, file);
      } catch { /* read-only checkout: no cache */ }
    },
  };
}
