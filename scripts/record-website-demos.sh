#!/usr/bin/env bash
# record-website-demos.sh — record the website's demo clips (tests/e2e/website-demos/clips.ts)
# against a seeded, isolated server, trim the page-load time, add the title card, and write
# <out>/<name>.webm. Used by .github/workflows/deploy-website.yml and runnable locally.
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
    --out)
      mkdir -p "$2"
      OUT_DIR="$(cd "$2" && pwd)"
      shift 2
      ;;
    *) echo "unknown option: $1" >&2; exit 1 ;;
  esac
done

SERVER_PID=""
DEMO_DIR=""
TMUX_SOCKET="ssq-website-demos-$$"
cleanup() {
  # Reap the server before rm: it may still write into DEMO_DIR, and under set -e a failed rm flips a successful run to exit 1.
  if [[ -n "$SERVER_PID" ]]; then kill "$SERVER_PID" 2>/dev/null || true; wait "$SERVER_PID" 2>/dev/null || true; fi
  tmux -L "$TMUX_SOCKET" kill-server 2>/dev/null || true
  if [[ -n "$DEMO_DIR" ]]; then rm -rf "$DEMO_DIR" 2>/dev/null || rm -rf "$DEMO_DIR" 2>/dev/null || true; fi
  return 0 # a false `[[ ]] &&` as the last command would otherwise become the exit status (bash 3.2)
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
  deadline=$((SECONDS + READY_TIMEOUT_S))
  until curl -sf "$TEST_SERVER_URL/" >/dev/null; do
    if ! kill -0 "$SERVER_PID" 2>/dev/null || [[ $SECONDS -ge $deadline ]]; then
      echo "❌  server never became ready; log tail:" >&2
      tail -20 "$DEMO_DIR/server.log" >&2
      exit 1
    fi
    sleep 2
  done
fi

rm -f "$OUT_DIR"/*.webm "$OUT_DIR"/*.meta.json
# A failing clip must not discard the others: keep going, then report it.
record_status=0
(cd "$REPO_ROOT/tests/e2e" && WEBSITE_DEMO_OUT="$OUT_DIR" \
  npx playwright test -c playwright.website-demos.config.ts) || record_status=$?

shopt -s nullglob
clips=("$OUT_DIR"/*.webm)
[[ ${#clips[@]} -gt 0 ]] || { echo "❌  no clips were recorded" >&2; exit 1; }

process_clip() {
  local clip="$1" name meta title fps trim_frames
  name="$(basename "$clip" .webm)"
  meta="$OUT_DIR/$name.meta.json"
  title="$(jq -r .title "$meta")" || return 1
  fps="$(ffprobe -v error -select_streams v:0 -show_entries stream=avg_frame_rate -of csv=p=0 "$clip")" || return 1
  # Frame-count trim from the measured load time (never a sleep), per demo-post-process.sh.
  trim_frames="$(awk -v ms="$(jq -r .trimStartMs "$meta")" -v fps="$fps" 'BEGIN{split(fps,f,"/"); printf "%d", ms/1000*f[1]/(f[2]?f[2]:1)}')" || return 1
  "$REPO_ROOT/scripts/demo-post-process.sh" --no-gif --title-card "$title" \
    --trim-start-frames "$trim_frames" "$clip"
}
# One clip failing to post-process must not discard the others.
for clip in "${clips[@]}"; do
  if ! (process_clip "$clip"); then
    echo "::warning::post-processing $(basename "$clip") failed; dropping it" >&2
    rm -f "$clip"
    record_status=1
  fi
done
rm -f "$OUT_DIR"/*.meta.json

# A zero-length or truncated file would otherwise ship; drop it so the card keeps its screenshot.
good=()
for clip in "$OUT_DIR"/*.webm; do
  duration="$(ffprobe -v error -show_entries format=duration -of csv=p=0 "$clip" 2>/dev/null || true)"
  if awk -v d="${duration:-0}" 'BEGIN{exit !(d > 2)}'; then
    good+=("$clip")
  else
    echo "::warning::dropping $(basename "$clip"): unplayable or under 2s (duration '${duration:-none}')" >&2
    rm -f "$clip"
  fi
done
[[ ${#good[@]} -gt 0 ]] || { echo "❌  no playable clips" >&2; exit 1; }
echo "✅  Recorded ${#good[@]} clip(s) in $OUT_DIR:"
(cd "$OUT_DIR" && shasum -a 256 -- *.webm)

# Partial success: the clips above are still usable (the workflow uploads them
# even when this fails), but exit non-zero so the failure is reported, not hidden.
if [[ "$record_status" -ne 0 ]]; then
  echo "::warning::some clips failed (exit $record_status); shipping the ${#good[@]} that worked" >&2
  exit 3
fi
