#!/usr/bin/env bash
# `make bazel-doctor`: nine host/worktree health checks, one line each, then the repro hint.
#   doctor.sh [--strict]   exit 0 unless a check FAILed (--strict also fails on WARN)
# Every probe has a DOCTOR_* override so doctor_test.sh can seed failures without touching the host.
# Remedy texts come from errors.yaml through translate-errors.sh, never restated here.
set -u
strict=""
[ "${1:-}" = "--strict" ] && strict=1
[ -z "${DOCTOR_STRICT:-}" ] || strict=1

here=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)
root="${DOCTOR_ROOT:-$(cd "$here/../.." && pwd)}"
cd "$root" || exit 2
tr_sh="$root/tools/bzl/translate-errors.sh"
# shellcheck source=slot_history.sh disable=SC1091
[ -f "$root/tools/bzl/slot_history.sh" ] && . "$root/tools/bzl/slot_history.sh"
home="${DOCTOR_HOME:-$HOME}"

failed=0
warned=0
line() { printf '%s\n' "$1"; }
ok() { line "[ ok ] $1"; }
warn() { line "[WARN] $1"; warned=$((warned + 1)); }
fail() { line "[FAIL] $1"; failed=$((failed + 1)); }
note() { line "       $1"; }

# Hint for a sample failure line, through the same table the wrapper uses.
hint_for() { # <sample line> [exit]
  printf '%s\n' "$1" | "$tr_sh" --hints-only --exit "${2:-1}" 2>/dev/null | head -n 2
}

# --- 1 version ---
want=$(tr -d '[:space:]' < .bazelversion 2>/dev/null || true)
got="${DOCTOR_BAZEL_VERSION:-}"
if [ -z "$got" ]; then
  bz=$(command -v bazelisk || command -v bazel || true)
  [ -z "$bz" ] || got=$(BAZELISK_SKIP_WRAPPER=1 "$bz" --version 2>/dev/null | awk '/^bazel /{print $2; exit}')
fi
if [ -z "$got" ]; then warn "1 version: no bazelisk/bazel on PATH (expected $want)"
elif [ "$got" = "$want" ]; then ok "1 version: Bazel $got matches .bazelversion"
else fail "1 version: Bazel $got does not match .bazelversion ($want); run through bazelisk"; fi

# --- 2 remote cache ---
url="${DOCTOR_CACHE_URL:-}"
if [ -z "$url" ]; then url=$(grep -hoE -- '--remote_cache=[^[:space:]]+' .bazelrc "$home/.bazelrc" 2>/dev/null | head -n 1 | cut -d= -f2-); fi
if [ -z "$url" ]; then
  ok "2 remote cache: none configured (local disk cache only)"
else
  case "$url" in
    http://*|https://*)
      code=$(curl -s -o /dev/null --max-time 3 -w '%{http_code}' "$url" 2>/dev/null || true)
      case "$code" in
        000|"") fail "2 remote cache: $url unreachable within 3 s"; note "$(hint_for 'WARNING: Remote Cache: UNAVAILABLE: Connection refused' 0 | head -n 1)" ;;
        401|403) fail "2 remote cache: $url rejected our credentials (HTTP $code)"; note "$(hint_for 'WARNING: HTTP/1.1 403 Forbidden' 0 | head -n 1)" ;;
        *) ok "2 remote cache: $url reachable (HTTP $code)" ;;
      esac ;;
    *) ok "2 remote cache: $url (not http; reachability not probed)" ;;
  esac
fi

# --- 3 disk space and filesystem layout ---
out_root="${DOCTOR_OUTPUT_ROOT:-$home/.cache/bazel}"
disk_cache="${DOCTOR_DISK_CACHE:-$home/.cache/stapler-squad-bazel/disk}"
nearest() { local p=$1; while [ ! -e "$p" ] && [ "$p" != / ]; do p=$(dirname "$p"); done; echo "$p"; }
out_p=$(nearest "$out_root"); cache_p=$(nearest "$disk_cache")
free_pct="${DOCTOR_FREE_PCT:-}"
if [ -z "$free_pct" ]; then
  used=$(df -Pk "$out_p" "$cache_p" 2>/dev/null | awk 'NR>1{gsub("%","",$5); if ($5+0>m) m=$5+0} END{print m+0}')
  free_pct=$((100 - used))
fi
if [ "$free_pct" -lt 15 ]; then
  fail "3 disk: ${free_pct}% free on the Bazel output-base/disk-cache filesystem (need at least 15%)"
  note "$(hint_for 'No space left on device' | head -n 1)"
else
  ok "3 disk: ${free_pct}% free (at least 15%)"
fi
fs_a="${DOCTOR_FS_A:-$(df -Pk "$out_p" 2>/dev/null | awk 'NR==2{print $1}')}"
fs_b="${DOCTOR_FS_B:-$(df -Pk "$cache_p" 2>/dev/null | awk 'NR==2{print $1}')}"
if [ "$fs_a" != "$fs_b" ]; then
  fstype=$(stat -f -c %T "$out_p" 2>/dev/null || echo unknown)
  warn "3 disk: output base and disk cache are on different filesystems ($fs_a vs $fs_b, output base is $fstype); cache hits are copied, not reflinked"
fi

# --- 4 slot holders ---
lock_dir="${DOCTOR_LOCK_DIR:-${XDG_RUNTIME_DIR:-${TMPDIR:-/tmp}}}"
nslots="${BAZEL_WRAPPER_SLOTS:-2}"
busy=0; desc=""
for f in "$lock_dir"/stapler-squad-bazel.holder.*; do
  [ -e "$f" ] || continue
  read -r hpid hstart hcmd < "$f" || continue
  if kill -0 "$hpid" 2>/dev/null; then
    busy=$((busy + 1))
    desc="$desc [pid $hpid '$hcmd' running $(($(date +%s) - hstart))s]"
  fi
