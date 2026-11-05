# Scoping ledger (Epic 13, ADR-010)

Measured with `tools/bzl/scoping` (`bazel run //tools/bzl/scoping -- t9`), baseline `tools/bzl/scoping/baseline.json`
(tree 145bce76b, history `33f46bb21`, 378 Go-touching changes, 17 PRs). Lib share = mean affected library-LOC share.

| Step | Decision | T9a test share | T9c lib share | Changes with lower lib LOC | `server/services` / `session` LOC | Inversions |
|---|---|---|---|---|---|---|
| baseline | | 0.5536 | 0.3945 | 0 | 93,019 / 92,060 | 12 |
| Tier 0 (inttest moves + `session/internal/peeraddr`) | executed | 0.4464 (predicted 0.449) | 0.3945 | 0 | 93,019 / 92,053 | 0 |
| Tier A `sessionbundle` + `devtools` | executed (passes SC6 via "fewer") | 0.4461 | 0.3940 | 12.7% (predicted 11.6%) | 91,697 / 92,053 | 0 |
| Tier A `session/external` | skipped by criterion SC6 | 0.4460 (-0.01 pp) | 0.3936 (-0.04 pp, needs 0.10) | unchanged | 91,703 / 90,844 | 0 |
| Tier A `server/services/wsbridge` | skipped by criterion SC6 | 0.4459 (-0.02 pp, needs 0.30) | 0.3935 (-0.05 pp, needs 0.10) | unchanged | 88,382 / 92,053 | 0 |
| Tier B `server/services` p0 (`BacklogService` group) | not executed: relocation done (below), 32 straddling test files and the shared test-helper layer remain | | | | | |

Notes:
- `session/external` and `wsbridge` compile and pass their tests when moved with `scoping move` (one boundary field
  setter, `Instance.SetProcessManager`, was needed for `external`; `wsbridge` needed a local metric-reader helper and
  fixture path and data fixes). Both were reverted because the measured payoff is below SC6, not because of a defect.
- Tier B `server/services` p0 with the plan's file globs (66 files with tests): 15 distinct non-test declarations used
  by the staying core (`SessionStopper`, `AutonomousDriverStarter`, `SessionSteerer`, `WithTransitionCaller`,
  `redactErrorDetail`, `fromProtoStuckReason`, `crossHostClaimDedupFlagName`, ...) and 33 straddling test files,
  against an SC1 budget of 10. A relocation commit plus test-helper extraction must come first.
- Tier B `session` p0/p1/p2: not attempted (same shape, 70/60/10 declarations in the research).
- The research prediction for the marginal effect of `wsbridge` (-0.148 pp lib, -0.31 pp test) and `external`
  (-0.18 pp lib) is not reproduced by the Go replay; the Go tool follows renames through `git diff -M` against the
  baseline commit, the research tool mapped moved files to the package's core node.

## Epic 13 continuation (branch `sdd/bazel-epic13-tierb`)

Method: each candidate is applied with `scoping move --allow-collisions` on a throwaway detached commit and compared with
`scoping replay` on its parent (marginal effect, the SC6 definition); pp = percentage points, negative is better. Parent
values: before the `session/prpoller` move lib 39.171% / test 44.543% / fewer 12.70% / universal 70.97%; after it
38.955% / 44.336% / 13.23% / 70.97% (`server/services` 91,713 and `session` 90,866 LOC). "core uses" is the number of
`WARNING: X (staying) uses moved symbol Y` lines (SC1; a line is one use, not one distinct declaration).

