#!/usr/bin/env bash
# Fixture 4-oom.sh: seeded failure for errors.yaml row E4.
set -uo pipefail
# shellcheck disable=SC2034  # KEY, REMEDY_RE, MAKE_APPLICABLE are read by fixture_main in common.sh
# shellcheck source=common.sh
. "$(dirname "${BASH_SOURCE[0]}")/common.sh"
KEY=E4
REMEDY_RE='GOMEMLIMIT|ulimit -|reduce|increase .*memory|retry'
MAKE_APPLICABLE=true
seed() { mkdir -p fixture_oom && printf 'package main\n\nfunc main() {}\n' > fixture_oom/main.go; }
run_make() { ( ulimit -v 262144; go build -o /dev/null ./fixture_oom ); }
run_bazel() { ( ulimit -v 4194304; tools/bazel build //... ); }
fixture_main "$@"
