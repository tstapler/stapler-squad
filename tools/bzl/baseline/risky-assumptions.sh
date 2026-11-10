#!/usr/bin/env bash
# Task 0.1f: pre-measure the two risky assumptions on the sampled partial PRs.
#
# RA1  reverse-dependency fan-out. For each sampled PR: the changed Go packages
#      (dir of each changed .go file; proto/ maps to the generated
#      gen/proto/go/session/v1 packages, web-app/ to server/web) are expanded to
#      every package that transitively imports them, over Imports + TestImports
#      + XTestImports from `go list -e` on the CURRENT tree (a proxy: the graph
#      is HEAD's, not each PR's base). fanout = |affected| / |module packages|,
#      where affected includes the changed packages. PRs touching no Go package
#      (docs, workflows) have fanout 0. Reports the median over all sampled PRs
#      (ra1_median_fanout, the field the plan names), the median over Go-touching
#      PRs only (the unflattered one), and the share of PRs at or under 0.30.
#
# RA2  non-Go share of the critical path. For each sampled PR, in the required
#      workflow run that finished last, a backward chain of jobs is rebuilt from
#      timestamps (the jobs API has no `needs`): start at the last-completing
#      job, repeatedly step to the job that completed latest at or before the
#      current job's start. Gaps between chain jobs and before the first job are
#      runner queue/setup. Share = (e2e + lint + setup/queue seconds) / total
#      chain span. Job classes are name heuristics (see CLASSES below). The
#      plan's definition of these parts is "e2e, lint and runner setup".
#
# Needs pr-sample.json (run pr-sample.sh) and cached or fetchable job data.
# usage: risky-assumptions.sh [--sample <pr-sample.json>]
set -euo pipefail
# shellcheck source=lib.sh
. "$(dirname "${BASH_SOURCE[0]}")/lib.sh"
need jq; need go; need gh; need python3
SAMPLE="$BZL_BASELINE_DIR/pr-sample.json"
while (($#)); do
  case "$1" in
    --sample) SAMPLE="$2"; shift 2 ;;
    -h|--help) sed -n '2,26p' "$0"; exit 0 ;;
    *) die "unknown arg: $1" ;;
  esac
done
[[ -s "$SAMPLE" ]] || bash "$BZL_BASELINE_DIR/pr-sample.sh" >/dev/null

T="$(mktemp -d)"; trap 'rm -rf "$T"' EXIT

# ---- RA1 ----
(cd "$BZL_REPO_ROOT" && nice -n 10 go list -e -f '{{.ImportPath}}|{{join .Imports " "}} {{join .TestImports " "}} {{join .XTestImports " "}}' ./... > "$T/graph.txt" 2>/dev/null) || true
jq '[.partial[]|{number, paths}]' "$SAMPLE" > "$T/prs.json"
MODPATH="$(cd "$BZL_REPO_ROOT" && go list -m)"
ra1="$(python3 -I - "$T/graph.txt" "$T/prs.json" "$MODPATH" <<'PY'
import json, sys, os, collections
graph_f, prs_f, mod = sys.argv[1:4]
edges = {}
for line in open(graph_f):
    line = line.rstrip("\n")
    if "|" not in line: continue
    pkg, imps = line.split("|", 1)
    edges[pkg] = {i for i in imps.split() if i.startswith(mod + "/") or i == mod}
pkgs = set(edges)
rev = collections.defaultdict(set)
for p, imps in edges.items():
    for i in imps: rev[i].add(p)
def pkg_of(path):
    if path.endswith(".go"):
        d = os.path.dirname(path)
        return {mod + ("/" + d if d else "")}
    if path.startswith("proto/"):
        return {mod + "/gen/proto/go/session/v1", mod + "/gen/proto/go/session/v1/sessionv1connect"}
    if path.startswith("web-app/"):
        return {mod + "/server/web"}
    return set()
out = []
for pr in json.load(open(prs_f)):
    changed = set()
    for p in pr["paths"]: changed |= pkg_of(p)
    seen, stack = set(), list(changed)
    while stack:
        c = stack.pop()
        if c in seen: continue
        seen.add(c)
        stack.extend(rev.get(c, ()))
    affected = seen & pkgs
    out.append({"number": pr["number"], "go_touching": bool(changed), "changed_pkgs": len(changed & pkgs),
                "affected": len(affected), "fanout": len(affected) / len(pkgs) if pkgs else None})
