import importlib.util
import pathlib
import sys
import unittest
from datetime import datetime, timedelta, timezone

_spec = importlib.util.spec_from_file_location(
    "prune", pathlib.Path(__file__).with_name("prune-actions-caches.py"))
prune = importlib.util.module_from_spec(_spec)
sys.modules["prune"] = prune
_spec.loader.exec_module(prune)

NOW = datetime(2026, 11, 10, tzinfo=timezone.utc)
MAIN = "refs/heads/main"
_ids = iter(range(1, 1000))


def cache(key, ref=MAIN, mb=100, age_days=0, used_days=0):
    return prune.Cache(next(_ids), key, ref, int(mb * 1e6),
                       NOW - timedelta(days=age_days), NOW - timedelta(days=used_days))


def run(caches, closed=()):
    return {c.id: why for c, why in prune.plan(caches, NOW, MAIN, lambda n: n in closed)}


class PruneTest(unittest.TestCase):
    def test_family_strips_hash_date_and_run_id(self):
        f = prune.family_of
        self.assertEqual(f("go-test-ubuntu24-x64-20261109"), "go-test-ubuntu24-x64")
        self.assertEqual(f("bun-Linux-" + "ab" * 32), "bun-Linux")
        self.assertEqual(f("setup-go-Linux-x64-ubuntu24-go-1.26.6-" + "f8" * 32),
                         "setup-go-Linux-x64-ubuntu24-go-1.26.6")
        self.assertEqual(f("bench-tier1-42587397140"), "bench-tier1")
        self.assertEqual(f("tmux-3.8-Linux-v1"), "tmux-3.8-Linux-v1")

    def test_keeps_newest_two_per_family_and_never_the_newest(self):
        rows = [cache(f"go-test-ubuntu24-x64-2026110{d}", age_days=10 - d) for d in range(1, 6)]
        doomed = run(rows)
        self.assertEqual(sorted(doomed), [rows[0].id, rows[1].id, rows[2].id])
        self.assertNotIn(rows[4].id, doomed)

    def test_closed_pr_and_stale_pr_entries_go_but_open_fresh_ones_stay(self):
        closed = cache("setup-go-x", ref="refs/pull/7/merge", mb=1100)
        stale = cache("go-test-a", ref="refs/pull/8/merge", used_days=4)
        fresh = cache("go-test-b", ref="refs/pull/9/merge", used_days=1)
        doomed = run([closed, stale, fresh], closed={7})
        self.assertEqual(set(doomed), {closed.id, stale.id})

    def test_size_cap_drops_oldest_used_but_spares_newest_main_and_bench(self):
        newest = cache("go-test-ubuntu24-x64-20261109", mb=5000)
        older = cache("go-test-ubuntu24-x64-20261108", mb=5000, age_days=1, used_days=1)
        bench = cache("bench-tier1-42", mb=5000)
        doomed = run([newest, older, bench])
        self.assertEqual(set(doomed), {older.id})

    def test_empty(self):
        self.assertEqual(run([]), {})


if __name__ == "__main__":
    unittest.main()
