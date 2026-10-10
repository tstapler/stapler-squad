#!/usr/bin/env bash
# Fails when CLAUDE.md quotes a different jscpd threshold than web-app/.jscpd.json,
# so the documented ratchet cannot drift from the enforced one.
set -euo pipefail
root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

enforced="$(python3 -c "import json,sys; print(json.load(open(sys.argv[1]))['threshold'])" "$root/web-app/.jscpd.json")"
documented="$(grep -m1 -oE 'absolute `threshold` \(([0-9.]+)%' "$root/CLAUDE.md" | grep -oE '[0-9.]+' | head -1 || true)"

if [[ -z "$documented" ]]; then
  echo "check-jscpd-threshold-doc: no jscpd threshold found in CLAUDE.md" >&2
  exit 1
fi
if [[ "$enforced" != "$documented" ]]; then
  echo "check-jscpd-threshold-doc: CLAUDE.md says ${documented}% but web-app/.jscpd.json enforces ${enforced}%" >&2
  exit 1
fi
echo "jscpd threshold documented and enforced agree: ${enforced}%"