| Step | Decision | T9a test share | T9c lib share | Changes with lower lib LOC | `server/services` / `session` LOC | Inversions |
|---|---|---|---|---|---|---|
| Tier A `session/prpoller` | executed (cc5392051): SC6 passes on library LOC | 0.4434 (-0.19 pp) | 0.3896 (-0.22 pp) | 13.2% (+0.5 pp) | 91,713 / 90,866 | 0 |
| tierA-reeval | hookctx: skipped by criterion SC6 (lib 0.00 pp, test -0.0015 pp; 10 core uses) | | | | 91,312 / 92,053 | |
| tierA-reeval | insights: skipped by criterion SC1 and SC6 (155 core uses, 23 capitalised; lib 0.00, test -0.020 pp, universal -0.40 pp) | | | | 88,155 / 92,053 | |
| tierA-reeval | gatestats: skipped by criterion SC1 and SC6 (19 core uses; lib 0.00, test -0.002 pp) | | | | 91,255 / 92,053 | |
| tierA-reeval | webhooks: skipped by criterion SC1 and SC6 (24 core uses, 58 capitalised; lib 0.00, test -0.008 pp) | | | | 90,256 / 92,053 | |
| tierA-reeval | backlogplugin: skipped by criterion SC6 (lib 0.00, test -0.024 pp; 0 core uses, 35 capitalised) | | | | 91,721 / 89,304 | |
| tierA-reeval | importer: skipped, not measurable: `newLiveInstance` would capitalise onto the existing `NewLiveInstance` (research marginal: test -0.04 pp, fails SC6) | | | | | |
| tierA-reeval | handoff: skipped by criterion SC6 (lib 0.00, test -0.080 pp; 0 core uses) | | | | 91,718 / 90,577 | |
| tierA-reeval | paneprompt: skipped by criterion SC1 and SC6 (30 core uses; lib 0.00, test -0.010 pp) | | | | 91,719 / 90,620 | |
| tierA-reeval | pipelinemode: skipped by criterion SC1 and SC6 (6 core uses; lib 0.00, test -0.007 pp) | | | | 91,718 / 91,045 | |
| tierA-reeval | sweeper: skipped by criterion SC6 (lib 0.00, test -0.005 pp; 0 core uses) | | | | 91,714 / 91,402 | |
| tierA-reeval | prpoller: executed, see the row above | | | | | |
| tierB-services-reeval | p1 (guidance/wake/notification helpers): skipped by criterion SC1 and SC6 (29 core uses; lib 0.00, test -0.02 pp); `metaGuidanceRequestID` collides with `MetaGuidanceRequestID` | | | | 90,427 / 90,866 | |
| tierB-services-reeval | p3: skipped by criterion SC1 and SC6 (31 core uses, 75 capitalised; lib 0.00, test -0.03 pp) | | | | 90,923 / 90,866 | |
| tierB-services-reeval | p4 (2 files): skipped by criterion SC2 (fewer than 3 files) and SC1 (32 core uses); lib -0.08 pp, test -0.10 pp | | | | 90,803 / 90,866 | |
| tierB-services-reeval | p5: skipped by criterion SC1 and SC6 (11 core uses; lib 0.00, test -0.08 pp) | | | | 91,097 / 90,866 | |
| tierB `server/services` p0 | SC6 passes (lib -0.25 pp, test -0.32 pp, universal -2.4 pp, services 76,352 LOC); relocation commits done (7bc6535bb, 6b9a1555e, 15 -> 0 core uses of the cluster); not moved, see "Services p0 test layer" | 0.4424 | 0.3897 | 12.7% | 76,352 / 92,053 (earlier parent) | |
| tierB `session` p0 | SC6 passes as measured without the relocation (lib -0.47 pp, test -0.67 pp, universal -2.4 pp, `session` 76,185 LOC) but SC1 fails: 519 core uses; `newTestInstance` collides with `NewTestInstance` | 0.4366 | 0.3848 | 14.8% | 91,736 / 76,185 | |
| tierB `session` p1 | SC6 passes as measured (lib -0.25 pp, test -0.26 pp, universal -1.6 pp, `session` 83,963 LOC) but SC1 fails: 216 core uses; same `newTestInstance` collision | 0.4408 | 0.3870 | 14.0% | 91,727 / 83,963 | |
| tierB-session-reeval | p2 startup backfills (8 files, 594 LOC): SC6 passes (lib -0.34 pp, test -0.31 pp, universal -0.8 pp, `session` 90,272 LOC) but not executed: `NewEntRepository` itself loops over `startupMigrations` (1 core use); moving needs an injected or init-registered migration list, which changes which of the 86 `NewEntRepository` call sites in 45 files run migrations, so it is not behaviour-preserving | 0.4402 | 0.3861 | 13.8% | 91,713 / 90,272 | |
| tierB-session-reeval | p3 (autonomous driver and scroll adapters, 7 files): skipped by criterion SC1 and SC6 (55 core uses, 25 capitalised; lib 0.00, test -0.02 pp). p4 (process backends): not measurable, the file list names `native_process_manager_windows.go`, which `go/packages` does not load on linux (research marginal: test -0.06 pp, fails SC6) | | | | 91,719 / 89,424 | |

### Services p0 test layer (why it was not moved)

After the two relocation commits the staying core uses no non-test declaration of the cluster (the dry run reports none).
The cluster move itself compiles for non-test code. What remains is test code, counted with
`go test -gcflags=-e -run NONE ./server/services/` after a trial move:

