"""forbidden_deps_test: fails analysis when a Go target's dep graph breaks an architecture guard (the Bazel twin of the `go list -deps` tests)."""

_GraphInfo = provider("Transitive (label, direct dep labels) edges.", fields = ["edges"])

def _graph_impl(target, ctx):
    kids = [d for a in ("deps", "embed") for d in getattr(ctx.rule.attr, a, []) if _GraphInfo in d]
    edge = (target.label, tuple([k.label for k in kids]))
    return [_GraphInfo(edges = depset([edge], transitive = [k[_GraphInfo].edges for k in kids]))]

_graph = aspect(implementation = _graph_impl, attr_aspects = ["deps", "embed"])

def _s(lab):
    return ("@" + lab.workspace_name if lab.workspace_name else "") + "//%s:%s" % (lab.package, lab.name)

def _match(label, pat):  # "@repo//pkg/...", "//pkg/...", "//pkg" or "//pkg:name"
    repo, _, rest = pat.partition("//")
    ws = label.workspace_name
    if (repo and not (ws == repo[1:] or ws.endswith("+" + repo[1:]))) or (not repo and ws):
        return False
    if rest.endswith("..."):
        pkg = rest[:-3].rstrip("/")
        return pkg == "" or label.package == pkg or label.package.startswith(pkg + "/")
    pkg, _, name = rest.partition(":")
    return label.package == pkg and (not name or label.name == name)

def _impl(ctx):
    kids = {lab: c for lab, c in ctx.attr.target[_GraphInfo].edges.to_list()}
    root = ctx.attr.target.label
    parent, queue = {root: None}, [root]
    for _ in range(len(kids) + 1):  # BFS: shortest chain to each dep
        nxt = []
        for n in queue:
            for c in kids.get(n, ()):
                if c not in parent and (not ctx.attr.direct_only or n == root):
                    parent[c] = n
                    nxt.append(c)
        queue = nxt
    for pat in ctx.attr.forbidden:
        for dep in parent:
            if dep != root and _match(dep, pat):
                chain, n = [], dep
                for _ in range(len(parent)):
                    chain.append(_s(n))
                    n = parent[n]
                    if n == None:
                        break
                fail("%s: %s\n  forbidden dep %s (pattern %s) reached via: %s" % (_s(ctx.label), ctx.attr.why, _s(dep), pat, " -> ".join(reversed(chain))))
    for pat in ctx.attr.required:
        if not [d for d in parent if d != root and _match(d, pat)]:
            fail("%s: %s\n  required dep %s is missing from the graph of %s (sanity check: the guard would pass vacuously)" % (_s(ctx.label), ctx.attr.why, pat, _s(root)))
    script = ctx.actions.declare_file(ctx.label.name + ".sh")
    ctx.actions.write(script, "#!/bin/sh\nexit 0\n", is_executable = True)
    return [DefaultInfo(executable = script)]

forbidden_deps_test = rule(
    implementation = _impl,
    test = True,
    attrs = {
        "target": attr.label(mandatory = True, aspects = [_graph]),
        "forbidden": attr.string_list(mandatory = True),
        "required": attr.string_list(),
        "direct_only": attr.bool(),
        "why": attr.string(),
    },
)
