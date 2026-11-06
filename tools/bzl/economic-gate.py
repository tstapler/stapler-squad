#!/usr/bin/env python3
"""Economic abort / re-plan decision gate (Task 0.14e), criteria EG1-EG5.

Exit codes:
  0  CLEAR, or every tripped criterion is covered by a recorded `continue`
     decision and nothing is UNMEASURED
  3  a criterion tripped and no recorded decision covers it (OWNER DECISION)
  4  nothing uncovered tripped but some criterion is UNMEASURED (never CLEAR)
  5  recorded decision is `re-plan`    6  `pause`    7  `revert`
A decision covers the criteria in its optional `criteria:` list, else the EGn
ids named in its `reason`. Only criteria that actually TRIPPED are covered; a
decision never turns UNMEASURED into CLEAR. owner-attestations.yaml is read
only, never written, and this script never picks a decision. Decisions apply
to the EG gate only, not --midpoint.

An input listed under `noisy_inputs` in that file was measured under load with the owner-authorised
override (BZL_ALLOW_NOISY_HOST=1 / -allow-noisy-host); any criterion computed from it prints NOISY and the
summary counts them. The verdict is still computed from the number; the tag is never hidden.

Measurements not available from measured.json (S6/S10 timings, re-costed CU)
come from a JSON file (--inputs, default spike-measurements.json beside
spike-results.yaml). Keys: s10_second_run_job_s, unconverted_jobs_s,
s10_cold_run_s, s6_first_cold_s, s6_second_cold_s, make_cold_median_s,
recosted_cu_m, e2e_selected_share_median (spike S14: median selected-spec
share of the replay, 1.0 when selection narrows nothing; unset means S14 has no number and s = 1.0);
midpoint: mid_t1_ratio, mid_second_cold_s, mid_first_cold_s,
mid_edit_test_s, baseline_edit_test_s.
"""
import argparse
import json
import os
import re
import sys

HERE = os.path.dirname(os.path.abspath(__file__))
IMPL = os.path.join(HERE, "..", "..", "project_plans", "bazel-conversion", "implementation")
MEASURED = os.path.join(HERE, "baseline", "measured.json")
VERIFIED_CU_M = 30.50  # plan.md header: verified total
TARGETS = os.path.join(HERE, "baseline", "targets.yaml")
# Defaults are the plan's proposal values; targets.yaml `economic_gate_factors` overrides them.
EG1_X, EG2_X, EG3_FIRST_X, EG3_SECOND_X, EG4_X, EG5_LIMIT = 1.10, 1.5, 3.0, 2.0, 1.25, 0.60

UNMEASURED = "UNMEASURED"
ATTESTATIONS = os.path.join(IMPL, "owner-attestations.yaml")
DECISION_EXIT = {"re-plan": 5, "pause": 6, "revert": 7}


def load_decision(path):
    """Read the top-level economic_gate block: {decision, date, covers} or None (no PyYAML needed)."""
    if not os.path.exists(path):
        return None
    block, in_block, key = {}, False, None
    with open(path) as f:
        for raw in f:
            line = raw.rstrip("\n")
            if re.match(r"^\S", line):
                in_block, key = line.startswith("economic_gate:"), None
                continue
            if not in_block or not line.strip() or line.lstrip().startswith("#"):
                continue
            m = re.match(r"^  (\w+):\s*(.*)$", line)
            if m:
                key = m.group(1)
                val = m.group(2).split(" #", 1)[0].strip()
                block[key] = "" if val in (">", ">-", "|", "|-") else val
            elif key:
                block[key] = (block[key] + " " + line.strip()).strip()
    decision = block.get("decision", "").strip()
    if not decision:
        return None
    listed = re.findall(r"EG\d", block.get("criteria", ""))
    covers = set(listed or re.findall(r"EG\d", block.get("reason", "")))
    return {"decision": decision, "date": block.get("date", "unknown"), "covers": covers}


