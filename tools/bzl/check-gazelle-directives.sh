#!/usr/bin/env bash
# List every added or removed `# gazelle:` directive in a PR diff (Task 11.6b) so a directive change cannot hide
# behind a collapsed generated diff, and fail unless the owner reviewed it.
#   check-gazelle-directives.sh <base> <head>
# Env: PR_LABELS (comma-separated), PR_REVIEWERS (comma-separated logins), OWNER (default tstapler),
#      GITHUB_ACTIONS (when set, each change is also printed as a ::notice:: annotation).
# Prints PASS (exit 0) or FAIL (exit 1). A change passes when the PR has the label gazelle-directive-reviewed
# or the owner is among the reviewers. Any BUILD file counts, including the root BUILD.bazel.
set -eu
base="${1:?usage: $0 <base> <head>}"; head="${2:?usage: $0 <base> <head>}"
owner="${OWNER:-tstapler}"
labels=",${PR_LABELS:-},"; reviewers=",${PR_REVIEWERS:-},"

changes=$(git diff -U0 --no-color "$base" "$head" -- 'BUILD.bazel' '**/BUILD.bazel' 'BUILD' '**/BUILD' | awk '
  /^diff --git/ { file=""; next }
  /^\+\+\+ / { f=$2; sub(/^b\//, "", f); if ($2 != "/dev/null") file=f; next }
  /^--- / { f=$2; sub(/^a\//, "", f); oldfile=f; next }
  /^@@/ { match($0, /-[0-9]+/); old=substr($0, RSTART+1, RLENGTH-1)+0
          match($0, /\+[0-9]+/); new=substr($0, RSTART+1, RLENGTH-1)+0; next }
  /^\+[ \t]*# gazelle:/ { printf "%s:%d\t+\t%s\n", file, new, substr($0, 2); new++; next }
  /^-[ \t]*# gazelle:/ { printf "%s:%d\t-\t%s\n", (file != "" ? file : oldfile), old, substr($0, 2); old++; next }
  /^\+/ { new++; next }
  /^-/ { old++; next }')

if [ -z "$changes" ]; then echo PASS; exit 0; fi

reviewed=0
case "$labels" in *,gazelle-directive-reviewed,*) reviewed=1 ;; esac
case "$reviewers" in *,"$owner",*) reviewed=1 ;; esac

while IFS=$'\t' read -r where sign text; do
  [ -n "$where" ] || continue
  msg="gazelle directive changed: $where; owner review required ($sign $text)"
  echo "$msg" >&2
  [ -z "${GITHUB_ACTIONS:-}" ] || echo "::notice::$msg"
done <<EOF2
$changes
EOF2

if [ "$reviewed" = 1 ]; then echo "PASS (reviewed)"; exit 0; fi
echo FAIL
exit 1
