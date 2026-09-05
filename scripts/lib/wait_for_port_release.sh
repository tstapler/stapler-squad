#!/bin/sh
# shellcheck shell=sh
#
# wait_for_port_release — the single implementation of "poll until these TCP
# ports have no LISTENer", sourced by both scripts/install-service.sh and
# scripts/dev-restart-guard.sh.
#
# Root incident (2026-09-04): install-service.sh already had this function
# (added by 1c2774d08 after a real crash-loop incident — see its own doc
# comment for the "launchctl bootout returns before the old process
# actually releases its sockets" race). The Makefile's restart-web/web-dev
# targets never reused it and instead did a blind `sleep 1`, which raced
# the same teardown and produced the identical "address already in use"
# crash loop when combined with a pkill pattern that also matched the
# installed launchd service. Two implementations of the same precondition
# meant the second one never got the fix the first one was written for.
#
# Usage: wait_for_port_release PORT [PORT...]
# Ports that are empty/unset are skipped. Proceeds with a warning on
# timeout (10s) rather than blocking forever — a genuinely stuck old
# process needs a human, not a longer sleep. Always returns 0 (even on
# timeout) so callers running under `set -e` don't abort on the very
# "proceed anyway" path this function documents — a bare timeout is not
# a caller error.
wait_for_port_release() {
    wfpr_max_ticks=20  # 20 * 0.5s = 10s
    wfpr_tick=0
    while [ "$wfpr_tick" -lt "$wfpr_max_ticks" ]; do
        wfpr_busy=0
        for wfpr_port in "$@"; do
            [ -n "$wfpr_port" ] || continue
            if lsof -nP -iTCP:"$wfpr_port" -sTCP:LISTEN >/dev/null 2>&1; then
                wfpr_busy=1
                break
            fi
        done
        [ "$wfpr_busy" = "0" ] && return 0
        sleep 0.5
        wfpr_tick=$((wfpr_tick + 1))
    done
    echo "Old process still holding a port after $((wfpr_max_ticks / 2))s — starting anyway." >&2
    return 0
}
