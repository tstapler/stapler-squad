#!/bin/sh
# git-soak-status.sh — print the git backend shadow-soak status against the flip criteria.
# Reads /actuator/git-backend from the running service (one request). Exit 0: every shadowed
# cohort meets the criteria; 1: at least one does not, or nothing is shadowed; 2: unreachable
# (including a 401 when auth is enabled; /actuator sits behind the same auth as /actuator/metrics).
# See docs/how-to/flip-git-backend-cohort.md. Usage: git-soak-status.sh [base-url]
base="${1:-${SSQ_URL:-http://localhost:8543}}"
tmp=$(mktemp -d) || exit 2
trap 'rm -rf "$tmp"' EXIT
curl -fsS -D "$tmp/headers" -o "$tmp/body" "$base/actuator/git-backend?format=text" \
  || { echo "cannot reach $base/actuator/git-backend (is the service up, is auth enabled?)" >&2; exit 2; }
cat "$tmp/body"
grep -qi '^x-git-soak-ready: true' "$tmp/headers"
