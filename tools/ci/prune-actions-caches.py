#!/usr/bin/env python3
"""Prune the repo's GitHub Actions cache so it stays well under the 10 GB limit.

Policy (see docs/reference/ci-job-budget.md, "Cache policy"):
  * A cache "family" is its key with the trailing hash/date/run-id stripped.
  * Default-branch entries: keep the newest KEEP_MAIN per family, never delete the newest one.
  * Other refs (PRs): delete once the PR is closed or the entry was last used PR_MAX_AGE_DAYS ago.
  * Anything left over is dropped oldest-used-first until the total is under TARGET_GB, still
    never touching a family's newest default-branch entry.
  * bench-* entries are zero-byte baseline markers and are left alone.

Dry run by default; pass --apply to delete. Needs an authenticated `gh`.
"""
from __future__ import annotations

import argparse
import json
import re
import subprocess
import sys
from dataclasses import dataclass
from datetime import datetime, timedelta, timezone
from typing import Callable, Iterable

KEEP_MAIN = 2
PR_MAX_AGE_DAYS = 3
MAIN_MAX_AGE_DAYS = 7
TARGET_GB = 7.5
PROTECTED_PREFIXES = ("bench-",)

_FAMILY_STRIPS = (
    re.compile(r"-[0-9a-f]{16,}.*$"),  # content hash, and anything after it
    re.compile(r"-\d{8}$"),  # rolling daily key
    re.compile(r"-\d+$"),  # run id
)


def family_of(key: str) -> str:
    for rx in _FAMILY_STRIPS:
        stripped = rx.sub("", key)
        if stripped != key:
            return stripped
    return key


@dataclass(frozen=True)
class Cache:
    id: int
    key: str
    ref: str
    size: int
    created: datetime
    accessed: datetime

    @property
    def family(self) -> str:
        return family_of(self.key)


def parse_time(s: str) -> datetime:
    return datetime.fromisoformat(s.replace("Z", "+00:00"))


def load(rows: Iterable[dict]) -> list[Cache]:
    return [
        Cache(
            id=r["id"],
            key=r["key"],
            ref=r["ref"],
            size=r["size_in_bytes"],
            created=parse_time(r["created_at"]),
            accessed=parse_time(r["last_accessed_at"]),
        )
        for r in rows
    ]


def plan(
    caches: list[Cache],
    now: datetime,
    default_ref: str,
    pr_is_closed: Callable[[int], bool],
    target_bytes: int = int(TARGET_GB * 1e9),
) -> list[tuple[Cache, str]]:
    """Return (cache, reason) for each entry to delete."""
    doomed: dict[int, str] = {}
    by_id = {c.id: c for c in caches}

    main_by_family: dict[str, list[Cache]] = {}
    for c in caches:
        if c.ref == default_ref:
            main_by_family.setdefault(c.family, []).append(c)
    newest_main = set()
    for fam, rows in main_by_family.items():
        rows.sort(key=lambda c: c.created, reverse=True)
        newest_main.add(rows[0].id)
        for c in rows[KEEP_MAIN:]:
            doomed[c.id] = f"{fam}: beyond newest {KEEP_MAIN} on default branch"
        for c in rows[1:KEEP_MAIN]:
            if now - c.accessed > timedelta(days=MAIN_MAX_AGE_DAYS):
                doomed[c.id] = f"{fam}: not used for >{MAIN_MAX_AGE_DAYS}d"

    pr_closed_memo: dict[int, bool] = {}
    for c in caches:
        if c.ref == default_ref:
            continue
        m = re.fullmatch(r"refs/pull/(\d+)/merge", c.ref)
        if m:
            n = int(m.group(1))
            if n not in pr_closed_memo:
                pr_closed_memo[n] = pr_is_closed(n)
            if pr_closed_memo[n]:
                doomed[c.id] = f"PR #{n} is closed"
                continue
        if now - c.accessed > timedelta(days=PR_MAX_AGE_DAYS):
            doomed[c.id] = f"{c.ref}: not used for >{PR_MAX_AGE_DAYS}d"

    for cid in list(doomed):
        if by_id[cid].key.startswith(PROTECTED_PREFIXES) or cid in newest_main:
            del doomed[cid]

    remaining = sum(c.size for c in caches if c.id not in doomed)
    if remaining > target_bytes:
        spare = sorted(
            (
                c
                for c in caches
                if c.id not in doomed
                and c.id not in newest_main
                and not c.key.startswith(PROTECTED_PREFIXES)
            ),
            key=lambda c: c.accessed,
        )
        for c in spare:
            if remaining <= target_bytes:
                break
            doomed[c.id] = f"over {target_bytes / 1e9:.1f} GB target: oldest-used first"
            remaining -= c.size

    return [(by_id[i], r) for i, r in doomed.items()]


def gh_json_lines(args: list[str]) -> list[dict]:
    out = subprocess.run(["gh", "api", *args], check=True, capture_output=True, text=True).stdout
    return [json.loads(line) for line in out.splitlines() if line.strip()]


def main() -> int:
    ap = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    ap.add_argument("--repo", required=True, help="owner/name")
    ap.add_argument("--default-ref", default="refs/heads/main")
    ap.add_argument("--apply", action="store_true", help="actually delete (default: dry run)")
    ap.add_argument("--target-gb", type=float, default=TARGET_GB)
    args = ap.parse_args()

    rows = gh_json_lines(
        ["--paginate", f"repos/{args.repo}/actions/caches?per_page=100", "--jq", ".actions_caches[]"]
    )
    caches = load(rows)

    def pr_is_closed(n: int) -> bool:
        res = subprocess.run(
            ["gh", "api", f"repos/{args.repo}/pulls/{n}", "--jq", ".state"],
            capture_output=True,
            text=True,
        )
        return res.returncode == 0 and res.stdout.strip() == "closed"

    now = datetime.now(timezone.utc)
    doomed = plan(caches, now, args.default_ref, pr_is_closed, int(args.target_gb * 1e9))
    total = sum(c.size for c in caches)
    freed = sum(c.size for c, _ in doomed)
    print(f"{len(caches)} caches, {total / 1e9:.2f} GB; {'deleting' if args.apply else 'would delete'} "
          f"{len(doomed)} ({freed / 1e9:.2f} GB) -> {(total - freed) / 1e9:.2f} GB")
    for c, why in sorted(doomed, key=lambda t: -t[0].size):
        print(f"  {c.size / 1e6:8.1f} MB  id={c.id}  {c.ref}  {c.key[:70]}  [{why}]")
    if args.apply:
        for c, _ in doomed:
            subprocess.run(["gh", "api", "-X", "DELETE", f"repos/{args.repo}/actions/caches/{c.id}"],
                           check=True, capture_output=True)
        left = gh_json_lines([f"repos/{args.repo}/actions/cache/usage"])[0]
        print(f"after: {left['active_caches_count']} caches, {left['active_caches_size_in_bytes'] / 1e9:.2f} GB")
    return 0


if __name__ == "__main__":
    sys.exit(main())
