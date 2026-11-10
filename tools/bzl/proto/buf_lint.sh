#!/usr/bin/env bash
# buf lint as a hermetic test with a ratchet baseline (Story 2.5).
#   buf_lint.sh <buf> <buf.yaml> <baseline> <proto>...           pass iff no finding outside the baseline
#   buf_lint.sh --seeded <buf> <buf.yaml> <baseline> <proto>...  pass iff a seeded violation is reported by rule name
#   BUILD_WORKSPACE_DIRECTORY=<ws> buf_lint.sh --update ...      rewrite the baseline from current findings
# A finding is "<path> <RULE> <message>" (line numbers dropped so edits above it do not churn the baseline).
# The ratchet fails both ways: a new finding, or a baseline entry that no longer fires (shrink the file).
# `buf breaking` stays outside Bazel: it needs git history (ADR-005).
set -eu
mode=lint
case "${1:-}" in --seeded|--update) mode=${1#--}; shift ;; esac
buf=$(realpath "$1"); cfg=$(realpath "$2"); baseline=$(realpath "$3"); shift 3
work=$(mktemp -d "${TEST_TMPDIR:-${TMPDIR:-/tmp}}/buf-lint.XXXXXX")
export HOME=$work BUF_CACHE_DIR=$work/cache
cp "$cfg" "$work/buf.yaml"
for p in "$@"; do mkdir -p "$work/$(dirname "$p")"; cp -L "$p" "$work/$p"; done
cd "$work"

lint() { # prints sorted normalised findings; buf exits 100 when there are findings
  "$buf" lint proto --error-format=json 2>"$work/stderr" | sed -n 's/.*"path":"\([^"]*\)".*"type":"\([^"]*\)","message":"\(.*\)"}$/\1 \2 \3/p' | LC_ALL=C sort
  if [ -s "$work/stderr" ]; then cat "$work/stderr" >&2; return 2; fi
}

if [ "$mode" = seeded ]; then
  set -- proto/session/v1/*.proto; first=$1
  printf '\nmessage seeded_lint_violation {}\n' >> "$first"
  out=$(lint) || { echo "buf_lint --seeded: buf failed to run" >&2; exit 2; }
  if printf '%s\n' "$out" | grep -q 'seeded_lint_violation'; then
    printf '%s\n' "$out" | grep 'seeded_lint_violation' | head -1; echo "seeded violation reported with its rule name: ok"; exit 0
  fi
  echo "buf_lint --seeded: the seeded violation was not reported" >&2; exit 1
fi

cur=$work/current.txt; lint > "$cur" || { echo "buf_lint: buf failed to run" >&2; exit 2; }
if [ "$mode" = update ]; then
  ws=${BUILD_WORKSPACE_DIRECTORY:?run via bazel run}
  { echo "# buf lint findings tolerated until the owner decides to fix them (tools/bzl/proto/buf_lint.sh ratchet)."
    echo "# Regenerate: bazel run //proto/session/v1:buf_lint_update_baseline. Entries only ever leave this file."
    cat "$cur"; } > "$ws/tools/bzl/proto/buf-lint-baseline.txt"
  echo "buf_lint: baseline now holds $(wc -l < "$cur") findings"; exit 0
fi
grep -v '^#' "$baseline" | LC_ALL=C sort > "$work/base.txt" || true
new=$(LC_ALL=C comm -23 "$cur" "$work/base.txt"); fixed=$(LC_ALL=C comm -13 "$cur" "$work/base.txt")
rc=0
if [ -n "$new" ]; then echo "buf lint: NEW findings (rule name is the second word):" >&2; printf '%s\n' "$new" >&2; rc=1; fi
if [ -n "$fixed" ]; then echo "buf lint: baseline lists findings that no longer fire; remove them (bazel run //proto/session/v1:buf_lint_update_baseline):" >&2; printf '%s\n' "$fixed" >&2; rc=1; fi
[ "$rc" = 0 ] && echo "buf lint: no new findings ($(wc -l < "$cur") baselined)"
exit $rc
