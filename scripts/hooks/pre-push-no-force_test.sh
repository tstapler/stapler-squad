#!/usr/bin/env bash
# Temp-repo tests for pre-push-no-force.sh: fast-forward accepted, rewritten history rejected,
# main deletion rejected.
set -u
hook=$(cd "$(dirname "$0")" && pwd)/pre-push-no-force.sh
T=$(mktemp -d "${TMPDIR:-/tmp}/prepush.XXXXXX"); trap 'rm -rf "$T"' EXIT
fail=0
cd "$T" || exit 2
git init -q -b main . && git config user.email t@example.com && git config user.name t
git commit -q --allow-empty -m a; a=$(git rev-parse HEAD)
git commit -q --allow-empty -m b; b=$(git rev-parse HEAD)
git reset -q --hard "$a"; git commit -q --allow-empty -m c2; c=$(git rev-parse HEAD)
zero=0000000000000000000000000000000000000000
check() { # check <want> <label> <local sha> <remote ref> <remote sha>
  echo "refs/heads/x $3 $4 $5" | bash "$hook" >/dev/null 2>&1
  local got=$?
  if [ "$got" = "$1" ]; then echo "ok   - $2"; else echo "FAIL - $2 (exit $got, want $1)" >&2; fail=1; fi
}
check 0 "fast-forward accepted" "$b" refs/heads/main "$a"
check 1 "rewritten history rejected" "$c" refs/heads/main "$b"
check 0 "new branch accepted" "$b" refs/heads/new "$zero"
check 1 "main deletion rejected" "$zero" refs/heads/main "$b"
check 0 "feature branch deletion accepted" "$zero" refs/heads/feature "$b"
exit $fail
