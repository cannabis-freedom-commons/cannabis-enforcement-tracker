#!/usr/bin/env bash
# Bash entrypoint; Python manages process isolation, streaming logs and graceful Ctrl+C.
exec python3 - "$@" <<'PY'
import datetime as dt
import json
import os
from pathlib import Path
import re
import shlex
import shutil
import signal
import subprocess
import sys
import threading

if len(sys.argv) != 2 or not re.fullmatch(r"\d{4}-\d{4}", sys.argv[1]):
    sys.exit("Usage: ./research.sh YYYY-YYYY")
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
log_dir = log_base / years / stamp
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
    final_parts = []
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
            if kind in ("task.lifecycle.side_effect_intent", "task.lifecycle.proposed"):
                operation = (payload.get("event") or {}).get("operation") or (payload.get("event") or {}).get("task_kind") or ""
                if operation.startswith("tool:") or operation.startswith("tool."):
                    say("→ " + operation.split(":", 1)[-1].removeprefix("tool."), log)
            elif kind == "tool.result":
                cf = payload.get("correlation_facts") or {}
                tool = cf.get("tool_name", "tool")
                result = cf.get("outcome", "done")
                say(f"✓ {tool}: {result}", log)
            elif kind == "run.output.delta":
                fragment = payload.get("text", "")
                if fragment:
                    delta_seen = True
                    final_parts.append(fragment)
                    say(fragment, log, end="")
            elif kind == "run.terminal.completed":
                final_text = payload.get("text", "")
                if not delta_seen and final_text:
                    say("\n" + final_text, log)
                elif delta_seen:
                    say("", log)  # newline after streamed final answer
            elif kind.startswith("run.terminal."):
                say("! Muse: " + kind + " " + str(payload.get("reason", "")), log)
    returncode = proc.wait()
    thread.join(timeout=5)
    return returncode


prompt = f"""Read .claude/skills/find-reports/SKILL.md and execute it for {years}.
Find NEW, independently verified events; check existing reports to avoid duplicates.

PHOTO / VIDEO RESEARCH IS REQUIRED FOR EACH EVENT:
Perform dedicated image and video searches IN ADDITION TO reading the news sources.
Search in local languages, government agency galleries, photo archives, and historical news.
Inspect every candidate and verify it depicts the specific event, not a generic stock photo.
Follow docs/WRITING.md §7: record each verified photo/video with its original page URL,
actual direct media_url, caption and publisher. Respect licensing requirements for covers.
Don't mistake 'no photo on this article' for 'no event photos exist'. If none is found,
record what sources were checked and the limitation in open_questions.

COMPLETE THE RESEARCH, ARCHIVING AND VALIDATION. Commit handling is done by the
OUTER research.sh controller, which has permission to write Git metadata.
Do NOT attempt git add/commit or request approval; leave report files ready for the
controller to commit. Do not edit unrelated files.
Finish with the full list of new/updated reports and sources, discarded candidates,
validation results, and photo-search outcomes.
"""
(log_dir / "prompt.txt").write_text(prompt, encoding="utf-8")
say(f"Research: {years}\nRepository: {repo}\nLogs: {log_dir}")
say("Ctrl+C = finish current research + validation + commit, then stop.\n")

if pending_report_changes():
    sys.exit("ERROR: Existing uncommitted report/image changes found.\n"
             "Commit or stash those first so the script cannot accidentally commit unrelated work.\n"
             f"Inspect with: git status --short -- {' '.join(scopes)}\nLogs: {log_dir}")

validate_cmd = os.environ.get("RESEARCH_VALIDATE_CMD", "npm run build").strip()
max_steps = os.environ.get("RESEARCH_MAX_STEPS", "120")
if not max_steps.isdecimal() or int(max_steps) < 1:
    sys.exit("RESEARCH_MAX_STEPS must be a positive integer")

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
            cmd = ["muse", "exec", "--json", "--disable-approval", "--max-model-steps", max_steps]
            if os.environ.get("RESEARCH_MODEL"):
                cmd += ["--model", os.environ["RESEARCH_MODEL"]]
            cmd.append(prompt)
            code = run_command(cmd, out, run_dir / "muse.jsonl", muse_json=True)
            say(f"Muse exit code: {code}", out)
            if code != 0:
                say("ERROR: Muse did not finish successfully. Leaving files untouched and stopping.", out)
                had_error = True
                break
            if git("rev-parse", "HEAD").stdout.strip() != before:
                say("ERROR: HEAD moved during the run. Review unexpected commits; stopping.", out)
                had_error = True
                break
            changes = pending_report_changes()
            if not changes:
                say("No new/changed report files. Stopping to avoid repeating an empty run.", out)
                break
            say("Changed report files:\n" + changes, out)
            if validate_cmd:
                say("\nValidating: " + validate_cmd, out)
                validation_code = run_command(shlex.split(validate_cmd), out)
                if validation_code:
                    say(f"ERROR: Validation exited {validation_code}; files left uncommitted. Stopping.", out)
                    had_error = True
                    break
            # Commit only actual report paths, never other already-staged changes.
            paths = changed_report_paths()
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
            if stop_requested:
                say("✓ Graceful stop: run completed and committed. No next run.", out)
                break
except Exception as exc:
    sys.exit(f"ERROR: {exc}\nLogs: {log_dir}")
finally:
    say("\nResearch stopped. Logs: " + str(log_dir))
if had_error:
    sys.exit(1)
PY