def load_factors(path):
    """Read EG rows from targets.yaml's economic_gate_factors with a line scanner (no PyYAML needed)."""
    out, section, cur = {}, None, None
    with open(path) as f:
        for line in f:
            line = line.split("#", 1)[0].rstrip()
            if re.match(r"^\S", line):
                section, cur = line.rstrip(":"), None
            elif section == "economic_gate_factors":
                m = re.match(r"^\s*-\s*id:\s*(EG\d)\s*$", line)
                if m:
                    cur = out.setdefault(m.group(1), {})
                    continue
                m = re.match(r"^\s+(target_value|second_value):\s*([0-9.]+)\s*$", line)
                if m and cur is not None:
                    cur[m.group(1)] = float(m.group(2))
    return out


def apply_factors(path):
    global EG1_X, EG2_X, EG3_FIRST_X, EG3_SECOND_X, EG4_X, EG5_LIMIT
    if not os.path.exists(path):
        return
    f = load_factors(path)
    EG1_X = f.get("EG1", {}).get("target_value", EG1_X)
    EG2_X = f.get("EG2", {}).get("target_value", EG2_X)
    EG3_FIRST_X = f.get("EG3", {}).get("target_value", EG3_FIRST_X)
    EG3_SECOND_X = f.get("EG3", {}).get("second_value", EG3_SECOND_X)
    EG4_X = f.get("EG4", {}).get("target_value", EG4_X)
    EG5_LIMIT = f.get("EG5", {}).get("target_value", EG5_LIMIT)


def num(d, k):
    v = d.get(k)
    return float(v) if isinstance(v, (int, float)) and not isinstance(v, bool) else None


def _result(cid, status, detail):
    return {"id": cid, "status": status, "detail": detail}


def _need(cid, missing):
    return _result(cid, UNMEASURED, "missing input(s): " + ", ".join(missing))


def eg1(m, i):
    b, s, u = num(m, "ci_p50_s"), num(i, "s10_second_run_job_s"), num(i, "unconverted_jobs_s")
    miss = [n for n, v in (("baseline ci_p50_s", b), ("S10 s10_second_run_job_s", s), ("unconverted_jobs_s", u)) if v is None]
    if miss:
        return _need("EG1", miss)
    proj, lim = s + u, EG1_X * b
    return _result("EG1", "TRIP" if proj > lim else "CLEAR", "projected ttg p50 %.0f s vs limit %.0f s (1.10 x %.0f)" % (proj, lim, b))


def eg2(m, i):
    b, c = num(m, "ci_p50_s"), num(i, "s10_cold_run_s")
    miss = [n for n, v in (("baseline ci_p50_s", b), ("S10 s10_cold_run_s", c)) if v is None]
    if miss:
        return _need("EG2", miss)
    lim = EG2_X * b
    return _result("EG2", "TRIP" if c > lim else "CLEAR", "cold CI %.0f s vs limit %.0f s (1.5 x %.0f)" % (c, lim, b))


def eg3(m, i):
    make = num(i, "make_cold_median_s") or num(m, "cold_worktree_wall_s")
    first, second = num(i, "s6_first_cold_s"), num(i, "s6_second_cold_s")
    miss = [n for n, v in (("make cold median (baseline-worktree.json)", make), ("S6 s6_first_cold_s", first), ("S6 s6_second_cold_s", second)) if v is None]
    if miss:
        return _need("EG3", miss)
    l1, l2 = EG3_FIRST_X * make, EG3_SECOND_X * (0.60 * make)
    trips = []
    if first > l1:
        trips.append("first cold %.0f > %.0f" % (first, l1))
    if second > l2:
        trips.append("second cold %.0f > %.0f" % (second, l2))
    return _result("EG3", "TRIP" if trips else "CLEAR", "; ".join(trips) or "first %.0f <= %.0f, second %.0f <= %.0f" % (first, l1, second, l2))


def eg4(m, i):
    c = num(i, "recosted_cu_m")
    if c is None:
        return _need("EG4", ["recosted_cu_m (CU total re-costed with chosen fallback paths)"])
    lim = EG4_X * VERIFIED_CU_M
    return _result("EG4", "TRIP" if c > lim else "CLEAR", "re-costed %.2fM CU vs limit %.2fM (1.25 x %.2fM)" % (c, lim, VERIFIED_CU_M))


RA2_PROJECTED_LIMIT = 0.90  # plan Task 0.14e: post-selection RA2 projection limit (ADR-009)


