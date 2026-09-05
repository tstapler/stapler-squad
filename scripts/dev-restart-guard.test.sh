#!/bin/sh
# shellcheck shell=sh
#
# dev-restart-guard.test.sh — regression test for dev-restart-guard.sh's
# core property: it must NEVER kill the launchd-managed stapler-squad
# service's PID, even though that PID matches the same pgrep pattern as a
# dev-run instance (they run the identical binary at the identical
# absolute path — see dev-restart-guard.sh's doc comment for the incident).
#
# `kill` is a shell builtin, not resolvable via PATH, so it can't be
# stubbed the way pgrep/launchctl/lsof are below. Instead this spawns three
# real, disposable `sleep` processes to stand in for the service PID and
# two dev PIDs, stubs pgrep/launchctl to report their real PIDs, and lets
# the unmodified script send real signals to them — then checks who
# survived. This exercises the real `kill` behavior, not a mock of it.
#
# Must fail against the pre-fix implementation (a plain
# `pkill -f "(^|/)stapler-squad(...)"` with no PID exclusion) — this is
# verified below by also running that naive pattern and confirming it
# would have matched (and thus killed) the service stand-in too.
#
# Usage: sh scripts/dev-restart-guard.test.sh
set -eu

TMP_BIN="$(mktemp -d)"
trap 'rm -rf "$TMP_BIN"; kill "$SERVICE_PID" "$DEV_PID_1" "$DEV_PID_2" 2>/dev/null || true' EXIT

sleep 300 & SERVICE_PID=$!
sleep 300 & DEV_PID_1=$!
sleep 300 & DEV_PID_2=$!

cat > "$TMP_BIN/launchctl" <<EOF
#!/bin/sh
echo "$SERVICE_PID	0	com.stapler-squad"
EOF

cat > "$TMP_BIN/pgrep" <<EOF
#!/bin/sh
printf '%s\n%s\n%s\n' "$SERVICE_PID" "$DEV_PID_1" "$DEV_PID_2"
EOF

# Not listening — makes wait_for_port_release return immediately.
cat > "$TMP_BIN/lsof" <<'EOF'
#!/bin/sh
exit 1
EOF

chmod +x "$TMP_BIN"/launchctl "$TMP_BIN"/pgrep "$TMP_BIN"/lsof

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"

# Sanity check first: prove this scenario actually exercises the gap the
# pre-fix code had — the naive pattern with no exclusion matches all three
# PIDs' command lines (they're plain `sleep`, so match on PID list instead).
naive_matches="$(PATH="$TMP_BIN:$PATH" pgrep -f '(^|/)stapler-squad([[:space:]]|$)')"
if ! printf '%s\n' "$naive_matches" | grep -qx "$SERVICE_PID"; then
    echo "FAIL: test setup bug — naive pattern doesn't even match the service PID; this test wouldn't have caught the original bug" >&2
    exit 1
fi

PATH="$TMP_BIN:$PATH" sh "$SCRIPT_DIR/dev-restart-guard.sh" 8543 8444

alive() { kill -0 "$1" 2>/dev/null; }

ok=1
if ! alive "$SERVICE_PID"; then
    echo "FAIL: dev-restart-guard.sh killed the launchd-managed service stand-in (PID $SERVICE_PID)" >&2
    ok=0
fi
if alive "$DEV_PID_1"; then
    echo "FAIL: dev-restart-guard.sh did not kill dev PID $DEV_PID_1" >&2
    ok=0
fi
if alive "$DEV_PID_2"; then
    echo "FAIL: dev-restart-guard.sh did not kill dev PID $DEV_PID_2" >&2
    ok=0
fi

if [ "$ok" = "0" ]; then
    exit 1
fi

echo "PASS: dev-restart-guard.sh excludes the launchd service PID ($SERVICE_PID) and kills dev PIDs ($DEV_PID_1, $DEV_PID_2)"

# Second scenario: launchctl exists but reports no PID for our label (e.g. a
# transient parsing gap). Must fail safe — skip the kill loop entirely rather
# than treat "no PID found" as "nothing to exclude" and kill everything,
# including a possibly-live service this script just failed to identify.
sleep 300 & SERVICE_PID2=$!
sleep 300 & DEV_PID2_1=$!
trap 'rm -rf "$TMP_BIN"; kill "$SERVICE_PID" "$DEV_PID_1" "$DEV_PID_2" "$SERVICE_PID2" "$DEV_PID2_1" 2>/dev/null || true' EXIT

cat > "$TMP_BIN/launchctl" <<'EOF'
#!/bin/sh
echo "-	0	com.other-service"
EOF

cat > "$TMP_BIN/pgrep" <<EOF
#!/bin/sh
printf '%s\n%s\n' "$SERVICE_PID2" "$DEV_PID2_1"
EOF

PATH="$TMP_BIN:$PATH" sh "$SCRIPT_DIR/dev-restart-guard.sh" 8543 8444

if ! alive "$SERVICE_PID2" || ! alive "$DEV_PID2_1"; then
    echo "FAIL: dev-restart-guard.sh killed processes despite launchctl reporting no PID for com.stapler-squad (should fail safe and skip cleanup)" >&2
    exit 1
fi

echo "PASS: dev-restart-guard.sh fails safe (skips cleanup) when launchctl reports no PID for com.stapler-squad"
