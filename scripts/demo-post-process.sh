#!/usr/bin/env bash
# demo-post-process.sh — Add browser chrome frame to assets/demo.webm and export a GIF preview.
#
# Usage: ./scripts/demo-post-process.sh [options] [input.webm]
#   Defaults to assets/demo.webm if no input is given.
#   Overwrites the input file in-place and writes assets/demo.gif alongside it.
#
# Options:
#   --title-card <text>       Prepend a ~1.5s static branded title-card frame (logo +
#                              <text>) before the captured footage. Pass "" to fall back
#                              to the input file's name, capitalized (e.g. review-queue ->
#                              "Review Queue") — see Epic 5.2 Story 5.2.1b in
#                              project_plans/stapler-squad-website/implementation/plan.md.
#   --trim-start-frames <N>    Drop the first N frames of captured footage before the
#                              browser-chrome stage. Frame-count-relative (never a
#                              wall-clock/sleep trigger), so output is deterministic
#                              regardless of which CI runner captured the input — see
#                              research/pitfalls.md §4 "timing-dependent framing/pacing
#                              risk". Default 0 (no-op).

set -euo pipefail

REPO_ROOT="$(cd "$(dirname "$0")/.." && pwd)"

# ── Parse arguments ─────────────────────────────────────────────────────────
TITLE_CARD_REQUESTED=0
TITLE_CARD_TEXT=""
TRIM_START_FRAMES=0
INPUT_ARG=""

