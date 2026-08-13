# Adversarial Review: triage-autonomous-migration

**Date**: 2026-06-15
**Revision**: 2 (post-patch)
**Verdict**: CONCERNS

## Previous Blockers — Resolution Status

- [x] Blocker 1: CONFIRMED RESOLVED — `CreateDirectorySession` (session_service.go:600–603) still has no `StartController()` call in the source. Epic 0 / Task 0.1a correctly identifies the exact gap and provides the fix with a guarded `if ctrlErr := instance.StartController()` pattern mirroring the `CreateSession` goroutine. The patch is correct and complete.
- [x] Blocker 2: CONFIRMED RESOLVED — `storage.go:197–202` confirms `InstanceStore` exposes `ListInstanceData() ([]InstanceData, error)` and NOT `GetAll()`. Task 5.1d uses `store.ListInstanceData()` throughout `findSessionTitleByUUID`. The note "Do NOT use `store.GetAll()`" is correctly present in the Pitfall Reference.
- [x] Blocker 3: CONFIRMED RESOLVED — Source-verified: `autonomous_driver.go:229–232` falls through to `fireCompletion(Stuck=true)` after any `break` from the for-loop, including the context-cancel break at line 171. The plan correctly removes the MCP stop from `submit_triage_result` and limits it to `submit_review_verdict` where `SessionRoleReview` skips all status transitions (Epic 4). The residual Stuck signal from the stop call is genuinely harmless for the review role.

## Blockers

_(none)_

## Concerns

- [ ] **`StartController()` called only inside `statusManager != nil` guard** — Epic 0 Task 0.1a inserts `StartController()` inside the `if s.statusManager != nil` block (session_service.go:600–603). If `s.statusManager == nil` (e.g. in tests or stripped-down servers), `StartController()` is never called, but `StartAutonomousDriverWithTimeout` will still be invoked and fail at `GetController()` returning nil. The `TriggerTriage`/`TriggerReReview` guard (`useAutonomous := s.autonomousStarter != nil`) does not protect against this. Recommendation: move `StartController()` unconditionally after `session.StartSessionDriver(instance, path)`, or gate the autonomous path on both `s.autonomousStarter != nil` AND `s.statusManager != nil`.

- [ ] **`StartAutonomousDriverWithTimeout` duplicates callback wiring verbatim** — Task 2.2a copies the full `RegisterTurnCallback` body from `StartAutonomousDriverForInstance` (~15 lines). These two methods will inevitably diverge. Recommendation: extract a private `buildTurnCallback(inst)` helper so both call sites share one implementation, or have `StartAutonomousDriverWithTimeout` delegate to a shared internal method after constructing the driver with the custom timeout option.

- [ ] **`maxTurns=0` may cause the driver loop to never execute** — Task 2.2a passes `maxTurns=0` to `NewAutonomousDriver` for triage sessions. The loop condition is `for turnCount := 0; turnCount < d.maxTurns` (autonomous_driver.go:169). If `maxTurns=0`, the loop body never executes and the driver immediately fires `fireCompletion(Stuck=true)`. The plan does not document what `0` means. Recommendation: confirm whether `0` is treated as "unlimited" (with a special-case at loop init) or "zero turns". If it means unlimited, add a code comment. If not, set a non-zero default cap (e.g. 20) for triage/review sessions.

- [ ] **Triage DONE detection is fragile against LLM format variation** — The plan relies entirely on the orchestration LLM emitting a `DONE:` prefix after `submit_triage_result` is called. If the LLM returns a summary without the prefix, the driver will keep injecting turns. There is no secondary timeout-based completion path for triage (only the startup timeout and maxTurns). This is accepted risk but the plan contains no mitigation guidance (e.g. a triage-specific maxTurns cap, or a terminal-output heuristic). Recommendation: document this in the Pitfall Reference and set `maxTurns` to a bounded value for triage so runaway drivers are constrained.

## Minors

- Epic 0 test (Task 0.1b) asserts `GetController()` is non-nil after `CreateDirectorySession`. This test will pass vacuously if the test does not wire a non-nil `statusManager`, since `StartController()` is inside the `statusManager != nil` guard. The test must explicitly set a mock statusManager to exercise the fix.
- Pitfall #8 references a compile-time assertion `var _ AutonomousDriverStarter = (*SessionService)(nil)` adjacent to the interface definition. The interface lives in `backlog_service.go`; the assertion should go there or in `session_service.go` directly below the method implementations so it actually guards the right thing.
- Epic 4 Task 4.1a says "verify exact type name from `backlog_lifecycle.go:199–203`" without recording the type name in the plan. Under time pressure an implementer may guess incorrectly. Low severity; worth resolving before implementation starts.
- The `work→review` transition precondition (`ExpectedStatus=in_progress`) will silently drop the transition if the operator manually advanced the item's status. The behaviour is correct but should be mentioned in the PR description so operators understand the drop-on-conflict semantics.
