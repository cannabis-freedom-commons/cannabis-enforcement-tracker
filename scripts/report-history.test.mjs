import test from "node:test";
import assert from "node:assert/strict";
import { mkdtempSync, mkdirSync, writeFileSync, renameSync, rmSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { execFileSync } from "node:child_process";
import { readReportHistory } from "../src/lib/report-history.js";

test("report update time follows its last commit, including renames, not unrelated commits or local edits", () => {
  const cwd = mkdtempSync(join(tmpdir(), "tracker-report-history-"));
  const git = (...args) => execFileSync("git", args, { cwd, encoding: "utf8", stdio: ["ignore", "pipe", "pipe"] }).trim();
  const commit = (date) => {
    git("add", "-A");
    execFileSync("git", ["commit", "-m", "Fixture"], { cwd, stdio: "pipe", env: { ...process.env, GIT_AUTHOR_DATE: date, GIT_COMMITTER_DATE: date } });
    return git("rev-parse", "HEAD");
  };
  try {
    assert.equal(readReportHistory(cwd).size, 0);
    git("init"); git("config", "user.name", "Test"); git("config", "user.email", "test@example.org");
    mkdirSync(join(cwd, "reports/2026/10"), { recursive: true });
    const path = "reports/2026/10/one.md", renamed = "reports/2026/10/new name.md";
    writeFileSync(join(cwd, path), "Original report with enough unchanged text to identify its later rename.\n");
    const first = commit("2026-10-28T10:00:00Z");
    writeFileSync(join(cwd, "README.md"), "Unrelated update");
    commit("2026-10-29T10:00:00Z");
    writeFileSync(join(cwd, path), "Original report with enough unchanged text to identify its later rename.\nUncommitted edit\n");
    writeFileSync(join(cwd, "reports/2026/10/untracked.md"), "Untracked report");
    let history = readReportHistory(cwd);
    assert.equal(new Date(history.get(path).date).toISOString(), "2026-10-28T10:00:00.000Z");
    assert.equal(history.get(path).updatedAt, null);
    assert.equal(history.get(path).addedAt, "2026-10-28T10:00:00.000Z");
    assert.ok(history.get(path).url.endsWith(`/blob/main/${path}`));
    assert.equal(history.has("reports/2026/10/untracked.md"), false);
    renameSync(join(cwd, path), join(cwd, renamed));
    const last = commit("2026-10-30T11:15:00Z");
    history = readReportHistory(cwd);
    assert.equal(new Date(history.get(renamed).date).toISOString(), "2026-10-30T11:15:00.000Z");
    assert.equal(history.get(renamed).updatedAt, "2026-10-30T11:15:00.000Z");
    assert.equal(history.get(renamed).addedAt, "2026-10-28T10:00:00.000Z");
    assert.ok(history.get(renamed).url.endsWith(`/blob/main/reports/2026/10/new%20name.md`));
  } finally { rmSync(cwd, { recursive: true, force: true }); }
});
