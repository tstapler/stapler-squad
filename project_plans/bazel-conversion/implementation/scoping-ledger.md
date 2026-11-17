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

## Epic 13 continuation 3 (branch `sdd/bazel-epic13-session`)

Method: the file set of each candidate was found by a declaration-graph fixpoint over the real package (eject any file whose used declarations
drag more than 25 declarations along, or whose declarations are mostly used by the core; relocate the rest). Marginal effect is
`scoping replay` on a throwaway commit of the move against its parent (the SC6 definition); pp = percentage points, negative is better.
Parent values: T9a 0.4391, T9b 0.6895, T9c 0.3852, `session` 90,866 LOC.

| Step | Decision | T9a test share | T9c lib share | Universal touch (500) | `server/services` / `session` LOC | Inversions |
|---|---|---|---|---|---|---|
| parent | | 0.4391 | 0.3852 | 0.6895 | 77,709 / 90,866 | 0 |
| tierB `session` p0 (research list, 32 files) | not executable as listed: 519 core uses and a 178-declaration closure behind `BacklogLifecycleListener`, 35 behind `NewChainFirer`; after two interface inversions (`BacklogListener`, `ChainDispatcher`) and 28 relocated declarations the cluster is 27 files | | | | | |
| tierB `session` p0 (27 files, 28 declarations relocated) | executed (b6f9f465b): SC1 0 core uses; SC6 passes (lib -0.50 pp, test -0.58 pp, universal -3.2 pp, fewer 14.8%) | 0.4333 | 0.3802 | 0.6573 | 77,715 / 77,473 | 0 |
| tierB `session` p1 (21 files) | rejected: `Instance`, `session_driver.go`, `health.go`, `startup_scanner.go`, `import_commit.go` and seven more core files construct or hold 12 of its collaborators (`ControllerManager`, `InstanceStatusManager`, `resolveHistoryAdapter`, `PortSessionHistory`, `NewPiStatusSource`, ...). Moving it needs those built outside the core and injected or registered, which changes every `Instance` construction path. The SC1-compliant remainder (3 files, 2.4k LOC) fails SC6 | 0.4329 | 0.3792 (-0.08 pp, needs 0.10) | 0.6532 (-0.41 pp, needs 1.0) | 77,727 / 75,107 | 0 |
| tierA `importer` (11 files) | skipped by criterion SC6 (lib +0.01 pp, test 0.00, universal 0.00; the `newLiveInstance` collision is moot) | 0.4333 | 0.3801 | 0.6573 | 77,719 / 75,980 | 0 |
| tierB `session` p4 (4 files + the windows-only file, moved by hand because `go/packages` loads linux only) | skipped by criteria SC1 and SC6: `backend_factory.go` in the core constructs `NewTymuxBackend`, `NewTmuxBackend`, `NewNativeProcessManager` and `Instance` reads `processManagerBackendLabel` (5 core uses); lib 0.00, test -0.01 pp, universal -0.41 pp | 0.4332 | 0.3800 | 0.6532 | 77,715 / 76,305 | 0 |
| tierB `session` p2 | stays rejected: the backfills are run by `NewEntRepository` itself; an identical migration run at all 86 call sites in 45 files cannot be proven while the list lives above `session` | | | | | |

Final tree: `scoping t9` T9a PASS (0.4333), T9b FAIL (0.6573 against 0.61), T9c PASS (0.3802), T9d FAIL (`server/services` 77,715 and `session`
77,473 against 72,000), T9e PASS. T9b and T9d now depend on `server/services` p2 and on Tier C work (`Instance`, `EntRepository`, `Storage`).

What the p0 move needed beyond the codemod (so the next cluster can reuse it): `BacklogController` holds a `BacklogListener`, `EntRepository`
holds a `ChainDispatcher`, `WireChainFirer` is a package function over `Storage.Repository()`, 28 shared declarations live in
`backlog_shared.go` and `workflow_engine_types.go`, 44 test files were split into `*_bk_test.go` before the move, the 18 helpers free of core
types became `session/internal/sessiontest` (with the hermetic `TestMain` body) and the 17 bound ones were copied as twins, git CLI helpers
went to the sanctioned `testutil/gitfixture`, and the path-keyed guards were re-pointed (wake-polling map, workflow bypass inventory and
strict test, `transition_callers.go` file constants, denied-guard source scans, golden `testdata`, `go_sources` filegroups).

## Epic 13 continuation 4 (branch `sdd/bazel-epic13-next`)

Parent values (tip of `sdd/bazel-epic0`): T9a 0.4330, T9b 0.6573, T9c 0.3785, `server/services` 77,715, `session` 77,473. Marginals below are
`scoping t9`/`replay` on the tree before and after the move (pp = percentage points, negative is better; SC6 needs lib -0.10, test -0.30, 3% of
changes lower, or universal -1.0). New tooling that picked the candidates: `scoping touch` (T9b gain of moving a file set: replayed production changes
whose only universal-package files are in the set) and `scoping leaves [--bases]` (files nothing else in the package uses; with `--bases` the files that
use nothing else in the package, i.e. lower layers). The lesson from the numbers: a leaf cluster still depends on the core, so a core change still
rebuilds it and its lib share barely moves; a lower cluster (the core imports it) leaves the core's invalidation set and does move it.

