#!/usr/bin/env bash
# Sandboxed otelc weave (spike S15b), run by tools/bzl/otel_weave_hermetic.bzl with one @paramfile (one arg per line):
#   MODE GO OUT, then repeated flags: --overlay F  --src F  --gen F  --ent F  --web F  --stage F  --tag T
#   --modcache F  --status F  --target GOOS/GOARCH (the modcache action gets neither --status nor --target, so it is shared across platforms).
#   MODE is modcache (go mod download of the overlay go.mod/go.sum into the tarball OUT; the only step that may touch
#   the network, go.sum verifies it) or weave (toolexec build of the declared source tree).
# Destinations are derived from the paths: bazel-out/*/bin/ is stripped, generated proto/ent are re-rooted.
# The weave never runs `otelc setup`: tools/bzl/otel_overlay/ holds its pre-computed output
# (go.mod/go.sum with the otelc modules, otel.instrumentation.go, otelc.runtime.go, .otelc-build/ as a tar, .bin because *.tar is commonly git-ignored).
set -euo pipefail

if [[ $1 == @* ]]; then mapfile -t args <"${1#@}"; set -- "${args[@]}"; fi
mode=$1 go_arg=$2 out_arg=$3
shift 3
exec_root=$PWD
abs() { case "$1" in /*) echo "$1" ;; *) echo "$exec_root/$1" ;; esac; }
overlay=() srcs=() gens=() ents=() webs=() stages=() tags=() modcache_arg='' status='' target=''
while [ $# -gt 0 ]; do
  case "$1" in
    --overlay) overlay+=("$2") ;; --src) srcs+=("$2") ;; --gen) gens+=("$2") ;; --ent) ents+=("$2") ;;
    --web) webs+=("$2") ;; --stage) stages+=("$2") ;; --tag) tags+=("$2") ;; --modcache) modcache_arg=$2 ;; --status) status=$2 ;; --target) target=$2 ;;
    *) echo "otel_weave_hermetic: bad flag $1" >&2; exit 2 ;;
  esac
  shift 2
done

apply_overlay() { # DEST_DIR (the overlay files are flat: dest is the basename)
  local f rel dest
  for f in "${overlay[@]}"; do
    rel=$(basename "$f")
    case "$rel" in
      *.overlay) dest=${rel%.overlay} ;;
      otelc-build.tar.bin) mkdir -p "$1/.otelc-build" && tar -xf "$exec_root/$f" -C "$1/.otelc-build"; continue ;;
      matched.json.tmpl) dest=.otelc-build/matched.json.tmpl ;;
      *) continue ;;
    esac
    mkdir -p "$1/$(dirname "$dest")"
    cp "$exec_root/$f" "$1/$dest"
  done
}

go_bin=$(abs "$go_arg")
[ -x "$go_bin" ] || { echo "otel_weave_hermetic: no go binary at $go_bin" >&2; exit 1; }
# A fixed work path keeps the Go build IDs byte-reproducible (a per-sandbox path leaks into them); Bazel's hermetic
# /tmp is empty and private to the action, so use it when it is, else fall back to a path inside the exec root.
if [ -w /tmp ] && [ -z "$(ls -A /tmp)" ]; then work=/tmp/ssq-otel-work; else work=$exec_root/_otel_work; fi
rm -rf "$work"
mkdir -p "$work/home" "$work/tmp" "$work/gocache"
# No HOME state, no GOFLAGS, no host module cache, no network: a failure here means a missing declared input.
go_dir=$(dirname "$go_bin")
export HOME=$work/home TMPDIR=$work/tmp GOTMPDIR=$work/tmp GOCACHE=$work/gocache GOTOOLCHAIN=local \
  GOPROXY=off GOSUMDB=off GOFLAGS='' GOENV=off PATH="$go_dir:/usr/bin:/bin"
unset GOROOT

if [ "$mode" = modcache ]; then
  tarball=$(abs "$out_arg")
  mkdir -p "$work/mod" "$work/mc"
  apply_overlay "$work/mod"
  cd "$work/mod"
  # The one network step: the real proxy, every module verified against the overlay go.sum.
  GOPROXY=https://proxy.golang.org GOMODCACHE=$work/mc GOFLAGS="-modcacherw -mod=readonly" "$go_bin" mod download
  # A tarball, not a tree artifact: sandbox symlinks break //go:embed ("irregular file") inside modules.
  tar --sort=name --mtime=@0 --owner=0 --group=0 --numeric-owner -cf "$tarball" -C "$work/mc" .
  exit 0
fi

modcache=$(abs "$modcache_arg")
out=$(abs "$out_arg")
src=$work/src
mkdir -p "$src"
put() { # SRC_PATH DEST
  [[ $2 == */* ]] && mkdir -p "$src/${2%/*}"
  cp -RL "$exec_root/$1" "$src/$2"
}
rel() { REPLY=$1; [[ $1 =~ ^bazel-out/[^/]+/bin/(.*)$ ]] && REPLY=${BASH_REMATCH[1]}; return 0; }
for f in "${srcs[@]}" "${webs[@]}" "${stages[@]}"; do rel "$f"; put "$f" "$REPLY"; done
for f in "${gens[@]}"; do put "$f" "gen/proto/go/session/v1/${f#*/gen/proto/go/session/v1/}"; done
for f in "${ents[@]}"; do
  if [[ $f == *.go ]]; then rel "$f"; put "$f" "session/ent/${REPLY#tools/bzl/ent/}"; fi
done
apply_overlay "$src"
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
mkdir -p "$work/bin"
"$go_bin" build -mod=readonly -o "$work/bin/otelc" go.opentelemetry.io/otelc/tool/cmd/otelc
# By name on PATH, not by path: cmd/go hashes the toolexec string into action IDs, so a sandbox path would make the binary differ per run.
export PATH=$work/bin:$PATH
export GOFLAGS="-mod=readonly -trimpath '-toolexec=otelc toolexec'"
goos=${target%/*} goarch=${target#*/}
# Cross-compile (--platforms): the target OS/arch come from rules_go; cgo is off, as for the plain Windows build.
# Exception (UNVERIFIED, needs a Mac): a darwin target on a darwin host keeps cgo, with clang told the target arch,
# because infoplist_darwin.go and the other darwin cgo files need it (Task 5.1d).
if [ "$goos/$goarch" != "$("$go_bin" env GOHOSTOS)/$("$go_bin" env GOHOSTARCH)" ]; then
  export GOOS=$goos GOARCH=$goarch
  if [ "$goos" = darwin ] && [ "$("$go_bin" env GOHOSTOS)" = darwin ]; then
    clang_arch=$goarch
    [ "$goarch" = amd64 ] && clang_arch=x86_64
    export CGO_ENABLED=1 CC="clang -arch $clang_arch"
  else
    export CGO_ENABLED=0
  fi
fi
# macOS TCC identity (Task 5.1b, UNVERIFIED, needs a Mac): embed the staged Info.plist as __TEXT,__info_plist, as the
# Makefile does. Skipped when cgo is off (the darwin cgo files, including the Info.plist one, are then not built).
if [ "$goos" = darwin ] && [ "${CGO_ENABLED:-1}" != 0 ] && [ -f "$src/macos/Info.plist" ]; then
  export CGO_LDFLAGS="-sectcreate __TEXT __info_plist $src/macos/Info.plist"
fi
taglist=$(IFS=,; echo "${tags[*]:-}")
buildargs=(-buildvcs=false -tags "${taglist:+$taglist,}otelcauto")
[ -n "$ldflags" ] && buildargs+=(-ldflags "$ldflags")
"$go_bin" build "${buildargs[@]}" -o "$out" .
