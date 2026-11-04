import importlib.util
import os
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


if __name__ == "__main__":
    unittest.main()
