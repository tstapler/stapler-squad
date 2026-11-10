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
  mkdir -p "$T/help"; printf '  --jobs (an integer)\n' > "$T/help/build.txt"; : > "$T/help/startup_options.txt"
}
# rc <line>: append an rc line whose flags are known to both the fake help and the results file
rc() {
  echo "$1" >> "$T/r/.bazelrc"
  local f n
  for f in $(echo "$1" | grep -oE -- '--[a-z_]+'); do
    n=${f#--}
    printf '| %s | spike-verified | q |\n' "$f" >> "$T/r/project_plans/bazel-conversion/implementation/spike-results.md"
    case "$1" in startup*) printf '  --%s (x)\n' "$n" >> "$T/help/startup_options.txt" ;; *) printf '  --%s (x)\n' "$n" >> "$T/help/build.txt" ;; esac
  done
}
expect() { # expect <exit> <label>
  BZL_CHECK_ROOT="$T/r" BZL_CHECK_HELP_DIR="$T/help" BZL_CHECK_SKIP_MOD_GRAPH=1 bash "$here/check-bazelrc.sh" >/dev/null 2>&1
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
# E9: startup:<config> ignored by Bazel; startup:<platform> is honoured
fresh; rc 'startup:linux --max_idle_secs=30'; expect 0 "startup:linux accepted (E9)"
fresh; rc 'startup:agent --max_idle_secs=30'; expect 1 "startup:agent rejected (E9)"
# flags absent on the pinned version, even when a results row and help line exist
fresh; rc 'build --local_ram_resources=2048'; expect 1 "--local_ram_resources rejected"
fresh; rc 'build --local_cpu_resources=2'; expect 1 "--local_cpu_resources rejected"
fresh; rc 'build --incompatible_sandbox_hermetic_tmp'; expect 1 "--incompatible_sandbox_hermetic_tmp rejected"
fresh; rc 'build --experimental_guard_against_concurrent_changes'; expect 1 "--experimental_guard_against_concurrent_changes rejected"
fresh; rc 'startup --digest_function=blake3'; expect 1 "--digest_function rejected"
fresh; rc 'build --action_env=PATH'; expect 1 "--action_env=PATH rejected"
fresh; rc 'build --profile=/tmp/p.gz'; expect 1 "--profile= in an rc rejected"
fresh; rc 'build --remote_local_fallback'; expect 1 "--remote_local_fallback without the incompatible flag rejected"
fresh; rc 'build --remote_local_fallback'; rc 'build --incompatible_remote_local_fallback_for_remote_cache'; expect 0 "--remote_local_fallback with the incompatible flag accepted"
fresh; rc 'build:agent --color=no'; rc 'build:agent --noshow_progress'; expect 0 "agent presentation flags accepted"
fresh; rc 'build:agent --jobs=1'; expect 1 "agent flag outside the presentation allow-list rejected"
fresh; rc 'build:agent --disk_cache=/x'; expect 1 "agent --disk_cache rejected"
fresh; rc 'build:ci-cache-read --remote_timeout=3s'; rc 'build:ci-cache-write --remote_timeout=3s'; expect 0 "matching ci-cache policy accepted"
fresh; rc 'build:ci-cache-read --remote_timeout=3s'; rc 'build:ci-cache-write --remote_timeout=9s'; expect 1 "differing ci-cache policy rejected"
fresh; rc 'build --flag_not_in_help_but_verified'; sed -i '/flag_not_in_help_but_verified/d' "$T/help/build.txt"; expect 1 "flag absent from bazel help rejected"
fresh; rm -r "$T/help"; BZL_CHECK_ROOT="$T/r" BZL_CHECK_HELP_DIR="$T/nohelp" BZL_CHECK_SKIP_MOD_GRAPH=1 bash "$here/check-bazelrc.sh" >/dev/null 2>&1; got=$?; if [ "$got" = 1 ]; then echo "ok   - unreadable bazel help is a failure"; else echo "FAIL - unreadable help passed ($got)" >&2; fail=1; fi
exit $fail
