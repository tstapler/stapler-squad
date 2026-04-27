#!/usr/bin/env python3
"""Report untested features and write coverage summary for CI."""
import json
import glob
import os

untested = []
for path in sorted(glob.glob('docs/registry/features/backend/**/*.json', recursive=True)):
    with open(path) as f:
        d = json.load(f)
    if not d.get('testIds'):
        untested.append(d['id'])

total = len(list(glob.glob('docs/registry/features/backend/**/*.json', recursive=True)))
covered = total - len(untested)
pct = round(covered / total * 100, 1) if total else 0

print(f"Coverage: {covered}/{total} features have testIds ({pct}%)")
if untested:
    print(f"\nUntested features ({len(untested)}):")
    for fid in untested:
        print(f"  - {fid}")

with open('/tmp/coverage_summary.txt', 'w') as f:
    f.write(f"covered={covered}\ntotal={total}\npct={pct}\nuntested_count={len(untested)}\n")
