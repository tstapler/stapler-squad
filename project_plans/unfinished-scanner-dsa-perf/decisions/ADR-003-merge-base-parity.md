# ADR-003: Merge base is git-style paint-down-to-common, matching the `object.Commit.MergeBase` oracle; the lockstep walk is retained only as the `plainWalk` fallback

**Status**: Accepted (plan phase; amended in repair iteration 1; `plainWalk`, the lockstep walk, is retained in production as the permanent fallback, Phase 4 patch); amended 2026-10-16 (owner Q3): paint-down is the only production walk, no flag · **Plan**: Stories 0.0.1, 0.1.3a, 2.1.3 (2.2.3 superseded)

## Context
`findMergeBase` returns the first common ancestor met by a lockstep BFS capped at `mergeBaseBFSLimit`, which is not git's best merge base on criss-cross histories (research/architecture.md R1/R2). **VERIFIED: the lockstep + `parentMemo` version is an uncommitted working-tree edit; the committed (and deployed, `39fa33a11`) walk is the old stack-based mark-then-walk** (`git diff HEAD` shows the lockstep as additions; the old functions are at `39fa33a11:session/unfinished/gogit_vcs_reader.go:2194,2255`). `object.Commit.MergeBase` (`plumbing/object/merge_base.go:17`) returns the best set and is available as an oracle.

## Decision
- `findMergeBase` is git-style paint-down-to-common (commit `fef7bb90f`: newest-first heap, FIFO ties, stale propagation), matching `object.Commit.MergeBase` on 100% of 600 single-merge and general cases. It is not required to reproduce the old lockstep's first-common-ancestor result; the oracle is the best merge base, not the previous walk.
- The id-graph walk (Story 2.1.3) and the plain `parentMemo` walk (`plainWalk`, the permanent fallback on a full arena, severe pressure or a failing storer) must agree with each other and with the oracle.
- Story 0.0.1 still commits the lockstep walk and retains the committed old walk (`39fa33a11`) as a test-only oracle (`legacyStack*`); the recorded divergence set (`session/unfinished/testdata/mergebase_lockstep_divergences.golden`, below) documents how the old and lockstep walks differed and is historical.
- `MergeBaseMode`, the `best_merge_base` flag, the generation column and Story 2.2.3 are dropped: there is one walk.
- BFS (not DFS) bounds the "within L per side" failure condition.

## Consequences
One algorithm in production, plus the plain fallback that returns the same answer. Ahead/behind can differ from the pre-lockstep deployed walk on merge-rich histories (the intended correction toward the best merge base); counts stay `min(exact, L)`.

## Behaviour change introduced by the lockstep commit
Measured by `mergebase_legacy_test.go` (old committed walk `39fa33a11` vs lockstep, same `mergeBaseBFSLimit`). Contradicts the Story 0.0.1 AC (a) as written: the lockstep is **not** identical to the old walk on every single-merge DAG.

- Identical (test-enforced, 600 DAGs): linear fork shapes and merge-free trees.
- **Diverges on single-merge DAGs**: 5 of 400 seeded (head, base) pairs. Cause: the old walk returns the first commit in a BFS from `base` that is an ancestor of `head` (so `base` itself when `base` is an ancestor of `head`); lockstep returns the first commit *both* frontiers have seen, which can be an older common ancestor when `head` is a merge whose first parent is old (example: head = merge(0,1), base = 1: old returns 1, lockstep returns 0, giving ahead/behind 2/1 instead of 1/0).
- Diverges on merge-rich general DAGs: 88 of 400 evaluated (200 DAGs x caps 2000 and 4), plus 41 of 400 on their shallow-truncated variants. Named criss-cross, octopus, shallow and cap-linear fixtures did not diverge.
- Against `object.Commit.MergeBase` (best set) on synthetic DAGs at the default cap: single-merge legacy-in-best 393/400, lockstep 392/400; general legacy 157/200, lockstep 148/200.
- Real repositories (`MERGEBASE_REAL_REPOS=1`, 13 repos, 55 pairs): 44 identical, 8 divergent or error-mismatched; vs best set over 52 pairs: legacy 48, lockstep 46.

The golden file lists every divergent case (one line each, old vs lockstep result as node indexes). Owner decision pending: accept as is, or restore "nearest to base" semantics in the lockstep before Story 2.1.3 ports it to interned ids.
