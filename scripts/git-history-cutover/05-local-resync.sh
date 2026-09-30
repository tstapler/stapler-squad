#!/usr/bin/env bash
# Run this on any machine/worktree/workspace AFTER 04-push-and-notify.sh has
# force-pushed rewritten history. Backs up your current state (so nothing is
# ever silently discarded), then re-syncs this checkout onto the new history.
#
# Safe by default: with no confirmation flag, only reports what it found and
# what it *would* do. Nothing destructive happens until you pass
# --i-understand-this-resets-local-history.
#
# This script is for a NORMAL clone/checkout of main (this repo's own root
# checkout, another machine's clone, etc.) -- not for one of this repo's 90+
# active git worktrees. A worktree with real in-progress commits should be
# replayed with 03-cutover-branch.sh instead (ask whoever ran the cutover, or
# run it yourself against their mirror before the old base disappears from
# the remote). A worktree with no unique commits can simply be deleted and
# recreated fresh -- there's nothing here worth resyncing.
set -euo pipefail
cd "$(git rev-parse --show-toplevel)"

CONFIRM=""
REMOTE=""
for arg in "$@"; do
  case "$arg" in
    --i-understand-this-resets-local-history) CONFIRM="yes" ;;
    --remote=*) REMOTE="${arg#--remote=}" ;;
    *) echo "unknown arg: $arg (expected --remote=<name> --i-understand-this-resets-local-history)" >&2; exit 1 ;;
  esac
done

# Auto-detect the canonical remote if not given explicitly. Remote naming has
# drifted across machines for this repo (some call the fork "origin" and the
# work repo "upstream-fanatics", others use "personal" -- see CLAUDE.md's Git
# Remotes section, or ask if unsure which one this machine treats as
# canonical). Prefer origin, then the two names this project's other tooling
# already knows about.
if [ -z "$REMOTE" ]; then
  for candidate in origin upstream-fanatics personal upstream; do
    if git remote get-url "$candidate" >/dev/null 2>&1; then
      REMOTE="$candidate"
      break
    fi
  done
fi
if [ -z "$REMOTE" ] || ! git remote get-url "$REMOTE" >/dev/null 2>&1; then
  echo "Could not find a usable remote (tried origin, upstream-fanatics, personal, upstream)." >&2
  echo "Pass one explicitly: $0 --remote=<name>" >&2
  exit 1
fi

echo "=== using remote: $REMOTE ($(git remote get-url "$REMOTE")) ==="

if ! command -v git-lfs >/dev/null 2>&1; then
  echo
  echo "WARNING: git-lfs is not installed on this machine. docs/demos/*.gif and"
  echo "benchmarks/**/*.{txt,json} will check out as pointer text instead of real"
  echo "content until you install it:"
  echo "  brew install git-lfs   # macOS"
  echo "  # or your distro's package manager, e.g. pacman -S git-lfs / apt install git-lfs"
  echo "  git lfs install"
  echo
fi

BACKUP_TAG="pre-cutover-resync-$(date +%Y%m%d-%H%M%S 2>/dev/null || echo manual)"
CURRENT_BRANCH=$(git rev-parse --abbrev-ref HEAD)
DIRTY=$(git status --porcelain)

echo "=== current state ==="
echo "branch:            $CURRENT_BRANCH"
echo "HEAD:              $(git rev-parse --short HEAD)"
echo "uncommitted changes: $([ -n "$DIRTY" ] && echo "YES -- $(echo "$DIRTY" | wc -l | tr -d ' ') file(s)" || echo "none")"

echo
echo "This will:"
echo "  1. Tag current HEAD as '$BACKUP_TAG' (and stash uncommitted changes under"
echo "     the same name, if any) -- your escape hatch if anything below needs"
echo "     to be recovered afterward."
echo "  2. git fetch $REMOTE"
echo "  3. If $CURRENT_BRANCH has commits the rewritten $REMOTE/$CURRENT_BRANCH doesn't"
echo "     recognize as an ancestor (expected after a history rewrite), hard-reset"
echo "     $CURRENT_BRANCH to $REMOTE/$CURRENT_BRANCH."
echo "     If your branch's commits are already reachable from the new history"
echo "     (nothing was rewritten out from under you), this is a no-op fast-forward."
echo
echo "It will NOT touch any other local branch -- re-run with a different branch"
echo "checked out for each one you care about, or use"
echo "  git branch -f <branch> $REMOTE/<branch>"
echo "directly for branches with no local-only commits."
echo

if [ -z "$CONFIRM" ]; then
  echo "Dry run only. Re-run with --i-understand-this-resets-local-history to execute."
  exit 0
fi

echo "=== backing up current state as '$BACKUP_TAG' ==="
git tag "$BACKUP_TAG" HEAD
if [ -n "$DIRTY" ]; then
  # Explicit pathspec, never a blanket `git stash -u` -- built from
  # NUL-delimited `git status` output so filenames with spaces/special
  # characters survive intact. A rename/copy entry ("Rxx"/"Cxx") is followed
  # by a second NUL-terminated field (the new path) with no status prefix of
  # its own -- consume it as a plain path, not a fresh status line.
  dirty_paths=()
  while IFS= read -r -d '' entry; do
    status="${entry:0:2}"
    dirty_paths+=("${entry:3}")
    case "$status" in
      R*|C*)
        IFS= read -r -d '' new_path
        dirty_paths+=("$new_path")
        ;;
    esac
  done < <(git status --porcelain -z)
  git stash push -u -m "$BACKUP_TAG" -- "${dirty_paths[@]}"
  echo "Uncommitted changes stashed (see: git stash list)."
fi
echo "Backup tag created: $BACKUP_TAG"
echo "Recover with: git checkout $BACKUP_TAG -- <path>   (or) git stash pop"

echo
echo "=== fetching $REMOTE ==="
git fetch "$REMOTE"

REMOTE_TIP=$(git rev-parse "$REMOTE/$CURRENT_BRANCH" 2>/dev/null || echo "")
if [ -z "$REMOTE_TIP" ]; then
  echo "No $REMOTE/$CURRENT_BRANCH found -- nothing to sync $CURRENT_BRANCH against." >&2
  echo "Your backup tag ($BACKUP_TAG) is still there if you need it." >&2
  exit 1
fi

if git merge-base --is-ancestor HEAD "$REMOTE_TIP" 2>/dev/null; then
  echo "=== fast-forwarding (history was not rewritten out from under this branch) ==="
  git merge --ff-only "$REMOTE_TIP"
else
  echo "=== resetting $CURRENT_BRANCH to $REMOTE/$CURRENT_BRANCH (history was rewritten) ==="
  git reset --hard "$REMOTE_TIP"
fi

echo
echo "=== done ==="
echo "$CURRENT_BRANCH is now at $(git rev-parse --short HEAD), matching $REMOTE/$CURRENT_BRANCH."
echo "Backup of your pre-resync state: git tag $BACKUP_TAG (and 'git stash list' if you had uncommitted changes)."
echo "Safe to delete once you've confirmed nothing was lost: git tag -d $BACKUP_TAG"
