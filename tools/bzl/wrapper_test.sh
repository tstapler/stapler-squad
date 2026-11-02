#!/usr/bin/env bash
# Tests tools/bazel with a fake BAZEL_REAL: concurrency cap (flock and mkdir locks), CI bypass,
# and (when a systemd user manager exists) the scope's MemoryMax and oom_score_adj.
set -u
root=$(git rev-parse --show-toplevel)
wrapper="$root/tools/bazel"
T=$(mktemp -d "${TMPDIR:-/tmp}/wrapper-test.XXXXXX")
trap 'rm -rf "$T"' EXIT
failures=0
ok() { echo "ok   - $1"; }
bad() { echo "FAIL - $1" >&2; failures=$((failures + 1)); }

cat > "$T/fake-bazel" <<'FAKE'
#!/bin/sh
# Records the peak number of simultaneously running fakes in $T/peak.
d="$T/running"; mkdir -p "$d"
touch "$d/$$"
n=$(ls "$d" | wc -l)
( flock 7; cur=$(cat "$T/peak" 2>/dev/null || echo 0); [ "$n" -gt "$cur" ] && echo "$n" > "$T/peak"; true ) 7>"$T/peak.lock"
sleep 1
rm -f "$d/$$"
FAKE
chmod +x "$T/fake-bazel"

run6() { # run6 <label> [env assignments...]
  rm -rf "$T/running" "$T/peak" "$T/locks"; mkdir -p "$T/locks"
  local pids=""
  for _ in 1 2 3 4 5 6; do
    env T="$T" BAZEL_REAL="$T/fake-bazel" BAZEL_WRAPPER_LOCK_DIR="$T/locks" "${@:2}" "$wrapper" version >/dev/null 2>&1 &
    pids="$pids $!"
  done
  # shellcheck disable=SC2086
  wait $pids
  cat "$T/peak" 2>/dev/null || echo 0
}

peak=$(run6 flock BAZEL_WRAPPER_NO_SYSTEMD=1)
if [ "$peak" -ge 1 ] && [ "$peak" -le 2 ]; then ok "flock: peak concurrency $peak <= 2"; else bad "flock: peak concurrency $peak"; fi
peak=$(run6 mkdir BAZEL_WRAPPER_NO_SYSTEMD=1 BAZEL_WRAPPER_NO_FLOCK=1)
if [ "$peak" -ge 1 ] && [ "$peak" -le 2 ]; then ok "mkdir: peak concurrency $peak <= 2"; else bad "mkdir: peak concurrency $peak"; fi
peak=$(run6 ci CI=true BAZEL_WRAPPER_NO_SYSTEMD=1)
if [ "$peak" -gt 2 ]; then ok "CI=true bypasses the semaphore (peak $peak)"; else bad "CI=true did not bypass (peak $peak)"; fi

# Missing BAZEL_REAL must fail loudly, not run something else.
if env -u BAZEL_REAL "$wrapper" version >/dev/null 2>&1; then bad "unset BAZEL_REAL accepted"; else ok "unset BAZEL_REAL rejected"; fi

# systemd scope properties, read from inside the scope.
if command -v systemd-run >/dev/null 2>&1 && systemctl --user show-environment >/dev/null 2>&1; then
  cat > "$T/probe-bazel" <<'PROBE'
#!/bin/sh
unit=$(sed 's#.*/##' /proc/self/cgroup)
{ cat /proc/self/oom_score_adj; systemctl --user show "$unit" -p MemoryMax; } > "$T/probe.out"
PROBE
  chmod +x "$T/probe-bazel"
  env -u CI -u GITHUB_ACTIONS T="$T" BAZEL_REAL="$T/probe-bazel" BAZEL_WRAPPER_LOCK_DIR="$T/locks" "$wrapper" version >/dev/null 2>&1
  if [ "$(sed -n 1p "$T/probe.out" 2>/dev/null)" = 500 ]; then ok "oom_score_adj=500 inside the scope"; else bad "oom_score_adj: $(sed -n 1p "$T/probe.out" 2>/dev/null)"; fi
  if [ "$(sed -n 2p "$T/probe.out" 2>/dev/null)" = "MemoryMax=15032385536" ]; then ok "scope MemoryMax=14G (15032385536)"; else bad "MemoryMax: $(sed -n 2p "$T/probe.out" 2>/dev/null)"; fi
else
  bad "no systemd user manager: scope properties could not be evaluated (counts as failure, not pass)"
fi

[ "$failures" -eq 0 ] || { echo "$failures check(s) failed" >&2; exit 1; }
echo "wrapper_test: all checks passed"
