#!/usr/bin/env bash
# Builds the e2e shard matrix for build.yml's e2e-env job and appends it to $GITHUB_OUTPUT.
#
#   PR runs: pick the specs the diff can affect (select-specs.ts; unknown file = all).
#   Everything else (main, dispatch): the full suite.
#
# `skip-baseline` is 1 only when this is a PR run AND every step below succeeded on a complete changed-file list.
# Any failure leaves it empty, so the shards run the genuinely full suite (baseline-fail tests included).
# Every step's status is checked explicitly: errexit is ignored inside `if x=$(...)`, so it cannot be relied on here.
#
# Env: E2E_SHARD_TOTAL, EVENT_NAME, REPO, PR_NUMBER, RUNNER_TEMP, GITHUB_OUTPUT, GITHUB_STEP_SUMMARY.
# Needs on PATH: gh, jq, git, bun.
set -u
cd "$(dirname "$0")/../../.." || exit 1

case "${E2E_SHARD_TOTAL:-}" in ''|*[!0-9]*|0) echo "::error::E2E_SHARD_TOTAL must be a positive integer, got '${E2E_SHARD_TOTAL:-}'" >&2; exit 1;; esac
mode=all; total="$E2E_SHARD_TOTAL"; specs=''; changed_specs=''; skip_baseline=''
tmp="${RUNNER_TEMP:?}"

# GitHub caps pulls/{n}/files at 3000 files; a list that long may be truncated, so it counts as a failed selection.
select_specs() {
  gh api "repos/$REPO/pulls/$PR_NUMBER/files" --paginate --jq '.[].filename' > "$tmp/changed.txt" || return 1
  local n; n=$(wc -l < "$tmp/changed.txt") || return 1
  [ "$n" -gt 0 ] && [ "$n" -lt 3000 ] || return 1
  git ls-files 'tests/e2e/*.spec.ts' 'tests/e2e/**/*.spec.ts' > "$tmp/ls.txt" || return 1
  # grep -v exits 1 when it filters out every line; that is an empty spec list, caught just below.
  sed 's#^tests/e2e/##' "$tmp/ls.txt" | { grep -v '^node_modules/' || true; } > "$tmp/specs.txt" || return 1
  [ -s "$tmp/specs.txt" ] || return 1
  bun tests/e2e/scripts/select-specs.ts --specs "$tmp/specs.txt" --changed-file "$tmp/changed.txt" --summary > "$tmp/summary.md" || return 1
  bun tests/e2e/scripts/select-specs.ts --specs "$tmp/specs.txt" --changed-file "$tmp/changed.txt" \
    --timings tests/e2e/shard-timings.json --max-shards "$E2E_SHARD_TOTAL" > "$tmp/selection.json" || return 1
  jq -e '.mode and .shards' "$tmp/selection.json" > /dev/null || return 1
}

if [ "${EVENT_NAME:-}" = pull_request ]; then
  rm -f "$tmp/changed.txt" "$tmp/selection.json" "$tmp/summary.md"
  if select_specs; then
    [ -n "${GITHUB_STEP_SUMMARY:-}" ] && cat "$tmp/summary.md" >> "$GITHUB_STEP_SUMMARY"
    mode=$(jq -r .mode "$tmp/selection.json"); total=$(jq -r .shards "$tmp/selection.json"); specs=$(jq -r '.specs[]' "$tmp/selection.json")
    # Spec files the diff touches are never baseline-skipped (baseline-skip.ts).
    changed_specs=$({ sed -n 's#^tests/e2e/\(.*\.spec\.ts\)$#\1#p' "$tmp/changed.txt" || true; })
    skip_baseline=1
  else
    echo "::warning::e2e spec selection failed (or the PR has no files or 3000+ files); running the full suite with baseline-fail tests included"
  fi
fi

# mode none skips every consumer; a one-element matrix keeps GitHub from rejecting an empty one.
{
  echo "mode=$mode"
  echo "total=$total"
  echo "matrix=$(jq -nc --argjson n "$total" 'if $n < 1 then [1] else [range(1; $n + 1)] end')"
  echo "specs<<E2E_SPECS_EOF"; printf '%s\n' "$specs" | { grep -E '^[A-Za-z0-9._/-]+$' || true; }; echo "E2E_SPECS_EOF"
  echo "skip-baseline=$skip_baseline"
  echo "changed-specs<<E2E_CHANGED_EOF"; printf '%s\n' "$changed_specs" | { grep -E '^[A-Za-z0-9._/-]+$' || true; }; echo "E2E_CHANGED_EOF"
} >> "${GITHUB_OUTPUT:?}"
