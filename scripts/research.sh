#!/usr/bin/env bash
# Bash entrypoint; Python manages process isolation, streaming logs and graceful Ctrl+C.
exec python3 - "$@" <<'PY'
import datetime as dt
import fcntl
import time
from collections import deque
import json
import os
from pathlib import Path
import re
import shlex
import shutil
import signal
import tempfile
import subprocess
import sys
import threading

build_only = len(sys.argv) == 2 and sys.argv[1] == "--build"
fix_only = len(sys.argv) == 3 and sys.argv[1] == "--fix-report"
if not (build_only or fix_only) and (len(sys.argv) != 2 or not re.fullmatch(r"\d{4}-\d{4}", sys.argv[1])):
    sys.exit("Usage: ./scripts/research.sh YYYY-YYYY | --build | --fix-report reports/YYYY/MM/name.md")
if not (build_only or fix_only):
    years = sys.argv[1]
    a, b = map(int, years.split("-"))
    if a > b:
        sys.exit("Error: starting year must not exceed ending year")
    if not shutil.which("muse"):
        sys.exit("Error: muse executable not found in PATH")

repo_result = subprocess.run(["git", "rev-parse", "--show-toplevel"],
                             text=True, capture_output=True)
if repo_result.returncode:
    sys.exit("Error: run this script inside your tracker Git repository")
repo = Path(repo_result.stdout.strip()).resolve()
if not (repo / ".claude/skills/find-reports/SKILL.md").is_file():
    sys.exit("Error: .claude/skills/find-reports/SKILL.md not found")

os.umask(0o077)  # Logs can contain source text and tool diagnostics.
log_base = Path(os.environ.get(
    "RESEARCH_LOG_DIR", str(Path.home() / "Library/Logs/cannabis-tracker-research")
)).expanduser().resolve()
stamp = dt.datetime.now().strftime("%Y%m%d-%H%M%S-%f")
log_dir = log_base / ("builds" if build_only else "repairs" if fix_only else years) / stamp
log_dir.mkdir(parents=True, exist_ok=False)

# Only stage report Markdown and report images; never stage source code, logs, or .git changes.
scopes = ["reports", "src/assets/reports"]
stop_requested = False
print_lock = threading.Lock()


def on_interrupt(_sig, _frame):
    global stop_requested
    if not stop_requested:
        stop_requested = True
        say("\n⏳ Ctrl+C received: finishing the current run, validation and commit; then stopping.")
    else:
        say("\n⏳ Still finishing the active run. No next run will start.")


signal.signal(signal.SIGINT, on_interrupt)


def say(message, log=None, end="\n"):
    with print_lock:
        sys.stdout.write(message + end)
        sys.stdout.flush()
        if log is not None:
            log.write(message + end)
            log.flush()


def git(*args, check=True):
    p = subprocess.run(["git", *args], cwd=repo, text=True,
                       capture_output=True, start_new_session=True)
    if check and p.returncode:
        raise RuntimeError(f"git {' '.join(args)} failed:\n{p.stderr or p.stdout}")
    return p


def pending_report_changes():
    return git("status", "--porcelain=v1", "--untracked-files=all", "--", *scopes).stdout.strip()


def changed_report_paths():
    tracked = git("diff", "--name-only", "-z", "HEAD", "--", *scopes).stdout.split("\0")
    untracked = git("ls-files", "--others", "--exclude-standard", "-z", "--", *scopes).stdout.split("\0")
    return sorted(set(filter(None, tracked + untracked)))


def compact(value, width=190):
    """One-line summary; tool outputs stay intact in muse.jsonl."""
    value = " ".join(str(value).split())
    return value if len(value) <= width else value[:width - 1] + "…"


