#!/usr/bin/env bash
# Spike S1 (Task 0.3b): byte-compare Bazel-built Go proto output with the `make proto-gen` (buf generate)
# Go half, after gofmt. Usage: compare-proto.sh [gopb|buf]
#   gopb  go_proto_library + connect compiler (Task 0.3a targets)
#   buf   buf generate as a Bazel action with local pinned plugins (Task 0.3d target)
# The reference is generated into a scratch dir (never gen/): PROTO_REF_DIR=<dir with gen/proto/go> reuses one.
# PASS iff exit 0 (empty diff). Needs network only for the reference (remote buf plugins).
set -u
mode=${1:-buf}
cd "$(git rev-parse --show-toplevel)" || exit 2
tmp=$(mktemp -d "${TMPDIR:-/tmp}/compare-proto.XXXXXX"); trap 'rm -rf "$tmp"' EXIT
ref=${PROTO_REF_DIR:-}
if [ -z "$ref" ]; then
  ref=$tmp/ref
  buf generate proto --template buf.gen.go-only.yaml -o "$ref" || { echo "compare-proto: reference generation failed" >&2; exit 2; }
fi
case "$mode" in
  gopb) scan='sessionv1*_go_proto_'; targets=(//proto/session/v1:sessionv1_go_proto //proto/session/v1:sessionv1connect_go_proto); groups=(--output_groups=go_generated_srcs) ;;
  buf)  scan='gen'; targets=(//proto/session/v1:gen_go); groups=() ;;
  *) echo "usage: $0 [gopb|buf]" >&2; exit 2 ;;
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
head -${COMPARE_PROTO_HEAD:-40} "$tmp/diff.txt" >&2
exit 1
