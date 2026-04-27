#!/usr/bin/env bash
# Build tmux from the third_party/tmux submodule.
# Output: bin/tmux  (always at project root, regardless of CWD)
#
# Usage:
#   ./scripts/build-tmux.sh           # build to bin/tmux
#   ./scripts/build-tmux.sh --clean   # remove bin/tmux and build artifacts
#
# Dependencies (auto-installed via Homebrew on macOS if missing):
#   macOS:  automake, libevent, pkg-config, ncurses
#   Ubuntu: automake, libevent-dev, libncurses-dev, pkg-config
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"
SUBMODULE_DIR="$ROOT/third_party/tmux"
BIN_DIR="$ROOT/bin"
OUT_BIN="$BIN_DIR/tmux"

# ── helpers ────────────────────────────────────────────────────────────────

log() { echo "▶ $*"; }
err() { echo "✗ $*" >&2; exit 1; }

ensure_dep() {
  local cmd="$1"; local pkg="${2:-$1}"
  if ! command -v "$cmd" &>/dev/null; then
    if [[ "$(uname)" == "Darwin" ]]; then
      log "Installing $pkg via Homebrew..."
      brew install "$pkg"
    else
      err "Missing: $cmd. Install with: sudo apt-get install -y $pkg"
    fi
  fi
}

# ── clean ──────────────────────────────────────────────────────────────────

if [[ "${1:-}" == "--clean" ]]; then
  rm -f "$OUT_BIN"
  if [[ -f "$SUBMODULE_DIR/Makefile" ]]; then
    (cd "$SUBMODULE_DIR" && make distclean 2>/dev/null || true)
  fi
  log "Cleaned."
  exit 0
fi

# ── init submodule if needed ───────────────────────────────────────────────

if [[ ! -f "$SUBMODULE_DIR/configure.ac" ]]; then
  # Check for a proper git submodule gitlink (mode 160000 in the index).
  # `git submodule status` exits 0 even when not registered, so we check the index directly.
  if git -C "$ROOT" ls-files --stage third_party/tmux 2>/dev/null | grep -q '^160000 '; then
    log "Initializing third_party/tmux submodule..."
    (cd "$ROOT" && git submodule update --init third_party/tmux)
  else
    log "Cloning tmux 3.4 into third_party/tmux (gitlink not registered; run 'git submodule add' to fix)..."
    # Clone into a temp dir then merge so we preserve any existing files (e.g. BUILD.bazel).
    TMUX_TMP="$(mktemp -d)"
    git clone --depth 1 --branch 3.4 https://github.com/tmux/tmux.git "$TMUX_TMP"
    cp -rn "$TMUX_TMP"/. "$SUBMODULE_DIR/"   # -n = no-clobber, keeps our BUILD.bazel
    rm -rf "$TMUX_TMP"
  fi
fi

# ── check deps ─────────────────────────────────────────────────────────────

ensure_dep pkg-config  pkg-config
ensure_dep automake    automake

if [[ "$(uname)" == "Darwin" ]]; then
  ensure_dep brew libevent
  brew list libevent &>/dev/null || brew install libevent
  brew list ncurses  &>/dev/null || brew install ncurses
else
  # Linux: check for libevent and ncurses headers
  if ! pkg-config --exists libevent 2>/dev/null; then
    err "Missing libevent-dev. Install with: sudo apt-get install -y libevent-dev libncurses-dev"
  fi
fi

# ── build ──────────────────────────────────────────────────────────────────

log "Building tmux from $SUBMODULE_DIR..."
cd "$SUBMODULE_DIR"

if [[ ! -f "./configure" ]]; then
  log "Running autogen.sh..."
  ./autogen.sh
fi

if [[ ! -f "./Makefile" ]]; then
  log "Configuring..."
  if [[ "$(uname)" == "Darwin" ]]; then
    LIBEVENT_PREFIX="$(brew --prefix libevent)"
    NCURSES_PREFIX="$(brew --prefix ncurses)"
    ./configure \
      --prefix="$ROOT/bin/tmux-install" \
      --disable-utf8proc \
      PKG_CONFIG_PATH="$LIBEVENT_PREFIX/lib/pkgconfig:$NCURSES_PREFIX/lib/pkgconfig" \
      CFLAGS="-I$LIBEVENT_PREFIX/include -I$NCURSES_PREFIX/include" \
      LDFLAGS="-L$LIBEVENT_PREFIX/lib -L$NCURSES_PREFIX/lib"
  else
    ./configure --prefix="$ROOT/bin/tmux-install" --disable-utf8proc
  fi
fi

log "Compiling (this takes ~30s)..."
make -j"$(nproc 2>/dev/null || sysctl -n hw.logicalcpu 2>/dev/null || echo 4)"

mkdir -p "$BIN_DIR"
cp tmux "$OUT_BIN"
chmod +x "$OUT_BIN"

log "tmux built: $OUT_BIN ($("$OUT_BIN" -V 2>/dev/null || echo unknown))"
