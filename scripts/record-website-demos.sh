#!/usr/bin/env bash
# record-website-demos.sh — record the website's demo clips (tests/e2e/website-demos/clips.ts)
# against a seeded, isolated server, trim the page-load time, add the title card, and write
# <out>/<name>.webm. Used by .github/workflows/website-demo-publish.yml and runnable locally.
#
# Usage: scripts/record-website-demos.sh [--out DIR]
#   Needs a built ./stapler-squad (SSQ_BIN overrides), ffmpeg, rsvg-convert, and
#   tests/e2e's dependencies (cd tests/e2e && bun install && npx playwright install chromium).
#   Set TEST_SERVER_URL to record against an already-running server instead of starting one.
#
# Output: <out>/<name>.webm for each clip, plus a sha256 line per clip on stdout.
set -euo pipefail

REPO_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
OUT_DIR="$REPO_ROOT/tests/e2e/website-demo-output"
BIN="${SSQ_BIN:-$REPO_ROOT/stapler-squad}"
READY_TIMEOUT_S=300   # hostname detection alone can take minutes on a corporate network

while [[ $# -gt 0 ]]; do
  case "$1" in
    --out) OUT_DIR="$(cd "$(dirname "$2")" && pwd)/$(basename "$2")"; shift 2 ;;
    *) echo "unknown option: $1" >&2; exit 1 ;;
  esac
done

SERVER_PID=""
DEMO_DIR=""
TMUX_SOCKET="ssq-website-demos-$$"
cleanup() {
  [[ -n "$SERVER_PID" ]] && kill "$SERVER_PID" 2>/dev/null || true
  tmux -L "$TMUX_SOCKET" kill-server 2>/dev/null || true
  [[ -n "$DEMO_DIR" ]] && rm -rf "$DEMO_DIR"
}
trap cleanup EXIT

if [[ -z "${TEST_SERVER_URL:-}" ]]; then
  [[ -x "$BIN" ]] || { echo "❌  $BIN not found; build it first (make build)" >&2; exit 1; }
  DEMO_DIR="$(mktemp -d)"
  (cd "$REPO_ROOT" && go run ./tests/demo/seed "$DEMO_DIR" 6)
  PORT="$(python3 -c 'import socket; s=socket.socket(); s.bind(("127.0.0.1",0)); print(s.getsockname()[1])')"
  # Private tmux socket so this never touches a developer's live sessions.
  PORT="$PORT" STAPLER_SQUAD_TMUX_SOCKET="$TMUX_SOCKET" STAPLER_SQUAD_USE_CONTROL_MODE=false \
    "$BIN" --test-mode --test-dir "$DEMO_DIR" --tmux-keep-server >"$DEMO_DIR/server.log" 2>&1 &
  SERVER_PID=$!
  export TEST_SERVER_URL="http://localhost:$PORT"
  echo "⏳  Waiting for $TEST_SERVER_URL (up to ${READY_TIMEOUT_S}s) …"
  timeout "$READY_TIMEOUT_S" bash -c "until curl -sf '$TEST_SERVER_URL/' >/dev/null; do sleep 2; done" \
    || { echo "❌  server never became ready; log tail:" >&2; tail -20 "$DEMO_DIR/server.log" >&2; exit 1; }
fi

rm -rf "$OUT_DIR"
(cd "$REPO_ROOT/tests/e2e" && WEBSITE_DEMO_OUT="$OUT_DIR" \
  npx playwright test -c playwright.website-demos.config.ts)

shopt -s nullglob
clips=("$OUT_DIR"/*.webm)
[[ ${#clips[@]} -gt 0 ]] || { echo "❌  no clips were recorded" >&2; exit 1; }

for clip in "${clips[@]}"; do
  name="$(basename "$clip" .webm)"
  meta="$OUT_DIR/$name.meta.json"
  title="$(jq -r .title "$meta")"
  fps="$(ffprobe -v error -select_streams v:0 -show_entries stream=r_frame_rate -of csv=p=0 "$clip")"
  # Frame-count trim from the measured load time (never a sleep), per demo-post-process.sh.
  trim_frames="$(awk -v ms="$(jq -r .trimStartMs "$meta")" -v fps="$fps" 'BEGIN{split(fps,f,"/"); printf "%d", ms/1000*f[1]/(f[2]?f[2]:1)}')"
  "$REPO_ROOT/scripts/demo-post-process.sh" --no-gif --title-card "$title" \
    --trim-start-frames "$trim_frames" "$clip"
done
rm -f "$OUT_DIR"/*.meta.json

echo "✅  Recorded ${#clips[@]} clip(s) in $OUT_DIR:"
(cd "$OUT_DIR" && shasum -a 256 -- *.webm)
