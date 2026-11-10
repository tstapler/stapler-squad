#!/usr/bin/env python3
"""Count file accesses of a strace log (strace -f -y) that escape the web-app work tree.

Usage: count_escapes.py LOG [--list]
Prints "<reads> <writes>": distinct successful reads outside the work tree that are not the pinned
toolchain or the system loader/proc/dev paths, and distinct successful writes outside the work tree
and the action's scratch HOME. Work tree = the first cwd seen (bun is started inside it) (execroot/_work).
"""
import os, re, sys

log = sys.argv[1]
show = "--list" in sys.argv
LINE = re.compile(r"^(\d+)\s+(\w+)\((.*)\)\s+=\s+(-?\d+|\?)(.*)$")
STR = re.compile(r'"((?:[^"\\]|\\.)*)"')
CWD = re.compile(r"AT_FDCWD<([^>]*)>")
DIRFD = re.compile(r"^(\d+)<([^>]*)>")
WRITE_FLAGS = ("O_WRONLY", "O_RDWR", "O_CREAT", "O_TRUNC")
SYSTEM = ("/usr/", "/lib/", "/lib64/", "/etc/ld.so", "/proc/", "/sys/", "/dev/", "/etc/localtime",
          "/etc/nsswitch.conf", "/etc/resolv.conf", "/etc/hosts", "/etc/passwd", "/etc/group",
          "/etc/ssl/", "/etc/pki/", "/etc/ca-certificates", "/etc/os-release", "/bin/", "/sbin/", "/etc/gai.conf",
          "/etc/host.conf", "/etc/hostname", "/etc/mime.types", "/etc/protocols", "/etc/services", "/etc/locale")
reads, writes, probes = set(), set(), set()
work = None
root = None
cwd_of = {}
with open(log, errors="replace") as f:
    for line in f:
        m = LINE.match(line)
        if not m:
            continue
        pid, sc, args, ret, _ = m.groups()
        ok = not ret.startswith("-")
        c = CWD.search(args)
        if c:
            cwd_of[pid] = c.group(1)
            if work is None:
                work = c.group(1)
        paths = []
        base = cwd_of.get(pid, work or "/")
        d = DIRFD.match(args)
        if d and sc.endswith("at") and not c:
            base = d.group(2)
        strs = STR.findall(args)
        if not strs:
            continue
        if sc in ("rename", "renameat", "renameat2", "link", "linkat"):
            paths = [("w", strs[0]), ("w", strs[1])] if len(strs) > 1 else [("w", strs[0])]
        elif sc in ("symlink", "symlinkat"):
            paths = [("w", strs[-1])]
        elif sc in ("mkdir", "mkdirat", "unlink", "unlinkat", "rmdir", "truncate", "chmod", "utimensat"):
            paths = [("w", strs[0])]
        elif sc in ("openat", "open"):
            wr = any(fl in args for fl in WRITE_FLAGS)
            if "O_DIRECTORY" in args:  # directory walks (realpath/readdir), not file reads
                continue
            paths = [("w" if wr else "r", strs[0])]
        elif sc in ("execve", "readlink", "readlinkat"):
            paths = [("r", strs[0])]
        elif sc in ("stat", "newfstatat", "lstat", "access"):
            paths = [("p", strs[0])]
        for kind, p in paths:
            if not p.startswith("/"):
                p = os.path.normpath(os.path.join(base, p))
            if not ok:
                if kind == "p" or kind == "r":
                    probes.add(p)
                continue
            if work is None:
                continue
            if kind == "r":
                reads.add(p)
            elif kind == "w":
                writes.add(p)
root = os.path.dirname(work) if work else ""
tool = [os.path.join(root, "external", n) + "/" for n in os.listdir(os.path.join(root, "external"))] if False else []
def inside(p): return p == work or p.startswith(work + "/")
def toolchain(p): return p.startswith(root + "/external/+http_archive+")
def ancestor(p): return work.startswith(p.rstrip("/") + "/")  # directory walks up from the cwd
def system(p): return any(p.startswith(s) for s in SYSTEM) or p.startswith(root + "/_home")
esc_r = sorted(p for p in reads if not inside(p) and not toolchain(p) and not system(p) and not ancestor(p))
esc_w = sorted(p for p in writes if not inside(p) and not p.startswith(root + "/_home"))
if show:
    print("work tree:", work)
    print("reads total:", len(reads), "| inside:", sum(inside(p) for p in reads), "| toolchain:", sum(toolchain(p) for p in reads), "| system:", sum(system(p) for p in reads))
    print("writes total:", len(writes), "| inside work tree:", sum(inside(p) for p in writes), "| scratch HOME:", sum(p.startswith(root + "/_home") for p in writes))
    for p in esc_r: print("READ-ESCAPE", p)
    for p in esc_w: print("WRITE-ESCAPE", p)
    out_probes = sorted(p for p in probes if not inside(p) and not system(p) and not toolchain(p))
    print("failed probes outside tree (not counted):", len(out_probes), out_probes[:8])
print(len(esc_r), len(esc_w))