def tool_details(payload, name):
    """Best-effort target from RESULT metadata (Muse JSONL omits tool inputs)."""
    facts = payload.get("edit_facts") or {}
    if isinstance(facts, dict):
        for key in ("path", "file_path", "target_path", "filename", "target"):
            value = facts.get(key)
            if isinstance(value, str) and value:
                return compact(value)

    raw = payload.get("text") or ""
    if not isinstance(raw, str):
        return ""

    # Tool results sometimes wrap structured tool outputs as JSON strings.
    try:
        obj = json.loads(raw)
    except (ValueError, TypeError):
        obj = None
    if isinstance(obj, dict):
        for container in (obj, obj.get("result"), obj.get("data")):
            if not isinstance(container, dict):
                continue
            if name in ("bash", "shell"):
                for key in ("description", "command", "cmd"):
                    val = container.get(key)
                    if isinstance(val, str) and val:
                        return compact(val)
            if name in ("read_file", "write_file", "edit_file", "read", "write", "edit"):
                for key in ("path", "file_path", "target_path", "filename"):
                    val = container.get(key)
                    if isinstance(val, str) and val:
                        return compact(val)
            if name in ("web_fetch", "web_search", "web_open", "web_get"):
                for key in ("url", "source_url", "requested_url", "final_url", "query"):
                    val = container.get(key)
                    if isinstance(val, str) and val:
                        return compact(val)

    # Only explicit result metadata, never grab the first random URL from page text.
    if name in ("web_fetch", "web_search", "web_open", "web_get"):
        pat = r"(?im)^(?:fetched|requested|source|final)?[ -]?url\s*[:=]\s*(https?://\S+)"
    elif name in ("read_file", "write_file", "edit_file"):
        pat = r"(?im)^(?:file|file_path|path|wrote|edited|reading)\s*[:=]\s*(\S+)"
    else:
        return ""
    match = re.search(pat, raw)
    return compact(match.group(1)) if match else ""


def quiet_build(cmd, log_path, output_log=None):
    """Cooperative cross-process build lock + complete on-disk log.

    Also exposed as `bash scripts/research.sh --build` for Muse and other agents.
    Plain `npm run build` does NOT participate in this advisory lock.
    """
    lock_path = repo / ".cache" / "astro-build.lock"
    lock_path.parent.mkdir(parents=True, exist_ok=True)
    with lock_path.open("a+") as lock_file:
        try:
            fcntl.flock(lock_file, fcntl.LOCK_EX | fcntl.LOCK_NB)
        except BlockingIOError:
            say("⏳ Waiting for another research build to finish…", output_log)
            fcntl.flock(lock_file, fcntl.LOCK_EX)

        started = time.monotonic()
        say(f"→ Building (details: {log_path})", output_log)
        # The controller spends minutes inside Muse before validating; a log
        # cleanup may have removed run_dir meanwhile. Recreate it so one
        # missing directory cannot abort the whole run with ENOENT.
        log_path.parent.mkdir(parents=True, exist_ok=True)
        with log_path.open("w", encoding="utf-8", errors="replace") as build_log:
            try:
                proc = subprocess.Popen(cmd, cwd=repo, stdin=subprocess.DEVNULL,
                                        stdout=build_log, stderr=subprocess.STDOUT,
                                        start_new_session=True)
                code = proc.wait()
            except OSError as e:
                build_log.write(f"Could not launch validation: {e}\n")
                code = 127
        elapsed = time.monotonic() - started
        if code == 0:
            say(f"✓ Build passed ({elapsed:.0f}s)", output_log)
        else:
            say(f"✗ Build failed (exit {code}, {elapsed:.0f}s). Log: {log_path}", output_log)
            # Errors can appear thousands of lines before the final progress line.
            # Pick actionable errors, then a small tail if no matching lines exist.
            diagnostic = deque(maxlen=14)
            tail = deque(maxlen=12)
            with log_path.open(encoding="utf-8", errors="replace") as src:
                for ln in src:
                    line = ln.rstrip("\n")
                    tail.append(line)
                    if re.search(r"\[ERROR\]|(?:\bError:|\bERROR:|ERR_MODULE_NOT_FOUND|Cannot find|failed|EPERM|EACCES)", line, re.I):
                        diagnostic.append(line[:350])
            for line in (diagnostic if diagnostic else tail):
                say("    " + line, output_log)
        fcntl.flock(lock_file, fcntl.LOCK_UN)
        return code


