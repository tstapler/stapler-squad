#!/usr/bin/env bash
# Read-only: reports current repo bloat. Safe to run anytime, changes nothing.
# No pipefail: several pipelines below intentionally truncate with `head`,
# which sends SIGPIPE upstream -- pipefail would treat that as a failure.
set -eu
cd "$(git rev-parse --show-toplevel)"

echo "=== .git size ==="
du -sh .git
git count-objects -vH | grep -v "^garbage"

echo
echo "=== largest blobs in history (top 20) ==="
git rev-list --objects --all |
  git cat-file --batch-check='%(objecttype) %(objectname) %(objectsize) %(rest)' |
  awk '$1=="blob"{print $3, $2, $4}' | sort -rn | head -20 |
  awk '{printf "%8.1f MB  %s\n", $1/1024/1024, $3}'

echo
echo "=== cumulative size by top-level path (rough) ==="
git rev-list --objects --all |
  git cat-file --batch-check='%(objecttype) %(objectname) %(objectsize) %(rest)' |
  awk '$1=="blob"{print $3, $4}' |
  awk '{split($2,a,"/"); print a[1], $1}' |
  awk '{sum[$1]+=$2} END {for (k in sum) print sum[k]/1024/1024" MB", k}' |
  sort -rn | head -15

echo
echo "=== gif count + total size in history ==="
GIF_TOTAL=$(git rev-list --objects --all | grep -i '\.gif$' | awk '{print $1}' |
  git cat-file --batch-check='%(objectsize)' | awk '{s+=$1} END {print s+0}')
echo "count: $(git rev-list --objects --all | grep -ci '\.gif$')"
echo "total: $((GIF_TOTAL / 1024 / 1024)) MB"

echo
echo "=== currently LFS-tracked patterns (.gitattributes) ==="
cat .gitattributes 2>/dev/null || echo "(none)"

echo
echo "=== active worktrees with commits not on main (candidates for step 3) ==="
git worktree list --porcelain | awk '/^worktree /{wt=$2} /^branch /{print wt, $2}' |
while read -r wt branch; do
  b="${branch#refs/heads/}"
  [ "$b" = "main" ] && continue
  ahead=$(git rev-list --count "main..$b" 2>/dev/null || echo "?")
  if [ "$ahead" != "0" ] && [ "$ahead" != "?" ]; then
    echo "$b: $ahead commit(s) ahead of main  ($wt)"
  fi
done
