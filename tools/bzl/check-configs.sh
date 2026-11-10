#!/usr/bin/env bash
# Sealed set of .bazelrc configs: every build:/test: name must be in tools/bzl/configs.txt and
# vice versa. Usage: check-configs.sh [--rc <file>]  (default .bazelrc). Exits 1 naming offenders.
set -u
root="$(git rev-parse --show-toplevel)" || exit 2
cd "$root" || exit 2
rc=.bazelrc
if [ "${1:-}" = "--rc" ]; then rc="${2:?--rc needs a file}"; fi
list=tools/bzl/configs.txt
[ -f "$rc" ] || { echo "check-configs: $rc not found" >&2; exit 2; }
[ -f "$list" ] || { echo "check-configs: $list not found" >&2; exit 2; }

# hyphen is allowed so ci-cache-read is not collapsed into ci
used=$(grep -oE '^(build|test):[a-z0-9_-]+' "$rc" | cut -d: -f2 | sort -u)
documented=$(grep -vE '^[[:space:]]*(#|$)' "$list" | sort -u)

status=0
undocumented=$(comm -23 <(printf '%s\n' "$used") <(printf '%s\n' "$documented") | grep -v '^$')
missing=$(comm -13 <(printf '%s\n' "$used") <(printf '%s\n' "$documented") | grep -v '^$')
for n in $undocumented; do echo "check-configs: config '$n' is in $rc but not in $list" >&2; status=1; done
for n in $missing; do echo "check-configs: config '$n' is in $list but not in $rc" >&2; status=1; done
exit $status
