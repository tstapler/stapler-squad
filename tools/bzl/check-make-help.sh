#!/usr/bin/env bash
# Asserts the two-line Bazel precondition header at the top of `make help` (Story 1.6).
set -u
root=$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)
cd "$root" || exit 2
# env -u MAKEFLAGS: an inherited --trace (CI timing) would print trace lines into this stdout.
out=$(env -u MAKEFLAGS make --no-print-directory help 2>/dev/null | sed -n '3,4p')
l3=$(printf '%s\n' "$out" | sed -n 1p)
l4=$(printf '%s\n' "$out" | sed -n 2p)
case "$l3" in "precondition: Bazel needs no generated code; plain go and gopls need 'make sync'"*) ;; *) echo "check-make-help: line 3 of 'make help' is not the precondition header: $l3" >&2; exit 1 ;; esac
case "$l4" in "  gen/ is not a Bazel input"*) ;; *) echo "check-make-help: line 4 of 'make help' is not the header continuation: $l4" >&2; exit 1 ;; esac
# Every wrapper target (Task 8.1b) is listed in `make help` with a description that names its Bazel command.
help=$(env -u MAKEFLAGS make --no-print-directory help 2>/dev/null)
bad=0
n=0
for t in $(bash tools/bzl/wrapper-targets.sh); do
  n=$((n + 1))
  line=$(printf '%s\n' "$help" | grep -E "^$t +" | head -1)
  if [ -z "$line" ]; then echo "check-make-help: wrapper target '$t' is missing from make help" >&2; bad=1
  elif ! printf '%s' "$line" | grep -qi 'bazel'; then echo "check-make-help: wrapper target '$t' has no Bazel command in its ## description: $line" >&2; bad=1; fi
done
[ "$bad" = 0 ] || exit 1
echo "make help header: ok; $n wrapper targets name their Bazel command"
