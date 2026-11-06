#!/usr/bin/env bash
# Changed-files lint gate: any ESLint warning or error in the given files fails (exit 1).
# Used by .github/workflows/lint.yml and scripts/lint-affected.sh. The whole-repo run
# (`bun run lint:eslint`) deliberately tolerates the ~1,450 pre-existing warnings; only
# errors fail it. Run from web-app/. Files outside eslint.config.mjs's src/** scope are
# skipped silently (--no-warn-ignored) so they do not count as warnings.
set -euo pipefail

if [ "$#" -eq 0 ]; then
  echo "usage: eslint-gate.sh <file>..." >&2
  exit 2
fi

exec ./node_modules/.bin/eslint --max-warnings 0 --no-warn-ignored -- "$@"
