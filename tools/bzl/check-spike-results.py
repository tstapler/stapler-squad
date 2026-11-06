#!/usr/bin/env python3
"""Validate spike-results.yaml (Task 0.14a). Exit 0 only if every required spike id has exactly one valid row.

Rows are {spike, verdict, path, cost}. Stdlib only (no PyYAML on the host).
"""
import json
import os
import re
import sys
from collections import Counter

REQUIRED = ["S-gomod", "S1", "S1-ts", "S2", "S3", "S4", "S5", "S6", "S7", "S8",
            "S9", "S10", "S10-redaction", "S11", "S12", "S13"]
OPTIONAL = ["S14", "S15", "S15b"]
VERDICTS = ("Pass", "FallbackWorkaround", "FallbackFork", "Fail")
FALLBACKS = ("FallbackWorkaround", "FallbackFork")
DEFAULT = "project_plans/bazel-conversion/implementation/spike-results.yaml"
MEASUREMENTS = "spike-measurements.json"  # beside the results file; its noisy_inputs map is display only
ATTESTATIONS = "owner-attestations.yaml"  # beside the results file; display only, never changes the verdict


def owner_deferrals(path):
    """Return (spike ids, label, date) for a recorded owner deferral, or None. Display only."""
    if not os.path.exists(path):
        return None
    sec, date, found = None, "", {}
    with open(path) as f:
        for raw in f:
            if re.match(r"^\S", raw):
                sec = raw.split(":", 1)[0]
                m = re.match(r"^interview_date:\s*(\S+)", raw)
                date = m.group(1) if m else date
            elif sec == "remote_cache_s10":
                m = re.match(r"^\s+(decision|pending):\s*(.*)$", raw.rstrip())
                if m:
                    found[m.group(1)] = m.group(2)
    decision = found.get("decision", "")
    if "defer" not in decision and "disk cache" not in decision:
        return None
    ids = [i for i in re.findall(r"S\d+(?:-redaction)?", found.get("pending", "")) if i in REQUIRED]
    ids = list(dict.fromkeys(ids))
    if not ids:
        return None
    return ids, "disk cache first" if "disk cache" in decision else decision, date


def owner_accepted_fails(path):
    """Return (spike ids, decision, date) for a recorded owner acceptance of a Fail verdict, or None.

    Reads the `adr_009_e2e` section of owner-attestations.yaml. Display only: a Fail row stays Fail and the
    exit code never depends on this.
    """
    if not os.path.exists(path):
        return None
    sec, found = None, {}
    with open(path) as f:
        for raw in f:
            if re.match(r"^\S", raw):
                sec = raw.split(":", 1)[0]
            elif sec == "adr_009_e2e":
                m = re.match(r"^\s+(decision|date|spikes|status):\s*(.*)$", raw.rstrip())
                if m:
                    found[m.group(1)] = m.group(2).strip().strip("\"'")
    if found.get("status") != "recorded" or not found.get("decision"):
        return None
    ids = [i for i in re.findall(r"S\d+[A-Za-z]*(?:-redaction)?", found.get("spikes", "")) if i in OPTIONAL + REQUIRED]
    if not ids:
        return None
    return ids, found["decision"], found.get("date", "")


def noisy_inputs(path):
    """Keys of spike-measurements.json recorded as measured under load (owner-authorised override); [] when none."""
    try:
        with open(path) as f:
            side = json.load(f).get("noisy_inputs")
    except (OSError, ValueError, AttributeError):
        return []
    return sorted(k for k, v in side.items() if v) if isinstance(side, dict) else []


def noisy_rows(rows):
    """Spike ids whose row carries `noisy: true` (verdict computed from a run under load)."""
    return [r["spike"] for r in rows if r.get("noisy", "").lower() == "true" and r.get("spike")]


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
    noisy_ids = set(noisy_rows(rows))
    for sid in REQUIRED + OPTIONAL:
        print("  %-14s %s%s" % (sid, per_id[sid], "  [NOISY: measured under load, owner-authorised]" if sid in noisy_ids else ""))
    attest_path = os.path.join(os.path.dirname(os.path.abspath(path)), ATTESTATIONS)
    accepted = owner_accepted_fails(attest_path)
    accepted_ids = set(accepted[0]) if accepted else set()
    blocking = [s for s, v in per_id.items() if v == "Fail" and s not in accepted_ids]
    if blocking:
        print("BLOCKING (verdict Fail): %s" % ", ".join(blocking))
    owned = [s for s, v in per_id.items() if v == "Fail" and s in accepted_ids]
    if owned:
        print("OWNER-ACCEPTED Fail (verdict stays Fail): %s (%s, %s)" % (", ".join(owned), accepted[1], accepted[2]))
    for e in errors:
        print("ERROR: %s" % e)
    missing = [s for s in REQUIRED if per_id[s] == "MISSING"]
    if missing:
        print("MISSING required ids: %s" % ", ".join(missing))
    deferred = owner_deferrals(attest_path)
    if deferred:
        print("owner-deferred: %s (%s, %s)" % (", ".join(deferred[0]), deferred[1], deferred[2]))
    noisy_keys = noisy_inputs(os.path.join(os.path.dirname(os.path.abspath(path)), MEASUREMENTS))
    if noisy_ids or noisy_keys:
        print("NOISY: %d verdict row(s) (%s) and %d measurement input(s) (%s) were measured under load, not on a quiet host"
              % (len(noisy_ids), ", ".join(sorted(noisy_ids)) or "none", len(noisy_keys), ", ".join(noisy_keys) or "none"))
    ok = not errors
    print("RESULT: %s" % ("OK" if ok else "FAIL"))
    return 0 if ok else 1


if __name__ == "__main__":
    sys.exit(main(sys.argv))
