#!/usr/bin/env bash
# Bazel host-state gauges for the weekly trend job (Task 11.4c). Prints one "name value" line per metric and a
# `::warning::` annotation on a disk alarm. Advisory only: always exits 0, like duration-trend.sh.
#   bazel_disk_alarm 0|1, bazel_cache_kb, bazel_disk_cache_kb, bazel_cache_df_pct
set -u
here=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)
alarm_script="${BAZEL_DISK_ALARM:-$here/../bzl/bazel-disk-alarm.sh}"

out=$(bash "$alarm_script" --gauges 2>&1)
rc=$?
printf '%s\n' "$out" | grep -E '^bazel_'
alarm=0; [ "$rc" -ne 0 ] && alarm=1
echo "bazel_disk_alarm $alarm"
printf '%s\n' "$out" | grep '^ALARM:' | sed 's/^/::warning::bazel-trend: /'
exit 0
