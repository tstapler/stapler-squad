"""`buf generate` with protoc-gen-es from the hermetic Bun install (ADR-003), as a tree artifact equal to web-app/src/gen."""

_SCRIPT = r"""
set -eu
ROOT=$PWD
export PATH="$ROOT/$(dirname {node}):/usr/bin:/bin"
export HOME="$(mktemp -d)" BUF_CACHE_DIR="$(mktemp -d)"
T="$(mktemp -d)"
sed -e "s#@PROTOC_GEN_ES@#$ROOT/{es}#" "{template}" > "$T/gen.yaml"
"{buf}" generate proto --template "$T/gen.yaml" -o "$T/out"
mkdir -p "{out}"
cp -R "$T/out/web-app/src/gen/." "{out}/"
"""

def _buf_gen_ts_impl(ctx):
    out = ctx.actions.declare_directory(ctx.attr.out or ctx.label.name)
    ctx.actions.run_shell(
        mnemonic = "BufGenTs",
        tools = [ctx.executable.buf],
        inputs = ctx.files.srcs + [ctx.file.template, ctx.file.buf_yaml, ctx.file.node, ctx.file.node_modules],
        outputs = [out],
        command = _SCRIPT.format(
            node = ctx.file.node.path,
            es = ctx.file.node_modules.path + "/.bin/protoc-gen-es",
            template = ctx.file.template.path,
            buf = ctx.executable.buf.path,
            out = out.path,
        ),
        use_default_shell_env = False,
        # tag-reason: buf plugin output not proven byte-reproducible across hosts; placeholder tracker (#1423)
        execution_requirements = {"no-remote-cache": "1"},
    )
    return [DefaultInfo(files = depset([out]))]

buf_gen_ts = rule(
    implementation = _buf_gen_ts_impl,
    attrs = {
        "srcs": attr.label_list(allow_files = [".proto"]),
        "out": attr.string(doc = "Tree artifact name; defaults to the target name."),
        "template": attr.label(allow_single_file = True, mandatory = True),
        "buf_yaml": attr.label(allow_single_file = True, mandatory = True),
        "buf": attr.label(allow_single_file = True, mandatory = True, cfg = "exec", executable = True),
        "node": attr.label(allow_single_file = True, mandatory = True),
        "node_modules": attr.label(allow_single_file = True, mandatory = True),
    },
)
