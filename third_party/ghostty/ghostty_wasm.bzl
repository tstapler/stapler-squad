"""Rule that runs Ghostty's own `zig build` with the pinned Zig toolchain.

Ghostty's build.zig is the real build graph, so Bazel wraps it instead of re-describing it with
rules_zig. Inputs (source archive, patches, Zig tarball) are all sha256-pinned in MODULE.bazel,
which makes the action cacheable by Bazel's local and remote caches.

Not fully hermetic: `zig build` fetches Ghostty's own Zig dependencies (build.zig.zon, content
hashed by Zig) over the network, hence `requires-network`. Vendoring those as Bazel repos is
the follow-up that would make the action network-free.
"""

_SCRIPT = """
set -eu
zig="$PWD/$1"; src="$2"; mode="$3"; out="$PWD/$4"
tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT
# Zig resolves sources by real path; copy out of the sandbox's symlink farm.
cp -RL "$src" "$tmp/src"
cd "$tmp/src"
common="--cache-dir $tmp/cache --global-cache-dir $tmp/global"
case "$mode" in
  wasm)
    "$zig" build lib-vt -Dtarget=wasm32-freestanding -Doptimize=ReleaseSmall $common --prefix "$tmp/prefix"
    cp "$tmp/prefix/bin/ghostty-vt.wasm" "$out"
    ;;
  test)
    # A filter that matches nothing still "passes", so require the filter to add tests over a
    # baseline run that matches nothing (the second run reuses the compile cache).
    count() { "$zig" build test-lib-vt "-Dtest-filter=$1" $common --summary all > "$tmp/out.txt" 2>&1 || true; sed -n 's/.*steps succeeded; [0-9]*\\/\\([0-9]*\\) tests passed.*/\\1/p' "$tmp/out.txt" | head -n1; grep -B1 -A4 -E "error:" "$tmp/out.txt" | head -n 40 >&2; tail -n 3 "$tmp/out.txt" >&2; }
    base="$(count NO_SUCH_TEST_NAME_EVER)"
    with="$(count "$5")"
    echo "baseline=$base with_filter=$with" >&2
    [ -n "$base" ] && [ -n "$with" ] && [ "$with" -gt "$base" ] || { echo "filter '$5' ran no tests" >&2; exit 1; }
    echo "zig test-lib-vt -Dtest-filter=$5 ran $((with - base)) tests, all passed" > "$out"
    ;;
  *) echo "unknown mode $mode" >&2; exit 1 ;;
esac
"""

def _impl(ctx):
    out = ctx.actions.declare_file(ctx.attr.name + (".wasm" if ctx.attr.mode == "wasm" else ".log"))
    ctx.actions.run_shell(
        mnemonic = "GhosttyZig" + ctx.attr.mode.capitalize(),
        progress_message = "zig build (%s) of pinned Ghostty" % ctx.attr.mode,
        inputs = depset(ctx.files.srcs + ctx.files.zig_lib + [ctx.file.zig]),
        outputs = [out],
        command = _SCRIPT,
        arguments = [
            ctx.file.zig.path,
            ctx.file.build_zig.dirname,
            ctx.attr.mode,
            out.path,
            ctx.attr.test_filter,
        ],
        env = {"LC_ALL": "C", "TZ": "UTC", "PATH": "/usr/bin:/bin"},
        use_default_shell_env = False,
        execution_requirements = {"requires-network": "1"},
    )
    return [DefaultInfo(files = depset([out]))]

ghostty_zig = rule(
    implementation = _impl,
    attrs = {
        "build_zig": attr.label(allow_single_file = True, mandatory = True),
        "mode": attr.string(values = ["wasm", "test"], mandatory = True),
        "srcs": attr.label(mandatory = True),
        "test_filter": attr.string(default = ""),
        "zig": attr.label(allow_single_file = True, mandatory = True),
        "zig_lib": attr.label(mandatory = True),
    },
)
