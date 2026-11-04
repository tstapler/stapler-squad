#!/usr/bin/env bash
# Verbatim contract test for tools/bzl/translate-errors.sh. --negative checks the negative fixtures
# shellcheck disable=SC2015  # A && pass || fail: pass() never fails
# and the E1-versus-E8 pair. Golden hint text lives in testdata/errors/E<n>.hint.
set -u
here=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)
root=$(cd "$here/../.." && pwd)
tr_sh="$here/translate-errors.sh"
d="$here/testdata/errors"
failures=0
pass() { echo "PASS $1"; }
fail() { echo "FAIL $1" >&2; failures=$((failures + 1)); }
hint_count() { grep -c 'bazel-hint:' ; }

# Hint lines only (the doc pointer line is checked separately).
hints_of() { "$tr_sh" "$@" | grep 'bazel-hint:'; }

if [ "${1:-}" = "--negative" ]; then
  for f in "$d"/negative/*.log; do
    n=$("$tr_sh" --exit 0 < "$f" | hint_count)
    [ "$n" -eq 0 ] && pass "negative/$(basename "$f")" || fail "negative/$(basename "$f") printed a hint"
  done
  # Bare Killed counts only with exit status 137.
  n=$("$tr_sh" --exit 1 < "$d/negative/killed-passing-test.log" | hint_count)
  [ "$n" -eq 0 ] && pass "Killed without 137 gives no hint" || fail "Killed without 137 gave a hint"
  # E1 versus E8: same words, with and without a sandbox marker.
  a=$(hints_of --exit 1 < "$d/E1-vs-E8-no-marker.log"); b=$(hints_of --exit 1 < "$d/E8.log")
  [ "$a" = "$(cat "$d/E1.hint")" ] && [ "$(printf '%s\n' "$a" | wc -l)" -eq 1 ] && pass "no marker -> E1 only" || fail "no marker did not give exactly E1"
  [ "$b" = "$(cat "$d/E8.hint")" ] && [ "$(printf '%s\n' "$b" | wc -l)" -eq 1 ] && pass "sandbox marker -> E8 only" || fail "marker did not give exactly E8"
  [ "$failures" -eq 0 ] || exit 1
  exit 0
fi

check_row() { # <row> <source> <exit> <log> <golden>
  local row=$1 src=$2 rc=$3 log=$4 golden=$5 out got first
  out=$("$tr_sh" --source "$src" --exit "$rc" < "$log")
  got=$(printf '%s\n' "$out" | grep 'bazel-hint:')
  first=$(grep -m1 '^ERROR:' "$log" || true)
  if [ "$got" != "$(cat "$golden")" ]; then fail "$row hint differs"; return; fi
  if [ -n "$first" ] && ! printf '%s\n' "$out" | grep -qxF "$first"; then fail "$row lost the original ERROR line"; return; fi
  pass "$row"
}
for n in 1 2 4 6 8 9 10 11; do
  rc=1; [ "$n" = 6 ] || [ "$n" = 11 ] && rc=0
  check_row "E$n" bazel "$rc" "$d/E$n.log" "$d/E$n.hint"
done
check_row E3 go 1 "$d/E3.log" "$d/E3.hint"
golden=$d/E5.fallback.hint
[ -e "$root/.claude/skills/worktree-disk-cleanup/bazel-orphan-outputbases.sh" ] && golden=$d/E5.hint
check_row E5 bazel 1 "$d/E5.log" "$golden"
# E7 is emitted by the wrapper from the template, not parsed.
[ "$("$tr_sh" --text E7)" = "$(cat "$d/E7.hint")" ] && pass E7 || fail "E7 template differs"
# Bare Killed with exit 137 is E4.
printf 'Killed\n' | "$tr_sh" --exit 137 | grep -qxF "$(cat "$d/E4.hint")" && pass "E4 bare Killed with 137" || fail "E4 bare Killed with 137"
# Doc pointer follows hints of rows with a troubleshooting section.
"$tr_sh" --exit 1 < "$d/E1.log" | grep -q 'See docs/how-to/troubleshoot-bazel.md#state-1' && pass "doc pointer" || fail "doc pointer"
[ "$failures" -eq 0 ] || { echo "$failures failure(s)" >&2; exit 1; }
