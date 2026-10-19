#!/bin/sh
# shellcheck shell=sh
#
# shared-node-modules-makefile.test.sh -- regression test for the Makefile
# staleness-check fix around web-app/node_modules/.ssq-install-stamp.
#
# Make's mtime-based prerequisites alone can't tell "pre-existing real
# node_modules with an already-fresh .ssq-install-stamp" (every worktree that
# installed before shared-node-modules.sh existed, main checkout included)
# apart from "genuinely up to date" -- confirmed empirically: before the
# Makefile fix, `make -n` reported that target as already up to date in
# exactly this state, meaning shared-node-modules.sh's backfill-migration
# logic would never run. This test extracts the real rule block from the
# repo's own Makefile (not a hand-copied duplicate, so it can't silently
# drift from what's actually shipped) and proves it forces a re-run in that
# state, via a real (non-dry-run) `make` invocation against a stubbed bun.
#
# Usage: sh scripts/shared-node-modules-makefile.test.sh
set -eu

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"
REAL_MAKEFILE="$REPO_ROOT/Makefile"

TMP="$(mktemp -d)"
trap 'rm -rf "$TMP"' EXIT

fail() {
	echo "FAIL: $1" >&2
	exit 1
}

# Extract the exact rule block this test is about, by anchor lines, so a
# future edit to the real Makefile is what this test runs against -- not a
# copy that could silently diverge from what's shipped.
START_PATTERN='^NEEDS_NODE_MODULES_MIGRATION :='
END_PATTERN='^FORCE-node-modules-migration: ;$'
RULE_BLOCK="$(awk "/$START_PATTERN/,/$END_PATTERN/" "$REAL_MAKEFILE")"
[ -n "$RULE_BLOCK" ] || fail "could not find the NEEDS_NODE_MODULES_MIGRATION..FORCE-node-modules-migration block in $REAL_MAKEFILE -- did it move or get renamed?"
printf '%s\n' "$RULE_BLOCK" | grep -q 'FORCE-node-modules-migration' || fail "extracted block missing FORCE-node-modules-migration"

printf '%s\n' "$RULE_BLOCK" > "$TMP/Makefile"

# Stub bun: records one line per `install --frozen-lockfile` invocation and
# fabricates a minimal node_modules, same pattern as shared-node-modules.test.sh.
STUB_BIN="$TMP/bin"
mkdir -p "$STUB_BIN"
INSTALL_COUNTER="$TMP/install-count"
export INSTALL_COUNTER
: > "$INSTALL_COUNTER"
cat > "$STUB_BIN/bun" <<'EOF'
#!/bin/sh
set -eu
echo "install" >> "$INSTALL_COUNTER"
mkdir -p node_modules
mkdir -p node_modules/.bun
exit 0
EOF
chmod +x "$STUB_BIN/bun"
export PATH="$STUB_BIN:$PATH"

# Real shared-node-modules.sh, invoked exactly as the extracted recipe calls
# it ("./scripts/shared-node-modules.sh web-app" relative to $TMP).
mkdir -p "$TMP/scripts"
cp "$SCRIPT_DIR/shared-node-modules.sh" "$TMP/scripts/shared-node-modules.sh"
chmod +x "$TMP/scripts/shared-node-modules.sh"
SHARED_ROOT="$TMP/shared-node-modules"
export STAPLER_SQUAD_SHARED_NODE_MODULES_DIR="$SHARED_ROOT"

# Fixture: a pre-existing, non-symlinked node_modules with an
# already-fresher .ssq-install-stamp than package.json/bun.lock -- the
# exact state that fooled plain mtime prerequisites.
mkdir -p "$TMP/web-app/my-plugin"
cat > "$TMP/web-app/package.json" <<'EOF'
{
  "name": "fixture",
  "devDependencies": {
    "my-plugin": "file:./my-plugin"
  }
}
EOF
echo "bun-lock-content-makefile-test" > "$TMP/web-app/bun.lock"
echo "module.exports = { rule: 1 };" > "$TMP/web-app/my-plugin/index.js"
sleep 1.1 # ensure the next mtime is strictly later than package.json/bun.lock
mkdir -p "$TMP/web-app/node_modules"
: > "$TMP/web-app/node_modules/.ssq-install-stamp"

cd "$TMP"

# Sanity check: confirm this fixture really does reproduce the staleness
# trap under the OLD (mtime-only) rule, so this test would have caught the
# original bug -- not just exercise the new code path.
OLD_RULE="web-app/node_modules/.ssq-install-stamp: web-app/package.json web-app/bun.lock
	@./scripts/shared-node-modules.sh web-app"
printf '%s\n' "$OLD_RULE" > "$TMP/Makefile.old"
OLD_OUTPUT="$(make -n -f Makefile.old web-app/node_modules/.ssq-install-stamp 2>&1)"
case "$OLD_OUTPUT" in
*"is up to date"*) : ;;
*) fail "fixture setup didn't reproduce the staleness trap under the old mtime-only rule (got: $OLD_OUTPUT) -- can't prove the fix matters" ;;
esac
echo "PASS: fixture confirmed stale under the old mtime-only rule (would have been silently skipped)"

# The real fix: a real (non-dry-run) make invocation against the extracted
# current rule must actually re-run the script and convert node_modules.
make web-app/node_modules/.ssq-install-stamp > "$TMP/make-out.log" 2>&1 || { cat "$TMP/make-out.log"; fail "make invocation failed"; }
[ "$(wc -l < "$INSTALL_COUNTER")" -eq 1 ] || fail "expected shared-node-modules.sh to run exactly once, got $(wc -l < "$INSTALL_COUNTER") (script didn't fire -- Makefile staleness fix regressed)"
[ -L "web-app/node_modules" ] || fail "web-app/node_modules was not converted to a symlink by the forced re-run"
echo "PASS: a pre-existing non-symlinked node_modules with an already-fresh .ssq-install-stamp is converted on the next make invocation"

# Re-running immediately afterward must be a no-op: the forced rebuild only
# fires while node_modules isn't yet a symlink into the shared store.
make web-app/node_modules/.ssq-install-stamp > "$TMP/make-out2.log" 2>&1 || { cat "$TMP/make-out2.log"; fail "second make invocation failed"; }
[ "$(wc -l < "$INSTALL_COUNTER")" -eq 1 ] || fail "re-running after migration triggered another install -- staleness fix doesn't settle back into normal mtime caching"
echo "PASS: once migrated, the target settles back into normal mtime-based caching (no repeated forced rebuilds)"

echo "All shared-node-modules Makefile scenarios passed."
