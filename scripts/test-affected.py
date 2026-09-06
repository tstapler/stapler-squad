#!/usr/bin/env python3
"""Print the set of Go packages whose tests are affected by changes vs a base ref.

"Affected" means: the package itself changed, or it transitively imports (in
its normal build OR its test build) a package that changed. Uses `go list
-json -test ./...` as the single source of truth for the dependency graph --
no separate go/packages dependency needed.

This only sees changed *.go files. Non-.go sources that generate .go code
many packages import -- .proto files above all, since the generated code
they produce is gitignored and not committed (see CLAUDE.md) -- are
invisible to a plain .go-file diff. FULL_RESCAN_TRIGGERS below forces the
safe fallback (print __ALL__, meaning "run everything") whenever one of
those changed, rather than silently under-selecting packages that actually
need re-testing.

Usage: test-affected.py [BASE_REF]   (default: origin/main)
Prints one import path per line to stdout, or the single line __ALL__ if a
FULL_RESCAN_TRIGGERS path changed (caller should treat that as "run the
full suite, this script can't safely narrow it"). Exits 0 with no output if
nothing changed or nothing is affected.
"""
import fnmatch
import json
import re
import subprocess
import sys

# Changing any of these means the .go-import-graph analysis above can't be
# trusted to have seen every affected package -- fall back to a full run.
# Globs are matched against repo-root-relative paths via fnmatch (supports
# '*' crossing '/', same as this project's other pathspec-style globs).
FULL_RESCAN_TRIGGERS = [
    "proto/*",
    "go.mod",
    "go.sum",
    "Makefile",
    ".golangci.yml",
    "scripts/test-affected.py",  # don't trust a change to this script's own logic
]


def sh(cmd):
    return subprocess.run(cmd, capture_output=True, text=True, check=True).stdout


def matches_full_rescan_trigger(path):
    return any(fnmatch.fnmatch(path, pattern) for pattern in FULL_RESCAN_TRIGGERS)


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
    # tree, not just what's already committed. Gathered across ALL files (not
    # just *.go) so FULL_RESCAN_TRIGGERS can see non-.go changes too.
    all_changed = set()
    try:
        all_changed.update(sh(["git", "diff", "--name-only", f"{base}...HEAD"]).splitlines())
        all_changed.update(sh(["git", "diff", "--name-only", "HEAD"]).splitlines())
        all_changed.update(sh(["git", "ls-files", "--others", "--exclude-standard"]).splitlines())
    except subprocess.CalledProcessError:
        # BASE_REF doesn't exist locally (e.g. origin/main not fetched) --
        # can't safely narrow the package set, so fall back to running
        # everything, same as FULL_RESCAN_TRIGGERS below.
        print("__ALL__")
        return
    all_changed.discard("")

    if any(matches_full_rescan_trigger(f) for f in all_changed):
        print("__ALL__")
        return

    changed_files = {f for f in all_changed if f.endswith(".go")}
    if not changed_files:
        return

    changed_dirs = sorted({f.rsplit("/", 1)[0] if "/" in f else "." for f in changed_files if f})
    changed_pkgs = set()
    for d in changed_dirs:
        try:
            changed_pkgs.update(sh(["go", "list", f"./{d}"]).split())
        except subprocess.CalledProcessError:
            # A changed file's directory can be gone entirely (e.g. a delete
            # removed the last file in it) -- go list fails on a nonexistent
            # dir; skip it rather than crashing the whole run over one path.
            continue
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
