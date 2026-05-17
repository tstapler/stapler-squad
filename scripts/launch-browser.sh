#!/usr/bin/env bash
# Launch Chrome/Chromium on the session's virtual display for browser passthrough.
# Usage: launch-browser.sh [URL]
# The DISPLAY env var is set automatically by stapler-squad for sessions with VNC enabled.

set -euo pipefail

URL="${1:-about:blank}"

if [ -z "${DISPLAY:-}" ]; then
    echo "Error: DISPLAY is not set. This script must be run inside a stapler-squad session with browser passthrough enabled." >&2
    exit 1
fi

# Try google-chrome first, fall back to chromium
if command -v google-chrome &>/dev/null; then
    BROWSER="google-chrome"
elif command -v chromium &>/dev/null; then
    BROWSER="chromium"
elif command -v chromium-browser &>/dev/null; then
    BROWSER="chromium-browser"
else
    echo "Error: Could not find google-chrome or chromium. Install one of them to use browser passthrough." >&2
    exit 1
fi

exec "$BROWSER" \
    --no-sandbox \
    --disable-dev-shm-usage \
    --disable-gpu \
    --display="${DISPLAY}" \
    "$URL"
