#!/usr/bin/env bash
# Seeded violations for check-no-force-push.sh.
set -u
here=$(cd "$(dirname "$0")" && pwd)
T=$(mktemp -d "${TMPDIR:-/tmp}/nfp.XXXXXX"); trap 'rm -rf "$T"' EXIT
fail=0
expect() { # expect <exit> <label>
  BZL_FORCE_PUSH_ROOT="$T" bash "$here/check-no-force-push.sh" >/dev/null 2>&1
  local got=$?
  if [ "$got" = "$1" ]; then echo "ok   - $2"; else echo "FAIL - $2 (exit $got, want $1)" >&2; fail=1; fi
}
mkdir -p "$T/.github/workflows" "$T/scripts"
echo 'git push origin main' > "$T/scripts/a.sh"; expect 0 "plain push allowed"
echo 'git push --force origin main' > "$T/scripts/a.sh"; expect 1 "--force rejected"
echo 'git push -f origin main' > "$T/scripts/a.sh"; expect 1 "-f rejected"
echo 'git push --force-with-lease origin x' > "$T/scripts/a.sh"; expect 1 "--force-with-lease rejected"
echo 'git push origin +refs/heads/x:refs/heads/x' > "$T/scripts/a.sh"; expect 1 "+refs refspec rejected"
echo 'run: git push origin +HEAD:main' > "$T/.github/workflows/w.yml"; echo ok > "$T/scripts/a.sh"; expect 1 "+HEAD:main refspec in workflow rejected"
exit $fail
