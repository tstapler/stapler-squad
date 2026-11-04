#!/usr/bin/env bash
# bazel run //:sync-gen (via `make sync` under BUILD_SYSTEM=bazel): copy the Bazel-built generated code
# into the source tree so plain `go build`, gopls and golangci-lint work (ADR-002). Bazel itself never
# reads the copies (gen/ and session/ent/<pkg> are excluded from Gazelle and .bazelignore'd).
# Args: the runfiles paths of the generator outputs (from BUILD.bazel). Destinations:
#   proto/session/v1/gen/...  -> gen/...                      (proto Go + connect-go)
#   tools/bzl/ent/<path>.go   -> session/ent/<path>.go        (ent; BUILD files are never copied)
# Story 2.2 adds the TypeScript tree (web-app/src/gen) once //proto/session/v1:session_ts exists.
set -eu
ws=${BUILD_WORKSPACE_DIRECTORY:-}
case "$ws" in
  ""|*/execroot/*|*/sandbox/*|*.runfiles*)
    echo "sync-gen: run via 'bazel run //:sync-gen' (BUILD_WORKSPACE_DIRECTORY unset or inside a sandbox: $ws)" >&2; exit 2 ;;
esac
[ -d "$ws/.git" ] || [ -f "$ws/.git" ] || { echo "sync-gen: $ws is not a git checkout" >&2; exit 2; }

# Replace generated trees wholesale so deleted outputs do not linger.
rm -rf "$ws/gen/proto/go"
find "$ws/session/ent" -mindepth 1 -maxdepth 1 \( -name '*.go' ! -name generate.go -o -type d ! -name schema \) -exec rm -rf {} +

n=0
# shellcheck disable=SC2048,SC2086 # $(rootpaths) expands each label to one space-separated word; paths hold no spaces
for f in $*; do
  case "$f" in
    proto/session/v1/gen/*) dest=$ws/${f#proto/session/v1/} ;;
    tools/bzl/ent/*.go)     dest=$ws/session/ent/${f#tools/bzl/ent/} ;;
    *) echo "sync-gen: unexpected output path $f" >&2; exit 2 ;;
  esac
  case "$dest" in *BUILD*|*.bazel) echo "sync-gen: refusing to copy $f" >&2; exit 2 ;; esac
  mkdir -p "$(dirname "$dest")"
  cp -L "$f" "$dest"; chmod u+w "$dest"
  n=$((n + 1))
done
bash "$ws/tools/bzl/check-gen-fresh-cheap.sh" --write
echo "sync-gen: copied $n files (gen/ and session/ent/; stamp gen/.sync-inputs.sha256)"
