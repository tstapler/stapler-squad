#!/usr/bin/env bash
# demo-post-process.test.sh — Epic 5.2 tests for demo-post-process.sh's
# --title-card and --trim-start-frames stages.
#
# Test names are the ones project_plans/stapler-squad-website/implementation/
# validation.md pre-designated for Stories 5.2.1/5.2.2. validation.md specifies
# them as `website/tests/{unit,integration}/demo-post-process.test.ts`, but
# demo-post-process.sh has no JS/TS runtime wrapper to unit-test — it's
# invoked and asserted on here directly via real ffmpeg runs, per the task
# brief's explicit direction ("shell/ffmpeg-based... not full visual
# diffing").
#
# demo-post-process.sh composites the title card via ffmpeg's `overlay`/
# `concat` filters, not `drawtext` — this repo's own ffmpeg build has no
# drawtext filter (not built with libfreetype: `ffmpeg -filters` omits it),
# so the script renders title text with rsvg-convert (already required for
# the logo) instead. Test names below keep validation.md's wording since
# they assert the same externally-observable behavior (a drawtext/overlay
# filter-graph stage producing a title card), not the literal filter name.
#
# Usage: ./scripts/demo-post-process.test.sh

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
POSTPROCESS="$SCRIPT_DIR/demo-post-process.sh"

WORK="$(mktemp -d "${TMPDIR:-/tmp}/demo-post-process-test-XXXXXX")"
trap 'rm -rf "$WORK"' EXIT

PASS=0
FAIL=0

ok()   { PASS=$((PASS + 1)); echo "  ok   - $1"; }
fail() { FAIL=$((FAIL + 1)); echo "  FAIL - $1" >&2; }

# make_fixture <dir> <name.webm> [duration] — a short synthetic captured-video
# stand-in (no real Playwright recording needed), 640x360 @30fps.
make_fixture() {
  local dir="$1" name="$2" dur="${3:-2}"
  mkdir -p "$dir"
  ffmpeg -y -f lavfi -i "testsrc=size=640x360:rate=30:duration=${dur}" \
    -pix_fmt yuv420p -c:v libvpx-vp9 -crf 30 -b:v 0 \
    "$dir/$name" &>/dev/null
}

webm_duration() {
  ffprobe -v error -show_entries format=duration -of csv=p=0 "$1"
}

webm_frame_count() {
  ffprobe -v error -count_frames -select_streams v:0 \
    -show_entries stream=nb_read_frames -of csv=p=0 "$1"
}

# ── Story 5.2.1: title-card filter stage ───────────────────────────────────
# Happy path — the title-card stage produces a visibly branded frame ahead of
# the captured footage when --title-card is given explicit text.
test_demoPostProcess_should_BuildDrawtextFilterGraph_When_TitleCardFlagProvided() {
  local name="demoPostProcess_should_BuildDrawtextFilterGraph_When_TitleCardFlagProvided"
  local dir="$WORK/t1" out
  make_fixture "$dir" "clip.webm"
  local before_dur after_dur
  before_dur="$(webm_duration "$dir/clip.webm")"

  if ! out="$("$POSTPROCESS" --title-card "Review Queue" "$dir/clip.webm" 2>&1)"; then
    fail "$name: script exited non-zero: $out"
    return
  fi

  if ! echo "$out" | grep -q 'Title card prepended'; then
    fail "$name: no title-card stage ran: $out"
    return
  fi

  after_dur="$(webm_duration "$dir/clip.webm")"
  local delta
  delta="$(echo "$after_dur - $before_dur" | bc)"
  # Expect ~+1.5s (allow 1.2-1.8s for encoder rounding).
  if (( $(echo "$delta < 1.2 || $delta > 1.8" | bc -l) )); then
    fail "$name: expected output duration to grow by ~1.5s, got delta=${delta}s (before=${before_dur}s after=${after_dur}s)"
    return
  fi

  if command -v compare &>/dev/null; then
    ffmpeg -y -ss 0.3 -i "$dir/clip.webm" -frames:v 1 "$dir/frame_titlecard.png" &>/dev/null
    ffmpeg -y -ss "$before_dur" -i "$dir/clip.webm" -frames:v 1 "$dir/frame_footage.png" 2>/dev/null \
      || ffmpeg -y -ss "$(echo "$before_dur - 0.1" | bc)" -i "$dir/clip.webm" -frames:v 1 "$dir/frame_footage.png" &>/dev/null
    local ae
    ae="$(compare -metric AE "$dir/frame_titlecard.png" "$dir/frame_footage.png" "$dir/diff.png" 2>&1 || true)"
    ae="${ae%% *}"
    # Title-card frame and captured-footage frame must differ substantially
    # (branded frame is visibly distinct from the testsrc pattern).
    if [[ -z "$ae" ]] || (( ae < 10000 )); then
      fail "$name: title-card frame doesn't look visibly distinct from footage (AE=$ae)"
      return
    fi
  fi

  ok "$name"
}

