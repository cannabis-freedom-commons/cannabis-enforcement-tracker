import { execFileSync } from "node:child_process";
import { isDeepStrictEqual } from "node:util";
import { load } from "js-yaml";
import { repositoryURL } from "./changelog.js";

export const reportSourceURL = (path) => `${repositoryURL}/blob/main/${path.split("/").map(encodeURIComponent).join("/")}`;

// Timestamp backfills change metadata, not the report's editorial update time.
function editorial(text) {
  const match = text.match(/^---\r?\n([\s\S]*?)\r?\n---(?:\r?\n|$)([\s\S]*)/);
  if (!match) return text;
  try {
    const { added_at, added_at_source, updated_at, ...fields } = load(match[1]) ?? {};
    return { fields, body: match[2].trim() };
  } catch { return text; }
}

// One history walk and one blob batch for all reports. Follow renames, compare committed
// content, and never substitute checkout/build time for a missing historical timestamp.
export function readReportHistory(cwd = process.cwd()) {
  const history = new Map();
  const git = (args, options = {}) => execFileSync("git", args, { cwd, encoding: "utf8", maxBuffer: 256 * 1024 * 1024, stdio: ["ignore", "pipe", "ignore"], ...options });
  let output, shallow;
  try {
    shallow = git(["rev-parse", "--is-shallow-repository"]).trim() === "true";
    output = git(["log", "--reverse", "--first-parent", "--root", "--diff-merges=first-parent", "--find-renames", "--raw", "--no-abbrev", "-z", "--format=%x00REPORT_COMMIT%x00%cI%x00", "--", "reports"]);
  } catch { return history; }
  const tokens = output.split("\0"), changes = [], blobs = new Set();
  let date;
  for (let i = 0; i < tokens.length; i++) {
    const token = tokens[i].replace(/^\n+/, "");
    if (token === "REPORT_COMMIT") { date = tokens[++i]; continue; }
    if (!date || !token.startsWith(":")) continue;
    const [, , before, after, status] = token.slice(1).split(" ");
    const oldPath = tokens[++i], path = /^[RC]/.test(status) ? tokens[++i] : oldPath;
    const compare = !/^[ADC]/.test(status) && before !== after && !/^0+$/.test(before) && !/^0+$/.test(after);
    changes.push({ date, oldPath, path, before, after, status, compare });
    if (compare) { blobs.add(before); blobs.add(after); }
  }
  const contents = new Map();
  if (blobs.size) {
    try {
      const data = git(["cat-file", "--batch"], { encoding: null, input: [...blobs].join("\n") + "\n", stdio: ["pipe", "pipe", "ignore"] });
      let offset = 0;
      while (offset < data.length) {
        const end = data.indexOf(10, offset);
        const [hash, type, length] = data.subarray(offset, end).toString().split(" ");
        if (type !== "blob" || !Number.isFinite(Number(length))) throw new Error("unavailable report blob");
        const size = Number(length);
        contents.set(hash, editorial(data.subarray(end + 1, end + 1 + size).toString()));
        offset = end + size + 2;
      }
    } catch { /* Missing historical objects leave update times unknown. */ }
  }
  for (const change of changes) {
    const { date, oldPath, path, before, after, status, compare } = change;
    const previous = history.get(oldPath);
    if (status === "D" || status.startsWith("R")) history.delete(oldPath);
    if (status === "D" || !path?.startsWith("reports/") || !path.endsWith(".md")) continue;
    const added = status === "A" || status.startsWith("C");
    const edited = compare && contents.has(before) && contents.has(after) && !isDeepStrictEqual(contents.get(before), contents.get(after));
    history.set(path, {
      date,
      addedAt: shallow ? null : added ? new Date(date).toISOString() : previous?.addedAt ?? null,
      updatedAt: added ? null : edited ? new Date(date).toISOString() : previous?.updatedAt ?? null,
      url: reportSourceURL(path),
    });
  }
  return history;
}
