#!/usr/bin/env bash
# Gate S3, Task 0.4b/0.4f: build timing protocol (n runs per mode per temperature), 3 concurrent Bun builds,
# footprint rows. Prints one JSON line per run to $OUT and a median/min/max table at the end.
#   baseline-timings.sh <web-app dir> <scratch dir>        env: N=5 PHASES="bun next-prod next-dev concurrent footprint"
# cold = every cache this mode owns is deleted before each run (Bun spike: none exists, so cold == warm apart from the
#   OS file cache, which is not dropped); warm = caches present (one priming run, not counted).
# Next caches: web-app/.next/cache and the shared webpack dir NEXTJS_SHARED_CACHE_DIR (a private one here, so the
#   user's ~/.stapler-squad/nextjs-webpack-cache is neither read nor polluted).
set -euo pipefail
WEB=$(cd "$1" && pwd)
SCRATCH=$2
N=${N:-5}
PHASES=${PHASES:-"bun next-prod next-dev concurrent footprint"}
HERE=$(cd "$(dirname "$0")" && pwd)
TIMED="python3 -I $HERE/timed.py"
OUT=$SCRATCH/timings.jsonl
NEXT_CACHE_DIR=$SCRATCH/next-webpack-cache
mkdir -p "$SCRATCH"
unset npm_config_user_agent

echo "{\"event\":\"start\",\"load\":\"$(cut -d' ' -f1-3 /proc/loadavg)\",\"bun\":\"$(bun --version)\",\"cpus\":$(nproc),\"date\":\"$(date -Is)\"}" >> "$OUT"

record() { # mode temp run -- command...
  local mode=$1 temp=$2 run=$3
  shift 4
  local log=$SCRATCH/log-$mode-$temp-$run.txt
  local res
  res=$($TIMED "$WEB" "$log" -- "$@")
  echo "{\"mode\":\"$mode\",\"temp\":\"$temp\",\"run\":$run,\"res\":$res}" >> "$OUT"
  echo "$mode $temp $run $res"
}

clear_next_caches() { rm -rf "$WEB/.next" "$WEB/out" "$NEXT_CACHE_DIR"; }

if [[ " $PHASES " == *" bun "* ]]; then
  # Real pipeline (web-app/build/, PR 5a); no persistent cache to clear, so cold == warm here (matches S1's finding).
  BUILD=(bun build/build.ts --out "$SCRATCH/bun-timing-out" --stats "$SCRATCH/bun-timing-stats.json")
  for temp in cold warm; do
    for i in $(seq 1 "$N"); do
      record bun-build "$temp" "$i" -- env WEB_MINIFY=1 "${BUILD[@]}"
    done
  done
fi

if [[ " $PHASES " == *" next-prod "* ]]; then
  for temp in cold warm; do
    if [[ $temp == warm ]]; then clear_next_caches; $TIMED "$WEB" "$SCRATCH/prime-prod.txt" -- bun run build >/dev/null; fi
    for i in $(seq 1 "$N"); do
      [[ $temp == cold ]] && clear_next_caches
      record next-prod "$temp" "$i" -- bun run build
    done
  done
fi

if [[ " $PHASES " == *" next-dev "* ]]; then
  # exactly what `make web-build` runs for the Next build: NEXT_BUILD_MODE=development next build
  for temp in cold warm; do
    if [[ $temp == warm ]]; then
      clear_next_caches
      NEXT_BUILD_MODE=development NEXTJS_SHARED_CACHE_DIR="$NEXT_CACHE_DIR" $TIMED "$WEB" "$SCRATCH/prime-dev.txt" -- bun run build >/dev/null
    fi
    for i in $(seq 1 "$N"); do
      [[ $temp == cold ]] && clear_next_caches
      NEXT_BUILD_MODE=development NEXTJS_SHARED_CACHE_DIR="$NEXT_CACHE_DIR" record next-dev-mode "$temp" "$i" -- bun run build
    done
  done
fi

if [[ " $PHASES " == *" concurrent "* ]]; then
  for i in $(seq 1 "$N"); do
    pids=()
    for k in 1 2 3; do
      ($TIMED "$WEB" "$SCRATCH/log-conc-$i-$k.txt" -- env WEB_MINIFY=1 bun build/build.ts --out "$SCRATCH/conc-out-$k" --stats "$SCRATCH/conc-stats-$k.json" > "$SCRATCH/conc-$i-$k.json") &
      pids+=($!)
    done
    wait "${pids[@]}"
    for k in 1 2 3; do echo "{\"mode\":\"bun-build-concurrent3\",\"temp\":\"warm\",\"run\":$i,\"slot\":$k,\"res\":$(cat "$SCRATCH/conc-$i-$k.json")}" >> "$OUT"; done
    echo "concurrent round $i: $(cat "$SCRATCH"/conc-"$i"-*.json | tr '\n' ' ')"
  done
fi

if [[ " $PHASES " == *" footprint "* ]]; then
  FP=$SCRATCH/footprint
  rm -rf "$FP"; mkdir -p "$FP"
  cp "$WEB/package.json" "$WEB/bun.lock" "$WEB/bunfig.toml" "$FP/"
  cp -r "$WEB/patches" "$WEB/eslint-plugin-analytics" "$WEB/eslint-plugin-rpc-lifecycle" "$FP/" 2>/dev/null || true
  echo "{\"mode\":\"footprint\",\"direct_dependencies\":$(python3 -I -c "import json;print(len(json.load(open('$WEB/package.json'))['dependencies']))"),\"direct_devDependencies\":$(python3 -I -c "import json;print(len(json.load(open('$WEB/package.json'))['devDependencies']))"),\"pm_ls_all_lines\":$(cd "$WEB" && bun pm ls --all | wc -l),\"node_modules_kb\":$(du -sk "$WEB/node_modules" | cut -f1)}" >> "$OUT"
  for temp in cold warm; do
    for i in $(seq 1 "$N"); do
      rm -rf "$FP/node_modules"
      [[ $temp == cold ]] && rm -rf "$FP/cache"
      BUN_INSTALL_CACHE_DIR="$FP/cache" record install "$temp" "$i" -- bun install --frozen-lockfile --cwd "$FP"
    done
  done
fi

python3 -I - "$OUT" <<'PY'
import json, statistics, sys, collections
rows = collections.defaultdict(list)
for line in open(sys.argv[1]):
    d = json.loads(line)
    if "res" in d and d["res"].get("rc") == 0:
        rows[(d["mode"], d["temp"])].append(d["res"])
    elif "res" in d:
        rows[(d["mode"], d["temp"] + " FAILED")].append(d["res"])
for (mode, temp), rs in sorted(rows.items()):
    w = [r["wall_s"] for r in rs]; m = [r["maxrss_kb"] for r in rs]
    print(f"{mode:22} {temp:12} n={len(rs)} wall median={statistics.median(w):.2f}s min={min(w):.2f} max={max(w):.2f}  maxrss median={statistics.median(m)/1024:.0f}MB max={max(m)/1024:.0f}MB")
PY
