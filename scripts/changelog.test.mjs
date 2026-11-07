import test from "node:test";
import assert from "node:assert/strict";
import { mkdtempSync, rmSync, mkdirSync, writeFileSync, renameSync, unlinkSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { execFileSync } from "node:child_process";
import { readChangelog, paginateChangelog, changelogTimeline } from "../src/lib/changelog.js";

test("history reads committed titles and identifies additions, edits, renames and deletions", () => {
  const cwd = mkdtempSync(join(tmpdir(), "tracker-changelog-"));
  const git = (...args) => execFileSync("git", args, { cwd, stdio: "pipe" });
  const report = (title, figure) => `---\ntitle: ${title}\ncountry: co\ntype: seizure\nalso: [arrest]\ndate: 2026-10-01\nsummary: Police arrested ${figure} people.\nfigures:\n  arrested: ${figure}\n---\nReport text.\n`;
  const commit = (message) => { git("add", "-A"); git("commit", "-m", message); };
  try {
    git("init"); git("config", "user.name", "Test"); git("config", "user.email", "test@example.org");
    mkdirSync(join(cwd, "reports/2026/10"), { recursive: true });
    const oldPath = "reports/2026/10/old.md", newPath = "reports/2026/10/new.md";
    writeFileSync(join(cwd, oldPath), report("Police seize cannabis — Bogotá", 2));
    commit("Add report");
    writeFileSync(join(cwd, oldPath), report("Police seize cannabis — Bogotá", 3));
    commit("Correct figure");
    renameSync(join(cwd, oldPath), join(cwd, newPath));
    commit("Rename report");
    unlinkSync(join(cwd, newPath));
    commit("Delete report");
    writeFileSync(join(cwd, "README.md"), "Site change"); commit("Update site");
    writeFileSync(join(cwd, newPath), report("Uncommitted report", 99));
    const { commits, shallow } = readChangelog(cwd);
    assert.equal(shallow, false);
    assert.equal(commits.length, 5);
    assert.equal(commits[0].otherFiles, 1);
    assert.deepEqual(commits.slice(1).map((c) => c.changes[0].kind), ["deleted", "renamed", "edited", "added"]);
    assert.equal(commits[1].changes[0].title, "Police seize cannabis — Bogotá");
    assert.equal(commits[2].changes[0].oldPath, oldPath);
    assert.equal(commits[2].changes[0].path, newPath);
    assert.equal(commits[1].changes[0].country, "co");
    assert.equal(commits[1].changes[0].date, "2026-10-01");
    assert.deepEqual(commits[1].changes[0].categories, ["seizure", "arrest"]);
    assert.equal(commits[1].changes[0].description, "Police arrested 3 people.");
    assert.equal(commits[4].changes[0].description, "Police arrested 2 people.");
    assert.deepEqual(commits[3].changes[0].fields, ["summary", "figures"]);
    // the second read comes from the cache and matches; a new commit is read and added to it
    assert.deepEqual(readChangelog(cwd).commits, commits);
    commit("Re-add report");
    const again = readChangelog(cwd).commits;
    assert.equal(again.length, 6);
    assert.equal(again[0].changes[0].kind, "added");
    assert.equal(again[0].changes[0].title, "Uncommitted report");
    assert.deepEqual(again.slice(1), commits);
  } finally { rmSync(cwd, { recursive: true, force: true }); }
});

test("report versions are read in batches, beyond one child-process buffer", async () => {
  const { readBlobs } = await import("../src/lib/git-blobs.js");
  const cwd = mkdtempSync(join(tmpdir(), "tracker-blobs-"));
  try {
    execFileSync("git", ["init"], { cwd, stdio: "pipe" });
    const hashes = ["a", "b", "c"].map((text) => execFileSync("git", ["hash-object", "-w", "--stdin"], { cwd, input: text }).toString().trim());
    const read = readBlobs(cwd, hashes, (text) => text.toUpperCase(), 2);
    assert.deepEqual([...read.values()], ["A", "B", "C"]);
  } finally { rmSync(cwd, { recursive: true, force: true }); }
});

test("pagination bounds large imports and commits without reports without losing changes", () => {
  const commits = [
    { hash: "a", changes: Array.from({ length: 125 }, (_, i) => ({ title: `Report ${i}` })) },
    ...Array.from({ length: 80 }, (_, i) => ({ hash: String(i), changes: [] })),
  ];
  const pages = paginateChangelog(commits);
  assert.equal(pages.length, 5);
  assert.ok(pages.every((page) => page.reduce((n, c) => n + Math.max(c.changes.length, 1), 0) <= 50));
  assert.deepEqual(pages.flat().flatMap((c) => c.changes), commits[0].changes);
  assert.equal(pages[1][0].continued, true);
  assert.equal(pages.flat().filter((c) => !c.changes.length).length, 80);
});

test("timeline groups by UTC commit time, includes gaps and counts changes once", () => {
  const commits = [
    { hash: "new", date: "2026-10-30T03:30:00+01:00", changes: [{ kind: "edited" }, { kind: "deleted" }] },
    { hash: "old", date: "2026-10-30T00:30:00Z", changes: [{ kind: "added" }] },
  ];
  const { buckets, interval } = changelogTimeline(commits);
  assert.equal(interval, "hour");
  assert.equal(buckets.length, 3);
  assert.equal(buckets[0].added, 1);
  assert.equal(buckets[1].commits, 0);
  assert.equal(buckets[2].edited, 1);
  assert.equal(buckets[2].deleted, 1);
  assert.equal(buckets[2].hash, "new");
  assert.deepEqual(changelogTimeline([]).buckets, []);
  const longHistory = changelogTimeline([
    ...commits,
    { hash: "ancient", date: "2000-01-01T00:00:00Z", changes: [{ kind: "renamed" }] },
  ]);
  assert.ok(longHistory.buckets.length <= 61);
  assert.equal(longHistory.buckets.reduce((n, bucket) => n + bucket.renamed, 0), 1);
});
