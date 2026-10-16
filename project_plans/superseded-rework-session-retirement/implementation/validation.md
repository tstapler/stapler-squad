# Validation: superseded-rework-session-retirement

**Date**: 2026-09-14
**Plan**: `implementation/plan.md` (Revision 3) + round-3 `implementation/adversarial-review.md`
(C13 seventh revival path, C14 comment, the 1.1.7a coupling correction).
**Purpose**: a requirement → test mapping written **before** the code, so every guard site has a
named test that is **proved red against unguarded code** before the guard is added.

---

## The red-before-green requirement

Each guard below has a **Red proof** column naming the exact edit that must make its test fail.
The procedure, per guard, is:

1. Write the test against the **unguarded** tree; run it; capture the failure output.
2. Add the guard; re-run the same command; capture the pass.
3. Paste both into the implementation report / PR body.

A guard whose test passes *before* the guard is added is not a regression test — it is a
tautology, and must be rewritten until it fails.

Guard 2 (Step 6b, `server/dependencies.go`) is the **one named exception**: `BuildRuntimeDeps`
makes real network calls (`server.BuildDependencies()`), so an end-to-end Step 6b test is neither
cheap nor hermetic. Its coverage is the `IsArchived()` unit test plus source inspection. This gap
is explicit, per plan Task 1.1.3a, and must be stated in the PR body rather than implied away.

---

## Requirement → test mapping