def isolated_build(owned, out, run_dir, name):
    """Validate HEAD + owned files in a throwaway worktree, ignoring live dirt.

    A full-tree `astro build` loads every reports/*.md in the working tree, so a
    concurrent worker's half-written report (e.g. `code` not yet assigned) fails
    validation for this run's finished files. This instead builds an isolated
    snapshot — HEAD plus exactly this run's owned paths overlaid — without
    touching the live tree or any other worker's files. Returns the exit code.
    """
    base = Path(tempfile.mkdtemp(prefix="research-validate-"))
    wt = base / "worktree"
    log_path = run_dir / f"{name}.log"
    log_path.parent.mkdir(parents=True, exist_ok=True)
    say(f"→ Isolated validation of {len(owned)} owned file(s) (details: {log_path})", out)
    started = time.monotonic()
    code = 127
    lock_path = repo / ".cache" / "astro-build.lock"
    lock_path.parent.mkdir(parents=True, exist_ok=True)
    with lock_path.open("a+") as lock_file:
        try:
            fcntl.flock(lock_file, fcntl.LOCK_EX | fcntl.LOCK_NB)
        except BlockingIOError:
            say("⏳ Waiting for another research build to finish…", out)
            fcntl.flock(lock_file, fcntl.LOCK_EX)
        try:
            git("worktree", "prune", check=False)
            git("worktree", "add", "--detach", str(wt), "HEAD")
            for relative in owned:
                # Owned paths passed safe_report_path at manifest parse time; recheck
                # the live file before copying it into the snapshot.
                src = safe_report_path(relative)
                dest = wt / relative
                dest.parent.mkdir(parents=True, exist_ok=True)
                shutil.copy2(src, dest)
            # Share installs read-only but keep Astro's cache private. A whole-dir
            # symlink would point wt/node_modules/.astro at the live tree, so two
            # concurrent builds race on data-store.json.tmp renames (ENOENT).
            node_modules = wt / "node_modules"
            repo_nm = repo / "node_modules"
            if not os.path.lexists(node_modules) and repo_nm.is_dir():
                node_modules.mkdir(parents=True)
                for entry in repo_nm.iterdir():
                    if entry.name == ".astro":
                        continue
                    (node_modules / entry.name).symlink_to(entry, target_is_directory=entry.is_dir())
                (node_modules / ".astro").mkdir(exist_ok=True)
            with log_path.open("w", encoding="utf-8", errors="replace") as build_log:
                try:
                    proc = subprocess.Popen(shlex.split(validate_cmd), cwd=wt,
                                            stdin=subprocess.DEVNULL,
                                            stdout=build_log, stderr=subprocess.STDOUT,
                                            start_new_session=True)
                    code = proc.wait()
                except OSError as e:
                    build_log.write(f"Could not launch validation: {e}\n")
                    code = 127
        finally:
            git("worktree", "remove", "--force", str(wt), check=False)
            shutil.rmtree(base, ignore_errors=True)
        fcntl.flock(lock_file, fcntl.LOCK_UN)
    elapsed = time.monotonic() - started
    if code == 0:
        say(f"✓ Isolated build passed ({elapsed:.0f}s)", out)
    else:
        say(f"✗ Isolated build failed (exit {code}, {elapsed:.0f}s). Log: {log_path}", out)
        diagnostic = deque(maxlen=14)
        tail = deque(maxlen=12)
        with log_path.open(encoding="utf-8", errors="replace") as src:
            for ln in src:
                line = ln.rstrip("\n")
                tail.append(line)
                if re.search(r"\[ERROR\]|(?:\bError:|\bERROR:|ERR_MODULE_NOT_FOUND|Cannot find|failed|EPERM|EACCES)", line, re.I):
                    diagnostic.append(line[:350])
        for line in (diagnostic if diagnostic else tail):
            say("    " + line, out)
    return code


if build_only:
    code = quiet_build(shlex.split(os.environ.get("RESEARCH_VALIDATE_CMD", "npx astro build")),
                       log_dir / "build.log")
    sys.exit(code)


