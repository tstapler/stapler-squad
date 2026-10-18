#!/bin/sh
# shellcheck shell=sh
#
# shared-node-modules.test.sh -- regression test for shared-node-modules.sh's
# core properties: two "worktrees" with an identical pnpm-lock.yaml + local
# file:-dependency content must end up symlinked to the SAME shared install
# (and only install once); a worktree whose local file: dependency content
# differs -- even with pnpm-lock.yaml byte-identical -- must get a DIFFERENT
# shared install, never the stale one; a missing local file: dependency
# directory must fail loudly instead of installing something wrong; and the
# STAPLER_SQUAD_SHARED_NODE_MODULES=0 escape hatch must fall back to a plain
# local install (a real directory, not a symlink).
#
# `pnpm` is stubbed so this test is fast/deterministic/offline (per the
# deterministic-fast-tests skill) -- it exercises the script's own hashing,
# locking and symlinking logic, not pnpm's real install behavior.
#
# Usage: sh scripts/shared-node-modules.test.sh
set -eu

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
SCRIPT="$SCRIPT_DIR/shared-node-modules.sh"

TMP="$(mktemp -d)"
trap 'rm -rf "$TMP"' EXIT

STUB_BIN="$TMP/bin"
mkdir -p "$STUB_BIN"
INSTALL_COUNTER="$TMP/install-count"
export INSTALL_COUNTER
: > "$INSTALL_COUNTER"

# Stub pnpm: records one line per `install --frozen-lockfile` invocation and
# fabricates a minimal node_modules so the script's own bookkeeping
# (.install-complete, node_modules/.modules.yaml) has something to find.
# Real pnpm behavior is out of scope for this test.
cat > "$STUB_BIN/pnpm" <<'EOF'
#!/bin/sh
set -eu
echo "install" >> "$INSTALL_COUNTER"
mkdir -p node_modules
: > node_modules/.modules.yaml
exit 0
EOF
chmod +x "$STUB_BIN/pnpm"

export PATH="$STUB_BIN:$PATH"
SHARED_ROOT="$TMP/shared-node-modules"
export STAPLER_SQUAD_SHARED_NODE_MODULES_DIR="$SHARED_ROOT"

fail() {
	echo "FAIL: $1" >&2
	exit 1
}

make_fixture() {
	dir="$1"
	mkdir -p "$dir/my-plugin"
	cat > "$dir/package.json" <<'EOF'
{
  "name": "fixture",
  "devDependencies": {
    "my-plugin": "file:./my-plugin"
  }
}
EOF
	echo "pnpm-lock-content-v1" > "$dir/pnpm-lock.yaml"
	echo "module.exports = { rule: 1 };" > "$dir/my-plugin/index.js"
}

# --- Scenario 1: two worktrees, identical lockfile + plugin content -> one
# shared install, reused by the second. ---
WT_A="$TMP/wt-a/web-app"
WT_B="$TMP/wt-b/web-app"
mkdir -p "$(dirname "$WT_A")" "$(dirname "$WT_B")"
make_fixture "$WT_A"
make_fixture "$WT_B"

"$SCRIPT" "$WT_A" > "$TMP/out-a1" 2>&1 || { cat "$TMP/out-a1"; fail "scenario 1: first install failed"; }
"$SCRIPT" "$WT_B" > "$TMP/out-b1" 2>&1 || { cat "$TMP/out-b1"; fail "scenario 1: second worktree failed"; }

[ -L "$WT_A/node_modules" ] || fail "scenario 1: WT_A/node_modules is not a symlink"
[ -L "$WT_B/node_modules" ] || fail "scenario 1: WT_B/node_modules is not a symlink"
TARGET_A="$(readlink "$WT_A/node_modules")"
TARGET_B="$(readlink "$WT_B/node_modules")"
[ "$TARGET_A" = "$TARGET_B" ] || fail "scenario 1: two worktrees with identical lockfile+plugin content linked to different shared installs ($TARGET_A vs $TARGET_B)"
[ "$(wc -l < "$INSTALL_COUNTER")" -eq 1 ] || fail "scenario 1: expected exactly 1 pnpm install, got $(wc -l < "$INSTALL_COUNTER")"
echo "PASS: scenario 1 -- identical worktrees share one install"

# Re-running on an already-linked worktree must be a no-op (fast path).
"$SCRIPT" "$WT_A" > "$TMP/out-a2" 2>&1 || fail "scenario 1b: re-run on already-linked worktree failed"
[ "$(wc -l < "$INSTALL_COUNTER")" -eq 1 ] || fail "scenario 1b: re-running on an already-linked worktree triggered a reinstall"
echo "PASS: scenario 1b -- already-linked worktree is a no-op"

# --- Scenario 2: same pnpm-lock.yaml, but local file: dependency content
# differs -- must NOT share WT_A's install. ---
WT_C="$TMP/wt-c/web-app"
mkdir -p "$(dirname "$WT_C")"
make_fixture "$WT_C"
echo "module.exports = { rule: 2 };" > "$WT_C/my-plugin/index.js" # differs from WT_A/WT_B

"$SCRIPT" "$WT_C" > "$TMP/out-c1" 2>&1 || { cat "$TMP/out-c1"; fail "scenario 2: install failed"; }
TARGET_C="$(readlink "$WT_C/node_modules")"
[ "$TARGET_C" != "$TARGET_A" ] || fail "scenario 2: worktree with DIFFERENT local plugin content shared WT_A's install -- stale-plugin bug regressed"
[ "$(wc -l < "$INSTALL_COUNTER")" -eq 2 ] || fail "scenario 2: expected a second install for the differing plugin content"
echo "PASS: scenario 2 -- differing local file: dependency content gets its own install, lockfile hash alone is not trusted"

# --- Scenario 3: local file: dependency directory missing -> fail loudly. ---
WT_D="$TMP/wt-d/web-app"
mkdir -p "$(dirname "$WT_D")"
make_fixture "$WT_D"
rm -rf "$WT_D/my-plugin"

if "$SCRIPT" "$WT_D" > "$TMP/out-d1" 2>&1; then
	fail "scenario 3: script succeeded despite a missing local file: dependency directory"
fi
grep -q "my-plugin" "$TMP/out-d1" || fail "scenario 3: failure didn't name the missing directory"
[ -e "$WT_D/node_modules" ] && fail "scenario 3: a node_modules was created despite the missing dependency"
echo "PASS: scenario 3 -- missing local file: dependency directory fails loudly instead of installing something wrong"

# --- Scenario 4: STAPLER_SQUAD_SHARED_NODE_MODULES=0 escape hatch -> plain
# local install, a real directory, not a symlink. ---
WT_E="$TMP/wt-e/web-app"
mkdir -p "$(dirname "$WT_E")"
make_fixture "$WT_E"

STAPLER_SQUAD_SHARED_NODE_MODULES=0 "$SCRIPT" "$WT_E" > "$TMP/out-e1" 2>&1 || { cat "$TMP/out-e1"; fail "scenario 4: opt-out install failed"; }
[ -d "$WT_E/node_modules" ] && [ ! -L "$WT_E/node_modules" ] || fail "scenario 4: STAPLER_SQUAD_SHARED_NODE_MODULES=0 did not produce a plain local directory"
echo "PASS: scenario 4 -- STAPLER_SQUAD_SHARED_NODE_MODULES=0 opts out to a plain local install"

echo "All shared-node-modules.sh scenarios passed."
