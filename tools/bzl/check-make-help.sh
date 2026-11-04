#!/usr/bin/env bash
# Asserts the two-line Bazel precondition header at the top of `make help` (Story 1.6).
set -u
root=$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)
cd "$root" || exit 2
out=$(make --no-print-directory help 2>/dev/null | sed -n '3,4p')
l3=$(printf '%s\n' "$out" | sed -n 1p)
l4=$(printf '%s\n' "$out" | sed -n 2p)
case "$l3" in "precondition: Bazel needs no generated code; plain go and gopls need 'make sync'"*) ;; *) echo "check-make-help: line 3 of 'make help' is not the precondition header: $l3" >&2; exit 1 ;; esac
case "$l4" in "  gen/ is not a Bazel input"*) ;; *) echo "check-make-help: line 4 of 'make help' is not the header continuation: $l4" >&2; exit 1 ;; esac
echo "make help header: ok"
