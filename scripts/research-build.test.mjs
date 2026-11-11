import test from "node:test";
import assert from "node:assert/strict";
import { execFileSync } from "node:child_process";
import fs from "node:fs";
import os from "node:os";
import path from "node:path";
import { fileURLToPath } from "node:url";

const repo = path.dirname(path.dirname(fileURLToPath(import.meta.url)));
const script = path.join(repo, "scripts", "research.sh");

// quiet_build lives inside the bash heredoc in scripts/research.sh, so the
// driver parses the real function out of that file (no copies) and calls it
// with a build.log whose parent directory does not exist. That is exactly
// what happens when a log cleanup removes run_dir while the controller
// spends minutes inside Muse before validating.
const driver = String.raw`
import ast, os, sys
from pathlib import Path
tmp, script = sys.argv[1], sys.argv[2]
text = Path(script).read_text()
prog = text.split("<<'PY'\n", 1)[1].rsplit("\nPY", 1)[0]
mod = ast.parse(prog)
import fcntl, threading, time, re, subprocess
from collections import deque
ns = {"fcntl": fcntl, "time": time, "os": os, "re": re,
      "subprocess": subprocess, "sys": sys, "deque": deque,
      "Path": Path, "print_lock": threading.Lock(), "repo": Path(tmp)}
for node in mod.body:
    if isinstance(node, ast.FunctionDef) and node.name in ("say", "quiet_build"):
        exec(compile(ast.Module(body=[node], type_ignores=[]), "<research>", "exec"), ns)
quiet_build = ns["quiet_build"]
mode = sys.argv[3]
if mode == "missing-parent":
    target = Path(tmp) / "run-003" / "build.log"
    assert not target.parent.exists(), "precondition: parent must be missing"
    code = quiet_build(["true"], target, None)
    assert code == 0, code
    assert target.is_file(), "build.log must exist after self-heal"
elif mode == "failing-cmd":
    target = Path(tmp) / "run-004" / "build.log"
    code = quiet_build(["/bin/sh", "-c", "echo '[ERROR] boom'; exit 3"], target, None)
    assert code == 3, code
    assert "boom" in target.read_text(), "failure output must land in build.log"
elif mode == "gate":
    import json
    cases = json.loads(sys.argv[4])
    ns2 = {"Path": Path, "repo": Path(script).parent.parent}
    scopes = None
    for node in mod.body:
        if isinstance(node, ast.Assign) and any(getattr(t, "id", "") == "scopes" for t in node.targets):
            scopes = eval(compile(ast.Expression(node.value), "<research>", "eval"), {})
        if isinstance(node, ast.FunctionDef) and node.name == "safe_report_path":
            exec(compile(ast.Module(body=[node], type_ignores=[]), "<research>", "exec"), ns2)
    assert scopes is not None and "safe_report_path" in ns2, "gate code must stay parseable"
    results = {}
    for c in cases:
        try:
            ns2["safe_report_path"](c)
            results[c] = "ok"
        except RuntimeError as e:
            results[c] = str(e)
    print(json.dumps({"scopes": scopes, "results": results}))
else:
    raise SystemExit("unknown mode: " + mode)
print("quiet_build OK: " + mode)
`;

const runDriver = (t, mode, extraArg) => {
  const dir = fs.mkdtempSync(path.join(os.tmpdir(), "cet-research-"));
  t.after(() => fs.rmSync(dir, { recursive: true, force: true }));
  const driverPath = path.join(dir, "driver.py");
  fs.writeFileSync(driverPath, driver);
  const args = extraArg === undefined ? [driverPath, dir, script, mode] : [driverPath, dir, script, mode, extraArg];
  return execFileSync("python3", args, { encoding: "utf8" });
};

test("research quiet_build recreates a missing build-log directory", (t) => {
  assert.match(runDriver(t, "missing-parent"), /quiet_build OK/);
});

test("research quiet_build logs failing commands with nonzero exit", (t) => {
  assert.match(runDriver(t, "failing-cmd"), /quiet_build OK/);
});

// safe_report_path and scopes must admit the media files a run declares in
// its manifest (report thumbnails and the media registry), while still
// refusing source files and missing paths instead of committing them.
const gateCases = [
  "reports/2024/10/leongatha-south-6525-plants-raid-2024.md",
  "src/data/media.json",
  "public/media/bb/a9/bba9a26d4ddb44e76fd82322dfcbbd06375d97f8f3ca07ef89455dc5881906d9.avif",
  "src/lib/media.js",
  "src/data/countries.json",
  "package.json",
  "public/media/00/11/0011deadbeef.avif",
  "reports/2099/01/no-such-report.md",
];

const runGate = (t) => {
  const out = runDriver(t, "gate", JSON.stringify(gateCases));
  return JSON.parse(out.split("\n")[0]);
};

test("research scopes cover reports, images and the media registry", (t) => {
  assert.deepEqual(runGate(t).scopes.sort(),
    ["public/media", "reports", "src/assets/reports", "src/data/media.json"]);
});

test("research manifest gate accepts owned report, thumbnail and registry paths", (t) => {
  const { results } = runGate(t);
  for (const c of gateCases.slice(0, 3)) assert.equal(results[c], "ok");
});

test("research manifest gate rejects source files and missing paths", (t) => {
  const { results } = runGate(t);
  for (const c of gateCases.slice(3, 6)) assert.match(results[c], /non-report path/);
  for (const c of gateCases.slice(6)) assert.match(results[c], /Unsafe or missing/);
});
