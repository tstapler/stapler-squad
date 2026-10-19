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
# Also covers the backfill-migration path: an existing real (non-symlink)
# node_modules -- the state of every worktree that installed before this
# mechanism shipped -- must be migrated (moved) into the shared store rather
# than reinstalled from scratch, discarded if another worktree already
# seeded that hash, and left completely untouched if the frozen-lockfile
# verify step fails.
#
# `pnpm` is stubbed so this test is fast/deterministic/offline (per the
# deterministic-fast-tests skill) -- it exercises the script's own hashing,
# locking and symlinking logic, not pnpm's real install behavior. The real
# frozen-lockfile divergence/success semantics this relies on were verified
# separately against actual pnpm (see the PR description), not just assumed.
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
# Backfill-verify failure simulation: a FAIL_INSTALL marker in cwd stands in
# for a genuine package.json/pnpm-lock.yaml divergence (real pnpm's
# `--frozen-lockfile` failure mode, verified separately against actual
# pnpm). Exits before touching anything, same as the real failure.
if [ -f FAIL_INSTALL ]; then
	echo "stub pnpm: FAIL_INSTALL marker present, simulating a frozen-lockfile divergence" >&2
	exit 1
fi
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
if [ ! -d "$WT_E/node_modules" ] || [ -L "$WT_E/node_modules" ]; then
	fail "scenario 4: STAPLER_SQUAD_SHARED_NODE_MODULES=0 did not produce a plain local directory"
fi
echo "PASS: scenario 4 -- STAPLER_SQUAD_SHARED_NODE_MODULES=0 opts out to a plain local install"

# --- Scenarios 5-7 use before/after deltas on INSTALL_COUNTER rather than
# absolute totals (unlike 1-4 above), so they stay correct regardless of
# what runs before them. ---

# --- Scenario 5: backfill migration -- existing real node_modules, no
# shared install yet for this hash -> migrate (mv) the already-verified
# local tree into the shared store instead of reinstalling from scratch. ---
WT_F="$TMP/wt-f/web-app"
mkdir -p "$WT_F"
make_fixture "$WT_F"
echo "pnpm-lock-content-scenario5" > "$WT_F/pnpm-lock.yaml" # unique hash -- must not collide with scenarios 1-4's shared install
mkdir -p "$WT_F/node_modules"
echo "original-local-install-marker" > "$WT_F/node_modules/ORIGINAL_MARKER"

COUNT_BEFORE="$(wc -l < "$INSTALL_COUNTER")"
"$SCRIPT" "$WT_F" > "$TMP/out-f1" 2>&1 || { cat "$TMP/out-f1"; fail "scenario 5: backfill migration failed"; }
COUNT_AFTER="$(wc -l < "$INSTALL_COUNTER")"
[ "$((COUNT_AFTER - COUNT_BEFORE))" -eq 1 ] || fail "scenario 5: expected exactly 1 pnpm invocation (the verify step only, no reinstall-into-shared), got $((COUNT_AFTER - COUNT_BEFORE))"
[ -L "$WT_F/node_modules" ] || fail "scenario 5: node_modules is not a symlink after migration"
[ -f "$WT_F/node_modules/ORIGINAL_MARKER" ] || fail "scenario 5: migrated tree lost the original local content -- looks like a reinstall happened instead of a move"
grep -q "original-local-install-marker" "$WT_F/node_modules/ORIGINAL_MARKER" || fail "scenario 5: ORIGINAL_MARKER content mismatch"
echo "PASS: scenario 5 -- existing real node_modules with no prior shared install for its hash is migrated (moved) into the shared store, not reinstalled"

