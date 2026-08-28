#!/usr/bin/env bash
# check-generated-files-literal.sh
# Guards the single-source-of-truth for the "generated-files" artifact name
# (.github/workflows/_prepare.yml's upload-artifact step / workflow_call
# output). Consumer workflows must reference
# `${{ needs.prepare.outputs.artifact-name }}` instead of retyping the
# literal string, so a future rename of the artifact is a one-file edit.
# See project_plans/ci-speed/implementation/plan.md Epic 3.1, Story 3.1.1,
# Task 3.1.1c.
#
# Usage: ./tools/ci/check-generated-files-literal.sh
#
# Exit codes:
#   0 - clean, no stray literal found
#   1 - one or more workflow files reference the literal outside _prepare.yml

set -euo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
WORKFLOWS_DIR="${REPO_ROOT}/.github/workflows"

MATCHES="$(grep -rn "generated-files" "${WORKFLOWS_DIR}" --include='*.yml' \
  | grep -v '_prepare.yml' \
  | grep -v 'outputs.artifact-name' || true)"

if [ -n "${MATCHES}" ]; then
  echo "Found literal 'generated-files' string outside .github/workflows/_prepare.yml:" >&2
  echo "${MATCHES}" >&2
  echo "" >&2
  echo "Reference \${{ needs.prepare.outputs.artifact-name }} instead of retyping the literal." >&2
  exit 1
fi

echo "clean: no stray 'generated-files' literal found outside _prepare.yml"
