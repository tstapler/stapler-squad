#!/usr/bin/env python3
"""Count the check runs a pull_request run lists, from the workflow YAML (static, no API).

Usage: uv run --with pyyaml python tools/ci/count-pr-checks.py [workflows-dir]

A job skipped by `if:` still shows up as a check, so every job of a triggered workflow counts.
A workflow-level `paths:` / `paths-ignore:` filter that does not match creates no checks at all.
Matrices expand statically; a matrix built with fromJSON() counts as 1. Jobs that skip on a PR
count as one skipped check (heuristic on `if:`; jobs whose skip comes from needs/detect-changes
outputs are counted as running, so the code-PR figure is an upper bound). Reusable-workflow calls count the callee's jobs.
"""
import glob
import os
import re
import sys

import yaml

wf_dir = sys.argv[1] if len(sys.argv) > 1 else ".github/workflows"
docs_only_file = "docs/adr/x.md"  # a path under every docs ignore pattern we use


def load(path):
    with open(path) as f:
        return yaml.safe_load(f)


def matrix_size(job):
    m = (job.get("strategy") or {}).get("matrix")
    if not isinstance(m, dict):
        return 1
    n = 1
    include = m.get("include")
    axes = {k: v for k, v in m.items() if k not in ("include", "exclude")}
    for v in axes.values():
        n *= len(v) if isinstance(v, list) else 1
    if axes:
        return n + (len(include) if isinstance(include, list) else 0)
    return len(include) if isinstance(include, list) else 1


def skipped_on_pr(job):
    """True for the common idioms that make a job skip on pull_request (it then shows as ONE
    skipped check, even for a matrix)."""
    cond = str(job.get("if", ""))
    not_pr = re.search(r"github\.event_name != 'pull_request'", cond)
    if not_pr and "||" not in cond[not_pr.end():]:
        return True
    return "github.ref == 'refs/heads/main'" in cond


def job_count(doc, wf_dir):
    total = 0
    for job in doc["jobs"].values():
        uses = job.get("uses", "")
        if uses.startswith("./.github/workflows/"):
            total += job_count(load(os.path.join(wf_dir, os.path.basename(uses))), wf_dir)
        elif skipped_on_pr(job):
            total += 1
        else:
            total += matrix_size(job)
    return total


rows, code_total, docs_total = [], 0, 0
for path in sorted(glob.glob(os.path.join(wf_dir, "*.yml"))):
    doc = load(path)
    on = doc.get(True, doc.get("on")) or {}
    pr = on.get("pull_request") if isinstance(on, dict) else None
    if isinstance(on, dict) and "pull_request" not in on:
        continue
    if isinstance(on, list) and "pull_request" not in on:
        continue
    pr = pr or {}
    n = job_count(doc, wf_dir)
    if pr.get("types") == ["labeled"]:
        mode, code, docs = "label-only", 0, 0
    elif pr.get("paths"):
        mode, code, docs = "paths", n, 0
    elif pr.get("paths-ignore"):
        mode, code, docs = "paths-ignore", n, 0
    else:
        mode, code, docs = "always", n, n
    rows.append((os.path.basename(path), mode, n))
    code_total += code
    docs_total += docs

print(f"{'workflow':38} {'PR trigger':13} jobs")
for name, mode, n in rows:
    print(f"{name:38} {mode:13} {n}")
print(f"\nchecks on a code PR touching every filtered path : {code_total}")
print(f"checks on a docs-only PR (e.g. {docs_only_file})     : {docs_total}")
