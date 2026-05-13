#!/usr/bin/env bash
# validate-registry.sh
# Validates that committed per-feature registry files match what the scanner would generate.
# Per-feature files live under docs/registry/features/backend/<domain>/<action>.json.
# The monolithic aggregate files (backend-features.json etc.) are generated artifacts — not compared.
#
# Usage: ./tools/scanner/validate-registry.sh [--threshold <percent>]
#
# Exit codes:
#   0 - Registry matches (or divergence within warning threshold)
#   1 - Registry divergence exceeds threshold (blocks PR)
#   2 - Usage/setup error

set -euo pipefail
export LC_ALL=C

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "${SCRIPT_DIR}/../.." && pwd)"

THRESHOLD_BLOCK=2
THRESHOLD_WARN=1

while [[ $# -gt 0 ]]; do
  case $1 in
    --threshold)
      THRESHOLD_BLOCK="$2"
      shift 2
      ;;
    *)
      echo "Unknown argument: $1" >&2
      exit 2
      ;;
  esac
done

cd "${PROJECT_ROOT}"

TMPDIR_WORK="$(mktemp -d)"
trap 'rm -rf "${TMPDIR_WORK}"' EXIT

echo "Registry Validation"
echo "==================="
echo ""

echo "Building backend scanner..."
if ! (cd tools/scanner && go build -o backend/cmd/scanner ./backend/cmd/) 2>&1; then
  echo "ERROR: failed to build backend scanner" >&2
  exit 2
fi

TEMP_BACKEND="${TMPDIR_WORK}/backend"
mkdir -p "${TEMP_BACKEND}"
echo "Scanning backend features..."
if ! ./tools/scanner/backend/cmd/scanner \
      proto/session/v1/session.proto \
      server/services/ \
      "${TEMP_BACKEND}" 2>&1; then
  echo "ERROR: backend scanner failed (session.proto)" >&2
  exit 2
fi
if ! ./tools/scanner/backend/cmd/scanner \
      proto/session/v1/unfinished.proto \
      server/services/ \
      "${TEMP_BACKEND}" 2>&1; then
  echo "ERROR: backend scanner failed (unfinished.proto)" >&2
  exit 2
fi
if ! ./tools/scanner/backend/cmd/scanner \
      proto/session/v1/backlog.proto \
      server/services/ \
      "${TEMP_BACKEND}" 2>&1; then
  echo "ERROR: backend scanner failed (backlog.proto)" >&2
  exit 2
fi

list_ids() {
  local dir="$1"
  python3 -c "
import json, glob
ids = []
for path in glob.glob('${dir}/**/*.json', recursive=True):
    try:
        ids.append(json.load(open(path))['id'])
    except Exception:
        pass
for i in sorted(ids): print(i)
" 2>/dev/null || true
}

COMMITTED_IDS="$(list_ids "${PROJECT_ROOT}/docs/registry/features/backend")"
GENERATED_IDS="$(list_ids "${TEMP_BACKEND}")"

count_lines() { python3 -c "import sys; lines=[l for l in sys.stdin.read().splitlines() if l.strip()]; print(len(lines))"; }
committed_count="$(echo "${COMMITTED_IDS}" | count_lines)"
generated_count="$(echo "${GENERATED_IDS}" | count_lines)"
added_ids="$(comm -13 <(echo "${COMMITTED_IDS}") <(echo "${GENERATED_IDS}") | count_lines)"
removed_ids="$(comm -23 <(echo "${COMMITTED_IDS}") <(echo "${GENERATED_IDS}") | count_lines)"
changed=$((added_ids + removed_ids))

divergence_pct=0
if [ "${committed_count}" -gt 0 ]; then
  divergence_pct=$(python3 -c "print(round(${changed} / ${committed_count} * 100, 2))")
fi

echo ""
echo "=== Backend Registry Diff ==="
echo "Committed: ${committed_count}  Generated: ${generated_count}  Divergence: ${divergence_pct}%"

if [ "${added_ids}" -gt 0 ]; then
  echo "⚠️  New RPCs (run 'make registry-generate' and commit):"
  comm -13 <(echo "${COMMITTED_IDS}") <(echo "${GENERATED_IDS}") | sed 's/^/  + /'
fi
if [ "${removed_ids}" -gt 0 ]; then
  echo "⚠️  Removed RPCs:"
  comm -23 <(echo "${COMMITTED_IDS}") <(echo "${GENERATED_IDS}") | sed 's/^/  - /'
fi

missing_marker=$(python3 -c "
import json, glob
n = sum(1 for p in glob.glob('docs/registry/features/backend/**/*.json', recursive=True)
        if not json.load(open(p)).get('markerFound', False))
print(n)
" 2>/dev/null || echo 0)

if [ "${missing_marker}" -gt 0 ]; then
  echo "⚠️  ${missing_marker} feature(s) missing // +api: marker (markerFound: false)"
fi

echo ""

if python3 -c "exit(0 if float('${divergence_pct}') > ${THRESHOLD_BLOCK} else 1)" 2>/dev/null; then
  echo "❌ Divergence ${divergence_pct}% > ${THRESHOLD_BLOCK}%. Run 'make registry-generate' and commit."
  exit 1
elif python3 -c "exit(0 if float('${divergence_pct}') > ${THRESHOLD_WARN} else 1)" 2>/dev/null; then
  echo "⚠️  Divergence ${divergence_pct}% above warning threshold."
  exit 0
else
  echo "✅ Registry validation passed. Divergence: ${divergence_pct}%"
  exit 0
fi
