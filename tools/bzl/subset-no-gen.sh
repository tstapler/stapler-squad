#!/usr/bin/env bash
# Lists Go packages (including their tests) whose transitive imports contain neither generated
# protobuf (gen/proto/go) nor generated ent code (session/ent). Temporary: deleted by Task 2.6a.
# `go list -e` keeps unresolved imports in -deps, so this works with or without a synced tree.
set -euo pipefail
cd "$(dirname "$0")/../.."
out=tools/bzl/no-gen-packages.txt
mod=$(go list -m)
tmp=$(mktemp)
trap 'rm -f "$tmp"' EXIT
go list -e -f '{{.ImportPath}}' ./... | while read -r imp; do
  rel=${imp#"$mod"}
  rel=${rel#/}
  [ -n "$rel" ] || continue
  # Deliberately BUILD-less packages (tools/bzl/no-build-packages.txt) are not Bazel targets.
  [ -f "$rel/BUILD.bazel" ] || [ -f "$rel/BUILD" ] || continue
  if go list -e -deps -test -f '{{.ImportPath}}' "./$rel" | grep -qE "$mod/(gen/proto/go|session/ent)(/|$)"; then
    continue
  fi
  echo "$rel"
done | sort > "$tmp"
mv "$tmp" "$out"
trap - EXIT
wc -l < "$out"
