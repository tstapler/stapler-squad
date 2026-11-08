#!/usr/bin/env bash
# Build our own ghostty-vt.wasm with Bazel (//third_party/ghostty:ghostty_vt_wasm), install it
# for the web build, and compare its sha256 with the recorded one. The pins (Ghostty commit,
# patches, Zig tarballs + sha256) live in MODULE.bazel; this script only drives Bazel and
# post-processes the output.
#
# Output (all gitignored; see docs/explanation/own-ghostty-wasm-build.md):
#   web-app/public/ghostty-vt.<sha12>.wasm   content-addressed copy the web build serves
#   web-app/public/ghostty-vt.json           {"file","sha256","rev","patches","zig"} manifest
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
PUBLIC_DIR="$ROOT/web-app/public"
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
      rm -f "$PUBLIC_DIR"/ghostty-vt.*.wasm "$PUBLIC_DIR/ghostty-vt.json"
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

# Identifies the patch series by content, so editing a patch invalidates the recorded sha.
PATCHSET_ID="$(cat "$ROOT"/third_party/ghostty-patches/[0-9][0-9][0-9][0-9]-*.patch 2>/dev/null | { sha256sum 2>/dev/null || shasum -a 256; } | cut -c1-12)"
[[ -n "$PATCHSET_ID" ]] || PATCHSET_ID="none"

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
RECORD_KEY="${GHOSTTY_REV:0:12}-${PATCHSET_ID}-zig${ZIG_VERSION}"
log "ghostty-vt.wasm  sha256=$WASM_SHA  bytes=$WASM_SIZE  key=$RECORD_KEY"

# ── install for the web build (content-addressed name; manifest is the stable URL) ──

mkdir -p "$PUBLIC_DIR"
rm -f "$PUBLIC_DIR"/ghostty-vt.*.wasm
HASHED="ghostty-vt.${WASM_SHA:0:12}.wasm"
cp "$ROOT/$BUILT" "$PUBLIC_DIR/$HASHED"
chmod 644 "$PUBLIC_DIR/$HASHED"
printf '{"file":"%s","sha256":"%s","rev":"%s","patches":"%s","zig":"%s"}\n' \
  "$HASHED" "$WASM_SHA" "$GHOSTTY_REV" "$PATCHSET_ID" "$ZIG_VERSION" > "$PUBLIC_DIR/ghostty-vt.json"
log "Installed web-app/public/$HASHED and ghostty-vt.json"

# ── reproducibility record ─────────────────────────────────────────────────

RECORDED="$(awk -v k="$RECORD_KEY" '$1 == k { print $2 }' "$RECORD_FILE" 2>/dev/null || true)"
if [[ -z "$RECORDED" ]]; then
  echo "! No recorded sha256 for key $RECORD_KEY. After reviewing the build, add to $RECORD_FILE:"
  echo "    $RECORD_KEY $WASM_SHA"
  [[ "$VERIFY" == 1 ]] && err "--verify needs a recorded sha256 for this pin + patch set"
elif [[ "$RECORDED" != "$WASM_SHA" ]]; then
  err "Non-reproducible or changed output: recorded $RECORDED, built $WASM_SHA (key $RECORD_KEY)"
else
  log "Matches recorded sha256 (reproducible)."
fi
