#!/bin/sh
# Regression test: --class is validated before it reaches sed or the network.
dir=$(dirname "$0")
fail=0
for args in "--class" "--class bogus" "--class a/b" "--class operator/../x"; do
  # shellcheck disable=SC2086
  "$dir/workflow-engine-soak-status.sh" $args http://127.0.0.1:1 >/dev/null 2>&1
  rc=$?
  if [ "$rc" -ne 2 ]; then
    echo "FAIL: '$args' exited $rc, want 2" >&2
    fail=1
  fi
done
[ "$fail" -eq 0 ] && echo "ok"
exit "$fail"
