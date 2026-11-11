"""Generates session/ent with Bazel: one declared-output action plus one go_library per package."""

load("@rules_go//go:def.bzl", "go_context", "go_library")

_PREFIX = "github.com/tstapler/stapler-squad/session/ent"

# External modules the generated code imports (import prefix -> label prefix).
_EXTERNAL = {
    "entgo.io/ent": "@io_entgo_ent//",
    "github.com/google/uuid": "@com_github_google_uuid//",
}

def _ent_gen_impl(ctx):
    go = go_context(ctx)
    out_dir = ctx.bin_dir.path + "/" + ctx.label.package
    args = ctx.actions.args()
    args.add("-pkg", _PREFIX)
    args.add("-schema-pkg", _PREFIX + "/schema")
    args.add("-flags", ctx.file.flags)
    args.add("-out", out_dir)
    args.add("-expect", ctx.file.packages)
    args.add_all(ctx.files.schema_srcs)
    ctx.actions.run(
        executable = ctx.executable.entc,
        arguments = [args],
        inputs = depset([ctx.file.flags, ctx.file.packages, go.sdk.go, go.sdk.root_file] + ctx.files.schema_srcs, transitive = [go.sdk.libs, go.sdk.srcs, go.sdk.tools, go.sdk.headers]),
        # goimports (run by ent's formatter) shells out to `go env`, so the SDK go must be on PATH.
        env = {"PATH": go.sdk.go.dirname, "GOROOT": go.sdk.root_file.dirname, "GOFLAGS": "-mod=mod", "GOPROXY": "off"},
        outputs = ctx.outputs.outs,
        mnemonic = "EntGen",
        progress_message = "Generating session/ent",
    )
    return DefaultInfo(files = depset(ctx.outputs.outs))

ent_gen = rule(
    implementation = _ent_gen_impl,
    attrs = {
        "entc": attr.label(executable = True, cfg = "exec", mandatory = True),
        "flags": attr.label(allow_single_file = True, mandatory = True),
        "outs": attr.output_list(mandatory = True),
        "packages": attr.label(allow_single_file = True, mandatory = True),
        "schema_srcs": attr.label_list(allow_files = [".go"], mandatory = True),
        "_go_context_data": attr.label(default = "@rules_go//:go_context_data"),
    },
    toolchains = ["@rules_go//go:toolchain"],
)

def _dep(imp):
    if imp == _PREFIX:
        return ":ent"
    if imp == _PREFIX + "/schema":
        return ":schema"
    if imp.startswith(_PREFIX + "/"):
        return ":" + imp[len(_PREFIX) + 1:]
    for p, label in _EXTERNAL.items():
        if imp == p:
            return label + ":" + p.split("/")[-1]
        if imp.startswith(p + "/"):
            return label + imp[len(p) + 1:]
    fail("ent_packages: no label mapping for import %s; add it to _EXTERNAL" % imp)

def ent_packages(name, packages, entc, flags, packages_file, schema_srcs, visibility = None):
    """Declares ent_gen(name) and one go_library per generated package.

    Args:
      name: the ent_gen target.
      packages: ENT_PACKAGES, a list of (dir, files, imports) from packages.bzl.
      entc: the generator go_binary.
      flags: file holding the `ent generate` flags (ent_flags.txt).
      packages_file: packages.bzl as a file, so a stale listing fails the action.
      schema_srcs: the schema .go files.
      visibility: of the ent_gen target (the generated libraries are always public).
    """
    outs = [(d + "/" if d != "." else "") + f for d, fs, _ in packages for f in fs]
    ent_gen(name = name, entc = entc, flags = flags, packages = packages_file, schema_srcs = schema_srcs, outs = outs, visibility = visibility)
    for d, fs, imports in packages:
        lib = "ent" if d == "." else d
        go_library(
            name = lib,
            srcs = [(d + "/" if d != "." else "") + f for f in fs],
            importpath = _PREFIX + ("" if d == "." else "/" + d),
            visibility = ["//visibility:public"],
            deps = sorted({_dep(i): True for i in imports}.keys()),
        )
