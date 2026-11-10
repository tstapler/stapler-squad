#!/usr/bin/env bash
# Owner-run (macOS only): checks that a signed copy of the Bazel-built binary carries the stable
# StaplerSquadDev identity, so TCC grants survive the switch from the make-built binary.
# Usage: verify-bazel-codesign.sh [SIGNED_BAZEL_BIN] [MAKE_BUILT_BIN]
# Exit 0 = pass, 1 = signature check failed, 2 = not macOS / bad usage.
# VBC_OS and CODESIGN override the OS name and tool (used by verify-bazel-codesign_test.sh).
set -u
bin=${1:-./stapler-squad-bazel}
make_bin=${2:-}
codesign=${CODESIGN:-codesign}

if [ "${VBC_OS:-$(uname)}" != "Darwin" ]; then
  echo "verify-bazel-codesign: not macOS; code signing and TCC checks only run on a Mac (owner-run, see docs/how-to/macos-codesigning.md)" >&2
  exit 2
fi
[ -f "$bin" ] || { echo "verify-bazel-codesign: $bin not found" >&2; exit 2; }

fail=0
dr_of() { "$codesign" -d -r- "$1" 2>&1 | grep '^designated' ; }

"$codesign" --verify --strict "$bin" || { echo "FAIL: codesign --verify --strict" >&2; fail=1; }
"$codesign" -dvv "$bin" 2>&1 | grep -q '^Authority=StaplerSquadDev' \
  || { echo "FAIL: signing authority is not StaplerSquadDev" >&2; fail=1; }

dr=$(dr_of "$bin")
echo "designated requirement: ${dr:-<none>}"
case "$dr" in
  *cdhash*|"") echo "FAIL: requirement is empty or cdhash-anchored (ad-hoc signature); TCC would re-prompt" >&2; fail=1 ;;
  *"certificate leaf"*|*anchor*) ;;
  *) echo "FAIL: requirement is not certificate-anchored" >&2; fail=1 ;;
esac

if [ -n "$make_bin" ]; then
  if [ "$dr" = "$(dr_of "$make_bin")" ]; then echo "requirement IDENTICAL to $make_bin"
  else echo "FAIL: requirement differs from $make_bin" >&2; fail=1; fi
fi

cat <<'G'
Next (owner, not automated): with the make-built binary's grants in place, swap in this binary and relaunch;
PASS only if macOS shows no new permission prompt. Inspect rows with: sudo sqlite3 /Library/Application\ Support/com.apple.TCC/TCC.db
"select service, client, auth_value from access where client like '%stapler%'". Do not run `tccutil reset` unless a re-prompt is the goal.
G
[ "$fail" = 0 ] && echo "verify-bazel-codesign: signature checks passed" || echo "verify-bazel-codesign: FAILED" >&2
exit "$fail"
