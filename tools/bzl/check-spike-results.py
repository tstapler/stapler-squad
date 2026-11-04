#!/usr/bin/env python3
"""Validate spike-results.yaml (Task 0.14a). Exit 0 only if every required spike id has exactly one valid row.

Rows are {spike, verdict, path, cost}. Stdlib only (no PyYAML on the host).
"""
import sys
from collections import Counter

REQUIRED = ["S-gomod", "S1", "S1-ts", "S2", "S3", "S4", "S5", "S6", "S7", "S8",
            "S9", "S10", "S10-redaction", "S11", "S12", "S13"]
OPTIONAL = ["S14", "S15", "S15b"]
VERDICTS = ("Pass", "FallbackWorkaround", "FallbackFork", "Fail")
FALLBACKS = ("FallbackWorkaround", "FallbackFork")
DEFAULT = "project_plans/bazel-conversion/implementation/spike-results.yaml"


def _scalar(v):
    v = v.strip()
    if v[:1] in "\"'":
        q = v[0]
        end = v.rfind(q)
        return v[1:end] if end > 0 else v[1:]
    if " #" in v:
        v = v.split(" #", 1)[0]
    return v.strip()


def parse_rows(text):
    """Parse a top-level list of flat `key: value` mappings."""
    rows, cur = [], None
    for raw in text.splitlines():
        line = raw.rstrip()
        if not line.strip() or line.lstrip().startswith("#"):
            continue
        body = line.lstrip()
        if body.startswith("- "):
            cur = {}
            rows.append(cur)
            body = body[2:]
        if cur is None or ":" not in body:
            raise ValueError("unparseable line: %r" % raw)
        key, val = body.split(":", 1)
        cur[key.strip()] = _scalar(val)
    return rows


def check(rows):
    """Return (per_id, errors); per_id maps id -> status string."""
    errors, per_id = [], {}
    known = set(REQUIRED) | set(OPTIONAL)
    counts = Counter(r.get("spike", "") for r in rows)
    for sid, n in counts.items():
        if sid not in known:
            errors.append("unknown spike id %r" % sid)
        elif n > 1:
            errors.append("duplicate rows for %s (%d)" % (sid, n))
    for sid in REQUIRED + OPTIONAL:
        mine = [r for r in rows if r.get("spike") == sid]
        if not mine:
            per_id[sid] = "MISSING" if sid in REQUIRED else "absent (optional)"
            if sid in REQUIRED:
                errors.append("%s: missing row" % sid)
            continue
        row, problems = mine[0], []
        verdict = row.get("verdict", "")
        if verdict not in VERDICTS:
            problems.append("invalid verdict %r" % verdict)
        elif verdict in FALLBACKS:
            for f in ("path", "cost"):
                if not row.get(f):
                    problems.append("%s requires %s" % (verdict, f))
        if len(mine) > 1:
            problems.append("duplicate")
        if problems:
            per_id[sid] = "INVALID: " + "; ".join(problems)
            errors.extend("%s: %s" % (sid, p) for p in problems if p != "duplicate")
        else:
            per_id[sid] = verdict
    return per_id, errors


def main(argv):
    path = argv[1] if len(argv) > 1 else DEFAULT
    try:
        with open(path) as f:
            rows = parse_rows(f.read())
    except (OSError, ValueError) as e:
        print("ERROR: %s" % e)
        return 2
    per_id, errors = check(rows)
    print("spike-results check: %s" % path)
    for sid in REQUIRED + OPTIONAL:
        print("  %-14s %s" % (sid, per_id[sid]))
    blocking = [s for s, v in per_id.items() if v == "Fail"]
    if blocking:
        print("BLOCKING (verdict Fail): %s" % ", ".join(blocking))
    for e in errors:
        print("ERROR: %s" % e)
    missing = [s for s in REQUIRED if per_id[s] == "MISSING"]
    if missing:
        print("MISSING required ids: %s" % ", ".join(missing))
    ok = not errors
    print("RESULT: %s" % ("OK" if ok else "FAIL"))
    return 0 if ok else 1


if __name__ == "__main__":
    sys.exit(main(sys.argv))
