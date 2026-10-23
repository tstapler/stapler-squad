#!/usr/bin/env bash
# Story 8.3 (project_plans/web-app-bun-build/implementation/plan.md): the gate that decides whether PR 8
# (the Bun bundler cutover) may merge. Runs `make web-build` for WEB_BUILD=next and WEB_BUILD=bun back to
# back on this host, n>=5 per mode per temperature (cold, warm), and prints:
#   - the warm-median ratio (next_warm / bun_warm) -- THE GATE, must be >= 5
#   - cold medians and min-max ranges for both modes
#   - a second, non-gating ratio using Next's warm number recomputed with both cache dirs cleared on every
#     run (this is numerically identical to the Next "cold" series below -- see plan Story 8.3's own text:
#     "Next's incremental cache makes warm Next faster... this second ratio is reported beside the gate for
#     the owner's decision, not a second gate")
#
# cold   = the mode's build caches are deleted before every run (Next: web-app/.next and the shared webpack
#          cache dir; Bun: none exist per gates S1/S3, so cold == warm there except for the
#          Makefile/proto-gen/ensure-tools overhead that is identical either way)
# warm   = caches present (one untimed priming run, then N measured runs with no cache clearing)
#
# NOTE: Makefile:260 hardcodes NEXTJS_SHARED_CACHE_DIR=$HOME/.stapler-squad/nextjs-webpack-cache inside the
# recipe's own shell line, so it cannot be redirected to a scratch path via an exported env var (a shell
# prefix assignment on a command always wins over an already-exported value for that command). "Next cold"
# below therefore deletes that real, shared directory -- harmless to correctness (it is only a build-speed
# cache, rebuilt on next use) but it is shared with any other Next build running on this host concurrently;
# check host load before running cold Next phases for real.
#
# Every timed run also deletes web-app/out, server/web/dist and the WEB_BUILD_STAMP file first, which is
# required: those are Make's own target/prerequisite files, and with them left alone `make web-build` would
# see nothing out of date on a repeat run of the *same* mode and skip the recipe entirely (0.00s, not a real
# build). Deleting them forces Make to actually invoke the bundler every single timed run -- this is
# orthogonal to the cold/warm distinction above, which is only about the bundler's own incremental caches.
#
# Usage: scripts/measure-web-build.sh [N]   (default N=5)
# Env:   PHASES="next bun" to run a subset (default: both)
# Run from the repo root. Writes nothing outside web-app/out, server/web/dist, web-app/.web-build-mode,
# web-app/.next, and a scratch dir under /tmp -- never touches :8543 or any installed service.
set -euo pipefail

N=${1:-${N:-5}}
PHASES=${PHASES:-"next bun"}
REPO=$(cd "$(dirname "$0")/.." && pwd)
WEB="$REPO/web-app"
SCRATCH=$(mktemp -d /tmp/measure-web-build.XXXXXX)
TIMED="python3 -I $REPO/project_plans/web-app-bun-build/spikes/s3/timed.py"
OUT="$SCRATCH/timings.jsonl"
NEXT_CACHE_DIR="$HOME/.stapler-squad/nextjs-webpack-cache" # real path the Makefile hardcodes; see NOTE above
unset npm_config_user_agent

echo "host: $(uname -a)"
echo "bun: $(bun --version), load avg at start: $(cut -d' ' -f1-3 /proc/loadavg 2>/dev/null || sysctl -n vm.loadavg), cpus: $(nproc 2>/dev/null || sysctl -n hw.ncpu), commit: $(git -C "$REPO" rev-parse --short HEAD)"
echo "n=$N, scratch=$SCRATCH"
echo

force_clean() {
  rm -rf "$WEB/out" "$REPO/server/web/dist" "$WEB/.web-build-mode"
}
clear_next_caches() {
  rm -rf "$WEB/.next" "$NEXT_CACHE_DIR"
}

