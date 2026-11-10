#!/usr/bin/env bash
# Task 0.1i: sample the T1/T2 partial-change PR set.
#
# Takes the 100 most recently created merged PRs (gh sorts merged PRs by
# creation, not merge, time; re-sorted here by mergedAt), classifies each
# 'partial' (touches no GlobalInputs seed glob) or 'global', takes the 30 most
# recent partial, and applies the representativeness rule (plan.md "Sampling,
# proxies and risky assumptions"): sample median changed-file count within 2x
# of the population's AND per-area share of PRs (server/, session/, web-app/,
# other) within 20 percentage points. On failure the sample is re-drawn
# stratified by each PR's primary area (largest-remainder quotas) and the
# report says restratified:true. gh returns at most 100 files per PR, so very
# large PRs are truncated (counted in .truncated_prs).
#
# Prints JSON on stdout and, unless --no-write, also writes pr-sample.json.
# usage: pr-sample.sh [--no-write] [--n 30] [--population 100]
set -euo pipefail
# shellcheck source=lib.sh
. "$(dirname "${BASH_SOURCE[0]}")/lib.sh"
need gh; need jq

N=30 POP=100 WRITE=1
while (($#)); do
  case "$1" in
    --no-write) WRITE=0; shift ;;
    --n) N="$2"; shift 2 ;;
    --population) POP="$2"; shift 2 ;;
    -h|--help) sed -n '2,17p' "$0"; exit 0 ;;
    *) die "unknown arg: $1" ;;
  esac
done

# Seed globs -> one anchored alternation regex.
# shellcheck disable=SC2016  # sed patterns, not shell expansions
GLOBAL_RE="$(grep -vE '^\s*(#|$)' "$BZL_BASELINE_DIR/global-inputs.seed.txt" \
  | sed -e 's/[.+^$(){}|]/\\&/g' -e 's/\*\*/\x01/g' -e 's/\*/[^\/]*/g' -e 's/\x01/.*/g' \
  | paste -sd'|' -)"
GLOBAL_RE="^(${GLOBAL_RE})\$"

PRS="$(mktemp)"; trap 'rm -f "$PRS"' EXIT
BZL_CACHE_KEY="prs-merged-$POP"
mkdir -p "$BZL_CACHE_DIR"
CF="$BZL_CACHE_DIR/$BZL_CACHE_KEY.json"
if [[ ! -s "$CF" || $(( $(date +%s) - $(stat -c %Y "$CF") )) -ge $BZL_CACHE_TTL_S ]]; then
  gh pr list --repo "$BZL_REPO" --state merged --limit "$POP" \
    --json number,mergedAt,headRefOid,files,title > "$CF.tmp" && mv "$CF.tmp" "$CF"
fi
cp "$CF" "$PRS"

# shellcheck disable=SC2016
jq --arg re "$GLOBAL_RE" --argjson n "$N" --arg d "$(date -u +%FT%TZ)" '
  def area: if startswith("server/") then "server" elif startswith("session/") then "session"
            elif startswith("web-app/") then "web-app" else "other" end;
  def median: sort as $s | ($s|length) as $l
    | if $l==0 then null elif $l%2==1 then $s[($l-1)/2] else (($s[$l/2-1]+$s[$l/2])/2) end;
  def areas: ["server","session","web-app","other"];
  def shares($prs): . as $x | ($prs|length) as $t
    | [areas[] as $a | {key:$a, value:(([$prs[]|select(.areas|index($a))]|length)/$t)}] | from_entries;
  def repr($sample; $pop):
    ($sample|map(.files_n)|median) as $ms | ($pop|map(.files_n)|median) as $mp
    | ($sample|shares($sample)) as $ss | ($pop|shares($pop)) as $sp
    | ([areas[] as $a | (($ss[$a]-$sp[$a])*100|fabs)]|max) as $dpp
    | (if $mp==0 or $ms==0 then false else ($ms/$mp <= 2 and $mp/$ms <= 2) end) as $medok
    | {median_files_sample:$ms, median_files_population:$mp, median_ok:$medok,
       area_share_sample:$ss, area_share_population:$sp, max_area_diff_pp:$dpp,
       area_ok:($dpp<=20), ok:($medok and $dpp<=20)};
  (sort_by(.mergedAt) | reverse
   | map(. as $p | ($p.files|map(.path)) as $paths
     | {number, mergedAt, headRefOid, files_n:($paths|length), paths:$paths,
        truncated:(($paths|length)>=100),
        areas:($paths|map(area)|unique),
        primary:(($paths|map(area)|group_by(.)|sort_by(-length)|.[0][0]) // "other"),
        class:(if ($paths|any(test($re))) then "global" else "partial" end)})) as $pop
  | ($pop|map(select(.class=="partial"))) as $partial
  | ($partial[:$n]) as $recent
  | repr($recent; $pop) as $r1
  | (if $r1.ok then {sample:$recent, restrat:false, repr:$r1}
     else
       # stratified re-draw by primary area, largest-remainder quotas
       ([areas[] as $a | {a:$a, raw:($n*(([$pop[]|select(.primary==$a)]|length)/($pop|length)))}]
         | map(.q=(.raw|floor)|.f=(.raw-.q))) as $qs
       | ($n - ($qs|map(.q)|add)) as $left
       | ($qs|sort_by(-.f)|to_entries|map(.value.q += (if .key<$left then 1 else 0 end)|.value)) as $quota
       | ([$quota[] as $q | $partial|map(select(.primary==$q.a))|.[:$q.q][]]) as $picked
       | ($picked|map(.number)) as $have
       | ($picked + ([$partial[]|select(.number as $x|$have|index($x)|not)]|.[:($n-($picked|length))])) as $s2
       | ($s2|sort_by(.mergedAt)|reverse) as $s2s
       | {sample:$s2s, restrat:true, repr:repr($s2s; $pop), quotas:($quota|map({(.a):.q})|add)}
     end) as $res
  | {
      run_date:$d, population:($pop|length),
      class_counts:($pop|group_by(.class)|map({key:.[0].class,value:length})|from_entries),
      truncated_prs:($pop|map(select(.truncated))|length),
      seed_global_regex:$re,
      repr_initial:$r1,
      restratified:$res.restrat,
      repr:$res.repr,
      quotas:($res.quotas // null),
      partial:$res.sample,
      all:$pop
    }' "$PRS" > "$PRS.out"

if ((WRITE)); then cp "$PRS.out" "$BZL_BASELINE_DIR/pr-sample.json"; fi
cat "$PRS.out"; rm -f "$PRS.out"