| Step | Decision | T9a test share | T9b universal | T9c lib share | `server/services` / `session` LOC | Criteria |
|---|---|---|---|---|---|---|
| parent | | 0.4330 | 0.6573 | 0.3785 | 77,715 / 77,473 | |
| `server/services/wsconn` (`connectrpc_websocket*`, `websocket_transport`, `ws_*`, `session_gone`, `terminal_stream_metrics`; 18 files, 8.5k LOC) | executed (4e0c21d58) | 0.4271 (-0.59 pp) | 0.6129 (-4.4 pp) | 0.3744 (-0.40 pp) | 69,539 / 77,473 | SC1: 9 declarations relocated first (`sendInputToTmux*` and `runTmuxGatedErr` to `tmux_input_send.go`/`external_pane_resize.go`, the stream-hub name helpers, `resizeExternalCapturePaneSession` over a new core `ExternalPaneTarget`), 0 staying uses after; SC2: 12% of services; SC3: 1 importer (`server`); SC4: 11 replayed changes touch only its files; SC6: all three |
| `session/importer` (`import_*`, `create_managed_instance`, `suspended_process_store`, `history_linker`, `history_watcher`, `scan_coalescer`; 13 files, 2.0k LOC) | executed (47124d07f) | 0.4233 (-0.38 pp) | 0.5968 (-1.6 pp) | 0.3707 (-0.37 pp) | 69,543 / 75,514 | SC1: 3 declarations relocated (`jsonlStartTime` and two constants to `history.go`), 0 staying uses; SC2: 2.7% of session; SC3: 3 importers (`server`, `server/services`, `server/services/inttest`); SC4: 4 sole-touch changes; SC6: universal -1.6 pp, lib -0.37 pp |
| `session/ptyio` (`circular_buffer`, `locked`, `pty_access`, `pty_readwait_*`, `response_stream`; 6 files, 1.3k LOC; lower package) | executed (4ecd90973) | 0.4213 | 0.5806 (-1.6 pp) | 0.3695 (-0.12 pp) | 69,543 / 74,306 | SC1: 0 relocations, 7 distinct declarations used by the core; SC2: 1.7% of session; SC3: 1 importer (`session`); SC4: no import edge to the rest of `session`; SC6: universal -1.6 pp and lib -0.12 pp |
| `session/hostclaim` (`host_*`, `claim_gossiper/index/lookup_auth/url`; 7 files, 1.7k LOC; lower package) | implemented, tested, then reverted (6a28a742d, d5d69307f) by criterion SC3 | 0.4198 | 0.5968 | 0.3664 (-0.43 pp) | 69,546 / 73,835 | SC1: 10 uses; SC3 fails: 7 non-test importers outside `session` (`.`, `server`, `server/auth`, `server/services`, `server/services/backlogsvc`, `server/services/internal/svctest`, `session/backlog`) against 5; SC6 passes |
| lower `command_executor/history/queue` over `ptyio` (3 files, 1.4k LOC) | skipped by criterion SC1 (13 distinct staying uses) | 0.4179 | 0.5806 | 0.3639 (-0.13 pp) | 69,546 / 71,252 | SC6 passes; with this and `hostclaim` `session` would be 71,252 |
| `server/services` `github_*` (7 files, 2.7k LOC) | skipped by criteria SC1 and SC6 (`GitHubService` is held by `SessionService`, a cycle; lib 0.00, test -0.02 pp, universal -0.40 pp) | | | | | |
| `session` leaf clusters: `session_summary`+`handoff_summary_*` (1.5k), `pane_reply`/`question_*`/`dialog_presence` (1.4k), `backlog_crypto`/`backlog_sync`/`feature_controller` (0.8k), `executor_program`+`pipeline_mode_*` (0.8k), `hibernation`/`orphan` sweepers (0.7k) | skipped by criterion SC6 (lib 0.00 pp, test -0.00 to -0.07 pp, universal 0.00); 0 staying non-test uses each | | | | | |
| `session` leaf `external_*` (3 files, 1.2k) | skipped by criterion SC6 (lib -0.086 pp, test -0.089 pp, 3% rule +0.26 pp; the closest miss) | | | | | |
| `session` families with staying uses: `history` rest (13 uses; lib -0.085, universal -0.4), `claude_*` (12; lib 0.00), `pi_*` (5; lib 0.00), `review_*` (23), `gate_*` (17), `command_*` (13), `workflow_*` (22), `autonomous_*` (7; lib 0.00), `claim*` (6; lib 0.00) | skipped by criteria SC1 and/or SC6 (numbers in parentheses) | | | | | |
| `session` `storage*` (8 files, 4.0k) and `ent_*` (22 files, 9.6k) | skipped by criterion SC1: 14 distinct staying uses each (284 and 121 staying test uses); both pass SC6 (`ent_*`: lib -0.134 pp, test -0.188 pp, universal -1.2 pp) and are the Tier C interface-extraction work | | | | | |
| lower `git_worktree_*` (0.9k), `process_terminate`+`tmux_process_manager` (0.8k, 2 files), `instance_shells`/`shell*` | skipped by criterion SC6 (lib -0.07 pp each) and SC2 (2 files); `shell*` uses `Instance` methods so `scoping move --lower` refuses it | | | | | |
| `session` `pr_*` (3 files, 421 LOC) | not movable: `pr_tracking.go` and `pr_provenance.go` declare methods on `Instance`, `Storage` and `ClaimIndexRecorder` | | | | | |

