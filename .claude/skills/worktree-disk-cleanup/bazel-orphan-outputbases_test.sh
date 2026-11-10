#!/usr/bin/env bash
# Fixture test for bazel-orphan-outputbases.sh; every path lives under a private temp root.
# shellcheck disable=SC2016,SC2034,SC2329  # eval-ed check bodies and trap-invoked cleanup
set -u
here=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)
script="$here/bazel-orphan-outputbases.sh"
T=$(mktemp -d "${TMPDIR:-/tmp}/orphan-test.XXXXXX")
pids=""
# shellcheck disable=SC2317  # cleanup runs from the EXIT trap
cleanup() { for p in $pids; do kill "$p" 2>/dev/null; done; chmod -R u+w "$T" 2>/dev/null; rm -rf "$T"; }
trap cleanup EXIT
failures=0
ok() { echo "ok   - $1"; }
bad() { echo "FAIL - $1" >&2; failures=$((failures + 1)); }
check() { if eval "$2"; then ok "$1"; else bad "$1"; fi; }

U="$T/user_root"; CACHE="$T/disk"; REPO="$T/repo-cache"; TMPR="$T/tmp"
mkdir -p "$U" "$CACHE" "$REPO" "$TMPR" "$T/live-ws"
mkbase() { # name workspace
  mkdir -p "$U/$1/server" "$U/$1/execroot"; printf '%s' "$2" > "$U/$1/DO_NOT_BUILD_HERE"; echo data > "$U/$1/execroot/f"
}
mkbase live "$T/live-ws"
mkbase orphan1 "$T/gone-ws-1"
mkbase orphan2 "$T/gone-ws-2"   # unrelated-pid case
mkbase orphan3 "$T/gone-ws-3"   # live-stub-server case
chmod 500 "$U/orphan1/execroot"  # read-only trees must still be removable
echo cachefile > "$CACHE/blob"; echo repofile > "$REPO/blob"
mkdir -p "$TMPR/ssq-stale" "$TMPR/ssq-fresh" "$TMPR/other-stale"
touch "$TMPR/ssq-stale/f" "$TMPR/ssq-fresh/f"
touch -d '3 days ago' "$TMPR/ssq-stale/f" "$TMPR/ssq-stale" "$TMPR/other-stale"

# Stub server: argv carries --output_base=<orphan3>. Bystander: same pid file content, no such argument.
bash -c 'while :; do sleep 1; done' stub "--output_base=$U/orphan3" & stub=$!
bash -c 'while :; do sleep 1; done' bystander & by=$!
pids="$stub $by"
echo "$stub" > "$U/orphan3/server/server.pid.txt"
echo "$by" > "$U/orphan2/server/server.pid.txt"

# Real fuser when installed; otherwise a stub that reports "no open handles" so the stale-dir cases still run.
if command -v fuser >/dev/null 2>&1; then FUSER_BIN="fuser"; else printf '#!/bin/sh\nexit 1\n' > "$T/fuser"; chmod +x "$T/fuser"; FUSER_BIN="$T/fuser"; fi
run() { BAZEL_USER_ROOT="$U" DISK_CACHE="$CACHE" TMP_ROOT="$TMPR" FUSER="$FUSER_BIN" bash "$script" "$@"; }
snapshot() { (cd "$T" && find user_root disk repo-cache tmp | sort | tr '\n' ' '); }

before=$(snapshot)
out=$(run)
check "dry run lists an orphan base" 'echo "$out" | grep -q "orphan: $U/orphan1"'
check "dry run never lists a live workspace base" '! echo "$out" | grep -q "orphan: $U/live"'
check "dry run names the live stub server" 'echo "$out" | grep -q "orphan: $U/orphan3.*live server pid $stub"'
check "dry run lists the stale temp dir" 'echo "$out" | grep -q "stale temp: $TMPR/ssq-stale"'
check "dry run ignores fresh and non-matching temp dirs" '! echo "$out" | grep -qE "ssq-fresh|other-stale"'
check "dry run changes nothing" '[ "$(snapshot)" = "$before" ]'
check "dry run leaves the stub server running" 'kill -0 "$stub" 2>/dev/null'

run --apply >/dev/null
check "--apply removes orphan bases" '[ ! -e "$U/orphan1" ] && [ ! -e "$U/orphan2" ] && [ ! -e "$U/orphan3" ]'
check "--apply keeps the live base" '[ -f "$U/live/execroot/f" ]'
check "--apply keeps both caches" '[ -f "$CACHE/blob" ] && [ -f "$REPO/blob" ]'
check "--apply without --tmp keeps temp dirs" '[ -d "$TMPR/ssq-stale" ]'
check "stub server was killed before removal" '! kill -0 "$stub" 2>/dev/null'
check "unrelated process with the same pid file survives" 'kill -0 "$by" 2>/dev/null'

run --apply --tmp >/dev/null
check "--apply --tmp removes the stale temp dir only" '[ ! -e "$TMPR/ssq-stale" ] && [ -d "$TMPR/ssq-fresh" ] && [ -d "$TMPR/other-stale" ]'

exit $((failures > 0))
