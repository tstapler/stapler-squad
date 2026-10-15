# Validation Plan: unfinished-scanner-dsa-perf

**Date**: 2026-10-15
**Inputs**: `requirements.md`, `implementation/plan.md` (17 epics / 45 stories / 102 tasks after the Phase 4 patch, which folded gate gaps GA1-GA18 into story ACs; see the plan's "Phase 4 patch log" for where each landed and which stay procedure-only; Verification Matrix, repair logs 1-3), ADR-001..009. Paper design only: no source changed, nothing built or run. Every file/test named below is a *planned* artifact; paths come from the plan's story `Files:` lists. Labels: **VERIFIED** = read in the cited file; **INFERRED** = my reasoning from it.

## Happy Path Scenario
Given a repo with 3 linked worktrees on the post-Wave-0 baseline (S1: lockstep walk committed, flags off, so each worktree re-decodes the shared commit walk), when a debounced `.git` Burst arrives with all `shared_commit_state`, `diff_counter`, `scan_skip` and `watch_worktree_gitdirs` flags on, then total commit decodes equal one shared walk (`DecodeCount(W=3) == DecodeCount(W=1) <= DecodeCount(committed-old, W=1)`), every `ScanResult` ahead/behind/dirty/shortstat field equals the legacy oracle's, cache hits allocate nothing, and the same-binary A/B on the scripted workload shows scanner CPU-seconds S2/S1 <= rho.

## Requirement Catalog (28 items; IDs used in the mapping table)

Derived from `requirements.md` section by section. "Rabbit hole" items (RH) are requirement-grade because the owner listed them as correctness traps that must not regress.

| ID | Source (requirements.md) | Statement |
|---|---|---|
| M1 | Success Metrics L22 | Scanner share of process CPU 39.3% -> <= 15% (plan restates as S2/S1 <= rho, absolute share judged only at live deploy) |
| M2a | L23 | Process alloc rate >= 2x lower from commit-walk stories (decode path) |
| M2b | L23 | Separate target for the other 48.1% allocators; each has an owner or "accepted" note |
| M3 | L24 | Reachable-set/commit-parent work O(R) per (repo, base), asserted by a decode-count test across >= 3 worktrees |
| M4 | L25 | Every changed hot path ships a `-benchmem` old-vs-new benchmark and a regression test that fails on the old behaviour |
| M5 | L26 | No functional regression; result parity (ahead/behind, dirty, shortstat) vs old implementation on a corpus |
| C1 | Constraints L34 | go-git only; no exec/shell-out in product, test, fixture, benchmark |
| C2 | L35 | Fork patches small and additive; one tag per patch; fork CI green before `replace` bump |
| C3 | L36 | Lock discipline: `entry.mu` only around go-git object access; `*Instance` reads via Snapshot() |
| C4 | L37 | Cache hits allocation-free; deterministic-fast-tests (no sleeps) |
| C5 | L38 | `make ready` incl. `dupl` and complexity gates |
| N1 | NFR L41 | Performance SLO; no scan holds a repo-wide lock across a stat walk |
| N2 | L42 | Scalability: tens of worktrees, several repos, >= 50k-commit histories, tens of thousands of tracked files |
| N3 | L43-44 | Security (internal) / data residency (none): nothing to exercise beyond not mutating owner repos |
| S1 | Scope L48 | Per-repo shared caches keyed by (common dir, hash) |
| S2 | L49 | Commit-graph usage and header-only commit/parents reader (fork patch) |
| S3 | L50 | Dirty detection that scales with changed paths, watcher-driven, where correct |
| S4 | L51 | Better diff algorithm for shortstat; allocation-lean walker data structures |
| S5 | L52 | Scheduling/coalescing across worktrees of one repo |
| S6 | L53 | Memory-bound shared caches; measurement and regression gates (Tier 1 benchmarks, pprof before/after) |
| O1 | Observability L81 | Per-cache hit/miss/eviction/bytes counters via `metrics.go` + actuator; debug scan-cycle log; Pyroscope comparison |
| R1 | Risk Control L84 | Feature flag per cache (off-switch via config); fork patch revertable by rolling `replace` back one tag; staged rollout |
| RH1 | Rabbit Holes L62 | Cross-worktree sharing boundary exactly what is truly shared (objects/refs shared; index, HEAD, `info/exclude` per worktree) |
| RH2 | L63 | Concurrency and invalidation of a repo-wide cache; eviction under budget |
| RH3 | L64 | Behaviour independent of commit-graph presence |
| RH4 | L65 | Watcher-driven dirty tracking never misses changes; safe full-walk fallback |
| RH5 | L66 | Diff parity with LCS counts (insertions/deletions semantics) |
| RH6 | L67 | Fork rebase burden bounded per patch |

## Requirement -> Test Mapping

Naming: `TestUnit_Should<Behavior>_When<Condition>`. Directories: `A` = `session/unfinished`, `RS` = `session/unfinished/repostate`, `GS` = `session/unfinished/gogitstore`, `TR` = `session/unfinished/internal/testrepo`, `TW` = `tools/scanner-workload`. Gate ids G1-G11 are the plan's (Task 0.1.1c). "(neg)" = negative-control test that proves a gate can fail (see Gate Audit GA4). "(!race)" = file carries `//go:build !race`, never behind `-short`. "(cond)" = exists only if the story's gate opens.

| Requirement | Test File | Test Name | Type | Scenario |
|---|---|---|---|---|
| M1: scanner CPU share | TW/compare_test.go | TestVerdict_ShouldMeet_WhenAllRatiosWithinTargets | Unit | Happy path: S2/S1 CPU ratio <= rho, alloc ratio <= target |
| M1 | TW/compare_test.go | TestVerdict_ShouldHold_WhenCPURatioExceedsRho | Unit | Error path: ratio > rho gives HOLD, defaults not flipped |
| M1 | TW/compare_test.go | TestVerdict_ShouldBeInconclusive_WhenSwappedRunsDisagreeInDirection | Unit | Alternation: control/candidate swapped ports, ratios disagree |
| M1 | TW/compare_test.go | TestVerdict_ShouldNotJudgeAbsoluteShare_WhenInstanceIsManual | Unit | Absolute 15% only accepted from live (:8543) input |
| M1 | A/metrics_test.go | TestScanCyclesTotal_ShouldIncrement_WhenScanCompletes | Unit | Normalisation counter used for per-scan diagnostics |
| M1 | TW/baselinecheck_test.go | TestBaselineCPUTable_ShouldCoverNinetyPercent_AndOwnEveryBucket | Unit | Parses `baseline.md` bucket table: buckets >= 90% of scanner CPU each have story id or "accepted" with share; `SHORTFALL` row fails |
| M1 | (script) Story 5.2.1 | ab_s2_vs_s1_cpu_seconds_should_meet_rho | Integration | Measured: 2 manual instances, same corpus + seeded stream, >= 30 min, 30-min pprof/Pyroscope; MEASURED, not CI-failable |
| M1 | (script) Story 5.2.2 | live_share_of_process_cpu_should_be_le_15pct | Integration | Measured at live deploy with flags on; miss => HOLD |
| M2a: decode-path alloc | A/aheadbehind_gate_test.go | TestAheadBehind_ShouldDecodeNoMoreThanLegacyAndLockstep_WhenCold2500LinearW1 | Unit | G1: `DecodeCount(new) <= legacyStack` and `<= lockstep`, branch 3 ahead |
| M2a | A/aheadbehind_gate_test.go | TestG1Gate_ShouldFail_WhenLegacyStackWalkIsWiredAsNew (neg) | Unit | Error path: gate harness fed the committed-old walk must report failure |
| M2a | A/commit_parents_test.go | TestCommitParentsStock_ShouldUseAtMost70PctBytesOfDecode_When2500Commits | Unit | In-test `TotalAlloc` delta over fixed N (GA1), stock vs `DecodeCommit` |
| M2a | A/commit_parents_test.go | TestCommitParentsStock_ShouldNotReturnParentText_WhenGpgsigContinuationHasParentLine | Unit | Error path: stops at first blank line, not by pattern |
| M2a | A/commit_parents_test.go | TestCommitParentsStock_ShouldMatchDecodeCommit_WhenCorpusOfNineShapes | Unit | 0/1/2/3 parents, gpgsig, mergetag, empty message, no trailing newline, blank-then-`parent ` message |
| M2a | A/commit_parents_test.go | TestCommitParentsStock_ShouldReturnError_WhenObjectIsNotACommit | Unit | Error path: tree/blob object, truncated header, header without `tree` line |
| M2a | A/aheadbehind_integration_test.go | TestAheadBehind_ShouldDecodeLEOracle_WhenColdThroughSharedPathOnFilesystemStore | Integration | Real `gogitstore` filesystem store, counting storer, flag on |
| M2a | (script) Story 5.2.1 | decode_bucket_cut_s2_vs_s1_should_be_ge_half_over_d1 | Integration | Measured; "n/a, d1 < 0.5" must be emitted by verdict script when applicable |
| M2b: other allocators | A/myers_test.go | TestMyersCounter_ShouldUseAtMost25PctBytesAndAllocsOfLcsLength_When2000Lines15Changes | Unit | G5, in-test measurement (not benchmark-only) |
| M2b | A/myers_test.go | TestG5Gate_ShouldFail_WhenLcsLengthIsWiredAsNew (neg) | Unit | Error path |
| M2b | A/vcsreader_gate_test.go | TestHasUncommittedClean_ShouldAllocAtMost60PctOfLegacy_WhenIndexSigUnchanged (!race) | Unit | G8, 20,000 files, index mtime backdated (GA2) |
| M2b | A/vcsreader_gate_test.go | TestHasUncommittedClean_ShouldReparse_WhenIndexMtimeWithin2sOfParse | Unit | Error path: racy guard denies the cached path, so G8 must *not* hold here |
| M2b | TW/baselinecheck_test.go | TestBaselineAllocTable_ShouldOwnEveryNonDecodeBucketOver3Pct | Unit | Each >= 3% non-decode `alloc_space` bucket has owner story or "accepted" with share |
| M2b | A/residual_alloc_test.go | TestResidualAllocator_ShouldMeetCeilingVsLegacy_WhenAssignedTo441 (cond) | Unit | One test per Story 4.4.1 row: `allocs/op <=` legacy ceiling |
| M2b | (script) Story 5.2.1 | scanner_alloc_space_ratio_s2_over_s1_should_meet_target | Integration | Measured per bucket, projected-vs-measured table |
| M3: O(R) per (repo, base) | A/aheadbehind_gate_test.go | TestAheadBehind_ShouldDecodeOnce_WhenThreeWorktreesShareHeadAndBase | Unit | G2 oracle half: `DecodeCount(W=3) == DecodeCount(W=1) <= committed-old(W=1)` |
| M3 | A/aheadbehind_gate_test.go | TestAheadBehind_ShouldDecodeWTimes_WhenSharedCommitStateFlagOff (neg) | Unit | Engagement control: flag off shows W-fold decodes, proving the G2 test exercises the shared path |
| M3 | A/aheadbehind_gate_test.go | TestAheadBehind_ShouldBoundDecodes_WhenThreeDistinctTipsRace | Unit | G2b: `distinct <= DecodeCount <= distinct + fill_lost_decode + reprobe` |
| M3 | A/aheadbehind_gate_test.go | TestAheadBehind_ShouldDecodeExactlyDistinct_WhenWalksSequencedByLatches | Unit | G2b sequential: lost-CAS counters 0, `DecodeCount == distinct`, `<=` sum of old per-tip |
| M3 | RS/reach_test.go | TestReach_ShouldComputeOncePerRepoBase_WhenThreeWorktreesCallCommitMessages (cond 2.2.1) | Unit | `DecodeCount` equals one set computation |
| M3 | RS/reach_test.go | TestReach_ShouldRebuildFromScratch_WhenAncestorSetWasTruncated (cond) | Unit | Error path: never extend a truncated set; equals legacy truncated set at lowered cap |
| M3 | RS/reach_test.go | TestReach_ShouldWalkOnlyNewCommits_WhenBaseAdvances12 (cond) | Unit | Incremental |
| M3 | A/commitmessages_test.go | TestCommitMessages_ShouldRunOneReachWalk_WhenFourGoroutinesUseDifferentMax | Unit | Story 1.1.4 singleflight regression (fails on old code: 4 walks) |
| M3 | A/multiworktree_integration_test.go | TestMultiWorktree_ShouldShareGraph_WhenLinkedWorktreesBuiltByTestrepo | Integration | Real linked-worktree fixture, flag on, counting storer |
| M3 | A/multiworktree_integration_test.go | TestStartupHerd_ShouldDecodeLEOracleTotal_WhenSixReposThreeWorktreesOpenTogether | Integration | G9 + G2b |
| M3 | A/vcsreader_bench_test.go | BenchmarkMultiWorktreeAheadBehind/W=1,3,8 | Integration | Reports `decodes/op`; advisory output, the hard gate is the `Test*` rows above |
| M4: bench + regression per hot path | A/gate_selfcheck_test.go | TestRegressionGates_ShouldAllFail_WhenLegacyImplementationIsWired (neg) | Unit | Table of {gate fn, legacy impl}; each gate run against a fake `testing.TB` must record a failure (GA4) |
| M4 | A/benchmark_regex_test.go | TestBenchmarkYmlTier1Regex_ShouldMatchEveryNewBenchmarkExactlyOnce | Unit | Parses `.github/workflows/benchmark.yml:94` regex; each of the 6 new tokens + `BenchmarkCommitParents` matches an existing `Benchmark*` func |
| M4 | A/benchmark_regex_test.go | TestBenchmarkYmlTier1Regex_ShouldNotMatchNativeBenchmark | Unit | Error path: `BenchmarkDiffShortstat_GoGitVsNative` absent; token anchored `BenchmarkDiffShortstat$` |
| M4 | A/benchmark_regex_test.go | TestHotPathTable_ShouldHaveBenchmarkAndGateTest_WhenStoryChangesHotPath | Unit | Static table maps each Verification-Matrix row 4 story to a `Benchmark*` + `Test*Gate*` symbol (go/parser) |
| M4 | A/racetag_test.go + norace_test.go | TestAllocGateHarness_ShouldSkipNotFail_WhenRaceEnabled | Unit | `raceEnabled` constant pair |
| M4 | A/vcsreader_bench_test.go | BenchmarkCommitParents/{stock,decode,fork}, BenchmarkReachableSetShared, BenchmarkHasUncommittedClean, BenchmarkHeadTreeMapShared, BenchmarkShortstatLines | Integration | `-benchtime=Nx -benchmem`; old/new sub-benchmarks present; advisory (CI step is `|| true`, GA11) |
| M5: parity | A/mergebase_legacy_test.go | TestFindMergeBase_ShouldEqualLegacyStack_WhenLinearOrSingleMergeDAG | Unit | Story 0.0.1: >= 500 seeded DAGs; any divergence fails |
| M5 | A/mergebase_legacy_test.go | TestFindMergeBase_ShouldRecordGoldenDivergence_WhenCrissCrossOctopusOrCapBoundary | Unit | Divergences must equal checked-in golden set |
| M5 | A/mergebase_legacy_test.go | TestLegacyStackWalk_ShouldReturnSameAsPreLockstepTests_WhenSentinelCasesRun | Unit | Sentinel for the copied oracle |
| M5 | A/mergebase_oracle_test.go | TestFindMergeBase_ShouldReturnMemberOfMergeBaseSet_WhenCrissCrossCorpus | Unit | vs `object.Commit.MergeBase` |
| M5 | A/mergebase_oracle_test.go | TestFindMergeBase_ShouldNotReturnNonCommonAncestor_WhenAnyCorpusCase | Unit | Error path |
| M5 | RS/walk_test.go | TestIDWalk_ShouldEqualLockstep_WhenRandom500DAGsOnAndOffFlag | Unit | Hash, count, "not found within L" error equal |
| M5 | RS/walk_test.go | TestIDWalk_ShouldAddNoDivergenceBeyondGolden_WhenComparedToLegacyStack | Unit | Golden set must not grow |
| M5 | RS/walk_test.go | TestIDWalk_ShouldFallBackToLegacyWithSameResult_WhenErrGraphFullMidWalk | Unit | Error path: lowered `MaxGraphCommits` |
| M5 | RS/walk_test.go | TestIDWalk_ShouldKeepBFSOrder_WhenDFSAndBFSDifferAtCap | Unit | R2 in research/architecture.md |
| M5 | A/diffcounts_oracle_test.go | TestMyersCounter_ShouldEqualLcsOracle_When10000SeededPairs | Unit | Includes empty, identical, disjoint, repeated lines, <= 2,000 lines |
| M5 | A/diffcounts_oracle_test.go | TestMyersCounter_ShouldReturnFullReplaceApproximation_WhenOver20000Lines | Unit | Cap behaviour preserved (`maxLCSLines`) |
| M5 | A/walkers_oracle_test.go | TestShortstat_ShouldEqualLegacy_WhenDiffCounterOnAndOffOnWalkersCorpus | Integration | Existing corpus, both flag states |
| M5 | A/flags_test.go | TestParitySuite_ShouldPass_WhenRunOnEveryLegalFlagCombination | Unit | Normalizer-derived legal set, not 2^6 |
| M5 | A/migration_assertions_test.go | TestMigratedTests_ShouldKeepAssertionASTHashes_WhenFixturesMovedToTestrepo (proposed, GA15) | Unit | Golden hash of assert/require/t.Errorf call ASTs per migrated test, written before Story 0.0.3 |
| M5 | A/vcsreader_test.go | TestAheadBehindBehindCount_ShouldHaveSameTopology_WhenFixtureBuiltByTestrepo | Unit | Fixture-equivalence: commit count and parent shape before original assertion |
| M5 | A/vcsreader_integration_test.go | TestAllExistingScannerTests_ShouldPass_WhenRunAfterEachStory | Integration | `make test` over `session/unfinished/...`; zero assertion edits except Story 0.0.3 fixtures |
| M5 | (script) Story 5.2.1 | scanresult_parity_control_vs_candidate_after_each_scripted_event | Integration | Script compares ahead/behind, dirty, shortstat per worktree; any diff fails |
| C1: go-git only | A/no_native_git_test.go | TestNoNativeGitInPackage_ShouldPass_WhenWalkingSessionUnfinishedAndScannerWorkload | Unit | `filepath.WalkDir` + `go/parser` imports; only allow-list entry `gogitstore/crash_subprocess_test.go` |
| C1 | A/no_native_git_test.go | TestNoNativeGitInPackage_ShouldFail_WhenPlantedFileImportsOsExecOrSafeexec | Unit | Error path: scan a temp tree with a planted file |
| C1 | A/no_native_git_test.go | TestNoNativeGit_AllowListEntry_ShouldHaveNoGitLiteralAndTargetOsArgs0 | Unit | Allow-list entry self-check |
| C1 | A/no_native_git_test.go | TestTestrepoDeps_ShouldNotImportExecOrSafeexec_WhenResolvedWithGoBuild | Unit | In-process transitive import walk via `go/build` (no `go list` subprocess, GA13) |
| C1 | A/no_native_git_test.go | TestNativeArmBenchmark_ShouldBeAbsent_WhenShelloutBenchFileDeleted | Unit | File absent; no `nativeGitDiffShortstat` symbol |
| C1 | TW/corpus_test.go | TestCorpusBuilder_ShouldCopyInProcess_WhenBuildingInstanceCorpus | Unit | No exec; byte-copy via Go |
| C1 | TR/testrepo_test.go | TestNewLinkedWorktree_ShouldProduceIndexedWorktree_WhenBuiltWithGoGitOnly | Unit | Writes `.git/worktrees/<n>/{HEAD,commondir,gitdir}`; `Reset(Hard)` populates index (plan INFERRED) |
| C1 | GS/gogitstore_fixture_test.go | TestRepackedFixtures_ShouldBeReadable_WhenWrittenByRepackObjects | Integration | Replacement for `git gc --aggressive` interop; records lost interop coverage (Q13) |
| C1 | Makefile `lint-custom` | norawexec/norawgitcli/nocommandpattern over migrated tests | Integration | CI gate; plan UNVERIFIED whether they scan `_test.go` |
| C2: fork additive | fork/ssq_additive_test.go | TestSsqPatchesAreAdditive_ShouldPass_WhenOnlyInsertsAgainstUpstream | Unit | `object.DiffTree` upstream tip vs branch tip: all `Insert` |
| C2 | fork/ssq_additive_test.go | TestSsqPatchesAreAdditive_ShouldFail_WhenExistingFileModified (neg) | Unit | Error path |
| C2 | fork/commit_header_test.go | TestDecodeCommitHeader_ShouldEqualDecodeCommitParents_WhenCorpus | Unit | Differential, shared corpus helper |
| C2 | A/commit_parents_fork_test.go | TestForkReader_ShouldMatchStockReader_WhenBuiltWithSsqforkTag (cond 1.2.2 gate) | Unit | `//go:build ssqfork` |
| C2 | (script) Story 1.2.2 | fork_local_modfile_bench_should_be_10pct_lower_bytes_than_stock | Integration | `-modfile` path-replace; verdict recorded; failing gate => no tag, ADR-004 amended |
| C2 | (CI) Story 1.2.3 | required_ci_leg_vet_and_differential_tests_with_ssqfork_tag | Integration | Prevents fork adapter rot (cond) |
| C2 | (procedure) Story 1.2.3 | fork_ci_green_and_tag_cut_before_replace_bump | Integration | PROCEDURAL; not enforceable offline (GA11) |
| C3: lock discipline | A/gogit_vcs_reader_repostate_test.go | TestAheadBehind_ShouldNotDeadlock_WhenThreeWorktreesAndCommitMessagesRunConcurrentlyUnderRace | Unit | G10; completion by channels; deadline per GA9 |
| C3 | A/gogit_vcs_reader_repostate_test.go | TestDeadlockHarness_ShouldDetectHang_WhenAdapterTakesEntryMuInsideWalk (neg) | Unit | Proves the G10 harness can fail |
| C3 | A/gogit_vcs_reader_repostate_test.go | TestOpenRepoEntry_ShouldNotCallGitCommonDirOrResolveGitFilesystems_WhenDerivingKey | Unit | `go/ast` body inspection |
| C3 | A/aheadbehind_test.go | TestAheadBehind_ShouldReleaseEntryMuBeforeGraphWalk_WhenSharedPath | Unit | Hook asserts `entry.mu` unlocked at first `Fill` |
| C3 | A/scanstate_lint_test.go | TestNewScannerFiles_ShouldNotReadInstanceFieldsRaw_WhenParsedByAST | Unit | `instance-lock-free-reads.md` applies to any new `*Instance` read |
| C3 | GS/extension_test.go | TestExtensionClose_ShouldBeDetected_WhenCloseCallsBackIntoRegistry (neg) | Unit | Callback-contract harness |
| C3 | GS/extension_test.go | TestRepoStateCloseAndBytes_ShouldPassCallbackContract_WhenRunUnderRegistryLock | Unit | Touches only atomics |
| C3 | A/aheadbehind_integration_test.go | TestAheadBehind_ShouldCompleteUnderRace_WhenSiblingsWalkAndPruneRepoCacheClosesStorer | Integration | Closed-storer mid-walk maps to legacy fallback (Story 2.1.2 note) |
| C4: alloc-free hits, no sleeps | RS/cache_test.go | TestRepoCacheGet_ShouldAllocZero_WhenStructKeyHit (!race) | Unit | `AllocsPerRun == 0` |
| C4 | A/commitmessages_test.go | TestCommitMessages_ShouldAllocZero_WhenPrimedWithinTTL (!race) | Unit | Fails on old `fmt.Sprintf` key |
| C4 | A/aheadbehind_gate_test.go | TestAheadBehind_ShouldAllocZero_WhenMemoHitWithinTTL (!race) | Unit | G3(a) memo-hit path only |
| C4 | A/aheadbehind_gate_test.go | TestAheadBehindFrom_ShouldAllocZeroAndDecodeZero_WhenGraphPrimedWithCallerScratch (!race) | Unit | G3(b) |
| C4 | A/aheadbehind_gate_test.go | TestAheadBehindPhase1_ShouldStayUnderLegacyAllocCeiling_WhenTTLExpired (!race) | Unit | Phase 1 is not claimed alloc-free; ceiling `<=` legacy phase 1 |
| C4 | A/racetag_test.go | TestAllocGateFiles_ShouldCarryNoRaceTagAndNoShortGuard_WhenParsed | Unit | Parser check: every `AllocsPerRun` test file has `!race` and no `testing.Short()` |
| C4 | RS/cache_test.go | TestRepoCache_ShouldCoalesceToOneCompute_WhenEightGoroutinesRequestSameKey | Unit | Latch, no sleep; `Coalesced == 7` |
| C4 | Makefile `lint-custom` | notimesleeptest / novartestseam clean over new files | Integration | CI gate |
| C5: make ready | Makefile | `make ready-complexity-gate` (dupl, gocyclo, gocognit, funlen, revive) clean on diff | Integration | New-code-only vs `origin/main` |
| C5 | Makefile | `make lint-custom` (noarchivedrevival, norawgitopen, hotpolllog) clean | Integration | |
| C5 | A/commit_parents_test.go | TestParentsAdapters_ShouldShareOneCorpusHelper_WhenParentMemoStockForkTested | Unit | Structural anti-`dupl` check: one table-driven corpus symbol referenced by 3 test files |
| N1: no repo-wide lock across stat | A/vcsreader_lock_test.go | TestHasUncommitted_ShouldLetSiblingScanFinish_WhenFirstWorktreeBlockedInStatFn | Unit | Block `statFn` via channel; sibling scan completes (no `entry.mu` held) |
| N1 | A/vcsreader_lock_test.go | TestStatWalkLockTest_ShouldFail_WhenEntryMuHeldAcrossWalk (neg) | Unit | Proves the test can fail |
| N1 | RS/graph_test.go | TestFill_ShouldNeverReadStorerUnderRepostateLock_WhenInstrumentedHookObservesLockState | Unit | Hook asserts no repostate lock held at storer read |
| N1 | (script) Task 0.1.1e | scanner_share_of_mutex_delay_on_s1_should_be_recorded_and_gate_epoch_2_3 | Integration | Measured; decides Epic 2.3 |
| N2: scalability | RS/reach_test.go | TestReachBitset_ShouldBeUnder8KB_When50000Commits | Unit | G4 |
| N2 | RS/graph_test.go | TestGraphBytes_ShouldBeUnder120BPerRow_When50000Rows | Unit | G7 |
| N2 | RS/graph_test.go | TestGraph_ShouldReturnErrGraphFullAndNeverExceedCap_When8GoroutinesAtCapMinus1CapCapPlusN | Unit | Error path; counter cannot wrap |
| N2 | RS/trees_test.go | TestTreeMap_ShouldUse40BPerFileAnd50PctOfLegacy_When20000Files (cond 3.1.1) | Unit | Memory gate |
| N2 | RS/graph_test.go | TestGraph_ShouldStayWithinBudget_WhenChurnedHistory20RoundsOfAbandonedTips | Unit | `Bytes() <= MaxGraphCommits * G7` |
| N2 | A/scale_integration_test.go | TestAheadBehind_ShouldCompleteAndHonourCap_WhenHistoryIs52kCommits | Integration | Loose objects written directly via go-git storer (not 52k `Worktree.Commit`); not behind `-short` |
| N2 | A/scale_integration_test.go | TestHasUncommittedClean_ShouldWork_When20000TrackedFiles | Integration | Fixture shared with G8 |
| N2 | A/scale_integration_test.go | TestRefsBalanced_ShouldReturnToZero_When24WorktreesOpenedEvictedForgotten | Integration | "Tens of worktrees" (plan's tests top out at W=8/18; GA16) |
| N3: security/residency | TW/corpus_test.go | TestCorpusBuilder_ShouldOpenSourceReposReadOnly_WhenCopying | Unit | Owner originals unmodified (hash before/after) |
| N3 | TW/corpus_test.go | TestCorpusCopy_ShouldDropGitWorktreesDir_AndLeakNoSourcePath | Unit | Error path: no copied `.git/worktrees/*` file mentions a source path |
| S1: shared caches by (common dir, hash) | GS/open_test.go | TestCommonDirKey_ShouldBeIdentical_WhenOpenedViaSymlinkedPaths | Unit | `/tmp/r` vs `/private/tmp/r`, registry holds 1 entry |
| S1 | GS/open_test.go | TestCommonDirKey_ShouldEqual_WhenMainAndLinkedWorktree | Unit | Same key |
| S1 | GS/open_test.go | TestCommonDirKey_ShouldFallBackAndCount_WhenWorktreeDangling | Unit | Error path |
| S1 | GS/extension_test.go | TestExtension_ShouldRunNewFnOnce_WhenEightGoroutinesCall | Unit | |
| S1 | GS/extension_test.go | TestExtension_ShouldCloseOnceAndCountBytes_WhenStorePruned | Unit | `approxBytesPerStore` includes `Bytes()` |
| S1 | GS/extension_test.go | TestExtensionsDo_ShouldNotHoldRegistryLockDuringCallback | Unit | |
| S1 | A/gogit_vcs_reader_repostate_test.go | TestRepoState_ShouldSurviveSiblingEviction_WhenOneOfThreeCachedReposPruned | Unit | RefCounts 3 -> 2; same `Extension` instance |
| S1 | A/gogit_vcs_reader_repostate_test.go | TestForgetWorktree_ShouldNotEvictSharedReach_WhenSiblingStillUsesIt | Unit | |
| S1 | A/gogit_vcs_reader_repostate_test.go | TestPruneToMemoryBudget_ShouldDropProductsThenGraphThenInterner_WhenOverBudget | Unit | ADR-007 order |
| S1 | A/gogit_vcs_reader_repostate_test.go | TestUnderSeverePressure_ShouldSkipFillAndServeFilledRows_WhenPressureFuncTrue | Unit | Error path: `ErrPressure` -> legacy walk |
| S1 | RS/products_test.go | TestAheadBehindMemo_ShouldHitOnSecondWorktree_WhenSameHeadAndBase | Unit | `Hits == 1`, decode delta 0 |
| S1 | RS/products_test.go | TestAheadBehindMemo_ShouldMiss_WhenShallowStampChangesAfterDeepen | Unit | Error path (key completeness) |
| S1 | RS/products_test.go | TestBehindMemo_ShouldRunOnce_WhenTwoHeadsShareMergeBase | Unit | |
| S1 | RS/products_test.go | TestMessageMemo_ShouldKeyByHashNotCommitID_WhenEpochChanges | Unit | |
| S1 | RS/products_test.go | TestMemoKey_ShouldMiss_WhenMergeBaseBFSLimitLoweredThenRestored | Unit | Cap in key |
| S1 | RS/products_test.go | TestAheadBehind_ShouldOnlyRerunPhase1_WhenHashesUnchangedAfterTTL | Unit | |
| S1 | RS/trees_test.go | TestHeadTreeMap_ShouldWalkOnce_WhenThreeWorktreesAtSameTree (cond 3.1.1) | Unit | `allocs/op(W=3) <= 1.2x(W=1)` |
| S1 | RS/trees_test.go | TestHeadTreeMap_ShouldWalkOnlyDifferingSubtrees_WhenHeadMoves3Of20000 (cond) | Unit | |
| S1 | RS/trees_test.go | TestPathInterner_ShouldBumpEpochAndPurgeTreeMaps_WhenOverBudget (cond) | Unit | Old `TreeMap` unreadable after bump |
| S1 | RS/trees_test.go | TestHeadTreeMap_ShouldReturnNilNilAndCacheNothing_WhenUnbornHead (cond) | Unit | Error path |
| S1 | RS/patterns_test.go | TestMatcherParseMemo_ShouldKeepPerWorktreeComposition_WhenGitignoreEditsDiffer (cond 3.3.1) | Unit | |
| S1 | A/gogit_vcs_reader_repostate_test.go | TestRefsBalanced_ShouldReturnToZero_WhenRandomizedOpenEvictForgetClearPanicInterleaving200Iterations | Integration | `assertRefsBalanced` via `Registry.RefCounts()` |
| S1 | A/multiworktree_integration_test.go | TestRepoState_ShouldBeSharedByTwoSymlinkedPaths_WhenScannerOpensBoth | Integration | End to end on macOS `/var` vs `/private/var` |
| S2: header-only reader / commit-graph | A/commit_parents_test.go | TestCommitParentsReader_ShouldSelectStock_WhenSsqforkTagAbsent | Unit | Selector + startup INFO `commit_parents_reader=stock` |
| S2 | A/commit_parents_select_fork_test.go | TestCommitParentsReader_ShouldSelectFork_WhenBuiltWithTag (cond) | Unit | Metric `commit_parents_reader{impl}=1` |
| S2 | A/commit_parents_test.go | TestCommitParentsStock_ShouldCopyParentsIntoCallerBuffer_AndCapPooledBuffer64KiB | Unit | No aliasing of pooled memory |
| S2 | A/commit_parents_test.go | TestCommitParentsStock_ShouldSurviveHugeHeader_WhenGpgsigOver64KiB | Unit | Error path: pooled buffer cap |
| S2 | A/graph_source_test.go | TestGraphSource_ShouldTreatClosedStorerAsFallback_WhenStorerClosedMidFill | Unit | Error path |
| S2 | A/graph_source_test.go | TestGraphSource_ShouldTakeNoEntryMu_WhenReadingParents | Unit | |
| S2 | RS/graph_base_test.go | TestGraphBase_ShouldBehaveIdenticallyToInProcessIndex_WhenNoCommitGraphFile (cond 5.2.3) | Unit | 9 of 10 repos |
| S2 | RS/graph_base_test.go | TestGraphBase_ShouldServeCoveredFromFileAndNewerFromDelta_WhenStaleV1Graph (cond) | Unit | stapler-squad-like stale graph |
| S2 | RS/graph_base_test.go | TestGraphBase_ShouldReopen_WhenCommitGraphRewrittenInProcess (cond) | Unit | Error path: no slice held across replacement |
| S2 | A/edgecases_test.go | TestScan_ShouldIgnoreCommitGraphFile_WhenGoGitNeverLoadsIt | Integration | Hand-written v1 header file present; results equal without it (RH3) |
| S2 | (script) Story 1.2.3 | probe_should_yield_ssqfork_iff_module_has_symbol | Integration | Makefile `.cache/ssq-go-tags.mk`; empty after `replace` removed; works at `ssq.10` |
| S2 | (CI) Story 1.2.3 | make_check_go_tags_should_fail_when_go_line_lacks_tags | Integration | Error path |
| S3: dirty scales with changes | A/index_cache_test.go | TestParsedIndexCache_ShouldSkipDecode_WhenIndexStatSigUnchanged | Unit | Counter: `index.Decode` not called |
| S3 | A/index_cache_test.go | TestParsedIndexCache_ShouldReparse_WhenIndexRewrittenWithin2sOfParse | Unit | Racy guard (error path) |
| S3 | A/index_cache_test.go | TestHasUncommitted_ShouldReportDirty_WhenTrackedFileEditedInPlaceAndIndexCacheHit | Unit | Cache removes decode, never the stat pass |
| S3 | A/index_cache_test.go | TestHasUncommitted_ShouldNotReportDeleted_WhenSkipWorktreeEntryAbsent | Unit | Sparse guard; deliberate-fix callout if today reports deleted |
| S3 | A/index_cache_test.go | TestStagedVsHead_ShouldBeO1_WhenGoGitWrittenCacheTreeValidAndAbsentToleranceHolds (cond 4.2.1) | Unit | |
| S3 | A/index_cache_test.go | TestHasUncommittedAllocs_ShouldGrowSublinearlyInFiles_WhenIndexCacheHits (proposed, GA7) | Unit | allocs(20k)/allocs(5k) <= 1.5; the only deterministic "scales with changes" shape gate |
| S3 | A/statpass_test.go | TestStatCount_ShouldEqualTrackedFiles_WhenCleanScan | Unit | Documents accepted O(n) floor |
| S3 | A/statpass_test.go | TestStatPassHandoff_ShouldNotExceedTrackedFilesOrLegacy_WhenDirtyWorktreeScanned (cond 4.2.4 build) | Unit | G6 |
| S3 | A/statpass_test.go | TestStatPassHandoff_ShouldUseConsistentVerdict_WhenFileEditedBetweenPasses | Unit | No new staleness; next scan sees edit |
| S3 | A/statpass_test.go | TestDuplicateStatFraction_ShouldBeRecorded_BeforeBuilding | Unit | < 20% closes story as accepted |
| S3 | A/untracked_cache_test.go | TestUntrackedDirCache_ShouldRewalkDir_WhenNewFileCreatedAndMtimeChanges | Unit | |
| S3 | A/untracked_cache_test.go | TestUntrackedDirCache_ShouldRewalkOnlyLeaf_WhenOnlyNestedDirChanges | Unit | |
| S3 | A/untracked_cache_test.go | TestUntrackedDirCache_ShouldReportUntracked_WhenIntermediateGitignoreAddedWithNegation | Unit | Ancestor marker |
| S3 | A/untracked_cache_test.go | TestUntrackedDirCache_ShouldReportUntracked_WhenGitignoreEditedInPlaceToUnignore | Unit | |
| S3 | A/untracked_cache_test.go | TestUntrackedDirCache_ShouldReportUntracked_WhenInfoExcludeOrGlobalExcludesEdited | Unit | |
| S3 | A/untracked_cache_test.go | TestUntrackedDirCache_ShouldReportUntracked_WhenIndexEntryRemovedKeepingFile | Unit | go-git storer equivalent of `git rm --cached` |
| S3 | A/untracked_cache_test.go | TestUntrackedDirCache_ShouldRewalk_WhenCheckoutChangesTrackedSet | Unit | |
| S3 | A/untracked_cache_test.go | TestUntrackedDirCache_ShouldRewalk_WhenDirRemovedRenamedOrAged5Min | Unit | `FullWalkFallback` |
| S3 | A/untracked_cache_test.go | TestUntrackedDirCache_ShouldRewalk_WhenDirMtimeWithin2sOfStore | Unit | Racy guard |
| S3 | A/untracked_model_test.go | TestUntrackedSkip_ShouldEqualFullWalk_WhenSeededModelRuns1000SequencesWithEventDrops | Unit | Must assert engagement: `skipped dirs > 0` per sequence and mtimes backdated via `testrepo.Age` (GA2) |
| S3 | A/untracked_model_test.go | TestUntrackedModel_ShouldFail_WhenAncestorMarkerRemovedFromKey (neg) | Unit | Mutation check on the oracle |
| S3 | A/dirtygen_test.go | TestDirtyGenPolicy_ShouldRunStatPass_WhenTierAArmedAndHealthy | Unit | Hard rule: Tier A never licenses clean skip |
| S3 | A/dirtygen_test.go | TestDirtyGenPolicy_ShouldFullWalk_WhenWatcherNotArmedOrFirstScanAfterArming | Unit | |
| S3 | A/dirtygen_test.go | TestDirtyGenPolicy_ShouldFullWalk_When5MinElapsedOnInjectedClock | Unit | |
| S3 | A/dirtygen_test.go | TestDirtyGenPolicy_ShouldFullWalkAndCountReason_WhenOverflowErrorOrDirEvent | Unit | `dirty_full_walk_total{reason}` |
| S3 | A/dirtygen_test.go | TestDirtyGenPolicy_ShouldReturnDirtyWithoutFullWalk_WhenEventOnTrackedPath | Unit | |
| S3 | A/scanner_watch_seam_test.go | TestWatchRepo_ShouldWatchLinkedWorktreeGitdir_WhenTierAEnabled | Unit | HEAD/index event bumps `DirtyGen(wt1)`, schedules scoped Burst |
| S3 | A/scanner_watch_seam_test.go | TestWatchRepo_ShouldWatchResolvedGitdir_WhenDotGitIsPointerFile | Unit | |
| S3 | A/scanner_watch_seam_test.go | TestClassifyGitEvent_ShouldTreatPackedRefsRenameLikeCreate_WhenMacLockfilePattern | Unit | |
| S3 | A/scanner_watch_seam_test.go | TestWatcher_ShouldFullWalkEveryWorktree_WhenFakeWatcherInjectsErrEventOverflow | Unit | Error path; no `time.Sleep` |
| S3 | A/dirtyset_test.go | TestTierB_ShouldOpenNoWatch_WhenFlagUnset (cond 4.3.3) | Unit | `dirty_tierb_armed==0` |
| S3 | A/dirtyset_test.go | TestTierB_ShouldFullWalk_WhenArmedWithRecordedLossAndZeroEvents (cond) | Unit | |
| S3 | A/scanner_linked_worktree_test.go | TestScanner_ShouldRescanLinkedWorktree_WhenHeadChangesWithTierAOn | Integration | Real fsnotify on temp dir, `wait.WaitForCondition` |
| S3 | A/scanner_linked_worktree_test.go | TestListWorktrees_ShouldReturnMainAndLinked_WhenCalledFromLinkedPath | Integration | Characterization |
| S3 | A/dirtyset_spike_test.go | TierB_kqueue_spike_should_report_fd_cost_and_in_place_edit_visibility (cond, owner-blocked) | Integration | Deleted after findings recorded in ADR-006 |
| S4: diff + lean walkers | A/myers_test.go | TestMyers_ShouldReturnZeroEdits_WhenInputsIdentical | Unit | |
| S4 | A/myers_test.go | TestMyers_ShouldReturnAllInsertionsOrDeletions_WhenOneSideEmptyOrDisjoint | Unit | |
| S4 | A/myers_test.go | TestMyers_ShouldTrimPrefixSuffix_WhenChangeInMiddle | Unit | |
| S4 | A/myers_test.go | TestMyersBytes_ShouldEqualLcsLengthBytes_WhenSeededPairs | Unit | Twin for `lcsLengthBytes` |
| S4 | A/diffcounts_spike_test.go | TestDiffCandidates_ShouldReportMismatchCountsVsOracle_When10000PairsEach | Unit | ADR-005 decision rule |
| S4 | A/diffmemo_test.go | TestFileDiffMemo_ShouldDoZeroBlobReadsAndDiffs_WhenFileUnchanged | Unit | Backdated mtime (GA2) |
| S4 | A/diffmemo_test.go | TestFileDiffMemo_ShouldMiss_WhenSizeMtimeCtimeOrInodeChanges | Unit | Includes equal-length edit with `os.Chtimes` restore |
| S4 | A/diffmemo_test.go | TestFileDiffMemo_ShouldNotTrust_WhenModifiedWithin2sOfStore | Unit | Racy guard |
| S4 | A/diffmemo_test.go | TestFileDiffMemo_ShouldBeDisabled_WhenCtimeUnavailable | Unit | `statdir_other.go` build |
| S4 | A/diffmemo_test.go | TestFileDiffMemo_ShouldStayUnderBudgetAndCountEvictions_When10000Files | Unit | |
| S4 | A/walkers_alloc_test.go | TestUntrackedWalker_ShouldMeetAllocCeilingVsLegacy_WhenAssignedTo441 (cond) | Unit | Row-specific |
| S4 | A/walkers_oracle_test.go | TestWalkers_ShouldReportSameVerdicts_WhenScanSkipOnAndOff | Integration | Parity with `gitignore_invalidation_test.go:71,113,146` |
| S5: scheduling/coalescing | A/scanner_prelude_test.go | TestBurst_ShouldResolveBaseOnceAndEachHeadOnce_WhenFourWorktrees | Unit | Counters + `shallow` stat once |
| S5 | A/scanner_prelude_test.go | TestRefSnapshot_ShouldRebuild_WhenGitEventArrivesBetweenBursts | Unit | |
| S5 | A/scanner_prelude_test.go | TestBurst_ShouldNotFillGraph_WhenSeverePressureOrAtStart | Unit | No eager graph work |
| S5 | A/scanner_signature_test.go | TestScanResultCoverageTable_ShouldFail_WhenNewFieldMissingFromTable | Unit | Reflection over `ScanResult` |
| S5 | A/scanner_signature_test.go | TestScanSignature_ShouldReuseCommittedFieldsAndStillRunHasUncommitted_WhenSiblingCommits | Unit | |
| S5 | A/scanner_signature_test.go | TestScanSignature_ShouldRecompute_WhenShallowStampOrBaseHashChanges | Unit | Error path |
| S5 | A/scanner_signature_test.go | TestScanOrder_ShouldProcessChangedWorktreesFirst_WhenW1W3ChangedW2Not | Unit | |
| S5 | A/scanner_signature_test.go | TestEnqueueRepo_ShouldNotSkipRepo_WhenSiblingSignatureChanged | Unit | |
| S5 | A/scanner_signature_test.go | TestScanPool_ShouldRunSequentially_WhenPoolOptionUnset | Unit | |
| S5 | A/scanner_test.go | TestClassifyGitEvent_ShouldKeepCommitAndFetchRepoWide_AndDebounceFlushAtomically | Unit | Existing `c46c69faf`/`150168a4c` behaviour pinned |
| S5 | A/scanner_prelude_test.go | TestBurstThreeWorktrees_ShouldDecodeOneSharedWalk_WhenCommitInOneWorktree | Integration | Full Scanner + counting storer + injected clock |
| S5 | (script) Story 5.2.1 | signature_skip_engagement_fraction_should_be_recorded | Integration | If < 10%, projection contribution set to 0 |
| S6: memory bound + gates | RS/cache_test.go | TestRepoCache_ShouldEvictLRUAndCountEviction_WhenBudget1000AndThree400ByteEntries | Unit | `Evictions==1`, `Bytes==800` |
| S6 | RS/cache_test.go | TestRepoCache_ShouldNotStoreErrors_WhenComputeReturnsErrNotFound | Unit | Error path |
| S6 | RS/state_test.go | TestRepoStateBytes_ShouldSumWithoutWalking_AndPruneInADR007Order | Unit | |
| S6 | RS/state_test.go | TestGenerationSwap_ShouldBumpEpochAndNeverMixIds_WhenGraphDroppedMidOperation | Unit | |
| S6 | A/countingstorer_test.go | TestCountingStorer_ShouldCountOnlyCommitEncodedObjects | Unit | Metric definition of `DecodeCount` (GA8) |
| S6 | A/countingstorer_test.go | TestStatFn_ShouldCountLstatCalls_When1000FileWorktreeScanned | Unit | `StatCount` |
| S6 | TR/testrepo_test.go | TestCrissCrossFixture_ShouldHaveTwoBestBases_WhenMergeBaseRunOnDE | Unit | Fixture self-check returns {B,C} |
| S6 | TR/testrepo_test.go | TestTestrepoAge_ShouldBackdateMtimeAndCtimeInputs_WhenCalled (proposed, GA2) | Unit | Needed for every racy-guard gate |
| S6 | TW/driver_test.go | TestDriver_ShouldProduceIdenticalOpLogHash_WhenSameSeedOnIdenticalCorpora | Unit | No wall-clock reads |
| S6 | TW/driver_test.go | TestDriver_ShouldUseCommitRateFromCorpusLog_NotFromProfile | Unit | Plan K3-adjacent: realism check never tunes commit-walk bucket |
| S6 | TW/corpus_test.go | TestCorpusSeed_ShouldGiveIdenticalPinnedReposOnBothInstances | Unit | `StateStore.PinnedRepos()` equal |
| S6 | A/gogit_vcs_reader_edgecases_test.go | TestSymlinkedPathKey_ShouldYieldOneRegistryEntry_WhileGitCommonDirYieldsTwoStrings | Unit | Characterization |
| S6 | (script) Task 0.1.1b | pprof_cpu_alloc_inuse_mutex_block_before_after_in_baseline_md | Integration | Procedural; GA5 |
| O1: observability | A/metrics_test.go | TestMetrics_ShouldExportAllRepostateSeriesAsZero_WhenFlagOffOrRepoStateUnallocated | Unit | Series present, value 0 |
| O1 | A/metrics_test.go | TestMetrics_ShouldMoveHitMissEvictionBytes_WhenThreeWorktreesScanned | Unit | `reach misses=1, hits>=2` |
| O1 | A/metrics_test.go | TestMetrics_ShouldExposeFillLostCasDecodeReprobeRollbackAndCasRetries | Unit | Counters feed G2b/G11 |
| O1 | A/scanner_test.go | TestScanCycleLog_ShouldEmitOneDebugLineAndNothingAtDefaultLevel | Unit | `hotpolllog` clean |
| O1 | A/metrics_test.go | TestStartupLog_ShouldEmitOneInfoWithParentsReaderAndFlagValues | Unit | |
| O1 | (CI) Story 5.1.1 | make_registry_generate_and_lint_should_report_no_registry_gap | Integration | If plain HTTP handler used, record why (plan UNVERIFIED) |
| R1: flags, rollback, rollout | A/flags_test.go | TestEffectiveFlags_ShouldForceDependentOff_WhenDependencyOffAndWarnOnce | Unit | Normalizer over legal set |
| R1 | A/flags_test.go | TestFlagOff_ShouldNeverTouchExtensionSlot_WhenSharedCommitStateFalse | Unit | Error path: legacy only |
| R1 | A/flags_test.go | TestEachRollbackUnit_ShouldRestoreLegacyResults_WhenFlagTurnedOff | Unit | Parity per unit |
| R1 | A/flags_test.go | TestFlags_ShouldBeRestartOnly_WhenConfigChangesAfterStartup | Unit | Atomic snapshot set once |
| R1 | config/config_test.go | TestUnfinishedCachesConfig_ShouldDefaultAllFalse_AndLoadPerInstanceConfigJson | Unit | Defaults off until soak flip |
| R1 | A/flags_test.go | TestMergeBaseMode_ShouldDefaultToLockstep_WhenBestMergeBaseOff | Unit | |
| R1 | A/flags_test.go | TestTierBAndBestMergeBase_ShouldDefaultOff_WhenConfigUntouched | Unit | Owner-gated flags |
| R1 | A/legacy_removal_test.go | TestOraclesSurvive_ShouldRunInTestFiles_WhenLegacyPathsRemoved (cond 5.3.1) | Unit | `legacyStack*`, lockstep, `lcsLength`, map reach set remain `_test.go` |
| R1 | (script) Story 5.2.1 | isolation_should_show_only_6287x_ports_and_live_8543_untouched | Integration | `ss -ltnp` check |
| R1 | (procedure) Story 5.2.2 | live_deploy_config_has_flags_on_and_keep_server_verified | Integration | Procedural |
| R1 | Makefile / script | rollback_one_fork_tag_back_should_build_stock_reader | Integration | `-modfile` replace `ssq.2` -> `ssq.1` (requirement wording; plan only rehearses full drop, GA10) |
| R1 | Makefile / script | rollback_drop_replace_go_get_tidy_should_build_and_test | Integration | Runbook rehearsal; network |
| R1 | A/flags_test.go + script | TestFlagDefaultsFlip_Migration_ShouldBeReversible_WhenFlippedThenRevertedByConfig | Migration | `migration_should_be_reversible` analogue: flip defaults, parity suite green, revert, legacy results restored |
| R1 | script | TestForkReplace_Migration_ShouldBeReversible_WhenAdoptedThenRolledBack | Migration | Up: `replace` at `ssq.2` + `-tags ssqfork` builds and passes differential tests; down: stock reader builds without tag (cond 1.2.3) |
| RH1: sharing boundary | A/edgecases_test.go | TestShallowFile_ShouldLiveInCommonDir_WhenLinkedWorktreeOfShallowRepo | Unit | Resolves Unresolved Q9 before Story 2.1.2 |
| RH1 | A/edgecases_test.go | TestResolveHeadTreeHashes_ShouldReturnNilNil_WhenUnbornBranch | Unit | |
| RH1 | A/edgecases_test.go | TestResolveHeadTreeHashes_ShouldRecordGitlinkAsBlobLike_WhenSubmodule | Unit | Parity bug documented |
| RH1 | A/edgecases_test.go | TestScan_ShouldHandleSparseCheckoutAndBareRepo_WhenCharacterized | Unit | |
| RH1 | A/multiworktree_integration_test.go | TestSharedGraph_ShouldGiveDifferentResultsPerWorktree_WhenHeadsDiffer | Integration | Shared graph, per-worktree HEAD/index |
| RH1 | A/gogit_vcs_reader_gitignore_test.go | TestMatcher_ShouldIgnoreInfoExclude_WhenCharacterizedAndNotShared | Unit | No semantic change; per-worktree matcher |
| RH1 | RS/graph_test.go | TestShallowDeepen_ShouldReprobeTruncatedRowToDeeperAnswer_WhenStampChanges | Unit | Equals cold legacy walk |
| RH1 | RS/graph_test.go | TestShallowDeepen_ShouldStampOldOrdinal_WhenDeepenLandsMidWalk | Unit | Error path: deepen-during-walk |
| RH2: concurrency + eviction | RS/graph_test.go | TestGraphRowStates_ShouldAcceptOnlyListedTransitions_WhenTransitionTableDriven | Unit | |
| RH2 | RS/graph_test.go | TestFill_ShouldRollBackFillingToPending_WhenHolderErrorsPanicsCancelsOrStorerCloses | Unit | Four exits; `fill_rollback_total` |
| RH2 | RS/graph_test.go | TestFill_ShouldPublishPayloadExactlyOnce_WhenEightGoroutinesRaceOn1000RowsUnderRace | Unit | `written` guard tag |
| RH2 | RS/graph_test.go | TestSegments_ShouldNeverMove_AndDirectoryShouldNotGrow_WhenRowsAppended | Unit | Reflection: pointer-free segment types |
| RH2 | RS/graph_test.go | TestGraph_ShouldKeepFilledRowChecksumStable_When5000RowsAppended | Unit | Id stability |
| RH2 | RS/graph_test.go | TestGraphHotAtomics_ShouldBeCachelinePadded_WhenLayoutInspected | Unit | |
| RH2 | RS/graph_test.go | TestGraphNoGenerationColumn_ShouldHold_WhenBestMergeBaseOff | Unit | |
| RH2 | RS/graph_fuzz_test.go | FuzzGraph_ShouldEqualMapReference_WhenRandomDAGs | Unit | |
| RH2 | RS/reach_test.go | TestReachBitset_ShouldNeverBeWrittenAfterPublish_WhenIncrementalRebuildRaces (cond) | Unit | `-race` |
| RH2 | RS/reach_test.go | TestEvictAndReingest_ShouldEqualLegacy_WhenDifferentIdOrderOnRandomDAGs (cond) | Unit | Epoch property test |
| RH2 | RS/linearize_test.go | TestHistory_ShouldLinearizeAgainstSequentialReference_When20SeedsEightGoroutines | Unit | G11 |
| RH2 | RS/linearize_test.go | TestLinearizeChecker_ShouldReject_WhenFilledRowObservedPendingAfterwards (neg) | Unit | Proves checker can fail |
| RH2 | RS/stress_test.go | TestStress_ShouldHaveNoViolationOrLeak_WhenYieldHookAtGOMAXPROCS1_2_8 | Unit | `goleak`; G2b bound |
| RH2 | RS/cache_clock_test.go | TestClockEviction_ShouldMatchLRUObservableResult_AndNeverDeleteReinsertedKey (cond 2.3.1) | Unit | |
| RH2 | RS/bitset_atomic_test.go | TestAtomicBitset_ShouldSetExactlyUnionOfBits_WhenEightWritersUnderRace (cond 2.3.2) | Unit | |
| RH2 | RS/graph_bench_test.go | BenchmarkGraphFillParallel (arena vs mutex reference, 1/3/8 goroutines, disjoint and hot row) | Integration | `allocs/op`, `B/op` arena <= reference hard (in a companion test); ns/op advisory |
| RH3: commit-graph independence | RS/graph_base_test.go | (see S2 rows: no-file, stale v1, reopen) | Unit | Covered by the three S2 `graph_base` tests and `TestScan_ShouldIgnoreCommitGraphFile_...` |
| RH4: watcher safety | A/untracked_model_test.go | (see S3 model-based row) | Unit | Random event drops vs full walk |
| RH4 | A/dirtygen_test.go | (see S3 policy rows) | Unit | Overflow, errors, dir events, 5 min bound, unarmed |
| RH4 | A/dirtygen_test.go | TestStalenessWindow_ShouldBeDocumentedPerMechanism_WhenADR006Read (proposed) | Unit | ADR-006 table lint: each mechanism lists worst-case false-clean window |
| RH5: diff parity | A/diffcounts_oracle_test.go | (see M5 rows: 10,000 pairs; 20,001-line approximation) | Unit | Insertions = len(new)-LCS, Deletions = len(old)-LCS |
| RH5 | A/diffcounts_oracle_test.go | TestEditCounts_ShouldMatchHandComputedExample_WhenAxyzVsXzw | Unit | `a=[x,y,z]`, `b=[x,z,w]` gives LCS 2, (1,1) |
| RH6: fork rebase burden | fork/ssq_additive_test.go | (see C2 rows: additive test, differential) | Unit | Per-patch additive check; probe works across rebases |
| RH6 | (script) | probe_should_return_empty_after_replace_removed_and_set_at_ssq10 | Integration | Same probe as S2 row |

## UX Acceptance Tests
Omitted: no `design/ux.md` exists and the project has no user-facing surface (infrastructure re-architecture of a background scanner). The only UI-adjacent effect, scanner results shown in Unfinished/Up Next, is covered by the `ScanResult` parity rows (M5) and the `Story 5.2.1` per-event comparison script.

## Test Stack
- **Unit**: Go `testing` + `testify` (`assert`/`require`), `testing.AllocsPerRun` in `//go:build !race` files, seeded `math/rand` generators (no `testing/quick` seeds from the clock), `go/parser`/`go/ast` for structural checks, `goleak`, `-race` for all `repostate` concurrency tests. Fixtures only through `session/unfinished/internal/testrepo` (go-git: `PlainInit`, `Worktree.Commit`, `Storer.SetEncodedObject`, `RepackObjects`, hand-written `.git/worktrees/<n>`).
- **Integration**: real filesystem `gogitstore` with `countingStorer` (`DecodeCount`), injectable `statFn` (`StatCount`), fake `gitWatcher` seam plus real `fsnotify` on temp dirs for the two linked-worktree tests, injected clock; `-benchtime=Nx -benchmem` benchmarks for output, with the pass/fail gate living in a `Test*`.
- **Measured / procedural** (rows marked "(script)", "(procedure)", "(CI)"): manual A/B on `:62871`/`:62873` driven by `tools/scanner-workload`, pprof or Pyroscope, `make ready`, fork `ssq-ci`. These cannot run in `go test`; their verdict logic is unit-tested in `TW/compare_test.go`.
- **E2E / UX**: none.
- **Forbidden**: `os/exec`, `safeexec`, `git`, `cp` aliases, `time.Sleep`, wall-clock ns/op thresholds. Tests that need a toolchain query (`go list`, `go doc`, `go vet`) run as Makefile/CI steps, not inside `go test` (GA13).

## Coverage Targets and How to Measure

| Stack | Coverage command | Target |
|---|---|---|
| Go (`repostate`, new `unfinished` files) | `go test ./session/unfinished/... -coverprofile=coverage.out && go tool cover -func=coverage.out` | >= 80% line per new package/file; `repostate` >= 90% (lock-free state machine: every transition edge) |
| Go concurrency | `make test-race` | `repostate` arena/fill/walk/linearize/stress all run under `-race`, none behind `-short` |
| Go alloc gates | `make test` (non-race) | every `AllocsPerRun`/bytes gate executes; a skipped gate fails CI (check `go test -v` output for `SKIP` on gate names) |
| Bench list | `go test -run '^$' -list 'Benchmark' ./session/unfinished` (CI step) | every Tier-1 regex token resolves |

- All public `repostate` methods: happy + error path; all external integrations (storer, fsnotify, config): unit with fakes + >= 1 integration.
- No UX criteria (none exist).

## Gate Audit: gates that cannot actually fail, and gaps (priority order)

Numbers recomputed by command at the end of this file; every statement below is INFERRED from plan text unless it cites a file line.

- **GA1 Benchmarks do not fail.** Ceilings stated as `B/op`/`allocs/op` (G4 aside) in Stories 1.2.1, 3.2.2 (G5), 4.2.2 (G8), 4.4.1 are produced by `go test -bench`, which never exits non-zero on a number. Plan only says "AllocsPerRun-based ceiling" for G5 (Task 3.2.2b). Design: measure inside the `Test*` with fixed iteration count, single goroutine, `runtime.ReadMemStats` `TotalAlloc`/`Mallocs` deltas (or `testing.AllocsPerRun` for allocs), fixtures that spawn no background goroutines. Rows above follow this.
- **GA2 Racy-guard vacuity (new, material).** The plan guards three caches against same-granularity timestamps: parsed-index (re-parse if mtime within 2 s of parse), untracked dir pruning (re-walk if within 2 s of store), diff memo (not trusted within 2 s). A fast fixture writes the index/dirs/files milliseconds before the scan, so every guard fires and the cache *never engages*: G8 would fail spuriously, while the model-based oracle (>= 1,000 sequences) and the parity tests would pass vacuously because skip never runs. Fix: `testrepo.Age(path, d)` backdating via `os.Chtimes` (not native git) plus an injected clock, and an engagement assertion (skipped-dir / decode-skipped counter > 0) in every such test. Rows: `TestUntrackedSkip_..._Model...`, `TestTestrepoAge_...`, `TestFileDiffMemo_...`.
- **GA3 Flag-off vacuity.** Flags default off until the soak (ADR-008). A shared-path test that forgets to enable `shared_commit_state` exercises the legacy path: G2 then fails loudly (W-fold decodes), good, but alloc-free and parity tests would pass vacuously. Every shared-path test asserts the engagement precondition (`CacheStats.Hits+Misses > 0` on the repostate, or flags snapshot true), and `TestAheadBehind_ShouldDecodeWTimes_WhenSharedCommitStateFlagOff (neg)` pins the contrast.
- **GA4 "Regression test fails on the old behaviour" is asserted, never proven.** Verification Matrix row 4 says each regression test is "written to fail against the retained oracle". Nothing runs the gate against the oracle. Add `TestRegressionGates_ShouldAllFail_WhenLegacyImplementationIsWired` plus per-gate `(neg)` rows (G1, G5, G10, N1 stat-lock, linearizability checker, model oracle).
- **GA5 M1 and the absolute 15% cannot fail in CI by design.** Only measured at the live deploy (Story 5.2.2); rho = 15/s1 where `s1` may be an INFERRED estimate (`39.3 x (1 - lockstep gain)`, assumes allocation gain transfers one-for-one to CPU share) unless the owner approves an S1 live deploy. Owner-confirmed S1-relative numbers are a Wave B gate; until then M1 and M2 have no numeric gate at all. The `0.1.1d` projection is arithmetic over authors' own reduction guesses, so it can read PASS while reality misses. Mitigation is only the 5.2.1/5.2.2 fail-and-hold, which is measured at load 110-160 (CPU-seconds, not wall-clock, but cache contention still adds noise); the swapped-port repeat is the noise control.
- **GA6 requirements.md is stale relative to the plan.** The "Proposed requirements.md edits" (plan end) are not applied: M1/M2 in `requirements.md` still state S0 absolute figures and "2x from commit-walk stories" (decode-only ceiling 2.08x, requirements.md L23 itself notes it). Tests above follow the plan's S1-relative restatement; owner must confirm or the mapping to the literal requirement text is loose.
- **GA7 S3 ("scales with changed paths") is only partly deliverable and not directly testable.** By default the tracked-file stat pass runs every scan (Tier A cannot see in-place edits; Tier B off, owner-gated, kqueue unknown). Cost stays O(tracked files). Gates that exist: G8 (clean-index allocation ratio), G6 (duplicate stat pass), dir-mtime pruning (O(dirs)). None asserts change-proportional cost, so S3 can end "accepted" with every gate green. Added deterministic shape test `TestHasUncommittedAllocs_ShouldGrowSublinearlyInFiles_...` and `TestStatCount_ShouldEqualTrackedFiles_...` to document the floor honestly.
- **GA8 `DecodeCount` is narrower than it looks.** It counts `EncodedObject(CommitObject)` reads only (Gate definition). It cannot fail on per-decode byte cost (header-only reader benefit shows only in the bytes ceiling), nor on tree/blob reads (tree-map story needs its own "tree reads <= 3 x depth + root" counter, which the plan names but does not add to the counting storer). Add tree/blob counters to `countingStorer` so Story 3.1.1 can fail.
- **GA9 Concurrency gates are probabilistic and one is wall-clock.** G10's 5 s `select`/`time.After` deadline is a wall-clock bound; with `-race` at load 110-160 three cold walks can exceed it without any deadlock. Recommend >= 30 s (hang detection, not speed). G11 (fixed seeds, yield hook) fixes the *inputs*, not the OS scheduler: it raises detection odds but cannot prove absence of races; `-race` only reports races it observes. Treat as strong evidence, not proof (ADR-009 caveats agree).
- **GA10 R1 literal text vs plan.** (a) "feature flag per shared cache": met at six rollback units (ADR-008 deviation, deliberate). (b) "fork patch revertable by rolling the `replace` back one tag": plan rehearses dropping the `replace` (to upstream `v5.19.3`, needs network, `go.sum` has no `go-git/go-git/v5` entries), never ssq.2 -> ssq.1. Added `rollback_one_fork_tag_back_should_build_stock_reader` (deterministic offline if `ssq.1` is in the module cache).
- **GA11 Advisory/procedural gates.** `benchmark.yml` Tier 1 comparison is `|| true` (`:120`) and the PR comment `continue-on-error: true` (`:140`), VERIFIED by plan citations: "benchmarks in CI Tier 1" (S6) cannot fail a build; real gates are the `Test*` rows. "Fork CI green before `replace` bump" (C2) and "owner go-ahead before deploy" are human procedure with no automated check.
- **GA12 Conditional scope may legitimately not exist.** S2's "commit-graph usage" (Story 5.2.3, gate-triggered), "gitignore matcher shared where valid" (Story 3.3.1, closes if < 1% CPU), reach/tree-map sharing (2.2.1/3.1.1 scheduling gates), and F2 (fork patch, closes if < 10% `B/op`) can all close "not needed". Requirements S1/S2 would then be met only in part, by design, with the measured number recorded; their tests are marked (cond) and absent when the gate closes. A reviewer should not read "28/28 mapped" as "28/28 delivered".
- **GA13 Plan ACs that use subprocess toolchain commands.** `go list -deps` (Stories 0.0.3, 1.1.1), `go doc` probe, `go vet`, `go mod ... -modfile` are not git, but they are `exec` calls; inside `go test` they would trip `TestNoNativeGitInPackage`. Designed in-process replacements (`go/build` import walk) for the two dependency checks; the rest stay in Makefile/CI.
- **GA14 G2's "new vs new" half is a tautology.** `DecodeCount(new, W=3) == DecodeCount(new, W=1)` passes even if the new path is worse than legacy at W=1. Only the `<= committed-old(W=1)` half, and G1, constrain it. Both halves kept; the second is the one that counts.
- **GA15 "Assertions unchanged" cannot be checked.** Story 0.0.3 migrates ~15 test files from native git to `testrepo`; only selected tests get a fixture-equivalence check. Nothing detects an assertion weakened in passing. Proposed AST-hash golden for assertion calls per test function, captured before the migration.
- **GA16 Scalability sizes.** Plan tests top out at W=8 (benchmark), 18 worktrees (G9), and R = 50,000 only at the bitset/arena unit level; "tens of worktrees" and a real 50k-commit filesystem repo are not exercised. Added a 24-worktree refcount test and a 52k loose-object integration (built directly through the go-git storer to keep fixture cost down).
- **GA17 S1 control is not S1 (A/B bias, material).** Task 0.1.1b profiles an S1 binary built from Wave-0 HEAD before any flag exists. Story 5.2.1's "control (state S1)" is the *new* binary with flags off. That binary still carries always-on changes (Story 1.1.4 struct keys and reach singleflight, Story 4.4.1 "default-on, no new flag" allocation cuts, the `statFn` seam), so S2/S1 understates the gain, and the S1 profile in `baseline.md` is not the control the ratio uses. Recommendation: also keep the Wave-0 binary (`manual-builds/manual-0`) and run it as a third instance on spare ports 62875/62876 (the plan's spare block), reporting both ratios.
- **GA18 Workload realism circularity.** Story 0.1.5 tunes the driver so non-commit-walk buckets sit within 0.5x to 2x of S0. A driver that is within band but lighter than real usage lowers `rho` pressure; no gate checks absolute event rate against the live process. Partly mitigated by taking commit/fetch cadence from a read-only go-git log of corpus repos; the in-band check itself is a plan procedure, now covered only by the `TW/baselinecheck_test.go` table lint, not a numeric test.
- **N3 (security/residency)** has no requirement-grade behaviour to test; rows only prove the corpus builder never mutates owner repos and leaks no source paths.

## Fixture and Determinism Rules Every Test Above Inherits
1. go-git only; backdate timestamps with `testrepo.Age` (GA2); no `time.Sleep`; use latches/channels and the injected clock.
2. Counting storer reports `commit`, `tree`, `blob` reads separately (GA8); `statFn` reports lstat calls.
3. Gate tests measure in-process with fixed `N`, single goroutine, no background goroutines; wall-clock is only ever *recorded*.
4. Every alloc gate file is `!race`, never `-short`, and fails CI if skipped.
5. Every shared-path test asserts engagement (GA3).
6. Every gate has a `(neg)` twin or a row in `TestRegressionGates_ShouldAllFail_...` (GA4).

## Migration
No schema/data migration (no database change). Two reversibility rehearsals are designed instead and typed `Migration`: `TestFlagDefaultsFlip_Migration_ShouldBeReversible_...` and `TestForkReplace_Migration_ShouldBeReversible_...`, plus the one-tag-back and drop-replace rollback rows (type Integration).

## Counts
Recomputed with `grep -cE` over the mapping table: 218 Unit rows (5 are RH cross-reference pointers to rows counted elsewhere, so 213 distinct), 45 Integration, 2 Migration, 0 UX. 28 of 28 requirement IDs have at least one row. 28 rows are conditional on a story gate, 10 are negative controls, 4 are additions beyond the plan (marked "proposed").