def run_command(cmd, log, raw_file=None, muse_json=False):
    """Run detached from terminal SIGINT; parent keeps receiving Ctrl+C."""
    proc = subprocess.Popen(cmd, cwd=repo, stdin=subprocess.DEVNULL,
                            stdout=subprocess.PIPE, stderr=subprocess.PIPE,
                            text=True, errors="replace", bufsize=1,
                            start_new_session=True)
    stderr_path = Path(log.name).parent / (f"{raw_file.stem}.stderr.log" if raw_file else "validation.stderr.log")

    def watch_errors():
        with stderr_path.open("w", encoding="utf-8") as err:
            for line in proc.stderr:
                err.write(line)
                err.flush()
                say("  " + line.rstrip("\n"), log)

    thread = threading.Thread(target=watch_errors, daemon=True)
    thread.start()
    delta_seen = False
    with (raw_file.open("w", encoding="utf-8") if raw_file else open(os.devnull, "w")) as raw:
        for line in proc.stdout:
            if not muse_json:
                say(line.rstrip("\n"), log)
                continue
            raw.write(line)
            raw.flush()
            try:
                event = json.loads(line)
            except json.JSONDecodeError:
                say(line.rstrip("\n"), log)
                continue
            kind = event.get("payload_type", "")
            payload = event.get("payload") or {}
            if kind == "tool.result":
                cf = payload.get("correlation_facts") or {}
                name = cf.get("tool_name") or "tool"
                outcome = cf.get("outcome", "done")
                detail = tool_details(payload, name)
                prefix = "✓" if outcome in ("success", "completed", "done") else "✗"
                suffix = f"  {detail}" if detail else ""
                say(f"{prefix} {name}{suffix}" + ("" if prefix == "✓" else f" ({outcome})"), log)
            elif kind == "run.output.delta":
                fragment = payload.get("text", "")
                if fragment:
                    delta_seen = True
                    say(fragment, log, end="")
            elif kind == "run.terminal.completed":
                final_text = payload.get("text", "")
                if not delta_seen and final_text:
                    say("\n" + final_text, log)
                elif delta_seen:
                    say("", log)
            elif kind.startswith("run.terminal."):
                say("! Muse: " + kind + " " + str(payload.get("reason", "")), log)
    returncode = proc.wait()
    thread.join(timeout=5)
    return returncode


validate_cmd = os.environ.get("RESEARCH_VALIDATE_CMD", "npx astro build").strip()
max_steps = os.environ.get("RESEARCH_MAX_STEPS", "120")
if not max_steps.isdecimal() or int(max_steps) < 1:
    sys.exit("RESEARCH_MAX_STEPS must be a positive integer")

# Specify both settings on the Muse CLI: never rely on its saved defaults.
muse_model = os.environ.get("RESEARCH_MODEL", "muse-spark-1.3").strip()
muse_effort = os.environ.get("RESEARCH_EFFORT", "xhigh").strip()
if not muse_model or not re.fullmatch(r"[a-zA-Z0-9._-]+", muse_model):
    sys.exit("RESEARCH_MODEL must be a Muse model identifier (e.g. muse-spark-1.3)")
if muse_effort not in ("minimal", "low", "medium", "high", "xhigh", "max", "ultra"):
    sys.exit("RESEARCH_EFFORT must be minimal, low, medium, high, xhigh, max or ultra")
provenance = f"muse-code/{muse_model}#{muse_effort}"


def set_new_report_provenance(paths, tracked_before, out):
    """Correct only new report files; preserve attribution of existing reports.

    The CLI model/effort are set explicitly by the controller. New files must
    contain these exact run settings even when the LLM writes an older shorthand.
    """
    for relative in paths:
        if relative in tracked_before or not re.fullmatch(r"reports/\d{4}/\d{2}/[^/]+\.md", relative):
            continue
        target = (repo / relative).resolve()
        if not target.is_relative_to(repo) or not target.is_file() or target.is_symlink():
            raise RuntimeError(f"Unsafe or missing new report: {relative}")
        raw = target.read_text(encoding="utf-8")
        if not raw.startswith("---\n") or "\n---\n" not in raw:
            raise RuntimeError(f"Missing YAML frontmatter in new report: {relative}")
        header, body = raw.split("\n---\n", 1)
        fields = re.findall(r"(?m)^created_by_model:[^\r\n]*$", header)
        if len(fields) > 1:
            raise RuntimeError(f"Multiple created_by_model fields: {relative}")
        if fields and fields[0] == f"created_by_model: {provenance}":
            continue
        if fields:
            previous = fields[0]
            header = header.replace(previous, f"created_by_model: {provenance}", 1)
        else:
            previous = "(missing)"
            header += f"\ncreated_by_model: {provenance}"
        target.write_text(header + "\n---\n" + body, encoding="utf-8")
        say(f"✓ Attribution: {relative}: {previous} → {provenance}", out)




