# Validation Plan: session-worktree-reconciliation

**Date**: 2026-09-23

## Happy Path Scenario
Given a worktree-backed session (`SessionTypeNewWorktree`, branch set) whose `Worktree` ent row is missing but a matching `LiveWorktreeEntry` exists on disk, when the periodic `sweep(ctx)` tick runs with `FeatureFlagWorktreeConsistencySweep` enabled, then `resolveFinding` auto-repairs the row via the unique `matchLiveWorktree` result and posts a visible "auto-repaired" notification — reproducing and closing PR #625's silent failure end-to-end.

## Requirement → Test Mapping

| Requirement | Test File | Test Name | Type | Scenario |
|-------------|-----------|-----------|------|----------|
| REQ-1 (AC1: periodic pass detects consistency for every live session) / Story 1.1.2 | `session/worktree_consistency_sweep_test.go` | `TestListConsistencyCandidates_should_IncludeSession_When_WorktreeRowMissingAndExpectsWorktreeTrue` | Unit | Happy path (Task 1.1.2c) |
| REQ-1 / Story 1.1.2 | `session/worktree_consistency_sweep_test.go` | `TestListConsistencyCandidates_should_ExcludeSession_When_StatusCreatingWithinGracePeriod` | Unit | Error/edge path — mid-creation session must not be misflagged (Task 1.1.2c) |
| REQ-1 / Story 1.3.1 | `session/worktree_consistency_sweep_test.go` | `TestSweep_should_CallStorageAndGit_When_FeatureFlagEnabled` | Unit | Happy path — flag on proceeds (Task 1.3.1c) |
| REQ-1 / Story 1.3.1 | `session/worktree_consistency_sweep_test.go` | `TestSweep_should_MakeZeroStorageOrGitCalls_When_FeatureFlagDisabled` | Unit | Error/edge path — flag off short-circuits (Task 1.3.1c) |
| REQ-1 / Story 1.3.2 | `server/dependencies_test.go` | `TestReconcileTicker_should_StartWorktreeConsistencySweeper_When_DependenciesBuilt` | Integration | Happy path — sweeper goroutine launches at startup via narrow `ServerDependencies` helper, not `BuildDependencies()` (Task 1.3.2b) |
| REQ-2 (AC2: every inconsistency repaired-with-before/after or flagged, never silent) / Story 1.2.3 | `session/worktree_consistency_sweep_test.go` | `TestResolveFinding_should_CreateWorktreeRowAndNotify_When_UniqueMatchOnNonIsolatedInstance` | Integration (real ent write) | Happy path — repair writes `Worktree` row + always notifies (Task 1.2.3d) |
| REQ-2 / Story 1.2.3 | `session/worktree_consistency_sweep_test.go` | `TestResolveFinding_should_FlagAndNotifyWarning_When_MatchCountIsAmbiguous` | Unit | Error/edge path — 0 or 2+ matches force `ResolutionFlagged`, never a guess (Task 1.2.3d) |
| REQ-2 / Story 1.2.3 | `session/worktree_consistency_sweep_test.go` | `TestResolveFinding_should_CallMarkStuck_When_LiveNonTerminalBacklogItemLinked` | Unit | Edge path — best-effort dual-write only when a live linked item exists (Task 1.2.3d) |
| REQ-2 / Story 1.2.3 | `session/worktree_consistency_sweep_test.go` | `TestResolveFinding_should_CallNotifySessionWithoutItemIdMetadata_When_NoBacklogItemLinked` | Unit | Error/edge path — Architecture-A1 regression: no session UUID ever lands in `metadata["item_id"]` (Task 1.2.3e) |
| REQ-3 (AC3: PR #625 regression test) / Story 1.4.1 | `session/worktree_consistency_sweep_test.go` | `TestSweep_should_DetectAndRepairMissingWorktreeRow_When_ReproducingPR625Scenario` | Integration | Happy path — exact PR #625 defect shape (worktree-backed session, no `worktrees` row, live git worktree present) is detected and repaired (Tasks 1.4.1a-c) |
| REQ-4 (AC4: ask #2 self-cleanup resolved) / Story 2.1.1 | `server/services/backlog_service_triage_test.go` | `TestCleanupItemWorktreesExcept_should_NotifyAndLogWarning_When_WorktreeRowMissingButExpected` | Integration (existing file's real ent test-storage convention) | Happy path — missing-but-expected row is logged + notified instead of silently `continue`d (Task 2.1.1c, test 1) |
| REQ-4 / Story 2.1.1 | `server/services/backlog_service_triage_test.go` | `TestCleanupItemWorktreesExcept_should_ContinueSilently_When_SessionDoesNotExpectWorktree` | Integration | Error/edge path — regression guard: legitimately non-worktree sessions (`SessionTypeDirectory`) keep the unchanged silent `continue` (Task 2.1.1c, test 2) |
| REQ-4 (documented, non-test half of AC4) | `project_plans/session-worktree-reconciliation/implementation/plan.md` (Epic 2.1 goal statement + Tech Debt Disposition table) | N/A — written confirmation, not a test | N/A | Confirms `pr_pending` exclusion, synchronous terminal cleanup, and the hourly retention sweep already satisfy ask #2 end-to-end, citing `architecture.md §5` / `features.md §3`; only the one gap above (Story 2.1.1) needed code |
| REQ-5 (AC5: no regression to lock-free `Instance` reads) / Story 1.1.2 | `session/worktree_consistency_sweep_test.go` | `TestSweep_NoRaceWithConcurrentActorWrites` | Integration (concurrency, `-race`) | Error/edge-proof path — `go test -race` against a goroutine mutating `i.Path` under `i.mu.Lock()` concurrently with `sweep(ctx)` reports no race (Task 1.1.2d) |
| Story 1.1.1 (git wrapper) | `session/git/native_worktree_list_test.go` | `TestListWorktrees_should_ReturnSameEntries_When_ComparedToNativeListWorktrees` | Integration (real git fixture repo) | Happy path — exported wrapper matches unexported parser exactly (Task 1.1.1b) |
| Story 1.2.1 (matchLiveWorktree) | `session/worktree_consistency_sweep_test.go` | `TestMatchLiveWorktree_should_ReturnUniqueMatch_When_OneEntryMatchesBranchRef` | Unit | Happy path — unique branch-ref match (Task 1.2.1b) |
| Story 1.2.1 | `session/worktree_consistency_sweep_test.go` | `TestMatchLiveWorktree_should_ReturnNilWithCountTwo_When_MultipleEntriesMatchSameBranch` | Unit | Error/edge path — ambiguous match returns `(nil, 2)`, never guesses (Task 1.2.1b) |
| Story 1.2.2 (classifyIssues) | `session/worktree_consistency_sweep_test.go` | `TestClassifyIssues_should_ReturnMissingWorktreeRowFinding_When_WorktreeNilAndLiveEntryMatches` | Unit | Happy path (Task 1.2.2d) |
| Story 1.2.2 | `session/worktree_consistency_sweep_test.go` | `TestClassifyIssues_should_ReturnNoFinding_When_SessionPausedAndDirectoryGone` | Unit | Edge path — named Paused-exclusion regression case (Task 1.2.2d) |
| Story 1.2.2 | `session/worktree_consistency_sweep_test.go` | `TestClassifyIssues_should_ReturnNoFindings_When_GitErrorIsTransient` | Unit | Error path — non-`os.IsNotExist`/non-`plumbing.ErrObjectNotFound` errors (permission, `context.DeadlineExceeded`) produce zero findings, not false positives (Task 1.2.2e) |
| Story 1.2.2 | `session/worktree_consistency_sweep_test.go` | `TestClassifyIssues_should_ReturnBaseCommitShaUnresolvableFinding_When_ShaGenuinelyMissingFromRealRepo` | Integration (real git repo, real `plumbing.ErrObjectNotFound`) | Happy path for the SHA-unresolvable branch — a genuine object-not-found error against a real repo, not a mocked sentinel (Task 1.2.2c/d) |
| Story 1.2.4 (backoff) | `session/worktree_consistency_sweep_test.go` | `TestSweep_should_NotifyOnce_When_FindingFirstDetected` | Unit | Happy path (Task 1.2.4a/b) |
| Story 1.2.4 | `session/worktree_consistency_sweep_test.go` | `TestSweep_should_SuppressDuplicateNotify_When_SameFindingWithinBackoffWindow` | Unit | Error/edge path — second tick within 24h window does not re-notify, but still logs (Task 1.2.4b) |
| Story 1.3.3 (isolated-instance guard) | `session/worktree_consistency_sweep_test.go` | `TestResolveFinding_should_DowngradeToFlagged_When_RunningOnIsolatedInstance` | Unit | Edge path — `config.IsIsolatedInstance()` downgrades an otherwise-repairable finding to flagged (Task 1.3.3b) |

## Coverage Gaps

1. **Story 1.1.1 — no error-path test for `git.ListWorktrees`.** Task 1.1.1b only asserts parity with `nativeListWorktrees` on a healthy fixture repo; no task exercises `git.ListWorktrees` against a non-existent/non-git path to confirm the error propagates unchanged. Low risk (it's a one-line pass-through wrapper — `func ListWorktrees(repoPath string) ([]NativeWorktreeEntry, error) { return nativeListWorktrees(repoPath) }`), but strictly a gap against this validation template's "1 unit test (error path)" requirement. Recommend adding `TestListWorktrees_should_ReturnError_When_RepoPathIsNotGitRepo` alongside Task 1.1.1b.

2. **Story 1.2.3 — `EventBusNotifier.NotifySession` itself has no assigned test task.** Task 1.2.3a-prep creates `server/services/backlog_notifier_test.go` ("new — no existing test file covers `backlog_notifier.go` today") and wires `NotifySession` into `fakeNotifier` (`session/backlog_lifecycle_test.go:2426`), but no task in Story 1.2.3's list writes a test asserting `EventBusNotifier.NotifySession` actually publishes via `events.NewNotificationEvent` with empty `metadata` (no `item_id` key). Tasks 1.2.3d/1.2.3e only exercise `resolveFinding` against the `Notifier` *interface* (via `fakeNotifier`), which proves `resolveFinding` calls the right method but not that the real `EventBusNotifier` implementation behaves correctly end-to-end. Recommend adding `TestEventBusNotifier_NotifySession_should_PublishEventWithoutItemIdMetadata_When_Called` to `server/services/backlog_notifier_test.go`.

3. **Story 1.3.2 — wiring smoke test only, no failure-path coverage.** Task 1.3.2b confirms the sweeper goroutine starts without panicking; no task covers what happens if `storage`/`notifierAdapter`/`cfgAccessor` construction fails before reaching the `go session.StartWorktreeConsistencySweeper(...)` line. Accepted as low-priority: this mirrors the sibling 60s-reconcile-ticker block's own test depth (`server/dependencies.go:1350-1358`), so it is not a new gap introduced by this feature, but is noted per this template's instruction to call out anything with no corresponding task-level test.

No requirements.md AC is left entirely unmapped: AC #3 (PR #625 regression), AC #4 (ask #2 resolution), and AC #5 (lock-free-reads compliance) each map to a concrete test/documentation row above, per the task's explicit callout.

## UX Acceptance Tests
N/A — pure infrastructure, no user-facing surface (research/ux.md's own conclusion; no `design/ux.md` exists for this project).

## Migration Plan
N/A — no schema migration. The `Worktree` ent schema (`session/ent/schema/worktree.go`) is unchanged; this feature only writes new rows through the existing create/update paths (`EntRepository`'s existing write shape, `ent_repository.go:636-666`).

## Test Stack
- **Unit**: Go stdlib `testing` + this repo's existing table-driven/fake-double conventions (e.g. `fakeNotifier` at `session/backlog_lifecycle_test.go:2426`, extended with `NotifySession` per Task 1.2.3a-prep).
- **Integration**: Go stdlib `testing` against a real (test-isolated) ent/SQLite storage instance and real on-disk git fixture repos, per this repo's `envtest`/config-dir-resolved test-isolation conventions (`docs/explanation/test-io-storage-isolation.md`). The `-race` concurrency proof (Task 1.1.2d) and the PR #625 regression fixture (Task 1.4.1a-c) both fall in this category — they exercise real `*Instance` actor-setter goroutines and real ent-persisted rows, not mocks.
- **E2E / UX**: N/A.

## Coverage Targets and How to Measure

| Stack | Coverage command | Target |
|---|---|---|
| Go | `go test ./session/... ./server/services/... -coverprofile=coverage.out && go tool cover -func=coverage.out` | ≥80% line |
| Go (race) | `go test -race -run TestSweep_NoRaceWithConcurrentActorWrites ./session/...` | Zero race reports (hard gate, not a coverage percentage — required by requirements.md AC5) |

- All public functions in `session/worktree_consistency_sweep.go` (`ExpectsWorktree`, `matchLiveWorktree`, `classifyIssues`, `resolveFinding`, `StartWorktreeConsistencySweeper`): happy path + error paths covered per the table above.
- All external integrations (git CLI via `git.ListWorktrees`/`git.CommitInfo`, ent writes via `EntRepository`, event-bus notification via `EventBusNotifier`): unit-mocked (`fakeNotifier`) coverage plus at least one integration test each — satisfied for ent writes (Task 1.2.3d) and git (Tasks 1.1.1b, 1.2.2c/d integration variant); the `EventBusNotifier.NotifySession` integration gap is called out above (Gap #2).
- UX acceptance criteria: N/A.
