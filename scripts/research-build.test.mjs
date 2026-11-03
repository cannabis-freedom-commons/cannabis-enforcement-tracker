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
print("quiet_build OK: " + mode)
`;

const runDriver = (t, mode) => {
  const dir = fs.mkdtempSync(path.join(os.tmpdir(), "cet-research-"));
  t.after(() => fs.rmSync(dir, { recursive: true, force: true }));
  const driverPath = path.join(dir, "driver.py");
  fs.writeFileSync(driverPath, driver);
  const out = execFileSync("python3", [driverPath, dir, script, mode], { encoding: "utf8" });
  assert.match(out, /quiet_build OK/);
};

test("research quiet_build recreates a missing build-log directory", (t) => {
  runDriver(t, "missing-parent");
});

test("research quiet_build logs failing commands with nonzero exit", (t) => {
  runDriver(t, "failing-cmd");
});
