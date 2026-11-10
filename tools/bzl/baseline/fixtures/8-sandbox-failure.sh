#!/usr/bin/env bash
# Fixture 8-sandbox-failure.sh: seeded failure for errors.yaml row E8.
# Bazel-mode body is PROVISIONAL: it needs the Gazelle-generated BUILD files and wrapper (Tasks 1.1a, 0.2f) and is finalised when Task 1.6d replays it.
set -uo pipefail
# shellcheck disable=SC2034  # KEY, REMEDY_RE, MAKE_APPLICABLE are read by fixture_main in common.sh
# shellcheck source=common.sh
. "$(dirname "${BASH_SOURCE[0]}")/common.sh"
KEY=E8
REMEDY_RE=''
MAKE_APPLICABLE=false
seed() { :; }
run_make() { :; }
run_bazel() { tools/bazel test //config:all --test_env=FIXTURE_TOUCH_OUTSIDE=1; }
fixture_main "$@"
