"""otelc-woven stapler-squad binary as a Bazel action (spike S15, ADR-005 amendment).

Non-hermetic on purpose: otelc rewrites go.mod and drives `go` through a toolexec, so the action runs
unsandboxed and is never cached remotely. See tools/bzl/otel_weave.sh for what it does and tracks.
"""

load("@rules_go//go:def.bzl", "go_context")

def _impl(ctx):
    go = go_context(ctx)
    out = ctx.actions.declare_file(ctx.label.name)
    stage = {}
    stage_files = []
    for label, dest in ctx.attr.stage.items():
        files = label.files.to_list()
        stage[dest] = files[0].path
        stage_files.extend(files)
    args = ctx.actions.args()
    args.add(out)
    args.add(go.sdk.go)
    args.add(",".join(ctx.attr.go_tags))
    args.add(ctx.info_file)
    args.add(ctx.file.web.path)
    args.add_all(["%s=%s" % (d, p) for d, p in stage.items()])
    args.add("--")
    args.add_all(ctx.files.gen)
    args.add("--")
    args.add_all(ctx.files.ent)
    ctx.actions.run(
        executable = ctx.file.script,
        arguments = [args],
        inputs = depset(
            [go.sdk.go, go.sdk.root_file, ctx.info_file] + ctx.files.srcs + [ctx.file.web] + ctx.files.gen + ctx.files.ent + stage_files,
            transitive = [go.sdk.libs, go.sdk.srcs, go.sdk.tools, go.sdk.headers],
        ),
        outputs = [out],
        mnemonic = "OtelWeave",
        progress_message = "Weaving otelc auto-instrumentation into %s" % ctx.label.name,
        use_default_shell_env = True,
        execution_requirements = {
            "local": "1",
            "no-sandbox": "1",
            "no-remote": "1",
            "no-cache": "1",
            "requires-network": "1",
        },
    )
    return DefaultInfo(files = depset([out]), executable = out)

otel_weave = rule(
    implementation = _impl,
    attrs = {
        "ent": attr.label_list(allow_files = True),
        "gen": attr.label_list(allow_files = True),
        "script": attr.label(allow_single_file = True, default = "//tools/bzl:otel_weave.sh"),
        "srcs": attr.label_list(allow_files = True, doc = "Change triggers: compiled Go library, go.mod, go.sum."),
        "stage": attr.label_keyed_string_dict(allow_files = True, doc = "embed staging file -> destination path in the build copy."),
        "go_tags": attr.string_list(),
        "web": attr.label(allow_single_file = True, mandatory = True),
        "_go_context_data": attr.label(default = "@rules_go//:go_context_data"),
    },
    executable = True,
    toolchains = ["@rules_go//go:toolchain"],
)
