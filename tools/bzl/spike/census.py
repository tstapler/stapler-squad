#!/usr/bin/env python3
"""Sandbox census producer (Task 0.9a): run Go packages' tests under Bazel, parse every test.xml
and write census.json rows {test, package, file, class} (+ evidence) for each failing test case.

Isolation is mandatory: every run gets a private HOME, TMUX_TMPDIR and tmux socket under
CENSUS_ROOT (default ~/.cache/stapler-squad-census), and the host default tmux socket is
fingerprinted before/after each package (a change aborts the run as HOST_STATE_ESCAPE).

  census.py --run session/tmux config            run + parse + write census.json / census-packages.json
  census.py --parse-only                         re-parse existing bazel-testlogs (no bazel)
  census.py --table                              print counts by class and by package
"""
import argparse
import glob
import json
import os
import random
import re
import subprocess
import sys
import xml.etree.ElementTree as ET

HERE = os.path.dirname(os.path.abspath(__file__))
REPO = os.path.abspath(os.path.join(HERE, "..", "..", ".."))
CENSUS_ROOT = os.path.expanduser(os.environ.get("CENSUS_ROOT", "~/.cache/stapler-squad-census"))

# SandboxClass (plan Epic 4 / glossary) plus the two census-only classes.
SANDBOX_CLASSES = ["cwd", "home", "tmpdir-socket", "exec-host-binary", "network",
                   "runtime-caller", "shard-timeout"]
EXTRA_CLASSES = ["LOAD_FLAKE", "HOST_STATE_ESCAPE"]
ALL_CLASSES = SANDBOX_CLASSES + EXTRA_CLASSES + ["other"]

# First match wins; evidence is the matching regex. Order matters: specific before generic.
RULES = [
    ("cwd", r"scripts/ssq-[a-z-]+: (N|n)o such file|proof test not found|not found in server/services|no \*_test\.go files found|only 0 bytes of distinct source|open [^ ]*/execroot/[^ ]*: no such file"),
    ("exec-host-binary", r"gh\) is not installed|env: 'tmux': No such file|expected: 0xd\s+// actual\s*: 0xe|expected: 0xd\s+actual\s*: 0xe"),
    ("shard-timeout", r"No pass/skip/fail event found for test"),
    ("home", r"[Aa]uthor identity unknown|no git author identity|to start with [^ ]*stapler-squad-census/home"),
    ("cwd", r"open [^ ]+\.go: no such file or directory|go\.mod not found above"),
    ("runtime-caller", r"runtime\.Caller|GOFLAGS|go\.mod: no such file|cannot find (repo|module|project) root|could not (find|locate) (the )?(repo|repository|project|module) root"),
    ("tmpdir-socket", r"socket path|too long|SUN_LEN|bind: invalid argument|listen unix .*invalid argument"),
    ("shard-timeout", r"test timed out|panic: test timed out|Test killed with signal|TIMEOUT"),
    ("exec-host-binary", r"exec: \"[^\"]+\": executable file not found|executable file not found in \$PATH|exec: \"(tmux|git|gh|bun|node|go)\""),
    ("network", r"dial tcp [^ ]+: (i/o timeout|connect: network is unreachable)|no such host|lookup [^ ]+ on .*:53|Temporary failure in name resolution"),
    ("home", r"read-only file system|\$HOME|os\.UserHomeDir|user\.Current|\.stapler-squad.*(permission denied|no such file)|permission denied.*/home/"),
    ("cwd", r"open [^ ]*(testdata|\.\./|\./)[^ ]*: no such file|no such file or directory.*(testdata|\.\./)|chdir .*: no such file"),
]


def run(cmd, **kw):
    return subprocess.run(cmd, cwd=REPO, text=True, capture_output=True, **kw)


def classify(text):
    for cls, rx in RULES:
        m = re.search(rx, text or "")
        if m:
            return cls, "regex:" + cls + ":" + m.group(0)[:80]
    return "other", "no rule matched"


def load_overrides():
    p = os.path.join(HERE, "census-overrides.json")
    return json.load(open(p)) if os.path.exists(p) else {}


def find_test_file(pkg, test):
    top = test.split("/")[0]
    rx = re.compile(r"^func\s+(\([^)]*\)\s*)?" + re.escape(top) + r"\s*\(", re.M)
    for f in sorted(glob.glob(os.path.join(REPO, pkg, "*_test.go"))):
        try:
            if rx.search(open(f, encoding="utf-8", errors="replace").read()):
                return os.path.relpath(f, REPO)
        except OSError:
            pass
    return ""


