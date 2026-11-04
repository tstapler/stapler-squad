import contextlib
import importlib.util
import io
import json
import os
import shutil
import tempfile
import unittest

HERE = os.path.dirname(os.path.abspath(__file__))
spec = importlib.util.spec_from_file_location("eg", os.path.join(HERE, "economic-gate.py"))
eg = importlib.util.module_from_spec(spec)
spec.loader.exec_module(eg)

MEASURED = {"ci_p50_s": 2339, "ra1_median_fanout": 0.259, "ra2_non_go_share": 0.30}
CLEAR = {"s10_second_run_job_s": 1000, "unconverted_jobs_s": 1000, "s10_cold_run_s": 3000,
         "make_cold_median_s": 600, "s6_first_cold_s": 900, "s6_second_cold_s": 300, "recosted_cu_m": 31.0}


NO_ATT = ("--attestations", os.devnull)


def run(measured, inputs, *args):
    if "--attestations" not in args:
        args = args + NO_ATT
    with tempfile.TemporaryDirectory() as d:
        mp, ip = os.path.join(d, "m.json"), os.path.join(d, "i.json")
        json.dump(measured, open(mp, "w"))
        json.dump(inputs, open(ip, "w"))
        out = io.StringIO()
        with contextlib.redirect_stdout(out):
            code = eg.main(["--measured", mp, "--inputs", ip, *args])
        return code, out.getvalue()


class Factors(unittest.TestCase):
    def test_repo_targets_yaml_has_proposal_factors(self):
        f = eg.load_factors(eg.TARGETS)
        self.assertEqual(f["EG1"]["target_value"], 1.10)
        self.assertEqual(f["EG2"]["target_value"], 1.5)
        self.assertEqual((f["EG3"]["target_value"], f["EG3"]["second_value"]), (3.0, 2.0))
        self.assertEqual(f["EG4"]["target_value"], 1.25)
        self.assertEqual(f["EG5"]["target_value"], 0.60)

    def test_targets_override_factor(self):
        with tempfile.TemporaryDirectory() as d:
            tp = os.path.join(d, "t.yaml")
            open(tp, "w").write("economic_gate_factors:\n  - id: EG2\n    target_value: 1.0\n")
            old = eg.EG2_X
            try:
                eg.apply_factors(tp)
                self.assertEqual(eg.EG2_X, 1.0)
            finally:
                eg.EG2_X = old

    def test_never_writes_attestations(self):
        path = os.path.join(eg.IMPL, "owner-attestations.yaml")
        before = open(path).read()
        run(MEASURED, CLEAR)
        self.assertEqual(open(path).read(), before)


class OwnerDecision(unittest.TestCase):
    TRIP = dict(CLEAR)
    M = dict(MEASURED, ra2_non_go_share=0.987)

    def att(self, decision, extra="", reason="EG5 trips (RA2)."):
        d = tempfile.mkdtemp()
        self.addCleanup(shutil.rmtree, d, True)
        p = os.path.join(d, "a.yaml")
        open(p, "w").write("interview_date: x\neconomic_gate:\n  decision: %s\n  date: 2026-11-03\n%s  reason: >-\n    %s\n    more text\n  status: recorded\nother:\n  decision: re-plan\n" % (decision, extra, reason))
        return ["--attestations", p]

    def test_continue_covers_tripped(self):
        code, out = run(self.M, self.TRIP, *self.att("continue"))
        self.assertEqual(code, 0)
        self.assertEqual(out.splitlines()[-1], "ECONOMIC GATE: TRIPPED (EG5) - owner decision recorded: continue (2026-11-03)")

    def test_continue_does_not_cover_other_trip(self):
        code, out = run(self.M, dict(self.TRIP, recosted_cu_m=39.0), *self.att("continue"))
        self.assertEqual((code, "OWNER DECISION EG4, EG5" in out), (3, True))

    def test_criteria_list_overrides_reason(self):
        code, _ = run(self.M, self.TRIP, *self.att("continue", extra="  criteria: [EG4]\n"))
        self.assertEqual(code, 3)

    def test_continue_never_clears_unmeasured(self):
        code, out = run(self.M, {}, *self.att("continue"))
        self.assertEqual(code, 4)
        self.assertIn("not CLEAR", out.splitlines()[-1])

    def test_other_decisions_distinct_codes(self):
        codes = {d: run(self.M, self.TRIP, *self.att(d))[0] for d in ("re-plan", "pause", "revert")}
        self.assertEqual(codes, {"re-plan": 5, "pause": 6, "revert": 7})

    def test_no_decision_keeps_exit_3(self):
        self.assertEqual(run(self.M, self.TRIP)[0], 3)

    def test_midpoint_ignores_decision(self):
        code, _ = run(MEASURED, {}, "--midpoint", *self.att("continue"))
        self.assertEqual(code, 4)

    def test_repo_attestation_parses(self):
        d = eg.load_decision(eg.ATTESTATIONS)
        self.assertEqual((d["decision"], d["date"]), ("continue", "2026-11-03"))
        self.assertIn("EG5", d["covers"])


