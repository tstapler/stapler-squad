#!/usr/bin/env python3
"""Aggregate runner-utilization artifacts across recent runs into a per-job table.

  python3 -I tools/ci/utilization-report.py [--repo OWNER/REPO] [--workflow Build] [--runs 20]
        [--event pull_request] [--branch NAME] [--prom-out FILE] [--push-vm URL]

Needs `gh` (authenticated). Artifacts come from .github/actions/runner-utilization and
expire after 7 days. --prom-out / --push-vm are for the owner's machine only (node_exporter
textfile collector, or VictoriaMetrics e.g. http://localhost:8428); CI never uses them.
"""
import argparse
import collections
import json
import re
import statistics
import subprocess
import sys
import tempfile
import urllib.request
from datetime import datetime
from pathlib import Path


def gh(*args):
    r = subprocess.run(["gh", *args], capture_output=True, text=True)
    if r.returncode:
        raise SystemExit(f"gh {' '.join(args)}: {r.stderr.strip()}")
    return r.stdout


def ts(s):
    return datetime.strptime(s, "%Y-%m-%dT%H:%M:%SZ")


NUMERIC = ("cores", "wall_s", "cpu_avg", "cpu_p95", "iowait_avg", "mem_peak_mb")


def valid(path):
    """Artifacts from fork PR runs are attacker-controlled: accept only a strictly shaped record."""
    try:
        u = json.loads(path.read_text())
        ok = isinstance(u, dict) and isinstance(u.get("label"), str) and re.fullmatch(r"[A-Za-z0-9_.${}-]{1,60}", u["label"])
        ok = ok and all(isinstance(u.get(k), (int, float)) and not isinstance(u.get(k), bool) for k in NUMERIC)
        for k in ("psi_cpu_avg",):
            ok = ok and (k not in u or isinstance(u[k], (int, float)))
        if ok and not isinstance(u.get("runner"), (str, type(None))):
            ok = False
        return u if ok else None
    except (OSError, ValueError):
        return None


def verdict(cpu, iowait, psi_cpu):
    if iowait is not None and iowait > 20:
        return "io-bound"
    if cpu >= 90 or (psi_cpu or 0) >= 40:
        return "saturated"
    return "under-used" if cpu < 40 else "right-sized"


def p90(v):
    v = sorted(v)
    return v[min(len(v) - 1, int(len(v) * 0.9))]


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("--repo", default="tstapler/stapler-squad")
    ap.add_argument("--workflow", default="Build")
    ap.add_argument("--runs", type=int, default=20)
    ap.add_argument("--event")
    ap.add_argument("--branch")
    ap.add_argument("--prom-out")
    ap.add_argument("--push-vm")
    a = ap.parse_args()
    if not re.fullmatch(r"[A-Za-z0-9 _.-]{1,60}", a.workflow):
        sys.exit("invalid --workflow")

    cmd = ["run", "list", "--repo", a.repo, "--workflow", a.workflow, "--status", "completed",
           "--limit", str(a.runs), "--json", "databaseId,event,headBranch"]
    if a.event:
        cmd += ["--event", a.event]
    if a.branch:
        cmd += ["--branch", a.branch]
    runs = json.loads(gh(*cmd))
    by_label = collections.defaultdict(list)
    for run in runs:
        rid = run["databaseId"]
        with tempfile.TemporaryDirectory() as d:
            r = subprocess.run(["gh", "run", "download", str(rid), "--repo", a.repo, "--pattern", "util-*", "--dir", d],
                               capture_output=True, text=True)
            if r.returncode:
                continue  # no artifacts (expired or job not instrumented)
            records = [u for u in map(valid, Path(d).glob("*/utilization.json")) if u]
        queue = {}
        try:
            for j in json.loads(gh("api", "--paginate", f"repos/{a.repo}/actions/runs/{rid}/jobs?per_page=100",
                                   "--jq", "[.jobs[]|{runner_name,created_at,started_at}]")):
                if j["started_at"]:
                    queue[j["runner_name"]] = (ts(j["started_at"]) - ts(j["created_at"])).total_seconds()
        except (SystemExit, json.JSONDecodeError):
            pass
        for u in records:
            if u:
                u["queue_s"] = queue.get(u.get("runner"))
                by_label[re.sub(r"-shard-\d+$", "-shard", u["label"])].append(u)

    if not by_label:
        sys.exit("no utilization artifacts found (jobs not instrumented yet, or artifacts expired after 7 days)")
    hdr = f"{'job':34} {'n':>3} {'cores':>5} {'wall med/p90 s':>15} {'cpu avg med/p90 %':>18} {'cpu p95 %':>9} {'iowait %':>8} {'mem peak MB':>11} {'psi cpu %':>9} {'queue med s':>11}  verdict"
    print(hdr)
    prom = []
    for label, us in sorted(by_label.items()):
        med = lambda k: statistics.median([u[k] for u in us if k in u]) if any(k in u for u in us) else None
        cpu = [u["cpu_avg"] for u in us]
        qs = [u["queue_s"] for u in us if u.get("queue_s") is not None]
        v = verdict(statistics.median(cpu), med("iowait_avg"), med("psi_cpu_avg"))
        w = [u["wall_s"] for u in us]
        print(f"{label[:34]:34} {len(us):>3} {us[0]['cores']:>5} {statistics.median(w):>7.0f}/{p90(w):<7.0f} "
              f"{statistics.median(cpu):>9.1f}/{p90(cpu):<8.1f} {med('cpu_p95'):>9.1f} {med('iowait_avg'):>8.1f} "
              f"{med('mem_peak_mb'):>11.0f} {(med('psi_cpu_avg') if med('psi_cpu_avg') is not None else float('nan')):>9.1f} "
              f"{(statistics.median(qs) if qs else float('nan')):>11.0f}  {v}")
        for k in ("cpu_avg", "cpu_p95", "iowait_avg", "mem_peak_mb", "wall_s", "psi_cpu_avg"):
            if med(k) is not None:
                prom.append(f'ci_runner_{k}{{workflow="{a.workflow}",job="{label}"}} {med(k)}')
    text = "\n".join(prom) + "\n"
    if a.prom_out:
        Path(a.prom_out).write_text(text)
    if a.push_vm:
        urllib.request.urlopen(urllib.request.Request(a.push_vm.rstrip("/") + "/api/v1/import/prometheus",
                                                      data=text.encode(), method="POST"), timeout=10).read()


if __name__ == "__main__":
    main()
