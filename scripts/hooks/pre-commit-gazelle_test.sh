#!/usr/bin/env bash
# Temp-repo tests for pre-commit-gazelle.sh using a fake GAZELLE_CMD: irrelevant change skips gazelle,
# relevant change with stable BUILD passes, relevant change that rewrites BUILD fails.
set -u
hook=$(cd "$(dirname "$0")" && pwd)/pre-commit-gazelle.sh
T=$(mktemp -d "${TMPDIR:-/tmp}/precommit.XXXXXX"); trap 'rm -rf "$T"' EXIT
fail=0
cd "$T" || exit 2
git init -q -b main . && git config user.email t@example.com && git config user.name t
echo old >BUILD.bazel; echo hi >README.md; git add -A; git commit -q -m init
check() { # check <want> <label> <gazelle cmd>
  GAZELLE_CMD=$3 bash "$hook" >/dev/null 2>&1
  local got=$?
  if [ "$got" = "$1" ]; then echo "ok   - $2"; else echo "FAIL - $2 (exit $got, want $1)" >&2; fail=1; fi
}
echo x >>README.md; git add README.md
check 0 "docs change skips gazelle" "false"
git reset -q --hard
printf 'package a\n' >a.go; git add a.go
check 0 "new go file, BUILD stable" "true"
check 1 "new go file, BUILD rewritten" "echo new >BUILD.bazel"
git checkout -q -- BUILD.bazel
check 1 "gazelle command failure" "false"
git reset -q --hard
git commit -q --allow-empty -m e; printf 'package a\nfunc F() {}\n' >a.go; git add a.go; git commit -q -m a
printf 'package a\nfunc F() { _ = 1 }\n' >a.go; git add a.go
check 0 "body-only go edit skips gazelle" "echo new >BUILD.bazel"
printf 'package a\nimport "fmt"\nfunc F() { fmt.Print() }\n' >a.go; git add a.go
check 1 "import edit runs gazelle" "echo new >BUILD.bazel"
exit $fail
