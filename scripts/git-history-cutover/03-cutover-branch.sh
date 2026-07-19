#!/usr/bin/env bash
# Replays one branch's unique commits onto the rewritten history in a mirror
# produced by 02-rewrite.sh. Run once per worktree/branch that has commits
# not yet on the pre-rewrite main. Never touches the original working repo;
# all rebasing happens inside the mirror clone.
set -euo pipefail

MIRROR=""
BRANCH=""
ORIGINAL_REPO="$(git rev-parse --show-toplevel 2>/dev/null || true)"

for arg in "$@"; do
  case "$arg" in
    --mirror=*) MIRROR="${arg#--mirror=}" ;;
    --branch=*) BRANCH="${arg#--branch=}" ;;
    *) echo "unknown arg: $arg (expected --mirror=<path> --branch=<name>)" >&2; exit 1 ;;
  esac
done

if [ -z "$MIRROR" ] || [ -z "$BRANCH" ]; then
  echo "usage: $0 --mirror=<path from 02-rewrite.sh> --branch=<branch-name>" >&2
  exit 1
fi
if [ ! -d "$MIRROR" ]; then
  echo "mirror not found: $MIRROR (did you run 02-rewrite.sh?)" >&2
  exit 1
fi

echo "=== branch: $BRANCH ==="

cd "$MIRROR"

# Case 1: branch already exists in the mirror (it was pushed to origin before
# 02-rewrite.sh ran with --everything, so it was already migrated together
# with main -- nothing to replay).
if git show-ref --verify --quiet "refs/heads/$BRANCH"; then
  echo "Branch already present in the rewritten mirror (was pushed to origin"
  echo "before the migration, so it was rewritten in the same pass as main)."
  echo "Nothing to replay. It will go out with 04-push-and-notify.sh."
  exit 0
fi

# Case 2: local-only branch. Pull it into the mirror as a new ref, find its
# unique commits relative to the OLD main, locate the equivalent commit in
# the NEW main by matching author+date+message (git lfs migrate preserves
# these verbatim -- only trees for migrated paths change), then rebase.
if [ -z "$ORIGINAL_REPO" ]; then
  echo "Run this from inside the worktree for $BRANCH so its commits can be" >&2
  echo "fetched into the mirror." >&2
  exit 1
fi

echo "=== fetching $BRANCH from $ORIGINAL_REPO into the mirror ==="
git fetch "$ORIGINAL_REPO" "$BRANCH:refs/heads/$BRANCH-incoming"

OLD_MAIN_TIP=$(awk '$1=="refs/heads/main"{print $2}' pre-migration-refs.txt)
if [ -z "$OLD_MAIN_TIP" ]; then
  echo "pre-migration-refs.txt missing an entry for refs/heads/main -- was" >&2
  echo "02-rewrite.sh run in this mirror?" >&2
  exit 1
fi

MERGE_BASE=$(git merge-base "$OLD_MAIN_TIP" "$BRANCH-incoming" 2>/dev/null || echo "")
if [ -z "$MERGE_BASE" ]; then
  echo "Could not find a merge-base between $BRANCH and the pre-migration" >&2
  echo "main tip. This branch may predate the mirror's history entirely --" >&2
  echo "resolve manually." >&2
  exit 1
fi

echo "=== unique commits on $BRANCH relative to old main ==="
git log --oneline "$OLD_MAIN_TIP..$BRANCH-incoming"

echo
echo "=== locating the equivalent merge-base commit in the rewritten history ==="
MB_AUTHOR=$(git show -s --format='%an <%ae>' "$MERGE_BASE")
MB_DATE=$(git show -s --format='%ad' --date=iso-strict "$MERGE_BASE")
MB_SUBJECT=$(git show -s --format='%s' "$MERGE_BASE")

NEW_MAIN_TIP=$(awk '$1=="refs/heads/main"{print $2}' post-migration-refs.txt)
CANDIDATES=$(git log "$NEW_MAIN_TIP" --format='%H %ad %s' --date=iso-strict |
  awk -v d="$MB_DATE" -v s="$MB_SUBJECT" '$0 ~ d && index($0, s) {print $1}')

CANDIDATE_COUNT=$(echo "$CANDIDATES" | grep -c . || true)
if [ "$CANDIDATE_COUNT" -ne 1 ]; then
  echo "Found $CANDIDATE_COUNT candidate(s) for the rewritten merge-base" >&2
  echo "(expected exactly 1) matching:" >&2
  echo "  author: $MB_AUTHOR" >&2
  echo "  date:   $MB_DATE" >&2
  echo "  subject: $MB_SUBJECT" >&2
  echo "Resolve manually: find the equivalent commit in '$NEW_MAIN_TIP' history" >&2
  echo "yourself, then run:" >&2
  echo "  git rebase --onto <new-equivalent-sha> $MERGE_BASE $BRANCH-incoming" >&2
  exit 1
fi
NEW_BASE="$CANDIDATES"
echo "Matched: $NEW_BASE"

echo
echo "=== rebasing $BRANCH onto the rewritten history ==="
echo "git rebase --onto $NEW_BASE $MERGE_BASE $BRANCH-incoming"
if git rebase --onto "$NEW_BASE" "$MERGE_BASE" "$BRANCH-incoming"; then
  git branch -f "$BRANCH" "$BRANCH-incoming"
  git branch -d "$BRANCH-incoming" 2>/dev/null || true
  echo "Done. '$BRANCH' now sits on the rewritten history in this mirror."
  echo "It will go out with the next 04-push-and-notify.sh run."
else
  echo
  echo "Rebase hit conflicts (likely a commit that touches a migrated path --"
  echo "docs/demos/*.gif or benchmarks/**/*.{txt,json} -- since those blobs"
  echo "are now LFS pointers instead of raw content). Resolve manually inside"
  echo "$MIRROR, then:"
  echo "  git add -A && git rebase --continue"
  echo "  git branch -f $BRANCH $BRANCH-incoming"
  exit 1
fi
