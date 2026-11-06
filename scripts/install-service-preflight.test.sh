#!/bin/sh
# shellcheck shell=sh
#
# install-service-preflight.test.sh — regression test for the Story 8.5
# (project_plans/web-app-bun-build) `install-service-preflight` Makefile
# target: `make install-service` must refuse to build/restart when the
# deployed unit lacks --tmux-keep-server, since that restart would destroy
# every live tmux session (docs/explanation/tmux-keep-server-on-restart.md).
#
# The guard is written as Make `$(shell ...)`/`$(error ...)` expansions
# rather than a shell conditional inside the recipe body, specifically so it
# fires during `make -n` (dry-run) expansion too — Make expands recipe text
# (including $(error)) before printing it, even under -n, it just skips
# *executing* the printed command. That lets this test exercise the real
# guard logic without ever letting `install-service` build or touch the
# live service.
#
# Three cases (Task 8.5d's AC):
#   1. flag present                              -> make -n exits 0
#   2. flag absent, no ACCEPT_SESSION_LOSS        -> make -n exits non-zero,
#                                                     naming the doc
#   3. flag absent, ACCEPT_SESSION_LOSS=1         -> make -n exits 0
#
# Usage: sh scripts/install-service-preflight.test.sh
set -eu

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"

TMP_DIR="$(mktemp -d)"
trap 'rm -rf "$TMP_DIR"' EXIT

NO_FLAG_UNIT="$TMP_DIR/fake-no-flag.service"
WITH_FLAG_UNIT="$TMP_DIR/fake-with-flag.service"

cat > "$NO_FLAG_UNIT" <<'EOF'
[Service]
ExecStart=/opt/stapler-squad/stapler-squad --remote-access
EOF

cat > "$WITH_FLAG_UNIT" <<'EOF'
[Service]
ExecStart=/opt/stapler-squad/stapler-squad --remote-access --tmux-keep-server
EOF

ok=1

run_preflight() {
    # $1=unit file  $2...=extra make vars
    unit="$1"
    shift
    (cd "$REPO_ROOT" && make -n install-service-preflight UNAME_S=Linux \
        "INSTALL_SERVICE_LINUX_UNIT=$unit" "$@")
}

echo "=== case 1: --tmux-keep-server present ==="
if out="$(run_preflight "$WITH_FLAG_UNIT" 2>&1)"; then
    echo "PASS: exits 0 when the flag is present"
else
    echo "FAIL: expected exit 0 when the flag is present; got:" >&2
    echo "$out" >&2
    ok=0
fi

echo "=== case 2: --tmux-keep-server absent, no override ==="
if out="$(run_preflight "$NO_FLAG_UNIT" 2>&1)"; then
    echo "FAIL: expected non-zero exit when the flag is absent and ACCEPT_SESSION_LOSS is unset; got exit 0:" >&2
    echo "$out" >&2
    ok=0
else
    if echo "$out" | grep -q 'docs/explanation/tmux-keep-server-on-restart.md'; then
        echo "PASS: exits non-zero and names docs/explanation/tmux-keep-server-on-restart.md"
    else
        echo "FAIL: exited non-zero but did not name docs/explanation/tmux-keep-server-on-restart.md; got:" >&2
        echo "$out" >&2
        ok=0
    fi
fi

echo "=== case 3: --tmux-keep-server absent, ACCEPT_SESSION_LOSS=1 ==="
if out="$(run_preflight "$NO_FLAG_UNIT" "ACCEPT_SESSION_LOSS=1" 2>&1)"; then
    echo "PASS: exits 0 when ACCEPT_SESSION_LOSS=1 overrides the missing flag"
else
    echo "FAIL: expected exit 0 with ACCEPT_SESSION_LOSS=1; got:" >&2
    echo "$out" >&2
    ok=0
fi

echo "=== sanity: unrelated targets (lint) are not gated by this guard ==="
if out="$(cd "$REPO_ROOT" && make -n lint "INSTALL_SERVICE_LINUX_UNIT=$NO_FLAG_UNIT" UNAME_S=Linux 2>&1)"; then
    echo "PASS: make -n lint is unaffected by a missing --tmux-keep-server"
else
    echo "FAIL: make -n lint unexpectedly failed:" >&2
    echo "$out" >&2
    ok=0
fi

if [ "$ok" = "0" ]; then
    echo "install-service-preflight.test.sh: FAILED" >&2
    exit 1
fi

echo "install-service-preflight.test.sh: all cases PASSED"