while [[ $# -gt 0 ]]; do
  case "$1" in
    --title-card)
      if [[ $# -lt 2 ]]; then
        echo "❌  --title-card requires an argument (pass '' to use the capitalized-input-name fallback)" >&2
        exit 1
      fi
      TITLE_CARD_REQUESTED=1
      TITLE_CARD_TEXT="$2"
      shift 2
      ;;
    --trim-start-frames)
      if [[ $# -lt 2 ]]; then
        echo "❌  --trim-start-frames requires a frame count" >&2
        exit 1
      fi
      TRIM_START_FRAMES="$2"
      if ! [[ "$TRIM_START_FRAMES" =~ ^[0-9]+$ ]]; then
        echo "❌  --trim-start-frames must be a non-negative integer, got: $TRIM_START_FRAMES" >&2
        exit 1
      fi
      shift 2
      ;;
    --)
      shift
      break
      ;;
    -*)
      echo "❌  Unknown option: $1" >&2
      exit 1
      ;;
    *)
      INPUT_ARG="$1"
      shift
      ;;
  esac
done

INPUT="${INPUT_ARG:-assets/demo.webm}"
if [[ "$INPUT" != /* ]]; then
  INPUT="$REPO_ROOT/$INPUT"
fi
GIF_OUT="$(dirname "$INPUT")/demo.gif"

# capitalize_spec_name — derive a title-card fallback from a clip's filename, per
# Story 5.2.1b's documented fallback (FeatureSpec name, capitalized): strips the
# extension and common Playwright per-browser-project suffixes, then title-cases
# the remaining kebab/snake-case words (e.g. "review-queue-chromium" -> "Review Queue").
capitalize_spec_name() {
  local base
  base="$(basename "$1")"
  base="${base%.webm}"
  base="${base%.gif}"
  base="${base%.spec}"
  base="${base%-chromium}"
  base="${base%-webkit}"
  base="${base%-firefox}"
  base="${base//[-_]/ }"
  local word result=""
  for word in $base; do
    # ${word^} (capitalize first letter) needs Bash 4+; macOS ships Bash 3.2 by
    # default, so use tr+substring instead for portability.
    result+="$(tr '[:lower:]' '[:upper:]' <<<"${word:0:1}")${word:1} "
  done
  printf '%s' "${result% }"
}

if [[ "$TITLE_CARD_REQUESTED" -eq 1 && -z "$TITLE_CARD_TEXT" ]]; then
  TITLE_CARD_TEXT="$(capitalize_spec_name "$INPUT")"
  echo "ℹ️  No --title-card text given; falling back to the clip name: \"$TITLE_CARD_TEXT\""
fi

# xml_escape — minimal XML-entity escaping for text embedded in a generated SVG
# (the title text is attacker/owner-controlled free text, not pre-sanitized).
xml_escape() {
  local s="$1"
  s="${s//&/&amp;}"
  s="${s//</&lt;}"
  s="${s//>/&gt;}"
  s="${s//\"/&quot;}"
  s="${s//\'/&apos;}"
  printf '%s' "$s"
}

# ── Preflight checks ───────────────────────────────────────────────────────
if ! command -v ffmpeg &>/dev/null; then
  echo "❌  ffmpeg not found. Install with: brew install ffmpeg" >&2
  exit 1
fi

if [[ ! -f "$INPUT" ]]; then
  echo "❌  Input not found: $INPUT" >&2
  exit 1
fi

RSVG_CONVERT=""
if [[ "$TITLE_CARD_REQUESTED" -eq 1 ]]; then
  if command -v rsvg-convert &>/dev/null; then
    RSVG_CONVERT="rsvg-convert"
  elif command -v inkscape &>/dev/null; then
    RSVG_CONVERT="inkscape"
  else
    echo "❌  --title-card needs rsvg-convert or inkscape to rasterize web-app/public/logo.svg (ffmpeg can't composite SVG directly). Install with: brew install librsvg" >&2
    exit 1
  fi
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
# 0. (optional) trim — drop the first N captured frames (frame-count-relative pacing,
#    Story 5.2.2 — never a wall-clock/sleep trigger; see research/pitfalls.md §4)
# 1. pad — extend canvas upward by CHROME_H pixels
# 2. drawbox — dark chrome background over the new top strip
# 3. Three traffic-light filled circles (drawn as small filled boxes)
# 4. URL bar (rounded-ish via a filled box)
TRIM_STAGE=""
if [[ "$TRIM_START_FRAMES" -gt 0 ]]; then
  TRIM_STAGE="trim=start_frame=${TRIM_START_FRAMES},setpts=PTS-STARTPTS,"
fi

FILTER="
${TRIM_STAGE}
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
TITLE_TMP=""
LOGO_PNG=""
TEXT_SVG=""
TEXT_PNG=""
cleanup() { rm -f "$TMP" "${PALETTE:-}" "$TITLE_TMP" "$LOGO_PNG" "$TEXT_SVG" "$TEXT_PNG"; }
trap cleanup EXIT

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

# ── Title-card stage (Story 5.2.1): prepend a static branded frame ─────────
# Uses rsvg-convert + ffmpeg's overlay/concat instead of drawtext: drawtext
# needs libfreetype/fontconfig, absent from this repo's own Homebrew ffmpeg.
if [[ "$TITLE_CARD_REQUESTED" -eq 1 ]]; then
  echo "    Building title card: \"$TITLE_CARD_TEXT\" …"

  FPS="$(ffprobe -v error -select_streams v:0 -show_entries stream=r_frame_rate -of csv=p=0 "$TMP")"
  TITLE_CARD_DURATION=1.5

  LOGO_SVG="$REPO_ROOT/web-app/public/logo.svg"
  if [[ ! -f "$LOGO_SVG" ]]; then
    echo "❌  Logo not found: $LOGO_SVG" >&2
    exit 1
  fi

  rasterize_svg() {
    # rasterize_svg <svg-path> <out-png> <height> [width]
    local width="${4:-}"
    if [[ "$RSVG_CONVERT" == "rsvg-convert" ]]; then
      if [[ -n "$width" ]]; then
        rsvg-convert -w "$width" -h "$3" -o "$2" "$1"
      else
        rsvg-convert -h "$3" --keep-aspect-ratio -o "$2" "$1"
      fi
    else
      if [[ -n "$width" ]]; then
        inkscape "$1" --export-type=png --export-height="$3" --export-width="$width" --export-filename="$2" &>/dev/null
      else
        inkscape "$1" --export-type=png --export-height="$3" --export-filename="$2" &>/dev/null
      fi
    fi
  }

  LOGO_H=$((NEW_H * 35 / 100))
  LOGO_PNG="$(mktemp "${TMPDIR:-/tmp}/demo-logo-XXXXXX.png")"
  rasterize_svg "$LOGO_SVG" "$LOGO_PNG" "$LOGO_H"

  FONT_SIZE=$((NEW_H * 10 / 100))
  TEXT_H=$((FONT_SIZE * 2))
  LOGO_Y=$((NEW_H * 12 / 100))
  TEXT_Y=$((LOGO_Y + LOGO_H + FONT_SIZE / 2))

  TEXT_SVG="$(mktemp "${TMPDIR:-/tmp}/demo-title-text-XXXXXX.svg")"
  TEXT_PNG="$(mktemp "${TMPDIR:-/tmp}/demo-title-text-XXXXXX.png")"
  cat > "$TEXT_SVG" <<EOF
<svg xmlns="http://www.w3.org/2000/svg" width="${WIDTH}" height="${TEXT_H}" viewBox="0 0 ${WIDTH} ${TEXT_H}">
  <text x="50%" y="50%" dominant-baseline="middle" text-anchor="middle"
        font-family="sans-serif" font-size="${FONT_SIZE}" font-weight="600"
        fill="#ffffff">$(xml_escape "$TITLE_CARD_TEXT")</text>
</svg>
EOF
  rasterize_svg "$TEXT_SVG" "$TEXT_PNG" "$TEXT_H" "$WIDTH"

  TITLE_TMP="$(mktemp "${TMPDIR:-/tmp}/demo-titlecard-XXXXXX.webm")"
  ffmpeg -y \
    -f lavfi -i "color=c=0x${BG_COLOR}:size=${WIDTH}x${NEW_H}:rate=${FPS}:duration=${TITLE_CARD_DURATION}" \
    -loop 1 -framerate "$FPS" -t "$TITLE_CARD_DURATION" -i "$LOGO_PNG" \
    -loop 1 -framerate "$FPS" -t "$TITLE_CARD_DURATION" -i "$TEXT_PNG" \
    -i "$TMP" \
    -filter_complex "[0:v][1:v]overlay=x=(W-w)/2:y=${LOGO_Y}:format=auto[bg1];[bg1][2:v]overlay=x=(W-w)/2:y=${TEXT_Y}:format=auto[titlecard];[titlecard]format=yuv420p[titlecardf];[3:v]format=yuv420p[footage];[titlecardf][footage]concat=n=2:v=1:a=0[outv]" \
    -map "[outv]" \
    -c:v libvpx-vp9 \
    -crf 28 \
    -b:v 0 \
    -deadline good \
    -cpu-used 2 \
    -an \
    "$TITLE_TMP" 2>&1 | grep -E "^(ffmpeg|Input|Output|Stream|frame=|video:)" || true
  ffmpeg_status=${PIPESTATUS[0]}
  if [[ "$ffmpeg_status" -ne 0 ]]; then
    echo "❌  Title-card ffmpeg stage exited with status $ffmpeg_status" >&2
    exit 1
  fi

  if [[ ! -s "$TITLE_TMP" ]]; then
    echo "❌  Title-card ffmpeg stage produced no output" >&2
    exit 1
  fi

  mv "$TITLE_TMP" "$TMP"
  TITLE_TMP=""
  rm -f "$TEXT_PNG"
  echo "    ✅  Title card prepended (~${TITLE_CARD_DURATION}s)"
fi

mv "$TMP" "$INPUT"
echo "    ✅  Re-encoded: $INPUT"

# ── Export animated GIF (first 12 s, 960 px wide, 10 fps) ─────────────────
echo "    Generating GIF preview …"
PALETTE="$(mktemp "${TMPDIR:-/tmp}/demo-palette-XXXXXX.png")"

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
