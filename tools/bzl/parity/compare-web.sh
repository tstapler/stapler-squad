#!/usr/bin/env bash
# Compare the Bazel //web-app:dist tree against `make web-build` output (server/web/dist or $2).
# Usage: compare-web.sh [bazel-dist-dir] [make-dist-dir]; exits 0 iff the file lists and sha256
# lines are equal after removing deltas listed in tools/bzl/parity/allowed-deltas.txt.
set -eu
here=$(cd "$(dirname "$0")" && pwd)
a=${1:-bazel-bin/web-app/dist}
b=${2:-server/web/dist}
sums() { (cd "$1" && find . -type f | LC_ALL=C sort | xargs sha256sum); }
sums "$a" > /tmp/compare-web.a.$$
sums "$b" > /tmp/compare-web.b.$$
if diff /tmp/compare-web.a.$$ /tmp/compare-web.b.$$ > /tmp/compare-web.d.$$; then
  echo "identical: $(wc -l < /tmp/compare-web.a.$$) files"; rc=0
else
  # Each non-comment line of allowed-deltas.txt is a file path (relative to dist) whose content may differ.
  rc=0
  while read -r l; do
    f=$(echo "$l" | sed -E 's/^[<>] +[0-9a-f]{64}  //')
    if ! grep -qxF "$f" <(grep -v '^#' "$here/allowed-deltas.txt" | sed '/^$/d'); then echo "unallowed delta: $l"; rc=1; fi
  done < <(grep '^[<>]' /tmp/compare-web.d.$$)
fi
rm -f /tmp/compare-web.*.$$
exit $rc
