#!/usr/bin/env bash
# P4 parity: byte-compare Bazel-built proto output with `make proto-gen` (buf generate), after gofmt.
# Usage: compare-proto.sh [buf|ts]   (default buf)
#   buf   //proto/session/v1:gen_go (buf generate with local pinned plugins) vs the Go half of buf.gen.yaml
#   ts    //tools/bzl/proto:gen_ts (Task 0.6e) vs the TS plugin of buf.gen.yaml (no formatter is applied by
#         `make proto-gen`); reference plugin: PROTOC_GEN_ES=<path> or web-app/node_modules/.bin/protoc-gen-es.
#         Deltas are only those listed in tools/bzl/parity/allowed-deltas.txt (paths relative to the gen root).
# The reference is generated into a scratch dir (never gen/): PROTO_REF_DIR=<dir with gen/proto/go> reuses one.
# PASS iff exit 0 (empty diff). Needs network only for the reference (remote buf plugins).
set -u
mode=${1:-buf}
cd "$(git rev-parse --show-toplevel)" || exit 2
# Without a `bazel` on PATH, use the repo wrapper over bazelisk.
command -v bazel >/dev/null || bazel() { BAZEL_REAL=${BAZEL_REAL:-$(command -v bazelisk)} tools/bazel "$@"; }
tmp=$(mktemp -d "${TMPDIR:-/tmp}/compare-proto.XXXXXX"); trap 'rm -rf "$tmp"' EXIT
if [ "$mode" = ts ]; then
  ref=${PROTO_REF_DIR:-}
  if [ -z "$ref" ]; then
    es=${PROTOC_GEN_ES:-$PWD/web-app/node_modules/.bin/protoc-gen-es}
    [ -x "$es" ] || { echo "compare-proto(ts): no reference protoc-gen-es at $es (set PROTOC_GEN_ES)" >&2; exit 2; }
    sed -e "s#@PROTOC_GEN_ES@#$es#" buf.gen.bazel-ts.yaml > "$tmp/ref.yaml"
    PATH="${REF_NODE_DIR:+$REF_NODE_DIR:}$PATH" buf generate proto --template "$tmp/ref.yaml" -o "$tmp/refout" || { echo "compare-proto(ts): reference generation failed" >&2; exit 2; }
    ref=$tmp/refout
  fi
  bazel build --config=agent --sandbox_default_allow_network=false //tools/bzl/proto:gen_ts 2>&1 | tail -3; [ "${PIPESTATUS[0]}" = 0 ] || { echo "compare-proto(ts): bazel build failed" >&2; exit 2; }
  got=$(bazel info --config=agent bazel-bin 2>/dev/null)/tools/bzl/proto/gen_ts
  [ -d "$got" ] || { echo "compare-proto(ts): $got missing" >&2; exit 2; }
  ( cd "$ref/web-app/src/gen" && find . -type f -print0 | sort -z | xargs -0 sha256sum ) > "$tmp/ref.sha"
  ( cd "$got" && find -L . -type f -print0 | sort -z | xargs -0 sha256sum ) > "$tmp/got.sha"
  n=$(wc -l < "$tmp/got.sha")
  if diff "$tmp/ref.sha" "$tmp/got.sha" > "$tmp/diff.txt"; then echo "compare-proto(ts): identical ($n files, sha256)"; exit 0; fi
  allowed=tools/bzl/parity/allowed-deltas.txt
  bad=0
  while read -r p; do
    grep -qxF "$p" <(grep -v '^#' "$allowed") || { echo "compare-proto(ts): unallowed delta: $p" >&2; bad=1; }
  done < <(grep '^[<>]' "$tmp/diff.txt" | awk '{print $3}' | sort -u)
  [ "$bad" = 0 ] && { echo "compare-proto(ts): only allow-listed deltas"; exit 0; }
  head -"${COMPARE_PROTO_HEAD:-40}" "$tmp/diff.txt" >&2; exit 1
fi
ref=${PROTO_REF_DIR:-}
if [ -z "$ref" ]; then
  ref=$tmp/ref
  buf generate proto --template buf.gen.go-only.yaml -o "$ref" || { echo "compare-proto: reference generation failed" >&2; exit 2; }
fi
case "$mode" in
  buf)  scan='gen'; targets=(//proto/session/v1:gen_go); groups=() ;;
  *) echo "usage: $0 [buf|ts]" >&2; exit 2 ;;
esac
bazel build --config=agent --keep_going --sandbox_default_allow_network=false "${groups[@]}" "${targets[@]}" 2>&1 | tail -3; [ "${PIPESTATUS[0]}" = 0 ] || echo "compare-proto: bazel build reported failures (continuing with what built)" >&2
out=$tmp/bazel/gen/proto/go/session/v1; mkdir -p "$out/sessionv1connect"
while IFS= read -r f; do
  case "$f" in
    *.connect.go) cp "$f" "$out/sessionv1connect/" ;;
    *.pb.go) cp "$f" "$out/" ;;
  esac
done < <(find -L "$(bazel info --config=agent bazel-bin 2>/dev/null)"/proto/session/v1/$scan \( -name '*.pb.go' -o -name '*.connect.go' \) -type f 2>/dev/null)
gofmt -w "$tmp/bazel" "$ref/gen" 2>/dev/null
if diff -r "$ref/gen/proto/go" "$tmp/bazel/gen/proto/go" > "$tmp/diff.txt" 2>&1; then
  echo "compare-proto($mode): identical ($(find "$tmp/bazel" -name '*.go' | wc -l) files)"; exit 0
fi
echo "compare-proto($mode): DIFFERENT; $(grep -c '^Only in' "$tmp/diff.txt") only-in lines, $(grep -c '^diff ' "$tmp/diff.txt") differing files" >&2
head -"${COMPARE_PROTO_HEAD:-40}" "$tmp/diff.txt" >&2
exit 1
