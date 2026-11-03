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


prompt = f"""Read .claude/skills/find-reports/SKILL.md and execute it for {years}.
Find NEW, independently verified events; check existing reports to avoid duplicates.

PHOTO / VIDEO RESEARCH (BEST EFFORT; MEDIA IS OPTIONAL):
Actively look for relevant event photos and videos when researching each report,
including dedicated searches beyond the original news sources where useful.
Try local-language searches, official agency galleries and historical archives.
Only include media verified as depicting the actual event, not generic stock imagery.
Follow docs/WRITING.md §7 for media URLs, captions, publishers and licensing.
If suitable media cannot be found or verified, continue and publish the report
WITHOUT photos or videos. Never invent media, use unrelated pictures, or reject
an otherwise well-sourced event because no visual evidence is available.
A brief note in open_questions is fine when a photo was found but could not be
verified or its direct URL could not be recovered; do not make a missing photo
a blocker or turn every unsuccessful image search into an open question.

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

validate_cmd = os.environ.get("RESEARCH_VALIDATE_CMD", "npx astro build").strip()
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
            branch_before = git("symbolic-ref", "--quiet", "--short", "HEAD", check=False).stdout.strip()
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
