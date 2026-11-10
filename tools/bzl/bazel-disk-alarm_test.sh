#!/usr/bin/env bash
# Stubbed du/df: the alarm exits 1 above each threshold and 0 below.
set -u
here=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)
T=$(mktemp -d "${TMPDIR:-/tmp}/disk-alarm-test.XXXXXX")
trap 'rm -rf "$T"' EXIT
mkdir -p "$T/cache" "$T/disk" "$T/remote"
failures=0
# stub tool: prints $STUB_<name> for the path's basename, so each case sets numbers without touching the disk
cat > "$T/du" <<'S'
#!/usr/bin/env bash
case "$2" in */cache) echo "${STUB_CACHE_KB:-1}	$2" ;; */disk) echo "${STUB_DISK_KB:-1}	$2" ;; esac
S
cat > "$T/df" <<'S'
#!/usr/bin/env bash
case "$2" in */remote) p=${STUB_REMOTE_PCT:-10} ;; *) p=${STUB_CACHE_PCT:-10} ;; esac
printf 'Filesystem 1024-blocks Used Available Capacity Mounted\nx 100 1 1 %s%% /\n' "$p"
S
chmod +x "$T/du" "$T/df"
GB=$((1024 * 1024))

case_() { # label want-exit want-text [env...]
  local label=$1 want=$2 text=$3 out rc; shift 3
  out=$(env DU="$T/du" DF="$T/df" BAZEL_CACHE_DIR="$T/cache" DISK_CACHE="$T/disk" BAZEL_REMOTE_DATA_DIR="$T/remote" "$@" bash "$here/bazel-disk-alarm.sh")
  rc=$?
  if [ "$rc" = "$want" ] && { [ -z "$text" ] || echo "$out" | grep -q -- "$text"; }; then echo "ok   - $label"; else echo "FAIL - $label (rc=$rc): $out" >&2; failures=$((failures + 1)); fi
}

case_ "all below thresholds" 0 "" STUB_CACHE_KB=$((10 * GB)) STUB_DISK_KB=$((50 * GB))
case_ "cache above 150 GB names the path" 1 "ALARM: $T/cache du" STUB_CACHE_KB=$((151 * GB))
case_ "disk cache above its 100 GB cap" 1 "ALARM: $T/disk du" STUB_DISK_KB=$((101 * GB))
case_ "cache filesystem above 85 percent" 1 "ALARM: $T/cache df 86%" STUB_CACHE_PCT=86
case_ "remote data dir above 85 percent" 1 "ALARM: $T/remote df 90%" STUB_REMOTE_PCT=90
case_ "exactly at the limits is fine" 0 "" STUB_CACHE_KB=$((150 * GB)) STUB_DISK_KB=$((100 * GB)) STUB_CACHE_PCT=85
exit $((failures > 0))
