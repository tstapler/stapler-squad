import importlib.util
import json
import os
import tempfile
import unittest

HERE = os.path.dirname(os.path.abspath(__file__))
_spec = importlib.util.spec_from_file_location("census_check", os.path.join(HERE, "census-check.py"))
cc = importlib.util.module_from_spec(_spec)
_spec.loader.exec_module(cc)

XML = """<testsuites>
<testsuite name="s"><testcase classname="p" name="TestOk"/>
<testcase classname="p" name="TestBad"><failure message="boom">open testdata/x: no such file</failure></testcase>
</testsuite></testsuites>"""


def make_logs(root, target_dir, xml):
    d = os.path.join(root, target_dir)
    os.makedirs(d)
    with open(os.path.join(d, "test.xml"), "w") as f:
        f.write(xml)


def census(rows):
    return {"rows": rows, "packages": {"p": {"targets": ["//p:p_test"], "status": {"//p:p_test": "FAILED"}}}}


def row(test, cls):
    return {"test": test, "package": "p", "file": "p/a_test.go", "class": cls, "evidence": ""}


class CensusCheckTest(unittest.TestCase):
    def setUp(self):
        self.tmp = tempfile.TemporaryDirectory()
        make_logs(self.tmp.name, "p/p_test", XML)

    def tearDown(self):
        self.tmp.cleanup()

    def test_complete_census_has_no_problems(self):
        problems, counts = cc.check(census([row("TestBad", "cwd")]), self.tmp.name)
        self.assertEqual(problems, [])
        self.assertEqual(counts["cwd"], 1)

    def test_missing_row_is_reported(self):
        problems, _ = cc.check(census([]), self.tmp.name)
        self.assertTrue(any("1 failing in test.xml, 0 census rows" in p for p in problems))

    def test_unclassified_row_is_reported(self):
        problems, _ = cc.check(census([row("TestBad", "other")]), self.tmp.name)
        self.assertTrue(any("not a SandboxClass" in p for p in problems))

    def test_duplicate_row_is_reported(self):
        problems, _ = cc.check(census([row("TestBad", "cwd"), row("TestBad", "home")]), self.tmp.name)
        self.assertTrue(problems)

    def test_row_for_passing_test_is_reported(self):
        problems, _ = cc.check(census([row("TestBad", "cwd"), row("TestOk", "cwd")]), self.tmp.name)
        self.assertTrue(any("without a failing test.xml case" in p for p in problems))

    def test_select_filters(self):
        rows = [row("A", "cwd"), row("B", "shard-timeout")]
        self.assertEqual(len(cc.select(rows, cls="cwd")), 1)
        self.assertEqual(len(cc.select(rows, residual=True)), 1)
        self.assertEqual(len(cc.select(rows, files={"nope"})), 0)

    def test_main_exit_codes(self):
        path = os.path.join(self.tmp.name, "census.json")
        json.dump(census([row("TestBad", "cwd")]), open(path, "w"))
        self.assertEqual(cc.main(["--census", path, "--testlogs", self.tmp.name]), 0)
        json.dump(census([]), open(path, "w"))
        self.assertEqual(cc.main(["--census", path, "--testlogs", self.tmp.name]), 1)


if __name__ == "__main__":
    unittest.main()
