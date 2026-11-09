#!/usr/bin/env bash
# Fetch the CI-built ghostty-vt.wasm for the current pin and install it for the web build, instead
# of building it (Zig cannot link on macOS 26, so a Mac cannot run scripts/build-ghostty-wasm.sh).
#
# Source: the release asset "ghostty-wasm-<key>/ghostty-vt.wasm" published by
# .github/workflows/ghostty-wasm-publish.yml, where <key> is "<rev12>-zig<version>" (the same key as
# scripts/ghostty-wasm.sha256). The download is installed only if its sha256 equals the recorded one
# (fail-closed): the bytes are never trusted for being served from our release. Nothing is installed on
# any failure, and an older install is removed so a stale wasm cannot outlive a pin bump.
#
# Exit status: 0 own wasm installed and verified; 1 it was not (the web build then ships ghostty-web's
# stock wasm, or fails if GHOSTTY_WASM_REQUIRE_OWN=1). Callers that can live with the stock wasm
# (the Makefile) turn a failure into a warning; CI does not.
#
# Usage: scripts/fetch-ghostty-wasm.sh
# Env:   GHOSTTY_WASM_BASE_URL  override the release download base (tests, mirrors)
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"
OWN_DIR="$ROOT/web-app/build/ghostty-own"
OWN_WASM="$OWN_DIR/ghostty-vt.wasm"
REPO="${GITHUB_REPOSITORY:-tstapler/stapler-squad}"

[[ $# -eq 0 ]] || { echo "✗ takes no arguments" >&2; exit 2; }

fail() {
  rm -f "$OWN_WASM"
  echo "!!! ghostty-vt.wasm: NOT installing our own build: $*" >&2
  echo "!!! The web build will ship ghostty-web's STOCK wasm (no DECSTR / page-zeroing fixes)." >&2
  exit 1
}

sha256_of() {
  if command -v sha256sum >/dev/null 2>&1; then sha256sum "$1" | cut -d' ' -f1
  else shasum -a 256 "$1" | cut -d' ' -f1; fi
}
pin() { sed -n "s/^$1 = \"\\(.*\\)\"\$/\\1/p" "$ROOT/MODULE.bazel" | head -n1; }

REV="$(pin GHOSTTY_REV)"
ZIG="$(pin ZIG_VERSION)"
[[ "$REV" =~ ^[0-9a-f]{40}$ && -n "$ZIG" ]] || fail "could not read GHOSTTY_REV/ZIG_VERSION from MODULE.bazel"
KEY="${REV:0:12}-zig${ZIG}"
WANT="$(awk -v k="$KEY" '$1 == k { print $2 }' "$SCRIPT_DIR/ghostty-wasm.sha256")"
if [[ -z "$WANT" && "${GHOSTTY_WASM_ALLOW_UNPINNED:-}" == 1 && -f "$OWN_WASM" ]]; then
  echo "▶ No recorded sha256 for $KEY; keeping the local wasm (GHOSTTY_WASM_ALLOW_UNPINNED=1)"
  exit 0
fi
[[ "$WANT" =~ ^[0-9a-f]{64}$ ]] || fail "scripts/ghostty-wasm.sha256 has no recorded sha256 for $KEY"

if [[ -f "$OWN_WASM" && "$(sha256_of "$OWN_WASM")" == "$WANT" ]]; then
  echo "▶ ghostty-vt.wasm already installed and matches the pinned sha256 ($KEY)"
  exit 0
fi
rm -f "$OWN_WASM" # absent or stale: never leave a wasm that does not match the pin

BASE="${GHOSTTY_WASM_BASE_URL:-https://github.com/$REPO/releases/download}"
URL="$BASE/ghostty-wasm-$KEY/ghostty-vt.wasm"
TMP="$(mktemp -d)"
trap 'rm -rf "$TMP"' EXIT
echo "▶ Downloading $URL"
curl -fsSL --retry 3 --retry-delay 2 --max-time 120 -o "$TMP/ghostty-vt.wasm" "$URL" \
  || fail "download failed (not published yet for $KEY, or offline); build it with 'make build-ghostty-wasm' on Linux"

GOT="$(sha256_of "$TMP/ghostty-vt.wasm")"
[[ "$GOT" == "$WANT" ]] || fail "sha256 mismatch for $KEY: downloaded $GOT, pinned $WANT"

mkdir -p "$OWN_DIR"
install -m 644 "$TMP/ghostty-vt.wasm" "$OWN_WASM"
echo "▶ Installed web-app/build/ghostty-own/ghostty-vt.wasm (sha256 $GOT, key $KEY)"
