#!/usr/bin/env bash
# Tests tools/bazel with a fake BAZEL_REAL: concurrency cap (flock and mkdir locks), CI bypass,
# and (when a systemd user manager exists) the scope's MemoryMax and oom_score_adj.
# shellcheck disable=SC2015  # A && ok || bad: ok() never fails
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

# Startup args: heap always, absolute --output_base only in the S6 fallback mode.
cat > "$T/args-bazel" <<'ARGS'
#!/bin/sh
echo "$*" > "$T/args.out"
ARGS
chmod +x "$T/args-bazel"
mkdir -p "$T/locks"
env T="$T" BAZEL_REAL="$T/args-bazel" BAZEL_WRAPPER_LOCK_DIR="$T/locks" BAZEL_WRAPPER_NO_SYSTEMD=1 "$wrapper" info >/dev/null 2>&1
case "$(cat "$T/args.out")" in *-Xmx2g*) ok "heap -Xmx2g passed on the command line" ;; *) bad "no -Xmx2g: $(cat "$T/args.out")" ;; esac
case "$(cat "$T/args.out")" in *--output_base*) bad "output_base set without BAZEL_WORKTREE_OUTPUT_BASE" ;; *) ok "no --output_base by default" ;; esac
( cd "$root" && env T="$T" BAZEL_REAL="$T/args-bazel" BAZEL_WRAPPER_LOCK_DIR="$T/locks" BAZEL_WRAPPER_NO_SYSTEMD=1 BAZEL_WORKTREE_OUTPUT_BASE=1 "$wrapper" info >/dev/null 2>&1 )
case "$(cat "$T/args.out")" in *"--output_base=$root/.bazel-out"*) ok "BAZEL_WORKTREE_OUTPUT_BASE=1 passes absolute --output_base=$root/.bazel-out" ;; *) bad "output_base arg: $(cat "$T/args.out")" ;; esac
env T="$T" BAZEL_REAL="$T/args-bazel" BAZEL_WRAPPER_LOCK_DIR="$T/locks" BAZEL_WRAPPER_NO_SYSTEMD=1 BAZEL_AGENT=1 "$wrapper" info >/dev/null 2>&1
case "$(cat "$T/args.out")" in *--max_idle_secs=30*) ok "agent caller gets --max_idle_secs=30" ;; *) bad "agent idle: $(cat "$T/args.out")" ;; esac

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


# ---- Story 1.6 cases: agent config, output caps, queue messages, heartbeats, detach, cold note ----
cat > "$T/scripted-bazel" <<'SCRIPTED'
#!/bin/sh
echo "$*" >> "$T/calls"
case "${FAKE_MODE:-lines}" in
  lines) i=0; while [ "$i" -lt "${FAKE_N:-100}" ]; do echo "line $i"; i=$((i + 1)); done ;;
  fail) i=0; while [ "$i" -lt 100 ]; do echo "noise $i"; i=$((i + 1)); done
        echo "ERROR: /w/x/BUILD.bazel:1:1: boom"; echo "missing strict dependencies"
        i=0; while [ "$i" -lt 100 ]; do echo "tail $i"; i=$((i + 1)); done; exit 1 ;;
  errors) i=0; while [ "$i" -lt "${FAKE_N:-120}" ]; do echo "ERROR: error number $i"; i=$((i + 1)); done; exit 1 ;;
  sleep) mkdir -p "${BAZEL_WRAPPER_OUTPUT_BASE:-/nonexistent}/execroot" 2>/dev/null; sleep "${FAKE_SLEEP:-1}"; echo done ;;
esac
mkdir -p "${BAZEL_WRAPPER_OUTPUT_BASE:-/nonexistent}/execroot" 2>/dev/null
exit 0
SCRIPTED
chmod +x "$T/scripted-bazel"
mkdir -p "$T/ob/execroot"
# wrun <lockdir-suffix> <env...> -- <wrapper args...> : runs the wrapper with a private, quiet environment
wrun() {
  local sfx=$1; shift
  local envs=()
  while [ "$1" != "--" ]; do envs+=("$1"); shift; done
  shift
  mkdir -p "$T/locks-$sfx"
  env T="$T" BAZEL_REAL="$T/scripted-bazel" BAZEL_WRAPPER_LOCK_DIR="$T/locks-$sfx" BAZEL_WRAPPER_NO_SYSTEMD=1 \
    BAZEL_WRAPPER_NO_STALE_NOTE=1 BAZEL_LOG_DIR="$T/logs-$sfx" BAZEL_SLOT_HISTORY="$T/hist-$sfx" \
    BAZEL_WRAPPER_OUTPUT_BASE="${OB:-$T/ob}" ${envs[@]+"${envs[@]}"} "$wrapper" "$@"
}