def testlogs_dir():
    r = run(["tools/bazel", "info", "bazel-testlogs"], env={**os.environ, "BAZEL_REAL": os.environ.get("BAZEL_REAL", "")})
    return r.stdout.strip()


def targets_for(pkg):
    r = run(["tools/bazel", "query", "kind(go_test, //%s:all)" % pkg])
    return [t for t in r.stdout.split() if t.startswith("//")]


def parse_target(testlogs, target, pkg, status):
    """Return (counts, rows) for one go_test target from its merged test.xml."""
    tdir = os.path.join(testlogs, target[2:].replace(":", "/"))
    counts = {"passed": 0, "failed": 0, "skipped": 0, "status": status}
    rows = []
    xmls = [os.path.join(tdir, "test.xml")]
    if not os.path.exists(xmls[0]):  # sharded targets that failed keep only per-shard files
        xmls = sorted(glob.glob(os.path.join(tdir, "shard_*_of_*", "test.xml")))
    if not xmls:
        rows.append({"test": target, "package": pkg, "file": "", "text": "no test.xml (status %s)" % status})
        counts["failed"] += 1
        return counts, rows
    n = 0
    for xmlp in xmls:
        try:
            root = ET.parse(xmlp).getroot()
        except ET.ParseError as e:
            rows.append({"test": target, "package": pkg, "file": "", "text": "unparsable %s: %s" % (xmlp, e)})
            counts["failed"] += 1
            continue
        for tc in root.iter("testcase"):
            n += 1
            fail = tc.find("failure")
            err = tc.find("error")
            if fail is not None or err is not None:
                el = fail if fail is not None else err
                counts["failed"] += 1
                rows.append({"test": tc.get("name"), "package": pkg, "file": find_test_file(pkg, tc.get("name", "")),
                             "text": (el.get("message") or "") + "\n" + (el.text or "")})
            elif tc.find("skipped") is not None:
                counts["skipped"] += 1
            else:
                counts["passed"] += 1
    if n == 0 and status not in ("PASSED", "NO STATUS", ""):
        rows.append({"test": target, "package": pkg, "file": "", "text": "no testcases in test.xml (status %s)" % status})
        counts["failed"] += 1
    return counts, rows


def finish_rows(rows, overrides):
    out = []
    for r in rows:
        key = r["package"] + ":" + r["test"]
        if key in overrides:
            cls, ev = overrides[key]["class"], "override:" + overrides[key].get("reason", "")
        else:
            cls, ev = classify(r["text"])
        out.append({"test": r["test"], "package": r["package"], "file": r["file"], "class": cls, "evidence": ev})
    return inherit_from_subtests(out)


def inherit_from_subtests(rows):
    """A parent test that only reports '--- FAIL' for failing subtests takes its first classified subtest's class."""
    for r in rows:
        if r["class"] != "other":
            continue
        for c in rows:
            if c["package"] == r["package"] and c["test"].startswith(r["test"] + "/") and c["class"] != "other":
                r["class"], r["evidence"] = c["class"], "parent-of:" + c["test"]
                break
    return rows


def default_socket_fp():
    d = "/tmp/tmux-%d/default" % os.getuid()
    try:
        st = os.stat(d)
        return (st.st_ino, st.st_mtime_ns, st.st_ctime_ns)
    except OSError:
        return None


def load1():
    return open("/proc/loadavg").read().split()[0]


def avail_gb():
    for line in open("/proc/meminfo"):
        if line.startswith("MemAvailable"):
            return int(line.split()[1]) / 1048576.0
    return 0.0


def wait_for_memory(minimum=8.0):
    import time
    for _ in range(20):
        if avail_gb() >= minimum:
            return True
        time.sleep(30)
    return False


