# ADR-003: Preserve lockstep first-common-ancestor semantics; the lockstep itself is a commit with a recorded divergence set; best-merge-base is a flagged behaviour change

**Status**: Accepted (plan phase; amended in repair iteration 1; `plainWalk`, the lockstep walk, is retained in production as the permanent fallback, Phase 4 patch); flip decision open (owner, plan Q3) · **Plan**: Stories 0.0.1, 0.1.3a, 2.1.3, 2.2.3

## Context
`findMergeBase` returns the first common ancestor met by a lockstep BFS capped at `mergeBaseBFSLimit`, which is not git's best merge base on criss-cross histories (research/architecture.md R1/R2). **VERIFIED: the lockstep + `parentMemo` version is an uncommitted working-tree edit; the committed (and deployed, `39fa33a11`) walk is the old stack-based mark-then-walk** (`git diff HEAD` shows the lockstep as additions; the old functions are at `39fa33a11:session/unfinished/gogit_vcs_reader.go:2194,2255`). `object.Commit.MergeBase` (`plumbing/object/merge_base.go:17`) returns the best set and is available as an oracle.

## Decision
- Story 0.0.1 commits the lockstep walk alone and retains the **committed old walk** verbatim as a test-only oracle (`legacyStack*`). A seeded differential test (>= 500 DAGs plus criss-cross/octopus/shallow) records every divergence old-vs-lockstep as a golden file. Divergences on linear/single-merge DAGs fail; divergences elsewhere are listed here as the behaviour change the lockstep commit introduced and reported to the owner. Golden divergence set: `session/unfinished/testdata/mergebase_lockstep_divergences.golden` (see "Behaviour change introduced by the lockstep commit" below).
- Moving the walk to interned ids (Story 2.1.3) must equal the **lockstep** exactly (queue order, parent order, cap semantics) and may add **no** divergence against the committed-old walk beyond the golden set.
- `MergeBaseMode` is a sum type: `LockstepFirstCommon` (default) and `PaintDownBest` (generation-ordered paint-down, generation computed on demand). `PaintDownBest` ships only behind `best_merge_base` (default off) with a differential test against `object.Commit.MergeBase`; generation 0 disables pruning. Flipping it is an owner decision and a documented user-visible behaviour change; the story is conditional on owner interest.
- BFS (not DFS) stays for the "within L per side" failure condition.

## Consequences
Two algorithms to maintain until the owner decides; counts stay `min(exact, L)` in both modes; the baseline can separate the lockstep's own gain from this project's.

## Behaviour change introduced by the lockstep commit
Measured by `mergebase_legacy_test.go` (old committed walk `39fa33a11` vs lockstep, same `mergeBaseBFSLimit`). Contradicts the Story 0.0.1 AC (a) as written: the lockstep is **not** identical to the old walk on every single-merge DAG.

- Identical (test-enforced, 600 DAGs): linear fork shapes and merge-free trees.
- **Diverges on single-merge DAGs**: 5 of 400 seeded (head, base) pairs. Cause: the old walk returns the first commit in a BFS from `base` that is an ancestor of `head` (so `base` itself when `base` is an ancestor of `head`); lockstep returns the first commit *both* frontiers have seen, which can be an older common ancestor when `head` is a merge whose first parent is old (example: head = merge(0,1), base = 1: old returns 1, lockstep returns 0, giving ahead/behind 2/1 instead of 1/0).
- Diverges on merge-rich general DAGs: 88 of 400 evaluated (200 DAGs x caps 2000 and 4), plus 41 of 400 on their shallow-truncated variants. Named criss-cross, octopus, shallow and cap-linear fixtures did not diverge.
- Against `object.Commit.MergeBase` (best set) on synthetic DAGs at the default cap: single-merge legacy-in-best 393/400, lockstep 392/400; general legacy 157/200, lockstep 148/200.
- Real repositories (`MERGEBASE_REAL_REPOS=1`, 13 repos, 55 pairs): 44 identical, 8 divergent or error-mismatched; vs best set over 52 pairs: legacy 48, lockstep 46.

The golden file lists every divergent case (one line each, old vs lockstep result as node indexes). Owner decision pending: accept as is, or restore "nearest to base" semantics in the lockstep before Story 2.1.3 ports it to interned ids.