print(json.dumps({"module_packages": len(pkgs), "per_pr": out}))
PY
)"
ra1_json="$(jq -c '
  def med: sort as $s | ($s|length) as $l | if $l==0 then null elif $l%2==1 then $s[($l-1)/2] else (($s[$l/2-1]+$s[$l/2])/2) end;
  .per_pr as $p |
  { ra1_median_fanout: ($p|map(.fanout)|med),
    ra1_median_fanout_go_touching: ($p|map(select(.go_touching)|.fanout)|med),
    ra1_go_touching_prs: ($p|map(select(.go_touching))|length),
    ra1_share_prs_le_30pct: (($p|map(select(.fanout<=0.30))|length)/($p|length)),
    ra1_module_packages: .module_packages, ra1_per_pr: $p }' <<<"$ra1")"

# ---- RA2 ----
rows=()
for sha in $(jq -r '.partial[].headRefOid' "$SAMPLE"); do
  runs="$(BZL_CACHE_TTL_S=31536000 gh_cached "runs-sha-$sha" "repos/$BZL_REPO/actions/runs?head_sha=$sha&per_page=100")"
  # last-finishing required workflow run that completed
  rid="$(jq -r '[.workflow_runs[]|select(.event=="pull_request" and .status=="completed" and (.path|test("build.yml|lint.yml|mcp-integration.yml")))]
        | sort_by(.updated_at) | last | .id // empty' <<<"$runs")"
  [[ -n "$rid" ]] || continue
  jobs="$(BZL_CACHE_TTL_S=31536000 gh_cached "jobs-$rid" "repos/$BZL_REPO/actions/runs/$rid/jobs?per_page=100&filter=latest")"
  rstart="$(jq -r --arg id "$rid" '.workflow_runs[]|select((.id|tostring)==$id)|.run_started_at' <<<"$runs")"
  rows+=("$(jq -c --arg sha "$sha" --arg rstart "$rstart" '
    def ts: fromdateiso8601;
    # CLASSES (case-insensitive name heuristics)
    def cls: if test("e2e|playwright";"i") then "e2e"
             elif test("lint|eslint|golangci|jscpd|dupl|shellcheck|actionlint|storybook|dependency screen|smoke test|guard";"i") then "lint"
             elif test("detect|prepare|environment|setup";"i") then "setup"
             else "go_or_other" end;
    [.jobs[]|select(.conclusion!="skipped" and .started_at and .completed_at)
      |{name, s:(.started_at|ts), e:(.completed_at|ts)}] as $j
    | if ($j|length)==0 then empty else
      ($j|max_by(.e)) as $last
      | (reduce range(0;200) as $i ({cur:$last, chain:[$last], done:false};
          if .done then . else
            (.cur.s) as $cs
            | ([$j[]|select(.e <= $cs + 1 and .name != .cur.name)] ) as $c
            | if ($c|length)==0 then .done=true else (($c|max_by(.e)) as $n | .cur=$n | .chain += [$n]) end
          end)) as $st
      | ($st.chain|reverse) as $ch
      | ($rstart|ts) as $r0
      | ($last.e - $r0) as $span
      | ([range(0;($ch|length))|
            ($ch[.]) as $c
            | (if . == 0 then ($c.s - $r0) else ($c.s - $ch[.-1].e) end) as $gap
            | {cls:($c.name|cls), dur:($c.e-$c.s), gap:(if $gap<0 then 0 else $gap end)}]) as $parts
      | {sha:$sha, span_s:$span,
         e2e_s:([$parts[]|select(.cls=="e2e")|.dur]|add // 0),
         lint_s:([$parts[]|select(.cls=="lint")|.dur]|add // 0),
         setup_s:(([$parts[]|select(.cls=="setup")|.dur]|add // 0) + ([$parts[].gap]|add // 0)),
         go_or_other_s:([$parts[]|select(.cls=="go_or_other")|.dur]|add // 0),
         chain_jobs:($ch|length)}
      | .non_go_share = (if .span_s>0 then ((.e2e_s+.lint_s+.setup_s)/.span_s) else null end)
      end' <<<"$jobs")")
done
((${#rows[@]} > 0)) || die "no job data for RA2"
ra2_json="$(printf '%s\n' "${rows[@]}" | jq -s '
  def med: sort as $s | ($s|length) as $l | if $l==0 then null elif $l%2==1 then $s[($l-1)/2] else (($s[$l/2-1]+$s[$l/2])/2) end;
  { ra2_non_go_share: (map(.non_go_share)|map(select(.!=null))|med), ra2_prs_measured: length,
    ra2_median_components_s: {e2e:(map(.e2e_s)|med), lint:(map(.lint_s)|med), setup_and_queue:(map(.setup_s)|med), go_or_other:(map(.go_or_other_s)|med), span:(map(.span_s)|med)},
    ra2_per_pr: . }')"

jq -n --argjson a "$ra1_json" --argjson b "$ra2_json" --arg d "$(date -u +%FT%TZ)" --arg head "$(git -C "$BZL_REPO_ROOT" rev-parse --short HEAD)" \
  '$a + $b + {run_date:$d, graph_head:$head}'