record() { # mode temp run -- command...
  local mode=$1 temp=$2 run=$3
  shift 3
  shift # drop the literal --
  local log="$SCRATCH/log-$mode-$temp-$run.txt"
  force_clean
  local res
  res=$($TIMED "$REPO" "$log" -- "$@")
  echo "{\"mode\":\"$mode\",\"temp\":\"$temp\",\"run\":$run,\"load1\":$(cut -d' ' -f1 /proc/loadavg 2>/dev/null || echo null),\"res\":$res}" >>"$OUT"
  echo "  $mode $temp run $run: $res"
}

if [[ " $PHASES " == *" next "* ]]; then
  echo "== WEB_BUILD=next (make web-build, NEXT_BUILD_MODE=development, exactly what Makefile:260 runs) =="
  for temp in cold warm; do
    if [[ $temp == warm ]]; then
      echo "-- priming Next's cache (untimed) --"
      clear_next_caches
      make -C "$REPO" web-build WEB_BUILD=next >"$SCRATCH/prime-next.txt" 2>&1 || {
        echo "priming build failed, see $SCRATCH/prime-next.txt" >&2
        exit 1
      }
    fi
    for i in $(seq 1 "$N"); do
      [[ $temp == cold ]] && clear_next_caches
      record next-warm-build "$temp" "$i" -- make -C "$REPO" web-build WEB_BUILD=next
    done
  done
fi

if [[ " $PHASES " == *" bun "* ]]; then
  echo "== WEB_BUILD=bun (make web-build, WEB_MINIFY=0, the deployed default until Story 8.6) =="
  for temp in cold warm; do
    if [[ $temp == warm ]]; then
      echo "-- priming (untimed; Bun has no persistent build cache per gates S1/S3, so this primes only the OS file cache / node_modules) --"
      make -C "$REPO" web-build WEB_BUILD=bun >"$SCRATCH/prime-bun.txt" 2>&1 || {
        echo "priming build failed, see $SCRATCH/prime-bun.txt" >&2
        exit 1
      }
    fi
    for i in $(seq 1 "$N"); do
      record bun-build "$temp" "$i" -- make -C "$REPO" web-build WEB_BUILD=bun
    done
  done
fi

force_clean

echo
echo "== Summary =="
python3 -I - "$OUT" <<'PY'
import json, statistics, sys, collections
rows = collections.defaultdict(list)
for line in open(sys.argv[1]):
    d = json.loads(line)
    key = (d["mode"], d["temp"])
    if d["res"].get("rc") == 0:
        rows[key].append(d["res"])
    else:
        rows[(d["mode"], d["temp"] + " FAILED")].append(d["res"])

def med(key):
    rs = rows.get(key)
    if not rs:
        return None
    return statistics.median(r["wall_s"] for r in rs)

for (mode, temp), rs in sorted(rows.items()):
    w = [r["wall_s"] for r in rs]
    print(f"{mode:20} {temp:8} n={len(rs):<2} wall median={statistics.median(w):7.2f}s min={min(w):7.2f} max={max(w):7.2f}")

print()
next_warm, bun_warm = med(("next-warm-build", "warm")), med(("bun-build", "warm"))
next_cold, bun_cold = med(("next-warm-build", "cold")), med(("bun-build", "cold"))
if next_warm and bun_warm:
    ratio = next_warm / bun_warm
    gate = "PASS" if ratio >= 5 else "MISS"
    print(f"GATE (warm medians): next {next_warm:.2f}s / bun {bun_warm:.2f}s = {ratio:.2f}x  -> {gate} (threshold: >= 5x)")
if next_cold and bun_warm:
    ratio2 = next_cold / bun_warm
    print(f"Secondary ratio (Next caches cleared every run): next-cold {next_cold:.2f}s / bun-warm {bun_warm:.2f}s = {ratio2:.2f}x (reported for the owner's decision, not a gate)")
if next_cold:
    print(f"Next cold median: {next_cold:.2f}s")
if bun_cold:
    print(f"Bun cold median:  {bun_cold:.2f}s (expected ~= bun warm: no persistent build cache, gates S1/S3)")
PY
echo
echo "raw data: $OUT"
