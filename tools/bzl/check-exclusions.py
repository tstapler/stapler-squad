#!/usr/bin/env python3
"""Guard for tools/bzl/test-exclusions.yaml (Task 4.9a): schema check and shrink-only ratchet.

  check-exclusions.py [--file F] [--base-ref origin/main] [--max-packages N]

Exit 0 only when (1) every entry has a `//pkg:name` target, a known sandbox_class, an owner, an
issue URL and a reason, with no duplicate targets; (2) the entry count is not above the count in
the same file at --base-ref (skipped, with a note, when the base has no such file); (3) when
--max-packages is given, the number of distinct packages is at most N (Task 4.7b uses 10).
Removing entries is always allowed. The YAML subset used by the file is parsed with the stdlib.
"""
import argparse
import os
import re
import subprocess
import sys

DEFAULT_FILE = "tools/bzl/test-exclusions.yaml"
CLASSES = {"cwd", "home", "tmpdir-socket", "exec-host-binary", "network", "runtime-caller", "shard-timeout", "none"}
ISSUE_RE = re.compile(r"^https://github\.com/[\w.-]+/[\w.-]+/issues/\d+$")
TARGET_RE = re.compile(r"^//[\w./+-]*:[\w./+-]+$")


def parse(text):
    """Return a list of dicts, one per `exclusions:` list item (scalars and folded `>-` blocks)."""
    entries, cur, fold_key = [], None, None
    for raw in text.splitlines():
        if fold_key and (raw.startswith("      ") or not raw.strip()):
            cur[fold_key] = (cur[fold_key] + " " + raw.strip()).strip()
            continue
        fold_key = None
        line = raw.rstrip()
        if not line.strip() or line.lstrip().startswith("#") or line.strip() == "exclusions:":
            continue
        m = re.match(r"^  - (\w+):\s*(.*)$", line)
        if m:
            cur = {}
            entries.append(cur)
        else:
            m = re.match(r"^    (\w+):\s*(.*)$", line)
        if not m or cur is None:
            raise ValueError("unparseable line: %r" % raw)
        key, val = m.group(1), m.group(2)
        if val.startswith(">-"):
            cur[key], fold_key = "", key
        else:
            cur[key] = re.sub(r"\s+#.*$", "", val).strip()
    return entries


def validate(entries):
    problems, seen = [], set()
    for i, e in enumerate(entries):
        name = e.get("target") or "entry #%d" % (i + 1)
        if not TARGET_RE.match(e.get("target", "")):
            problems.append("%s: target must look like //pkg:name" % name)
        if e.get("target") in seen:
            problems.append("%s: duplicate target" % name)
        seen.add(e.get("target"))
        if e.get("sandbox_class") not in CLASSES:
            problems.append("%s: sandbox_class %r not in %s" % (name, e.get("sandbox_class"), sorted(CLASSES)))
        if not e.get("owner"):
            problems.append("%s: missing owner" % name)
        if not ISSUE_RE.match(e.get("issue", "")):
            problems.append("%s: issue must be a https://github.com/<org>/<repo>/issues/<n> URL" % name)
        if not e.get("reason"):
            problems.append("%s: missing reason" % name)
    return problems


def packages(entries):
    return {e["target"].split(":")[0] for e in entries if e.get("target")}


def base_count(path, ref):
    try:
        out = subprocess.run(["git", "show", "%s:%s" % (ref, path)], capture_output=True, text=True, check=True).stdout
    except (subprocess.CalledProcessError, FileNotFoundError):
        return None
    return len(parse(out))


def main(argv=None):
    ap = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    ap.add_argument("--file", default=DEFAULT_FILE)
    ap.add_argument("--base-ref", default="origin/main")
    ap.add_argument("--max-packages", type=int)
    args = ap.parse_args(argv)
    os.chdir(subprocess.run(["git", "rev-parse", "--show-toplevel"], capture_output=True, text=True).stdout.strip() or ".")
    with open(args.file) as f:
        entries = parse(f.read())
    problems = validate(entries)
    base = base_count(args.file, args.base_ref)
    if base is None:
        print("note: %s has no %s on %s; ratchet skipped" % (args.base_ref, args.file, args.base_ref), file=sys.stderr)
    elif len(entries) > base:
        problems.append("entry count %d is above %d on %s (the list may only shrink)" % (len(entries), base, args.base_ref))
    if args.max_packages is not None and len(packages(entries)) > args.max_packages:
        problems.append("%d packages excluded, limit is %d" % (len(packages(entries)), args.max_packages))
    for p in problems:
        print("FAIL: " + p, file=sys.stderr)
    print("exclusions: %d entries, %d packages" % (len(entries), len(packages(entries))))
    return 1 if problems else 0


if __name__ == "__main__":
    sys.exit(main())
