#!/usr/bin/env python3
"""Group a Go goroutine dump (debug=2) by state and by top app frame.

Usage (run with `python3 -I`):
  goroutine-states.py [FILE|URL] [--top N] [--module PATH] [--find FUNC]

Default source is http://localhost:6060/debug/pprof/goroutine?debug=2.
--find FUNC lists every goroutine whose stack contains FUNC, so in a lock
pile-up the one NOT in `semacquire`/`sync.Mutex.Lock` is the likely holder.
"""
import argparse
import collections
import re
import sys
import urllib.request

HEADER = re.compile(r"^goroutine (\d+) \[([^\]]*)\]:$")
DEFAULT_URL = "http://localhost:6060/debug/pprof/goroutine?debug=2"


def load(src):
    if src.startswith("http"):
        return urllib.request.urlopen(src, timeout=30).read().decode("utf-8", "replace")
    with open(src, encoding="utf-8", errors="replace") as f:
        return f.read()


def parse(text):
    """Yield (id, state, wait_minutes, frames[]) per goroutine."""
    for block in text.split("\n\n"):
        lines = block.strip().split("\n")
        m = HEADER.match(lines[0]) if lines else None
        if not m:
            continue
        parts = [p.strip() for p in m.group(2).split(",")]
        state, minutes = parts[0], 0
        for p in parts[1:]:
            mm = re.match(r"(\d+) minutes?", p)
            if mm:
                minutes = int(mm.group(1))
        frames = []
        for ln in lines[1:]:
            if ln.startswith("\t") or ln.startswith("created by"):
                continue
            frames.append(re.sub(r"\([^()]*\)$", "", ln.strip()))
        yield int(m.group(1)), state, minutes, frames


SYNC_STATES = ("semacquire", "sync.", "chan send", "chan receive (nil", "select (no cases)")


def is_sync_wait(state):
    return state.startswith(SYNC_STATES)


def top_app_frame(frames, module):
    for f in frames:
        if module in f:
            return f.replace(module + "/", "")
    return frames[0] if frames else "?"


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("src", nargs="?", default=DEFAULT_URL)
    ap.add_argument("--top", type=int, default=15)
    ap.add_argument("--module", default="github.com/tstapler/stapler-squad")
    ap.add_argument("--find", default="")
    a = ap.parse_args()

    gs = list(parse(load(a.src)))
    by_state = collections.Counter(s for _, s, _, _ in gs)
    by_group = collections.Counter((s, top_app_frame(fr, a.module)) for _, s, _, fr in gs)
    print(f"total goroutines: {len(gs)}\n\nBY STATE")
    for s, n in by_state.most_common():
        print(f"  {n:6d}  {s}")
    print(f"\nTOP {a.top} (state, top app frame)")
    for (s, f), n in by_group.most_common(a.top):
        print(f"  {n:6d}  [{s}] {f}")
    # Idle parks (select, chan receive, IO wait) are normal; lock/WaitGroup/chan-send waits are not.
    waited = sorted((g for g in gs if g[2] >= 1 and is_sync_wait(g[1])), key=lambda g: -g[2])
    print(f"\nLOCK/WAITGROUP/CHAN-SEND WAITS >= 1 MINUTE: {len(waited)}")
    for gid, s, m, fr in waited[: a.top]:
        print(f"  {m:5d} min  g{gid} [{s}] {top_app_frame(fr, a.module)}")
    if a.find:
        print(f"\nSTACKS CONTAINING {a.find!r} (holder = the one not blocked on a lock)")
        for gid, s, m, fr in gs:
            if any(a.find in f for f in fr):
                print(f"  g{gid} [{s}, {m}m] " + " <- ".join(f.split("/")[-1] for f in fr[:6]))


if __name__ == "__main__":
    sys.exit(main())
