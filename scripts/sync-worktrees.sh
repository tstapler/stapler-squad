#!/usr/bin/env bash
# Merges main into every git worktree of this repo, skipping any worktree
# with uncommitted changes and aborting cleanly on conflict instead of
# forcing a resolution. Safe to re-run; a no-op for worktrees already
# up to date.
#
# Intended use: run before assigning new work to an idle backlog worktree,
# or periodically against idle worktrees, so fixes on main (e.g. test
# hygiene, lint rules) reach every worktree instead of only the ones a
# human happens to touch. Never run this against a worktree an agent is
# actively working in — a merge landing mid-session can surprise a running
# test/build.
set -euo pipefail

cd "$(git rev-parse --show-toplevel)"
main_tip=$(git rev-parse main)

merged=0 uptodate=0 skipped_dirty=0 conflicts=0
conflict_paths=()

while IFS= read -r wt; do
  [ -d "$wt" ] || continue
  [ "$wt" = "$(pwd)" ] && continue
  branch=$(git -C "$wt" rev-parse --abbrev-ref HEAD 2>/dev/null || echo "(detached)")

  if [ -n "$(git -C "$wt" status --porcelain 2>/dev/null)" ]; then
    echo "skip (dirty):     $wt [$branch]"
    skipped_dirty=$((skipped_dirty + 1))
    continue
  fi

  if git -C "$wt" merge-base --is-ancestor "$main_tip" HEAD 2>/dev/null; then
    echo "up to date:       $wt [$branch]"
    uptodate=$((uptodate + 1))
    continue
  fi

  if git -C "$wt" merge main --no-edit >/dev/null 2>&1; then
    echo "merged:           $wt [$branch]"
    merged=$((merged + 1))
  else
    git -C "$wt" merge --abort 2>/dev/null || true
    echo "CONFLICT (skip):  $wt [$branch]"
    conflicts=$((conflicts + 1))
    conflict_paths+=("$wt")
  fi
done < <(git worktree list --porcelain | awk '/^worktree /{print $2}')

echo
echo "merged=$merged up-to-date=$uptodate skipped-dirty=$skipped_dirty conflicts=$conflicts"
if [ "${#conflict_paths[@]}" -gt 0 ]; then
  echo
  echo "Needs manual resolution (each has real, unrelated divergence from main):"
  printf '  %s\n' "${conflict_paths[@]}"
fi
