#!/usr/bin/env python3
"""Reconstruct why a backlog item got (or is) stuck.

Prints the item's full stuck-state timeline (including resolved rows, which
/unfinished never shows), its item-session history with cost and end reason,
and its activity/diagnose notes — the cross-table archaeology that currently
has to be done by hand against sessions.db. For a pr_pending item it also
cross-references the live PR's failing CI checks against the PR's own diff:
a failing test outside the diff is flagged as likely pre-existing/flaky, with
a suggested `gh run rerun --failed` instead of spawning another fix session.

Usage: scripts/backlog-stuck-report.py <item-uuid-or-bl_public_id>
"""
import glob
import os
import re
import sqlite3
import subprocess
import sys

GH = "gh"
GH_TIMEOUT_SECS = 30


def candidate_dbs():
    home = os.path.expanduser("~/.stapler-squad")
    paths = [os.path.join(home, "sessions.db")]
    paths += glob.glob(os.path.join(home, "workspaces", "*", "sessions.db"))
    return [p for p in paths if os.path.exists(p)]


def find_item(ref):
    for db_path in candidate_dbs():
        conn = sqlite3.connect(f"file:{db_path}?mode=ro", uri=True)
        conn.row_factory = sqlite3.Row
        row = conn.execute(
            "SELECT * FROM backlog_items WHERE id = ? OR public_id = ?",
            (ref, ref),
        ).fetchone()
        if row:
            return db_path, conn, row
        conn.close()
    return None, None, None


def print_header(item):
    print(f"# {item['title']}")
    print(f"status={item['status']}  priority={item['priority']}  id={item['id']}  public_id={item['public_id']}")
    # kibitzer:ignore replace-magic-literal -- sqlite3.Row dict-key lookups mirroring the DB column names; a constant would just rename the same string once
    if item["pr_number"]:
        print(f"PR: {item['pr_url']}")
    print()


def print_stuck_timeline(conn, item_id):
    rows = conn.execute(
        """SELECT reason, first_detected_at, last_checked_at, resolved_at,
                  remediation_attempts, diagnose_nudge_count, context
           FROM backlog_stuck_states WHERE item_id = ?
           ORDER BY first_detected_at""",
        (item_id,),
    ).fetchall()
    print(f"## Stuck-state timeline ({len(rows)} rows, incl. resolved)")
    for r in rows:
        state = "OPEN" if r["resolved_at"] is None else f"resolved {r['resolved_at']}"
        print(f"- {r['first_detected_at']}  {r['reason']:<24} attempts={r['remediation_attempts']} nudges={r['diagnose_nudge_count']}  [{state}]")
        if r["context"]:
            context = r["context"]
            if len(context) > 300:
                context = context[:300] + "...[truncated]"
            print(f"    {context}")
    print()


def print_sessions(conn, item_id):
    rows = conn.execute(
        """SELECT session_role, created_at, started_at, ended_at, end_reason, estimated_cost_usd,
                  commit_count_since_spawn
           FROM item_sessions WHERE backlog_item_item_sessions = ?
           ORDER BY created_at""",
        (item_id,),
    ).fetchall()
    total_cost = sum(r["estimated_cost_usd"] or 0 for r in rows)
    print(f"## Item sessions ({len(rows)}, ~${total_cost:.2f} total estimated cost)")
    for r in rows:
        start = r["started_at"] or r["created_at"]
        end = r["ended_at"] or "(still open)"
        print(f"- {start} -> {end}  role={r['session_role']:<9} end_reason={r['end_reason'] or '-':<16} "
              f"commits={r['commit_count_since_spawn']} cost=${(r['estimated_cost_usd'] or 0):.2f}")
    print()


def print_notes(conn, item_id):
    rows = conn.execute(
        "SELECT created_at, message FROM backlog_activity_notes WHERE item_id = ? ORDER BY created_at",
        (item_id,),
    ).fetchall()
    print(f"## Activity notes ({len(rows)})")
    for r in rows:
        first_line = r["message"].splitlines()[0]
        print(f"- {r['created_at']}: {first_line}")
    print()


def repo_slug_from_pr_url(pr_url):
    m = re.match(r"https://github.com/([^/]+/[^/]+)/pull/(\d+)", pr_url)
    # kibitzer:ignore replace-magic-literal -- regex capture-group indices, not magic numbers
    return (m.group(1), m.group(2)) if m else (None, None)


def analyze_failing_check(slug, run_id, job_id, diff_files):
    try:
        log = subprocess.run(
            # kibitzer:ignore replace-magic-literal -- "--repo" is a literal gh CLI flag name, not a magic number/config value
            [GH, "run", "view", run_id, "--repo", slug, "--job", job_id, "--log-failed"],
            capture_output=True, text=True, timeout=2 * GH_TIMEOUT_SECS,
        ).stdout
    except Exception as e:
        print(f"    (log fetch failed: {e})")
        return
    failed_tests = sorted(set(re.findall(r"(\w+)\[\w*\]\s*>.*FAILED", log)))
    if not failed_tests:
        print("    could not identify a specific failing test class from the log")
        return
    for test_class in failed_tests:
        in_diff = any(test_class in f for f in diff_files)
        verdict = "in this PR's diff" if in_diff else "NOT in this PR's diff — likely pre-existing/flaky"
        print(f"    {test_class}: {verdict}")
        if not in_diff:
            print(f"    suggestion: `gh run rerun {run_id} --repo {slug} --failed` before spawning another fix session")


def cross_check_ci(pr_url, pr_number):
    slug, _ = repo_slug_from_pr_url(pr_url)
    if not slug:
        return
    print("## Live CI cross-check against this PR's own diff")
    try:
        checks = subprocess.run(
            [GH, "pr", "checks", pr_number, "--repo", slug],
            capture_output=True, text=True, timeout=GH_TIMEOUT_SECS,
        ).stdout
    except Exception as e:
        print(f"  (gh pr checks failed: {e})")
        return
    failing = [line.split("\t") for line in checks.splitlines() if "\tfail\t" in line]
    if not failing:
        print("  No failing checks right now.")
        return

    diff_files = subprocess.run(
        [GH, "pr", "diff", pr_number, "--repo", slug, "--name-only"],
        capture_output=True, text=True, timeout=GH_TIMEOUT_SECS,
    ).stdout.splitlines()

    for row in failing:
        name, url = row[0], row[3] if len(row) > 3 else ""
        print(f"- FAILING: {name}  {url}")
        m = re.search(r"/runs/(\d+)/job/(\d+)", url)
        if m:
            analyze_failing_check(slug, m.group(1), m.group(2), diff_files)


def main():
    if len(sys.argv) != 2:
        print(__doc__)
        sys.exit(1)
    db_path, conn, item = find_item(sys.argv[1])
    if not item:
        print(f"No backlog item found for {sys.argv[1]!r} in any known sessions.db")
        sys.exit(1)

    print_header(item)
    # kibitzer:ignore replace-magic-literal -- sqlite3.Row dict-key lookup mirroring the DB column name
    print_stuck_timeline(conn, item["id"])
    print_sessions(conn, item["id"])
    print_notes(conn, item["id"])

    if item["status"] == "pr_pending" and item["pr_number"]:
        cross_check_ci(item["pr_url"], str(item["pr_number"]))

    conn.close()


if __name__ == "__main__":
    main()
