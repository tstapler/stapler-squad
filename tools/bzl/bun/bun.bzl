"""Hermetic Bun web build (ADR-003: Bun-owned build, pinned toolchain, `bun install` as a repository rule)."""

def _host_platform(rctx):
    os = rctx.os.name.lower()
    arch = rctx.os.arch
    os = "darwin" if os.startswith("mac") else ("linux" if os.startswith("linux") else fail("unsupported OS " + os))
    arch = "arm64" if arch in ("aarch64", "arm64") else ("x64" if arch in ("amd64", "x86_64") else fail("unsupported arch " + arch))
    return os + "_" + arch

def _host_tool_impl(rctx):
    # Only the matching archive is resolved, so the other platforms' archives are never downloaded.
    want = _host_platform(rctx)
    for label, platform in rctx.attr.tools.items():
        if platform == want:
            rctx.symlink(rctx.path(label), rctx.attr.tool)
            rctx.file("BUILD.bazel", 'exports_files(["%s"])\n' % rctx.attr.tool)
            return
    fail("no pinned %s for platform %s" % (rctx.attr.tool, want))

host_tool = repository_rule(
    implementation = _host_tool_impl,
    attrs = {
        "tool": attr.string(mandatory = True),
        "tools": attr.label_keyed_string_dict(mandatory = True, allow_files = True),
    },
    doc = "Exposes the pinned binary of the host OS/arch as @<name>//:<tool>.",
)

def _bun_install_impl(rctx):
    bun = rctx.path(rctx.attr.bun)
    node = rctx.path(rctx.attr.node)
    home = rctx.getenv("HOME")

    # Mirror web-app/ layout inside the repo so `file:./eslint-plugin-*` and patches resolve.
    for f in [rctx.attr.package_json, rctx.attr.bun_lock, rctx.attr.bunfig]:
        rctx.file(f.name, rctx.read(f))
    for f in rctx.attr.files:
        rel = (f.package + "/" + f.name)[len(rctx.attr.strip_prefix):]
        rctx.file(rel, rctx.read(f))
    rctx.file("BUILD.bazel", 'exports_files(["node_modules"])\n')
    res = rctx.execute(
        [str(bun), "install", "--frozen-lockfile", "--no-progress"],
        environment = {
            "HOME": home,  # only for bun's global download cache (integrity-checked by bun.lock)
            "PATH": str(node.dirname) + ":/usr/bin:/bin",
            "npm_config_user_agent": "bun/" + rctx.attr.bun_version,
            # The global virtual store makes build output embed ~/.bun/install/cache/links/<pkg>-<hash>
            # paths (CSS source comments, debugId), so the install must be location-independent.
            "BUN_INSTALL_GLOBAL_STORE": "0",
        },
        timeout = 1800,
        quiet = False,
    )
    if res.return_code != 0:
        fail("bun install failed:\n" + res.stdout + res.stderr)

bun_install = repository_rule(
    implementation = _bun_install_impl,
    attrs = {
        "bun": attr.label(allow_single_file = True, mandatory = True),
        "node": attr.label(allow_single_file = True, mandatory = True),
        "bun_version": attr.string(mandatory = True),
        "package_json": attr.label(allow_single_file = True, mandatory = True),
        "bun_lock": attr.label(allow_single_file = True, mandatory = True),
        "bunfig": attr.label(allow_single_file = True, mandatory = True),
        "files": attr.label_list(allow_files = True),
        "strip_prefix": attr.string(default = "web-app/"),
    },
)

# The action is not sandboxed (exec_tags), so $PWD is shared by every bun_dist: work and HOME dirs are
# per output path (target AND configuration: //... builds the same target under several configs),
# or two concurrent actions delete each other's tree.
_SCRIPT = r"""
set -eu
T0=$(date +%s.%N)
ROOT=$PWD
export PATH="$ROOT/$(dirname {node}):$ROOT/$(dirname {bun}):/usr/bin:/bin"
export HOME="$ROOT/_home_{wid}"; mkdir -p "$HOME"
export WEB_MINIFY={minify}
W="$ROOT/_work_{wid}"; chmod -R u+w "$W" 2>/dev/null || true; rm -rf "$W"; mkdir -p "$W/node_modules"
# Copy (not symlink) sources: bun resolves realpath and would otherwise walk up from the real
# source tree to the host's web-app/node_modules.
tar -chf - -T "{manifest}" | tar -xf - -C "$W" --strip-components=1
# Bun embeds paths relative to the realpath of node_modules in source maps and debugIds, so the
# tree must live under the work dir like it does in a `make` checkout. Hardlink-copy keeps that cheap.
cp -al "$ROOT/{nm}/." "$W/node_modules" 2>/dev/null || cp -a "$ROOT/{nm}/." "$W/node_modules"
cp "{allowlist}" "$W/public-allowlist.json"
{gencmd}T1=$(date +%s.%N)
cd "$W"
{prefix}bun build/build.ts --out "$W/out" --stats "$W/stats.json" --allowlist "$W/public-allowlist.json"
T2=$(date +%s.%N)
cd "$ROOT"
mkdir -p "{out}"
cp -R "$W/out/." "{out}/"
rm -rf "$W"
T3=$(date +%s.%N)
echo "bunwebbuild-timing stage=$(echo "$T1 - $T0" | bc 2>/dev/null || echo na) build=$(echo "$T2 - $T1" | bc 2>/dev/null || echo na) publish=$(echo "$T3 - $T2" | bc 2>/dev/null || echo na)"
"""

