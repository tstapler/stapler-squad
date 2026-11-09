#!/usr/bin/env bash
# Authoritative GenFreshness (Task 2.4b, the CI gate): the synced gen/proto/go and session/ent trees must
# hash-equal what Bazel builds (bazel-bin). Exit 0 fresh, 1 stale ("run: make sync"), 2 tooling failure.
# The cheap, Bazel-free form is check-gen-fresh-cheap.sh. The hash covers every file's path and bytes.
set -u
cd "$(git rev-parse --show-toplevel)" || exit 2
command -v bazel >/dev/null || bazel() { BAZEL_REAL=${BAZEL_REAL:-$(command -v bazelisk)} tools/bazel "$@"; }
bazel build //proto/session/v1:gen_go //tools/bzl/ent:ent_gen >/dev/null 2>"${TMPDIR:-/tmp}/check-gen-fresh.$$.log" \
  || { tail -5 "${TMPDIR:-/tmp}/check-gen-fresh.$$.log" >&2; rm -f "${TMPDIR:-/tmp}/check-gen-fresh.$$.log"; echo "check-gen-fresh: bazel build failed" >&2; exit 2; }
rm -f "${TMPDIR:-/tmp}/check-gen-fresh.$$.log"
# shellcheck disable=SC2015  # the || arm reports an unresolvable bazel-bin, which is the intended failure for either test
bin=$(bazel info bazel-bin 2>/dev/null) && [ -d "$bin" ] || { echo "check-gen-fresh: cannot resolve bazel-bin" >&2; exit 2; }

tree_hash() { # <dir> <find args...>: sha256 over sorted "path sha256" lines
  local d=$1; shift
  ( cd "$d" 2>/dev/null && find -L . -type f "$@" -print0 | LC_ALL=C sort -z | xargs -0 -r sha256sum ) | sha256sum | cut -d' ' -f1
}
# Mirrors sync_gen.sh: ent .go only (bazel-bin also holds compiled libraries and entc), no schema/entc dirs.
ent_filter=(-name '*.go' ! -path './entc/*' ! -path './schema/*' ! -path './*.runfiles/*' ! -name 'generate.go')
bad=""
[ "$(tree_hash "$bin/proto/session/v1/gen/proto/go")" = "$(tree_hash gen/proto/go)" ] || bad="$bad gen/proto/go"
# session/ent: only the generated files (everything but schema/ and generate.go).
ent_synced=$(cd session/ent 2>/dev/null && find . -type f -name '*.go' ! -path './schema/*' ! -name generate.go -print0 | LC_ALL=C sort -z | xargs -0 -r sha256sum | sha256sum | cut -d' ' -f1)
[ "$(tree_hash "$bin/tools/bzl/ent" "${ent_filter[@]}")" = "$ent_synced" ] || bad="$bad session/ent"
if [ -n "$bad" ]; then
  echo "gen: STALE (differs from Bazel output:$bad)"
  echo "run: make sync"
  exit 1
fi
echo "gen: fresh (matches bazel-bin)"
