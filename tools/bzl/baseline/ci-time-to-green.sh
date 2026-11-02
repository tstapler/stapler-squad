#!/usr/bin/env bash
# Task 0.1a: PR time-to-green baseline from GitHub Actions history.
#
# Definition (plan.md Story 0.1): per PR head SHA, time-to-green is
#   max(updated_at) - min(run_started_at)
# over the required-in-practice workflows (build.yml, lint.yml,
# mcp-integration.yml when triggered). A SHA counts only if the latest run of
# every such workflow that ran for it is completed with conclusion success or
# skipped. The n most recent qualifying SHAs (by latest updated_at) are used.
# Percentiles are nearest-rank (ceil(p*n)-th smallest).
#
# Known limit: the runs API reports only the latest attempt of a re-run
# workflow, so a re-run shortens that workflow's observed span. Counted in
# .rerun_shas so the bias is visible.
#
# usage: ci-time-to-green.sh [--n 30] [--max-pages 40] [--help]
set -euo pipefail
# shellcheck source=lib.sh
. "$(dirname "${BASH_SOURCE[0]}")/lib.sh"

N=30 MAX_PAGES=40
while (($#)); do
  case "$1" in
    --n) N="$2"; shift 2 ;;
    --max-pages) MAX_PAGES="$2"; shift 2 ;;
    -h|--help) sed -n '2,16p' "$0"; exit 0 ;;
    *) die "unknown arg: $1" ;;
  esac
done
need gh; need jq

# shellcheck disable=SC2016
JQ_LIB='
def pct($p): (sort) as $s | if ($s|length)==0 then null else $s[((($p*($s|length))|ceil)-1) | if .<0 then 0 else . end] end;
def ts: fromdateiso8601;
def required: ["build.yml","lint.yml","mcp-integration.yml"];
def wf: (.path // "" | split("/") | last);
# $mode: "required" or "all"
def per_sha($mode):
  group_by(.head_sha)
  | map(
      (map(select($mode=="all" or (wf as $w | required | index($w)))) ) as $rs
      | select(($rs|length)>0)
      # latest run per workflow (by run_number)
      | ($rs | group_by(.workflow_id) | map(max_by(.run_number))) as $latest
      | select($latest | all(.status=="completed" and (.conclusion=="success" or .conclusion=="skipped")))
      | select($mode=="all" or ($latest|any(wf=="build.yml")))
      | {
          head_sha: $latest[0].head_sha,
          end: ($latest|map(.updated_at|ts)|max),
          start: ($latest|map(.run_started_at|ts)|min),
          rerun: ($latest|any(.run_attempt>1)),
          wfs: ($latest|map({key:wf, value:((.updated_at|ts)-(.run_started_at|ts))})|from_entries )
        }
      | .ttg_s = (.end-.start)
    );
'

SLIM='[.workflow_runs[] | {id, workflow_id, run_number, run_attempt, path, name, head_sha, status, conclusion, run_started_at, updated_at, created_at}]'

# fetch_runs <outfile> <mode>: appends pages (slimmed) until enough qualifying SHAs.
fetch_runs() {
  local out="$1" mode="$2" page=1 body cnt
  echo '[]' > "$out"
  while ((page <= MAX_PAGES)); do
    body="$(gh_cached "runs-pr-p$page" "repos/$BZL_REPO/actions/runs?event=pull_request&per_page=100&page=$page")"
    jq -s '.[0] + .[1]' "$out" <(jq -c "$SLIM" <<<"$body") > "$out.new" && mv "$out.new" "$out"
    # A SHA's runs may span a page boundary; require N+3 qualifying SHAs before stopping.
    cnt="$(jq "$JQ_LIB per_sha(\"$mode\") | length" "$out")"
    if (( cnt >= N + 3 )); then break; fi
    if [[ "$(jq '.workflow_runs|length' <<<"$body")" -lt 100 ]]; then break; fi
    page=$((page+1))
  done
  echo "$page" > "$out.pages"
}

collect() { # collect <mode>
  local mode="$1" f
  f="$(mktemp)"; trap 'rm -f "$f" "$f.pages" "$f.new"' RETURN
  fetch_runs "$f" "$mode"
  jq -c --argjson n "$N" --argjson pages "$(cat "$f.pages")" "$JQ_LIB per_sha(\"$mode\") | sort_by(-.end) | .[:\$n] | {pages:\$pages, shas:.}" "$f"
}

summ() { # reads collect output on stdin, prints summary object
  jq -c "$JQ_LIB"'
    .shas as $s | ($s|map(.ttg_s)) as $t
    | {
        n: ($s|length),
        p50_s: ($t|pct(0.5)), p90_s: ($t|pct(0.9)), p95_s: ($t|pct(0.95)),
        min_s: ($t|min), max_s: ($t|max),
        rerun_shas: ($s|map(select(.rerun))|length),
        oldest_head_sha_end: ($s|map(.end)|min|todate), newest_head_sha_end: ($s|map(.end)|max|todate),
        pages_fetched: .pages,
        per_workflow: (
          $s | map(.wfs|to_entries) | add | group_by(.key)
          | map({workflow: .[0].key, n: length, p50_s: (map(.value)|pct(0.5)), p90_s: (map(.value)|pct(0.9))})
        ),
        head_shas: ($s|map(.head_sha))
      }'
}

req_f="$(mktemp)"; all_f="$(mktemp)"; trap 'rm -f "$req_f" "$all_f"' EXIT
collect required | summ > "$req_f"
collect all | summ | jq -c 'del(.head_shas)' > "$all_f"
jq -n --slurpfile r "$req_f" --slurpfile a "$all_f" --arg d "$(date -u +%FT%TZ)" --arg repo "$BZL_REPO" '
  $r[0] + {
    definition: "max(updated_at)-min(run_started_at) per PR head SHA over build.yml, lint.yml, mcp-integration.yml (when triggered); all must be success|skipped; nearest-rank percentiles",
    repo: $repo, run_date: $d,
    sensitivity_all_workflows: $a[0]
  }'
