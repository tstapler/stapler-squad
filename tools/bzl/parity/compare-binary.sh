#!/usr/bin/env bash
# P5 binary parity: the Bazel `--config=release` //:stapler-squad_plain against the `go build` command that
# `make build-embedded-tymux` runs (taken from `make -n`, so the tags and LDFLAGS are Make's own).
#
# Checks (exit 0 iff all pass):
#   1. `stapler-squad version` output is byte-equal.
#   2. `version` reports a non-empty branch and commit. rules_go does not record -ldflags in `go version -m`
#      (that line only exists for cmd/go builds), so the runtime output is the evidence for stamping.
#   3. `go version -m`: the dep set and -tags are equal. CGO_ENABLED differs on Linux by design
#      (Bazel builds pure Go, ADR-006), which is printed and allowed.
#   4. netdns=go (go.mod `godebug`): present in both binaries. cmd/go writes DefaultGODEBUG into
#      `go version -m`; rules_go has no such step, so the Bazel side carries it as
#      runtime.godebugDefault (BUILD.bazel x_defs) and is checked by `strings`.
#   5. The embedded tmux and tymuxd bytes appear in both binaries. The staged files come from the
#      pinned archives (MODULE.bazel sha256, tools/bzl/tmux/PIN, scripts/tymuxd-checksums.txt);
#      tmux must also report the PIN tag.
# The reference build needs gen/, session/ent and the embeds in the tree; any that is missing is
# staged from Bazel outputs (all gitignored except server/web/dist, which is removed again on exit).
# REF_BINARY=<path> skips the reference build. SCRATCH overrides the default scratch dir.
set -eu
here=$(cd "$(dirname "$0")" && pwd)
root=$(cd "$here/../../.." && pwd)
cd "$root"
export BAZEL_REAL="${BAZEL_REAL:-$(command -v bazelisk)}"
scratch=${SCRATCH:-$HOME/.cache/ssq-epic3-c/compare-binary}
mkdir -p "$scratch"
cleanup=()
# shellcheck disable=SC2154 # f is the trap's loop variable
trap 'for f in "${cleanup[@]:-}"; do [ -n "$f" ] && rm -rf "$f"; done' EXIT

bz() { tools/bazel build --jobs="${BAZEL_JOBS:-4}" --local_resources=memory="${BAZEL_MEM:-3000}" --config=release "$@"; }
bz //:stapler-squad_plain //session/tmux/embed:tmux_staged //session/tymux/embed:tymuxd_staged //server/web:dist_staged
# bazel-bin is a symlink to the LAST invocation's config, so resolve the release output dir explicitly.
bin=$(tools/bazel info --config=release bazel-bin 2>/dev/null)
bazel_bin=$bin/stapler-squad_plain_/stapler-squad_plain
tmux_staged=$bin/session/tmux/embed/tmux
tymuxd_staged=$bin/session/tymux/embed/tymuxd

