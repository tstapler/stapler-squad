#!/usr/bin/env bash
# Two-branch simulation: each branch adds a different entry to a lockfile-shaped JSON; merging through the
# driver must succeed without conflict markers and keep both entries. Also exercises --check. No Bazel needed.
# shellcheck disable=SC2015  # A && ok || bad: ok never fails
set -u
here=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)
repo_root=$(cd "$here/.." && pwd)
T=$(mktemp -d "${TMPDIR:-/tmp}/lockmerge-test.XXXXXX")
trap 'rm -rf "$T"' EXIT
failures=0
ok() { echo "ok   - $1"; }
bad() { echo "FAIL - $1" >&2; failures=$((failures + 1)); }

R="$T/repo"
mkdir -p "$R/scripts" "$R/tools/bzl"
cp "$here/git-merge-bazel-lock.sh" "$R/scripts/"; cp "$repo_root/tools/bzl/bazel-lockfile-merge.jq" "$R/tools/bzl/"
cd "$R" || exit 2
git init -q -b main . && git config user.email t@t && git config user.name t
echo 'MODULE.bazel.lock merge=bazel-lockfile-merge' > .gitattributes

( cd "$R" && ! bash scripts/git-merge-bazel-lock.sh --check 2>/dev/null ) && ok "--check fails before registration" || bad "--check fails before registration"
git config merge.bazel-lockfile-merge.driver "scripts/git-merge-bazel-lock.sh %O %A %B"
git config merge.bazel-lockfile-merge.driver "scripts/git-merge-bazel-lock.sh %O %A %B"   # idempotent re-run
[ "$(git config --get-all merge.bazel-lockfile-merge.driver | wc -l)" = 1 ] && ok "registration is idempotent" || bad "registration is idempotent"
bash scripts/git-merge-bazel-lock.sh --check >/dev/null && ok "--check passes after registration" || bad "--check passes after registration"

jq -n '{lockFileVersion: 24, registryFileHashes: {"https://bcr/a": "h1"}, selectedYankedVersions: {}, moduleExtensions: {}}' > MODULE.bazel.lock
git add . && git commit -qm base
for side in one two; do
  git checkout -q -b "$side" main
  jq --arg k "https://bcr/$side" '.registryFileHashes[$k] = "h-" + $k' MODULE.bazel.lock > x && mv x MODULE.bazel.lock
  git commit -qam "add $side"
done
git checkout -q one
if git merge -q --no-edit two >/dev/null 2>&1; then ok "merge of two lock edits has no conflict"; else bad "merge of two lock edits has no conflict"; fi
! grep -q '<<<<<<<' MODULE.bazel.lock && ok "no conflict markers" || bad "no conflict markers"
[ "$(jq '.registryFileHashes | keys | length' MODULE.bazel.lock 2>/dev/null)" = 3 ] && ok "merged lock keeps all three entries" || bad "merged lock keeps all three entries"
exit $((failures > 0))
