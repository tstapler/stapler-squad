# Validation Plan: diff-stats-file-watch-cache

**Date**: 2026-09-10

## Happy Path Scenario

Given a live `Instance` whose worktree is clean at HEAD `abc123` with the
`vcs:worktree-change-detection` feature flag on (Baseline: today's 15s pure-TTL
`GetSessionDiff`/`GetVCSStatus` caches, per requirements.md), when a background
process edits a tracked file in that worktree without touching `.git`, then
`GitWorktreeManager`'s `WorktreeChangeDetector` (periodic stat-walk, ≤15s tick)
detects the dirty-flag flip, invalidates both `diffStatsAt` and the matching
`vcsStatusCache` entry, and the next `GetSessionDiff`/`GetVCSStatus` call
recomputes fresh data instead of serving a now-wrong 5-minute-old cached
value.

## Requirement → Test Mapping

| Requirement | Test File | Test Name | Type | Scenario |
|-------------|-----------|-----------|------|----------|
| Story 1.1.1: register `vcs:worktree-change-detection` flag, default off | `server/services/feature_flag_service_test.go` | `TestGetFeatureFlagWithDefault_should_ReturnFalse_When_WorktreeChangeDetectionFlagUnset` | Unit (happy) | Fresh `config.json` with no `feature_flags` key → `GetFeatureFlagWithDefault("vcs:worktree-change-detection", false)` returns `false`. |
| Story 1.1.1: flag non-retroactivity for already-open sessions | `session/git_worktree_manager_test.go` | `TestGitWorktreeManager_Setup_should_KeepDetectorNil_When_FlagFlippedOnAfterSetupAlreadyRan` | Integration (error/edge path) | Flag flipped `false`→`true` after `Setup()` already ran with it off; session keeps 15s TTL until torn down and recreated. |
| Story 1.1.2: `GitWorktree.IsDirtyUncached()` bypasses `IsDirtyWithHint`'s TTL cache | `session/git/worktree_git_test.go` | `TestIsDirtyUncached_should_ReturnTrue_When_IsDirtyWithHintCacheIsStillClean` | Unit (happy) | Real temp git repo; `IsDirtyWithHint(false)` populates 5-min clean cache, then an untracked file is created; `IsDirtyUncached()` returns `true` while a same-instant `IsDirtyWithHint(false)` still returns stale `false`. |
| Story 1.1.2: `IsDirtyUncached()` reuses inner allocation-avoidance caches, not `nil,nil` | `session/git/worktree_git_test.go` | `TestIsDirtyUncached_should_ReuseHeadTreeCache_When_HeadUnchangedAcrossCalls` | Unit (error/regression path) | Guards against `worktreeIsDirtyFast`'s params being passed as `nil` — asserts `g.headTreeCache` is populated/reused across two calls with an unchanged HEAD. |
| Story 2.1.1: `Start()` takes baseline fingerprint synchronously, no spurious fire | `session/git_worktree_watcher_test.go` | `TestWorktreeChangeDetector_should_NotFireOnChange_When_FingerprintUnchangedAcrossTicks` | Unit (happy) | Fake `fingerprintFunc` returns the same `(dirty, headSHA)` tuple every tick (short injected `statWalkInterval`, Task 2.1.1g); assert zero fires across several ticks. |
| Story 2.1.1: `.git` watch failure degrades to periodic-only, logged once | `session/git_worktree_watcher_test.go` | `TestWorktreeChangeDetector_should_FallBackToPeriodicOnly_When_GitFsnotifyAddFails` | Unit (error path) | `worktreePath` points at a path with no `.git` subdirectory (`Add()` fails ENOENT); assert `GitWatchActive() == false`, periodic loop still fires on a fingerprint change, exactly one `Warn` log line. |
| Story 2.1.1: panicking `OnChange` callback doesn't kill the detector or other callbacks | `session/git_worktree_watcher_test.go` | `TestWorktreeChangeDetector_should_KeepOtherCallbacksRunning_When_OneCallbackPanics` | Unit (error path) | Two registered callbacks, first panics unconditionally, second increments a counter; two successive fingerprint flips; counter increments both times, detector goroutines stay alive. |
| Story 2.1.1: detector fires on real dirty-flag flip (fake fingerprint) | `session/git_worktree_watcher_test.go` | `TestWorktreeChangeDetector_should_FireOnChangeExactlyOnce_When_DirtyFlagFlips` | Unit (happy) | Fake `fingerprintFunc` returns `(false,"sha1",nil)` then `(true,"sha1",nil)` after a manual trigger; assert `OnChange` fires exactly once, and zero times before the flip. |
| Story 2.1.1: `Stop()` releases both goroutines, no leak | `session/git_worktree_watcher_test.go` | `TestWorktreeChangeDetector_should_ReleaseAllGoroutines_When_StopIsCalled` | Unit (happy, via `goleak`) | `goleak.VerifyNone(t)` wrapped around `Start()`/`Stop()`. |
| Story 2.1.1 / 3.3.2: real file edit invalidates within the periodic interval | `session/git_worktree_watcher_test.go` | `TestWorktreeChangeDetector_should_FireOnChangeWithinInterval_When_RealUntrackedFileIsWritten` | Integration (real temp git worktree, real fingerprintFunc) | `t.TempDir()` + `git init` + one commit; short injected `statWalkInterval` (e.g. 50ms); write `bar.txt`; assert callback fires within 200ms via buffered channel + bounded `select` (no `time.Sleep`). Covers requirements.md Success Metric (b)'s plain-edit, interval-bound path. |
| Story 2.1.1 / 3.3.2: real git commit invalidates via the `.git` fsnotify watch specifically | `session/git_worktree_watcher_test.go` | `TestWorktreeChangeDetector_should_FireOnChangeViaGitWatch_When_RealCommitIsMade` | Integration (real temp git worktree, real unfaked `.git` fsnotify watch) | Same setup as above but `statWalkInterval` set long (30s) so only the `.git` watch can plausibly fire; assert `GitWatchActive() == true`, real `git add`+`commit`, callback fires within 1-2s via buffered channel + bounded `select`. Covers requirements.md Success Metric (b)'s sub-second `.git`-triggered path. |
| Story 2.2.1: `Setup()` never constructs a detector when flag is off | `session/git_worktree_manager_test.go` | `TestGitWorktreeManager_Setup_should_LeaveChangeDetectorNil_When_FlagIsOff` | Unit (happy) | Flag off; assert `gm.changeDetector == nil`, `gm.changeDetectionActive == false`. |
| Story 2.2.1: `Cleanup()` stops the detector before delegating to the underlying worktree | `session/git_worktree_manager_test.go` | `TestGitWorktreeManager_Cleanup_should_StopDetectorBeforeDelegatingToWorktree_When_FlagIsOn` | Integration (real fsnotify watcher on a real temp worktree) | Flag on, `Setup()` starts a detector for a real temp worktree; `Cleanup()` blocks until `Stop()` returns (goroutines exited) before `wt.Cleanup()` runs; `gm.changeDetector` reset to `nil`. |
| Story 2.2.1: `Cleanup()`/`Remove()` on a detector that never started doesn't panic | `session/git_worktree_manager_test.go` | `TestGitWorktreeManager_Cleanup_should_NoOp_When_NoDetectorWasEverStarted` | Unit (error/edge path) | Flag off (no detector constructed); `Cleanup()` and `Remove()` both return cleanly with no nil-pointer panic. |
| Story 2.2.1: `DiffStatsFresh()` widens TTL only when change detection is active | `session/git_worktree_manager_test.go` | `TestDiffStatsFresh_should_Use5MinuteTTL_When_ChangeDetectionActive` | Unit (happy) | `changeDetectionActive == true`, `diffStatsAt` set to `-4m` → fresh; `-6m` → stale. |
| Story 2.2.1 / 3.3.1: `DiffStatsFresh()` TTL boundary is provably unchanged at 15s when flag is off | `session/git_worktree_manager_test.go` | `TestDiffStatsFresh_should_Use15sTTLBoundaryUnchanged_When_FlagIsOff` | Unit (error/regression path — proves no accidental widening) | Table test: `diffStatsAt` at `-14s` (flag off) → `true`; at `-16s` → `false`; no detector constructed. |
| Story 2.2.1 / Success Metric: no fd/goroutine leak across many Setup/Cleanup cycles | `session/git_worktree_manager_test.go` | `TestGitWorktreeManager_SetupCleanupCycle_should_LeakNoGoroutines_When_Run20TimesWithFlagOn` | Integration (real temp worktrees, `goleak`) | 20 Setup/Cleanup cycles, flag on, wrapped in `goleak.VerifyNone(t)`. |
| Success Metric (fd/goroutine-leak-freedom at fleet scale) | `session/git_worktree_manager_test.go` | `TestGitWorktreeManager_150SetupCleanupCycles_should_LeakNoGoroutinesOrFds_When_FlagIsOn` | Integration (real temp worktrees, `goleak` + `/proc/self/fd` count on Linux) | 150 sequential Setup/Cleanup cycles (fleet-scale proxy for 134 concurrent worktrees) leave zero leaked goroutines and fd count within a small constant of the pre-loop count (Linux-only assertion, `runtime.GOOS` guard matching existing platform-conditional test idiom). |
| Story 2.3.1: `Instance.OnWorktreeChange` no-ops safely with no worktree | `session/instance_worktree_test.go` | `TestInstance_OnWorktreeChange_should_NotPanic_When_InstanceHasNoGitWorktree` | Unit (error/edge path) | Directory-mode `Instance` (`HasGitWorktree() == false`); `OnWorktreeChange(fn)` returns without calling `fn` and without panicking. |
| Story 2.3.1: `Instance.WorktreeChangeDetectionActive`/`WorktreeGitWatchActive` passthroughs | `session/instance_worktree_test.go` | `TestInstance_WorktreeChangeDetectionActive_should_ReflectGitWorktreeManagerState_When_DetectorRunning` | Unit (happy) | Flag on, worktree-mode `Instance`; `WorktreeChangeDetectionActive()`/`WorktreeGitWatchActive()` mirror the underlying `GitWorktreeManager`'s state. |
| Story 2.3.2: `GetVCSStatus` cache-miss path subscribes lazily, once per workdir | `server/services/workspace_service_test.go` | `TestGetVCSStatus_should_SubscribeOnceViaOnWorktreeChange_When_CalledTwiceForSameWorkdir` | Unit (happy, fake `LiveInstanceFinder`) | Flag on; `GetVCSStatus("/tmp/wt-1")` called twice; assert `watchedWorkdirs.Load` exists after the first call and `instance.OnWorktreeChange` was invoked exactly once. |
| Story 2.3.2: widened TTL only applies when the entry's own detector was active at store time | `server/services/workspace_service_test.go` | `TestGetVCSStatus_should_UseUnwidenedTTL_When_EntryCachedWithChangeDetectionInactive` | Unit (error/edge path) | `changeDetectionActive == false` at cache-store time (flag off, or jj-managed worktree); a call 20s later recomputes rather than serving the stale entry — proves the widened TTL never silently applies without a backing detector. |
| Story 2.3.2: a fired change callback actually invalidates the `vcsStatusCache` entry | `server/services/workspace_service_test.go` | `TestGetVCSStatus_should_RecomputeOnNextCall_When_RegisteredChangeCallbackFires` | Unit (happy) | Flag on, cache populated, registered callback invoked directly (simulating the detector firing); next `GetVCSStatus` call recomputes instead of serving the deleted entry. |
| Story 2.3.2 (VCS-scope Strategy rejection / jj worktrees keep 15s unconditionally) | `server/services/workspace_service_test.go` | `TestGetVCSStatus_should_KeepUnwidened15sTTL_When_WorktreeIsJujutsuManaged` | Unit (error/edge path) | A jj-managed workdir (falls back to `vc.NewJujutsuProvider`) never gets a `WorktreeChangeDetector`; `instance.WorktreeChangeDetectionActive()` returns `false` unconditionally regardless of flag state; cache entry TTL stays 15s. |
| Epic 3.1: fsnotify bump to v1.10.1 doesn't break existing `.git`-only watcher usage | `session/unfinished/scanner_test.go` (existing suite, run unmodified) | *(no new test — regression gate)* `go test ./session/unfinished/... ./session/... -run Watcher` | Integration (existing suite as regression gate) | Confirms `go.mod`/`go.sum` bump introduces no behavior change to `session/unfinished/watcher.go`'s pre-existing `.git` fsnotify usage. |
| Story 3.2.1: `CreateDebugSnapshot` surfaces detector status per session (active case) | `server/services/window_debug_test.go` | `TestCreateDebugSnapshot_should_ReportDetectionActiveTrue_When_GitWatchIsRunning` | Unit (happy) | Flag-on session, `.git` watch successfully started; snapshot JSON's `worktree_change_detection_active` and `worktree_git_watch_active` are both `true`. |
| Story 3.2.1: `CreateDebugSnapshot` surfaces detector status per session (inactive case) | `server/services/window_debug_test.go` | `TestCreateDebugSnapshot_should_OmitGitWatchActive_When_FlagIsOff` | Unit (error/edge path) | Flag-off session; `worktree_change_detection_active` is `false`, `worktree_git_watch_active` is omitted (`omitempty` zero value). |
| Success Metric: CPU-share reduction for a fleet of idle/background worktrees | — | — | **Deferred, not tested in this validation plan** | See "Deferred Success Metric" note below — plan.md's Unresolved Questions section explicitly defers this to a post-merge, staged-rollout measurement; no synthetic test can reproduce the fleet-scale idle-worktree access pattern the ~13%+7.5% Pyroscope baseline was measured against. |

