#!/usr/bin/env bash
# Task 0.1h: run the 9 seeded failure fixtures through the make path and record
# per fixture whether the message names a remedy.
#   names_remedy: true | false | "N/A-make" (failure cannot occur without Bazel)
# A remedy is "named" only when the output matches the fixture's REMEDY_RE (an
# actionable command or instruction that would actually fix the failure, e.g.
# `make proto-gen` for missing generated code; Go's own `go get` hint for a
# generated package is NOT a correct remedy and counts as false).
# Each fixture runs in its own throwaway worktree (/tmp/bzlfixture.*), removed
# afterwards. The commands are cheap (go build of one package or a trivial
# main), so no quiet-host guard; they never run make build.
# Also writes fixtures/expected.key (fixture -> errors.yaml row).
# usage: run-make-path.sh [--out <json>]
set -euo pipefail
HERE="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=../lib.sh
. "$HERE/../lib.sh"
need git; need jq
OUT=""
while (($#)); do
  case "$1" in
    --out) OUT="$2"; shift 2 ;;
    -h|--help) sed -n '2,15p' "$0"; exit 0 ;;
    *) die "unknown arg: $1" ;;
  esac
done

: > "$HERE/expected.key"
rows=()
for f in "$HERE"/[0-9]*.sh; do
  name="$(basename "$f" .sh)"
  key="$(bash "$f" --key)"; re="$(bash "$f" --remedy-re)"; applicable="$(bash "$f" --make-applicable)"
  printf '%s %s\n' "$name" "$key" >> "$HERE/expected.key"
  wt="$(mktemp -d /tmp/bzlfixture.XXXXXX)"
  git -C "$BZL_REPO_ROOT" worktree add --quiet --detach "$wt/wt" HEAD
  rc=0
  out="$(bash "$f" --run make "$wt/wt" 2>&1)" || rc=$?
  git -C "$BZL_REPO_ROOT" worktree remove --force "$wt/wt"; rm -rf "$wt"
  if [[ "$applicable" != true ]]; then names=\"N/A-make\"
  elif [[ -n "$re" ]] && grep -qE "$re" <<<"$out"; then names=true
  else names=false; fi
  rows+=("$(jq -c -n --arg fixture "$name" --arg key "$key" --argjson names "$names" --argjson rc "$rc" \
    --arg re "$re" --arg out "$(head -c 1500 <<<"$out")" --argjson lines "$(wc -l <<<"$out")" \
    '{fixture:$fixture, expected_key:$key, command_exit:$rc, names_remedy:$names, remedy_re:$re, output_lines:$lines, output_head:$out}')")
done
git -C "$BZL_REPO_ROOT" worktree prune
res="$(printf '%s\n' "${rows[@]}" | jq -s --arg d "$(date -u +%FT%TZ)" '{run_date:$d, fixtures:.}')"
[[ -n "$OUT" ]] && printf '%s\n' "$res" > "$OUT"
printf '%s\n' "$res"