| # | Requirement (guard site) | Test file | Test name | Type | Scenario | Red proof |
|---|---|---|---|---|---|---|
| **0** | `(*Instance).IsArchived()` reads the **published snapshot**, not the raw field (`.claude/rules/instance-lock-free-reads.md`) | `session/instance_state_test.go` | `TestInstance_IsArchived_should_ReadPublishedSnapshot_When_ArchivedAtSet` | unit | Table: `{Stopped}` → false; `{Stopped, ArchivedAt}` → true; `{PermanentlyFailed, ArchivedAt}` → true; `{Active, ArchivedAt}` → true. `ArchivedAt` set **before** any `Snapshot()` call (it caches). | Method absent ⇒ compile failure. |
| **1** | Cold restore (`fromInstanceData` final `else`, `session/instance_serialization.go`): archived ⇒ `Started()==true`; `Active`/`Creating` ⇒ self-healed to `Stopped`; `Restoring`/`PermanentlyFailed`/`Failed` ⇒ **status preserved** | `session/instance_serialization_test.go` | `TestFromInstanceData_should_NotAutoRestoreAndNormalizeOnlyActiveCreating_When_Archived` | unit | Table, all rows `ArchivedAt` set, all asserting `Started()==true`: `Active`→`Stopped`, `Creating`→`Stopped`, `Restoring`→`Restoring`, `PermanentlyFailed`→`PermanentlyFailed`, `Failed`→`Failed`. Controls: `Active` + `ArchivedAt==nil` ⇒ `Started()==false`, status unchanged; `Paused` + archived ⇒ stays `Paused`. | Comment out the whole `if instance.ArchivedAt != nil {…}` block ⇒ the five archived rows report `Started()==false` and `Active`/`Creating` keep their status. **B1 regression proof**: widen the heal to every status ⇒ the three "preserved" rows go red. |
| **1b** | The self-heal **persists** through a storage round trip (this is what `saveInstancesToRepo`'s `!inst.Started()` skip makes possible) | `session/storage_test.go` | `TestSaveInstances_should_PersistSelfHealedStoppedStatus_When_ArchivedActiveRoundTrips` | integration (in-memory SQLite via `createTestStorage(t)`) | `AddInstance` an archived `Active` row → `LoadInstances()` → `SaveInstances(loaded)` → `FindInstanceDataByID` ⇒ `Status==Stopped` **and** `ArchivedAt != nil`. | Same edit as #1 ⇒ the reloaded row stays `Active` **and** is skipped by `saveInstancesToRepo` (`!Started()`), so the persisted row never changes. |
| **2** | Boot hot restore (Step 6b, `server/dependencies.go`): archived instances are skipped **before** `TmuxSessionExists()` | — | **no direct test (explicit gap)** | — | Covered by #0 (the predicate) + source inspection. `BuildRuntimeDeps` is not hermetic. | n/a — stated in the PR body. |
| **3** | `reconcileSessions` never revives an archived instance from `Stopped`/`Hibernated`/`Crashed` to `Active` | `session/review_queue_poller_test.go` | `TestReviewQueuePoller_ReconcileSessions_ArchivedStoppedWithLivePane_StaysStopped` | unit (fake tmux socket querier + `mockTmuxManager`) | Archived `Stopped` instance, live pane, wrapped program alive ⇒ status stays `Stopped`. | Remove the `if archived { … continue }` block from the `Stopped` arm ⇒ the instance is revived to `Active`. |
| **3b** | The guard did **not** over-apply: the `Active` → `Stopped` convergence arm is still unguarded | `session/review_queue_poller_test.go` | `TestReviewQueuePoller_ReconcileSessions_ArchivedActiveWithNoPane_StillTransitionsToStopped` | unit | Archived `Active` instance, **no** live session name ⇒ still reaches `Stopped`. | Guarding the `Active` case (the over-application mistake) ⇒ red. |
| **3c** | Non-archived revival is unchanged | `session/review_queue_poller_test.go` (**existing, unmodified**) | `TestReviewQueuePoller_ReconcileSessions_StoppedWithLivePane_RevivesToActive`, `..._CrashedButTmuxAlive_RevivesToActive` | unit | Must keep passing byte-for-byte. | n/a (regression watch). |
| **4** | **PRIMARY.** `healthCheckSkipReason` skips archived instances in **any** status — including the four `IsSuspended()` omits (`Active`, `Creating`, `Restoring`, `Failed`) — so `recoverMissingSession`'s `Start(false)` is never reached | `session/health_test.go` | `TestHealthCheckerRecovery_ArchivedInstance_SkippedNotAutoRestarted` | unit (`mockTmuxManager{hasSessionReturn:false}`) | Table over `Active`/`Creating`/`Restoring`/`Failed`, each `ArchivedAt` set, `started=true`; `checkSingleSession` called **twice** (past the 2-tick debounce) ⇒ `RecoveryAttempted==false`, an `Actions` entry containing `"archived"`, `mock.startCalls==0`. | Comment out the `if instance.IsArchived()` block ⇒ every row reaches `Start(false)` on the second call (`startCalls > 0`). **B2 regression proof**: revert guard 4 while **keeping** guard 1 ⇒ this test is red for `Restoring`/`Failed` too, i.e. exactly the respawn loop that would otherwise have shipped. |
| **4b** | The guard did **not** over-apply: a live (non-archived) `Active` session with a missing pane is still recovered | `session/health_test.go` (same test, control row) | `…_When_NotArchived` sub-case | unit | Same instance with `ArchivedAt==nil` ⇒ `RecoveryAttempted==true` on the second call. Proves `health.go:275-277`'s force-start still works. | Making the guard unconditional ⇒ red. |
| **4c** | Existing health-checker behaviour unchanged | `session/health_test.go` (**existing, unmodified**) | `TestHealthCheckerRecovery_PermanentlyFailedInstance_SkippedNotAutoRestarted` and the other `TestHealthCheckerRecovery_*` | unit | Must keep passing. | n/a (regression watch). |
| **5** | `handleDriverFailure` never restarts **and never marks `PermanentlyFailed`** an archived session | `session/session_driver_test.go` | `TestHandleDriverFailure_should_NotRestartOrMarkFailed_When_InstanceArchived` | unit | Archived `Stopped` instance, `reason="tmux_exited"` inside `restartGraceWindow` (the arm that calls `restartForRetry` **directly**) ⇒ returns `(false, true)`, `mock.startCalls==0`, `Snapshot().Status==Stopped` (not `PermanentlyFailed`), no `NextRetryAt` armed. | Comment out the guard ⇒ `retryDecisionRestartGrace` reaches `restartForRetry` → `Start(false)` (`startCalls > 0`). |
| **5b** | The guard did **not** over-apply: a non-archived failure still schedules / still marks permanently failed | `session/session_driver_test.go` (same test, control rows) | `…_When_NotArchived_Scheduled` / `…_When_NotArchived_Exhausted` | unit | Non-archived, budget available ⇒ `(true,false)` + `NextRetryAt` armed. Non-archived, budget exhausted ⇒ `Status==PermanentlyFailed`. | Making the guard unconditional ⇒ red. |
| **5c** | `handleRetryPendingTick` drops a retry scheduled **before** the archive landed | `session/session_driver_test.go` | `TestHandleRetryPendingTick_should_DropScheduledRetry_When_InstanceArchived` | unit | Archived instance with `NextRetryAt` in the past ⇒ `(false,true)`, `mock.startCalls==0`, `!IsRetryPending()`. Control: non-archived with `NextRetryAt` in the **future** ⇒ `(true,false)`, still pending, no restart. | Comment out the guard ⇒ `restartForRetry` is called (`startCalls > 0`). |
| **5d** | `restartForRetry` itself is **unchanged**, so manual `RetryNow` still works on archived rows | `session/retry_state.go` (**no edit**) + `session/session_driver_test.go` | reviewed by diff inspection; existing `TestEvaluateSessionRetry_*` / `TestStopSessionDriver_WaitsForHandleDriverFailureRetryGoroutine_NoGoroutineLeak` unmodified | unit | n/a | A guard inside `restartForRetry` would be visible in the diff and is forbidden by ADR-001. |
| **6** | **(round-3 C13)** `recoverFromStaleResume` never restarts an archived session — the seventh revival path, which bypasses all five planned guards and calls `Start(false)` directly | `session/instance_claude_test.go` | `TestRecoverFromStaleResume_should_NotRestart_When_InstanceArchived` | unit | Archived `Stopped` instance with a `mockTmuxManager` backend ⇒ `mock.startCalls==0` and the conversation state is left alone. Control: same instance with `ArchivedAt==nil` ⇒ `mock.startCalls > 0`. | Comment out the `if i.IsArchived()` early return ⇒ the archived row calls `RecoverFromStopped()` + `Start(false)` (`startCalls > 0`). |
| **7** | **(B4)** `ArchiveWorkflowSessions` publishes the snapshot, so `IsArchived()` — the single predicate guards 3/4/5/6 rest on — is true for sessions it archived | `server/services/workflow_service_test.go` | `TestArchiveWorkflowSessions_should_PublishSnapshotSoIsArchivedIsTrue_When_ArchivingInMemoryInstance` | integration (in-memory SQLite + real poller) | A `Stopped` instance with the target `WorkflowID` in the poller ⇒ **`inst.IsArchived() == true`** (asserted on `IsArchived()`, **not** on the raw `inst.ArchivedAt`, which is true even in the broken version). Control: an `Active` instance with the same `WorkflowID` stays unarchived. | Revert to `inst.ArchivedAt = &now` ⇒ `IsArchived()` stays **false** while a raw-field assertion would stay green. That asymmetry is the whole point of the test. |
| **7b** | The raw write cannot come back | `Makefile` (`actor-field-guard`) | `make actor-field-guard` | lint ratchet | `server/services/workflow_service.go` is added to the scanned file list. VERIFIED zero collateral: that file contains exactly one matching line today, the one being fixed. | Reverting 1.1.7a without 1.1.7c ⇒ `make actor-field-guard` fails (intended). |
| **8** | **(C8)** `retention.go` Phase 2's *update* carries the same status/archived predicate as its *ID query*, so a session revived between the two adjacent ent calls is not archived while `Active` | `server/workflows/retention_test.go` | `TestArchiveExcessSessions_should_NotArchiveActiveOrArchivedSession_When_RevivedBetweenQueryAndUpdate` | integration (in-memory SQLite) | Pass an ID set containing one `Stopped` and one `Active` session (the shape a mid-sweep revival produces) ⇒ only the `Stopped` one is archived. | Drop the added `ArchivedAtIsNil()` / `StatusNotIn(...)` predicates ⇒ the `Active` session is archived too. |
| **8b** | Existing retention behaviour unchanged | `server/workflows/retention_test.go` (**existing, unmodified**) | `TestRunRetentionSweep_*` | integration | Must keep passing. | n/a (regression watch). |

---

## Coupling constraints the tests must not paper over

Recorded here because a future splitter reading only the task list would get them wrong:

- **Guard 1 + guard 4 ship together.** Guard 1's `started.Store(true)` is exactly the
  `health.go:284` precondition guard 4 closes. Guard 1 alone is a **net regression** for archived
  `Failed`/`Restoring` rows. Test #4's `Restoring`/`Failed` rows are the proof.
- **The snapshot fix (#7) ships with guards 3, 4, 5 *and 6*.** Plan.md says "1.1.7a couples with
  1.1.4a/1.1.5a"; round-3 review correctly adds **1.1.6a**, and C13's guard 6 is in the same boat —
  all four read the **live in-memory** instance, so without the snapshot publication every one of
  them is silently inert for workflow-archived sessions. The corrected constraint is
  "**1.1.7a must ship with 1.1.4a, 1.1.5a, 1.1.6a and 1.1.8a**".
- **1.1.4c is a compile prerequisite of 1.1.4a**, not merely a logical one: 1.1.4a's inserted block
  calls `warnArchivedLivePaneOnce`, which 1.1.4c defines.

## Test-convention constraints (from `research/SYNTHESIS.md` and the repo's skills)

- In-memory SQLite via `session.NewTestEntRepository(t)` / `createTestStorage(t)`; never a real
  state dir, never `~/.stapler-squad/`.
- `envtest.NewIsolatedStateDir(t)` **before** `t.Parallel()` where a test needs a state dir
  (none of the tests above does — they are all struct-literal or in-memory-DB based).
- No real sleeps, no `waitForTimeout`-shaped waiting, no real tmux: `mockTmuxManager` /
  `newFakeTmuxSocketQuerier` doubles only.
- Table-driven where the plan says table-driven; every decision asserted as a pure function of its
  inputs.
- `ArchivedAt` must be set on a struct literal **before** the first `Snapshot()` call, because
  `Snapshot()` caches its first lazily-built value (`session/instance.go:1082-1093`).
- Naming: `Func_should_Behavior_When_Condition`, matching the repo's newer convention
  (`TestStorage_ArchiveInstanceDataByID_should_setArchivedAt_When_…`). The three tests that copy an
  existing neighbouring test keep that neighbour's older `Test<Subject>_<Case>_<Expected>` shape so
  the pair reads as a matched set.

## Gates that must be green before the commit

| Gate | Command |
|---|---|
| Build (regenerates ent/proto) | `make build` |
| Targeted + full package tests | `go test ./session/... ./server/services/... ./server/workflows/... -timeout=20m` |
| Race detector | `go test -race ./session ./server/workflows -count=1` (+ `./server/services`) |
| Lint | `make lint` |
| Custom analyzers | `make lint-custom` (`noliveinstanceraw`, `silenttransition`, `entfullscan`, `tmuxsocketscope`) |
| Actor field guard (its file list is edited by this change) | `make actor-field-guard` |
| Duplication + complexity, new-code-only | `make ready-complexity-gate` |
| Feature registry | `make registry-diff` — must be a no-op for this diff (no RPC added) |