## Deferred Success Metric: CPU-share reduction

requirements.md's Success Metrics require `GetSessionDiff`/`GetVCSStatus`'s
combined CPU share to drop "measurably" from the ~13%+7.5% Pyroscope baseline.
Per plan.md's **Unresolved Questions** section, this is explicitly *not* a
task in this plan and is **not** covered by a test here: it's only observable
under sustained, realistic idle/background-worktree load at fleet scale (134
worktrees), which none of Epic 3.3's tests attempt to simulate (a synthetic
burst in a test run would not reproduce the access pattern the baseline was
measured against). Per plan.md's Risk Control §"Staged rollout," this is a
tracked post-merge follow-up:

- **Owner**: whoever flips `vcs:worktree-change-detection` to `true` on the
  134-worktree dev box.
- **Trigger**: 48 hours after the flag is enabled on that dev box.
- **Method**: re-run the same Pyroscope `SelectSeries`/`SelectMergeProfile` +
  `go tool pprof -top -cum` comparison used to establish the ~13%+7.5%
  baseline, before vs. after enabling the flag; record the delta in a PR
  comment or follow-up issue referencing plan.md.
- **Escalation if the delta doesn't materialize**: re-open plan.md's Design
  Pivot assumptions (periodic-stat-walk cost, 15s interval, 5-minute TTL)
  rather than assuming the measurement methodology was wrong.

