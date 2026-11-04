#!/usr/bin/env bash
# Task 0.1b: cold build time and disk per worktree on the make path.
#
# Each run: git worktree add --detach <tmp>/wt <ref> (outside the repo), time
# `make build`, record du deltas, remove the worktree. Never touches :8543 and
# never installs the service.
#
# The host is shared with agent sessions, so quiet is enforced: before every
# run the script refuses (exit 3) while the 1-minute load average exceeds
# BZL_MAX_LOAD1 (default 2.0). There is no override flag.
#
# --gocache cold (default): fresh empty GOCACHE per run, matching the Story 0.1
#   AC ("no shared caches"); GOMODCACHE stays shared (downloads, not builds).
# --gocache default: the user's normal GOCACHE (Task 0.1b text, "left as
#   default"). The two texts in plan.md conflict; both modes exist and the
#   chosen mode is recorded in the output as .gocache_mode.
#
# usage: worktree-cost.sh [--runs 5] [--ref origin/main] [--gocache cold|default] [--dry-run] [--out baseline-worktree.json]
# Output: JSON {runs:[{wall_s,disk_kb_worktree,disk_kb_node_modules,
#   disk_kb_gocache,loadavg1,mem_used_gb,...}], median/min/max, ...}
set -euo pipefail
# shellcheck source=lib.sh
. "$(dirname "${BASH_SOURCE[0]}")/lib.sh"

RUNS=5 REF=origin/main GOCACHE_MODE=cold DRY=0 OUT=""
while (($#)); do
  case "$1" in
    --runs) RUNS="$2"; shift 2 ;;
    --ref) REF="$2"; shift 2 ;;
    --gocache) GOCACHE_MODE="$2"; shift 2 ;;
    --dry-run) DRY=1; shift ;;
    --out) OUT="$2"; shift 2 ;;
    -h|--help) sed -n "2,23p" "$0"; exit 0 ;;
    *) die "unknown arg: $1" ;;
  esac
done
[[ "$GOCACHE_MODE" == cold || "$GOCACHE_MODE" == default ]] || die "--gocache must be cold|default"
need git; need jq; need make; need go

if ((DRY)); then
  jq -n --argjson runs "$RUNS" --arg ref "$REF" --arg m "$GOCACHE_MODE" --arg l "$(loadavg1)" --arg max "$BZL_MAX_LOAD1" \
    '{dry_run:true, would_run:$runs, ref:$ref, gocache_mode:$m, loadavg1_now:($l|tonumber), max_loadavg1:($max|tonumber),
      steps:["require_quiet_host","git worktree add --detach <tmp>/wt <ref>","make build (timed)","du -sk","git worktree remove --force"]}'
  exit 0
fi

mem_used_gb() { free -g | awk '/^Mem:/{print $3}'; }
kb() { du -sk "$1" 2>/dev/null | cut -f1; }

TMP=""
cleanup() {
  if [[ -n "$TMP" ]]; then
    git -C "$BZL_REPO_ROOT" worktree remove --force "$TMP/wt" 2>/dev/null || true
    rm -rf "$TMP"
    git -C "$BZL_REPO_ROOT" worktree prune
  fi
}
trap cleanup EXIT

results=()
for ((i = 1; i <= RUNS; i++)); do
  require_quiet_host
  TMP="$(mktemp -d "${TMPDIR:-/tmp}/bzlwc.XXXXXX")"
  load="$(loadavg1)"; mem="$(mem_used_gb)"
  git -C "$BZL_REPO_ROOT" worktree add --quiet --detach "$TMP/wt" "$REF"
  env_prefix=()
  gocache_dir=""
  if [[ "$GOCACHE_MODE" == cold ]]; then
    gocache_dir="$TMP/gocache"; mkdir -p "$gocache_dir"; env_prefix=(env "GOCACHE=$gocache_dir")
  fi
  t0="$(date +%s.%N)"
  rc=0
  (cd "$TMP/wt" && unset npm_config_user_agent && "${env_prefix[@]}" make build >"$TMP/build.log" 2>&1) || rc=$?
  t1="$(date +%s.%N)"
  nm="$TMP/wt/web-app/node_modules"
  nm_resolved="$(readlink -f "$nm" 2>/dev/null || true)"
  results+=("$(jq -n \
    --argjson run "$i" --argjson rc "$rc" --argjson wall "$(awk -v a="$t0" -v b="$t1" 'BEGIN{printf "%.2f", b-a}')" \
    --argjson wt "$(kb "$TMP/wt")" --argjson nm "$( [[ -n "$nm_resolved" && -e "$nm_resolved" ]] && kb "$nm_resolved" || echo 0 )" \
    --argjson gc "$( [[ -n "$gocache_dir" ]] && kb "$gocache_dir" || echo 0 )" \
    --argjson shared "$( [[ -L "$nm" ]] && echo true || echo false )" \
    --argjson load "$load" --argjson mem "$mem" --arg sha "$(git -C "$TMP/wt" rev-parse --short HEAD)" \
    '{run:$run, make_exit:$rc, wall_s:$wall, disk_kb_worktree:$wt, disk_kb_node_modules:$nm, node_modules_is_symlink:$shared,
      disk_kb_gocache:$gc, loadavg1:$load, mem_used_gb:$mem, ref_sha:$sha}')")
  cleanup; TMP=""
done

final="$(printf '%s\n' "${results[@]}" | jq -s --arg m "$GOCACHE_MODE" --arg ref "$REF" --arg d "$(date -u +%FT%TZ)" '
  def med: sort as $s | ($s|length) as $l | if $l%2==1 then $s[($l-1)/2] else (($s[$l/2-1]+$s[$l/2])/2) end;
  def st($k): map(.[$k]) | {median:med, min:min, max:max};
  { runs:., gocache_mode:$m, ref:$ref, run_date:$d,
    wall_s:st("wall_s"), disk_kb_worktree:st("disk_kb_worktree"), disk_kb_node_modules:st("disk_kb_node_modules"),
    disk_kb_gocache:st("disk_kb_gocache"), loadavg1:st("loadavg1"), mem_used_gb:st("mem_used_gb"),
    all_builds_ok:(all(.make_exit==0)) }')"
[[ -n "$OUT" ]] && printf '%s\n' "$final" > "$OUT"
printf '%s\n' "$final"
