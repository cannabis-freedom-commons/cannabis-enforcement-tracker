#!/usr/bin/env bash
# Puts the server clone's `submissions` branch back on top of `main` once, by hand, the way the
# backend now does it itself (alignWithMain in backend/internal/worker/batch.go): for a clone an
# older backend left mid-rebase, before rebuilding with the new code.
#   cd /corn/cannabis-freedom-commons/cet && bash deploy/realign-submissions.sh
# It ends any unfinished rebase, merge or cherry-pick, keeps the old branch as `submissions-backup`,
# resets `submissions` to origin/main and replays only the report commits whose Submission-Case
# main does not have yet, one at a time (the branch's newer report wins a conflict; a commit that
# still cannot apply is listed and left out). It does not push: the backend does that on its next
# batch. The backend must be stopped while it runs.
set -euo pipefail

git rebase --abort 2>/dev/null || true
git merge --abort 2>/dev/null || true
git cherry-pick --abort 2>/dev/null || true
git fetch --quiet origin

branch=$(git rev-parse --abbrev-ref HEAD)
if [ "$branch" != "submissions" ]; then
  git checkout --quiet submissions
fi
git branch -f submissions-backup HEAD
echo "The branch as it was is kept as submissions-backup ($(git rev-parse --short HEAD))."

published=$(git log -n 5000 --format=%B origin/main | grep -o 'Submission-Case: [0-9a-f]\{32\}' | sort -u || true)
keep=()
for c in $(git rev-list --reverse --no-merges origin/main..HEAD); do
  cases=$(git log -1 --format=%B "$c" | grep -o 'Submission-Case: [0-9a-f]\{32\}' || true)
  if [ -z "$cases" ] || [ -n "$(comm -23 <(echo "$cases" | sort -u) <(echo "$published"))" ]; then
    keep+=("$c")
  fi
done
echo "${#keep[@]} report commit(s) are not on main yet."

# the commits are replayed as the backend's own
export GIT_COMMITTER_NAME="${GIT_COMMITTER_NAME:-$(git log -1 --format=%cn origin/main)}"
export GIT_COMMITTER_EMAIL="${GIT_COMMITTER_EMAIL:-$(git log -1 --format=%ce origin/main)}"
git reset --quiet --hard origin/main
left_out=0
for c in ${keep[@]+"${keep[@]}"}; do
  if ! git cherry-pick --empty=drop -X theirs "$c" >/dev/null 2>&1; then
    git cherry-pick --abort 2>/dev/null || true
    echo "Left out (could not apply): $(git log -1 --format='%h %s' "$c")"
    left_out=$((left_out + 1))
  fi
done
echo "submissions is now main plus $(git rev-list --count origin/main..HEAD) commit(s); ${left_out} left out."
echo "Next: docker compose up -d --build"
