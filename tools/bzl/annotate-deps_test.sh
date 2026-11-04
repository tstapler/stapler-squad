#!/usr/bin/env bash
# Fixtures: an added module, an upgrade, a removed bazel_dep, and a lock-only change.
set -u
here=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)
T=$(mktemp -d "${TMPDIR:-/tmp}/annotate-test.XXXXXX")
trap 'rm -rf "$T"' EXIT
failures=0
R="$T/repo"; mkdir -p "$R"; cd "$R" || exit 2
git init -q -b main . && git config user.email t@t && git config user.name t
cat > go.mod <<'M'
module example.com/x

go 1.25

require (
	github.com/a/a v1.0.0
	github.com/b/b v1.0.0 // indirect
)

require github.com/single/s v0.1.0
M
cat > MODULE.bazel <<'M'
bazel_dep(name = "rules_go", version = "0.65.0")
bazel_dep(name = "gazelle", version = "0.54.0", repo_name = "bazel_gazelle")
bazel_dep(name = "rules_shell", version = "0.8.0")
go_deps = use_extension("@gazelle//:extensions.bzl", "go_deps")
use_repo(
    go_deps,
    "com_github_a_a",
    "com_github_b_b",
)
M
echo '{"lockFileVersion": 24, "registryFileHashes": {}}' > MODULE.bazel.lock
git add . && git commit -qm base; base=$(git rev-parse HEAD)

expect() { # label text... (all must appear)
  local label=$1 out; shift
  out=$(bash "$here/annotate-deps.sh" "$base" HEAD 2>&1)
  for want in "$@"; do
    echo "$out" | grep -qF -- "$want" || { echo "FAIL - $label: missing '$want' in:" >&2; echo "$out" >&2; failures=$((failures + 1)); return; }
  done
  echo "PASS - $label"
}

# lock-only change
echo '{"lockFileVersion": 24, "registryFileHashes": {"x": "y"}}' > MODULE.bazel.lock
git commit -qam "lock only"
expect "lock-only change" "no dependency change"

# added module, upgrade, removed bazel_dep, use_repo change
sed -i 's#github.com/a/a v1.0.0#github.com/a/a v1.2.0#; s#^)$#\tgithub.com/new/n v0.3.0\n)#' go.mod
sed -i '/rules_shell/d; s/"com_github_b_b",/"com_github_new_n",/' MODULE.bazel
git commit -qam "dependency edits"
expect "added module" "go module added: github.com/new/n v0.3.0"
expect "upgraded module" "go module upgraded: github.com/a/a (v1.0.0 -> v1.2.0)"
expect "removed bazel_dep" "bazel_dep removed: rules_shell 0.8.0"
expect "use_repo added and removed" "use_repo added: com_github_new_n" "use_repo removed: com_github_b_b"
# dependency files absent at the base revision: everything is "added", nothing "removed"
empty=$(git commit-tree -m empty "$(git hash-object -t tree /dev/null)")
out=$(bash "$here/annotate-deps.sh" "$empty" HEAD 2>&1)
if echo "$out" | grep -q 'bazel_dep added: rules_go 0.65.0' && ! echo "$out" | grep -q removed; then echo "PASS - empty base"; else echo "FAIL - empty base: $out" >&2; failures=$((failures + 1)); fi
exit $((failures > 0))
