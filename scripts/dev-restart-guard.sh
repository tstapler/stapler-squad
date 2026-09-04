#!/bin/sh
# shellcheck shell=bash
#
# dev-restart-guard.sh — stop stapler-squad dev processes without touching
# the launchd-managed service, then wait for the ports they held to
# actually clear before the caller starts a new one.
#
# Root incident (2026-09-04): `make restart-web`'s pkill pattern was widened
# by fa2926e8d to also match the installed launchd service's absolute-path
# invocation ("match service-installed stapler-squad in restart pkill") —
# a plain dev `make restart-web` now killed the live service as collateral
# damage, since a dev run and the installed service execute the exact same
# binary at the exact same absolute path (only the launching parent and
# flags differ) and can't be told apart by path alone. Then the target's
# blind `sleep 1` (instead of the port-release poll that
# scripts/install-service.sh already had, for the same underlying reason —
# see wait_for_port_release's doc comment) raced the killed process's
# teardown and hit "address already in use", triggering launchd's
# KeepAlive crash-restart loop, which redid the process's expensive
# startup work (a full git-history walk) on every crash.
#
# Usage: dev-restart-guard.sh PORT [PORT...]
set -eu

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
# shellcheck source=scripts/lib/wait_for_port_release.sh
. "$SCRIPT_DIR/lib/wait_for_port_release.sh"

# Matches any stapler-squad invocation regardless of path — deliberately
# broad (mirrors fa2926e8d's intent of catching the service-installed
# absolute path too) because the exclusion below, not the pattern, is what
# keeps this safe.
STAPLER_PATTERN='(^|/)stapler-squad([[:space:]]|$)'

# The launchd job runs the identical binary/path as a dev run, so it must be
# excluded by PID, not by pattern. Re-resolved on every loop iteration
# (rather than captured once up front) because launchd can restart the
# service mid-loop and hand it a new PID — a stale snapshot would fail to
# exclude the new one and kill the live service.
#
# If launchctl exists but reports no PID for the service, fail safe: skip
# the kill loop entirely rather than risk killing an unrecognized live
# service. On platforms without launchctl (Linux), there's no service to
# protect, so proceed with an empty exclusion set as before.
get_launchd_pid() {
    launchctl list 2>/dev/null | awk '$3=="com.stapler-squad"{print $1}'
}

if command -v launchctl >/dev/null 2>&1; then
    if [ -z "$(get_launchd_pid)" ]; then
        echo "warning: launchctl found but reported no PID for com.stapler-squad — skipping process cleanup to avoid killing an unrecognized service" >&2
        wait_for_port_release "$@"
        exit 0
    fi
fi

for pid in $(pgrep -f "$STAPLER_PATTERN" 2>/dev/null || true); do
    if [ "$pid" = "$(get_launchd_pid)" ]; then
        continue
    fi
    kill "$pid" 2>/dev/null || true
done

wait_for_port_release "$@"
