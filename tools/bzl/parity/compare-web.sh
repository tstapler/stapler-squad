#!/usr/bin/env bash
# P6 web parity: compare the Bazel //web-app:dist tree with the Bun reference build and print deltas.
#
# Usage: compare-web.sh [bazel-dist-dir] [reference-dist-dir]
#   Dirs given: compare only.
#   No args: build both ways, then compare.
#     bazel side: bazel build //web-app:dist  (BAZEL_REAL=$(command -v bazelisk) tools/bazel)
#     reference : the step `make web-build` runs (`cd web-app && WEB_MINIFY=0 bun build/build.ts`) with the
#                 Bazel-pinned Bun and Node (S4: parity holds against a pinned environment, not an
#                 arbitrary developer one). `make web-build` itself is not invoked because its install
#                 step goes through tools/shared-node-modules, a store shared across checkouts. It writes
#                 web-app/out like make does (source maps hold paths relative to the out dir, so the
#                 out dir must sit next to src/ to match). Set
#                 REF_MAKE=1 to run `make web-build` and compare server/web/dist instead.
# Exit 0 iff file lists and sha256 lines are equal after removing deltas listed in allowed-deltas.txt
# (one dist-relative path per line; each entry needs a reason comment above it).
set -eu
here=$(cd "$(dirname "$0")" && pwd)
root=$(cd "$here/../../.." && pwd)
scratch=$(mktemp -d "${TMPDIR:-$HOME/.cache}/compare-web.XXXXXX")
trap 'chmod -R u+w "$scratch" 2>/dev/null; rm -rf "$scratch"' EXIT

a=${1:-}
b=${2:-}
if [ -z "$a" ]; then
  cd "$root"
  export BAZEL_REAL="${BAZEL_REAL:-$(command -v bazelisk)}"
  tools/bazel build //web-app:dist
  a="$root/bazel-bin/web-app/dist"
  if [ "${REF_MAKE:-0}" = 1 ]; then
    make web-build WEB_MINIFY=0
    b="$root/server/web/dist"
  else
    ext=$(tools/bazel info output_base 2>/dev/null)/external
    [ -d "$root/web-app/node_modules" ] || { echo "compare-web: web-app/node_modules missing (hardlink-copy $ext/+bun_install+web_node_modules/node_modules)" >&2; exit 2; }
    [ -d "$root/web-app/src/gen" ] || { echo "compare-web: web-app/src/gen missing (copy bazel-bin/proto/session/v1/ts)" >&2; exit 2; }
    PATH="$ext/+host_tool+node_host:$ext/+host_tool+bun_host:/usr/bin:/bin" \
      BUN_INSTALL_GLOBAL_STORE=0 WEB_MINIFY=0 \
      bash -c 'cd "$1/web-app" && bun build/build.ts' _ "$root"
    b="$root/web-app/out"
  fi
fi
b=${b:-$root/server/web/dist}

sums() { (cd "$1" && find . -type f | LC_ALL=C sort | xargs sha256sum); }
sums "$a" > "$scratch/a"
sums "$b" > "$scratch/b"
rc=0
if diff "$scratch/a" "$scratch/b" > "$scratch/d"; then
  echo "identical: $(wc -l < "$scratch/a") files"
else
  grep -v '^#' "$here/allowed-deltas.txt" | sed '/^$/d' > "$scratch/allowed"
  while read -r l; do
    f=$(echo "$l" | sed -E 's/^[<>] +[0-9a-f]{64}  //')
    if ! grep -qxF "$f" "$scratch/allowed"; then echo "unallowed delta: $l"; rc=1; fi
  done < <(grep '^[<>]' "$scratch/d")
fi
exit $rc
