#!/usr/bin/env python3
"""Build the task-level dependency table from plan.md "Depends on" lines and check it is acyclic.

Usage: python3 check-task-deps.py [plan.md]   (prints a markdown table, then CYCLE-CHECK result)

Rules (an over-approximation, so "acyclic" here implies truly acyclic):
- Only lines before "## Test-file ownership" containing "Depends on" count; parenthetical text is stripped
  and the reference list ends at the first ". " / ";" / " **" so explanatory IDs are not read as edges.
- A line under a Task heading is a task edge; a line under a Story heading but before its first Task is a
  story edge (every task of the story depends on the target).
- A target that is a story id means all of that story's tasks; "Epic N.1" means every story of that epic;
  "A through B" is an inclusive story range. Within-story task order and shared-file serialization (Wave B)
  are scheduling, not Depends-on edges, and are not modeled.
"""
import re
import sys
from collections import defaultdict

path = sys.argv[1] if len(sys.argv) > 1 else "plan.md"
lines = open(path, encoding="utf-8").read().split("\n")
end = next(i for i, l in enumerate(lines) if l.startswith("## Test-file ownership"))

story_tasks = defaultdict(list)
task_order = []
cur_story = cur_task = None
story_edges = defaultdict(set)  # story -> raw targets
task_edges = defaultdict(set)   # task  -> raw targets

def refs(text):
    text = re.sub(r"`[^`]*`", "", text)
    text = re.sub(r"\([^)]*\)", "", text)
    text = re.split(r"\. |;| \*\*|, not ", text)[0]
    out = []
    for m in re.finditer(r"(\d+\.\d+\.\d+)\s+through\s+(\d+\.\d+\.\d+)", text):
        a, b = m.group(1).split("."), m.group(2).split(".")
        out += [f"{a[0]}.{a[1]}.{n}" for n in range(int(a[2]), int(b[2]) + 1)]
    text = re.sub(r"\d+\.\d+\.\d+\s+through\s+\d+\.\d+\.\d+", "", text)
    for m in re.finditer(r"Epic (\d+\.\d+)", text):
        out.append("EPIC:" + m.group(1))
    out += re.findall(r"\b\d+\.\d+\.\d+[a-z]?\b", text)
    return out

for l in lines[:end]:
    m = re.match(r"#### Story (\d+\.\d+\.\d+)", l)
    if m:
        cur_story, cur_task = m.group(1), None
        continue
    m = re.match(r"##### Task (\d+\.\d+\.\d+[a-z]):", l)
    if m:
        cur_task = m.group(1)
        story_tasks[cur_story].append(cur_task)
        task_order.append(cur_task)
        continue
    if "Depends on" in l and cur_story:
        tail = re.split(r"Depends on:?\*{0,2}:?", l, maxsplit=1)[1]
        (task_edges[cur_task] if cur_task else story_edges[cur_story]).update(refs(tail))

def expand(raw, self_story):
    res = set()
    for r in raw:
        if r.startswith("EPIC:"):
            for s, ts in story_tasks.items():
                if s.startswith(r[5:] + "."):
                    res.update(ts)
        elif re.fullmatch(r"\d+\.\d+\.\d+[a-z]", r):
            if r in task_order:
                res.add(r)
        elif r in story_tasks and r != self_story:
            res.update(story_tasks[r])
    return res

deps = {}
for s, ts in story_tasks.items():
    for t in ts:
        deps[t] = expand(story_edges[s], s) | expand(task_edges[t], s)
        deps[t].discard(t)

def story_of(t):
    return t[:-1]

def compact(ts):
    by = defaultdict(set)
    for t in ts:
        by[story_of(t)].add(t)
    out = []
    for s in sorted(by, key=lambda x: [int(p) for p in x.split(".")]):
        out.append(s + " (all)" if by[s] == set(story_tasks[s]) else ", ".join(sorted(by[s])))
    return ", ".join(out)

print("| Task | Depends on |")
print("|---|---|")
for t in task_order:
    print(f"| {t} | {compact(deps[t]) or 'none'} |")

# Kahn's algorithm
indeg = {t: len(deps[t]) for t in deps}
rev = defaultdict(list)
for t, ds in deps.items():
    for d in ds:
        rev[d].append(t)
queue = [t for t, n in indeg.items() if n == 0]
seen = 0
while queue:
    n = queue.pop()
    seen += 1
    for m in rev[n]:
        indeg[m] -= 1
        if indeg[m] == 0:
            queue.append(m)
edges = sum(len(d) for d in deps.values())
if seen == len(deps):
    print(f"\nCYCLE-CHECK: acyclic; {len(deps)} task nodes, {edges} edges, {len(story_tasks)} stories")
else:
    stuck = sorted(t for t, n in indeg.items() if n > 0)
    print(f"\nCYCLE-CHECK: CYCLE among {len(stuck)} tasks: {stuck}")
    sys.exit(1)
