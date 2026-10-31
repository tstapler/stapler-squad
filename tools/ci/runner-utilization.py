#!/usr/bin/env python3
"""Runner utilization sampler for GitHub Actions jobs (Linux runners; stdlib only).

  runner-utilization.py start  <dir>            fork a background sampler (every 5 s)
  runner-utilization.py finish <dir> <label>    stop it, write the step summary + JSON

Never fails the job: every error path exits 0. Samples /proc/stat (CPU, iowait,
steal), /proc/meminfo (used = MemTotal - MemAvailable) and /proc/pressure/*
(PSI "some" stall time). Aggregated across runs by tools/ci/utilization-report.py.
"""
import json
import os
import signal
import subprocess
import sys
import threading
import time

INTERVAL_S = 5


def read_cpu():
    f = open("/proc/stat").readline().split()[1:]
    v = [int(x) for x in f]
    # user nice system idle iowait irq softirq steal
    return {"total": sum(v[:8]), "idle": v[3], "iowait": v[4], "steal": v[7] if len(v) > 7 else 0}


def read_mem_mb():
    m = {}
    for line in open("/proc/meminfo"):
        k, _, rest = line.partition(":")
        m[k] = int(rest.split()[0]) // 1024
    return m["MemTotal"], m["MemTotal"] - m["MemAvailable"]


def read_psi():
    out = {}
    for res in ("cpu", "memory", "io"):
        try:
            for line in open(f"/proc/pressure/{res}"):
                if line.startswith("some"):
                    out[res] = int(dict(p.split("=") for p in line.split()[1:])["total"])  # microseconds
        except OSError:
            pass
    return out


def sample_loop(d):
    os.makedirs(d, exist_ok=True)
    stop = threading.Event()
    signal.signal(signal.SIGTERM, lambda *_: stop.set())
    prev, prev_t, prev_psi = read_cpu(), time.time(), read_psi()
    with open(os.path.join(d, "samples.jsonl"), "a", buffering=1) as out:
        while True:
            stopping = stop.wait(INTERVAL_S)  # wakes at once on SIGTERM; the final partial window is still sampled
            try:
                cur, now, psi = read_cpu(), time.time(), read_psi()
                dt = cur["total"] - prev["total"]
                _, used = read_mem_mb()
            except (OSError, ValueError, KeyError):
                if stopping:
                    break
                continue
            if dt > 0:
                row = {
                    "cpu": 100.0 * (dt - (cur["idle"] - prev["idle"]) - (cur["iowait"] - prev["iowait"])) / dt,
                    "iowait": 100.0 * (cur["iowait"] - prev["iowait"]) / dt,
                    "steal": 100.0 * (cur["steal"] - prev["steal"]) / dt,
                    "mem_mb": used,
                }
                for res, tot in psi.items():
                    if res in prev_psi:
                        row["psi_" + res] = 100.0 * (tot - prev_psi[res]) / ((now - prev_t) * 1e6)
                out.write(json.dumps(row) + "\n")
            prev, prev_t, prev_psi = cur, now, psi
            if stopping:
                break


def start(d):
    os.makedirs(d, exist_ok=True)
    with open(os.path.join(d, "started"), "w") as f:
        f.write(str(time.time()))
    p = subprocess.Popen(
        [sys.executable, "-I", os.path.abspath(__file__), "sample", d],
        stdin=subprocess.DEVNULL, stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL,
        start_new_session=True,
    )
    with open(os.path.join(d, "pid"), "w") as f:
        f.write(str(p.pid))


def pct(vals, p):
    s = sorted(vals)
    return s[min(len(s) - 1, int(len(s) * p))]


def finish(d, label):
    try:
        os.kill(int(open(os.path.join(d, "pid")).read()), signal.SIGTERM)
        time.sleep(0.3)  # let the sampler flush its last sample
    except (OSError, ValueError):
        pass
    wall = time.time() - float(open(os.path.join(d, "started")).read())
    rows = []
    try:
        rows = [json.loads(line) for line in open(os.path.join(d, "samples.jsonl")) if line.strip()]
    except OSError:
        pass
    cores = os.cpu_count() or 0
    mem_total, _ = read_mem_mb()
    s = {
        "label": label, "run_id": os.environ.get("GITHUB_RUN_ID"), "run_attempt": os.environ.get("GITHUB_RUN_ATTEMPT"),
        "workflow": os.environ.get("GITHUB_WORKFLOW"), "job": os.environ.get("GITHUB_JOB"),
        "event": os.environ.get("GITHUB_EVENT_NAME"), "runner": os.environ.get("RUNNER_NAME"), "ref": os.environ.get("GITHUB_REF_NAME"),
        "cores": cores, "mem_total_mb": mem_total, "wall_s": round(wall), "samples": len(rows),
    }
    if rows:
        cpu = [r["cpu"] for r in rows]
        s.update(
            cpu_avg=round(sum(cpu) / len(cpu), 1), cpu_p95=round(pct(cpu, 0.95), 1), cpu_max=round(max(cpu), 1),
            iowait_avg=round(sum(r["iowait"] for r in rows) / len(rows), 1),
            steal_avg=round(sum(r["steal"] for r in rows) / len(rows), 1),
            mem_peak_mb=max(r["mem_mb"] for r in rows),
        )
        for res in ("cpu", "memory", "io"):
            v = [r["psi_" + res] for r in rows if "psi_" + res in r]
            if v:
                s["psi_" + res + "_avg"] = round(sum(v) / len(v), 1)
    with open(os.path.join(d, "utilization.json"), "w") as f:
        json.dump(s, f)
    lines = [f"#### Runner utilization: {label}", "", "| metric | value |", "|---|---|",
             f"| runner | {cores} vCPU, {mem_total} MB RAM |", f"| wall | {s['wall_s']} s ({s['samples']} samples @ {INTERVAL_S} s) |"]
    if rows:
        lines += [f"| CPU avg / p95 / max | {s['cpu_avg']}% / {s['cpu_p95']}% / {s['cpu_max']}% (avg = {s['cpu_avg'] * cores / 100:.1f} of {cores} cores busy) |",
                  f"| iowait avg / steal avg | {s['iowait_avg']}% / {s['steal_avg']}% |",
                  f"| memory peak | {s['mem_peak_mb']} MB ({100 * s['mem_peak_mb'] // max(mem_total, 1)}%) |"]
        psi = [f"{r} {s['psi_' + r + '_avg']}%" for r in ("cpu", "memory", "io") if "psi_" + r + "_avg" in s]
        lines.append("| PSI some (stalled share) | " + (", ".join(psi) if psi else "unavailable") + " |")
    summary = os.environ.get("GITHUB_STEP_SUMMARY")
    if summary:
        with open(summary, "a") as f:
            f.write("\n".join(lines) + "\n")


if __name__ == "__main__":
    try:
        mode = sys.argv[1]
        if mode == "start":
            start(sys.argv[2])
        elif mode == "sample":
            sample_loop(sys.argv[2])
        elif mode == "finish":
            finish(sys.argv[2], sys.argv[3])
    except Exception as e:  # observability must never fail the job
        print(f"runner-utilization: {e}", file=sys.stderr)
    sys.exit(0)