def _bun_dist_impl(ctx):
    out = ctx.actions.declare_directory(ctx.label.name)
    manifest = ctx.actions.declare_file(ctx.label.name + ".manifest")
    ctx.actions.write(manifest, "\n".join([f.path for f in ctx.files.srcs]) + "\n")
    nm = ctx.file.node_modules
    outputs = [out]
    prefix = ctx.attr.cmd_prefix
    if ctx.attr.trace:
        # Spike probe (Task 0.6b): record every file syscall of the bun process.
        log = ctx.actions.declare_file(ctx.label.name + ".strace")
        outputs.append(log)
        prefix = "/home/linuxbrew/.linuxbrew/bin/strace -f -qq -y -s 512 -e trace=socket,connect,openat,open,stat,newfstatat,lstat,access,readlink,readlinkat,mkdir,mkdirat,rename,renameat,renameat2,unlink,unlinkat,rmdir,chdir,execve,symlink,symlinkat,link,linkat,truncate,chmod,utimensat -o $ROOT/" + log.path + " "
    script = _SCRIPT.format(
        wid = out.path.replace("/", "_"),
        node = ctx.file.node.path,
        bun = ctx.file.bun.path,
        minify = ctx.attr.minify,
        manifest = manifest.path,
        nm = nm.path,
        allowlist = ctx.file.allowlist.path,
        out = out.path,
        prefix = prefix,
        gencmd = ("mkdir -p \"$W/src/gen\"; cp -RL \"$ROOT/" + ctx.file.gen.path + "/.\" \"$W/src/gen/\"; chmod -R u+w \"$W/src/gen\"\n") if ctx.file.gen else "",
    )
    ctx.actions.run_shell(
        mnemonic = "BunWebBuild",
        inputs = ctx.files.srcs + [manifest, nm, ctx.file.node, ctx.file.bun, ctx.file.allowlist] + ([ctx.file.gen] if ctx.file.gen else []),
        outputs = outputs,
        command = script,
        use_default_shell_env = False,
        execution_requirements = {k: "1" for k in ctx.attr.exec_tags},
    )
    return [DefaultInfo(files = depset(outputs))]

bun_dist = rule(
    implementation = _bun_dist_impl,
    attrs = {
        "srcs": attr.label_list(allow_files = True),
        "node_modules": attr.label(allow_single_file = True, mandatory = True),
        "bun": attr.label(allow_single_file = True, mandatory = True),
        "node": attr.label(allow_single_file = True, mandatory = True),
        "allowlist": attr.label(allow_single_file = True, mandatory = True),
        "gen": attr.label(allow_single_file = True, doc = "Tree artifact copied to src/gen (generated proto TS)."),
        "minify": attr.string(default = "0"),
        "cmd_prefix": attr.string(default = ""),
        "trace": attr.bool(default = False),
        # tag-reason: sandboxed action cost 2.6x make (S4); node_modules not proven byte-reproducible (Story 7.7); no per-issue tracker yet, placeholder (#1423)
        "exec_tags": attr.string_list(default = ["no-sandbox", "no-remote-cache"], doc = "S4: no-sandbox because the sandboxed action cost 2.6x make; no-remote-cache because the repository rule's node_modules is not proven byte-reproducible (Story 7.7)."),
    },
)

