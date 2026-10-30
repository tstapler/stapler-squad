#!/bin/sh
# shellcheck shell=sh
#
# Regression test: `ssq-hook-handler stop` emits a notification unless the session's
# settings.local.json carries the stapler HTTP Stop hook (the endpoint then owns it).
#
# Usage: sh scripts/ssq-hook-handler-stop.test.sh
set -eu

HERE="$(cd "$(dirname "$0")" && pwd)"
TMP="$(mktemp -d)"
trap 'rm -rf "$TMP"' EXIT

cat > "$TMP/notify" <<EOF
#!/bin/sh
echo called >> "$TMP/calls"
EOF
chmod +x "$TMP/notify"

run_stop() {
    : > "$TMP/calls"
    printf '{"cwd":"%s","last_assistant_message":"Done."}' "$1" |
        CS_NOTIFY="$TMP/notify" CS_SESSION_ID=demo bash "$HERE/ssq-hook-handler" stop >/dev/null 2>&1
    wc -l < "$TMP/calls" | tr -d ' '
}

fail() { echo "FAIL: $1" >&2; exit 1; }

mkdir -p "$TMP/plain" "$TMP/wired/.claude"
echo '{}' > "$TMP/plain/settings.json"
echo '{"hooks":{"Stop":[{"hooks":[{"type":"command","command":"curl -X POST '"'"'http://localhost:8543/api/hooks/stop'"'"'"}]}]}}' > "$TMP/wired/.claude/settings.local.json"

[ "$(run_stop "$TMP/plain")" = 1 ] || fail "no endpoint hook: script must still notify"
[ "$(run_stop "$TMP/wired")" = 0 ] || fail "endpoint hook present: script must stay silent"
[ "$(SSQ_STOP_ENDPOINT=on run_stop "$TMP/plain")" = 0 ] || fail "SSQ_STOP_ENDPOINT=on must silence"
[ "$(SSQ_STOP_ENDPOINT=off run_stop "$TMP/wired")" = 1 ] || fail "SSQ_STOP_ENDPOINT=off must notify"
echo "ok"
