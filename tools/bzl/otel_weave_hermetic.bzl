"""Sandboxed otelc-woven stapler-squad (spike S15b; the default //:stapler-squad is still otel_weave).

Declared inputs only: module sources from the :stapler-squad_lib archive closure, Bazel-generated proto/ent/web,
the committed otelc overlay (tools/bzl/otel_overlay), the Go SDK and a module cache fetched once by a
go.sum-verified `go mod download`. See tools/bzl/otel_weave_hermetic.sh.
"""

load("@rules_go//go:def.bzl", "GoArchive", "go_context")

_MODULE = "github.com/tstapler/stapler-squad"
_OVERLAY = "tools/bzl/otel_overlay/"
_ENT = "tools/bzl/ent/"
_PROTO = "proto/session/v1/"

def _sdk_inputs(go):
    return depset([go.sdk.go, go.sdk.root_file], transitive = [go.sdk.libs, go.sdk.srcs, go.sdk.tools, go.sdk.headers])

def _line(dest, f):
    return "%s\t%s" % (dest, f.path)

def _overlay_manifest(ctx):
    out = ctx.actions.declare_file(ctx.label.name + ".overlay.tsv")
    ctx.actions.write(out, "".join([_line(f.short_path[len(_OVERLAY):], f) + "\n" for f in ctx.files.overlay]))
    return out

def _modcache_impl(ctx):
    go = go_context(ctx)
    tree = ctx.actions.declare_file(ctx.label.name + ".tar")
    manifest = _overlay_manifest(ctx)
    ctx.actions.run(
        executable = ctx.file.script,
        arguments = ["modcache", go.sdk.go.path, tree.path, manifest.path],
        inputs = depset([manifest] + ctx.files.overlay, transitive = [_sdk_inputs(go)]),
        outputs = [tree],
        mnemonic = "OtelModcache",
        execution_requirements = {"requires-network": "1"},  # the one fetch; go.sum-verified, then cached
    )
    return DefaultInfo(files = depset([tree]))

def _closure_files(ctx):
    archives = [l[GoArchive].transitive for l in [ctx.attr.lib] + ctx.attr.extra_libs]
    return depset([f for d in depset(transitive = archives).to_list() if d.importpath.startswith(_MODULE) for f in list(d.srcs) + list(d._embedsrcs)])

def _sources(ctx):
    lines = [_line(f.short_path, f) for f in _closure_files(ctx).to_list() if f.is_source]
    for f in ctx.files.gen:
        lines.append(_line("gen/proto/go/session/v1/" + f.path.split("/gen/proto/go/session/v1/")[1], f))
    for f in ctx.files.ent:
        if f.short_path.startswith(_ENT) and f.extension == "go":
            lines.append(_line("session/ent/" + f.short_path[len(_ENT):], f))
    lines.append(_line("server/web/dist", ctx.files.web[0]))  # a directory: the script copies with cp -R
    for label, dest in ctx.attr.stage.items():
        lines.append(_line(dest, label.files.to_list()[0]))
    return lines

def _weave_impl(ctx):
    go = go_context(ctx)
    out = ctx.actions.declare_file(ctx.label.name)
    manifest = ctx.actions.declare_file(ctx.label.name + ".src.tsv")
    ctx.actions.write(manifest, "".join([l + "\n" for l in _sources(ctx)]))
    overlay = _overlay_manifest(ctx)
    modcache = ctx.attr.modcache.files.to_list()[0]
    ctx.actions.run(
        executable = ctx.file.script,
        arguments = ["weave", go.sdk.go.path, out.path, ",".join(ctx.attr.go_tags), ctx.info_file.path, modcache.path, manifest.path, overlay.path, go.mode.goos, go.mode.goarch],
        inputs = depset(
            [manifest, overlay, modcache, ctx.info_file] + ctx.files.overlay + ctx.files.gen + ctx.files.ent + ctx.files.web + [f for l in ctx.attr.stage for f in l.files.to_list()],
            transitive = [_sdk_inputs(go), _closure_files(ctx)],
        ),
        outputs = [out],
        mnemonic = "OtelWeaveHermetic",
        progress_message = "Weaving (sandboxed) %s" % ctx.label.name,
        execution_requirements = {"no-remote-exec": "1"},  # sandboxed + remote-cacheable; run locally
    )
    return DefaultInfo(files = depset([out]), executable = out)

_COMMON = {
    "overlay": attr.label(default = "//tools/bzl/otel_overlay:overlay", allow_files = True),
    "script": attr.label(allow_single_file = True, default = "//tools/bzl:otel_weave_hermetic.sh"),
    "_go_context_data": attr.label(default = "@rules_go//:go_context_data"),
}

otel_modcache = rule(
    implementation = _modcache_impl,
    attrs = _COMMON,
    toolchains = ["@rules_go//go:toolchain"],
)

otel_weave_hermetic = rule(
    implementation = _weave_impl,
    attrs = dict(_COMMON, **{
        "ent": attr.label_list(allow_files = True),
        "gen": attr.label_list(allow_files = True),
        "extra_libs": attr.label_list(providers = [GoArchive], doc = "Packages only the weave imports (the otelc custom rule)."),
        "go_tags": attr.string_list(),
        "lib": attr.label(mandatory = True, providers = [GoArchive]),
        "modcache": attr.label(mandatory = True),
        "stage": attr.label_keyed_string_dict(allow_files = True),
        "web": attr.label(mandatory = True, allow_single_file = True),
    }),
    executable = True,
    toolchains = ["@rules_go//go:toolchain"],
)