# agent config: added for build, never for query, removed by BAZEL_HUMAN / --config=human
: > "$T/calls"; wrun a FAKE_MODE=lines FAKE_N=1 -- build //x >/dev/null 2>&1
grep -q -- 'build --config=agent //x' "$T/calls" && ok "build gets --config=agent on a non-TTY" || bad "no --config=agent: $(cat "$T/calls")"
: > "$T/calls"; wrun a FAKE_MODE=lines FAKE_N=1 -- query //x >/dev/null 2>&1
grep -q -- '--config=agent' "$T/calls" && bad "query got --config=agent" || ok "query never gets --config=agent"
: > "$T/calls"; wrun a BAZEL_HUMAN=1 FAKE_MODE=lines FAKE_N=1 -- build //x >/dev/null 2>&1
grep -q -- '--config=agent' "$T/calls" && bad "BAZEL_HUMAN=1 still got agent" || ok "BAZEL_HUMAN=1 opts out"
: > "$T/calls"; wrun a FAKE_MODE=lines FAKE_N=1 -- build --config=human //x >/dev/null 2>&1
if grep -q -- '--config=' "$T/calls"; then bad "--config=human not stripped: $(cat "$T/calls")"; else ok "--config=human opts out and is stripped"; fi

# success output cap
n=$(wrun a FAKE_MODE=lines FAKE_N=500 -- build //x 2>&1 | wc -l)
[ "$n" -le 40 ] && ok "success output capped ($n <= 40 lines)" || bad "success output $n lines"

# failure cap, hint, log path
out=$(wrun a FAKE_MODE=fail -- build //x 2>&1); n=$(printf '%s\n' "$out" | wc -l)
[ "$n" -le 60 ] && [ "$(printf '%s\n' "$out" | tail -n 1 | cut -c1-9)" = "full log:" ] && ok "failure view $n lines ends with full log:" || bad "failure view: $n lines, last: $(printf '%s\n' "$out" | tail -n 1)"
printf '%s\n' "$out" | head -n 20 | grep -q 'Remedy:' && ok "remedy within the first 20 lines" || bad "no remedy in first 20 lines"
out=$(wrun a FAKE_MODE=errors FAKE_N=120 -- build //x 2>&1); n=$(printf '%s\n' "$out" | wc -l)
if [ "$n" -le 60 ] && printf '%s\n' "$out" | grep -q 'more ERROR lines not shown'; then ok "120 ERROR lines: $n lines with '... more ERROR lines not shown'"; else bad "120 errors: $n lines"; fi

# E7: three holders plus one waiter print the first message within 2 s
for _ in 1 2 3; do wrun e BAZEL_WRAPPER_SLOTS=3 FAKE_MODE=sleep FAKE_SLEEP=6 -- version >/dev/null 2>&1 & done
sleep 1
( wrun e BAZEL_WRAPPER_SLOTS=3 FAKE_MODE=lines FAKE_N=1 -- version > "$T/e7.out" 2>&1 ) &
waiter=$!
sleep 2
grep -q 'bazel: waiting for a build slot (position 1 of 1, ETA ~' "$T/e7.out" && ok "E7 first message within 2 s" || bad "E7 message: $(cat "$T/e7.out")"
wait "$waiter"

# timeout exits 75
wrun t FAKE_MODE=sleep FAKE_SLEEP=8 BAZEL_WRAPPER_SLOTS=1 -- version >/dev/null 2>&1 &
holder=$!
sleep 1
start=$SECONDS
wrun t BAZEL_WRAPPER_SLOTS=1 BAZEL_SLOT_TIMEOUT=3 -- version > "$T/t.out" 2>&1; rc=$?
[ "$rc" -eq 75 ] && grep -q 'no build slot after 3s' "$T/t.out" && ok "BAZEL_SLOT_TIMEOUT=3 prints the timeout line and exits 75 ($((SECONDS - start)) s)" || bad "timeout: rc=$rc $(cat "$T/t.out")"
wait "$holder"

# kill -9 / kill -TERM of a wrapper holding the only slot frees it within 3 s (flock and mkdir locks)
for lockmode in flock mkdir; do
  for sig in KILL TERM; do
    extra_env=(); [ "$lockmode" = mkdir ] && extra_env=(BAZEL_WRAPPER_NO_FLOCK=1)
    wrun "k$lockmode$sig" BAZEL_WRAPPER_SLOTS=1 FAKE_MODE=sleep FAKE_SLEEP=20 ${extra_env[@]+"${extra_env[@]}"} -- version >/dev/null 2>&1 &
    wp=$!
    sleep 1
    kill -"$sig" "$wp" 2>/dev/null
    start=$SECONDS
    wrun "k$lockmode$sig" BAZEL_WRAPPER_SLOTS=1 BAZEL_SLOT_TIMEOUT=10 FAKE_MODE=lines FAKE_N=1 ${extra_env[@]+"${extra_env[@]}"} -- version >/dev/null 2>&1; rc=$?
    took=$((SECONDS - start))
    [ "$rc" -eq 0 ] && [ "$took" -le 3 ] && ok "$lockmode lock freed ${took}s after kill -$sig" || bad "$lockmode kill -$sig: rc=$rc took ${took}s"
    wait "$wp" 2>/dev/null
  done
done

# heartbeats: ticks shrink so 130 "seconds" take ~14 s; 30/60/120 give exactly 3, never more than 10
wrun h1 BAZEL_WRAPPER_TICK=0.1 FAKE_MODE=sleep FAKE_SLEEP=14 -- build //x > "$T/hb1.out" 2>&1 &
p1=$!
wrun h2 BAZEL_WRAPPER_TICK=0.01 FAKE_MODE=sleep FAKE_SLEEP=14 -- build //x > "$T/hb2.out" 2>&1 &
p2=$!
wait "$p1"; wait "$p2"
c1=$(grep -c 'bazel: still running' "$T/hb1.out"); c2=$(grep -c 'bazel: still running' "$T/hb2.out")
[ "$c1" -eq 3 ] && ok "heartbeats at 30/60/120: $c1 lines" || bad "heartbeat count $c1"
[ "$c2" -ge 5 ] && [ "$c2" -le 10 ] && [ "$(wc -l < "$T/hb2.out")" -le 40 ] && ok "heartbeats capped at 10 inside the 40-line cap ($c2)" || bad "heartbeat cap: $c2"

# BAZEL_BG=1 returns at once; bg-status.sh later reports the exit
start=$SECONDS
out=$(wrun b BAZEL_BG=1 FAKE_MODE=sleep FAKE_SLEEP=3 -- build //x 2>&1); took=$((SECONDS - start))
case "$out" in *"started in background (pid "*"), log: .bazel-logs/last-build.log, poll: bash tools/bzl/bg-status.sh"*) [ "$took" -le 2 ] && ok "BAZEL_BG=1 returned in ${took}s" || bad "BAZEL_BG took ${took}s" ;; *) bad "BAZEL_BG output: $out" ;; esac
[ "$(BAZEL_LOG_DIR="$T/logs-b" bash "$root/tools/bzl/bg-status.sh")" = running ] && ok "bg-status: running" || bad "bg-status not running"
sleep 5
[ "$(BAZEL_LOG_DIR="$T/logs-b" bash "$root/tools/bzl/bg-status.sh")" = "finished exit=0" ] && ok "bg-status: finished exit=0" || bad "bg-status: $(BAZEL_LOG_DIR="$T/logs-b" bash "$root/tools/bzl/bg-status.sh")"

# auto-detach: returns within 4 s with exit 76 while Bazel keeps running
start=$SECONDS
out=$(wrun d BAZEL_AUTO_DETACH_SECS=2 FAKE_MODE=sleep FAKE_SLEEP=6 -- build //x 2>&1); rc=$?; took=$((SECONDS - start))
case "$out" in *"detached after 2s; poll with tools/bzl/bg-status.sh"*) [ "$rc" -eq 76 ] && [ "$took" -le 4 ] && ok "auto-detach: exit 76 in ${took}s" || bad "detach rc=$rc took=$took" ;; *) bad "detach output: $out" ;; esac
sleep 6
[ "$(BAZEL_LOG_DIR="$T/logs-d" bash "$root/tools/bzl/bg-status.sh")" = "finished exit=0" ] && ok "detached run later reports finished exit=0" || bad "detached status: $(BAZEL_LOG_DIR="$T/logs-d" bash "$root/tools/bzl/bg-status.sh")"

# cold-first-build note: exactly once with an empty output base, then never
OB="$T/ob-cold"
c1=$(OB=$OB wrun c FAKE_MODE=sleep FAKE_SLEEP=0 -- build //x 2>&1 | grep -c 'first build in this worktree is cold')
c2=$(OB=$OB wrun c FAKE_MODE=sleep FAKE_SLEEP=0 -- build //x 2>&1 | grep -c 'first build in this worktree is cold')
[ "$c1" -eq 1 ] && [ "$c2" -eq 0 ] && ok "cold note printed once ($c1 then $c2)" || bad "cold note counts $c1/$c2"
OB=""

[ "$failures" -eq 0 ] || { echo "$failures check(s) failed" >&2; exit 1; }
echo "wrapper_test: all checks passed"
