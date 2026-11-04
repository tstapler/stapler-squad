#!/usr/bin/env bash
# T5 collector (P10, Task 7.5g): replays the 9 seeded failures of tools/bzl/baseline/fixtures through
# the tools/bazel wrapper and asserts each prints a `Remedy:` line within its first 20 lines and at
# most 60 lines in total. Replay uses the captured failure log for the fixture's errors.yaml row
# (tools/bzl/testdata/errors/<row>.log) as a stand-in Bazel, so it needs no Bazel server and no seeding;
# E3 is seen by the plain-go shims (translator, --source go) and E7 is wrapper-emitted (a held slot).
# ERRORS_YAML=<file> swaps the table (used to prove a missing remedy is caught).
# Prints one PASS/FAIL line per fixture, then `LEGIBLE: <n>/9`; exits 1 unless all pass.
set -u
here=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)
root=$(cd "$here/../.." && pwd)
fixtures="$here/baseline/fixtures"
logs="$here/testdata/errors"
T=$(mktemp -d "${TMPDIR:-/tmp}/legibility.XXXXXX")
trap 'rm -rf "$T"' EXIT

cat > "$T/replay-bazel" <<'REPLAY'
#!/bin/sh
cat "$REPLAY_LOG"
exit "${REPLAY_EXIT:-1}"
REPLAY
chmod +x "$T/replay-bazel"
mkdir -p "$T/ob/execroot" "$T/locks"

wrapper_env() {
  env -u CI -u GITHUB_ACTIONS BAZEL_REAL="$T/replay-bazel" BAZEL_WRAPPER_LOCK_DIR="$T/locks" BAZEL_WRAPPER_NO_SYSTEMD=1 \
    BAZEL_WRAPPER_NO_STALE_NOTE=1 BAZEL_LOG_DIR="$T/logs" BAZEL_SLOT_HISTORY="$T/hist" \
    BAZEL_WRAPPER_OUTPUT_BASE="$T/ob" "$@"
}

total=0
legible=0
for fx in "$fixtures"/[0-9]-*.sh; do
  total=$((total + 1))
  name=$(basename "$fx")
  key=$(bash "$fx" --key)
  case "$key" in
    E3) out=$("$here/translate-errors.sh" --source go --exit 1 < "$logs/E3.log") ;;
    E7)
      # Hold the only slot, then ask for another with a 2 s timeout: the wrapper's own messages.
      ( exec 9>>"$T/locks/stapler-squad-bazel.lock.1"; flock 9; sleep 3 ) &
      sleep 0.3
      out=$(wrapper_env BAZEL_WRAPPER_SLOTS=1 BAZEL_SLOT_TIMEOUT=2 REPLAY_LOG="$logs/E1.log" "$root/tools/bazel" build //x 2>&1)
      wait
      ;;
    *)
      rc=1; case "$key" in E6|E11) rc=0 ;; E4) rc=137 ;; esac
      out=$(wrapper_env REPLAY_LOG="$logs/$key.log" REPLAY_EXIT="$rc" "$root/tools/bazel" build //x 2>&1)
      ;;
  esac
  n=$(printf '%s\n' "$out" | wc -l)
  pos=$(printf '%s\n' "$out" | grep -n 'Remedy:' | head -n 1 | cut -d: -f1)
  if [ -n "$pos" ] && [ "$pos" -le 20 ] && [ "$n" -le 60 ]; then
    echo "PASS $name ($key): Remedy: on line $pos of $n"
    legible=$((legible + 1))
  else
    echo "FAIL $name ($key): Remedy: line=${pos:-none}, total lines=$n" >&2
  fi
done
echo "LEGIBLE: $legible/$total"
[ "$legible" -eq "$total" ] && [ "$total" -eq 9 ]