_JEST_SCRIPT = r"""#!/usr/bin/env bash
set -eu
RF="${{TEST_SRCDIR}}"
rp() {{ case "$1" in ../*) echo "$RF/${{1#../}}";; *) echo "$RF/_main/$1";; esac; }}
NODE="$(rp {node})"; BUN="$(rp {bun})"; NM="$(rp {nm})"
export PATH="$(dirname "$NODE"):$(dirname "$BUN"):/usr/bin:/bin"
export HOME="$TEST_TMPDIR/home"; mkdir -p "$HOME"
WS="$TEST_TMPDIR/ws"; W="$WS/web-app"; mkdir -p "$W/node_modules"
# jest.config.js lists roots outside web-app/ for projects we do not run; jest validates them anyway.
mkdir -p "$WS/scripts/dev-stack" "$WS/tests/e2e"
tar -chf - -C "$RF/_main" -T "$(rp {manifest})" | tar -xf - -C "$W" --strip-components=1
# Files outside web-app/ that suites read through ../ paths, staged at their repo-relative location.
if [ -s "$(rp {repo_manifest})" ]; then tar -chf - -C "$RF/_main" -T "$(rp {repo_manifest})" | tar -xf - -C "$WS"; fi
cp -al "$NM/." "$W/node_modules" 2>/dev/null || cp -a "$NM/." "$W/node_modules"
{gencmd}cd "$W"
mkdir -p "$TEST_UNDECLARED_OUTPUTS_DIR"
# Bazel's shard protocol: TEST_TOTAL_SHARDS/TEST_SHARD_INDEX (0-based); jest's --shard is 1-based.
SHARD=""
if [ "${{TEST_TOTAL_SHARDS:-0}}" -gt 1 ]; then
  [ -z "${{TEST_SHARD_STATUS_FILE:-}}" ] || touch "$TEST_SHARD_STATUS_FILE"
  SHARD="--shard=$((TEST_SHARD_INDEX + 1))/$TEST_TOTAL_SHARDS"
fi
bun x jest --selectProjects web-app --ci --no-coverage --maxWorkers=2 --reporters=summary $SHARD {ignore} --json --outputFile="$TEST_UNDECLARED_OUTPUTS_DIR/jest.json" {tests}
"""

def _bun_jest_test_impl(ctx):
    manifest = ctx.actions.declare_file(ctx.label.name + ".manifest")
    ctx.actions.write(manifest, "\n".join([f.short_path for f in ctx.files.srcs]) + "\n")
    repo_manifest = ctx.actions.declare_file(ctx.label.name + ".repo_manifest")
    ctx.actions.write(repo_manifest, "".join([f.short_path + "\n" for f in ctx.files.repo_data]))
    script = ctx.actions.declare_file(ctx.label.name + ".sh")
    ctx.actions.write(script, _JEST_SCRIPT.format(
        node = ctx.file.node.short_path,
        bun = ctx.file.bun.short_path,
        nm = ctx.file.node_modules.short_path,
        manifest = manifest.short_path,
        repo_manifest = repo_manifest.short_path,
        ignore = " ".join(["--testPathIgnorePatterns='%s'" % p for p in ctx.attr.ignore_patterns]),
        tests = " ".join(ctx.attr.tests),
        gencmd = ("mkdir -p \"$W/src/gen\"; cp -RL \"$(rp " + ctx.file.gen.short_path + ")/.\" \"$W/src/gen/\"; chmod -R u+w \"$W/src/gen\"\n") if ctx.file.gen else "",
    ), is_executable = True)
    rf = ctx.runfiles(files = ctx.files.srcs + ctx.files.repo_data + [manifest, repo_manifest, ctx.file.node, ctx.file.bun, ctx.file.node_modules] + ([ctx.file.gen] if ctx.file.gen else []))
    return [DefaultInfo(executable = script, runfiles = rf)]

bun_jest_test = rule(
    implementation = _bun_jest_test_impl,
    test = True,
    attrs = {
        "srcs": attr.label_list(allow_files = True),
        "gen": attr.label(allow_single_file = True),
        "repo_data": attr.label_list(allow_files = True, doc = "Repo files outside web-app/ staged at their repo-relative path."),
        "ignore_patterns": attr.string_list(doc = "jest --testPathIgnorePatterns regexes for suites that need files Bazel cannot supply."),
        "tests": attr.string_list(doc = "Test files to run; empty runs the whole web-app project."),
        "node_modules": attr.label(allow_single_file = True, mandatory = True),
        "bun": attr.label(allow_single_file = True, mandatory = True),
        "node": attr.label(allow_single_file = True, mandatory = True),
    },
)

def _tree_fixture_impl(ctx):
    out = ctx.actions.declare_directory(ctx.label.name)
    ctx.actions.run_shell(
        outputs = [out],
        command = "mkdir -p {o}/assets && printf '<html>fixture</html>' > {o}/index.html && printf 'console.log(1)' > {o}/assets/app.abc123.js".format(o = out.path),
    )
    return [DefaultInfo(files = depset([out]))]

# Spike S3 fixture: a tree artifact shaped like dist/ (a genrule cannot declare a directory output).
tree_fixture = rule(implementation = _tree_fixture_impl)
