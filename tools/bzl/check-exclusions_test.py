import importlib.util
import os
import unittest

spec = importlib.util.spec_from_file_location("ce", os.path.join(os.path.dirname(os.path.abspath(__file__)), "check-exclusions.py"))
ce = importlib.util.module_from_spec(spec)
spec.loader.exec_module(ce)

GOOD = """# header
exclusions:
  - target: //a/b:b_test
    sandbox_class: cwd
    owner: tstapler
    issue: https://github.com/tstapler/stapler-squad/issues/1421 # note
    reason: >-
      first line
      second line.
  - target: //c:c_test
    sandbox_class: none # not a sandbox failure
    owner: tstapler
    issue: https://github.com/tstapler/stapler-squad/issues/1422
    reason: >-
      one.
"""


class CheckExclusions(unittest.TestCase):
    def test_parses_folded_reason_and_trailing_comments(self):
        e = ce.parse(GOOD)
        self.assertEqual(len(e), 2)
        self.assertEqual(e[0]["reason"], "first line second line.")
        self.assertEqual(e[0]["issue"], "https://github.com/tstapler/stapler-squad/issues/1421")
        self.assertEqual(e[1]["sandbox_class"], "none")
        self.assertEqual(ce.validate(e), [])

    def test_missing_owner_and_issue_name_the_entry(self):
        bad = GOOD.replace("    owner: tstapler\n", "", 1).replace("issues/1422", "pull/1422")
        problems = ce.validate(ce.parse(bad))
        self.assertTrue(any("//a/b:b_test: missing owner" in p for p in problems))
        self.assertTrue(any("//c:c_test: issue must be" in p for p in problems))

    def test_unknown_class_and_duplicate_target(self):
        bad = GOOD.replace("sandbox_class: cwd", "sandbox_class: bogus") + GOOD.split("exclusions:\n")[1].split("  - target: //c")[0]
        problems = ce.validate(ce.parse(bad))
        self.assertTrue(any("bogus" in p for p in problems))
        self.assertTrue(any("duplicate target" in p for p in problems))

    def test_packages_counts_distinct(self):
        self.assertEqual(ce.packages(ce.parse(GOOD)), {"//a/b", "//c"})

    def test_real_file_parses_and_validates(self):
        root = os.path.abspath(os.path.join(os.path.dirname(__file__), "..", ".."))
        with open(os.path.join(root, ce.DEFAULT_FILE)) as f:
            self.assertEqual(ce.validate(ce.parse(f.read())), [])

    def test_ratchet_fails_on_growth_and_allows_shrink(self):
        import tempfile
        with tempfile.NamedTemporaryFile("w", suffix=".yaml", delete=False) as f:
            f.write(GOOD)
        self.addCleanup(os.unlink, f.name)
        orig = ce.base_count
        self.addCleanup(setattr, ce, "base_count", orig)
        ce.base_count = lambda path, ref: 1
        self.assertEqual(ce.main(["--file", f.name]), 1)
        ce.base_count = lambda path, ref: 3
        self.assertEqual(ce.main(["--file", f.name]), 0)
        self.assertEqual(ce.main(["--file", f.name, "--max-packages", "1"]), 1)


if __name__ == "__main__":
    unittest.main()
