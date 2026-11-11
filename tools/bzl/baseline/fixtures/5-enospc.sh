#!/usr/bin/env bash
# Fixture 5-enospc.sh: seeded failure for errors.yaml row E5.
set -uo pipefail
# shellcheck disable=SC2034  # KEY, REMEDY_RE, MAKE_APPLICABLE are read by fixture_main in common.sh
# shellcheck source=common.sh
. "$(dirname "${BASH_SOURCE[0]}")/common.sh"
KEY=E5
REMEDY_RE='free|clean|df |cleanup'
MAKE_APPLICABLE=true
seed() { mkdir -p fixture_enospc && printf 'package main\n\nfunc main() {}\n' > fixture_enospc/main.go; }
run_make() { go build -o /dev/full ./fixture_enospc; }
run_bazel() { tools/bazel build //... --output_user_root=/dev/full/x; }
fixture_main "$@"
