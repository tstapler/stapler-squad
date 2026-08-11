# Validation Plan: backlog-session-thrashing

**Date**: 2026-07-25

This maps plan.md's already-specified tests (it names ~25+ across Phases 1-3) against
requirements.md's Success Metrics and In-Scope items, and checks the 5 risk areas called
out by the Phase 3 adversarial review (verdict: CONCERNS, 0 blockers, 3 new minor
concerns). No test design was invented from scratch here except where a gap is flagged.

## Requirement -> Test Mapping

### REQ-1 (Primary): At most one live work session per backlog item at any time

| Requirement | Test File | Test Name | Type | Scenario |
|---|---|---|---|---|
| REQ-1 | `server/services/backlog_service_test.go` | `TestSpawnSessionFromItem_ConcurrentSpawns_OnlyOneWorkSessionCreated` (existing, unmodified) | Unit | Happy path — concurrent `SpawnSessionFromItem` calls for the same item serialize via `spawnInFlight`, only one `ItemSession` created |
| REQ-1 | `server/services/backlog_service_triage_test.go` | `TestDequeueNextQueuedItems_should_ClaimOnlyOneItem_When_CalledConcurrentlyWithOneFreeSlot` (existing, unmodified) | Unit | Happy path — concurrent `DequeueNextQueuedItems` calls claim disjoint items, respecting the free-slot count |
| REQ-1 | `server/services/backlog_service_triage_test.go` | `TestSpawnSessionFromItem_RacesWithDequeue_OnlyOneWorkSessionCreated` (new, Task 1.1.2a) | Error path | A concurrent `SpawnSessionFromItem` call, raced against a deterministically-paused `DequeueNextQueuedItems` inside the exact TOCTOU window, fails fast with `connect.CodeAlreadyExists` |
| REQ-1 | `server/services/backlog_service_triage_test.go` | `TestSpawnSessionFromItem_RacesWithDequeue_OnlyOneWorkSessionCreated` (new, Task 1.1.2a) | **Integration** (real SQLite-backed `session.Storage` via `createTestStorage`/`createTestStorageWithRepo`, `-race`) | **Cross-acquisition-site mutual exclusion**: after both goroutines complete, `ListItemSessions` shows exactly 1 open (`EndedAt == nil`) work-role `ItemSession` and `len(creator.calls) == 1` — this is the dual-acquisition-site proof requested in Step 3 (`SpawnSessionFromItem` vs. `DequeueNextQueuedItems`, not just within one function) |

### REQ-2 (Primary): Genuine forward progress is not killed/restarted purely for crossing a fixed turn count

| Requirement | Test File | Test Name | Type | Scenario |
|---|---|---|---|---|
| REQ-2 | `session/autonomous_driver_test.go` | `TestAutonomousDriver_ExitKind_Done_When_DoneSignalReceived` (new, Task 2.1.1d) | Unit | Happy path — driver reaches DONE, `ExitKind == DriverExitDone` |
| REQ-2 | `session/autonomous_driver_test.go` | `TestAutonomousDriver_ExtendsTurnBudget_When_RecentProgressAtSoftCap` (new, Task 2.3.1c) | Unit | Happy path — recent output at the soft cap extends `effectiveMaxTurns` instead of stopping |
| REQ-2 | `session/autonomous_driver_test.go` | `TestAutonomousDriver_StopsAtBaseBudget_When_NoRecentProgress` (new, Task 2.3.1c) | Error/failure path | No recent output at the soft cap → stops at exactly `maxTurns`, `ExitKind == DriverExitMaxTurns` |
| REQ-2 | `session/autonomous_driver_test.go` | `TestAutonomousDriver_HardCapWinsRegardlessOfProgress` (new, Task 2.3.1c) | Error/failure path (ceiling) | Progress signal held true throughout — driver still never exceeds `hardMaxTurns` (absolute ceiling honored) |
| REQ-2 | `session/autonomous_driver_test.go` | `TestAutonomousDriver_ExitKind_MaxTurns_When_LoopExhaustsNaturally`, `..._ContextCancelled_When_StopCalled`, `..._LLMCallError_When_CallBlockingFails` (new, Task 2.1.1d) | Unit — error paths | Each non-DONE exit path sets the correct `ExitKind` and accurate `Turns` (fixes the pre-existing `Turns` hardcode bug) |
| REQ-2 | `session/autonomous_driver_test.go` | `TestAutonomousDriver_AbortsEarly_When_ThreeConsecutiveMalformedResponses`, `..._ResetsConsecutiveCounter_When_FollowedByValidReply` (new, Task 2.2.1b) | Unit (happy + error) | Malformed-response sub-cap: 3-in-a-row aborts early distinguishably; an isolated malformed response does not trip it |
| REQ-2 | `session/autonomous_driver_test.go` | `TestAutonomousDriver_MalformedStreakAtSoftCap_AbortsWithMalformedReason_NotSoftCapExtension`, `..._IsolatedMalformedResponse_DoesNotBlockSoftCapExtension` (new, Task 2.5.1b) | **Integration** (in-process, multi-feature interaction, `-race`) | Malformed-sub-cap vs. soft-cap-extension composition: a malformed streak at exactly the soft-cap boundary aborts via the malformed reason (not silently swallowed by/racing with the extension), and an isolated earlier malformed reply doesn't block a later legitimate extension |
| REQ-2 | `config/config_test.go` | `TestMaxAutonomousTurnsOrDefault` (new, Task 2.4.1c) | Unit (happy + error/edge) | nil/zero/negative → default 20; explicit positive → that value (configurable base budget, no rebuild required) |

