#!/usr/bin/env bash
# Task 4.2b. Two modes:
#   shard-union.sh <go_test target>   run it sharded (--test_sharding_strategy=explicit) and unsharded
#                                     (=disabled), write the passing test names of each to
#                                     sharded.names / unsharded.names in $SHARD_UNION_OUT (default: cwd),
#                                     print "SHARD UNION EQUAL n=<count>" and exit 0 only if the sets match.
#   shard-union.sh --filter-semantics probe how --test_filter reaches the Go test binary and print
#                                     "filter: <passthrough|translated> <anchored|unanchored>".
# Bazel flags default to the host-containment pair; override with SHARD_UNION_BAZEL_FLAGS.
# BAZEL_REAL must point at bazelisk (tools/bazel needs it).
set -u
here=$(cd "$(dirname "$0")" && pwd)
root=$(cd "$here/../../.." && pwd)
cd "$root" || exit 2
out="${SHARD_UNION_OUT:-$PWD}"
mkdir -p "$out"
# shellcheck disable=SC2206
flags=(${SHARD_UNION_BAZEL_FLAGS:---jobs=4 --local_resources=memory=3000})
common=("${flags[@]}" --nocache_test_results --test_env=GO_TEST_WRAP_TESTV=1 --test_output=errors)
export BAZEL_REAL="${BAZEL_REAL:-$(command -v bazelisk)}"
export BAZEL_HUMAN=1

bz() { "$root/tools/bazel" "$@"; }

# Passing test names (testcase without failure/error/skipped) under a target's testlogs dir.
passing_names() { # passing_names <target>
  local t=${1#//} dir
  dir="$(bz info bazel-testlogs 2>/dev/null)/${t%%:*}/${t##*:}"
  python3 -I - "$dir" <<'PY'
import sys, glob, os
import xml.etree.ElementTree as ET
d = sys.argv[1]
files = sorted(glob.glob(os.path.join(d, "shard_*_of_*", "test.xml"))) or [os.path.join(d, "test.xml")]
names = set()
for f in files:
    for tc in ET.parse(f).getroot().iter("testcase"):
        if not any(tc.find(k) is not None for k in ("failure", "error", "skipped")):
            names.add(tc.get("name"))
print("\n".join(sorted(names)))
PY
}

if [ "${1:-}" = "--filter-semantics" ]; then
  target=//internal/testenv:testenv_test
  ran() { # ran <filter>: names of tests that ran
    bz test "${common[@]}" --test_filter="$1" "$target" >/dev/null 2>&1
    passing_names "$target"
  }
  # `TestLiveState` is a strict prefix of TestLiveStateError (+ subtests): anchored would run neither.
  prefix=$(ran 'TestLiveState')
  alt=$(ran 'TestIsolatedHome|TestShortTempDir')
  anchored=unanchored
  if ! printf '%s\n' "$prefix" | grep -q '^TestLiveStateError$'; then anchored=anchored; fi
  form=translated
  if printf '%s\n' "$alt" | grep -q '^TestIsolatedHome' && printf '%s\n' "$alt" | grep -q '^TestShortTempDir'; then form=passthrough; fi
  echo "filter: $form $anchored"
  exit 0
fi

target=${1:?usage: shard-union.sh <target> | --filter-semantics}
bz test "${common[@]}" --test_sharding_strategy=explicit "$target" || echo "note: sharded run had failing tests (compared by passing names)" >&2
passing_names "$target" > "$out/sharded.names"
bz test "${common[@]}" --test_sharding_strategy=disabled "$target" || echo "note: unsharded run had failing tests (compared by passing names)" >&2
passing_names "$target" > "$out/unsharded.names"
if [ ! -s "$out/sharded.names" ] || [ ! -s "$out/unsharded.names" ]; then
  echo "shard-union: no passing test names collected" >&2
  exit 1
fi
if diff <(sort "$out/sharded.names") <(sort "$out/unsharded.names") >"$out/shard-union.diff"; then
  echo "SHARD UNION EQUAL n=$(wc -l < "$out/sharded.names")"
  exit 0
fi
echo "SHARD UNION DIFFERS (see $out/shard-union.diff)" >&2
exit 1
