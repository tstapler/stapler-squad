#!/bin/sh
# Stand-in CLI for cli-flag-discovery e2e: prints a known --help and leaves a
# marker file beside itself so the spec can prove when (and whether) it ran.
# Never `claude`: the e2e server's environment may not have it.
touch "$(dirname "$0")/probe-fixture.ran"
cat <<'HELP'
Usage: probe-fixture [options]

Options:
  --alpha          first flag
  --beta <v>       second
HELP
