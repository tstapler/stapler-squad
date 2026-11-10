#!/usr/bin/env bash
# GoReleaser `tool:` shim: routes `go build` through the otelc weave so the released
# stapler-squad is the same instrumented binary `make build` produces. Every other go
# subcommand passes through. The weave rewrites go.mod in the working tree, so run
# goreleaser with `--parallelism 1`.
set -euo pipefail
if [ "${1:-}" = build ]; then
  exec "$(dirname "$0")/otel-auto-build.sh" go "$@"
fi
exec go "$@"
