#!/usr/bin/env bash
# Fixture 3-missing-generated-code.sh: seeded failure for errors.yaml row E3.
set -uo pipefail
# shellcheck disable=SC2034  # KEY, REMEDY_RE, MAKE_APPLICABLE are read by fixture_main in common.sh
# shellcheck source=common.sh
. "$(dirname "${BASH_SOURCE[0]}")/common.sh"
KEY=E3
REMEDY_RE='make (proto-gen|ent-gen|sync)'
MAKE_APPLICABLE=true
seed() { :; }
run_make() { go build ./session/scanstate; }
run_bazel() { go build ./session/scanstate; }
fixture_main "$@"
