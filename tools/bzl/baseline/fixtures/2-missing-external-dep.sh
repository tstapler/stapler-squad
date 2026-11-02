#!/usr/bin/env bash
# Fixture 2-missing-external-dep.sh: seeded failure for errors.yaml row E2.
set -uo pipefail
# shellcheck disable=SC2034  # KEY, REMEDY_RE, MAKE_APPLICABLE are read by fixture_main in common.sh
# shellcheck source=common.sh
. "$(dirname "${BASH_SOURCE[0]}")/common.sh"
KEY=E2
REMEDY_RE='go get|go mod tidy'
MAKE_APPLICABLE=true
seed() { printf 'package config\n\nimport _ "rsc.io/quote"\n' > config/zz_fixture.go; }
run_make() { go build ./config; }
run_bazel() { tools/bazel build //config:all; }
fixture_main "$@"
