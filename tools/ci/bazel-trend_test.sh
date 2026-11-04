#!/usr/bin/env bash
# Prints bazel-trend.sh output for a seeded breach and a clean run; exits 1 if either misbehaves.
set -u
here=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)
T=$(mktemp -d "${TMPDIR:-/tmp}/bazel-trend-test.XXXXXX")
trap 'rm -rf "$T"' EXIT
printf '#!/usr/bin/env bash\necho "bazel_cache_kb 5"\necho "bazel_disk_cache_kb 4"\necho "bazel_cache_df_pct 3"\n' > "$T/clean"
printf '#!/usr/bin/env bash\necho "ALARM: /x/.cache/bazel du 160G"\necho "bazel_cache_kb 167772160"\necho "bazel_disk_cache_kb 4"\necho "bazel_cache_df_pct 3"\nexit 1\n' > "$T/breach"
fail=0
clean=$(BAZEL_DISK_ALARM="$T/clean" bash "$here/bazel-trend.sh")
breach=$(BAZEL_DISK_ALARM="$T/breach" bash "$here/bazel-trend.sh")
echo "$clean"; echo "$breach"
echo "$clean" | grep -qx 'bazel_disk_alarm 0' || { echo "FAIL: clean run did not report alarm 0" >&2; fail=1; }
echo "$breach" | grep -qx 'bazel_disk_alarm 1' || { echo "FAIL: breach did not report alarm 1" >&2; fail=1; }
echo "$breach" | grep -q '^::warning::bazel-trend: ALARM: /x/.cache/bazel du 160G' || { echo "FAIL: breach did not annotate" >&2; fail=1; }
[ "$(echo "$clean" | grep -c '^bazel_.*[0-9]$')" -ge 4 ] || { echo "FAIL: gauges missing" >&2; fail=1; }
exit "$fail"