def safe_report_path(relative):
    if not isinstance(relative, str) or relative.startswith("/") or "\\" in relative:
        raise RuntimeError(f"Invalid report path in manifest: {relative!r}")
    if not (relative.startswith("reports/") or relative.startswith("src/assets/reports/")):
        raise RuntimeError(f"Manifest contains non-report path: {relative!r}")
    initial = repo / relative
    path = initial.resolve()
    if not path.is_relative_to(repo) or not path.is_file() or initial.is_symlink():
        raise RuntimeError(f"Unsafe or missing report path: {relative!r}")
    return path


def parse_run_manifest(manifest, out):
    """Trust only Muse-declared changed paths; don't take other workers' files."""
    if not manifest.is_file():
        raise RuntimeError(f"Muse did not write its per-run file manifest: {manifest}. "
                           "Leaving files uncommitted, rather than guessing which worker owns them.")
    try:
        data = json.loads(manifest.read_text(encoding="utf-8"))
    except (ValueError, OSError) as exc:
        raise RuntimeError(f"Invalid run manifest {manifest}: {exc}") from exc
    values = data.get("files") if isinstance(data, dict) else None
    if not isinstance(values, list) or not all(isinstance(x, str) for x in values):
        raise RuntimeError("Run manifest must be JSON: {\"files\": [\"reports/...md\", ...]}")
    declared = sorted(set(values))
    for relative in declared:
        safe_report_path(relative)
    actual = set(changed_report_paths())
    owned = [p for p in declared if p in actual]
    stale = [p for p in declared if p not in actual]
    if stale:
        say(f"ℹ {len(stale)} declared file(s) already committed/unchanged; skipped: " + ", ".join(stale), out)
    external = sorted(actual - set(declared))
    if external:
        say("ℹ Other uncommitted report files, NOT owned by this Muse run:\n" +
            "\n".join("   " + p for p in external), out)
    return owned, external


def fix_unquoted_question_colons(paths, out):
    """Safely correct the classic '- description: details' YAML-mapping error.

    In open_questions each item must be a scalar string. This is a narrow,
    deterministic textual fix; complex malformed YAML goes to Muse repair.
    """
    for relative in paths:
        if not re.fullmatch(r"reports/\d{4}/\d{2}/[^/]+\.md", relative):
            continue
        path = safe_report_path(relative)
        raw = path.read_text(encoding="utf-8")
        if not raw.startswith("---\n") or "\n---\n" not in raw:
            continue
        header, body = raw.split("\n---\n", 1)
        lines = header.splitlines(keepends=True)
        in_questions = False
        fixed = 0
        for i, line in enumerate(lines):
            if re.match(r"^open_questions:\s*(?:#.*)?$", line.rstrip("\r\n")):
                in_questions = True
                continue
            if in_questions and re.match(r"^[a-zA-Z_][\w-]*:", line):
                in_questions = False
            if not in_questions:
                continue
            m = re.fullmatch(r"(\s+-\s+)(.*?)(\r?\n)?", line)
            if not m:
                continue
            prefix, value, ending = m.group(1), m.group(2), m.group(3) or ""
            if not re.search(r":(?=\s|$)", value) or value.startswith(("'", '"', "|", ">")):
                continue
            # Skip YAML mappings with indented continuation; a language model
            # must preserve those semantics rather than a blind string rewrite.
            next_line = lines[i + 1] if i + 1 < len(lines) else ""
            if re.match(r"^\s{4,}\S", next_line):
                continue
            lines[i] = prefix + json.dumps(value, ensure_ascii=False) + ending
            fixed += 1
        if fixed:
            path.write_text("".join(lines) + "\n---\n" + body, encoding="utf-8")
            say(f"✓ Fixed {fixed} unquoted open_questions item(s): {relative}", out)