# Error path — Task 5.2.1b's documented fallback when the owner-decision text
# isn't resolved yet: capitalized clip/FeatureSpec name.
test_demoPostProcess_should_FallBackToCapitalizedSpecName_When_TitleCardTextNotProvided() {
  local name="demoPostProcess_should_FallBackToCapitalizedSpecName_When_TitleCardTextNotProvided"
  local dir="$WORK/t2" out
  make_fixture "$dir" "review-queue-chromium.webm"

  if ! out="$("$POSTPROCESS" --title-card "" "$dir/review-queue-chromium.webm" 2>&1)"; then
    fail "$name: script exited non-zero: $out"
    return
  fi

  if ! echo "$out" | grep -q 'falling back to the clip name: "Review Queue"'; then
    fail "$name: fallback text wasn't derived as \"Review Queue\": $out"
    return
  fi

  if ! echo "$out" | grep -q 'Building title card: "Review Queue"'; then
    fail "$name: title-card stage didn't use the fallback text: $out"
    return
  fi

  ok "$name"
}

# ── Story 5.2.2: frame-relative pacing ──────────────────────────────────────
test_demoPostProcess_should_UseSetptsFrameOffset_When_PacingStageApplied() {
  local name="demoPostProcess_should_UseSetptsFrameOffset_When_PacingStageApplied"
  local dir="$WORK/t3"
  make_fixture "$dir" "clip.webm" 3

  local before_frames after_frames
  before_frames="$(webm_frame_count "$dir/clip.webm")"

  if ! "$POSTPROCESS" --trim-start-frames 20 "$dir/clip.webm" &>/dev/null; then
    fail "$name: script exited non-zero"
    return
  fi

  after_frames="$(webm_frame_count "$dir/clip.webm")"
  local expected=$((before_frames - 20))
  if [[ "$after_frames" -ne "$expected" ]]; then
    fail "$name: expected ${expected} frames after trimming 20 (started at ${before_frames}), got ${after_frames}"
    return
  fi

  # The pacing stage itself must be frame-count-relative, never wall-clock:
  # confirm the source has no sleep/real-time-elapsed trigger at all (code
  # lines only — comments are allowed to mention "sleep" when explaining why
  # the script avoids it, as the pacing-stage header comment does).
  if grep -vE '^\s*#' "$POSTPROCESS" | grep -qE '(^|[^#[:alnum:]_])sleep[[:space:]]'; then
    fail "$name: demo-post-process.sh contains a sleep call — pacing must be frame-relative, not wall-clock"
    return
  fi
  if ! grep -q 'trim=start_frame=' "$POSTPROCESS" || ! grep -q 'setpts=PTS-STARTPTS' "$POSTPROCESS"; then
    fail "$name: pacing stage doesn't use trim=start_frame=/setpts (frame-relative) filters"
    return
  fi

  ok "$name"
}

# Error path — no sleep/wall-clock dependency; two runs against the same
# fixture must hash identically.
test_demoPostProcess_should_ProduceByteIdenticalOutput_When_RunTwiceAgainstSameInput() {
  local name="demoPostProcess_should_ProduceByteIdenticalOutput_When_RunTwiceAgainstSameInput"
  local dir1="$WORK/t4a" dir2="$WORK/t4b"
  make_fixture "$dir1" "clip.webm" 3
  make_fixture "$dir2" "clip.webm" 3

  "$POSTPROCESS" --trim-start-frames 10 "$dir1/clip.webm" &>/dev/null
  "$POSTPROCESS" --trim-start-frames 10 "$dir2/clip.webm" &>/dev/null

  local gif1 gif2 frames1 frames2
  gif1="$(sha256sum "$dir1/demo.gif" | awk '{print $1}')"
  gif2="$(sha256sum "$dir2/demo.gif" | awk '{print $1}')"
  frames1="$(webm_frame_count "$dir1/clip.webm")"
  frames2="$(webm_frame_count "$dir2/clip.webm")"

  # VP9's row-multithreaded encoder is not bit-deterministic run-to-run even in
  # the unmodified pre-Epic-5.2 script (verified by hand against the real
  # ffmpeg binary: two runs of the untouched original script produced two
  # different .webm SHA256 hashes for the same input), so .webm bytes aren't a
  # meaningful determinism signal here. The GIF pipeline (palette + paletteuse)
  # and frame count are: assert those instead.
  if [[ "$gif1" != "$gif2" ]]; then
    fail "$name: GIF output differs between two runs of the same input ($gif1 vs $gif2)"
    return
  fi
  if [[ "$frames1" != "$frames2" ]]; then
    fail "$name: webm frame count differs between two runs ($frames1 vs $frames2)"
    return
  fi

  ok "$name"
}