### REQ-3 (Secondary): A stuck session reaches one well-defined, operator-visible state — not a silent retry/duplicate/vanish

| Requirement | Test File | Test Name | Type | Scenario |
|---|---|---|---|---|
| REQ-3 | `server/services/autonomous_orchestration_service_test.go` | `TestAutonomousOrchestrationService_OnAutonomousDriverComplete_NoStuckRow_When_Done` (existing) | Unit | Happy path — a DONE outcome never opens an `autonomous_stuck` row |
| REQ-3 | `server/services/autonomous_orchestration_service_test.go` | `TestAutonomousOrchestrationService_OnAutonomousDriverComplete_MarksAutonomousStuck_When_NotDone` (existing, updated to set `ExitKind: session.DriverExitMaxTurns`, Task 3.2.1b) | Unit | Error/failure path — genuine turn-cap exhaustion opens the `autonomous_stuck` row and fires the generic notification |
| REQ-3 | `server/services/autonomous_orchestration_service_test.go` | `TestAutonomousOrchestrationService_OnAutonomousDriverComplete_SkipsStuckMarkingAndNotification_When_ContextCancelled_WorkRole` (new, Task 3.1.1b) | Unit | Error/failure path — intentional cancellation (autonomous mode toggled off, hibernate, delete) does NOT open `autonomous_stuck` or fire the generic "stuck" notification |
| REQ-3 | `server/services/autonomous_orchestration_service_test.go` | `TestAutonomousOrchestrationService_OnAutonomousDriverComplete_StillEndsAbandonedReviewRow_When_ContextCancelled` (new, Task 3.1.1b) | **Integration** (cross-component: driver `ExitKind` classification x role-specific storage bookkeeping x BUG-048's `abandoned_review` detector precondition) | **BUG-048 regression test** — see dedicated section below |
| REQ-3 | `server/services/autonomous_orchestration_service_test.go` | `..._ReasonText_When_LLMCallError`, `..._When_SendKeysFailed`, `TestStuckReasonForExitKind_FallsBackToGenericText_When_ExitKindUnset` (new, Task 3.2.1b) | Unit (happy + error/fallback) | Kind-specific operator-visible reason text per `ExitKind`, plus a graceful non-panicking fallback for an unset/future `ExitKind` (`DriverExitReason` is not a closed enum) |
| REQ-3 | `server/services/backlog_service_test.go` | `TestAutoRespawnAutonomousWork_EndsAbandonedSession_When_KillConfirmsDead` (new, Task 3.3.1c) | Unit/integration (real SQLite storage) — happy path | Kill confirms dead → row ends → exactly one respawn (closes the ~20-25 min accidental respawn-delay gap) |
| REQ-3 | `server/services/backlog_service_test.go` | `TestAutoRespawnAutonomousWork_FailsClosed_When_KillDoesNotConfirmDead` (new, Task 3.3.1d) | Unit — error/failure path | Kill does not confirm dead → error returned, row NOT ended, no respawn (prevents two live agents on one worktree) |

### REQ-4 (Secondary): Regression tests exist that would fail against pre-fix behavior for both failure modes

Satisfied by the tests above that are explicitly documented as regression tests in
plan.md's own task text: `TestSpawnSessionFromItem_RacesWithDequeue_OnlyOneWorkSessionCreated`
(Task 1.1.2a — documented as the regression test for architecture.md §3b, and the plan
requires manually reverting Task 1.1.1b once during review to confirm it fails pre-fix),
`TestAutonomousOrchestrationService_OnAutonomousDriverComplete_StillEndsAbandonedReviewRow_When_ContextCancelled`
(Task 3.1.1b — documented as "would have failed against the earlier 'blanket early return'
revision"), and `TestAutoRespawnAutonomousWork_EndsAbandonedSession_When_KillConfirmsDead`
(Task 3.3.1c — documented as "the regression test for the ~20-25 minute accidental
respawn-delay gap").

## Step 3 Risk-Area Checks

1. **Dual-acquisition-site `spawnInFlight` mutual exclusion (`SpawnSessionFromItem` vs.
   `DequeueNextQueuedItems`), not just within one function.** PRESENT. Task 1.1.2a's
   `TestSpawnSessionFromItem_RacesWithDequeue_OnlyOneWorkSessionCreated` races the two
   distinct entry points against each other using the `dequeueSpawnPauseHook` to
   deterministically land inside the exact window, and asserts both (a) the concurrent
   `SpawnSessionFromItem` call fails fast with `CodeAlreadyExists` while the guard is held,
   and (b) after both complete, storage shows exactly one open work session. This is
   distinct from — and in addition to — the two existing single-site tests
   (`TestSpawnSessionFromItem_ConcurrentSpawns_OnlyOneWorkSessionCreated` and
   `TestDequeueNextQueuedItems_should_ClaimOnlyOneItem_When_CalledConcurrentlyWithOneFreeSlot`),
   which the adversarial review independently confirmed still pass unmodified. **No gap.**

2. **`dequeueSpawnPauseHook` leak/hang safety if a test fails mid-assertion.** GAP — see
   Coverage Gaps below. The adversarial review flagged this itself as a New Concern and it
   is not yet resolved by a plan task.

3. **`AutoRespawnAutonomousWork` fail-closed behavior vs. `RemediationDue`'s attempt
   counting.** GAP — see Coverage Gaps below. Also an adversarial-review New Concern with
   no corresponding plan task.

4. **Epic 2.5's malformed-streak-at-soft-cap interaction test.** CONFIRMED PRESENT. Task
   2.5.1b specifies exactly this test
   (`TestAutonomousDriver_MalformedStreakAtSoftCap_AbortsWithMalformedReason_NotSoftCapExtension`)
   plus its inverse composition test. The adversarial review independently verified the
   consolidated loop listing (Task 2.5.1a) matches both epics' own descriptions of check
   order. **No gap.**

5. **BUG-048 regression test (review session row still ends on
   `DriverExitContextCancelled`).** CONFIRMED PRESENT and would actually catch a
   regression. Task 3.1.1b's
   `TestAutonomousOrchestrationService_OnAutonomousDriverComplete_StillEndsAbandonedReviewRow_When_ContextCancelled`
   constructs a review-role outcome with `ExitKind: DriverExitContextCancelled` (mirroring
   `submit_review_verdict`'s belt-and-suspenders `Stop()` call while a review driver may
   still be mid-loop) and asserts `UpdateItemSessionEnded` **was** called (BUG-048's fix
   preserved) while `MarkStuck` was **not** called (this epic's actual fix). The
   adversarial review independently traced the code path (`SessionRoleReview`'s `default`
   branch at `autonomous_orchestration_service.go:449-477`, unconditionally reachable
   regardless of `ExitKind`) and confirmed this test would have failed against the
   earlier, reverted "blanket early return" design that caused the original BLOCKER.
   **No gap.**

## Test Stack

- **Unit**: Go stdlib `testing` package. `session/` package tests (e.g.
  `autonomous_driver_test.go`) use plain stdlib assertions (`if got != want { t.Errorf(...) }`)
  with hand-rolled fakes (`fakeHeadlessPool`, `fakePaneSettleChecker`) — no testify in this
  package today; new tests should match that convention. `server/services/` package tests
  use `github.com/stretchr/testify` (`require`/`assert`, confirmed dependency in `go.mod`)
  with hand-rolled mocks/fakes (`mockSessionCreator`, `mockSessionStopper`,
  `fakeAutonomousDriverStarter`, `fakeHeadlessPool`, `fakeGitHubResolver`). Naming
  convention in this codebase area is mixed but plan.md's proposed names correctly follow
  local precedent: `session/` tests use `TestXxx_Effect_When_Condition` (no "should", e.g.
  `TestAutonomousDriver_MaxTurnsLimit`, `TestAutonomousDriver_Stop_CancelsLoop`) while
  `server/services/backlog_service_triage_test.go`'s newer tests use
  `TestXxx_should_Effect_When_Condition` (e.g.
  `TestDequeueNextQueuedItems_should_ClaimOnlyOneItem_When_CalledConcurrentlyWithOneFreeSlot`) —
  plan.md's new test names are consistent with whichever file they land in.
- **Integration**: Same `go test` binaries, but backed by a real SQLite-backed
  `session.Storage` (`createTestStorage`/`createTestStorageWithRepo` in
  `server/services/session_service_test.go` / `backlog_service_triage_test.go`, using
  `session.NewEntRepository(session.WithDatabasePath(...))` against a `t.TempDir()`), run
  with `-race` for the concurrency-sensitive dedup tests. External process boundaries
  (tmux, the headless LLM pool) remain mocked (`mockSessionStopper`, `fakeHeadlessPool`);
  only the storage layer and in-process goroutine races are real.
- **API/E2E**: Not applicable — this is a backend-only reliability fix
  (`session/autonomous_driver.go`, `server/services/backlog_service_triage.go`,
  `server/services/autonomous_orchestration_service.go`, `config/config.go`). No proto
  changes, no new RPCs, no frontend touchpoints — requirements.md explicitly scopes UI
  treatment of stuck items out of this project. No Playwright/e2e coverage is required or
  specified by plan.md, consistent with that scope boundary.

## Coverage Gaps Found

1. **`dequeueSpawnPauseHook` goroutine-leak/hang safety is not tested or defended against
   at the task level (Step 3 risk area #2).** Task 1.1.2a specifies saving/restoring the
   hook itself via `t.Cleanup`, but does not specify a bounded-time unblock path for the
   *paused goroutine* if the test's own `CodeAlreadyExists` assertion fails before the
   unblock channel is signaled — exactly the failure mode the adversarial review's New
   Concerns section names ("a failed assertion can't leave the production method's own
   mutex held for the rest of the test run"). Left as-is, a failing assertion in this one
   new test could hang every other test in the package that touches `DequeueNextQueuedItems`
   (directly or transitively) until the test binary's overall timeout, misattributing the
   failure far from its actual cause.
   **Recommendation**: add task **1.1.2b: bound the pause hook's unblock wait with a
   timeout** — either (a) have the hook itself `select` on the unblock channel with a
   `time.After` fallback (e.g. 5s) that logs and proceeds rather than blocking forever, or
   (b) register the unblock send itself inside `t.Cleanup` (a non-blocking `select`/default
   send) so it fires even if the test function returns early via `t.Fatal`. Either closes
   the gap the adversarial review already identified; this plan simply never turned that
   New Concern into a task.

2. **No test (or documented acceptance) of whether a failed-closed `AutoRespawnAutonomousWork`
   consumes a `RemediationDue`/`MaxRemediationAttempts` backoff-gated attempt (Step 3 risk
   area #3).** The adversarial review's New Concerns section traces this precisely:
   `RemediationDue` is checked and "spent" synchronously in
   `onAutonomousDriverComplete`'s `SessionRoleWork` branch (line 358) *before* the
   `go func(){ AutoRespawnAutonomousWork(...) }()` goroutine that can return the new
   fail-closed error runs — so a transient `IsSessionLive` false-positive (pane briefly
   still reporting live right after `KillTmuxPaneOnly`, before tmux/the OS finishes
   teardown) silently consumes one of the item's limited remediation attempts for zero
   effect, a real behavior change from today's best-effort-always-proceed pattern. Neither
   plan.md's Story 3.3.1 acceptance criteria nor Tasks 3.3.1a/c/d mention this interaction,
   and no test asserts either outcome (attempt consumed vs. not consumed) for this
   specific case.
   **Recommendation**: add task **3.3.1e: document and assert the `RemediationDue`
   interaction** — at minimum, add one sentence to Story 3.3.1's acceptance criteria
   accepting the trade-off explicitly (a transient fail-closed result costs one remediation
   attempt with no compensating credit — an accepted, bounded cost since
   `MaxRemediationAttempts` is finite and the operator sees a stuck item one cycle longer,
   not indefinitely), and ideally a unit test
   (`TestAutoRespawnAutonomousWork_FailsClosed_DoesNotItselfCreditOrDebitRemediationAttempt`
   or similar) confirming `AutoRespawnAutonomousWork`'s own return value doesn't further
   mutate the remediation-attempt counter beyond whatever the caller already did before
   invoking it — i.e., the function itself doesn't double-count. If, after writing that
   test, the finding is "yes, this can silently burn attempts on a slow-to-die pane," the
   adversarial review's own suggested mitigation (a bounded 2-3 short-interval
   `IsSessionLive` retry/poll before giving up) should be adopted rather than accepted as
   a silent gap, since `MaxRemediationAttempts` parking an item prematurely due to tmux
   teardown latency — not genuine repeated failure — would itself become a new "vanished
   item" shape, the exact failure mode this project exists to eliminate.

Both gaps above were already identified by the Phase 3 adversarial review as "New
Concerns" (not blockers) but never converted into plan tasks in the two patch passes that
followed — this validation pass is flagging that they remain open, not surfacing them for
the first time. Neither blocks proceeding to Phase 5 (the adversarial review's own verdict
of CONCERNS, not BLOCKED, reflects this — these are hardening follow-ups, not correctness
holes in the primary dedup/turn-budget fixes), but they should be picked up either as
Phase 5 implementation-time additions to Stories 1.1.2/3.3.1, or as a fast-follow ticket if
deferred.

## Coverage Targets

- Unit test coverage: no numeric repo-wide gate is enforced by `make ci` for this project;
  the practical target is 100% of new/changed branches in `session/autonomous_driver.go`'s
  `run()` loop (every `ExitKind` assignment, the malformed sub-cap, the soft/hard cap
  check) and `onAutonomousDriverComplete`'s new `ExitKind`-conditioned guards — plan.md's
  task list already achieves this branch-level target per the mapping above.
- All public service methods touched by this project (`SpawnSessionFromItem`,
  `DequeueNextQueuedItems`, `AutoRespawnAutonomousWork`, `onAutonomousDriverComplete`,
  `AutonomousDriver.run`, `MaxAutonomousTurnsOrDefault`) have both a happy-path and at
  least one error-path test per the mapping above.
- All external integrations touched (tmux liveness via `SessionStopper`/`IsSessionLive`,
  SQLite-backed storage via `session.Storage`) are unit-mocked for the tmux boundary and
  covered by at least one real-storage integration/race test for the dedup guarantee
  (`TestSpawnSessionFromItem_RacesWithDequeue_OnlyOneWorkSessionCreated`) and the respawn
  row-ending guarantee (`TestAutoRespawnAutonomousWork_EndsAbandonedSession_When_KillConfirmsDead`).

## Readiness Gate

| # | Criterion | Pass? |
|---|-----------|-------|
| 1 | Every requirement in requirements.md has >=1 test case mapped in validation.md | **PASS** — REQ-1 (dedup), REQ-2 (adaptive turn budget), REQ-3 (well-defined visible stuck state), and REQ-4 (regression-test existence) each have >=1 mapped test above, each with both a happy-path and an error-path case, plus an integration-style case for REQ-1 and REQ-3. |
| 2 | plan.md has no TODO/TBD placeholders in architecture or task sections | **PASS** — grepped `project_plans/backlog-session-thrashing/implementation/plan.md` for `TODO`/`TBD`: zero matches. |
| 3 | All ADRs referenced in plan.md exist on disk | **PASS** — `ADR-001-progress-adaptive-turn-budget.md` exists at `project_plans/backlog-session-thrashing/decisions/`; the precedent ADR it cites, `ADR-001-staleness-threshold-recalibration.md`, exists at `project_plans/review-gate-stale-session-rework/decisions/`. Both verified present on disk in this worktree. |
| 4 | No BLOCKER items remain in adversarial-review.md | **PASS** — grepped for `BLOCKER`: zero matches in the current document body (the two historical blockers from pass 1 are referenced only in the "Prior Blocker Verification" section as resolved — `[PASS] Blocker 1...`, `[PASS, ...] Blocker 2...` — and the "New Blockers" section states "None found." Verdict is CONCERNS, not BLOCKED. |

**Overall verdict: CONCERNS.**

Rationale: all 4 gate criteria pass, and the plan's own test list already covers every
requirement and every Step-3 risk area except two — both of which are pre-existing
adversarial-review New Concerns that were never converted into plan tasks (dequeueSpawnPauseHook
goroutine-leak safety; AutoRespawnAutonomousWork's interaction with RemediationDue's
attempt-counting). Neither is a correctness hole in the primary dedup or turn-budget fix;
both are test-hygiene / edge-case hardening gaps that should be closed either before or
early in Phase 5 implementation (recommended as Tasks 1.1.2b and 3.3.1e above) rather than
blocking the start of implementation outright.
