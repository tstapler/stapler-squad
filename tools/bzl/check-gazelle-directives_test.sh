#!/usr/bin/env bash
# Seeded diffs in a throwaway repo: a directive change fails, no directive change passes, review clears it.
set -u
here=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)
T=$(mktemp -d "${TMPDIR:-/tmp}/gazelle-dir-test.XXXXXX")
trap 'rm -rf "$T"' EXIT
failures=0
R="$T/repo"; mkdir -p "$R/pkg"; cd "$R" || exit 2
git init -q -b main . && git config user.email t@t && git config user.name t
printf '# gazelle:prefix example.com/x\n# gazelle:exclude gen\n' > BUILD.bazel
printf 'go_library(name = "pkg")\n' > pkg/BUILD.bazel
git add . && git commit -qm base
base=$(git rev-parse HEAD)

check() { # label want-exit want-text [env...]
  local label=$1 want=$2 text=$3 out rc; shift 3
  out=$(env "$@" bash "$here/check-gazelle-directives.sh" "$base" HEAD 2>&1); rc=$?
  if [ "$rc" = "$want" ] && echo "$out" | grep -q -- "$text"; then echo "PASS - $label"; else echo "FAIL - $label (rc=$rc): $out" >&2; failures=$((failures + 1)); fi
}

printf 'go_library(name = "pkg", visibility = ["//visibility:public"])\n' > pkg/BUILD.bazel
git commit -qam "edit a rule, no directive"
check "no directive change" 0 '^PASS$' X=1

printf '# gazelle:prefix example.com/x\n# gazelle:exclude gen\n# gazelle:resolve go example.com/y //y\n' > BUILD.bazel
git commit -qam "add a resolve directive in the root BUILD only"
check "added # gazelle:resolve in root BUILD.bazel" 1 'gazelle directive changed: BUILD.bazel:3; owner review required' X=1
check "label clears it" 0 'PASS (reviewed)' PR_LABELS=gazelle-directive-reviewed
check "owner as reviewer clears it" 0 'PASS (reviewed)' PR_REVIEWERS=someone,tstapler

printf '# gazelle:prefix example.com/x\n' > BUILD.bazel
git commit -qam "remove directives"
check "removed directives are listed too" 1 'gazelle directive changed: BUILD.bazel:2' X=1
exit $((failures > 0))
