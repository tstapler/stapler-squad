#!/usr/bin/env bash
# Fail when go.mod's `go` or `toolchain` version is not listed in tools/bzl/supported-go.txt (Task 11.8a):
# rules_go picks the Go SDK from go.mod, so an unlisted bump changes the toolchain under every Bazel build.
# Env: GO_MOD, SUPPORTED_GO (tests seed them).
set -eu
root=$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)
gomod="${GO_MOD:-$root/go.mod}"
supported="${SUPPORTED_GO:-$root/tools/bzl/supported-go.txt}"
rc=0
check() { # <label> <version>
  [ -n "$2" ] || return 0
  if ! grep -vE '^[[:space:]]*(#|$)' "$supported" | sed 's/[[:space:]]*#.*$//' | grep -Fxq "$2"; then
    echo "go.mod $1 version $2 is not in tools/bzl/supported-go.txt. Remedy: use the bazel-version-upgrades skill to verify the upgrade canary on that Go version, then list it." >&2
    rc=1
  fi
}
check go "$(awk '$1 == "go" { print $2; exit }' "$gomod")"
check toolchain "$(awk '$1 == "toolchain" { sub(/^go/, "", $2); print $2; exit }' "$gomod")"
exit "$rc"
