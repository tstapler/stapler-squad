#!/usr/bin/env bash
# Task 0.1i: join per PR head SHA runner-minutes and time-to-green with the
# PR class from pr-sample.json, and print p50 per class.
#
# runner_min = sum over every job of every pull_request workflow run on the
# head SHA of (completed_at - started_at) in minutes, weighted by runner OS
# (Linux 1, macOS 10, Windows 2). The repository is public, so the GitHub
# billing API returns 0; this is a labelled proxy, not billed minutes.
# billed_style_min additionally rounds each job up to a whole minute.
# Skipped jobs (no real start/finish) contribute 0.
# time-to-green uses the Task 0.1a definition (required workflows, latest run
# each, max updated_at - min run_started_at); ttg_p50 is over SHAs where all
# those runs ended success|skipped.
# Classes: partial = the sampled partial PRs in pr-sample.json; global = every
# global PR of the 100; all = all 100.
# usage: ci-billed-minutes.sh [--sample <pr-sample.json>]
set -euo pipefail
# shellcheck source=lib.sh
. "$(dirname "${BASH_SOURCE[0]}")/lib.sh"
need gh; need jq

SAMPLE="$BZL_BASELINE_DIR/pr-sample.json"
while (($#)); do
  case "$1" in
    --sample) SAMPLE="$2"; shift 2 ;;
    -h|--help) sed -n '2,19p' "$0"; exit 0 ;;
    *) die "unknown arg: $1" ;;
  esac
done
[[ -s "$SAMPLE" ]] || bash "$BZL_BASELINE_DIR/pr-sample.sh" >/dev/null

# Jobs and runs of completed workflow runs never change: cache "forever".
export BZL_CACHE_TTL_S=31536000
OUT="$(mktemp)"; trap 'rm -f "$OUT"' EXIT
: > "$OUT"
while read -r number sha; do
  runs="$(gh_cached "runs-sha-$sha" "repos/$BZL_REPO/actions/runs?head_sha=$sha&per_page=100" \
    | jq -c '[.workflow_runs[]|select(.event=="pull_request")|{id,path,run_number,run_attempt,status,conclusion,run_started_at,updated_at}]')"
  jobs='[]'
  for id in $(jq -r '.[]|select(.status=="completed")|.id' <<<"$runs"); do
    j="$(gh_cached "jobs-$id" "repos/$BZL_REPO/actions/runs/$id/jobs?per_page=100&filter=latest" \
      | jq -c '[.jobs[]|{labels,conclusion,started_at,completed_at}]')"
    jobs="$(jq -c -n --argjson a "$jobs" --argjson b "$j" '$a+$b')"
  done
  jq -c -n --argjson number "$number" --arg sha "$sha" --argjson runs "$runs" --argjson jobs "$jobs" \
    '{number:$number, sha:$sha, runs:$runs, jobs:$jobs}' >> "$OUT"
done < <(jq -r '.all[]|"\(.number) \(.headRefOid)"' "$SAMPLE")

# shellcheck disable=SC2016
jq -s --slurpfile s "$SAMPLE" --arg d "$(date -u +%FT%TZ)" '
  def pct($p): (sort) as $x | if ($x|length)==0 then null else $x[((($p*($x|length))|ceil)-1)|if .<0 then 0 else . end] end;
  def ts: fromdateiso8601;
  def wf: (.path // "" | split("/") | last);
  def req: ["build.yml","lint.yml","mcp-integration.yml"];
  def osw: if any(.[]; test("macos";"i")) then 10 elif any(.[]; test("windows";"i")) then 2 else 1 end;
  def job_s: if .started_at and .completed_at and .conclusion!="skipped"
             then ((.completed_at|ts)-(.started_at|ts)) else 0 end;
  def per_pr:
    . as $r
    | ($r.jobs|map(job_s as $s | (.labels|osw) as $w | {m:($s/60*$w), b:((($s/60)|ceil)*$w)})) as $jm
    | ($r.runs|map(select(wf as $w|req|index($w)))|group_by(.path)|map(max_by(.run_number))) as $lat
    | {number:$r.number, sha:$r.sha, runs:($r.runs|length), jobs:($r.jobs|length),
       runner_min:($jm|map(.m)|add // 0), billed_style_min:($jm|map(.b)|add // 0),
       has_required:($lat|length>0),
       green:($lat|length>0 and all(.status=="completed" and (.conclusion=="success" or .conclusion=="skipped"))),
       ttg_s:(if ($lat|length)>0 then (($lat|map(.updated_at|ts)|max)-($lat|map(.run_started_at|ts)|min)) else null end)};
  (map(per_pr)) as $all
  | ($s[0].partial|map(.number)) as $pn
  | ($s[0].all|map(select(.class=="global")|.number)) as $gn
  | def cls($rows): {
      n:($rows|length),
      n_with_runs:($rows|map(select(.runs>0))|length),
      runner_min_p50:($rows|map(.runner_min)|pct(0.5)),
      runner_min_p90:($rows|map(.runner_min)|pct(0.9)),
      billed_style_min_p50:($rows|map(.billed_style_min)|pct(0.5)),
      n_ttg:($rows|map(select(.green and .ttg_s!=null))|length),
      ttg_p50_s:($rows|map(select(.green and .ttg_s!=null)|.ttg_s)|pct(0.5)),
      ttg_p90_s:($rows|map(select(.green and .ttg_s!=null)|.ttg_s)|pct(0.9))
    };
  {
    run_date:$d,
    unit_note:"runner_min = sum(job completed_at-started_at)/60 x OS multiplier (Linux 1, macOS 10, Windows 2); public repo so GitHub billing is 0; nearest-rank percentiles",
    partial:cls($all|map(select(.number as $x|$pn|index($x)))),
    global:cls($all|map(select(.number as $x|$gn|index($x)))),
    all:cls($all),
    per_pr:$all
  }' "$OUT"