Final tree: `scoping t9` T9a PASS (0.4213), T9b PASS (0.5806 against 0.61), T9c PASS (0.3695), T9d FAIL (`server/services` 69,543 passes, `session`
74,306 against 72,000), T9e PASS. `session` needs 2,306 more lines. The two moves that would close it, `hostclaim` (-1,683) and the `command_*` lower
package (-1,376), fail SC3 and SC1 as written; both are lower packages whose "uses" and importers are just their callers, so the owner may want SC1 and SC3
read as relocation count and rewrite blast radius for lower packages (ADR-010 Decision 2 words SC1 as declarations relocated into the core). The cluster
maps stay as measured above; `baseline.json` is unchanged (it is the pre-epic anchor `scoping t9` compares against).

Flakes seen: `TestUnaryWsBridge_should_CountConnectionOpenAndPeerClose` failed once while `go test` ran two packages at once under load, then passed three
more times and under Bazel; `//server:server_test` failed once on `TestServerDependencies_should_DegradeFeatureNotServer_When_KeychainUnreadableAtStartup`
(TempDir cleanup "directory not empty", background sweeper) and passed twice with `--cache_test_results=no`.

## Epic 13 owner decisions (branch `sdd/bazel-owner-decisions`, 2026-11-05)

Owner decision (AskUserQuestion, 2026-11-05): for lower packages (packages `session` imports), SC1 counts declarations relocated into the core (not callers' uses) and SC3 does not count natural callers as importers (`scoping criteria --lower`; ADR-010 Decision 2). Under it the two moves the previous branch skipped were redone:

| Step | Decision | T9a | T9b | T9c | `server/services` / `session` LOC | Criteria |
|---|---|---|---|---|---|---|
| parent | | 0.4213 | 0.5806 | 0.3695 | 69,543 / 74,306 | |
| `session/hostclaim` (revert of d5d69307f, re-applies 6a28a742d) | executed | | | | not measured separately (see final) | SC1 (lower): about 1 relocated declaration (`RecordOwn` became a method); SC3 (lower): the 7 importers (`.`, `server`, `server/auth`, `server/services`, `server/services/backlogsvc`, `server/services/internal/svctest`, `session/backlog`) already imported `session`, so 0 new; SC6 passes (lib -0.43 pp measured on the previous branch) |
| `session/cmdexec` (`command_executor/history/queue`, 3 files plus 4 test files; lower) | executed | | | | not measured separately (see final) | SC1 (lower): 0 relocated (its test now has a local `pipePTY` double); SC3: 1 importer (`session`); SC6 passes (lib -0.13 pp measured previously) |
| final | `scoping t9` | 0.4158 | 0.5806 | 0.3637 | 69,546 / 71,252 | T9d PASS (session 71,252 <= 72,000), T9f N/A by owner decision (ADR-009 "Full e2e run") |

## Backlog DTO series BL-A..BL-E (2026-11-11)

Prerequisite-only investment, no standalone Bazel or cache payoff (`project_plans/package-split-ocp/dto-lowering-roadmap.md`). `scoping replay` before BL-A (`efac6df74`) vs after BL-E: T9a 0.4097 -> 0.4095, T9b(500) 0.5887 -> 0.5887, T9c mean lib share 0.3549 -> 0.3540, T9d `session` LOC 78,593 -> 77,746 (`server/services` 73,449 -> 73,469), T9e 0.0329 -> 0.0328. `scoping t9` after: T9c FAIL (0.3540 vs max 0.395) and T9d FAIL (services 73,469 and session 77,746 vs max 72,000), unchanged in kind. Direct module imports went up by one for every consumer (`session` 106 -> 107, `server/services` 65 -> 66, `backlogsvc` 20 -> 21, `session/backlog` 14 -> 15, `server/mcp` 16 -> 17) because they now also import `session/domain/backlogdto`; no consumer shed `session`. SC6 is therefore not met. Gate further DTO families (stage, pipeline-mode, gate) on a consumer that actually sheds `session`.