ref=${REF_BINARY:-}
if [ -z "$ref" ]; then
  ref=$scratch/stapler-squad.make
  # Plain-vs-plain: strip the otelc weave wrapper, so this compares //:stapler-squad_plain with the same go build.
  cmd=$(make -n build-embedded-tymux 2>/dev/null | grep -E '^\s*(\./scripts/otel-auto-build\.sh )?go build .*-tags' | tail -1 | sed -E 's#^\s*(\./scripts/otel-auto-build\.sh )?##')
  [ -n "$cmd" ] || { echo "compare-binary: no go build line in make -n build-embedded-tymux" >&2; exit 2; }
  cmd=${cmd//-o stapler-squad ./-o $ref .}
  if [ ! -f gen/proto/go/session/v1/session.pb.go ] || [ ! -f session/ent/ent.go ]; then
    tools/bazel run --config=release //:sync-gen 2>&1 | tail -1
  fi
  [ -f session/tmux/embed/tmux ] || cp "$tmux_staged" session/tmux/embed/tmux
  [ -f session/tymux/embed/tymuxd ] || cp "$tymuxd_staged" session/tymux/embed/tymuxd
  if [ ! -d server/web/dist ]; then
    cp -r "$bin/server/web/dist" server/web/dist && chmod -R u+w server/web/dist
    cleanup+=("$root/server/web/dist")
  fi
  GOTMPDIR=$scratch GOFLAGS="${GOFLAGS:-} -p=${GO_P:-4}" CGO_ENABLED=1 CGO_CFLAGS=-Wno-discarded-qualifiers\ -Wno-ignored-qualifiers \
    bash -c "$cmd"
fi

rc=0
fail() { echo "FAIL: $*"; rc=1; }

"$bazel_bin" version > "$scratch/v.bazel"
"$ref" version > "$scratch/v.make"
# shellcheck disable=SC2015  # the `A && echo ok || fail` arm: echo cannot fail, so C runs only when A fails
diff "$scratch/v.bazel" "$scratch/v.make" > /dev/null && echo "ok: version output equal" || { fail "version output differs"; diff "$scratch/v.bazel" "$scratch/v.make" || true; }
# shellcheck disable=SC2015  # the `A && echo ok || fail` arm: echo cannot fail, so C runs only when A fails
grep -Eq 'branch: [^ ]+ +commit: [0-9a-f]+' "$scratch/v.bazel" && echo "ok: branch and commit non-empty" || fail "version lacks branch/commit"

# Module set and versions, minus allowed-binary-deltas.txt (which lists module paths with a reason each).
grep -v '^#' "$here/allowed-binary-deltas.txt" | sed '/^$/d' > "$scratch/allowed"
mods() { go version -m "$1" | awk '$1 == "dep" {print $2, $3} $1 == "build" && $2 ~ /^-tags=/ {print $2}' | LC_ALL=C sort; }
mods "$bazel_bin" > "$scratch/m.bazel"
mods "$ref" > "$scratch/m.make"
if diff "$scratch/m.bazel" "$scratch/m.make" > "$scratch/m.diff"; then
  echo "ok: go version -m deps and -tags equal"
else
  n=0
  while read -r mod; do
    if grep -qxF "$mod" "$scratch/allowed"; then n=$((n + 1)); else fail "go version -m delta not allowed: $mod"; fi
  done < <(grep '^[<>]' "$scratch/m.diff" | awk '{print $2}' | sort -u)
  echo "info: $n module(s) differ in version, all listed in allowed-binary-deltas.txt"
fi
echo "info: CGO_ENABLED bazel=$(go version -m "$bazel_bin" | awk '$2 ~ /CGO_ENABLED/ {print $2}') make=$(go version -m "$ref" | awk '$2 ~ /CGO_ENABLED/ {print $2}') (allowed delta, ADR-006)"

for b in "$bazel_bin" "$ref"; do
  [ "$(strings -n 6 "$b" | grep -c 'netdns=go')" -ge 1 ] || fail "netdns=go not found in $b"
done
echo "ok: netdns=go present in both"

contains() { python3 -I - "$1" "$2" <<'PY'
import sys
sys.exit(0 if open(sys.argv[2], 'rb').read().find(open(sys.argv[1], 'rb').read()) >= 0 else 1)
PY
}
pin_tag=$(sed -n 's/^tag=//p' tools/bzl/tmux/PIN)
# shellcheck disable=SC2015  # the `A && echo ok || fail` arm: echo cannot fail, so C runs only when A fails
"$tmux_staged" -V | grep -qx "tmux $pin_tag" && echo "ok: staged tmux is $pin_tag" || fail "staged tmux is not $pin_tag"
for pair in "tmux:$tmux_staged" "tymuxd:$tymuxd_staged"; do
  for b in "$bazel_bin" "$ref"; do
    contains "${pair#*:}" "$b" || fail "${pair%%:*} bytes missing from $b"
  done
  echo "ok: ${pair%%:*} embedded in both (sha256 $(sha256sum "${pair#*:}" | cut -c1-16)...)"
done
exit $rc