- 32 staying test files use `BacklogService`, `NewBacklogService`, `newBacklogService`, `mockSessionCreator`,
  `mockSessionStopper`, `itemStatus` and about 35 other symbols declared in the 65 cluster test files. A test in package
  `services` cannot import `backlogsvc` (which imports `services`), so each must move into `backlogsvc` or be rewritten.
- The 65 cluster test files use 34 test-declared helpers from core test files (`createTestStorage` in about 50 files,
  `withIsolatedStateDir`, the git fixtures, the claim fixtures, `setupForkTestFixture`, the `TestMain` hermetic setup
  and driver-leak recorder). `scoping deps` shows 29 of them have no dependency on the package's own types and can go to
  a `server/services/internal/svctest` package; `setupForkTestFixture`, `updateQuota`, `transitionDeniedRows` and
  `countDeniedNotes` are bound to `SessionService` or `BacklogService` and need either an exported non-test helper in
  the core or a rewrite of 3 tests.
- Path-keyed guards must follow the move: `workflow_bypass_strict_test.go` names `server/services/backlog_service_*.go`
  sites in its registry and scans the module root; `TestNoBareEnvMutationInServicesTests` scans one directory.
- Tooling added for this: `scoping relocate`, `scoping deps`, `scoping move --lower` (re-points staying files at a
  test-support package) and a collision refusal in `scoping move`.

The same shape applies to `session` p0 and p1 plus 519 and 216 non-test core uses, so those need the relocation commits
the plan describes (70 and 60 declarations) before any of this can start.

### Predicted versus measured (Task 13.7a), cumulative on this branch

| Cluster | Predicted lib share | Measured lib share | Error | Cause |
|---|---|---|---|---|
| Tier 0 inttest moves | 0 pp | 0.00 pp | 0 | test-only, as predicted |
| `sessionbundle` + `devtools` | -0.04 pp, 11.6% fewer | -0.05 pp, 12.7% fewer | within 1 pp | |
| `session/prpoller` | not predicted as passing (0.0 pp in the 2026-11-02 table) | -0.22 pp | 0.2 pp | churn in `pr_status_poller.go` and `worktree_pr_poller.go` rose since the research window |
| `session/external`, `wsbridge` | -0.18 / -0.15 pp | -0.04 / -0.05 pp | 0.1 pp | reverted earlier, see the notes above |

Final tree: `scoping t9` prints T9a PASS, T9b FAIL (universal touch 70.97% against 61%), T9c PASS, T9d FAIL (`server/services`
91,713 and `session` 90,866 LOC against 72,000), T9e PASS. T9b and T9d need the unexecuted Tier B moves: the plan's bound of
72,000 LOC is only reachable with `server/services` p0 and p2 and `session` p0, p1 and p2 all moved.

## Epic 13 continuation 2 (branch `sdd/bazel-epic13-tierb2`)

| Step | Decision | T9a test share | T9c lib share | Universal touch | `server/services` / `session` LOC | Inversions |
|---|---|---|---|---|---|---|
| parent (after `session/prpoller`) | | 0.4434 | 0.3896 | 0.7097 | 91,713 / 90,866 | 0 |
| tierB `server/services` p0 (real cluster, `github_user_*` stays in core) | executed (83318702b): SC1 0 core uses; SC6 passes (lib -0.44 pp, test -0.43 pp, universal -2.0 pp) | 0.4391 | 0.3852 | 0.6895 | 77,709 / 90,866 | 0 |
| tierB `session` p0, p1 | not attempted this pass | | | | | |
| tierB `session` p2 | rejected: moving needs an injected or init-registered migration list, which changes which of the 86 `NewEntRepository` call sites run migrations | | | | | |
| `importer`, `session` p4 | not re-measured this pass | | | | | |

Method notes: `scoping closure` (declaration-level cut of staying tests that use a cluster), `scoping peel` (cluster declarations the staying code uses that do not need the anchor type), `scoping move` reporting stay-behind test helpers as twins. Hand work needed after the codemod: exported `TransitionWithGuard` and `HasUnresolvedBlockers` (core jules guard interface), a nudge-guard clock constructor, backlogsvc copies of five fixtures, and path-keyed guards (workflow bypass inventory and strict, terminal-write pins, `TransitionCallers`, wake-polling map, source-scanning data deps). Test function names are unchanged (3752 before and after; the grep also sees one raw-string line `func TestQualified` and a second `TestMain`).
