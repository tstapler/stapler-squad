#!/usr/bin/env bash
# Bazel action behind //:stapler-squad: builds the otelc-woven binary (owner decision: the one
# stapler-squad binary is always otelc auto-instrumented; ADR-005 amendment, spike S15).
#
# NON-HERMETIC by design (otelc rewrites go.mod, runs `go` through a toolexec and may resolve modules
# over the network), so the genrule runs unsandboxed with no remote caching. It builds in a private
# copy of the checkout under ~/.cache/ssq-otel; the real tree's go.mod/go.sum are never touched.
# Inputs Bazel does track (and so re-triggers the action): the compiled Go library, generated
# proto/ent code, the web dist, embed staging files and go.mod/go.sum.
#
# Usage: otel_weave.sh OUT GO TAGS STATUS_FILE WEB_DIR [STAGE_DEST=SRC ...] -- GEN_FILE... -- ENT_FILE...
set -euo pipefail

exec_root=$PWD
out=$1 go_bin=$2 tags=$3 status=$4 web=$5
shift 5
stage=()
while [ "$#" -gt 0 ] && [ "$1" != "--" ]; do stage+=("$1"); shift; done
shift
gen=()
while [ "$#" -gt 0 ] && [ "$1" != "--" ]; do gen+=("$1"); shift; done
shift
ent=("$@")

case "$out" in /*) ;; *) out=$exec_root/$out ;; esac
case "$go_bin" in /*) ;; *) go_bin=$exec_root/$go_bin ;; esac
[ -x "$go_bin" ] || { echo "otel_weave: go binary not found at $go_bin" >&2; exit 1; }

ws=$(dirname "$(readlink -f go.mod)")
{ [ -d "$ws/.git" ] || [ -f "$ws/.git" ]; } || { echo "otel_weave: $ws is not a git checkout (run unsandboxed: this action is tagged local)" >&2; exit 1; }

home=${HOME:-$(getent passwd "$(id -u)" | cut -d: -f6)}
scratch=${SSQ_OTEL_SCRATCH:-${XDG_CACHE_HOME:-$home/.cache}/ssq-otel}
tmpdir=${SSQ_OTEL_TMP:-$home/.cache/tmp/ssq-otel}
mkdir -p "$tmpdir" "$scratch/gocache"
gocache_external=${GOCACHE:+1}
export HOME=$home TMPDIR=$tmpdir GOTMPDIR=$tmpdir GOCACHE=${GOCACHE:-$scratch/gocache} GOTOOLCHAIN=local
unset GOFLAGS GOROOT
PATH=$(dirname "$go_bin"):$PATH
export PATH

# otelc is an external tool pinned to the version scripts/otel-auto-build.sh was validated with.
otelc_version=v1.0.1
if ! command -v otelc >/dev/null 2>&1; then
  export GOBIN=$scratch/otelc-$otelc_version
  [ -x "$GOBIN/otelc" ] || "$go_bin" install "go.opentelemetry.io/otelc/tool/cmd/otelc@$otelc_version"
  PATH=$GOBIN:$PATH
  export PATH
  unset GOBIN
fi

build=$scratch/src
# Bazel outputs are read-only; the previous run's copy must be writable to delete.
[ -d "$build" ] && chmod -R u+w "$build"
rm -rf "$build"
mkdir -p "$build"
(cd "$ws" && git ls-files -z --cached --others --exclude-standard \
  | grep -zEv '^(web-app|website|docs|project_plans|third_party|tests|\.claude)/' \
  | tar --null -T - -cf -) | tar -C "$build" -xf -

for f in "${gen[@]}"; do
  case "$f" in *bin/proto/session/v1/*) dest=$build/${f#*bin/proto/session/v1/} ;; *) continue ;; esac
  mkdir -p "$(dirname "$dest")"; cp "$exec_root/$f" "$dest"
done
for f in "${ent[@]}"; do
  case "$f" in *BUILD*|*.bazel) continue ;; *bin/tools/bzl/ent/*.go) dest=$build/session/ent/${f#*bin/tools/bzl/ent/} ;; *) continue ;; esac
  mkdir -p "$(dirname "$dest")"; cp "$exec_root/$f" "$dest"
done
mkdir -p "$build/server/web/dist"
cp -R "$exec_root/$web/." "$build/server/web/dist/"
for s in "${stage[@]}"; do
  dest=$build/${s%%=*}; src=$exec_root/${s#*=}
  mkdir -p "$(dirname "$dest")"; cp "$src" "$dest"
done

chmod -R u+w "$build"
ldflags=""
get() { awk -v k="$1" '$1==k {sub(/^[^ ]+ /,""); print}' "$exec_root/$status"; }
if [ -n "$(get STABLE_GIT_VERSION)" ]; then
  p=github.com/tstapler/stapler-squad/pkg/buildinfo
  ldflags="-X main.version=$(get STABLE_GIT_VERSION) -X $p.Version=$(get STABLE_GIT_VERSION) -X $p.Branch=$(get STABLE_GIT_BRANCH) -X $p.Commit=$(get STABLE_GIT_COMMIT) -X $p.Worktree=$(get STABLE_GIT_WORKTREE)"
fi

cd "$build"
buildargs=()
[ -n "$tags" ] && buildargs+=(-tags "$tags")
[ -n "$ldflags" ] && buildargs+=(-ldflags "$ldflags")
./scripts/otel-auto-build.sh go build "${buildargs[@]}" -o "$out" .

# The private GOCACHE grows by ~1-2 GB per distinct tag/ldflags set; drop it past 2 GB.
if [ -z "$gocache_external" ] && [ "$(du -sk "$scratch/gocache" 2>/dev/null | cut -f1)" -gt 2097152 ]; then
  rm -rf "$scratch/gocache"
fi
