#!/usr/bin/env bash
# Prune per-feature backend registry files whose RPC no longer exists.
#
# `make registry-generate-backend` writes/updates files in place (preserving
# human-edited testIds/tested), but it is additive — it never deletes a file
# whose RPC was removed or renamed in the proto. That additive behavior caused
# registry-validation divergence (committed > generated). This script closes the
# gap: it regenerates the authoritative id-set into a temp dir and removes any
# committed file whose id is absent from it.
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"
cd "$REPO_ROOT"

COMMITTED="${1:-docs/registry/features/backend}"
SCANNER="tools/scanner/backend/cmd/scanner"

[ -x "$SCANNER" ] || (cd tools/scanner && go build -o backend/cmd/scanner ./backend/cmd/)

TMP="$(mktemp -d)"
trap 'rm -rf "$TMP"' EXIT

for p in session unfinished backlog insights github_user; do
  "$SCANNER" "proto/session/v1/$p.proto" server/services/ "$TMP" >/dev/null
done

python3 - "$TMP" "$COMMITTED" <<'PY'
import glob, json, os, sys

tmp, committed = sys.argv[1], sys.argv[2]

valid = set()
for path in glob.glob(os.path.join(tmp, "**", "*.json"), recursive=True):
    try:
        valid.add(json.load(open(path))["id"])
    except Exception:
        pass

removed = 0
for path in glob.glob(os.path.join(committed, "**", "*.json"), recursive=True):
    try:
        fid = json.load(open(path))["id"]
    except Exception:
        continue
    if fid not in valid:
        os.remove(path)
        removed += 1
        print(f"  pruned stale: {os.path.relpath(path)}")

print(f"Pruned {removed} stale backend feature file(s).")
PY
