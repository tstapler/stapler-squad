#!/usr/bin/env bash
# Fails when a BUILD/.bzl/.bazelrc line carries a test tag that hides or loosens a test
# (no-sandbox, local, requires-network, external, no-cache, no-remote-cache) without
# `# tag-reason: <text> (#<issue>)` on the same or the preceding line (Task 4.2c).
# tools/bzl/test-tags-baseline.txt lists "<path> <tag>" pairs that predate the check; it may only shrink.
# BZL_TAGS_ROOT overrides the tree to check (tests). Prints PASS, or FAIL lines and exit 1.
set -u
root="${BZL_TAGS_ROOT:-$(git rev-parse --show-toplevel)}"
cd "$root" || exit 2
tags='no-sandbox|local|requires-network|external|no-cache|no-remote-cache'
reason='#[[:space:]]*tag-reason:[[:space:]]*.+\(#[0-9]+\)'
baseline=tools/bzl/test-tags-baseline.txt

if [ -d .git ] || git rev-parse --git-dir >/dev/null 2>&1; then
  files=$(git ls-files -co --exclude-standard -- 'BUILD' '*/BUILD' 'BUILD.bazel' '*/BUILD.bazel' '*.bzl' '.bazelrc' '*.bazelrc')
else
  files=$(find . -type f \( -name BUILD -o -name BUILD.bazel -o -name '*.bzl' -o -name '*.bazelrc' -o -name .bazelrc \) | sed 's|^\./||')
fi

bad=0
# shellcheck disable=SC2094,SC2086
for f in $files; do
  case "$f" in tools/bzl/check-test-tags*|tools/bzl/go_test_test.bzl|*/testdata/*) continue ;; esac
  [ -f "$f" ] || continue
  prev=""
  n=0
  while IFS= read -r line || [ -n "$line" ]; do
    n=$((n + 1))
    code=${line%%#*}
    hit=""
    case "$f" in
      *bazelrc) hit=$(printf '%s' "$code" | grep -E 'tag' | grep -oE "(^|[=, +-])($tags)($|[, ])" | tr -d '=, +-' | head -n1) ;;
      *) hit=$(printf '%s' "$code" | grep -oE "\"($tags)\"" | tr -d '"' | head -n1) ;;
    esac
    if [ -n "$hit" ]; then
      if ! printf '%s\n%s\n' "$prev" "$line" | grep -qE "$reason"; then
        if [ -f "$baseline" ] && grep -qxF "$f $hit" "$baseline"; then :; else
          echo "FAIL $f:$n: tag \"$hit\" needs a '# tag-reason: <text> (#<issue>)' comment on this or the preceding line"
          bad=1
        fi
      fi
    fi
    prev=$line
  done < "$f"
done
if [ "$bad" = 0 ]; then echo PASS; exit 0; fi
exit 1
