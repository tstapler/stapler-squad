#!/usr/bin/env bash
# Cases for check-build-fresh-cheap.sh in a throwaway git repo (never touches the real checkout).
# shellcheck disable=SC2015  # A && ok || bad: ok() never fails
set -u
script=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)/check-build-fresh-cheap.sh
T=$(mktemp -d "${TMPDIR:-/tmp}/fresh-test.XXXXXX")
trap 'rm -rf "$T"' EXIT
failures=0
ok() { echo "ok   - $1"; }
bad() { echo "FAIL - $1" >&2; failures=$((failures + 1)); }
cd "$T" || exit 2
git init -q . && git config user.email t@t && git config user.name t
mkdir -p pkg
printf 'package pkg\n\nimport "strings"\n\nvar _ = strings.ToLower\n' > pkg/a.go
echo 'go_library(name = "pkg")' > pkg/BUILD.bazel
git add . && git commit -qm init

out=$(bash "$script" --warn); [ "$out" = "BUILD: fresh" ] && ok "clean tree prints BUILD: fresh" || bad "clean tree: $out"

printf 'package pkg\n' > pkg/b.go
out=$(bash "$script" --warn); rc=$?
case "$out" in *"BUILD.bazel may be stale for pkg - run 'make gazelle'"*) ok "new .go file without BUILD change warns" ;; *) bad "new file: $out" ;; esac
[ "$rc" -eq 0 ] && ok "--warn exits 0" || bad "--warn rc=$rc"
bash "$script" >/dev/null; [ $? -eq 1 ] && ok "without --warn a hit exits 1" || bad "no-warn exit"
echo '# touched' >> pkg/BUILD.bazel
out=$(bash "$script" --warn); [ "$out" = "BUILD: fresh" ] && ok "same change with BUILD.bazel modified is silent" || bad "with BUILD: $out"
git checkout -q -- pkg/BUILD.bazel; rm pkg/b.go

printf 'package pkg\n\nimport (\n\t"os"\n\t"strings"\n)\n\nvar _ = strings.ToLower\nvar _ = os.Args\n' > pkg/a.go
out=$(bash "$script" --warn)
case "$out" in *"stale for pkg"*) ok "new import warns" ;; *) bad "new import: $out" ;; esac
echo '# touched' >> pkg/BUILD.bazel
out=$(bash "$script" --warn); [ "$out" = "BUILD: fresh" ] && ok "new import with BUILD change is silent" || bad "import+BUILD: $out"
git checkout -q -- . 
printf 'package pkg\n\nimport "strings"\n\nvar _ = strings.ToUpper\n' > pkg/a.go
out=$(bash "$script" --warn); [ "$out" = "BUILD: fresh" ] && ok "body-only edit is silent" || bad "body edit: $out"

[ "$failures" -eq 0 ] || exit 1
