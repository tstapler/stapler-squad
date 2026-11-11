#!/usr/bin/env bash
# Seeded forks registers: empty passes; an overdue row, a fixed-upstream row and a link-less row fail.
set -u
here=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)
T=$(mktemp -d "${TMPDIR:-/tmp}/forkage-test.XXXXXX"); trap 'rm -rf "$T"' EXIT
failures=0
head='| Fork | Upstream link | Removal condition | Pinned commit | Added | Remove by | Fixed upstream in |\n|---|---|---|---|---|---|---|\n'
check() { # label want-exit rows
  local label=$1 want=$2 rc
  printf '# t\n\n%b%b' "$head" "$3" > "$T/forks.md"
  FORKS_FILE="$T/forks.md" TODAY=2026-11-03 bash "$here/check-fork-age.sh" >/dev/null 2>&1; rc=$?
  if [ "$rc" = "$want" ]; then echo "PASS - $label"; else echo "FAIL - $label (rc=$rc)" >&2; failures=$((failures + 1)); fi
}
check "no forks" 0 ''
check "fork within its window" 0 '| rules_x | https://github.com/o/r/pull/1 | drop when 2.0 ships | abc123 | 2026-10-01 | 2027-01-01 | - |\n'
check "fork past its remove-by date" 1 '| rules_x | https://github.com/o/r/pull/1 | drop when 2.0 ships | abc123 | 2026-01-01 | 2026-06-01 | - |\n'
check "fork whose upstream fix shipped" 1 '| rules_x | https://github.com/o/r/pull/1 | drop when 2.0 ships | abc123 | 2026-10-01 | 2027-01-01 | 2.0.0 |\n'
check "fork without an upstream link" 1 '| rules_x | - | drop when 2.0 ships | abc123 | 2026-10-01 | 2027-01-01 | - |\n'
if bash "$here/check-fork-age.sh" >/dev/null 2>&1; then echo "PASS - repo register"; else echo "FAIL - repo register" >&2; failures=$((failures + 1)); fi
exit $((failures > 0))
