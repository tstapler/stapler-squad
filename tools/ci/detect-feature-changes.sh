#!/usr/bin/env bash
# detect-feature-changes.sh
# Detects if a PR contains feature-marked file changes.
#
# Usage: ./tools/ci/detect-feature-changes.sh [base-ref]
#
# Exit codes:
#   0 - Feature changes detected (RECORD_FEATURES should be set to 'true')
#   1 - No feature changes detected
#   2 - Error / usage issue
#
# Sets RECORD_FEATURES=true in GITHUB_ENV if running in GitHub Actions.

set -euo pipefail

BASE_REF="${1:-origin/main}"

# Get changed files
if ! CHANGED_FILES="$(git diff --name-only "${BASE_REF}...HEAD" 2>/dev/null)"; then
  # Try without three-dot notation for shallow clones
  if ! CHANGED_FILES="$(git diff --name-only "${BASE_REF}" HEAD 2>/dev/null)"; then
    echo "Warning: Could not determine changed files, assuming no feature changes" >&2
    exit 1
  fi
fi

if [ -z "${CHANGED_FILES}" ]; then
  echo "No files changed"
  exit 1
fi

FEATURE_CHANGE_DETECTED=0

while IFS= read -r file; do
  # Check 1: Registry files changed
  if [[ "${file}" == docs/registry/* ]]; then
    echo "Feature change detected: registry file changed: ${file}"
    FEATURE_CHANGE_DETECTED=1
    break
  fi

  # Check 2: File contains // +feature: marker
  if [ -f "${file}" ]; then
    if grep -q '// +feature:' "${file}" 2>/dev/null; then
      echo "Feature change detected: ${file} contains // +feature: marker"
      FEATURE_CHANGE_DETECTED=1
      break
    fi

    # Check 3: File contains // +api: marker
    if grep -q '// +api:' "${file}" 2>/dev/null; then
      echo "Feature change detected: ${file} contains // +api: marker"
      FEATURE_CHANGE_DETECTED=1
      break
    fi

    # Check 4: E2E spec file contains // @feature annotation
    if grep -q '// @feature' "${file}" 2>/dev/null; then
      echo "Feature change detected: ${file} contains // @feature annotation"
      FEATURE_CHANGE_DETECTED=1
      break
    fi
  fi
done <<< "${CHANGED_FILES}"

if [ "${FEATURE_CHANGE_DETECTED}" -eq 1 ]; then
  echo "RECORD_FEATURES=true"
  # Set GitHub Actions env var if running in CI
  if [ -n "${GITHUB_ENV:-}" ]; then
    echo "RECORD_FEATURES=true" >> "${GITHUB_ENV}"
  fi
  exit 0
else
  echo "No feature changes detected in this PR"
  exit 1
fi