def selected_share(i, spike_results=None):
    """e2e_selected_share_median from the inputs file, else spike-results.yaml (spike S14); None when S14 has no number."""
    v = num(i, "e2e_selected_share_median")
    if v is not None:
        return v
    path = spike_results or os.path.join(IMPL, "spike-results.yaml")
    if os.path.exists(path):
        with open(path) as f:
            for line in f:
                m = re.match(r"^\s*e2e_selected_share_median:\s*([0-9.]+)\s*$", line)
                if m:
                    return float(m.group(1))
    return None


def eg5(m, i):
    ra1, ra2 = num(m, "ra1_median_fanout"), num(m, "ra2_non_go_share")
    miss = [n for n, v in (("ra1_median_fanout", ra1), ("ra2_non_go_share", ra2)) if v is None]
    if miss:
        return _need("EG5", miss)
    trips = []
    if ra1 > EG5_LIMIT:
        trips.append("RA1 %.3f > %.2f" % (ra1, EG5_LIMIT))
    comp = m.get("ra2_median_components_s")
    span = num(comp, "span") if isinstance(comp, dict) else None
    parts = [num(comp, k) for k in ("setup_and_queue", "lint", "go_or_other", "e2e")] if span else []
    if span and all(p is not None for p in parts):
        # ADR-009 / Task 0.14e: RA2 on the post-selection projection. Unknown s is 1.0 (never assumed better).
        setup, lint, go, e2e = parts
        s_known = selected_share(i)
        s = 1.0 if s_known is None else s_known
        residual, projected = (setup + lint + go) / span, (setup + lint + go + e2e * s) / span
        if residual > EG5_LIMIT:
            trips.append("RA2 residual %.3f > %.2f" % (residual, EG5_LIMIT))
        if projected > RA2_PROJECTED_LIMIT:
            trips.append("RA2 projected %.3f > %.2f (s=%.2f%s)" % (projected, RA2_PROJECTED_LIMIT, s, "" if s_known is not None else ", S14 undecided"))
        detail = "RA1 %.3f; RA2 residual %.3f, projected %.3f at s=%.2f" % (ra1, residual, projected, s)
    else:
        if ra2 > EG5_LIMIT:
            trips.append("RA2 %.3f > %.2f" % (ra2, EG5_LIMIT))
        detail = "RA1 %.3f, RA2 %.3f both <= %.2f" % (ra1, ra2, EG5_LIMIT)
    return _result("EG5", "TRIP" if trips else "CLEAR", "; ".join(trips) or detail)


def mid_t1(m, i):
    r, ra1 = num(i, "mid_t1_ratio"), num(m, "ra1_median_fanout")
    if r is None or ra1 is None:
        return _need("T1", ["mid_t1_ratio (replay)"] if r is None else ["ra1_median_fanout"])
    lim = min(0.30, 1.5 * ra1)
    return _result("T1", "TRIP" if r > lim else "CLEAR", "affected ratio %.3f vs target %.3f" % (r, lim))


def mid_t3(m, i):
    s, f = num(i, "mid_second_cold_s"), num(i, "mid_first_cold_s")
    if s is None or f is None:
        return _need("T3", [k for k, v in (("mid_second_cold_s", s), ("mid_first_cold_s", f)) if v is None])
    return _result("T3", "TRIP" if s > 0.25 * f else "CLEAR", "second %.0f s vs 0.25 x first = %.0f s" % (s, 0.25 * f))


def mid_t4(m, i):
    e, b = num(i, "mid_edit_test_s"), num(i, "baseline_edit_test_s") or num(m, "warm_one_pkg_test_s")
    if e is None or b is None:
        return _need("T4", [k for k, v in (("mid_edit_test_s", e), ("baseline warm one-package test", b)) if v is None])
    lim = max(1.25 * b, b + 5)
    return _result("T4", "TRIP" if e > lim else "CLEAR", "latency %.1f s vs limit %.1f s" % (e, lim))


CRITERION_INPUTS = {
    "EG1": ("s10_second_run_job_s", "unconverted_jobs_s"), "EG2": ("s10_cold_run_s",),
    "EG3": ("make_cold_median_s", "s6_first_cold_s", "s6_second_cold_s"), "EG4": ("recosted_cu_m",),
    "EG5": ("e2e_selected_share_median",), "T1": ("mid_t1_ratio",),
    "T3": ("mid_second_cold_s", "mid_first_cold_s"), "T4": ("mid_edit_test_s", "baseline_edit_test_s"),
}


