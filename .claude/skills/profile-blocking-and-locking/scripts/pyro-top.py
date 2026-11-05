#!/usr/bin/env python3
"""Top frames from a Pyroscope profile over a time window (no UI, no auth).

Usage (run with `python3 -I`):
  pyro-top.py TYPE [--minutes 60] [--top 15] [--match SUBSTR] [--service stapler-squad]
TYPE is a short alias or a full profile-type id:
  cpu | mutex | mutex-count | block | block-count | goroutine | alloc
Prints SELF value per function (with --match, a leaf's value is charged to its nearest matching caller) (ns for cpu/mutex/block delay) with percent of total.

Caveat: Alloy scrapes the CUMULATIVE /debug/pprof/{mutex,block} unless the godeltaprof
scrape types are enabled (docs/observability/README.md), so mutex/block values are
inflated by the scrape count. Trust ranking shape; for absolute numbers use
`curl` twice + `go tool pprof -base` (see reference.md). CPU and goroutine are fine.
"""
import argparse
import collections
import json
import sys
import time
import urllib.request

ALIASES = {
    "cpu": "process_cpu:cpu:nanoseconds:cpu:nanoseconds",
    "mutex": "mutex:delay:nanoseconds:contentions:count",
    "mutex-count": "mutex:contentions:count:contentions:count",
    "block": "block:delay:nanoseconds:contentions:count",
    "block-count": "block:contentions:count:contentions:count",
    "goroutine": "goroutine:goroutine:count:goroutine:count",
    "alloc": "memory:alloc_space:bytes:space:bytes",
}


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("type")
    ap.add_argument("--minutes", type=int, default=60)
    ap.add_argument("--top", type=int, default=15)
    ap.add_argument("--match", default="")
    ap.add_argument("--service", default="stapler-squad")
    ap.add_argument("--url", default="http://localhost:4040")
    a = ap.parse_args()

    now = int(time.time() * 1000)
    body = {
        "profileTypeID": ALIASES.get(a.type, a.type),
        "labelSelector": '{service_name="%s"}' % a.service,
        "start": now - a.minutes * 60000,
        "end": now,
    }
    req = urllib.request.Request(
        a.url + "/querier.v1.QuerierService/SelectMergeStacktraces",
        data=json.dumps(body).encode(),
        headers={"Content-Type": "application/json"},
    )
    fg = json.load(urllib.request.urlopen(req, timeout=60))["flamegraph"]
    names, total = fg["names"], int(fg["total"])
    selfv = collections.Counter()
    # levels[i] is a flat list of (xDelta, total, self, nameIdx) quadruples; x is delta-encoded
    # from the end of the previous bar on the same level. Rebuild parents by x-range so a
    # leaf like sync.(*Mutex).Unlock can be charged to the nearest --match ancestor (the holder).
    prev = []  # (x, end, node) of the previous level
    for level in fg["levels"]:
        v, cur, bars = level["values"], 0, []
        for i in range(0, len(v), 4):
            x = cur + int(v[i])
            cur = x + int(v[i + 1])
            node = {"name": names[int(v[i + 3])], "self": int(v[i + 2]), "parent": None}
            for px, pend, pnode in prev:
                if px <= x < pend:
                    node["parent"] = pnode
                    break
            bars.append((x, cur, node))
            if node["self"]:
                owner = node
                while a.match and owner and a.match not in owner["name"]:
                    owner = owner["parent"]
                if owner:
                    selfv[owner["name"]] += node["self"]
        prev = bars
    print(f"{body['profileTypeID']}  window={a.minutes}m  total={total}")
    shown = 0
    for name, s in selfv.most_common():
        if a.match and a.match not in name:
            continue
        print(f"{s:>18d} {100.0 * s / max(total, 1):5.1f}%  {name}")
        shown += 1
        if shown >= a.top:
            break


if __name__ == "__main__":
    sys.exit(main())
