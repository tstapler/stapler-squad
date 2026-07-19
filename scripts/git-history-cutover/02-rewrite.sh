#!/usr/bin/env bash
# Rewrites history in a DISPOSABLE MIRROR CLONE, never this working repo.
# Without --i-understand-this-rewrites-history, only prints what would run.
set -euo pipefail
cd "$(git rev-parse --show-toplevel)"

CONFIRM=""
MIRROR_DIR="/tmp/stapler-squad-cutover-$(date +%s 2>/dev/null || echo manual)"
ORIGIN_URL=$(git remote get-url origin)

for arg in "$@"; do
  case "$arg" in
    --i-understand-this-rewrites-history) CONFIRM="yes" ;;
    --mirror-dir=*) MIRROR_DIR="${arg#--mirror-dir=}" ;;
    *) echo "unknown arg: $arg" >&2; exit 1 ;;
  esac
done

echo "This will:"
echo "  1. git clone --mirror $ORIGIN_URL $MIRROR_DIR"
echo "  2. cd $MIRROR_DIR"
echo "  3. git lfs migrate import --everything --yes \\"
echo "       --include=\"docs/demos/*.gif,benchmarks/**/*.txt,benchmarks/**/*.json\""
echo "  4. Write an old-sha -> new-sha commit map to $MIRROR_DIR/commit-map.txt"
echo "  5. Report new size. NOTHING is pushed by this script."
echo

if [ -z "$CONFIRM" ]; then
  echo "Dry run only. Re-run with --i-understand-this-rewrites-history to execute."
  exit 0
fi

echo "=== cloning mirror ==="
git clone --mirror "$ORIGIN_URL" "$MIRROR_DIR"
cd "$MIRROR_DIR"

echo "=== recording pre-migration ref -> sha map (for step 3's merge-base lookups) ==="
git for-each-ref --format='%(refname) %(objectname)' > pre-migration-refs.txt

echo "=== running git lfs migrate import (rewrites every ref) ==="
git lfs migrate import --everything --yes \
  --include="docs/demos/*.gif,benchmarks/**/*.txt,benchmarks/**/*.json"

echo "=== recording post-migration ref -> sha map ==="
git for-each-ref --format='%(refname) %(objectname)' > post-migration-refs.txt

# git lfs migrate doesn't emit an explicit old->new commit map file, but since
# it rewrites refs in place and preserves ref names, the pre/post ref-sha
# pairs above already tell us the new tip for every branch/tag that existed.
# For per-commit lookups mid-branch (needed when a worktree's merge-base with
# main isn't a ref tip), 03-cutover-branch.sh falls back to matching commit
# messages + author + committer date across old and new history, which
# git lfs migrate preserves verbatim.

echo
echo "=== done ==="
du -sh .
echo "Mirror left at: $MIRROR_DIR"
echo "Next: run 03-cutover-branch.sh for each worktree with unpushed commits,"
echo "then 04-push-and-notify.sh when all branches are replayed."
