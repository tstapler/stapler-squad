#!/usr/bin/env bash
# One seeded violation per rule of check-bazelrc.sh; each must exit 1, the clean tree 0.
set -u
here=$(cd "$(dirname "$0")" && pwd)
T=$(mktemp -d "${TMPDIR:-/tmp}/bazelrc-test.XXXXXX"); trap 'rm -rf "$T"' EXIT
fail=0
fresh() {
  rm -rf "$T/r"; mkdir -p "$T/r/project_plans/bazel-conversion/implementation"
  ( cd "$T/r" && git init -q . )
  printf 'build --jobs=4\n' > "$T/r/.bazelrc"
  printf '| --jobs | spike-verified | quoted |\n' > "$T/r/project_plans/bazel-conversion/implementation/spike-results.md"
}
expect() { # expect <exit> <label>
  BZL_CHECK_ROOT="$T/r" BZL_CHECK_SKIP_MOD_GRAPH=1 bash "$here/check-bazelrc.sh" >/dev/null 2>&1
  local got=$?
  if [ "$got" = "$1" ]; then echo "ok   - $2"; else echo "FAIL - $2 (exit $got, want $1)" >&2; fail=1; fi
}
fresh; expect 0 "clean tree passes"
fresh; touch "$T/r/WORKSPACE"; expect 1 "WORKSPACE rejected"
fresh; touch "$T/r/WORKSPACE.bazel"; expect 1 "WORKSPACE.bazel rejected"
fresh; echo 'common --enable_workspace' >> "$T/r/.bazelrc"; expect 1 "--enable_workspace rejected"
fresh; echo 'common --enable_bzlmod=false' >> "$T/r/.bazelrc"; expect 1 "--enable_bzlmod=false rejected"
fresh; echo 'build --spawn_strategy=local' >> "$T/r/.bazelrc"; expect 1 "--spawn_strategy=local rejected"
fresh; echo 'build --remote_executor=grpc://x' >> "$T/r/.bazelrc"; expect 1 "--remote_executor rejected"
fresh; echo 'build --remote_header=Authorization=Bearer abc' >> "$T/r/.bazelrc"; expect 1 "--remote_header secret rejected"
fresh; echo 'test --flaky_test_attempts=3' >> "$T/r/.bazelrc"; expect 1 "--flaky_test_attempts=3 rejected"
fresh; printf 'go_test(name = "t", flaky = True)\n' > "$T/r/BUILD.bazel"; ( cd "$T/r" && git add BUILD.bazel ); expect 1 "flaky = True rejected"
fresh; echo 'startup:agent --max_idle_secs=30' >> "$T/r/.bazelrc"; expect 1 "startup:<config> rejected"
fresh; echo 'build --made_up_flag=1' >> "$T/r/.bazelrc"; expect 1 "flag without spike-verified row rejected"
fresh; rm "$T/r/project_plans/bazel-conversion/implementation/spike-results.md"; expect 1 "missing spike-results.md is a failure, not a pass"
exit $fail