def bazel_test(targets, sockid, extra, socket_env=True):
    env_flags = [
        "--test_env=HOME=%s/home" % CENSUS_ROOT,
        "--test_env=TMUX_TMPDIR=%s/tmux" % CENSUS_ROOT,
        "--test_env=STAPLER_SQUAD_TMUX_SOCKET=ssq-census-%s" % sockid,
        "--test_env=STAPLER_SQUAD_INSTANCE=census",
        "--test_env=GO_TEST_WRAP_TESTV=1",
        "--sandbox_writable_path=%s" % CENSUS_ROOT,
    ]
    if not socket_env:  # repair run: tests that assert their own socket name; TMUX_TMPDIR stays private
        env_flags = [f for f in env_flags if "STAPLER_SQUAD_TMUX_SOCKET" not in f]
    cmd = ["tools/bazel", "test", "--jobs=4", "--local_resources=memory=3000", "--local_test_jobs=1",
           "--sandbox_debug", "--test_output=errors", "--keep_going", "--test_timeout=300",
           "--sandbox_default_allow_network=false", "--nocache_test_results"] + env_flags + extra + targets
    r = run(cmd)
    status = {}
    for line in (r.stdout + r.stderr).splitlines():
        m = re.match(r"^(//\S+)\s+(PASSED|FAILED|TIMEOUT|FLAKY|NO STATUS|SKIPPED|FAILED TO BUILD|\(cached\) PASSED)\b", line)
        if m:
            status[m.group(1)] = m.group(2)
    return r, status


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("--run", nargs="*", metavar="PKG", help="go package dirs (repo-relative) to test and parse")
    ap.add_argument("--parse-only", action="store_true")
    ap.add_argument("--packages", nargs="*", help="with --parse-only: packages to parse")
    ap.add_argument("--no-socket-env", action="store_true", help="repair run: omit STAPLER_SQUAD_TMUX_SOCKET")
    ap.add_argument("--table", action="store_true")
    ap.add_argument("--out", default=os.path.join(HERE, "census.json"))
    ap.add_argument("--extra", nargs=argparse.REMAINDER, default=[], help="extra bazel test flags")
    a = ap.parse_args()

    if a.table:
        rows = json.load(open(a.out))["rows"]
        by_c, by_p = {}, {}
        for r in rows:
            by_c[r["class"]] = by_c.get(r["class"], 0) + 1
            by_p.setdefault(r["package"], {})
            by_p[r["package"]][r["class"]] = by_p[r["package"]].get(r["class"], 0) + 1
        print(json.dumps({"by_class": by_c, "by_package": by_p, "total": len(rows)}, indent=1))
        return 0

    pkgs = a.run if a.run is not None else (a.packages or [])
    os.makedirs(CENSUS_ROOT + "/home", exist_ok=True)
    os.makedirs(CENSUS_ROOT + "/tmux", exist_ok=True)
    sockid = "%08x" % random.getrandbits(32)
    overrides = load_overrides()
    tl = testlogs_dir()
    existing = {"rows": [], "packages": {}}
    if os.path.exists(a.out):
        existing = json.load(open(a.out))
    all_rows = [r for r in existing["rows"] if r["package"] not in pkgs]
    pkg_info = {k: v for k, v in existing.get("packages", {}).items() if k not in pkgs}
    for pkg in pkgs:
        targets = targets_for(pkg)
        info = {"targets": targets, "passed": 0, "failed": 0, "skipped": 0}
        if not targets:
            info["note"] = "no go_test targets"
            pkg_info[pkg] = info
            continue
        before = default_socket_fp()
        info["load_start"] = load1()
        status = {}
        if not a.parse_only:
            if not wait_for_memory():
                print("census: <8 GB available for 10 minutes; stopping before %s" % pkg, file=sys.stderr)
                return 3
            r, status = bazel_test(targets, sockid, a.extra, not a.no_socket_env)
            info["bazel_exit"] = r.returncode
        info["load_end"] = load1()
        if default_socket_fp() != before:
            info["escape"] = "default tmux socket fingerprint changed"
            print("census: HOST_STATE_ESCAPE in %s; aborting" % pkg, file=sys.stderr)
        rows = []
        for t in targets:
            c, rr = parse_target(tl, t, pkg, status.get(t, ""))
            for k in ("passed", "failed", "skipped"):
                info[k] += c[k]
            info.setdefault("status", {})[t] = c["status"]
            rows += rr
        all_rows += finish_rows(rows, overrides)
        pkg_info[pkg] = info
        json.dump({"rows": all_rows, "packages": pkg_info}, open(a.out, "w"), indent=1)
        if "escape" in info:
            return 4
    json.dump({"rows": all_rows, "packages": pkg_info}, open(a.out, "w"), indent=1)
    print("wrote %d rows to %s" % (len(all_rows), a.out))
    return 0


if __name__ == "__main__":
    sys.exit(main())
