#!/usr/bin/env bash
# Sandboxed otelc weave (spike S15b). Two modes, both run by tools/bzl/otel_weave_hermetic.bzl:
#   modcache GO TAR_OUT OVERLAY_MANIFEST       go mod download of the overlay go.mod/go.sum, tarred into TAR_OUT
#                                              (the only step that may touch the network; go.sum verifies it)
#   weave GO OUT TAGS STATUS_FILE MODCACHE_TAR SRC_MANIFEST OVERLAY_MANIFEST [GOOS GOARCH]
#                                              toolexec build of the declared source tree, no network
# Manifests are lines "dest<TAB>src"; overlay dests are relative to tools/bzl/otel_overlay.
# The weave never runs `otelc setup`: tools/bzl/otel_overlay/ holds its pre-computed output
# (go.mod/go.sum with the otelc modules, otel.instrumentation.go, otelc.runtime.go, .otelc-build/ as a tar, .bin because *.tar is commonly git-ignored).
set -euo pipefail

mode=$1
exec_root=$PWD
abs() { case "$1" in /*) echo "$1" ;; *) echo "$exec_root/$1" ;; esac; }

apply_overlay() { # DEST_DIR MANIFEST
  local rel src dest
  while IFS=$'\t' read -r rel src; do
    case "$rel" in
      *.overlay) dest=${rel%.overlay} ;;
      otelc-build.tar.bin) mkdir -p "$1/.otelc-build" && tar -xf "$exec_root/$src" -C "$1/.otelc-build"; continue ;;
      matched.json.tmpl) dest=.otelc-build/matched.json.tmpl ;;
      *) continue ;;
    esac
    mkdir -p "$1/$(dirname "$dest")"
    cp "$exec_root/$src" "$1/$dest"
  done <"$2"
}

go_bin=$(abs "$2")
[ -x "$go_bin" ] || { echo "otel_weave_hermetic: no go binary at $go_bin" >&2; exit 1; }
work=$exec_root/_otel_work
rm -rf "$work"
mkdir -p "$work/home" "$work/tmp" "$work/gocache"
# No HOME state, no GOFLAGS, no host module cache, no network: a failure here means a missing declared input.
export HOME=$work/home TMPDIR=$work/tmp GOTMPDIR=$work/tmp GOCACHE=$work/gocache GOTOOLCHAIN=local \
  GOPROXY=off GOSUMDB=off GOFLAGS= GOENV=off PATH="$(dirname "$go_bin"):/usr/bin:/bin"
unset GOROOT

if [ "$mode" = modcache ]; then
  tarball=$(abs "$3")
  mkdir -p "$work/mod" "$work/mc"
  apply_overlay "$work/mod" "$4"
  cd "$work/mod"
  # The one network step: the real proxy, every module verified against the overlay go.sum.
  GOPROXY=https://proxy.golang.org GOMODCACHE=$work/mc GOFLAGS="-modcacherw -mod=readonly" "$go_bin" mod download
  # A tarball, not a tree artifact: sandbox symlinks break //go:embed ("irregular file") inside modules.
  tar --sort=name --mtime=@0 --owner=0 --group=0 --numeric-owner -cf "$tarball" -C "$work/mc" .
  exit 0
fi

tags=$4 status=$5 modcache=$(abs "$6")
manifest=$(abs "$7") overlay=$(abs "$8")
goos=${9:-} goarch=${10:-}
out=$(abs "$3")
src=$work/src
mkdir -p "$src"
while IFS=$'\t' read -r dest from; do
  mkdir -p "$src/$(dirname "$dest")"
  cp -RL "$exec_root/$from" "$src/$dest"
done <"$manifest"
apply_overlay "$src" "$overlay"
chmod -R u+w "$src"

mkdir -p "$work/mc"
tar -xf "$modcache" -C "$work/mc"
modcache=$work/mc
export GOMODCACHE=$modcache
goroot=$("$go_bin" env GOROOT)
sed -e "s#@SRC@#$src#g" -e "s#@GOROOT@#$goroot#g" -e "s#@MODCACHE@#$modcache#g" \
  "$src/.otelc-build/matched.json.tmpl" >"$src/.otelc-build/matched.json"
rm "$src/.otelc-build/matched.json.tmpl"

get() { awk -v k="$1" '$1==k {sub(/^[^ ]+ /,""); print}' "$exec_root/$status"; }
ldflags=""
if [ -n "$(get STABLE_GIT_VERSION)" ]; then
  p=github.com/tstapler/stapler-squad/pkg/buildinfo
  ldflags="-X main.version=$(get STABLE_GIT_VERSION) -X $p.Version=$(get STABLE_GIT_VERSION) -X $p.Branch=$(get STABLE_GIT_BRANCH) -X $p.Commit=$(get STABLE_GIT_COMMIT) -X $p.Worktree=$(get STABLE_GIT_WORKTREE)"
fi

cd "$src"
"$go_bin" build -mod=readonly -o "$work/otelc" go.opentelemetry.io/otelc/tool/cmd/otelc
export GOFLAGS="-mod=readonly '-toolexec=$work/otelc toolexec'"
[ -n "$goos" ] && export GOOS=$goos GOARCH=$goarch CGO_ENABLED=0
buildargs=(-tags "${tags:+$tags,}otelcauto")
[ -n "$ldflags" ] && buildargs+=(-ldflags "$ldflags")
"$go_bin" build "${buildargs[@]}" -o "$out" .
