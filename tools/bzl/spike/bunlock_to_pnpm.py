#!/usr/bin/env python3
"""Spike 0.6d: prototype of the bun.lock -> rules_js internal model reader (pnpm lockfile v9 shape).

Usage: bunlock_to_pnpm.py web-app/bun.lock [N]   (N = restrict to the N first direct deps' closure)
Emits {importers, packages, snapshots} JSON on stdout and a summary on stderr.
"""
import json, re, sys

raw = open(sys.argv[1]).read()
lock = json.loads(re.sub(r",(\s*[}\]])", r"\1", raw))  # bun.lock is JSONC (trailing commas)
pk = lock["packages"]
root = lock["workspaces"][""]
limit = int(sys.argv[2]) if len(sys.argv) > 2 else None

def resolve(parent, dep):
    """Nested key 'parent/dep' wins, then climb to the hoisted key 'dep' (bun.lock install layout)."""
    parts = parent.split("/")
    while parts:
        k = "/".join(parts + [dep])
        if k in pk:
            return k
        parts.pop()
    return dep if dep in pk else None

def ident(entry):  # "name@version"
    return entry[0]

packages, snapshots, unresolved = {}, {}, []
todo = []
direct = {}
for sect in ("dependencies", "devDependencies", "optionalDependencies"):
    for name, spec in root.get(sect, {}).items():
        if limit is not None and len(direct) >= limit:
            break
        direct[name] = (sect, spec)
        todo.append(name)
seen = set()
while todo:
    key = todo.pop()
    if key in seen:
        continue
    seen.add(key)
    e = pk[key]
    id_ = ident(e)
    meta = e[2] if len(e) > 2 and isinstance(e[2], dict) else {}
    integrity = e[3] if len(e) > 3 else None
    packages[id_] = {"resolution": {"integrity": integrity}, "hasBin": bool(meta.get("bin")),
                     **({"os": [meta["os"]] if isinstance(meta.get("os"), str) else meta["os"]} if "os" in meta else {}),
                     **({"cpu": [meta["cpu"]] if isinstance(meta.get("cpu"), str) else meta["cpu"]} if "cpu" in meta else {})}
    snap = {}
    for sect, out in (("dependencies", "dependencies"), ("optionalDependencies", "optionalDependencies"), ("peerDependencies", "dependencies")):
        for dn in meta.get(sect, {}):
            target = resolve(key, dn)
            if target is None:
                if sect != "peerDependencies":
                    unresolved.append((id_, dn))
                continue
            snap.setdefault(out, {})[dn] = ident(pk[target]).split("@", 1 if dn.startswith("@") is False else 2)[-1] if False else ident(pk[target])[len(dn) + 1:]
            todo.append(target)
    snapshots[id_] = snap
importers = {".": {sect: {n: {"specifier": s, "version": ident(pk[n])[len(n) + 1:]} for n, (sc, s) in direct.items() if sc == sect}
                   for sect in ("dependencies", "devDependencies", "optionalDependencies")}}
json.dump({"lockfileVersion": "9.0", "importers": importers, "packages": packages, "snapshots": snapshots}, sys.stdout)
print(f"direct={len(direct)} packages={len(packages)} edges={sum(len(v.get('dependencies', {})) + len(v.get('optionalDependencies', {})) for v in snapshots.values())} unresolved={len(unresolved)} {unresolved[:3]}", file=sys.stderr)
