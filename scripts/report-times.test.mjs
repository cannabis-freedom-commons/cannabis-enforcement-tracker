import test from "node:test";
import assert from "node:assert/strict";
import fs from "node:fs";
import os from "node:os";
import path from "node:path";
import { execFileSync } from "node:child_process";
import { assignReportTimes } from "./report-times.mjs";
import { readReportHistory } from "../src/lib/report-history.js";

const text = `---\ncode: "ABC123"\ndate: 2010-01-01\n---\nOriginal report.\n`;
function fixture(t) {
  const cwd = fs.mkdtempSync(path.join(os.tmpdir(), "cet-times-"));
  t.after(() => fs.rmSync(cwd, { recursive: true, force: true }));
  const root = path.join(cwd, "reports/2010/01");
  fs.mkdirSync(root, { recursive: true });
  const git = (...args) => execFileSync("git", args, { cwd, encoding: "utf8", stdio: "pipe" });
  git("init"); git("config", "user.name", "Test"); git("config", "user.email", "test@example.org");
  const commit = (time) => { git("add", "-A"); execFileSync("git", ["commit", "-m", "Report"], { cwd, stdio: "pipe", env: { ...process.env, GIT_AUTHOR_DATE: time, GIT_COMMITTER_DATE: time } }); };
  return { cwd, root: path.join(cwd, "reports"), file: path.join(root, "original.md"), git, commit };
}
test("new timestamps survive edits and folder/slug moves", t => {
  const { cwd, root, file, commit } = fixture(t);
  fs.writeFileSync(file, text);
  const at = new Date("2026-11-03T12:45:37Z");
  assert.equal(assignReportTimes(root, [file], cwd, at)[0].added_at, at.toISOString());
  commit("2026-11-03T13:00:00Z");
  const renamed = path.join(root, "renamed.md");
  fs.renameSync(file, renamed);
  fs.appendFileSync(renamed, "Updated.\n");
  const before = fs.readFileSync(renamed, "utf8");
  assert.deepEqual(assignReportTimes(root, [renamed], cwd), []);
  assert.equal(fs.readFileSync(renamed, "utf8"), before);
  assert.doesNotMatch(before, /^updated_at:/m);
});
test("legacy Git backfill follows renamed codes, preserves timestamp origins and does not stamp migration time", t => {
  const { cwd, root, file, commit } = fixture(t);
  fs.writeFileSync(file, text);
  commit("2026-10-28T10:00:00Z");
  assert.equal(assignReportTimes(root, undefined, cwd).length, 1); // Git times are persisted
  assert.deepEqual(assignReportTimes(root, undefined, cwd), []); // metadata-only migration is idempotent
  fs.writeFileSync(file, text); // also exercise legacy code lookup after a rename
  const renamed = path.join(root, "renamed.md");
  fs.renameSync(file, renamed);
  const changed = assignReportTimes(root, [renamed], cwd);
  assert.equal(changed[0].added_at, "2026-10-28T10:00:00.000Z");
  assert.match(fs.readFileSync(renamed, "utf8"), /added_at_source: git/);
});
test("shallow histories leave legacy addition times unknown", t => {
  const { cwd, root, file, commit, git } = fixture(t);
  fs.writeFileSync(file, text);
  commit("2026-10-28T10:00:00Z");
  fs.appendFileSync(file, "Edited.\n");
  commit("2026-10-29T10:00:00Z");
  const clone = path.join(cwd, "shallow");
  git("clone", "--depth=1", `file://${cwd}`, clone);
  const record = readReportHistory(clone).get("reports/2010/01/original.md");
  assert.equal(record.addedAt, null);
  assert.match(record.date, /^2026-10-29/);
  assert.deepEqual(assignReportTimes(path.join(clone, "reports"), undefined, clone), []);
});

test("Git derives edit times while ignoring persisted addition metadata", t => {
  const { cwd, root, file, commit } = fixture(t);
  fs.writeFileSync(file, text);
  commit("2026-10-28T10:00:00Z");
  fs.appendFileSync(file, "An actual later change.\n");
  commit("2026-10-29T11:00:00Z");
  assignReportTimes(root, undefined, cwd, new Date("2026-11-03T14:00:00Z"));
  assert.doesNotMatch(fs.readFileSync(file, "utf8"), /^updated_at:/m);
  assert.deepEqual(assignReportTimes(root, undefined, cwd), []);
  commit("2026-11-03T14:00:00Z");
  const record = readReportHistory(cwd).get("reports/2010/01/original.md");
  assert.equal(record.updatedAt, "2026-10-29T11:00:00.000Z");
  assert.match(record.url, /\/blob\/main\//);
  assert.deepEqual(assignReportTimes(root, undefined, cwd), []);
  fs.appendFileSync(file, "Another editorial change.\n");
  commit("2026-11-03T15:00:00Z");
  assert.equal(readReportHistory(cwd).get("reports/2010/01/original.md").updatedAt, "2026-11-03T15:00:00.000Z");
});

test("persisting only an addition time does not make an untouched report look updated", t => {
  const { cwd, root, file, commit } = fixture(t);
  fs.writeFileSync(file, text);
  commit("2026-10-28T10:00:00Z");
  assignReportTimes(root, undefined, cwd);
  commit("2026-11-03T14:00:00Z");
  assert.equal(readReportHistory(cwd).get("reports/2010/01/original.md").updatedAt, null);
});
