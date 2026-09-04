#!/bin/sh
# shellcheck shell=bash
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
# process needs a human, not a longer sleep.
wait_for_port_release() {
    max_ticks=20  # 20 * 0.5s = 10s
    tick=0
    while [ "$tick" -lt "$max_ticks" ]; do
        busy=0
        for port in "$@"; do
            [ -n "$port" ] || continue
            if lsof -nP -iTCP:"$port" -sTCP:LISTEN >/dev/null 2>&1; then
                busy=1
                break
            fi
        done
        [ "$busy" = "0" ] && return 0
        sleep 0.5
        tick=$((tick + 1))
    done
    echo "Old process still holding a port after $((max_ticks / 2))s — starting anyway." >&2
    return 1
}
