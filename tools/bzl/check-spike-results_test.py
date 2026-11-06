import importlib.util
import contextlib
import io
import os
import tempfile
import unittest

spec = importlib.util.spec_from_file_location(
    "csr", os.path.join(os.path.dirname(os.path.abspath(__file__)), "check-spike-results.py"))
csr = importlib.util.module_from_spec(spec)
spec.loader.exec_module(csr)


def good_rows():
    return [{"spike": s, "verdict": "Pass", "path": "direct", "cost": "S"} for s in csr.REQUIRED]


class CheckSpikeResults(unittest.TestCase):
    def test_all_valid(self):
        _, errs = csr.check(good_rows())
        self.assertEqual(errs, [])

    def test_missing_id(self):
        _, errs = csr.check(good_rows()[:-1])
        self.assertTrue(any("S13" in e for e in errs))

    def test_unknown_id_rejected(self):
        rows = good_rows() + [{"spike": "S99", "verdict": "Pass"}]
        _, errs = csr.check(rows)
        self.assertTrue(any("unknown" in e for e in errs))

    def test_duplicate_rejected(self):
        _, errs = csr.check(good_rows() + [good_rows()[0]])
        self.assertTrue(any("duplicate" in e for e in errs))

    def test_unknown_verdict_rejected(self):
        rows = good_rows()
        rows[0]["verdict"] = "Maybe"
        _, errs = csr.check(rows)
        self.assertTrue(any("invalid verdict" in e for e in errs))

    def test_fallback_needs_path_and_cost(self):
        rows = good_rows()
        rows[1] = {"spike": "S1", "verdict": "FallbackFork"}
        _, errs = csr.check(rows)
        self.assertEqual(len([e for e in errs if e.startswith("S1:")]), 2)

    def test_optional_s14_accepted(self):
        rows = good_rows() + [{"spike": "S14", "verdict": "Pass"}]
        _, errs = csr.check(rows)
        self.assertEqual(errs, [])

    def test_optional_s15_accepted_and_needs_path_when_fallback(self):
        rows = good_rows() + [{"spike": "S15", "verdict": "FallbackWorkaround", "path": "p", "cost": "M"}]
        _, errs = csr.check(rows)
        self.assertEqual(errs, [])
        rows = good_rows() + [{"spike": "S15", "verdict": "FallbackWorkaround"}]
        _, errs = csr.check(rows)
        self.assertEqual(len([e for e in errs if e.startswith("S15:")]), 2)

    def test_optional_s15b_accepted_and_needs_path_when_fallback(self):
        rows = good_rows() + [{"spike": "S15b", "verdict": "FallbackWorkaround", "path": "p", "cost": "M"}]
        _, errs = csr.check(rows)
        self.assertEqual(errs, [])
        rows = good_rows() + [{"spike": "S15b", "verdict": "FallbackWorkaround"}]
        _, errs = csr.check(rows)
        self.assertEqual(len([e for e in errs if e.startswith("S15b:")]), 2)

    def test_parse_quotes_and_comments(self):
        rows = csr.parse_rows('# c\n- spike: S1\n  verdict: Pass # x\n  path: "a: b"\n  cost: M\n')
        self.assertEqual(rows, [{"spike": "S1", "verdict": "Pass", "path": "a: b", "cost": "M"}])

    def test_owner_deferral_is_display_only(self):
        with tempfile.TemporaryDirectory() as d:
            open(os.path.join(d, "owner-attestations.yaml"), "w").write(
                "interview_date: 2026-11-03\nremote_cache_s10:\n  decision: start with disk cache only; defer the onyx remote cache\n"
                "  pending: S10 and S10-redaction have no verdict; EG1 measured locally\n")
            rows = [r for r in good_rows() if r["spike"] not in ("S10", "S10-redaction")]
            rows = [dict(r, verdict="Fail") if r["spike"] == "S13" else r for r in rows]
            res = os.path.join(d, "spike-results.yaml")
            open(res, "w").write("".join("- spike: %s\n  verdict: %s\n  path: p\n  cost: S\n" % (r["spike"], r["verdict"]) for r in rows))
            out = io.StringIO()
            with contextlib.redirect_stdout(out):
                code = csr.main(["x", res])
            text = out.getvalue()
            self.assertIn("owner-deferred: S10, S10-redaction (disk cache first, 2026-11-03)", text)
            self.assertIn("MISSING required ids: S10, S10-redaction", text)
            self.assertIn("BLOCKING (verdict Fail): S13", text)
            self.assertEqual(code, 1)

    def test_no_attestations_no_deferral_line(self):
        self.assertIsNone(csr.owner_deferrals("/nonexistent/owner-attestations.yaml"))

    def test_owner_accepted_fail_is_display_only(self):
        with tempfile.TemporaryDirectory() as d:
            open(os.path.join(d, "owner-attestations.yaml"), "w").write(
                "adr_009_e2e:\n  decision: Full e2e run\n  date: 2026-11-05\n  status: recorded\n  spikes: [S14]\n")
            rows = [dict(r, verdict="Fail") if r["spike"] == "S13" else r for r in good_rows()]
            rows.append({"spike": "S14", "verdict": "Fail", "path": "p", "cost": "M"})
            res = os.path.join(d, "spike-results.yaml")
            open(res, "w").write("".join("- spike: %s\n  verdict: %s\n  path: p\n  cost: S\n" % (r["spike"], r["verdict"]) for r in rows))
            out = io.StringIO()
            with contextlib.redirect_stdout(out):
                code = csr.main(["x", res])
            text = out.getvalue()
            self.assertIn("OWNER-ACCEPTED Fail (verdict stays Fail): S14 (Full e2e run, 2026-11-05)", text)
            self.assertIn("BLOCKING (verdict Fail): S13", text)
            self.assertIn("S14            Fail", text)
            self.assertEqual(code, 0)

    def test_owner_accepted_fail_requires_recorded_status(self):
        with tempfile.TemporaryDirectory() as d:
            p = os.path.join(d, "owner-attestations.yaml")
            open(p, "w").write("adr_009_e2e:\n  decision: Full e2e run\n  spikes: [S14]\n")
            self.assertIsNone(csr.owner_accepted_fails(p))
            self.assertIsNone(csr.owner_accepted_fails(os.path.join(d, "missing.yaml")))


