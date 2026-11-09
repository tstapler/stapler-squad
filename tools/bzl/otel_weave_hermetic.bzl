"""Sandboxed otelc-woven stapler-squad (spike S15b); path logic lives in otel_weave_hermetic.sh."""

load("@rules_go//go:def.bzl", "GoArchive", "go_context")

def _impl(ctx):
    go = go_context(ctx)
    weave = ctx.attr.mode == "weave"
    out = ctx.actions.declare_file(ctx.label.name + ("" if weave else ".tar"))
    closure = depset(transitive = [x[GoArchive].transitive for x in ctx.attr.libs]).to_list()
    srcs = [f for d in closure if d.importpath.startswith("github.com/tstapler/stapler-squad") for f in list(d.srcs) + list(d._embedsrcs) if f.is_source]
    groups = [("--overlay", ctx.files.overlay), ("--src", srcs), ("--gen", ctx.files.gen), ("--ent", ctx.files.ent), ("--web", ctx.files.web), ("--stage", ctx.files.stage), ("--tag", ctx.attr.go_tags), ("--modcache", ctx.files.modcache), ("--status", [ctx.info_file] if weave else []), ("--target", [go.mode.goos + "/" + go.mode.goarch] if weave else [])]
    args = ctx.actions.args().use_param_file("@%s", use_always = True).set_param_file_format("multiline")
    args.add_all([ctx.attr.mode, go.sdk.go, out])
    files = []
    for flag, items in groups:
        args.add_all(items, before_each = flag)
        files += [f for f in items if type(f) == "File"]
    sdk = depset([go.sdk.go, go.sdk.root_file] + files, transitive = [go.sdk.libs, go.sdk.srcs, go.sdk.tools, go.sdk.headers])

    # tag-reason: modcache fetch needs the network, go.sum-verified; the weave is sandboxed and remote-cacheable; no per-issue tracker yet, placeholder (#1423)
    reqs = {"no-remote-exec": "1"} if weave else {"requires-network": "1"}
    ctx.actions.run(executable = ctx.file.script, arguments = [args], outputs = [out], inputs = sdk, execution_requirements = reqs, mnemonic = "OtelWeaveHermetic" if weave else "OtelModcache")
    return DefaultInfo(files = depset([out]), executable = out)  # executable only satisfies the rule type; the modcache tar is never run

otel_weave_hermetic = rule(
    implementation = _impl,
    attrs = {
        "ent": attr.label_list(allow_files = True),
        "gen": attr.label_list(allow_files = True),
        "go_tags": attr.string_list(),
        "libs": attr.label_list(providers = [GoArchive]),
        "mode": attr.string(default = "weave", values = ["weave", "modcache"]),
        "modcache": attr.label(allow_single_file = True),
        "overlay": attr.label(default = "//tools/bzl/otel_overlay:overlay", allow_files = True),
        "script": attr.label(allow_single_file = True, default = "//tools/bzl:otel_weave_hermetic.sh"),
        "stage": attr.label_list(allow_files = True),
        "web": attr.label(allow_single_file = True),
        "_go_context_data": attr.label(default = "@rules_go//:go_context_data"),
    },
    executable = True,
    toolchains = ["@rules_go//go:toolchain"],
)

def _cross_impl(ctx):
    out = ctx.actions.declare_file(ctx.label.name)
    ctx.actions.symlink(output = out, target_file = ctx.attr.target[0][DefaultInfo].files.to_list()[0])
    return DefaultInfo(files = depset([out]))

_platform_tx = transition(implementation = lambda _settings, attr: {"//command_line_option:platforms": str(attr.platform)}, inputs = [], outputs = ["//command_line_option:platforms"])

# Builds `target` for `platform`: the weave reads the target GOOS/GOARCH from rules_go.
otel_cross_binary = rule(
    implementation = _cross_impl,
    attrs = {"platform": attr.label(mandatory = True), "target": attr.label(cfg = _platform_tx, mandatory = True)},
)
