#!/usr/bin/env bash
# Compares every row of tools/bzl/testdata/pkg-mapping.tsv with what `BUILD_SYSTEM=bazel make` produces
# for PKG=/RUN=/TAGS=/TEST_TAGS=/RACE=/FORCE= (Task 8.2e). Needs make and bash only, never Bazel.
set -u
root=$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)
cd "$root" || exit 2
table=tools/bzl/testdata/pkg-mapping.tsv
fail=0
rows=0
while IFS=$'\t' read -r kind args expected; do
  case "$kind" in ''|'#'*) continue ;; esac
  rows=$((rows + 1))
  case "$kind" in
    cmd)
      out=$(eval "BUILD_SYSTEM=bazel make --no-print-directory -n $args" 2>&1); rc=$?
      got=$(printf '%s\n' "$out" | sed -n 's/^.*\[build-system=bazel\] \(bazel test .*\)"$/\1/p' | head -1)
      if [ "$rc" != 0 ] || [ "$got" != "$expected" ]; then
        echo "check-pkg-mapping: row '$args' (rc=$rc)"; echo "  want: $expected"; echo "  got:  $got"; fail=1
      fi ;;
    rc)
      out=$(eval "BUILD_SYSTEM=bazel make --no-print-directory -n $args" 2>&1); rc=$?
      case "$out" in *"$expected"*) [ "$rc" = 2 ] || { echo "check-pkg-mapping: row '$args' exited $rc, want 2"; fail=1; } ;;
        *) echo "check-pkg-mapping: row '$args' missing message: $expected"; fail=1 ;; esac ;;
    run)
      out=$(eval "BUILD_SYSTEM=bazel make --no-print-directory $args" 2>&1); rc=$?
      case "$out" in *"$expected"*) [ "$rc" = 0 ] || { echo "check-pkg-mapping: row '$args' exited $rc, want 0"; fail=1; } ;;
        *) echo "check-pkg-mapping: row '$args' missing message: $expected"; fail=1 ;; esac ;;
    *) echo "check-pkg-mapping: unknown row kind '$kind'"; fail=1 ;;
  esac
done < "$table"
[ "$fail" = 0 ] && echo "pkg mapping: $rows rows ok"
exit "$fail"
