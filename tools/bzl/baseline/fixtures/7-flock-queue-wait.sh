#!/usr/bin/env bash
# Fixture 7-flock-queue-wait.sh: seeded failure for errors.yaml row E7.
# Bazel-mode body is PROVISIONAL: it needs the Gazelle-generated BUILD files and wrapper (Tasks 1.1a, 0.2f) and is finalised when Task 1.6d replays it.
set -uo pipefail
# shellcheck disable=SC2034  # KEY, REMEDY_RE, MAKE_APPLICABLE are read by fixture_main in common.sh
# shellcheck source=common.sh
. "$(dirname "${BASH_SOURCE[0]}")/common.sh"
KEY=E7
REMEDY_RE=''
MAKE_APPLICABLE=false
seed() { :; }
run_make() { :; }
run_bazel() { ( tools/bazel build //config:all & sleep 1; tools/bazel build //config:all; wait ); }
fixture_main "$@"
