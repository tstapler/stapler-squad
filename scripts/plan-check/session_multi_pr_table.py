#!/usr/bin/env python3
"""Validate the session-multi-pr-tracking plan's task dependency table and print per-task briefs.

Usage:
  session_multi_pr_table.py [--plan PATH]            check the table against the task headings
  session_multi_pr_table.py [--plan PATH] --brief T4.2.3
"""
import argparse
import re
import sys
from pathlib import Path

DEFAULT_PLAN = "project_plans/session-multi-pr-tracking/implementation/plan.md"
TASK_HEADING = re.compile(r"^##### Task (\d+\.\d+\.\d+):")
STORY_HEADING = re.compile(r"^#### Story (\d+\.\d+):")
TABLE_ROW = re.compile(r"^\|\s*(T\d+\.\d+\.\d+)\s*\|\s*W(\d+)\s*\|\s*(.*?)\s*\|\s*$")
SECTION_START = re.compile(r"^## Task dependency graph")
TASK_ID = re.compile(r"T\d+\.\d+\.\d+")


def parse(lines):
    headings, table, dups = {}, {}, []
    in_table = False
    for i, line in enumerate(lines):
        m = TASK_HEADING.match(line)
        if m:
            headings["T" + m.group(1)] = i
        if SECTION_START.match(line):
            in_table = True
        elif in_table and line.startswith("## "):
            in_table = False
        if in_table:
            r = TABLE_ROW.match(line)
            if r:
                tid = r.group(1)
                if tid in table:
                    dups.append(tid)
                table[tid] = (int(r.group(2)), TASK_ID.findall(r.group(3)))
    return headings, table, dups


def find_cycle(table):
    state = {}

    def visit(node, path):
        state[node] = 1
        for dep in table[node][1]:
            if dep not in table:
                continue
            if state.get(dep) == 1:
                return path + [node, dep]
            if dep not in state:
                found = visit(dep, path + [node])
                if found:
                    return found
        state[node] = 2
        return None

    for n in table:
        if n not in state:
            found = visit(n, [])
            if found:
                return found
    return None


def check(headings, table, dups):
    v = []
    v += [f"{t}: duplicate table row" for t in dups]
    v += [f"{t}: task heading missing from table" for t in sorted(set(headings) - set(table))]
    v += [f"{t}: table row has no task heading" for t in sorted(set(table) - set(headings))]
    for t, (wave, deps) in table.items():
        for d in deps:
            if d not in table:
                v.append(f"{t}: unknown prerequisite {d}")
            elif table[d][0] > wave:
                v.append(f"{t}: prerequisite {d} is in a later wave (W{table[d][0]} > W{wave})")
    cyc = find_cycle(table)
    if cyc:
        v.append("cycle: " + " -> ".join(cyc))
    return v


def block(lines, start, stop_prefixes):
    end = start + 1
    while end < len(lines) and not lines[end].startswith(stop_prefixes):
        end += 1
    return "".join(lines[start:end]).rstrip() + "\n"


def brief(lines, headings, table, tid):
    if tid not in headings:
        sys.exit(f"{tid}: no such task heading")
    start = headings[tid]
    out = [block(lines, start, ("#### ", "### ", "## ", "##### "))]
    wave, deps = table.get(tid, (None, []))
    out.append(f"Wave: W{wave}\nPrerequisites:")
    out += [f"- {d} (W{table[d][0]})" if d in table else f"- {d} (unknown)" for d in deps] or ["- none"]
    story = None
    for i in range(start, -1, -1):
        if STORY_HEADING.match(lines[i]):
            story = i
            break
    if story is not None:
        out.append("\nOwning story:\n" + block(lines, story, ("##### ", "#### ", "### ", "## ")))
    return "\n".join(out)


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("--plan", default=DEFAULT_PLAN)
    ap.add_argument("--brief")
    args = ap.parse_args()
    lines = Path(args.plan).read_text().splitlines(keepends=True)
    headings, table, dups = parse(lines)
    if args.brief:
        print(brief(lines, headings, table, args.brief))
        return 0
    violations = check(headings, table, dups)
    for item in violations:
        print("VIOLATION " + item)
    print(f"tasks {len(table)} headings {len(headings)} violations {len(violations)}")
    return 1 if violations else 0


if __name__ == "__main__":
    sys.exit(main())
