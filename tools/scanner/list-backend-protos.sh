#!/usr/bin/env bash
# Single source of truth for "which proto/session/v1/*.proto files declare a
# service" -- see proto_scanner_completeness_test.go's doc comment for the
# history of this enumeration drifting out of sync across Makefile,
# prune-stale-backend.sh, and validate-registry.sh.
#
# The filter pattern mirrors proto_scanner.go's servicePattern regex;
# service_pattern_parity_test.go checks the two stay in sync.
#
# Usage: tools/scanner/list-backend-protos.sh
# Prints one service-bearing .proto path per line, sorted under the C locale.
# Exits 1 with an error on stderr if none match.
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
