#!/usr/bin/env python3
import subprocess
import sys

def run_cmd(args):
    try:
        result = subprocess.run(args, capture_output=True, text=True, check=True)
        return result.stdout.strip()
    except subprocess.CalledProcessError as e:
        # Some commands might fail if main branch is missing or something, but git branch should work
        return ""

def main():
    # 1. Get list of local branches (excluding main and HEAD pointer)
    branches_raw = run_cmd(["git", "branch"])
    branches = []
    active_branch = ""
    for line in branches_raw.splitlines():
        line = line.strip()
        if not line:
            continue
        if line.startswith("*"):
            line = line[1:].strip()
            active_branch = line
        if line == "main" or line.startswith("("):
            continue
        branches.append(line)

    print(f"Active Branch: {active_branch}")
    print(f"Checking {len(branches)} local branches for commits not in main...")
    print("=" * 100)
    print(f"{'Local Branch':<40} | {'Ahead':<5} | {'Behind':<6} | {'Last Commit'}")
    print("-" * 100)

    unmerged_details = []

    for b in sorted(branches):
        # Find commits in b but not in main (ahead)
        ahead_raw = run_cmd(["git", "rev-list", "--count", f"main..{b}"])
        # Find commits in main but not in b (behind)
        behind_raw = run_cmd(["git", "rev-list", "--count", f"{b}..main"])
        
        ahead = int(ahead_raw) if ahead_raw.isdigit() else 0
        behind = int(behind_raw) if behind_raw.isdigit() else 0
        
        if ahead > 0:
            last_commit = run_cmd(["git", "log", "-n", "1", f"main..{b}", "--oneline"])
            print(f"{b:<40} | {ahead:<5} | {behind:<6} | {last_commit}")
            
            # Get list of files changed in main..b
            files_changed = run_cmd(["git", "diff", "--name-only", f"main..{b}"]).splitlines()
            unmerged_details.append({
                "branch": b,
                "ahead": ahead,
                "behind": behind,
                "last_commit": last_commit,
                "files": files_changed
            })
        else:
            # Fully merged or behind
            pass

    print("=" * 100)
    print("\n--- Detailed File Changes for Unmerged Branches ---")
    for detail in unmerged_details:
        if detail["ahead"] > 0:
            print(f"\nBranch: {detail['branch']} ({detail['ahead']} commits ahead)")
            print(f"Last commit: {detail['last_commit']}")
            print("Files changed:")
            # Limit to 5 files to avoid clutter
            for f in detail["files"][:5]:
                print(f"  - {f}")
            if len(detail["files"]) > 5:
                print(f"  ... and {len(detail['files']) - 5} more files")

if __name__ == "__main__":
    main()
