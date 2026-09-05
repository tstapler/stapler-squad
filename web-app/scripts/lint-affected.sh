#!/usr/bin/env bash
# Local-dev equivalent of .github/workflows/lint.yml's file-scoped ESLint
# step: lint only files changed vs BASE (default origin/main) via
# `next lint --file`, instead of a full-repo `next lint` surfacing every
# pre-existing warning in the repo. Run from web-app/ (or via `pnpm run
# lint:affected`); override the base with `BASE=some-ref pnpm run lint:affected`.
set -euo pipefail

BASE="${BASE:-origin/main}"

# Union of committed changes since the merge-base with BASE, uncommitted
# tracked edits, and new untracked files -- so this reflects the working
# tree during local dev, not just what's already committed (unlike CI's
# equivalent step in .github/workflows/lint.yml, which only needs the
# committed range since it always runs against a clean checkout).
CHANGED=$( { \
  git diff --name-only "$BASE"...HEAD -- '*.ts' '*.tsx' 2>/dev/null; \
  git diff --name-only HEAD -- '*.ts' '*.tsx' 2>/dev/null; \
  git ls-files --others --exclude-standard -- '*.ts' '*.tsx' 2>/dev/null; \
} | sed 's|^web-app/||' | sort -u)

if [ -z "$CHANGED" ]; then
  echo "No TS/TSX files changed vs $BASE — nothing to lint"
  exit 0
fi

FILE_FLAGS=()
while IFS= read -r f; do
  [ -n "$f" ] && [ -f "$f" ] && FILE_FLAGS+=(--file "$f")
done <<< "$CHANGED"

if [ ${#FILE_FLAGS[@]} -eq 0 ]; then
  echo "Changed TS/TSX files vs $BASE no longer exist on disk — nothing to lint"
  exit 0
fi

npx next lint "${FILE_FLAGS[@]}"
