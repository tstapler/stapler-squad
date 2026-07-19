#!/usr/bin/env bash
# Force-pushes the rewritten history from the mirror to both remotes.
# Without --i-understand-this-force-pushes, only prints what would run.
set -euo pipefail

MIRROR=""
CONFIRM=""
for arg in "$@"; do
  case "$arg" in
    --mirror=*) MIRROR="${arg#--mirror=}" ;;
    --i-understand-this-force-pushes) CONFIRM="yes" ;;
    *) echo "unknown arg: $arg" >&2; exit 1 ;;
  esac
done
if [ -z "$MIRROR" ]; then
  echo "usage: $0 --mirror=<path from 02-rewrite.sh> --i-understand-this-force-pushes" >&2
  exit 1
fi

cd "$MIRROR"

echo "This will force-push ALL rewritten refs from $MIRROR to:"
echo "  origin   (as configured when 02-rewrite.sh cloned it)"
echo "  personal (added below if not already a remote in the mirror)"
echo
echo "Before running this:"
echo "  - Confirm every branch with unique commits has been replayed via"
echo "    03-cutover-branch.sh (check scripts/git-history-cutover/01-analyze.sh's"
echo "    'active worktrees with commits not on main' section against what's"
echo "    now in this mirror: 'git branch' here should include all of them)."
echo "  - Double check nobody is mid-push to the old main right now."
echo

if [ -z "$CONFIRM" ]; then
  echo "Dry run only. Would run:"
  echo "  git push --mirror --force origin"
  echo "  git remote get-url personal >/dev/null 2>&1 || git remote add personal <url>"
  echo "  git push --mirror --force personal"
  echo "Re-run with --i-understand-this-force-pushes to execute."
  exit 0
fi

echo "=== force-pushing to origin ==="
git push --mirror --force origin

if git remote get-url personal >/dev/null 2>&1; then
  echo "=== force-pushing to personal ==="
  git push --mirror --force personal
else
  echo "No 'personal' remote configured in this mirror -- add it and push"
  echo "manually if the fork also needs the rewritten history:"
  echo "  git remote add personal <personal-fork-url>"
  echo "  git push --mirror --force personal"
fi

cat <<'MSG'

=== Cutover complete. Message for anyone with an existing local clone/worktree: ===

The stapler-squad history was rewritten to move docs/demos/*.gif and
benchmarks/**/*.{txt,json} into Git LFS, reclaiming ~15GB. Every commit hash
changed. To pick up the new history:

  If you have NO unpushed local commits on top of main:
    git fetch origin
    git reset --hard origin/main
    # then re-run for any other local branch you still care about:
    git fetch origin
    git branch -f <branch> origin/<branch>   # if it was pushed and replayed centrally

  If you DO have unpushed local commits (a worktree ahead of main):
    Someone should have run scripts/git-history-cutover/03-cutover-branch.sh
    for your branch before this push. Ask if you're not sure, or run it
    yourself against this mirror before your branch's old base disappears
    from the remote.

Make sure `git-lfs` is installed locally (`brew install git-lfs`, then
`git lfs install`) before pulling -- without it, docs/demos/*.gif and
benchmarks/**/*.{txt,json} will check out as pointer text instead of real
content.
MSG
