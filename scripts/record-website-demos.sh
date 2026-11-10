#!/usr/bin/env bash
# record-website-demos.sh — record the website's demo scenes (tests/e2e/website-demos: the
# legacy clips.ts plus tours/*.ts) against a seeded, isolated server, trim the page-load time,
# add the title card, encode the H.264 rendition, and write the shard's media-manifest.json.
# Used by .github/workflows/deploy-website.yml and runnable locally.
#
# This script owns only the server lifecycle and the ffmpeg steps. Scene discovery, assertions,
# transcripts and manifest rows are the recorder's (TypeScript); shard planning, manifest
# refresh and freshness are `mediatool` (Go).
#
# Usage: scripts/record-website-demos.sh [--out DIR] [--only a,b] [--smoke] [--scenario LIST|all]
#   --only      record just these scene ids (a shard's slice from `mediatool shard-plan`)
#   --smoke     record only the PR smoke subset (the dashboard-lifecycle hero scene)
#   --scenario  comma-separated seed scenarios (default: `default`, the legacy 6 sessions);
#               `all` seeds every tour scenario
#   Needs a built ./stapler-squad (SSQ_BIN overrides), ffmpeg, cwebp, rsvg-convert, go, and
#   tests/e2e's dependencies (cd tests/e2e && bun install && npx playwright install chromium).
#   Set TEST_SERVER_URL to record against an already-running server instead of starting one
#   (then seeding is yours to do).
#
# Hermetic: the server gets a free PORT, a private ssq-* tmux socket, --test-mode --test-dir and a
# per-run HOME (BUG-116: a second instance's startup cleanup can delete a live instance's shared
# ~/.stapler-squad/cdp-bins wrapper dirs; see docs/bugs/open/BUG-116*). Never the live service.
#
# Output (the shipped layout): <out>/clips/<name>.webm, .mp4 and .transcript.json per clip,
# <out>/shots/<name>.webp (shots and clip posters, converted from the recorder's PNGs),
# dom-text/*.txt and media-manifest.json (tools/promo/internal/manifest wire shape). Exit 3 = some scenes failed (the rest are usable);
# exit 4 = a hard failure (wrong theme) that must fail the shard.
set -euo pipefail

REPO_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
OUT_DIR="$REPO_ROOT/tests/e2e/website-demo-output"
BIN="${SSQ_BIN:-$REPO_ROOT/stapler-squad}"
BUDGETS="$REPO_ROOT/tools/promo/budgets.json"
CLIP_MAX_BYTES="$(jq -r .clipMaxBytes "$BUDGETS")"
IMAGE_MAX_BYTES="$(jq -r .imageMaxBytes "$BUDGETS")"
READY_TIMEOUT_S=300   # hostname detection alone can take minutes on a corporate network

ONLY=""
SCENARIOS="default"
ALL_SCENARIOS="default,dashboard,approvals,backlog,insights,notifications,automation,fanout,files,history"
SMOKE_SCENES="dashboard-lifecycle"

while [[ $# -gt 0 ]]; do
  case "$1" in
    --out)
      mkdir -p "$2"
      OUT_DIR="$(cd "$2" && pwd)"
      shift 2
      ;;
    --only) ONLY="$2"; shift 2 ;;
    --smoke) ONLY="$SMOKE_SCENES"; shift ;;
    --scenario)
      SCENARIOS="$2"
      [[ "$SCENARIOS" == "all" ]] && SCENARIOS="$ALL_SCENARIOS"
      shift 2
      ;;
    *) echo "unknown option: $1" >&2; exit 1 ;;
  esac
done

SERVER_PID=""
DEMO_DIR=""
SERVER_HOME=""
TMUX_SOCKET="ssq-website-demos-$$"
cleanup() {
  # Reap the server before rm: it may still write into DEMO_DIR, and under set -e a failed rm flips a successful run to exit 1.
  if [[ -n "$SERVER_PID" ]]; then
    kill "$SERVER_PID" 2>/dev/null || true
    for _ in $(seq 1 50); do kill -0 "$SERVER_PID" 2>/dev/null || break; sleep 0.1; done
    kill -9 "$SERVER_PID" 2>/dev/null || true
    wait "$SERVER_PID" 2>/dev/null || true
  fi
  tmux -L "$TMUX_SOCKET" kill-server 2>/dev/null || true
  if [[ -n "$SERVER_HOME" ]]; then rm -rf "$SERVER_HOME" 2>/dev/null || true; fi
  if [[ -n "$DEMO_DIR" ]]; then rm -rf "$DEMO_DIR" 2>/dev/null || rm -rf "$DEMO_DIR" || echo "warning: could not remove $DEMO_DIR" >&2; fi
  return 0 # a false `[[ ]] &&` as the last command would otherwise become the exit status (bash 3.2)
}
trap cleanup EXIT

# One reference instant for the seeder and the recorder's frozen page clock.
DEMO_REF_TIME="$(date -u +%Y-%m-%dT%H:%M:%SZ)"
export DEMO_REF_TIME
export PROMO_COMMIT="${GITHUB_SHA:-$(git -C "$REPO_ROOT" rev-parse HEAD)}"