class Noisy(unittest.TestCase):
    def run_main(self, rows_text, measurements):
        import json
        with tempfile.TemporaryDirectory() as d:
            p = os.path.join(d, "spike-results.yaml")
            open(p, "w").write(rows_text)
            if measurements is not None:
                json.dump(measurements, open(os.path.join(d, "spike-measurements.json"), "w"))
            out = io.StringIO()
            with contextlib.redirect_stdout(out):
                code = csr.main(["x", p])
            return code, out.getvalue()

    def rows(self, noisy):
        return "".join("- spike: %s\n  verdict: Pass\n%s" % (s, "  noisy: true\n" if noisy and s == "S6" else "") for s in csr.REQUIRED)

    def test_noisy_row_and_inputs_are_tagged_but_verdict_unchanged(self):
        code, out = self.run_main(self.rows(True), {"noisy_inputs": {"mid_edit_test_s": {"not_quiet": True}}})
        self.assertEqual(code, 0)
        self.assertIn("S6", [ln for ln in out.splitlines() if "[NOISY" in ln][0])
        self.assertIn("NOISY: 1 verdict row(s) (S6) and 1 measurement input(s) (mid_edit_test_s)", out)

    def test_clean_run_prints_nothing_noisy(self):
        _, out = self.run_main(self.rows(False), {"mid_t1_ratio": 0.01})
        self.assertNotIn("NOISY", out)


if __name__ == "__main__":
    unittest.main()