def failing_report_paths(build_log):
    content = build_log.read_text(encoding="utf-8", errors="replace")
    matches = re.findall(r"(?:(?:[A-Za-z]:)?[^\n ]*?/)?(reports/\d{4}/\d{2}/[A-Za-z0-9._-]+\.md)", content)
    return sorted(set(matches))


def recover_failed_build(owned, tracked_before, out, run_dir, first_code, first_log, isolated=False):
    """Targeted repair once or twice, then revalidate. No broad repo edits."""
    code = first_code
    log_path = first_log
    for attempt in (1, 2):
        invalid = failing_report_paths(log_path)
        # A failure elsewhere in the project must NOT be delegated to Muse.
        to_fix = [p for p in invalid if p in owned]
        other_errors = [p for p in invalid if p not in owned]
        if other_errors:
            if not isolated:
                # Full-tree build raced with a concurrent worker's half-written
                # report. Retry HEAD + owned files in isolation before giving up.
                say("→ Full-tree validation hit unowned file(s) (concurrent worker dirt); "
                    "retrying HEAD + owned files in isolation: " + ", ".join(other_errors), out)
                isolated = True
                name = f"build-isolated-fallback-{attempt}"
                log_path = run_dir / f"{name}.log"
                code = isolated_build(owned, out, run_dir, name)
                if code == 0:
                    return 0
                continue
            say("ERROR: Build errors refer to files not owned by this run; refusing to modify them: " +
                ", ".join(other_errors), out)
            return code
        if not to_fix:
            say("ERROR: Build failed without an identifiable invalid report owned by this run. "
                "Not asking Muse to modify unrelated code or files.", out)
            return code
        say("→ Muse repair pass for: " + ", ".join(to_fix), out)
        diagnostic = log_path.read_text(encoding="utf-8", errors="replace")[-11000:]
        repair_prompt = f"""Repair report validation errors. DO NOT research new events.
Only edit these existing report files: {', '.join(to_fix)}.
Do not modify any other file, and do not git commit or run a build.
Correct the exact content-schema/YAML problem in the build error below.
`open_questions` must be an array of strings; quote colon-containing items.
Preserve all factual content, source citations, and created_by_model fields.
The controller will run validation and commit when you finish.
BUILD FAILURE LOG (tail):
{diagnostic}
"""
        fix_code = run_command([
            "muse", "exec", "--json", "--disable-approval",
            "--model", muse_model, "--reasoning-effort", muse_effort,
            "--max-model-steps", str(min(int(max_steps), 40)), repair_prompt
        ], out, run_dir / f"repair-{attempt}.jsonl", muse_json=True)
        if fix_code:
            say(f"ERROR: Muse repair pass exited {fix_code}", out)
            return fix_code
        # Reapply deterministic normalization and model attribution for new reports.
        set_new_report_provenance(to_fix, tracked_before, out)
        fix_unquoted_question_colons(to_fix, out)
        say("→ Retrying validation after targeted repair", out)
        if isolated or sorted(set(changed_report_paths()) - set(owned)):
            isolated = True
            name = f"build-isolated-retry-{attempt}"
            log_path = run_dir / f"{name}.log"
            code = isolated_build(owned, out, run_dir, name)
        else:
            log_path = run_dir / f"build-retry-{attempt}.log"
            code = quiet_build(shlex.split(validate_cmd), log_path, out)
        if code == 0:
            return 0
    return code


if fix_only:
    relative = sys.argv[2]
    if not re.fullmatch(r"reports/\d{4}/\d{2}/[^/]+\.md", relative):
        sys.exit("Error: --fix-report requires a reports/YYYY/MM/name.md path")
    safe_report_path(relative)
    fix_unquoted_question_colons([relative], None)
    say(f"Checked YAML open_questions in {relative}.")
    sys.exit(0)


