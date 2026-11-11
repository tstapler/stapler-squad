#!/usr/bin/env bash
# Seeded go.mod files against a seeded supported list.
set -u
here=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)
T=$(mktemp -d "${TMPDIR:-/tmp}/gotoolchain-test.XXXXXX"); trap 'rm -rf "$T"' EXIT
failures=0
printf '# comment\n1.26.6\n1.27.0 # after canary\n' > "$T/supported"
check() { # label want-exit go.mod-body
  local label=$1 want=$2 rc
  printf '%b' "$3" > "$T/go.mod"
  GO_MOD="$T/go.mod" SUPPORTED_GO="$T/supported" bash "$here/check-go-toolchain.sh" >/dev/null 2>&1; rc=$?
  if [ "$rc" = "$want" ]; then echo "PASS - $label"; else echo "FAIL - $label (rc=$rc)" >&2; failures=$((failures + 1)); fi
}
check "supported go line" 0 'module x\n\ngo 1.26.6\n'
check "supported go and toolchain lines" 0 'module x\n\ngo 1.26.6\n\ntoolchain go1.27.0\n'
check "unsupported go line" 1 'module x\n\ngo 1.99.0\n'
check "unsupported toolchain line" 1 'module x\n\ngo 1.26.6\n\ntoolchain go1.99.1\n'
real=$(bash "$here/check-go-toolchain.sh"; echo "rc=$?")
case "$real" in *rc=0) echo "PASS - repo go.mod is listed" ;; *) echo "FAIL - repo go.mod is listed" >&2; failures=$((failures + 1)) ;; esac
exit $((failures > 0))
