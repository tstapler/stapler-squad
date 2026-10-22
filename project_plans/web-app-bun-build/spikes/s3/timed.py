#!/usr/bin/env python3
"""Run a command; print one JSON line {rc, wall_s, maxrss_kb, load1}. Stand-in for `/usr/bin/time -v` (absent on this host).
Usage: python3 -I timed.py <cwd> <logfile> -- cmd args...
maxrss_kb is RUSAGE_CHILDREN: the largest single descendant of this process (not the sum)."""
import json, os, resource, subprocess, sys, time

cwd, log = sys.argv[1], sys.argv[2]
cmd = sys.argv[sys.argv.index("--") + 1:]
env = {k: v for k, v in os.environ.items() if k != "npm_config_user_agent"}
t0 = time.monotonic()
with open(log, "wb") as f:
    rc = subprocess.run(cmd, cwd=cwd, env=env, stdout=f, stderr=subprocess.STDOUT).returncode
wall = time.monotonic() - t0
print(json.dumps({
    "rc": rc,
    "wall_s": round(wall, 2),
    "maxrss_kb": resource.getrusage(resource.RUSAGE_CHILDREN).ru_maxrss,
    "load1": os.getloadavg()[0],
}))
