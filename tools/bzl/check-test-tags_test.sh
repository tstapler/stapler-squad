#!/usr/bin/env bash
# Seeded cases for check-test-tags.sh. Prints one ok/FAIL line per case and PASS when all behave.
set -u
here=$(cd "$(dirname "$0")" && pwd)
T=$(mktemp -d "${TMPDIR:-/tmp}/ctt.XXXXXX"); trap 'rm -rf "$T"' EXIT
fail=0
expect() { # expect <exit> <label>
  BZL_TAGS_ROOT="$T" bash "$here/check-test-tags.sh" >"$T.out" 2>&1
  local got=$?
  if [ "$got" = "$1" ]; then echo "ok   - $2"; else echo "FAIL - $2 (exit $got, want $1): $(cat "$T.out")" >&2; fail=1; fi
}
mkdir -p "$T/pkg"
printf 'go_test(\n    name = "t",\n    tags = ["external"],  # tag-reason: reads the host tmux (#123)\n)\n' > "$T/pkg/BUILD.bazel"
expect 0 "PASS: same-line reason with issue"
printf 'go_test(\n    name = "t",\n    # tag-reason: reads the host tmux (#123)\n    tags = ["external"],\n)\n' > "$T/pkg/BUILD.bazel"
expect 0 "PASS: preceding-line reason with issue"
printf 'go_test(\n    name = "t",\n    tags = ["no-sandbox"],\n)\n' > "$T/pkg/BUILD.bazel"
expect 1 "FAIL: tag without comment"
printf 'go_test(\n    name = "t",\n    tags = ["local"],  # tag-reason: reads the host tmux\n)\n' > "$T/pkg/BUILD.bazel"
expect 1 "FAIL: comment lacks (#issue)"
printf 'go_test(\n    name = "t",\n    tags = ["manual"],\n)\n' > "$T/pkg/BUILD.bazel"
expect 0 "PASS: untracked tag"
printf 'test --test_tag_filters=-external\n' > "$T/.bazelrc"
rm -f "$T/pkg/BUILD.bazel"
expect 1 "FAIL: .bazelrc tag filter without comment"
printf '# tag-reason: skip host-coupled tests in the hermetic job (#123)\ntest:hermetic --test_tag_filters=-external\n' > "$T/.bazelrc"
expect 0 "PASS: .bazelrc with reason"
if [ "$fail" = 0 ]; then echo PASS; fi
exit $fail
