#!/usr/bin/env bash
# Exit 0 when at least one PKG word (./dir, ./dir/..., ./...) has *_test.go files; otherwise print the
# `go test` equivalent message for each word and exit 1 (the wrapper then skips Bazel and exits 0).
set -u
root=$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)
cd "$root" || exit 2
found=0
msgs=()
for p in "$@"; do
  dir=${p#./}
  case "$p" in ./...|...) found=1; continue ;; esac
  if [ "${dir%/...}" != "$dir" ]; then
    dir=${dir%/...}
    if [ -d "$dir" ] && [ -n "$(find "$dir" -name '*_test.go' -not -path '*/node_modules/*' -print -quit 2>/dev/null)" ]; then found=1; continue; fi
  elif [ -d "$dir" ] && compgen -G "$dir/*_test.go" >/dev/null; then
    found=1; continue
  fi
  msgs+=("no tests in $p (go test would say: no test files)")
done
[ "$found" = 1 ] && exit 0
printf '%s\n' "${msgs[@]}"
exit 1
