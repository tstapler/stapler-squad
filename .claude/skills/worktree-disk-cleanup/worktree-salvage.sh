#!/usr/bin/env bash
# Usage: worktree-salvage.sh <worktree-path> [base-ref=main]
# Snapshots a worktree's uncommitted work (tracked edits + untracked files, minus build junk and
# files >5MB) into refs/salvage/<name>/<ts> in the shared object DB, and exports a reviewable
# tree to ~/.stapler-squad/salvage/<name>-<ts>/ :
#   meta.txt        path, branch, HEAD, ahead-of-base count, skipped files
#   uncommitted.patch   git apply-able diff of the uncommitted work (binary-safe)
#   commits.patch   git am-able patches of commits ahead of base (only if any)
#   stat.txt        diffstat of uncommitted work
# Prints the export dir on success. Nonzero exit = nothing verified; DO NOT remove the worktree.
set -euo pipefail
wt=$(readlink -f "${1:?worktree path}"); base=${2:-main}
junk='(^|/)(\.[a-z-]+\.stamp|bin/|gen/|coverage\.(html|out)|node_modules/|tsconfig\.tsbuildinfo|session/ent/|third_party/tmux|test-results/|docs/registry/)|^stapler-squad$|web-app/src/gen/'
max=$((5*1024*1024))
name=$(basename "$wt"); ts=$(date +%Y%m%d-%H%M%S)
out=~/.stapler-squad/salvage/$name-$ts; ref=refs/salvage/$name/$ts
cd "$wt"
head=$(git rev-parse HEAD); branch=$(git symbolic-ref -q --short HEAD || echo detached)
idx=$(mktemp -u); export GIT_INDEX_FILE=$idx; trap 'rm -f "$idx"' EXIT
git read-tree HEAD
skipped=()
while IFS= read -r -d '' f; do   # tracked changes (incl. deletions) + untracked, honoring .gitignore
  if printf %s "$f" | grep -Eq "$junk"; then skipped+=("junk: $f"); continue; fi
  if [ -f "$f" ] && [ "$(stat -c %s "$f")" -gt "$max" ]; then skipped+=("over 5MB: $f"); continue; fi
  git update-index --add --remove -- "$f"
done < <(git ls-files -m -o -d --exclude-standard -z)
tree=$(git write-tree)
[ "$tree" = "$(git rev-parse "HEAD^{tree}")" ] && { echo "nothing to salvage in $wt" >&2; exit 2; }
c=$(git commit-tree "$tree" -p HEAD -m "salvage $branch from $wt")
unset GIT_INDEX_FILE
git update-ref "$ref" "$c"
# Verify: ref resolves and its tree matches what we wrote.
[ "$(git rev-parse "${ref}^{tree}")" = "$tree" ] || { echo "verify failed" >&2; exit 3; }
mkdir -p "$out"
git diff --binary HEAD "$c" >"$out/uncommitted.patch"
git diff --stat HEAD "$c" >"$out/stat.txt"
ahead=$(git rev-list --count "$base..HEAD" 2>/dev/null || echo "?")
[ "$ahead" != 0 ] && [ "$ahead" != "?" ] && git format-patch -q --stdout "$base..HEAD" >"$out/commits.patch"
{ echo "path: $wt"; echo "branch: $branch"; echo "head: $head"; echo "ahead_of_$base: $ahead"
  echo "ref: $ref"; echo "restore: git worktree add <path> $ref"
  printf 'skipped: %s\n' "${skipped[@]:-none}"; } >"$out/meta.txt"
[ -s "$out/uncommitted.patch" ] || { echo "empty patch; verify failed" >&2; exit 3; }
echo "$out"
