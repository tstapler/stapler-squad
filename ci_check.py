#!/usr/bin/env python3
"""CI status check and re-run helper for PR #130."""
import json, subprocess, sys

PR = 130
BRANCH = "stapler-squad-transfer"

def run(cmd, check=True):
    r = subprocess.run(cmd, shell=True, capture_output=True, text=True)
    if check and r.returncode != 0:
        print(f"ERROR: {r.stderr.strip()}", file=sys.stderr)
    return r.stdout.strip()

def gh(cmd):
    return json.loads(run(f"gh {cmd}"))

def main():
    mode = sys.argv[1] if len(sys.argv) > 1 else "status"

    # --- status / logs / rerun ---
    if mode == "status":
        head = gh(f"pr view {PR} --json headRefOid,mergeable,mergeStateStatus")
        sha = head["headRefOid"][:8]
        print(f"HEAD: {sha}  mergeable={head['mergeable']}  state={head['mergeStateStatus']}\n")

        runs = gh(f"run list --branch {BRANCH} --limit 20 --json databaseId,name,status,conclusion,event,headSha")
        latest = [r for r in runs if r["headSha"][:8] == sha]
        if not latest:
            print("No CI runs yet for HEAD commit.")
            return

        icons = {"success": "✓", "failure": "✗", "cancelled": "⊘"}
        for r in sorted(latest, key=lambda x: x["name"]):
            st = r["conclusion"] or r["status"]
            icon = icons.get(r["conclusion"] or "", "…")
            print(f"  {icon} {r['name']:35s} {st}")

        failed = [r for r in latest if r["conclusion"] == "failure"]
        passing = [r for r in latest if r["conclusion"] == "success"]
        running = [r for r in latest if not r["conclusion"]]
        print(f"\n{len(passing)} passed, {len(failed)} failed, {len(running)} running")

        if failed:
            print(f"\nFailed IDs: {', '.join(str(r['databaseId']) for r in failed)}")
            print("Run:  python3 ci_check.py logs     # show failure output")
            print("Run:  python3 ci_check.py rerun    # re-run all failed jobs")

    elif mode == "logs":
        runs = gh(f"run list --branch {BRANCH} --limit 20 --json databaseId,name,conclusion,headSha")
        head = gh(f"pr view {PR} --json headRefOid")["headRefOid"][:8]
        failed = [r for r in runs if r["headSha"][:8] == head and r["conclusion"] == "failure"]
        for r in failed:
            print(f"\n{'='*60}")
            print(f"FAILED: {r['name']} (run {r['databaseId']})")
            print('='*60)
            out = run(f"gh run view {r['databaseId']} --log-failed", check=False)
            # Extract just the error lines — strip timestamp/job prefix noise
            for line in out.splitlines():
                parts = line.split("\t")
                msg = parts[-1] if parts else line
                if any(k in msg for k in ("error", "Error", "FAIL", "Type error", "Cannot find", "##[error]", "panic")):
                    print(msg)

    elif mode == "rerun":
        runs = gh(f"run list --branch {BRANCH} --limit 20 --json databaseId,name,conclusion,headSha")
        head = gh(f"pr view {PR} --json headRefOid")["headRefOid"][:8]
        failed = [r for r in runs if r["headSha"][:8] == head and r["conclusion"] == "failure"]
        if not failed:
            print("No failed runs to re-run.")
            return
        for r in failed:
            print(f"Re-running: {r['name']} ({r['databaseId']})")
            run(f"gh run rerun {r['databaseId']} --failed", check=False)
        print("Done. Run: python3 ci_check.py status")

    elif mode == "mergeable":
        head = gh(f"pr view {PR} --json headRefOid,mergeable,mergeStateStatus,statusCheckRollup")
        print(f"mergeable: {head['mergeable']}")
        print(f"state:     {head['mergeStateStatus']}")

    else:
        print("Usage: ci_check.py [status|logs|rerun|mergeable]")

if __name__ == "__main__":
    main()
