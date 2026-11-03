import test from "node:test";
import assert from "node:assert/strict";
import fs from "node:fs";
import os from "node:os";
import path from "node:path";
import * as yaml from "js-yaml";
import { assignCodes } from "./codes.mjs";
import { codeOf } from "../src/lib/code.js";

const report = (code) => `---\n${code == null ? "" : `code: ${code}\n`}date: 2026-01-01\n---\nOriginal body.\n`;
const fixture = (t) => {
  const dir = fs.mkdtempSync(path.join(os.tmpdir(), "cet-codes-"));
  t.after(() => fs.rmSync(dir, { recursive: true, force: true }));
  const write = (name, content) => { const f = path.join(dir, name + ".md"); fs.writeFileSync(f, content); return f; };
  const readCode = (file) => yaml.load(fs.readFileSync(file, "utf8").split("---")[1], { schema: yaml.JSON_SCHEMA }).code;
  return { dir, write, readCode };
};

test("code hash matches the backend's shared test vectors", () => {
  for (const [seed, code] of [["example-raid", "MZ48CH"], ["example-raid#1", "WJS3NQ"], ["test-0", "7XC52R"], ["chennai-airport-1478g-hydro", "211101"]]) assert.equal(codeOf(seed), code);
});

test("targeted assignment resolves collisions and preserves codes after renaming", (t) => {
  const { dir, write, readCode } = fixture(t);
  const original = report('"MZ48CH"');
  const reserved = write("reserved", original), target = write("example-raid", report()), other = write("untouched", report());
  const changed = assignCodes(dir, [target]);
  assert.equal(changed.length, 1);
  assert.equal(readCode(target), "WJS3NQ");
  assert.equal(fs.readFileSync(reserved, "utf8"), original);
  assert.equal(fs.readFileSync(other, "utf8"), report());
  const moved = path.join(dir, "renamed.md"), before = fs.readFileSync(target, "utf8");
  fs.renameSync(target, moved);
  assert.deepEqual(assignCodes(dir, [moved]), []);
  assert.equal(fs.readFileSync(moved, "utf8"), before);
});

test("numeric-looking codes stay strings, including legacy leading zeros", (t) => {
  const { dir, write, readCode } = fixture(t);
  const numeric = write("chennai-airport-1478g-hydro", report());
  const legacy = write("legacy", report("001234"));
  const exponent = write("exponent", report("12E345"));
  assignCodes(dir);
  assert.equal(readCode(numeric), "211101");
  assert.equal(readCode(legacy), "001234");
  assert.equal(readCode(exponent), "12E345");
  assert.deepEqual(assignCodes(dir), []);
});

test("duplicate and invalid existing codes fail before any files change", (t) => {
  const { dir, write } = fixture(t);
  const pending = write("pending", report()), a = write("a", report("ABC123")), b = write("b", report("ABC123"));
  assert.throws(() => assignCodes(dir), /share code/);
  assert.equal(fs.readFileSync(pending, "utf8"), report());
  fs.writeFileSync(b, report("null"));
  assert.throws(() => assignCodes(dir), /invalid code/);
  assert.equal(fs.readFileSync(a, "utf8"), report("ABC123"));
});
