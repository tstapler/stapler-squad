#!/usr/bin/env bash
# Build our own ghostty-vt.wasm with Bazel (//third_party/ghostty:ghostty_vt_wasm), install it
# for the web build, and compare its sha256 with the recorded one. The pins (Ghostty fork commit,
# Zig tarballs + sha256) live in MODULE.bazel; this script only drives Bazel and
# post-processes the output.
#
# Output (gitignored): web-app/build/ghostty-own/ghostty-vt.wasm. The Bun build's ghostty plugin
# (web-app/build/plugins/ghostty-wasm.ts) picks it over ghostty-web's stock wasm and ships it as
# a hashed asset, the same path as the stock one. See docs/explanation/own-ghostty-wasm-build.md.
#
# Usage:
#   scripts/build-ghostty-wasm.sh            build and install
#   scripts/build-ghostty-wasm.sh --test     run the Zig softReset test first (fails the build if it fails)
#   scripts/build-ghostty-wasm.sh --verify   FAIL unless the sha256 matches scripts/ghostty-wasm.sha256 (CI)
#   scripts/build-ghostty-wasm.sh --clean    remove the installed outputs
#
# Extra Bazel flags go in the BAZEL_FLAGS env var, e.g. a remote cache:
#   BAZEL_FLAGS="--remote_cache=grpcs://cache.example" make build-ghostty-wasm
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"
MODULE_FILE="$ROOT/MODULE.bazel"
RECORD_FILE="$SCRIPT_DIR/ghostty-wasm.sha256"
OWN_DIR="$ROOT/web-app/build/ghostty-own"
TARGET="//third_party/ghostty:ghostty_vt_wasm"
TEST_TARGET="//third_party/ghostty:softreset_test"

log() { echo "▶ $*"; }
err() { echo "✗ $*" >&2; exit 1; }

RUN_TESTS=0
VERIFY=0
for arg in "$@"; do
  case "$arg" in
    --test) RUN_TESTS=1 ;;
    --verify) VERIFY=1 ;;
    --clean)
      rm -rf "$OWN_DIR"
      log "Removed installed outputs (Bazel's own cache is untouched: bazel clean)."
      exit 0
      ;;
    *) err "Unknown argument: $arg" ;;
  esac
done

command -v bazel >/dev/null 2>&1 || err "bazel not found; install bazelisk (brew install bazelisk). .bazelversion pins the version."

pin() { sed -n "s/^$1 = \"\\(.*\\)\"\$/\\1/p" "$MODULE_FILE" | head -n1; }
sha256_of() {
  if command -v sha256sum >/dev/null 2>&1; then sha256sum "$1" | cut -d' ' -f1
  else shasum -a 256 "$1" | cut -d' ' -f1; fi
}

GHOSTTY_REV="$(pin GHOSTTY_REV)"
ZIG_VERSION="$(pin ZIG_VERSION)"
[[ "$GHOSTTY_REV" =~ ^[0-9a-f]{40}$ ]] || err "could not read GHOSTTY_REV from MODULE.bazel"
[[ -n "$ZIG_VERSION" ]] || err "could not read ZIG_VERSION from MODULE.bazel"

# shellcheck disable=SC2206  # BAZEL_FLAGS is intentionally word-split
FLAGS=(${BAZEL_FLAGS:-})

cd "$ROOT"
if [[ "$RUN_TESTS" == 1 ]]; then
  log "bazel build $TEST_TARGET (zig build test-lib-vt -Dtest-filter=softReset)"
  bazel build "${FLAGS[@]}" "$TEST_TARGET"
  TEST_LOG="$(bazel cquery "${FLAGS[@]}" --output=files "$TEST_TARGET" 2>/dev/null | head -n1)"
  [[ -f "$ROOT/$TEST_LOG" ]] || err "Bazel did not produce the test log"
  log "$(cat "$ROOT/$TEST_LOG")"
fi

log "bazel build $TARGET"
bazel build "${FLAGS[@]}" "$TARGET"
BUILT="$(bazel cquery "${FLAGS[@]}" --output=files "$TARGET" 2>/dev/null | head -n1)"
[[ -f "$ROOT/$BUILT" ]] || err "Bazel did not produce the wasm (looked for '$BUILT')"

WASM_SHA="$(sha256_of "$ROOT/$BUILT")"
WASM_SIZE="$(wc -c < "$ROOT/$BUILT" | tr -d ' ')"
RECORD_KEY="${GHOSTTY_REV:0:12}-zig${ZIG_VERSION}"
log "ghostty-vt.wasm  sha256=$WASM_SHA  bytes=$WASM_SIZE  key=$RECORD_KEY"

# ── install for the web build ──────────────────────────────────────────────

mkdir -p "$OWN_DIR"
cp "$ROOT/$BUILT" "$OWN_DIR/ghostty-vt.wasm"
chmod 644 "$OWN_DIR/ghostty-vt.wasm"
log "Installed web-app/build/ghostty-own/ghostty-vt.wasm (rebuild the web app to use it)"

# ── reproducibility record ─────────────────────────────────────────────────

RECORDED="$(awk -v k="$RECORD_KEY" '$1 == k { print $2 }' "$RECORD_FILE" 2>/dev/null || true)"
if [[ -z "$RECORDED" ]]; then
  echo "! No recorded sha256 for key $RECORD_KEY. After reviewing the build, add to $RECORD_FILE:"
  echo "    $RECORD_KEY $WASM_SHA"
  if [[ "$VERIFY" == 1 ]]; then err "--verify needs a recorded sha256 for this pin"; fi  # `&&` as the last command would exit 1 under set -e even without --verify
elif [[ "$RECORDED" != "$WASM_SHA" ]]; then
  err "Non-reproducible or changed output: recorded $RECORDED, built $WASM_SHA (key $RECORD_KEY)"
else
  log "Matches recorded sha256 (reproducible)."
fi
