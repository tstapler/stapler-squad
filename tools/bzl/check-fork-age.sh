#!/usr/bin/env bash
# Check every row of the forks register (Task 11.8a, ADR-008 rule 4): fail on a row whose Remove-by date has
# passed, whose "Fixed upstream in" cell is filled (the removal condition is met), or that lacks an upstream link.
# Exit 0 with an empty register.
# Env: FORKS_FILE, TODAY (YYYY-MM-DD; tests seed both).
set -eu
root=$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)
file="${FORKS_FILE:-$root/docs/reference/bazel-forks.md}"
today="${TODAY:-$(date -u +%Y-%m-%d)}"
awk -F'|' -v today="$today" '
  function trim(s) { gsub(/^[ \t]+|[ \t]+$/, "", s); return s }
  /^\| *Fork *\|/ { header=1; next }
  header && /^\|[ -]+\|/ { next }
  header && /^\|/ {
    fork=trim($2); upstream=trim($3); removeby=trim($7); fixed=trim($8)
    if (fork == "") next
    if (upstream == "" || upstream == "-") { printf "fork %s: no upstream PR or issue link (ADR-008 rule 4)\n", fork; bad=1 }
    if (removeby !~ /^[0-9][0-9][0-9][0-9]-[0-9][0-9]-[0-9][0-9]$/) { printf "fork %s: Remove by is not a YYYY-MM-DD date\n", fork; bad=1 }
    else if (removeby < today) { printf "fork %s: past its remove-by date %s; drop it or renew with the owner\n", fork, removeby; bad=1 }
    if (fixed != "" && fixed != "-") { printf "fork %s: fixed upstream in %s, so the removal condition is met; remove the fork\n", fork, fixed; bad=1 }
  }
  END { exit bad }' "$file" >&2
