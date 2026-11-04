#!/usr/bin/env bash
# P4 parity, ent half: byte-compare //tools/bzl/ent:ent_gen output with `make ent-gen`'s command
# (`ent generate <ent_flags.txt flags> ./session/ent/schema`) run in a scratch copy, after gofmt.
# The scratch copy holds only go.mod, go.sum and the non-test schema files, so the repo is untouched.
# PASS iff exit 0 (empty diff). Needs the Go module cache or network for entgo.io/ent/cmd/ent.
set -u
cd "$(git rev-parse --show-toplevel)" || exit 2
command -v bazel >/dev/null || bazel() { BAZEL_REAL=${BAZEL_REAL:-$(command -v bazelisk)} tools/bazel "$@"; }
tmp=$(mktemp -d "${TMPDIR:-/tmp}/compare-ent.XXXXXX"); trap 'rm -rf "$tmp"' EXIT
mkdir -p "$tmp/ref/session/ent/schema"
cp go.mod go.sum "$tmp/ref/"
find session/ent/schema -maxdepth 1 -name '*.go' ! -name '*_test.go' -exec cp {} "$tmp/ref/session/ent/schema/" \;
# shellcheck disable=SC2046 # the flags file holds several words on purpose, as in the Makefile
( cd "$tmp/ref" && go run -mod=mod entgo.io/ent/cmd/ent generate $(cat "$OLDPWD/tools/bzl/ent/ent_flags.txt") ./session/ent/schema ) || { echo "compare-ent: reference generation failed" >&2; exit 2; }
bazel build --config=agent --sandbox_default_allow_network=false //tools/bzl/ent:ent_gen 2>&1 | tail -3; [ "${PIPESTATUS[0]}" = 0 ] || { echo "compare-ent: bazel build failed" >&2; exit 2; }
got=$(bazel info --config=agent bazel-bin 2>/dev/null)/tools/bzl/ent
[ -d "$got" ] || { echo "compare-ent: $got missing" >&2; exit 2; }
# .go only: bazel-bin also holds compiled libraries, test scripts and the entc binary.
mkdir -p "$tmp/bazel"
( cd "$got" && find -L . -name '*.go' ! -path './entc/*' ! -path './schema/*' ! -path '*.runfiles/*' -print0 | cpio -0pdmL --quiet "$tmp/bazel" )
chmod -R u+w "$tmp/bazel"
gofmt -w "$tmp/bazel" "$tmp/ref/session/ent" 2>/dev/null
if diff -r -x schema -x go.mod -x go.sum "$tmp/ref/session/ent" "$tmp/bazel" > "$tmp/diff.txt" 2>&1; then
  echo "compare-ent: identical ($(find "$tmp/bazel" -name '*.go' | wc -l) files)"; exit 0
fi
echo "compare-ent: DIFFERENT; $(grep -c '^Only in' "$tmp/diff.txt") only-in lines, $(grep -c '^diff ' "$tmp/diff.txt") differing files" >&2
head -"${COMPARE_ENT_HEAD:-40}" "$tmp/diff.txt" >&2
exit 1
