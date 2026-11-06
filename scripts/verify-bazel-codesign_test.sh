#!/usr/bin/env bash
# Tests verify-bazel-codesign.sh: the non-macOS path for real, and the macOS path with a stub codesign.
set -u
here=$(cd "$(dirname "$0")" && pwd)
T=$(mktemp -d "${TMPDIR:-/tmp}/vbc.XXXXXX"); trap 'rm -rf "$T"' EXIT
fail=0
expect() { # expect <want-exit> <label> <env...> -- <args...>
  local want=$1 label=$2; shift 2
  local envs=(); while [ "$1" != -- ]; do envs+=("$1"); shift; done; shift
  env "${envs[@]}" bash "$here/verify-bazel-codesign.sh" "$@" >"$T/out" 2>&1
  local got=$?
  if [ "$got" = "$want" ]; then echo "ok   - $label"; else echo "FAIL - $label (exit $got, want $want)" >&2; cat "$T/out" >&2; fail=1; fi
}
stub() { # stub <dr-line> <authority>
  cat >"$T/codesign" <<S
#!/usr/bin/env bash
case "\$1" in
  --verify) exit 0 ;;
  -dvv) echo "Authority=$2" >&2 ;;
  -d) echo '$1' ;;
esac
S
  chmod +x "$T/codesign"
}
: >"$T/bin"
expect 2 "non-macOS exits 2" VBC_OS=Linux -- "$T/bin"
grep -q "not macOS" "$T/out" || { echo "FAIL - non-macOS message" >&2; fail=1; }
stub 'designated => identifier "com.stapler-squad" and certificate leaf = H"abc"' StaplerSquadDev
expect 0 "cert-anchored StaplerSquadDev passes" VBC_OS=Darwin "CODESIGN=$T/codesign" -- "$T/bin"
stub 'designated => cdhash H"abc"' StaplerSquadDev
expect 1 "cdhash requirement fails" VBC_OS=Darwin "CODESIGN=$T/codesign" -- "$T/bin"
stub 'designated => certificate leaf = H"abc"' Other
expect 1 "wrong authority fails" VBC_OS=Darwin "CODESIGN=$T/codesign" -- "$T/bin"
exit $fail
