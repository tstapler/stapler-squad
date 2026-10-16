# Baseline: unfinished-scanner-dsa-perf

Measurements that later stories compare against. Appended to by Stories 0.0.3 and 0.1.1.

## Revisions

- Live baseline (deployed binary): `39fa33a11`, old stack-based `findMergeBase`/`countCommitsTo`.
- Lockstep `findMergeBase` + `parentMemo`: landed in commit `f0a75ac10` (after `39fa33a11`).

## Lockstep-only row (Story 0.0.1)

Cold, `W=1`, 2,500-commit linear go-git history (fork of 3 commits, 5 behind), in-memory storer wrapped by a counting storer; `findMergeBase` + two `countCommitsTo` per run. Test: `TestFindMergeBase_ShouldDecodeFewerThanLegacyStack_WhenCold2500Linear` (`session/unfinished/mergebase_decodecount_test.go`).

| Walk | DecodeCount | allocs/op |
|---|---|---|
| old committed walk (`legacyStack*`) | 2013 | 50,354 |
| lockstep | 11 | 305 |

This gain belongs to the lockstep commit, not to this project; the 24 h profile (decode = 51.9% of `alloc_space`) was taken on the old walk.

## Differential: old committed walk vs lockstep (Story 0.0.1)

- Identical on 600 seeded DAGs (300 linear forks, 300 merge-free trees): enforced by test.
- Divergence exists: yes. Golden file `session/unfinished/testdata/mergebase_lockstep_divergences.golden` (1213 cases, 134 diverge): single-merge 5 of 400 pairs; general merge-rich DAGs 88 of 400 (caps 2000 and 4); shallow-truncated variants 41 of 400; named criss-cross, octopus, shallow and cap-linear fixtures 0. Details in ADR-003.
- Vs `object.Commit.MergeBase` best set (synthetic, default cap): single-merge legacy 393/400, lockstep 392/400; general legacy 157/200, lockstep 148/200.

## Real-worktree differential (reporting only)

Opt-in: `MERGEBASE_REAL_REPOS=1 go test ./session/unfinished/ -run RealWorktree -v`. Current repo plus `~/code/github.com/tstapler/*` (excluding `stapler-squad-*` sibling clones), 6 refs per repo, read-only via go-git. Run on the machine at commit time: 13 repos, 55 pairs; 44 identical, 8 divergent or error-mismatched (3 pairs had an error on at least one side); vs `object.Commit.MergeBase` over 52 pairs: legacy-in-best 48, lockstep-in-best 46. Not repeatable bit-for-bit (refs change).
