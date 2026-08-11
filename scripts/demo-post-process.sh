#!/usr/bin/env bash
# demo-post-process.sh — Add browser chrome frame to assets/demo.webm and export a GIF preview.
#
# Usage: ./scripts/demo-post-process.sh [input.webm]
#   Defaults to assets/demo.webm if no argument is given.
#   Overwrites the input file in-place and writes assets/demo.gif alongside it.

set -euo pipefail

INPUT="${1:-assets/demo.webm}"
REPO_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
INPUT="$REPO_ROOT/$INPUT"
GIF_OUT="$REPO_ROOT/assets/demo.gif"

# ── Preflight checks ───────────────────────────────────────────────────────
if ! command -v ffmpeg &>/dev/null; then
  echo "❌  ffmpeg not found. Install with: brew install ffmpeg" >&2
  exit 1
fi

if [[ ! -f "$INPUT" ]]; then
  echo "❌  Input not found: $INPUT" >&2
  exit 1
fi

echo "🎬  Post-processing $INPUT"

# ── Detect source dimensions ───────────────────────────────────────────────
WIDTH=$(ffprobe -v error -select_streams v:0 \
  -show_entries stream=width -of csv=p=0 "$INPUT")
HEIGHT=$(ffprobe -v error -select_streams v:0 \
  -show_entries stream=height -of csv=p=0 "$INPUT")
echo "    Source: ${WIDTH}×${HEIGHT}"

# ── Browser chrome layout constants ───────────────────────────────────────
CHROME_H=40          # height of the top browser-chrome bar in pixels
BG_COLOR="1e293b"    # dark slate background (#1e293b)
URL_BAR_COLOR="2d3f55"  # slightly lighter for URL bar

NEW_H=$((HEIGHT + CHROME_H))

# Traffic-light button centres (x) and shared y
TL_Y=$((CHROME_H / 2))   # vertical centre of chrome bar
TL_R=6                    # radius of each button circle (drawn as filled box)
RED_X=16
YEL_X=36
GRN_X=56

# URL bar: centred, spans most of the chrome bar
URL_X=80
URL_Y=8
URL_W=$((WIDTH - URL_X - 20))
URL_H=$((CHROME_H - 16))

# ── Build ffmpeg filter graph ──────────────────────────────────────────────
# 1. pad — extend canvas upward by CHROME_H pixels
# 2. drawbox — dark chrome background over the new top strip
# 3. Three traffic-light filled circles (drawn as small filled boxes)
# 4. URL bar (rounded-ish via a filled box)
FILTER="
pad=w=${WIDTH}:h=${NEW_H}:x=0:y=${CHROME_H}:color=${BG_COLOR}@1,
drawbox=x=0:y=0:w=${WIDTH}:h=${CHROME_H}:color=0x${BG_COLOR}:t=fill,
drawbox=x=$((RED_X - TL_R)):y=$((TL_Y - TL_R)):w=$((TL_R * 2)):h=$((TL_R * 2)):color=0xff5f57:t=fill,
drawbox=x=$((YEL_X - TL_R)):y=$((TL_Y - TL_R)):w=$((TL_R * 2)):h=$((TL_R * 2)):color=0xffbd2e:t=fill,
drawbox=x=$((GRN_X - TL_R)):y=$((TL_Y - TL_R)):w=$((TL_R * 2)):h=$((TL_R * 2)):color=0x28c840:t=fill,
drawbox=x=${URL_X}:y=${URL_Y}:w=${URL_W}:h=${URL_H}:color=0x${URL_BAR_COLOR}:t=fill
"

# Remove internal newlines/spaces so ffmpeg receives a single -vf value
FILTER="$(echo "$FILTER" | tr -d '\n' | tr -s ' ')"

# ── Re-encode WebM (VP9, higher quality) ──────────────────────────────────
TMP="$(mktemp "${TMPDIR:-/tmp}/demo-postproc-XXXXXX.webm")"
trap 'rm -f "$TMP"' EXIT

echo "    Adding browser chrome + re-encoding VP9 …"
ffmpeg -y -i "$INPUT" \
  -vf "$FILTER" \
  -c:v libvpx-vp9 \
  -crf 28 \
  -b:v 0 \
  -deadline good \
  -cpu-used 2 \
  -an \
  "$TMP" 2>&1 | grep -E "^(ffmpeg|Input|Output|Stream|frame=|video:)" || true

mv "$TMP" "$INPUT"
echo "    ✅  Re-encoded: $INPUT"

# ── Export animated GIF (first 12 s, 960 px wide, 10 fps) ─────────────────
echo "    Generating GIF preview …"
PALETTE="$(mktemp "${TMPDIR:-/tmp}/demo-palette-XXXXXX.png")"
trap 'rm -f "$TMP" "$PALETTE"' EXIT

SCALE_FILTER="scale=960:-1:flags=lanczos"
GIF_DURATION=12

# Pass 1: generate optimal palette
ffmpeg -y \
  -ss 0 -t $GIF_DURATION \
  -i "$INPUT" \
  -vf "${SCALE_FILTER},fps=10,palettegen=max_colors=128:stats_mode=diff" \
  "$PALETTE" 2>/dev/null

# Pass 2: render GIF using palette
ffmpeg -y \
  -ss 0 -t $GIF_DURATION \
  -i "$INPUT" \
  -i "$PALETTE" \
  -lavfi "${SCALE_FILTER},fps=10[x];[x][1:v]paletteuse=dither=bayer:bayer_scale=5" \
  "$GIF_OUT" 2>/dev/null

echo "    ✅  GIF preview: $GIF_OUT"
echo "🎉  Done."
