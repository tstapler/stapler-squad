#!/bin/sh
# git-soak-status.sh — print the git backend shadow-soak status against the flip criteria.
# Reads /actuator/git-backend from the running service. Exit 0: every shadowed cohort meets
# the criteria; 1: at least one does not (or nothing is shadowed); 2: service unreachable.
# See docs/how-to/flip-git-backend-cohort.md. Usage: git-soak-status.sh [base-url]
base="${1:-${SSQ_URL:-http://localhost:8543}}"
json=$(curl -fsS "$base/actuator/git-backend") || { echo "cannot reach $base/actuator/git-backend" >&2; exit 2; }
curl -fsS "$base/actuator/git-backend?format=text" || exit 2
case "$json" in
  *'"verdict":{}'*) exit 1 ;;
  *'"ready":false'*) exit 1 ;;
  *'"ready":true'*) exit 0 ;;
esac
exit 1