prompt = f"""Read .claude/skills/find-reports/SKILL.md and execute it for {years}.
Find NEW, independently verified events; check existing reports to avoid duplicates.

REQUIRED NEW-REPORT MODEL ATTRIBUTION:
The controller invokes Muse Code with --model {muse_model} and --reasoning-effort {muse_effort}.
For EVERY NEW report, set `created_by_model: {provenance}` EXACTLY in YAML frontmatter.
Do not use the old shorthand `meta/muse-spark` or claim the effort is unknown.
Do not change `created_by_model` on existing reports that you update: preserve their original creator.
The controller checks and corrects attribution of newly added report files before validating.

PHOTO / VIDEO RESEARCH (BEST EFFORT; MEDIA IS OPTIONAL):
Actively look for relevant event photos and videos when researching each report,
including dedicated searches beyond the original news sources where useful.
Try local-language searches, official agency galleries and historical archives.
Only include media verified as depicting the actual event, not generic stock imagery
(gavel, handcuffs, bars, police sign or car, city/section banner, cartoon, drugs from
another story; see docs/WRITING.md §7 for the signs). If a caption would have to say a
picture is not known to show the event, leave the picture out.
Follow docs/WRITING.md §7 for media URLs, captions, publishers and licensing.
If suitable media cannot be found or verified, continue and publish the report
WITHOUT photos or videos. Never invent media, use unrelated pictures, or reject
an otherwise well-sourced event because no visual evidence is available.
A brief note in open_questions is fine when a photo was found but could not be
verified or its direct URL could not be recovered; do not make a missing photo
a blocker or turn every unsuccessful image search into an open question.

COMPLETE THE RESEARCH AND ARCHIVING, AND RUN LIGHTWEIGHT CHECKS AS NEEDED.
Do not run `npm run build` or `npx astro build` directly: those can race with
another Astro build and corrupt the shared dist/. If a full build is necessary,
ONLY run `bash scripts/research.sh --build` (quiet, holds the build lock).
The outer research.sh controller always performs final build validation once
your Muse run completes, so usually skip full builds yourself.
YAML SAFETY: open_questions must be a list of STRINGS. Quote any item containing
colon + space, such as `- "Disputed count: two publications disagree"`, or YAML
will parse the item as an object and Astro will reject the report.
Commit handling is done by the OUTER research.sh controller, which has permission
to write Git metadata. Do NOT attempt git add/commit or request approval; leave
report files ready for the controller to commit. Do not edit unrelated files.
Finish with the full list of new/updated reports and sources, discarded candidates,
validation results, and photo-search outcomes.
"""
(log_dir / "prompt.txt").write_text(prompt, encoding="utf-8")
say(f"Research: {years}\nRepository: {repo}\nLogs: {log_dir}")
say("Ctrl+C = finish current research + validation + commit, then stop.\n")

# Other agents may have uncommitted report files before or during a run.
# Those belong to them, not this controller. The per-run manifest controls ownership.
if pending_report_changes():
    say("ℹ Existing uncommitted report files detected. Leaving them alone; "
        "only Muse-owned files declared in each run manifest may be committed.")