if [[ -z "${TEST_SERVER_URL:-}" ]]; then
  [[ -x "$BIN" ]] || { echo "❌  $BIN not found; build it first (make build)" >&2; exit 1; }
  # Session rows print the work dir under DEMO_DIR; a bare `mktemp -d` name (/tmp/tmp.XXXX) is what termcheck's leak scan flags.
  DEMO_DIR="$(mktemp -d "${TMPDIR:-/tmp}/ssq-demo-XXXXXX")"
  SERVER_HOME="$(mktemp -d)"
  (cd "$REPO_ROOT" && go run ./tests/demo/seed --scenario "$SCENARIOS" --ref "$DEMO_REF_TIME" "$DEMO_DIR")
  PORT="$(python3 -c 'import socket; s=socket.socket(); s.bind(("127.0.0.1",0)); print(s.getsockname()[1])')"
  # Private tmux socket so this never touches a developer's live sessions.
  HOME="$SERVER_HOME" PORT="$PORT" STAPLER_SQUAD_TMUX_SOCKET="$TMUX_SOCKET" STAPLER_SQUAD_USE_CONTROL_MODE=false \
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
  # Second seeding stage: flags, approval rules, backlog items, notifications over RPC.
  if [[ "$SCENARIOS" != "default" ]]; then
    mkdir -p "$DEMO_DIR/work"
    (cd "$REPO_ROOT" && go run ./tests/demo/seed rpc --scenario "$SCENARIOS" --ref "$DEMO_REF_TIME" \
      --url "$TEST_SERVER_URL" --workdir "$DEMO_DIR/work")
  fi
fi

rm -rf "$OUT_DIR"/clips "$OUT_DIR"/*.webm "$OUT_DIR"/*.mp4 "$OUT_DIR"/*.meta.json "$OUT_DIR"/*.transcript.json \
  "$OUT_DIR"/shots "$OUT_DIR"/dom-text "$OUT_DIR"/media-manifest.json "$OUT_DIR"/.manifest-parts \
  "$OUT_DIR"/.started-at "$OUT_DIR"/.fatal-*.txt
# A failing clip must not discard the others: keep going, then report it.
record_status=0
(cd "$REPO_ROOT/tests/e2e" && WEBSITE_DEMO_OUT="$OUT_DIR" WEBSITE_DEMO_ONLY="$ONLY" \
  npx playwright test -c playwright.website-demos.config.ts) || record_status=$?

# A wrong theme is a content defect, not flake: fail the shard whatever freshnessEnforce says.
fatal=("$OUT_DIR"/.fatal-*.txt)
if [[ -e "${fatal[0]}" ]]; then
  echo "❌  hard failure, not shipping this shard:" >&2
  cat "${fatal[@]}" >&2
  exit 4
fi

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
  # H.264 rendition for Safari/iOS (Cross-cutting 7e): audio-less, raise CRF until it fits the clip budget.
  local crf
  for crf in 30 34 38; do
    ffmpeg -v error -y -i "$clip" -c:v libx264 -pix_fmt yuv420p -movflags +faststart -crf "$crf" -preset slow -an \
      "$OUT_DIR/$name.mp4"
    [[ "$(wc -c <"$OUT_DIR/$name.mp4")" -le "$CLIP_MAX_BYTES" ]] && break
  done
}
# One clip failing to post-process must not discard the others.
for clip in "${clips[@]}"; do
  if ! (process_clip "$clip"); then
    echo "::warning::post-processing $(basename "$clip") failed; dropping it" >&2
    rm -f "$clip" "${clip%.webm}.mp4"
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
    rm -f "$clip" "${clip%.webm}.mp4"
  fi
done
[[ ${#good[@]} -gt 0 ]] || { echo "❌  no playable clips" >&2; exit 1; }
echo "✅  Recorded ${#good[@]} clip(s) in $OUT_DIR:"
(cd "$OUT_DIR" && shasum -a 256 -- *.webm *.mp4)

# Shipped layout: clips/ holds the renditions and transcript sidecars; shots become WebP within the image budget.
mkdir -p "$OUT_DIR/clips"
mv "$OUT_DIR"/*.webm "$OUT_DIR"/*.mp4 "$OUT_DIR"/*.transcript.json "$OUT_DIR/clips/"
for png in "$OUT_DIR"/shots/*.png; do
  webp="${png%.png}.webp"
  for q in 82 70 58 46; do
    cwebp -quiet -q "$q" -m 6 "$png" -o "$webp"
    [[ "$(wc -c <"$webp")" -le "$IMAGE_MAX_BYTES" ]] && break
  done
  rm -f "$png"
done

# Re-measure what actually ships (post-processed bytes and hashes, mp4 rows) in the manifest.
(cd "$REPO_ROOT" && go run ./tools/promo/cmd/mediatool refresh -dir "$OUT_DIR")

# Partial success: the clips above are still usable (the workflow uploads them
# even when this fails), but exit non-zero so the failure is reported, not hidden.
if [[ "$record_status" -ne 0 ]]; then
  echo "::warning::some clips failed (exit $record_status); shipping the ${#good[@]} that worked" >&2
  exit 3
fi
