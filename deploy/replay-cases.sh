#!/usr/bin/env bash
# Puts back report commits whose case the server clone's branch lost: every commit of `from`
# (default origin/submissions) that is not on main and names a Submission-Case the current branch
# does not have is replayed on top of it, oldest first, the commit's own report winning a conflict.
# For a clone realigned by hand while GitHub's copy of the branch held reports the clone did not.
#   cd /corn/cannabis-freedom-commons/cet && bash deploy/replay-cases.sh [origin/submissions]
# Stop the backend first (docker compose stop tracker); it keeps the branch as it was as
# `submissions-before-replay` and does not push: the backend force-pushes it on its next commit.
set -euo pipefail
from="${1:-origin/submissions}"

git rebase --abort 2>/dev/null || true
git cherry-pick --abort 2>/dev/null || true
git fetch --quiet origin
git branch -f submissions-before-replay HEAD
echo "The branch as it was is kept as submissions-before-replay ($(git rev-parse --short HEAD))."

trailers() { git log -n 5000 --format=%B "$1" | grep -o 'Submission-Case: [0-9a-f]\{32\}' | sort -u || true; }
here=$(trailers HEAD)
missing=()
for c in $(git rev-list --reverse --no-merges "origin/main..$from"); do
  cases=$(git log -1 --format=%B "$c" | grep -o 'Submission-Case: [0-9a-f]\{32\}' | sort -u || true)
  [ -z "$cases" ] && continue
  if [ -n "$(comm -23 <(echo "$cases") <(echo "$here"))" ]; then missing+=("$c"); fi
done
echo "${#missing[@]} commit(s) of $from carry cases this branch does not have."

export GIT_COMMITTER_NAME="${GIT_COMMITTER_NAME:-$(git log -1 --format=%cn "$from")}"
export GIT_COMMITTER_EMAIL="${GIT_COMMITTER_EMAIL:-$(git log -1 --format=%ce "$from")}"
left_out=0
for c in ${missing[@]+"${missing[@]}"}; do
  if ! git cherry-pick --empty=drop -X theirs "$c" >/dev/null 2>&1; then
    git cherry-pick --abort 2>/dev/null || true
    echo "Left out (could not apply): $(git log -1 --format='%h %s' "$c")"
    left_out=$((left_out + 1))
  fi
done

still=$(for c in $(git rev-list --no-merges "origin/main..$from"); do git log -1 --format=%B "$c" | grep -o 'Submission-Case: [0-9a-f]\{32\}' || true; done | sort -u | comm -23 - <(trailers HEAD) | wc -l | tr -d ' ')
echo "Replayed $(( ${#missing[@]} - left_out )) commit(s), ${left_out} left out; cases of $from still missing here: ${still}."
echo "Next: docker compose start tracker"
