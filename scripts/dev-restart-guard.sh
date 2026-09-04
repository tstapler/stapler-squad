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
# excluded by PID, not by pattern.
launchd_pid="$(launchctl list 2>/dev/null | awk '$3=="com.stapler-squad"{print $1}')"

for pid in $(pgrep -f "$STAPLER_PATTERN" 2>/dev/null || true); do
    if [ "$pid" = "$launchd_pid" ]; then
        continue
    fi
    kill "$pid" 2>/dev/null || true
done

wait_for_port_release "$@"
