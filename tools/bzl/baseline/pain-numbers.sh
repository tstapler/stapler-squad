#!/usr/bin/env bash
# Task 0.1f: assemble the "Measured pain" numbers as one JSON object, reusing
# the outputs of the other collectors (no recomputation of CI numbers beyond
# calling ci-time-to-green.sh, whose API responses are cached).
#
# Inputs that need a quiet host (cold worktree build, warm one-package test
# latency, make-path hermeticity) are read from files written by their own
# scripts when present:
#   tools/bzl/baseline/baseline-worktree.json  (worktree-cost.sh --out)
#   tools/bzl/baseline/hermeticity-make.json   (hermeticity.sh --system make --out)
# and are otherwise reported as "UNMEASURED: PENDING-QUIET-HOST", never
# estimated. The warm one-package latency is measured here with `go test` (plan
# text says `make test PKG=./config`, but the Makefile has no PKG variable) and
# only when the host is quiet (load <= 2.0, no override).
# usage: pain-numbers.sh
set -euo pipefail
# shellcheck source=lib.sh
. "$(dirname "${BASH_SOURCE[0]}")/lib.sh"
need jq; need go
B="$BZL_BASELINE_DIR"
PEND='"UNMEASURED: PENDING-QUIET-HOST"'

ci="$(bash "$B/ci-time-to-green.sh")"
regen="$(bash "$B/regen-steps.sh")"
billed="$(bash "$B/ci-billed-minutes.sh" | jq -c 'del(.per_pr)')"
risky="$(bash "$B/risky-assumptions.sh" | jq -c 'del(.ra1_per_pr, .ra2_per_pr)')"
fixtures='null'; [[ -s "$B/fixtures/make-path-results.json" ]] && fixtures="$(jq -c '.fixtures|map({fixture,names_remedy})' "$B/fixtures/make-path-results.json")"
wt="$PEND"; [[ -s "$B/baseline-worktree.json" ]] && wt="$(jq -c . "$B/baseline-worktree.json")"
herm="$PEND"; [[ -s "$B/hermeticity-make.json" ]] && herm="$(jq -c . "$B/hermeticity-make.json")"

warm="$PEND"
if awk -v l="$(loadavg1)" -v m="$BZL_MAX_LOAD1" 'BEGIN{exit !(l<=m)}'; then
  (cd "$BZL_REPO_ROOT" && go test -short -count=1 ./config >/dev/null 2>&1) || true # warm-up
  ts=()
  for _ in 1 2 3 4 5; do
    t0="$(date +%s.%N)"; (cd "$BZL_REPO_ROOT" && go test -short -count=1 ./config >/dev/null 2>&1) || true
    ts+=("$(awk -v a="$t0" -v b="$(date +%s.%N)" 'BEGIN{printf "%.3f", b-a}')")
  done
  warm="$(printf '%s\n' "${ts[@]}" | jq -s 'sort|.[2]')"
fi

jq -n --argjson ci "$ci" --argjson regen "$regen" --argjson billed "$billed" --argjson risky "$risky" \
  --argjson fixtures "$fixtures" --argjson wt "$wt" --argjson herm "$herm" --argjson warm "$warm" \
  --arg d "$(date -u +%FT%TZ)" '
  def pick($o; $path): if ($o|type)=="object" then ($o|getpath($path)) else $o end;
  {
    run_date:$d,
    ci_n:$ci.n, ci_p50_s:$ci.p50_s, ci_p90_s:$ci.p90_s, ci_p95_s:$ci.p95_s, ci_per_workflow:$ci.per_workflow,
    cold_worktree_wall_s: pick($wt; ["wall_s","median"]),
    disk_kb_worktree: pick($wt; ["disk_kb_worktree","median"]),
    disk_kb_node_modules: pick($wt; ["disk_kb_node_modules","median"]),
    regen_steps_per_session_median:$regen.regen_steps_per_session_median,
    regen_steps_per_session_p90:$regen.regen_steps_per_session_p90,
    regen_steps_detail:{all:$regen.all, build_active:$regen.build_active},
    warm_one_pkg_test_s:$warm,
    partial_class:$billed.partial, global_class:$billed.global, all_class:$billed.all,
    ra1_median_fanout:$risky.ra1_median_fanout, ra1_median_fanout_go_touching:$risky.ra1_median_fanout_go_touching,
    ra1_share_prs_le_30pct:$risky.ra1_share_prs_le_30pct, ra2_non_go_share:$risky.ra2_non_go_share,
    ra2_median_components_s:$risky.ra2_median_components_s,
    fixtures:$fixtures, hermeticity_make:$herm
  }'
