#!/usr/bin/env bash
# Maintainer script (needs network and a Go SDK): regenerates this directory, the pre-computed
# `otelc setup` output that lets //:stapler-squad skip `otelc setup` (go mod tidy, module
# bumps, build-plan dry run) inside the sandboxed build. Run it whenever go.mod, go.sum, the otelc
# version or the custom rule (instrumentation/otelc/safeexec) changes, then commit the result.
#
# Prerequisite: bazel build //proto/session/v1:gen_go //tools/bzl/ent:ent_gen //server/web:dist_staged
# (the generated Go and the web dist come from bazel-bin).
# Usage: GO=/path/to/go tools/bzl/otel_overlay/gen.sh        # CHECK=1 diffs instead of writing
# otelc setup output is order-nondeterministic (go.mod replace order, otelc.runtime.go stub numbering, matched.json
# order), so canon() below rewrites those three files into a canonical order before they are written or compared.
set -euo pipefail

OTELC_VERSION=v1.0.1
repo=$(cd "$(dirname "${BASH_SOURCE[0]}")/../../.." && pwd -P)
out=$repo/tools/bzl/otel_overlay
: "${GO:?set GO to the go binary (the rules_go SDK one, so GOROOT matches)}"
scratch=${SSQ_OTEL_SCRATCH:-$HOME/.cache/tmp/ssq-otel-overlay}
bin=$(readlink -f "$repo/bazel-bin")
chmod -R u+w "$scratch" 2>/dev/null || true # the module cache is read-only
rm -rf "$scratch"
mkdir -p "$scratch/src" "$scratch/bin" "$scratch/tmp"

(cd "$repo" && git ls-files -z --cached --others --exclude-standard \
  | grep -zEv '^(web-app|website|docs|project_plans|third_party|tests|\.claude|tools/bzl/otel_overlay)/' \
  | tar --null -T - -cf -) | tar -C "$scratch/src" -xf -
mkdir -p "$scratch/src/gen" "$scratch/src/server/web/dist"
cp -R "$bin/proto/session/v1/gen/." "$scratch/src/gen/"
cp -R "$bin/tools/bzl/ent/." "$scratch/src/session/ent/"
rm -f "$scratch/src/session/ent/BUILD.bazel"
cp -R "$bin/server/web/dist/." "$scratch/src/server/web/dist/"
chmod -R u+w "$scratch/src"

go_dir=$(dirname "$GO")
export PATH=$go_dir:$scratch/bin:$PATH TMPDIR=$scratch/tmp GOTMPDIR=$scratch/tmp GOCACHE=$scratch/gocache \
  GOMODCACHE=$scratch/mc GOTOOLCHAIN=local GOBIN=$scratch/bin
unset GOFLAGS GOROOT
"$GO" install "go.opentelemetry.io/otelc/tool/cmd/otelc@$OTELC_VERSION"
unset GOBIN
goroot=$("$GO" env GOROOT)

# Canonical order: replace lines sorted at the end of go.mod; stub pairs sorted by linkname target and renumbered;
# matched.json keys sorted, top level by module_path, rule lists (items with a "target") by their JSON text.
# Struct new_field lists keep their order (field layout is semantic).
canon() { # GOMOD RUNTIME_GO MATCHED_JSON
  python3 - "$@" <<'PY'
import json, re, sys
mod, rt, mj = sys.argv[1:]
lines = open(mod).read().split("\n")
rep = sorted(l for l in lines if l.startswith("replace "))
rest = [l for l in lines if not l.startswith("replace ")]
body = re.sub(r"\n{3,}", "\n\n", "\n".join(rest)).rstrip("\n")
open(mod, "w").write(body + "\n\n" + "\n\n".join(rep) + "\n")
head, *blocks = re.split(r"\n\n(?=//go:linkname _(?:get|print)stack)", open(rt).read().rstrip("\n"))
pairs = {}
for b in blocks:
    m = re.match(r"//go:linkname _(get|print)stack(\d+) (\S+)", b)
    pairs.setdefault(m[2], {})[m[1]] = b
out = []
for i, n in enumerate(sorted(pairs, key=lambda n: pairs[n]["get"].split()[2])):
    out += [re.sub(r"_(get|print)stack%s\b" % n, r"_\1stack%d" % i, pairs[n][k]) for k in ("get", "print")]
open(rt, "w").write("\n\n".join([head] + out) + "\n")
def canon(v):
    if isinstance(v, dict):
        return {k: canon(x) for k, x in sorted(v.items())}
    if isinstance(v, list):
        v = [canon(x) for x in v]
        if v and all(isinstance(x, dict) and "target" in x for x in v):
            v.sort(key=lambda x: json.dumps(x, sort_keys=True))
    return v
d = canon(json.load(open(mj)))
d.sort(key=lambda x: x["module_path"])
open(mj, "w").write(json.dumps(d, separators=(",", ":")))
PY
}

cd "$scratch/src"
export GOFLAGS=-tags=otelcauto
otelc setup .
sed -i 's#^)$#\t_ "github.com/tstapler/stapler-squad/instrumentation/otelc/safeexec"\n)#' otel.instrumentation.go
otelc setup .

new=$scratch/overlay
mkdir -p "$new"
sed "s#=> $scratch/src/.otelc-build#=> ./.otelc-build#" go.mod >"$new/go.mod.overlay"
cp go.sum "$new/go.sum.overlay"
cp otel.instrumentation.go "$new/otel.instrumentation.go.overlay"
cp otelc.runtime.go "$new/otelc.runtime.go.overlay"
tar --sort=name --mtime=@0 --owner=0 --group=0 --numeric-owner -cf "$new/otelc-build.tar.bin" -C .otelc-build instrumentation pkg
sed -e "s#$scratch/src#@SRC@#g" -e "s#$goroot#@GOROOT@#g" -e "s#$scratch/mc#@MODCACHE@#g" .otelc-build/matched.json >"$new/matched.json.tmpl"
canon "$new/go.mod.overlay" "$new/otelc.runtime.go.overlay" "$new/matched.json.tmpl"
if grep -rlq "$scratch" "$new"; then echo "gen.sh: absolute scratch path leaked into the overlay" >&2; exit 1; fi

if [ -n "${CHECK:-}" ]; then
  diff -r "$new" "$out" -x gen.sh -x BUILD.bazel && echo "otel overlay is up to date"
else
  find "$out" -mindepth 1 -maxdepth 1 ! -name gen.sh ! -name BUILD.bazel -exec rm -rf {} +
  cp -R "$new/." "$out/"
fi
