#!/usr/bin/env bash
# Single source of truth for "which proto/session/v1/*.proto files declare a
# service" — the enumeration this repo has repeatedly hand-duplicated and let
# drift out of sync (see tools/scanner/backend/proto_scanner_completeness_test.go's
# doc comment for the history: remote.proto, headless.proto, handoff_summary.proto
# each went invisible to one or more of Makefile's registry-generate-backend,
# prune-stale-backend.sh, and validate-registry.sh before this was consolidated).
#
# The filter pattern mirrors tools/scanner/backend/proto_scanner.go's
# `servicePattern` regex (`^\s*service\s+(\w+)\s*\{`) rather than a bare
# `^service `, so a proto formatted with leading whitespace or a tab before
# "service" is still picked up here the same way ScanProto would pick it up.
#
# Usage: tools/scanner/list-backend-protos.sh
# Prints one service-bearing .proto path per line (repo-root-relative), sorted
# under the C locale. Exits 1 with an error on stderr if none match.
set -euo pipefail
export LC_ALL=C
shopt -s nullglob

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"
cd "$REPO_ROOT"

SERVICE_PATTERN='^[[:space:]]*service[[:space:]]+[A-Za-z0-9_]+[[:space:]]*\{'

protos=()
for proto in proto/session/v1/*.proto; do
  grep -qE "$SERVICE_PATTERN" "$proto" && protos+=("$proto")
done

if [ "${#protos[@]}" -eq 0 ]; then
  echo "ERROR: no service-bearing .proto files found under proto/session/v1/" >&2
  exit 1
fi

printf '%s\n' "${protos[@]}"
