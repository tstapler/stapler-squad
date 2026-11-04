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
bash "$c" --write
[ -z "$(bash "$c" --note)" ] || fail "--note must be silent when fresh"
bash "$c" --strict >/dev/null || fail "--strict must pass when fresh"
echo '// z' >> buf.yaml
case "$(bash "$c" --note)" in "note: gen/ is stale for plain go and gopls; Bazel is unaffected. Run 'make sync'.") ;; *) fail "--note text" ;; esac
bash "$c" --note >/dev/null || fail "--note must exit 0"
out=$(bash "$c" --strict) && fail "--strict must fail on a stale stamp"
[ "$out" = "gen: STALE - run 'make sync'" ] || fail "--strict text: $out"
rm gen/.sync-inputs.sha256
bash "$c" --strict >/dev/null || fail "--strict must pass with no stamp"
echo PASS
