#!/usr/bin/env python3
"""Print the set of Go packages whose tests are affected by changes vs a base ref.

"Affected" means: the package itself changed, or it transitively imports (in
its normal build OR its test build) a package that changed. Uses `go list
-json -test ./...` as the single source of truth for the dependency graph --
no separate go/packages dependency needed.

Usage: test-affected.py [BASE_REF]   (default: origin/main)
Prints one import path per line to stdout. Exits 0 with no output if nothing
changed or nothing is affected.
"""
import json
import re
import subprocess
import sys


def sh(cmd):
    return subprocess.run(cmd, capture_output=True, text=True, check=True).stdout


def parse_json_stream(raw):
    dec = json.JSONDecoder()
    idx = 0
    objs = []
    while idx < len(raw):
        chunk = raw[idx:].lstrip()
        if not chunk:
            break
        idx = len(raw) - len(chunk)
        obj, end = dec.raw_decode(raw, idx)
        objs.append(obj)
        idx = end
    return objs


def main():
    base = sys.argv[1] if len(sys.argv) > 1 else "origin/main"

    # Union of: committed changes since the merge-base with BASE, uncommitted
    # tracked edits, and new untracked files -- so this reflects the working
    # tree, not just what's already committed.
    changed_files = set()
    changed_files.update(sh(["git", "diff", "--name-only", f"{base}...HEAD", "--", "*.go"]).splitlines())
    changed_files.update(sh(["git", "diff", "--name-only", "HEAD", "--", "*.go"]).splitlines())
    changed_files.update(sh(["git", "ls-files", "--others", "--exclude-standard", "--", "*.go"]).splitlines())
    changed_files.discard("")
    if not changed_files:
        return

    changed_dirs = sorted({f.rsplit("/", 1)[0] if "/" in f else "." for f in changed_files if f})
    dir_args = [f"./{d}" for d in changed_dirs]
    changed_pkgs = set(sh(["go", "list"] + dir_args).split())
    if not changed_pkgs:
        return

    raw = sh(["go", "list", "-json", "-test", "./..."])
    objs = parse_json_stream(raw)

    deps_by_pkg = {}
    has_tests = set()
    variant_re = re.compile(r"^(.*) \[(.*)\.test\]$")
    for o in objs:
        ip = o["ImportPath"]
        m = variant_re.match(ip)
        if m:
            real = m.group(1)
        elif ip.endswith(".test"):
            continue  # synthetic test-binary main package, not a real target
        else:
            real = ip
        deps_by_pkg.setdefault(real, set()).update(o.get("Deps", []))
        if o.get("TestGoFiles") or o.get("XTestGoFiles"):
            has_tests.add(real)

    affected = set()
    for pkg, deps in deps_by_pkg.items():
        if pkg not in has_tests:
            continue
        if pkg in changed_pkgs or (deps & changed_pkgs):
            affected.add(pkg)

    for pkg in sorted(affected):
        print(pkg)


if __name__ == "__main__":
    main()
