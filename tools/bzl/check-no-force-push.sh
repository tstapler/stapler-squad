#!/usr/bin/env bash
# Repo rule C8: nothing automated force-pushes. Greps workflows and scripts for force-push
# forms; BZL_FORCE_PUSH_ROOT lets tests point it at a fixture tree.
set -u
root="${BZL_FORCE_PUSH_ROOT:-$(git rev-parse --show-toplevel)}"
cd "$root" || exit 2
allow=tools/bzl/no-force-push-allow.txt
self=tools/bzl/check-no-force-push.sh

pattern='push[[:space:]]+([^#]*[[:space:]])?(-f|--force|--force-with-lease|--mirror)([[:space:]=]|$)|[[:space:]]\+refs/|push[[:space:]]+[^#]*[[:space:]]\+[A-Za-z0-9_./-]+:'
dirs=()
for d in .github scripts tools/bazel tools/bzl tools/ci; do
  [ -e "$d" ] && dirs+=("$d")
done
[ "${#dirs[@]}" -gt 0 ] || exit 0

while IFS= read -r hit; do
  file=${hit%%:*}
  case "$file" in
    "$self"|tools/bzl/check-no-force-push_test.sh|scripts/hooks/pre-push-no-force_test.sh|"$allow") continue ;;
  esac
  if [ -f "$allow" ] && grep -qE "^${file//./\\.}([[:space:]]|$)" "$allow"; then continue; fi
  echo "check-no-force-push: force-push form in $hit" >&2
  exit 1
done < <(grep -rnE -- "$pattern" "${dirs[@]}" 2>/dev/null)
exit 0