class EconomicGate(unittest.TestCase):
    def test_all_clear(self):
        code, out = run(MEASURED, CLEAR)
        self.assertEqual(code, 0)
        self.assertIn("ECONOMIC GATE: CLEAR", out)

    def trip(self, **over):
        return run(dict(MEASURED, **over.pop("m", {})), dict(CLEAR, **over))

    def test_eg1(self):
        code, out = self.trip(s10_second_run_job_s=2000)
        self.assertEqual(code, 3)
        self.assertIn("OWNER DECISION EG1", out)

    def test_eg2(self):
        code, out = self.trip(s10_cold_run_s=4000)
        self.assertEqual((code, "OWNER DECISION EG2" in out), (3, True))

    def test_eg3_first(self):
        code, out = self.trip(s6_first_cold_s=1900)
        self.assertEqual((code, "OWNER DECISION EG3" in out), (3, True))

    def test_eg3_second(self):
        code, out = self.trip(s6_second_cold_s=800)
        self.assertEqual((code, "OWNER DECISION EG3" in out), (3, True))

    def test_eg4(self):
        code, out = self.trip(recosted_cu_m=39.0)
        self.assertEqual((code, "OWNER DECISION EG4" in out), (3, True))

    def test_eg5_ra2(self):
        code, out = self.trip(m={"ra2_non_go_share": 0.987})
        self.assertEqual((code, "OWNER DECISION EG5" in out), (3, True))

    def test_eg5_ra1(self):
        code, out = self.trip(m={"ra1_median_fanout": 0.7})
        self.assertEqual((code, "OWNER DECISION EG5" in out), (3, True))

    def test_unmeasured_is_not_clear(self):
        code, out = run(MEASURED, {})
        self.assertEqual(code, 4)
        self.assertNotIn("CLEAR\n", out.splitlines()[-1])
        self.assertIn("UNMEASURED EG1, EG2, EG3, EG4", out)
        self.assertIn("evaluable: EG5", out)

    def test_trip_beats_unmeasured(self):
        code, _ = run(dict(MEASURED, ra2_non_go_share=0.9), {})
        self.assertEqual(code, 3)

    def test_midpoint(self):
        ok = {"mid_t1_ratio": 0.2, "mid_second_cold_s": 100, "mid_first_cold_s": 600,
              "mid_edit_test_s": 5, "baseline_edit_test_s": 4}
        self.assertEqual(run(MEASURED, ok, "--midpoint")[0], 0)
        self.assertEqual(run(MEASURED, dict(ok, mid_t1_ratio=0.5), "--midpoint")[0], 3)
        self.assertEqual(run(MEASURED, dict(ok, mid_second_cold_s=300), "--midpoint")[0], 3)
        self.assertEqual(run(MEASURED, dict(ok, mid_edit_test_s=20), "--midpoint")[0], 3)
        self.assertEqual(run(MEASURED, {}, "--midpoint")[0], 4)


if __name__ == "__main__":
    unittest.main()
