#!/usr/bin/env bash
# duration-trend.sh
#
# For each of this repo's tracked workflow files, fetches the last 15 runs
# via `gh api`, filters to runs whose triggering event matches that
# workflow's *current* on: block (to avoid averaging in runs from a
# since-changed trigger config — see project_plans/ci-speed/research/
# pitfalls.md §0, the demo-publish.yml stale-baseline finding this guards
# against), computes each run's wall-clock duration
# (updated_at - run_started_at), and compares the new rolling average
# against the last recorded value in docs/ci/duration-history.jsonl.
#
# Emits `::warning::` on a >25% regression. Never fails the run — this is
# an advisory signal only, per project_plans/ci-speed/decisions/
# ADR-002-ci-duration-budget-mechanism.md. A real `gh api` fetch/parse
# failure is reported as `::error::ci-duration-trend: fetch failed for
# <file> — <reason>` so it reads as visibly distinct from the expected
# "insufficient recent same-trigger-config runs" case.
#
# Usage: ./tools/ci/duration-trend.sh
# Requires: gh (authenticated, e.g. via GH_TOKEN), jq
# Run from the repository root.

set -uo pipefail

REPO="tstapler/stapler-squad"
HISTORY_FILE="docs/ci/duration-history.jsonl"

# filename|comma-separated list of trigger events currently in that
# workflow's on: block. Keep in sync with .github/workflows/*.yml whenever a
# workflow's trigger config changes — an out-of-date list here reproduces
# the exact stale-baseline mistake documented in pitfalls.md §0.
WORKFLOWS=(
  "backlog-scaffolding-guard.yml|pull_request"
  "benchmark.yml|push,pull_request"
  "build.yml|push,pull_request,workflow_dispatch"
  "demo-publish.yml|workflow_dispatch"
  "deploy-pages.yml|push,pull_request"
  "e2e-video.yml|pull_request"
  "generated-proto-guard.yml|pull_request"
  "goreleaser-check.yml|push,pull_request,workflow_dispatch"
  "lint.yml|push,pull_request,workflow_dispatch"
  "mcp-integration.yml|push,pull_request"
  "registry-validation.yml|pull_request"
  "release-please.yml|push"
  "release.yml|workflow_dispatch"
  "ux-analysis.yml|pull_request"
)

mkdir -p "$(dirname "$HISTORY_FILE")"
touch "$HISTORY_FILE"

timestamp="$(date -u +%Y-%m-%dT%H:%M:%SZ)"

for entry in "${WORKFLOWS[@]}"; do
  file="${entry%%|*}"
  events_csv="${entry#*|}"
  events_json="$(printf '%s\n' "$events_csv" | tr ',' '\n' | jq -R . | jq -sc .)"

  echo "::group::${file}"

  err_file="$(mktemp)"
  raw_runs="$(gh api "repos/${REPO}/actions/workflows/${file}/runs" \
    --jq '.workflow_runs[:15] | map({id, event, created_at, run_started_at, updated_at, conclusion})' \
    2>"$err_file")"
  rc=$?
  fetch_err="$(cat "$err_file")"
  rm -f "$err_file"

  if [ "$rc" -ne 0 ]; then
    echo "::error::ci-duration-trend: fetch failed for ${file} — gh api exited ${rc}: ${fetch_err}"
    echo "::endgroup::"
    continue
  fi

  if ! printf '%s' "$raw_runs" | jq -e 'type == "array"' >/dev/null 2>&1; then
    echo "::error::ci-duration-trend: fetch failed for ${file} — unexpected response shape (not a JSON array)"
    echo "::endgroup::"
    continue
  fi

  total_runs="$(printf '%s' "$raw_runs" | jq 'length')"

  durations_minutes="$(printf '%s' "$raw_runs" | jq -c --argjson events "$events_json" '
    [ .[]
      | select(.conclusion != null)
      | select(.run_started_at != null and .updated_at != null)
      | select(.event as $e | $events | index($e) != null)
      | ((.updated_at | fromdateiso8601) - (.run_started_at | fromdateiso8601)) / 60
    ]')"

  if ! printf '%s' "$durations_minutes" | jq -e 'type == "array"' >/dev/null 2>&1; then
    echo "::error::ci-duration-trend: fetch failed for ${file} — could not compute durations from response"
    echo "::endgroup::"
    continue
  fi

  count="$(printf '%s' "$durations_minutes" | jq 'length')"

  if [ "$count" -eq 0 ]; then
    echo "ci-duration-trend: insufficient recent same-trigger-config runs for ${file} (0 of ${total_runs} recent runs match expected events: ${events_csv})"
    echo "::endgroup::"
    continue
  fi

  avg_minutes="$(printf '%s' "$durations_minutes" | jq '(add / length * 10 | round) / 10')"

  echo "ci-duration-trend: ${file} rolling avg over ${count} run(s) = ${avg_minutes}m"

  prior_avg="$(jq -c --arg wf "$file" 'select(.workflow == $wf)' "$HISTORY_FILE" 2>/dev/null | tail -1 | jq -r '.rolling_avg_minutes // empty' 2>/dev/null)"

  if [ -n "${prior_avg:-}" ]; then
    pct_change="$(awk -v new="$avg_minutes" -v old="$prior_avg" 'BEGIN { if (old == 0) { print "0" } else { printf "%.1f", ((new - old) / old) * 100 } }')"
    over_threshold="$(awk -v pct="$pct_change" 'BEGIN { print (pct > 25) ? "1" : "0" }')"
    if [ "$over_threshold" = "1" ]; then
      echo "::warning::${file} rolling avg grew from ${prior_avg}m to ${avg_minutes}m (+${pct_change}%)"
    fi
  fi

  entry_json="$(jq -nc --arg wf "$file" --arg ts "$timestamp" --argjson avg "$avg_minutes" \
    '{workflow: $wf, timestamp: $ts, rolling_avg_minutes: $avg}')"
  printf '%s\n' "$entry_json" >> "$HISTORY_FILE"

  echo "::endgroup::"
done

exit 0