done
eta=120
if command -v slot_history_median >/dev/null 2>&1; then eta=$(slot_history_median "" 5 1 2>/dev/null || echo 120); fi
if [ "$busy" -gt 0 ]; then warn "4 slots: $busy of $nslots busy:$desc; ETA ~${eta}s per queued build"
else ok "4 slots: 0 of $nslots busy; typical build holds a slot ~${eta}s"; fi

# --- 5 where this worktree's state lives ---
links=""
for l in bazel-bin bazel-out bazel-testlogs; do [ -L "$root/$l" ] && links="$links $l"; done
if command -v md5sum >/dev/null 2>&1; then h=$(printf '%s' "$(pwd -P)" | md5sum | cut -d' ' -f1); else h=$(printf '%s' "$(pwd -P)" | md5 -q 2>/dev/null || echo unknown); fi
ok "5 state: symlinks in worktree root:${links:- none yet}; output base $out_root/_bazel_${USER:-$(id -un)}/$h"

# --- 6 generated-code precondition ---
gen_check="$root/tools/bzl/check-gen-fresh-cheap.sh"
if [ -x "$gen_check" ]; then
  if "$gen_check" >/dev/null 2>&1; then ok "6 precondition: Bazel needs no generated code; plain go and gopls need 'make sync' - gen/ is fresh"
  else fail "6 precondition: Bazel needs no generated code; plain go and gopls need 'make sync' - gen/ is STALE: run 'make sync'"; fi
else
  ok "6 precondition: Bazel needs no generated code; plain go and gopls need 'make sync' (gen/ freshness check skipped until Task 2.4d lands)"
fi

# --- 7 memory, load, server, rc overrides ---
meminfo="${DOCTOR_MEMINFO:-/proc/meminfo}"
parts=""; bad7=""
if [ -r "$meminfo" ]; then
  avail_kb=$(awk '/^MemAvailable:/{print $2}' "$meminfo")
  avail_gb=$((avail_kb / 1048576))
  parts="available memory ${avail_gb} GB"
  if [ "$avail_kb" -lt 8388608 ]; then
    bad7=1
    top=$(ps -eo rss=,comm= --sort=-rss 2>/dev/null | head -n 3 | awk '{printf "%s(%d MB) ", $2, $1/1024}')
    parts="$parts (below 8 GB; largest: ${top:-unknown})"
  fi
else
  parts="available memory unknown (no $meminfo)"
fi
loadavg="${DOCTOR_LOADAVG:-$(awk '{print $1}' /proc/loadavg 2>/dev/null || true)}"
cores=$(nproc 2>/dev/null || sysctl -n hw.ncpu 2>/dev/null || echo 1)
if [ -n "$loadavg" ]; then
  parts="$parts; load $loadavg on $cores cores"
  if awk -v l="$loadavg" -v c="$cores" 'BEGIN{exit !(l > 0.8 * c)}'; then bad7=1; parts="$parts (above 0.8 x cores)"; fi
fi
spid_file="$out_root/_bazel_${USER:-$(id -un)}/$h/server/server.pid.txt"
if [ -r "$spid_file" ] && kill -0 "$(cat "$spid_file")" 2>/dev/null; then
  parts="$parts; server $(ps -o etimes=,rss= -p "$(cat "$spid_file")" 2>/dev/null | awk '{printf "age %ds rss %d MB", $1, $2/1024}')"
else
  parts="$parts; no Bazel server running for this worktree"
fi
if grep -qs -- '-Xmx' "$home/.bazelrc"; then bad7=1; parts="$parts; $home/.bazelrc sets -Xmx (overrides the committed 2g in rc order; the wrapper still forces 2g)"; fi
if [ -e "${DOCTOR_ETC_BAZELRC:-/etc/bazel.bazelrc}" ]; then bad7=1; parts="$parts; system rc ${DOCTOR_ETC_BAZELRC:-/etc/bazel.bazelrc} exists (a PATH whitelist there can cut cache hits)"; fi
if [ -n "$bad7" ]; then warn "7 host: $parts"; else ok "7 host: $parts"; fi

# --- 8 BUILD freshness ---
if [ -x "$root/tools/bzl/check-build-fresh-cheap.sh" ]; then
  fresh=$("$root/tools/bzl/check-build-fresh-cheap.sh" --warn 2>/dev/null)
  if [ "$fresh" = "BUILD: fresh" ]; then ok "8 BUILD: fresh"
  else
    pkgs=$(printf '%s\n' "$fresh" | sed -n "s/^note: BUILD.bazel may be stale for \(.*\) - run.*/\1/p" | tr '\n' ' ')
    warn "8 BUILD: MAYBE STALE for ${pkgs% } - run 'make gazelle'"
  fi
else
  ok "8 BUILD: freshness check not present"
fi

# --- 9 merge drivers ---
drv=$(git config --get merge.bazel-lockfile-merge.driver 2>/dev/null || true)
if [ -x scripts/git-merge-bazel-lock.sh ]; then
  if msg=$(scripts/git-merge-bazel-lock.sh --check 2>&1); then ok "9 merge driver: $msg"
  else fail "9 merge driver: $msg"; fi
elif [ -n "$drv" ]; then ok "9 merge driver: merge.bazel-lockfile-merge is configured"
else fail "9 merge driver: merge.bazel-lockfile-merge.driver is unset; run 'make setup-git'"; fi

line "to reproduce a failing test: bazel test <target> --sandbox_debug --test_output=streamed --nocache_test_results"
[ "$failed" -eq 0 ] || exit 1
[ -z "$strict" ] || [ "$warned" -eq 0 ] || exit 1
exit 0
