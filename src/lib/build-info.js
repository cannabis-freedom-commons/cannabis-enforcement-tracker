// What this build of the site is: the commit it was built from (GitHub sets GITHUB_SHA; a local
// build asks Git), when it was built, how many reports it has and when the newest was added. The
// pages carry the id and /version.json says what is deployed now, so an open page can tell that a
// newer build is out (src/scripts/site-updates.js).
import { execSync } from "node:child_process";
import { reports } from "./reports.js";

const commit = process.env.GITHUB_SHA || (() => {
  try { return execSync("git rev-parse HEAD", { stdio: ["ignore", "pipe", "ignore"] }).toString().trim(); } catch { return ""; }
})();
const builtAt = new Date().toISOString();
const latest = reports.reduce((max, r) => (r.added_at && r.added_at > max ? r.added_at : max), "");

export const build = {
  // the commit, and the report count in case one commit is built with different reports
  id: `${commit.slice(0, 12) || builtAt}-${reports.length}`,
  commit: commit || null,
  built_at: builtAt,
  reports: reports.length,
  latest_added: latest || null,
};