index = 0
had_error = False
try:
    while not stop_requested:
        index += 1
        run_dir = log_dir / f"run-{index:03d}"
        run_dir.mkdir()
        with (run_dir / "output.log").open("w", encoding="utf-8", buffering=1) as out:
            say(f"\n========== RUN {index} | {years} ==========", out)
            before = git("rev-parse", "HEAD").stdout.strip()
            branch_before = git("symbolic-ref", "--quiet", "--short", "HEAD", check=False).stdout.strip()
            tracked_before = set(filter(None, git("ls-files", "-z", "--", *scopes).stdout.split("\0")))
            manifest = repo / ".cache" / "research-manifests" / f"{stamp}-run-{index:03d}.json"
            manifest.parent.mkdir(parents=True, exist_ok=True)
            run_prompt = prompt + f"""
MANDATORY PER-RUN FILE MANIFEST (CRITICAL WITH CONCURRENT WORKERS):
Before your final response, write exactly this JSON file in the workspace:
  {manifest.relative_to(repo)}
with this structure: {{"files": ["reports/YYYY/MM/report.md", ...]}}.
List EVERY report file and report image that YOU created or modified in this run.
Include ONLY your own files, never a different worker's files or unchanged ones.
Other workers may have uncommitted report files already in the repository.
Do not edit, stage, commit or claim their files; their presence is normal and NOT a reason to stop.
Even if you created zero reports, write {{"files": []}}.
The controller reads this file to stage and commit only your changes.
This file is ignored by Git. Do not commit it yourself.
"""
            (run_dir / "prompt.txt").write_text(run_prompt, encoding="utf-8")
            say(f"→ Muse model: {muse_model} | effort: {muse_effort} | attribution: {provenance}", out)
            cmd = ["muse", "exec", "--json", "--disable-approval",
                   "--model", muse_model, "--reasoning-effort", muse_effort,
                   "--max-model-steps", max_steps, run_prompt]
            code = run_command(cmd, out, run_dir / "muse.jsonl", muse_json=True)
            say(f"Muse exit code: {code}", out)
            if code != 0:
                say("ERROR: Muse did not finish successfully. Leaving files untouched and stopping.", out)
                had_error = True
                break
            after = git("rev-parse", "HEAD").stdout.strip()
            branch_after = git("symbolic-ref", "--quiet", "--short", "HEAD", check=False).stdout.strip()
            if branch_before != branch_after:
                say(f"ERROR: Git branch switched from {branch_before or '(detached)'} to {branch_after or '(detached)'}. Stopping to avoid committing on the wrong branch.", out)
                had_error = True
                break
            if after != before:
                say(f"ℹ HEAD advanced {before[:10]} → {after[:10]} during research (concurrent commits are OK).", out)
            changes = pending_report_changes()
            if not changes:
                say("No new/changed report files. Stopping to avoid repeating an empty run.", out)
                break
            say("Changed report files:\n" + changes, out)
            owned, external = parse_run_manifest(manifest, out)
            if not owned:
                say("No uncommitted reports owned by this run. Stopping without claiming another worker's files.", out)
                break
            say("Muse-owned files to validate and commit:\n" + "\n".join("  " + p for p in owned), out)
            # Fix new-report provenance, then repair the common unquoted-colon issue.
            set_new_report_provenance(owned, tracked_before, out)
            fix_unquoted_question_colons(owned, out)
            if validate_cmd:
                foreign = sorted(set(changed_report_paths()) - set(owned))
                if foreign:
                    say(f"\nValidating in isolation: {len(foreign)} unowned uncommitted file(s) "
                        "present, so the live tree cannot validate this run", out)
                    first_log = run_dir / "build-isolated.log"
                    validation_code = isolated_build(owned, out, run_dir, "build-isolated")
                else:
                    say("\nValidating (quiet, build locked): " + validate_cmd, out)
                    first_log = run_dir / "build.log"
                    validation_code = quiet_build(shlex.split(validate_cmd), first_log, out)
                if validation_code:
                    validation_code = recover_failed_build(owned, tracked_before, out, run_dir,
                                                           validation_code, first_log,
                                                           isolated=bool(foreign))
                if validation_code:
                    say(f"ERROR: Validation exited {validation_code}; files left uncommitted. Stopping.", out)
                    had_error = True
                    break
            # Commit only Muse-declared report paths, never another worker's files.
            paths = [p for p in owned if p in set(changed_report_paths())]
            if not paths:
                say("ERROR: No owned report changes remain to commit.", out)
                had_error = True
                break
            git("add", "-A", "--", *paths)
            message = f"Reports: research {years} (run {index})"
            commit = git("commit", "--only", "-m", message, "--", *paths, check=False)
            if commit.returncode:
                say("ERROR: Could not commit; stopping.\n" + commit.stderr, out)
                had_error = True
                break
            sha = git("rev-parse", "HEAD").stdout.strip()
            say(f"\n✓ COMMITTED {sha}\n{commit.stdout.strip()}", out)
            (run_dir / "commit.txt").write_text(sha + "\n" + commit.stdout, encoding="utf-8")
            if pending_report_changes():
                say("ℹ Other workers still have uncommitted report changes. "
                    "They remain untouched; continuing with a fresh Muse run.", out)
            if stop_requested:
                say("✓ Graceful stop: run completed and committed. No next run.", out)
                break
except Exception as exc:
    had_error = True
    say(f"ERROR: {exc}\nLogs: {log_dir}")
finally:
    say("\nResearch stopped. Logs: " + str(log_dir))
if had_error:
    sys.exit(1)
PY
