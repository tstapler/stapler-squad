#!/usr/bin/env bash
# Task 4.8a / P1, P2: compare the passing Go test names of a live `go test` run (gotestsum --jsonfile)
# with those of a live `bazel test` run on the same commit, for one TagConfig.
#
#   test-set-diff.sh <default|race|integration|sinkguard|gitoracle>   run both sides, then compare
#   test-set-diff.sh <config> --go-json <file> --testlogs <dir>       compare existing outputs only
#
# Prints `missing_in_bazel: <n>` (passed under go test, not under Bazel, excluding packages whose
# target is in tools/bzl/test-exclusions.yaml), `extra_in_bazel: <n>`, then the missing names, and
# exits 0 only when missing_in_bazel is 0. DIFF_OUT (default: a temp dir) keeps both name lists.
# Heavy when it runs both sides; BAZEL_REAL must point at bazelisk. Bazel flags: TSD_BAZEL_FLAGS.
set -u
here=$(cd "$(dirname "$0")" && pwd)
root=$(cd "$here/../../.." && pwd)
cd "$root" || exit 2

cfg=${1:-}
[ -n "$cfg" ] || { sed -n '2,11p' "$0"; exit 2; }
shift
gojson="" testlogs=""
while [ $# -gt 0 ]; do
  case "$1" in
    --go-json) gojson=$2; shift 2 ;;
    --testlogs) testlogs=$2; shift 2 ;;
    *) echo "test-set-diff: unknown arg $1" >&2; exit 2 ;;
  esac
done

case "$cfg" in
  default) go_args=(-short -timeout=20m); bz_cfg=() ;;
  race) go_args=(-race -short -timeout=20m); bz_cfg=(--config=race) ;;
  integration) go_args=(-race -tags integration -timeout 20m); bz_cfg=(--config=integration) ;;
  sinkguard) go_args=(-tags sinkguard -timeout 20m); bz_cfg=(--config=sinkguard) ;;
  gitoracle) go_args=(-race -tags gitoracle -timeout 10m); bz_cfg=(--config=gitoracle) ;;
  *) echo "test-set-diff: unknown config $cfg" >&2; exit 2 ;;
esac

out="${DIFF_OUT:-$(mktemp -d "${TMPDIR:-/tmp}/tsd.XXXXXX")}"
mkdir -p "$out"
module=$(go list -m)
summary=(go run ./tools/bzl/testsummary)

if [ -z "$gojson" ]; then
  gojson="$out/go.json"
  gotestsum --jsonfile "$gojson" --format pkgname-and-test-fails -- "${go_args[@]}" ./... >"$out/go.log" 2>&1 || true
fi
if [ -z "$testlogs" ]; then
  export BAZEL_REAL="${BAZEL_REAL:-$(command -v bazelisk)}"
  # shellcheck disable=SC2206
  flags=(${TSD_BAZEL_FLAGS:---jobs=4 --local_resources=memory=3000 --local_test_jobs=2})
  tools/bazel test //... "${bz_cfg[@]}" "${flags[@]}" --test_tag_filters=-manual --keep_going \
    --test_env=GO_TEST_WRAP_TESTV=1 --test_output=errors >"$out/bazel.log" 2>&1 || true
  testlogs=$(tools/bazel info bazel-testlogs 2>/dev/null)
fi

"${summary[@]}" -gojson "$gojson" | sort -u >"$out/go.names" || exit 2
"${summary[@]}" -testlogs "$testlogs" -module "$module" | sort -u >"$out/bazel.names" || exit 2

# Packages of excluded targets (//pkg:name -> <module>/pkg/) are accepted deltas.
sed -n 's|^  - target: //\([^:]*\):.*|'"$module"'/\1/|p' tools/bzl/test-exclusions.yaml >"$out/excluded.prefixes"

comm -23 "$out/go.names" "$out/bazel.names" >"$out/missing.all"
missing="$out/missing.names"
: >"$missing"
while IFS= read -r name; do
  skip=0
  while IFS= read -r prefix; do
    # Direct package only: <pkg>/<Test...>, not <pkg>/<subpkg>/<Test...>.
    rest=${name#"$prefix"}
    if [ "$rest" != "$name" ] && [[ $rest == Test* || $rest == Example* || $rest == Fuzz* ]]; then skip=1; break; fi
  done <"$out/excluded.prefixes"
  [ $skip -eq 1 ] || printf '%s\n' "$name" >>"$missing"
done <"$out/missing.all"

n_missing=$(wc -l <"$missing" | tr -d ' ')
n_extra=$(comm -13 "$out/go.names" "$out/bazel.names" | wc -l | tr -d ' ')
echo "config: $cfg"
echo "go_passed: $(wc -l <"$out/go.names" | tr -d ' ')"
echo "bazel_passed: $(wc -l <"$out/bazel.names" | tr -d ' ')"
echo "extra_in_bazel: $n_extra"
echo "missing_in_bazel: $n_missing"
[ "$n_missing" -eq 0 ] || { head -n 50 "$missing" | sed 's/^/  missing: /'; echo "(full lists in $out)"; exit 1; }
exit 0
