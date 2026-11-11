#!/usr/bin/env bash
# Usage: pe_header_test.sh <path-to-.exe>. Fails unless the file is a PE image.
set -eu
exe="${1:?usage: pe_header_test.sh <exe>}"
[ "$(head -c2 "$exe")" = "MZ" ] || { echo "pe_header_test: $exe has no MZ header" >&2; exit 1; }
off=$(od -An -t u4 -j 60 -N 4 "$exe" | tr -d ' ')
sig=$(od -An -t x1 -j "$off" -N 4 "$exe" | tr -d ' ')
[ "$sig" = "50450000" ] || { echo "pe_header_test: no PE signature at offset $off (got $sig)" >&2; exit 1; }
echo "pe_header_test: $exe is a PE image"
