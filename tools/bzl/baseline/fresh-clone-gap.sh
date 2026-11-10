#!/usr/bin/env bash
# Task 0.1c: quantify the hermetic-clone gap. Clones HEAD into a short temp
# dir, runs `go build ./...` with no generation, and prints JSON with the exit
# code and the missing generated packages. Package loading fails before any
# compilation, so this is cheap; it still runs niced with -p 1.
# usage: fresh-clone-gap.sh [--src <repo-or-url>]
set -euo pipefail
# shellcheck source=lib.sh
. "$(dirname "${BASH_SOURCE[0]}")/lib.sh"
need git; need go; need jq
SRC="$BZL_REPO_ROOT"
while (($#)); do
  case "$1" in
    --src) SRC="$2"; shift 2 ;;
    -h|--help) sed -n '2,7p' "$0"; exit 0 ;;
    *) die "unknown arg: $1" ;;
  esac
done
D="$(mktemp -d /tmp/bzlfc.XXXXXX)"
trap 'rm -rf "$D"' EXIT
git clone --quiet --no-hardlinks "$SRC" "$D/c"
cd "$D/c"
set +e
out="$(nice -n 19 timeout 600 go build -p 1 ./... 2>&1)"
rc=$?
set -e
missing="$(grep -oE '(github\.com/[^" ]+/(gen/proto/go|session/ent)[^" :;]*)' <<<"$out" | sort -u || true)"
sample="$(head -40 <<<"$out")"
jq -n --argjson rc "$rc" --arg head "$(git rev-parse --short HEAD)" --arg m "$missing" --arg s "$sample" '
  {head: $head, go_build_exit: $rc,
   missing_generated_packages: ($m|split("\n")|map(select(length>0))),
   missing_count: ($m|split("\n")|map(select(length>0))|length),
   first_40_lines: $s}'
