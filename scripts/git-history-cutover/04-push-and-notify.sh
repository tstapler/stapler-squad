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

# Remote naming has drifted across machines for this repo -- some treat the
# fork as "origin" and the work repo as "upstream-fanatics", others use
# "personal"/"upstream". Push to every canonical name that's actually
# configured rather than assuming one specific pair.
REMOTES=()
for candidate in origin upstream-fanatics personal upstream; do
  if git remote get-url "$candidate" >/dev/null 2>&1; then
    REMOTES+=("$candidate")
  fi
done

echo "This will force-push ALL rewritten refs from $MIRROR to: ${REMOTES[*]}"
echo
echo "Before running this:"
echo "  - Confirm every branch with unique commits has been replayed via"
echo "    03-cutover-branch.sh (check scripts/git-history-cutover/01-analyze.sh's"
echo "    'active worktrees with commits not on main' section against what's"
echo "    now in this mirror: 'git branch' here should include all of them)."
echo "  - Double check nobody is mid-push to the old main right now."
echo

if [ -z "$CONFIRM" ]; then
  echo "Dry run only. Would run, for each of ${REMOTES[*]}:"
  echo "  git push --mirror --force <remote>"
  echo "Re-run with --i-understand-this-force-pushes to execute."
  exit 0
fi

for remote in "${REMOTES[@]}"; do
  echo "=== force-pushing to $remote ==="
  git push --mirror --force "$remote"
done

cat <<MSG

=== Cutover complete. Message for anyone with an existing local clone/worktree: ===

The stapler-squad history was rewritten to move docs/demos/*.gif and
benchmarks/**/*.{txt,json} into Git LFS, reclaiming ~15GB. Every commit hash
changed. To pick up the new history, run from the repo root:

  scripts/git-history-cutover/05-local-resync.sh --i-understand-this-resets-local-history

It backs up your current state first (a tag plus a stash of any uncommitted
changes -- nothing is discarded silently), then fetches and resyncs. See that
script's own output for recovery instructions if anything looks wrong
afterward.

If you're in one of this machine's 90+ active worktrees instead of a normal
clone: delete and recreate it if it has no commits unique to it, or use
03-cutover-branch.sh against this mirror if it does, before the old base
disappears from the remote.

Make sure \`git-lfs\` is installed locally (\`brew install git-lfs\`, then
\`git lfs install\`) before pulling -- without it, docs/demos/*.gif and
benchmarks/**/*.{txt,json} will check out as pointer text instead of real
content.
MSG