def noisy_keys(i, cid):
    """Inputs of criterion cid that the inputs file marks as measured under load."""
    side = i.get("noisy_inputs")
    side = side if isinstance(side, dict) else {}
    return [k for k in CRITERION_INPUTS.get(cid, ()) if side.get(k)]


def evaluate(m, i, midpoint=False):
    fns = (mid_t1, mid_t3, mid_t4) if midpoint else (eg1, eg2, eg3, eg4, eg5)
    return [f(m, i) for f in fns]


def verdict(results):
    """Return (exit_code, headline). A trip is decisive even if others are unmeasured."""
    tripped = [r["id"] for r in results if r["status"] == "TRIP"]
    unmeasured = [r["id"] for r in results if r["status"] == UNMEASURED]
    if tripped:
        return 3, "ECONOMIC GATE: OWNER DECISION " + ", ".join(tripped)
    if unmeasured:
        return 4, "ECONOMIC GATE: INCOMPLETE (UNMEASURED " + ", ".join(unmeasured) + "); not CLEAR"
    return 0, "ECONOMIC GATE: CLEAR"


def apply_decision(code, results, dec):
    """Map the plain verdict code through a recorded owner decision; returns (code, headline or None)."""
    tripped = [r["id"] for r in results if r["status"] == "TRIP"]
    if code != 3 or dec is None:
        return code, None
    label = "TRIPPED (%s) - owner decision recorded: %s (%s)" % (", ".join(tripped), dec["decision"], dec["date"])
    if dec["decision"] == "continue":
        if not set(tripped) <= dec["covers"]:
            return 3, None
        unmeasured = [r["id"] for r in results if r["status"] == UNMEASURED]
        if unmeasured:
            return 4, "ECONOMIC GATE: " + label + "; INCOMPLETE (UNMEASURED " + ", ".join(unmeasured) + "); not CLEAR"
        return 0, "ECONOMIC GATE: " + label
    if dec["decision"] in DECISION_EXIT:
        return DECISION_EXIT[dec["decision"]], "ECONOMIC GATE: " + label
    return 3, None


def load_json(path, required):
    if not os.path.exists(path):
        if required:
            raise SystemExit("ERROR: %s not found" % path)
        return {}
    with open(path) as f:
        return json.load(f)


def main(argv=None):
    ap = argparse.ArgumentParser(description=__doc__.split("\n")[0])
    ap.add_argument("--midpoint", action="store_true", help="evaluate T1, T3, T4 (Story 2.7)")
    ap.add_argument("--measured", default=MEASURED)
    ap.add_argument("--targets", default=TARGETS)
    ap.add_argument("--attestations", default=ATTESTATIONS)
    ap.add_argument("--inputs", default=os.path.join(IMPL, "spike-measurements.json"))
    a = ap.parse_args(argv)
    apply_factors(a.targets)
    m = load_json(a.measured, True)
    i = load_json(a.inputs, False)
    results = evaluate(m, i, a.midpoint)
    noisy = {}
    for r in results:
        keys = noisy_keys(i, r["id"]) if r["status"] != UNMEASURED else []
        if keys:
            noisy[r["id"]] = keys
        print("  %-3s %-10s %s%s" % (r["id"], r["status"], r["detail"], "  [NOISY: measured under load, owner-authorised; inputs " + ", ".join(keys) + "]" if keys else ""))
    evaluable = [r["id"] for r in results if r["status"] != UNMEASURED]
    unmeasured = [r["id"] for r in results if r["status"] == UNMEASURED]
    if noisy:
        print("NOISY: %d of %d evaluated criteria derive from inputs measured under load (%s); not quiet-host numbers" % (len(noisy), len(evaluable), ", ".join(sorted(noisy))))
    print("evaluable: %s; unmeasured: %s" % (", ".join(evaluable) or "none", ", ".join(unmeasured) or "none"))
    code, headline = verdict(results)
    if not a.midpoint:
        code, decided = apply_decision(code, results, load_decision(a.attestations))
        headline = decided or headline
    print(headline)
    if code == 3:
        print("Epics 1-11 stay unreleased until owner-attestations.yaml records economic_gate: {decision, date, reason}; this script never selects a decision.")
    return code


if __name__ == "__main__":
    sys.exit(main())