# ── Regression guard: pre-existing drawbox browser-chrome stage is unchanged ─
test_demoPostProcess_should_MatchPreEpicBaseline_When_NoFlagsGiven() {
  local name="demoPostProcess_should_MatchPreEpicBaseline_When_NoFlagsGiven"
  local dir="$WORK/t5"
  make_fixture "$dir" "clip.webm"

  if ! "$POSTPROCESS" "$dir/clip.webm" &>/dev/null; then
    fail "$name: script exited non-zero with no flags"
    return
  fi

  local w h
  w="$(ffprobe -v error -select_streams v:0 -show_entries stream=width -of csv=p=0 "$dir/clip.webm")"
  h="$(ffprobe -v error -select_streams v:0 -show_entries stream=height -of csv=p=0 "$dir/clip.webm")"
  # Source is 640x360; the pre-existing drawbox chrome stage pads +40px tall.
  if [[ "$w" -ne 640 || "$h" -ne 400 ]]; then
    fail "$name: expected unchanged chrome-stage output (640x400), got ${w}x${h}"
    return
  fi
  if [[ ! -s "$dir/demo.gif" ]]; then
    fail "$name: no GIF produced"
    return
  fi

  ok "$name"
}

# ── Integration: real ffmpeg run against a sample fixture ──────────────────
test_demoPostProcess_should_ProduceValidWebmWithTitleCardFrame_When_RunAgainstSampleCapturedVideo() {
  local name="demoPostProcess_should_ProduceValidWebmWithTitleCardFrame_When_RunAgainstSampleCapturedVideo"
  local dir="$WORK/t6"
  make_fixture "$dir" "sample.webm"

  if ! "$POSTPROCESS" --title-card "Session Lifecycle" "$dir/sample.webm" &>/dev/null; then
    fail "$name: script exited non-zero"
    return
  fi

  if ! ffprobe -v error "$dir/sample.webm" &>/dev/null; then
    fail "$name: output is not a valid webm per ffprobe"
    return
  fi

  local dur
  dur="$(webm_duration "$dir/sample.webm")"
  if (( $(echo "$dur < 3.2" | bc -l) )); then
    fail "$name: output duration ${dur}s too short for 2s footage + ~1.5s title card"
    return
  fi

  if command -v compare &>/dev/null; then
    ffmpeg -y -ss 0.2 -i "$dir/sample.webm" -frames:v 1 "$dir/early.png" &>/dev/null
    ffmpeg -y -ss 1.1 -i "$dir/sample.webm" -frames:v 1 "$dir/late.png" &>/dev/null
    local ae
    ae="$(compare -metric AE "$dir/early.png" "$dir/late.png" "$dir/diff.png" 2>&1 || true)"
    ae="${ae%% *}"
    # The title card itself must be static across its ~1.5s window.
    if [[ -z "$ae" ]] || (( ae > 5000 )); then
      fail "$name: title-card frame isn't static across its window (AE=$ae)"
      return
    fi
  fi

  ok "$name"
}

echo "=== demo-post-process.test.sh ==="
test_demoPostProcess_should_BuildDrawtextFilterGraph_When_TitleCardFlagProvided
test_demoPostProcess_should_FallBackToCapitalizedSpecName_When_TitleCardTextNotProvided
test_demoPostProcess_should_UseSetptsFrameOffset_When_PacingStageApplied
test_demoPostProcess_should_ProduceByteIdenticalOutput_When_RunTwiceAgainstSameInput
test_demoPostProcess_should_MatchPreEpicBaseline_When_NoFlagsGiven
test_demoPostProcess_should_ProduceValidWebmWithTitleCardFrame_When_RunAgainstSampleCapturedVideo

echo
echo "=== $PASS passed, $FAIL failed ==="
[[ "$FAIL" -eq 0 ]]