This row is listed explicitly in the mapping table above (marked "Deferred")
rather than silently omitted, per this project's validation instructions.

## Test Stack

- **Unit**: Go's standard `testing` package + table-driven tests, matching
  this repo's existing convention (`session/instance_test.go`,
  `session/unfinished/scanner_test.go`). Fake `fingerprintFunc` closures and
  fake `LiveInstanceFinder`/`Instance`-shaped test doubles isolate
  `WorktreeChangeDetector` and `WorkspaceService` tests from real git/fsnotify
  where the scenario doesn't require them. `go.uber.org/goleak` for
  goroutine-leak assertions (already a repo dependency per the
  `golang-testing` skill's conventions).
- **Integration**: Real `t.TempDir()` git repos/worktrees (`git init` + one
  commit) with a real, unfaked `fsnotify.Watcher` for the `.git`-watch tests
  (Story 2.1.1/3.3.2) and real `GitWorktreeManager.Setup()`/`Cleanup()` cycles
  for the lifecycle and leak-proxy tests (Story 2.2.1/3.3.3) — these are the
  "at least one real integration test per fsnotify/worktree-involving story"
  tests called for by the project's test-design instructions. All use
  bounded `select`/channel synchronization, never `time.Sleep`, per the
  `deterministic-fast-tests` skill.
- **E2E / UX**: N/A — no user-facing surface (pure backend caching/
  infrastructure change; `design/ux.md` does not exist for this project).

## Coverage Targets and How to Measure

| Stack | Coverage command | Target |
|---|---|---|
| Go | `go test ./session/... ./server/services/... -coverprofile=coverage.out && go tool cover -func=coverage.out` | ≥80% line, for the touched packages/files: `session/git_worktree_watcher.go`, `session/git_worktree_manager.go`, `session/git/worktree_dirty_fast.go`, `session/instance_worktree.go`, `server/services/workspace_service.go`, `server/services/debug_snapshot.go` |

- All public methods added by this feature (`NewWorktreeChangeDetector`,
  `Start`, `Stop`, `OnChange`, `GitWatchActive`, `IsDirtyUncached`,
  `WorktreeChangeDetectionActive`, `Instance.OnWorktreeChange`, etc.): happy
  path + error paths covered per the mapping table above.
- All external integrations (fsnotify `.git` watch, real git worktrees via
  `t.TempDir()`+`git init`): unit-tested with an injectable/fake
  `fingerprintFunc` **and** at least one real, unfaked integration test each
  (Stories 2.1.1/2.2.1/3.3.2/3.3.3 above) — satisfying this project's
  "1 unit + 1 integration per fsnotify/worktree-involving story" test-design
  requirement.
- `make ready`'s duplication gates (`dupl` for new Go code, new-code-only via
  `--new-from-rev=origin/main`) apply to this feature's test files same as
  production code — the parallel-structured `RealFileEdit`/`RealGitCommit`
  integration tests (3.3.2a/3.3.2b) share setup boilerplate deliberately (per
  plan.md's own task descriptions describing 3.3.2b as "sibling to Task
  3.3.2a"); if `dupl` flags that shared `t.TempDir()`/`git init`/commit setup,
  extract a shared test helper rather than suppressing the finding.
