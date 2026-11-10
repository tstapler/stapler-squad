#!/usr/bin/env bash
# Usage: upsert_test.sh <ent_flags.txt> <generated .go files...>
# Fails unless the flag file enables sql/upsert and the generated code has OnConflict builders.
set -eu
flags=$1; shift
grep -qx -- '--feature sql/upsert' "$flags" || { echo "upsert_test: $flags lacks '--feature sql/upsert'" >&2; exit 1; }
n=$(grep -l 'OnConflict' "$@" | wc -l)
[ "$n" -gt 0 ] || { echo "upsert_test: no OnConflict builders in generated ent code" >&2; exit 1; }
echo "upsert_test: $n generated files contain OnConflict"
