#!/usr/bin/env bash
# Fetch the pinned, checksum-verified tymuxd release binary from
# github.com/tstapler/tymux/releases — no cargo/rustc toolchain required.
# Output: session/tymux/embed/tymuxd  (always at project root, regardless of CWD)
#
# Usage:
#   TYMUX_VERSION=v1.0.0 ./scripts/fetch-tymuxd.sh
#
# See project_plans/tymux-bundled-integration/decisions/ADR-001-prebuilt-tymuxd-binary-download.md
# for why this fetches a prebuilt binary instead of compiling from source.
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"
CHECKSUMS_FILE="$SCRIPT_DIR/tymuxd-checksums.txt"
EMBED_DIR="$ROOT/session/tymux/embed"
OUT_BIN="$EMBED_DIR/tymuxd"

TYMUX_VERSION="${TYMUX_VERSION:-v1.0.0}"

# ── helpers ────────────────────────────────────────────────────────────────

log() { echo "▶ $*"; }
err() { echo "✗ $*" >&2; exit 1; }

# ── platform detection ─────────────────────────────────────────────────────
# Map uname output to the exact target triples tstapler/tymux publishes
# release tarballs for. Anything else (including Windows shells — MSYS/Cygwin
# report a Windows-flavored uname -s, and there is no tymux Windows release
# target at all) fails loudly with a named gap rather than a confusing
# download failure.

OS="$(uname -s)"
ARCH="$(uname -m)"

case "$OS-$ARCH" in
  Darwin-arm64)
    TARGET="aarch64-apple-darwin"
    ;;
  Darwin-x86_64)
    TARGET="x86_64-apple-darwin"
    ;;
  Linux-x86_64)
    TARGET="x86_64-unknown-linux-musl"
    ;;
  Linux-aarch64|Linux-arm64)
    TARGET="aarch64-unknown-linux-musl"
    ;;
  *)
    err "Unsupported platform: $OS-$ARCH. tymux publishes prebuilt binaries only for \
aarch64-apple-darwin, x86_64-apple-darwin, x86_64-unknown-linux-musl, and \
aarch64-unknown-linux-musl (see https://github.com/tstapler/tymux/releases). \
Windows has no tymux release target — the tymux backend is unavailable there \
until upstream adds one (see ADR-001's Decision section)."
    ;;
esac

log "Detected platform: $OS-$ARCH -> $TARGET"

# ── look up pinned checksum ────────────────────────────────────────────────

if [[ ! -f "$CHECKSUMS_FILE" ]]; then
  err "Missing checksums file: $CHECKSUMS_FILE"
fi

EXPECTED_SHA="$(awk -v version="$TYMUX_VERSION" -v target="$TARGET" \
  '$1 == version && $2 == target { print $3 }' "$CHECKSUMS_FILE")"

if [[ -z "$EXPECTED_SHA" ]]; then
  err "No pinned checksum for (version=$TYMUX_VERSION, target=$TARGET) in $CHECKSUMS_FILE. \
Add one (see the header comment in that file for how the existing entries were computed) \
before fetching an unpinned version/target combination."
fi

# ── download ───────────────────────────────────────────────────────────────

ASSET="tymux-${TARGET}.tar.gz"
TARBALL_URL="https://github.com/tstapler/tymux/releases/download/${TYMUX_VERSION}/${ASSET}"

TMPDIR_FETCH="$(mktemp -d /tmp/tymuxd-fetch-XXXXXX)"
trap 'rm -rf "$TMPDIR_FETCH"' EXIT

TMPTAR="${TMPDIR_FETCH}/${ASSET}"

log "Downloading ${TARBALL_URL}..."
if ! curl -fsSL -o "$TMPTAR" "$TARBALL_URL"; then
  err "Download failed: $TARBALL_URL. Check that TYMUX_VERSION=$TYMUX_VERSION is a real \
release tag at https://github.com/tstapler/tymux/releases and that the network is reachable."
fi

# ── verify checksum ────────────────────────────────────────────────────────
# sha256sum -c (GNU, Linux) and `shasum -a 256 -c` (macOS/BSD) both accept the
# same "<hash>  <filename>" line format on stdin, so build one checksum line
# and feed it to whichever tool is present rather than parsing tool-specific
# output.

log "Verifying checksum..."
ACTUAL_SHA=""
if command -v sha256sum &>/dev/null; then
  ACTUAL_SHA="$(sha256sum "$TMPTAR" | awk '{print $1}')"
elif command -v shasum &>/dev/null; then
  ACTUAL_SHA="$(shasum -a 256 "$TMPTAR" | awk '{print $1}')"
else
  err "Neither sha256sum nor shasum is available to verify the download. Install \
coreutils (Linux) or use the macOS-builtin shasum."
fi

if [[ "$ACTUAL_SHA" != "$EXPECTED_SHA" ]]; then
  err "Checksum mismatch for $ASSET (version $TYMUX_VERSION)! \
expected=$EXPECTED_SHA actual=$ACTUAL_SHA. Refusing to extract an unverified binary. \
If this release asset legitimately changed, update $CHECKSUMS_FILE deliberately — \
do not silently accept a new hash."
fi

log "Checksum verified: $ACTUAL_SHA"

# ── extract only tymuxd (not the tymux CLI, which this project doesn't need) ─

mkdir -p "$EMBED_DIR"
tar xzf "$TMPTAR" -C "$TMPDIR_FETCH" tymuxd
mv "$TMPDIR_FETCH/tymuxd" "$OUT_BIN"
chmod +x "$OUT_BIN"

# Note: tymuxd has no --version/--help flag — any unrecognized args are
# ignored and it starts the daemon (binds a real port, touches real state
# under ~/.local/state/tymux/), so don't exec it here just to report success.
log "tymuxd fetched: $OUT_BIN ($(du -h "$OUT_BIN" | cut -f1))"
