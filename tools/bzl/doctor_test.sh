#!/usr/bin/env bash
# Seeds one failure per doctor check in a throwaway git repo (DOCTOR_* overrides; the host is never
# touched) and asserts each prints its hint and exits non-zero under --strict.
# shellcheck disable=SC2015  # A && ok || bad: ok() never fails
set -u
real_root=$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)
T=$(mktemp -d "${TMPDIR:-/tmp}/doctor-test.XXXXXX")
trap 'rm -rf "$T"' EXIT
failures=0
ok() { echo "ok   - $1"; }
bad() { echo "FAIL - $1" >&2; failures=$((failures + 1)); }

R="$T/repo"
mkdir -p "$R/tools/bzl" "$T/home" "$T/locks" "$T/out" "$T/cache"
for f in doctor.sh translate-errors.sh errors.yaml slot_history.sh check-build-fresh-cheap.sh; do
  cp "$real_root/tools/bzl/$f" "$R/tools/bzl/$f"
done
echo 9.3.1 > "$R/.bazelversion"
: > "$R/.bazelrc"
mkdir -p "$R/pkg"; echo 'package pkg' > "$R/pkg/a.go"; echo '# build' > "$R/pkg/BUILD.bazel"
( cd "$R" && git init -q . && git config user.email t@t && git config user.name t \
  && git config merge.bazel-lockfile-merge.driver "x %O %A %B" && git add . && git commit -qm init )
printf 'MemAvailable:   33554432 kB\n' > "$T/meminfo-ok"
printf 'MemAvailable:    2097152 kB\n' > "$T/meminfo-low"

# doc <label> <expected text> [env assignments...]: --strict must exit non-zero and print the text
doc() {
  local label=$1 want=$2 out rc; shift 2
  out=$(env DOCTOR_ROOT="$R" DOCTOR_HOME="$T/home" DOCTOR_LOCK_DIR="$T/locks" DOCTOR_OUTPUT_ROOT="$T/out" \
    DOCTOR_DISK_CACHE="$T/cache" DOCTOR_BAZEL_VERSION=9.3.1 DOCTOR_FREE_PCT=60 DOCTOR_FS_A=d1 DOCTOR_FS_B=d1 \
    DOCTOR_MEMINFO="$T/meminfo-ok" DOCTOR_LOADAVG=0.1 DOCTOR_ETC_BAZELRC="$T/none" BAZEL_SLOT_HISTORY="$T/hist" \
    "$@" bash "$R/tools/bzl/doctor.sh" --strict 2>&1); rc=$?
  if [ "$want" = HEALTHY ]; then
    [ "$rc" -eq 0 ] && [ "$(printf '%s\n' "$out" | grep -c '^\[ ok \]')" -eq 9 ] && ok "$label: nine ok lines, exit 0" || { bad "$label: rc=$rc"; printf '%s\n' "$out" >&2; }
    printf '%s\n' "$out" | tail -n 1 | grep -q '^to reproduce a failing test: bazel test <target> --sandbox_debug' && ok "$label: repro hint last" || bad "$label: no repro hint"
    return
  fi
  if [ "$rc" -ne 0 ] && printf '%s\n' "$out" | grep -qF -- "$want"; then ok "$label"; else bad "$label (rc=$rc, wanted: $want)"; printf '%s\n' "$out" >&2; fi
}

doc healthy HEALTHY
doc "wrong version" "does not match .bazelversion" DOCTOR_BAZEL_VERSION=8.0.0
doc "unreachable cache" "remote cache unreachable, building locally" DOCTOR_CACHE_URL=http://127.0.0.1:1
doc "full disk" "bazel-hint: the disk is full" DOCTOR_FREE_PCT=5
doc "cache on another filesystem" "different filesystems" DOCTOR_FS_B=d2
echo 'startup --host_jvm_args=-Xmx8g' > "$T/home/.bazelrc"
doc "home .bazelrc with -Xmx8g" "sets -Xmx" 
rm "$T/home/.bazelrc"
sleep 30 & holder=$!
printf '%s %s build\n' "$holder" "$(date +%s)" > "$T/locks/stapler-squad-bazel.holder.1"
doc "held slot" "1 of 2 busy"
kill "$holder" 2>/dev/null; rm -f "$T/locks/stapler-squad-bazel.holder.1"
printf '#!/bin/sh\nexit 1\n' > "$R/tools/bzl/check-gen-fresh-cheap.sh"; chmod +x "$R/tools/bzl/check-gen-fresh-cheap.sh"
doc "stale gen/" "gen/ is STALE: run 'make sync'"
rm "$R/tools/bzl/check-gen-fresh-cheap.sh"
doc "low memory" "available memory 2 GB (below 8 GB" DOCTOR_MEMINFO="$T/meminfo-low"
doc "high load" "above 0.8 x cores" DOCTOR_LOADAVG=9999
echo 'package pkg' > "$R/pkg/new.go"
doc "stale BUILD file" "BUILD: MAYBE STALE for pkg - run 'make gazelle'"
rm "$R/pkg/new.go"
( cd "$R" && git config --unset merge.bazel-lockfile-merge.driver )
doc "unset merge driver" "merge.bazel-lockfile-merge.driver is unset; run 'make setup-git'"

[ "$failures" -eq 0 ] || exit 1
