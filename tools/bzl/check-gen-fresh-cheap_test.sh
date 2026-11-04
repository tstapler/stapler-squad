#!/usr/bin/env bash
# Test for check-gen-fresh-cheap.sh in a throwaway git repo (never touches the real tree).
set -eu
here=$(cd "$(dirname "$0")" && pwd)
tmp=$(mktemp -d "${TMPDIR:-/tmp}/gen-fresh-cheap.XXXXXX"); trap 'rm -rf "$tmp"' EXIT
cd "$tmp"; git init -q .
mkdir -p proto/session/v1 session/ent/schema
echo 'syntax = "proto3";' > proto/session/v1/a.proto
echo 'version: v2' > buf.yaml
echo 'package schema' > session/ent/schema/x.go
echo 'package ent' > session/ent/generate.go
c="$here/check-gen-fresh-cheap.sh"
fail() { echo "FAIL: $*" >&2; exit 1; }
out=$(bash "$c") && fail "no stamp must be stale"
[ "$out" = "gen: STALE - run 'make sync'" ] || fail "stale text: $out"
bash "$c" --write
[ "$(bash "$c")" = "gen: fresh" ] || fail "fresh after write"
echo '// x' >> proto/session/v1/a.proto
out=$(bash "$c") && fail "changed proto must be stale"
bash "$c" --warn >/dev/null || fail "--warn must exit 0"
bash "$c" --write
echo '// y' >> session/ent/schema/x.go
bash "$c" >/dev/null && fail "changed schema must be stale"
bash "$c" --write
echo '# y' >> buf.yaml
bash "$c" >/dev/null && fail "changed buf.yaml must be stale"
echo PASS
