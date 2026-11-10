#!/usr/bin/env python3
"""Census integrity check (Task 0.9a verify). Exit 0 only if every failing test case found in the
test.xml files under bazel-testlogs for the census packages has exactly one row in census.json with
a SandboxClass, and the per-class sums equal the failure count.

  census-check.py                       integrity check, prints counts by class
  census-check.py --class home          also print remaining failures of that class
  census-check.py --package session    ... in that package
  census-check.py --files list.txt      ... in the files named in list.txt (one repo-relative path per line)
  census-check.py --residual            ... whose class is not one of the 4.10 sweep classes
"""
import argparse
import collections
import glob
import importlib.util
import json
import os
import sys
import xml.etree.ElementTree as ET

HERE = os.path.dirname(os.path.abspath(__file__))
SWEEP_CLASSES = {"cwd", "runtime-caller", "home", "tmpdir-socket", "exec-host-binary", "network"}
VALID_CLASSES = SWEEP_CLASSES | {"shard-timeout", "LOAD_FLAKE", "HOST_STATE_ESCAPE"}


def xml_files(testlogs, target):
    tdir = os.path.join(testlogs, target[2:].replace(":", "/"))
    top = os.path.join(tdir, "test.xml")
    if os.path.exists(top):
        return [top]
    return sorted(glob.glob(os.path.join(tdir, "shard_*_of_*", "test.xml")))


def failing_cases(testlogs, census):
    """Multiset of (package, test) failing in the XML, one entry per failing testcase element."""
    found = collections.Counter()
    for pkg, info in census.get("packages", {}).items():
        for target in info.get("targets", []):
            files = xml_files(testlogs, target)
            if not files:
                found[(pkg, target)] += 1  # target without any test.xml counts as one failure
                continue
            seen = 0
            for f in files:
                for tc in ET.parse(f).getroot().iter("testcase"):
                    seen += 1
                    if tc.find("failure") is not None or tc.find("error") is not None:
                        found[(pkg, tc.get("name"))] += 1
            if seen == 0 and info.get("status", {}).get(target) not in ("PASSED", "NO STATUS", ""):
                found[(pkg, target)] += 1
    return found


def check(census, testlogs):
    """Return (problems, counts_by_class). problems is empty iff the census is complete and consistent."""
    problems = []
    rows = census["rows"]
    by_key = collections.defaultdict(list)
    for r in rows:
        by_key[(r["package"], r["test"])].append(r)
    xml_cases = failing_cases(testlogs, census)
    for key, n in xml_cases.items():
        got = len(by_key.get(key, []))
        if got != n:
            problems.append("%s: %d failing in test.xml, %d census rows" % (key, n, got))
    for key in by_key:
        if key not in xml_cases:
            problems.append("%s: census row without a failing test.xml case" % (key,))
    for r in rows:
        if r.get("class") not in VALID_CLASSES:
            problems.append("%s/%s: class %r is not a SandboxClass" % (r["package"], r["test"], r.get("class")))
    counts = collections.Counter(r["class"] for r in rows)
    failures = sum(xml_cases.values())
    if sum(counts.values()) != failures:
        problems.append("class sum %d != failure count %d" % (sum(counts.values()), failures))
    return problems, counts


def select(rows, cls=None, package=None, files=None, residual=False):
    out = []
    for r in rows:
        if cls and r["class"] != cls:
            continue
        if package and r["package"] != package:
            continue
        if files is not None and r["file"] not in files:
            continue
        if residual and r["class"] in SWEEP_CLASSES:
            continue
        out.append(r)
    return out


def default_testlogs():
    link = os.path.join(os.path.abspath(os.path.join(HERE, "..", "..", "..")), "bazel-testlogs")
    return os.path.realpath(link)


def main(argv=None):
    ap = argparse.ArgumentParser()
    ap.add_argument("--census", default=os.path.join(HERE, "census.json"))
    ap.add_argument("--testlogs", default=default_testlogs())
    ap.add_argument("--class", dest="cls")
    ap.add_argument("--package")
    ap.add_argument("--files", help="file holding one repo-relative path per line")
    ap.add_argument("--residual", action="store_true")
    a = ap.parse_args(argv)
    census = json.load(open(a.census))
    problems, counts = check(census, a.testlogs)
    files = None
    if a.files:
        files = {l.strip() for l in open(a.files) if l.strip()}
    sel = select(census["rows"], a.cls, a.package, files, a.residual)
    print(json.dumps({"failures": sum(counts.values()), "by_class": dict(counts), "problems": len(problems)}))
    if a.cls or a.package or a.files or a.residual:
        print("remaining: %d" % len(sel))
    for p in problems:
        print("PROBLEM:", p, file=sys.stderr)
    return 1 if problems else 0


if __name__ == "__main__":
    sys.exit(main())