# --- Scenario 6: backfill migration -- existing real node_modules, but a
# shared install ALREADY exists for this hash -> discard the local tree,
# symlink to the pre-existing shared content unchanged. ---
WT_G="$TMP/wt-g/web-app"
WT_H="$TMP/wt-h/web-app"
mkdir -p "$WT_G" "$WT_H"
make_fixture "$WT_G"
make_fixture "$WT_H"
echo "pnpm-lock-content-scenario6" > "$WT_G/pnpm-lock.yaml" # unique hash, shared between WT_G and WT_H only
echo "pnpm-lock-content-scenario6" > "$WT_H/pnpm-lock.yaml"

# WT_G installs normally first (no pre-existing node_modules), establishing
# the shared install for this fixture's hash.
"$SCRIPT" "$WT_G" > "$TMP/out-g1" 2>&1 || { cat "$TMP/out-g1"; fail "scenario 6: seeding shared install via WT_G failed"; }
SHARED_TARGET="$(readlink "$WT_G/node_modules")"

# WT_H already has a real, stale-looking local node_modules before the
# script runs on it.
mkdir -p "$WT_H/node_modules"
echo "stale-local-content-must-be-discarded" > "$WT_H/node_modules/LOCAL_MARKER"

COUNT_BEFORE="$(wc -l < "$INSTALL_COUNTER")"
"$SCRIPT" "$WT_H" > "$TMP/out-h1" 2>&1 || { cat "$TMP/out-h1"; fail "scenario 6: migration onto an existing shared install failed"; }
COUNT_AFTER="$(wc -l < "$INSTALL_COUNTER")"
[ "$((COUNT_AFTER - COUNT_BEFORE))" -eq 1 ] || fail "scenario 6: expected exactly 1 pnpm invocation (the verify step only, no reinstall), got $((COUNT_AFTER - COUNT_BEFORE))"
[ -L "$WT_H/node_modules" ] || fail "scenario 6: node_modules is not a symlink"
[ "$(readlink "$WT_H/node_modules")" = "$SHARED_TARGET" ] || fail "scenario 6: did not link to the pre-existing shared install ($SHARED_TARGET)"
[ ! -e "$WT_H/node_modules/LOCAL_MARKER" ] || fail "scenario 6: stale local content leaked into the shared tree instead of being discarded"
echo "PASS: scenario 6 -- existing real node_modules is discarded when a shared install for its hash already exists, symlinked to the pre-existing shared content unchanged"

# --- Scenario 7: backfill verify step fails -> script aborts, the existing
# node_modules is left completely untouched (not partially moved/deleted). ---
WT_I="$TMP/wt-i/web-app"
mkdir -p "$WT_I"
make_fixture "$WT_I"
echo "pnpm-lock-content-scenario7" > "$WT_I/pnpm-lock.yaml" # unique hash -- irrelevant here since it never gets computed, but keeps intent clear
mkdir -p "$WT_I/node_modules"
echo "must-survive-a-failed-verify" > "$WT_I/node_modules/SURVIVOR_MARKER"
: > "$WT_I/FAIL_INSTALL" # tells the stub pnpm to simulate a frozen-lockfile divergence

COUNT_BEFORE="$(wc -l < "$INSTALL_COUNTER")"
if "$SCRIPT" "$WT_I" > "$TMP/out-i1" 2>&1; then
	fail "scenario 7: script succeeded despite a failing frozen-lockfile verify step"
fi
COUNT_AFTER="$(wc -l < "$INSTALL_COUNTER")"
[ "$COUNT_AFTER" -eq "$COUNT_BEFORE" ] || fail "scenario 7: a pnpm install was recorded despite the verify step failing"
[ -d "$WT_I/node_modules" ] && [ ! -L "$WT_I/node_modules" ] || fail "scenario 7: node_modules is no longer the original real directory after an aborted verify"
[ -f "$WT_I/node_modules/SURVIVOR_MARKER" ] || fail "scenario 7: original node_modules content was lost despite the script aborting"
echo "PASS: scenario 7 -- a failing frozen-lockfile verify step aborts the script and leaves the existing node_modules untouched"

echo "All shared-node-modules.sh scenarios passed."
