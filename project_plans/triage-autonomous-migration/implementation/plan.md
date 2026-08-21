# Implementation Plan: triage-autonomous-migration

**Feature**: Fix hidden session leak and add AutonomousDriver to triage/re-review sessions
**Date**: 2026-06-15
**Status**: Ready for implementation
**ADRs**: decisions/ADR-001-watchsessions-hidden-unconditional-filter.md, decisions/ADR-002-triage-autonomous-driver-over-oneshot.md, decisions/ADR-003-triage-completion-signaler-interface.md

---

## Adversarial Review Patches (applied 2026-06-15)

Three blockers were identified and resolved before implementation:

**Blocker 1 (resolved in Epic 0)**: `CreateDirectorySession` never calls `instance.StartController()`. The async goroutine in `CreateSession` (lines 1150–1154) does, but `CreateDirectorySession` (used by TriggerTriage/TriggerReReview) does not. When `StartAutonomousDriverWithTimeout` is called immediately after, `GetController()` returns `nil` at `autonomous_driver.go:104`, `Start()` returns an error, and the driver never runs. Fix: add `instance.StartController()` call to `CreateDirectorySession` after `SetStatusManager`.

**Blocker 2 (resolved in Epic 5)**: `InstanceStore` has no `GetAll()` method. The interface (storage.go:197) exposes `ListInstanceData()` which returns `[]InstanceData` with UUID fields. Fix: `findInstanceByUUID` uses `store.ListInstanceData()` instead of `store.GetAll()`.

**Blocker 3 (resolved in Epic 5)**: `Stop()` always fires `fireCompletion(Stuck=true)` because the driver loop exits via context cancel and falls through to lines 229–232 unconditionally. A MCP-triggered `StopDriverForSession` call causes `onAutonomousDriverComplete(Stuck=true)` to fire — this is not "belt and suspenders", it is a forced interruption that produces an incorrect Stuck signal while `submit_triage_result` is still processing. Fix: remove the active stop-signal from Epic 5. Instead, rely solely on LLM detection (`DONE:` prefix in the orchestration response) which fires naturally after the agent calls `submit_triage_result` and becomes idle. The stop interface is retained but only used for cleanup on session delete/hibernate (existing behavior), not as a triage completion hook.

---

## Dependency Visualization

```
Epic 0 (StartController fix in CreateDirectorySession) ──────────► blocks Epics 2, 3
Epic 1 (WatchSessions hidden filter)   ──────────────────────────► independent
Epic 6 (AutonomousDriver startup timeout) ──────────────────────► independent
Epic 2 (TriggerTriage → AutonomousDriver) ──needs Epics 0+6────► after Epics 0, 6
Epic 3 (TriggerReReview → AutonomousDriver) ──needs Epics 0+6──► after Epics 0, 6
Epic 4 (role-aware completion callback) ──needs Epic 2+3──────► after Epics 2, 3
Epic 5 (submit_review_verdict stop signal) ──needs Epic 3──────► after Epic 3
Epic 8 (stuck-triage notification) ────────needs Epic 4───────► after Epic 4
```

Start Epics 0, 1, and 6 in parallel. Then Epics 2 and 3 (both after 0 and 6). Then Epics 4 and 5 (both after 2 and 3). Then Epic 8 (after Epic 4).

---

## Phase 0: Critical prerequisite (blocks Epics 2 and 3)

### Epic 0: Add `StartController()` to `CreateDirectorySession`
**Goal**: Ensure the ClaudeController is started before `StartAutonomousDriverWithTimeout` is called, so `GetController()` returns a non-nil controller.

**Root cause**: `CreateSession` (the UI-facing handler) starts the controller in its async goroutine (lines 1150–1154) with this comment: "Wire the status manager and start the controller AFTER Start() returns so the tmux attach-session process has had time to fully initialize." `CreateDirectorySession` (the internal path used by TriggerTriage/TriggerReReview) follows the same `SetStatusManager` pattern but never calls `StartController()`. When `StartAutonomousDriverForInstance` runs, `d.inst.GetController()` at `autonomous_driver.go:104` returns `nil`, causing `Start()` to return an error (line 110) that is logged and swallowed. The AutonomousDriver silently never starts.

#### Story 0.1: Call `StartController` in `CreateDirectorySession`
**As a** session created via `CreateDirectorySession`, **I want** the ClaudeController started before returning, **so that** `AutonomousDriver.Start()` succeeds.

**Acceptance Criteria**:
- `CreateDirectorySession` calls `instance.StartController()` after `instance.SetStatusManager(s.statusManager)`.
- If `StartController()` returns an error, it is logged as a warning (same pattern as lines 1152–1154 in the `CreateSession` goroutine) and the session is still returned.
- `StartAutonomousDriverForInstance` called immediately after `CreateDirectorySession` returns no longer returns `"no controller available"` error.

**Files**: `server/services/session_service.go`

##### Task 0.1a: Add StartController call to CreateDirectorySession (~3 min)
File: `server/services/session_service.go`, lines 600–603.

Replace:
```go
if s.statusManager != nil {
    instance.SetStatusManager(s.statusManager)
}
session.StartSessionDriver(instance, path)
```
With:
```go
if s.statusManager != nil {
    instance.SetStatusManager(s.statusManager)
    if ctrlErr := instance.StartController(); ctrlErr != nil {
        log.Warn("[CreateDirectorySession] failed to start controller after wiring", "session", title, "err", ctrlErr)
    }
}
session.StartSessionDriver(instance, path)
```

This mirrors the exact pattern at lines 1150–1154 in the `CreateSession` async goroutine.

##### Task 0.1b: Write a test confirming GetController is non-nil after CreateDirectorySession (~4 min)
File: `server/services/session_service_test.go`.

Test: call `CreateDirectorySession` with a non-nil `statusManager` wired. Assert that the returned instance's `GetController()` is non-nil.

---

## Phase 1: Foundation fixes (independent)

### Epic 1: WatchSessions hidden filter
**Goal**: All three WatchSessions code paths must suppress sessions where `inst.Hidden == true`, mirroring `ListSessions` default behaviour.

#### Story 1.1: Hide hidden sessions in WatchSessions
**As a** client connected to WatchSessions, **I want** triage and review sessions to be invisible, **so that** the main session list is not polluted with hidden backlog sessions.

**Acceptance Criteria**:
- Initial snapshot loop does not send events for `inst.Hidden == true`.
- Real-time event loop does not forward events where `event.Session.Hidden == true`.
- Reconnect replay (`EventsSince`) does not replay events for hidden sessions.
- Existing tests for `WatchSessions` still pass.

**Files**: `server/services/session_service.go`

##### Task 1.1a: Add hidden filter to initial snapshot loop (~3 min)
File: `server/services/session_service.go`, lines 1655–1668.

After the existing `StatusFilter` block (line 1664), add:
```go
if inst.Hidden {
    continue
}
```
This mirrors the guard at line 798 in `ListSessions`.

##### Task 1.1b: Add hidden filter to real-time event loop (~3 min)
File: `server/services/session_service.go`, lines 1684–1695.

After the existing `StatusFilter` block (line 1694), add:
```go
if event.Session != nil && event.Session.Hidden {
    continue
}
```

##### Task 1.1c: Add hidden filter to EventsSince replay path (~3 min)
File: `server/services/session_service.go`, lines 1633–1640.

The `EventsSince` loop at line 1636 has NO filtering at all (no category, status, or hidden filter). After the send call, add the hidden check before the send:
```go
for _, event := range s.eventBus.EventsSince(req.Msg.AfterSeq) {
    if event.Session != nil && event.Session.Hidden {
        continue
    }
    if err := stream.Send(convertEventToProto(event)); err != nil {
        return fmt.Errorf("failed to send replayed event: %w", err)
    }
}
```

##### Task 1.1d: Write Go test for hidden filter (~4 min)
File: `server/services/session_service_test.go` (or a new `watch_sessions_test.go`).

Test cases:
- Hidden session NOT in initial snapshot.
- Hidden session NOT forwarded via live event.
- Non-hidden session IS in initial snapshot.

---

### Epic 6: AutonomousDriver configurable startup timeout
**Goal**: Make the 60-second startup idle-wait configurable so triage sessions (which spawn parallel subagents) can use an extended timeout (e.g. 300s) without hitting the hardcoded limit.

#### Story 6.1: Add `WithStartupTimeout` functional option to AutonomousDriver
**As a** caller of `NewAutonomousDriver`, **I want** to pass an optional startup timeout, **so that** triage sessions can tolerate slower initial idle states without spurious stuck signals.

**Acceptance Criteria**:
- `NewAutonomousDriver` accepts functional options.
- `WithStartupTimeout(d time.Duration)` overrides the default 60s timeout.
- When not passed, default is 60s (no regression).
- `TriggerTriage` and `TriggerReReview` pass `WithStartupTimeout(5 * time.Minute)`.

**Files**: `session/autonomous_driver.go`, `server/services/backlog_service.go`, `server/services/session_service.go`

##### Task 6.1a: Add functional option type and field (~3 min)
File: `session/autonomous_driver.go`.

1. Add a `startupTimeout time.Duration` field to `AutonomousDriver` struct (after `maxTurns`).
2. Add a `DriverOption` functional type and `WithStartupTimeout` option:
```go
type DriverOption func(*AutonomousDriver)

func WithStartupTimeout(d time.Duration) DriverOption {
    return func(a *AutonomousDriver) { a.startupTimeout = d }
}
```
3. Update `NewAutonomousDriver` signature to accept variadic `...DriverOption`:
```go
func NewAutonomousDriver(inst *Instance, pool HeadlessPoolClient, goal string, maxTurns int, opts ...DriverOption) *AutonomousDriver {
```
4. Apply options after struct init and set default if `startupTimeout == 0`:
```go
for _, o := range opts {
    o(d)
}
if d.startupTimeout == 0 {
    d.startupTimeout = 60 * time.Second
}
```

##### Task 6.1b: Use `d.startupTimeout` in `run()` (~2 min)
File: `session/autonomous_driver.go`, line 158.

Change:
```go
startupCtx, startupCancel := context.WithTimeout(ctx, 60*time.Second)
```
To:
```go
startupCtx, startupCancel := context.WithTimeout(ctx, d.startupTimeout)
```

##### Task 6.1c: Update all callers of `NewAutonomousDriver` (~3 min)
Files: `server/services/session_service.go` (lines 678, ~1163).

Both existing calls pass no options — adding `...DriverOption` with no args is backwards-compatible. No changes needed at call sites for the default case; verify that the variadic signature compiles with zero options.

##### Task 6.1d: Unit test for configurable timeout (~3 min)
File: `session/autonomous_driver_test.go` (new or existing).

Test: `NewAutonomousDriver` with `WithStartupTimeout(5*time.Minute)` sets `startupTimeout` to 5 minutes. Default (no option) is 60 seconds.

---

## Phase 2: Triage/ReReview AutonomousDriver wiring (depends on Phase 1 Epic 6)

### Epic 2: TriggerTriage — switch from oneShot to AutonomousDriver
**Goal**: `TriggerTriage` creates a non-oneShot session and starts an `AutonomousDriver` with extended startup timeout. Falls back to oneShot when `headlessPool` is nil.

#### Story 2.1: Remove oneShot and start AutonomousDriver in TriggerTriage
**As a** triage session, **I want** to run with MCP enabled and multi-turn orchestration, **so that** `submit_triage_result` can be called and the AutonomousDriver can reinject the goal if the session stalls.

**Acceptance Criteria**:
- `TriggerTriage` calls `CreateDirectorySession` with `oneShot=false` when `s.autonomousStarter != nil`.
- When `s.autonomousStarter == nil` (headlessPool unavailable), falls back to `oneShot=true` (graceful degradation).
- An `AutonomousDriver` is started with `WithStartupTimeout(5 * time.Minute)` for the triage session.
- `isOneShot()` tag-based no-retry logic in `session_driver.go` is unaffected (it reads tags, not `OneShot` field).

**Files**: `server/services/backlog_service.go`

##### Task 2.1a: Branch on autonomousStarter in TriggerTriage (~4 min)
File: `server/services/backlog_service.go`, lines 1177–1200.

Replace the single `CreateDirectorySession` call (line 1179) with a branched call:
```go
// 8. Spawn triage session — AutonomousDriver mode if available, oneShot fallback.
title := "triage:" + slug
useAutonomous := s.autonomousStarter != nil
inst, err := s.sessionCreator.CreateDirectorySession(ctx, title, item.RepoPath, triagePrompt,
    []string{"backlog:triage"}, !useAutonomous /*oneShot*/, true /*hidden*/)
if err != nil {
    return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to spawn triage session: %w", err))
}
if useAutonomous {
    s.autonomousStarter.StartAutonomousDriverForInstance(inst)
}
```

Note: `StartAutonomousDriverForInstance` already guards `headlessPool == nil` internally (line 674), but we use `autonomousStarter != nil` as the outer gate because `autonomousStarter` IS the mechanism for starting the driver.

#### Story 2.2: Thread WithStartupTimeout into StartAutonomousDriverForInstance
**As a** triage AutonomousDriver, **I want** a 300s startup timeout, **so that** parallel subagent spawning doesn't trigger a spurious stuck signal.

**Acceptance Criteria**:
- `AutonomousDriverStarter` interface updated to accept an optional timeout parameter OR a separate method is added for triage.
- Simpler approach: add a new method `StartAutonomousDriverWithTimeout(inst *session.Instance, startupTimeout time.Duration)` to `SessionService` and wire `BacklogService` to call it.
- Existing `StartAutonomousDriverForInstance` is unchanged (uses default 60s).

**Files**: `server/services/backlog_service.go`, `server/services/session_service.go`

##### Task 2.2a: Add `StartAutonomousDriverWithTimeout` to SessionService (~4 min)
File: `server/services/session_service.go`.

Add method after `StartAutonomousDriverForInstance` (line 703):
```go
// StartAutonomousDriverWithTimeout is like StartAutonomousDriverForInstance but
// uses a configurable startup timeout for sessions that need a longer warm-up
// (e.g. triage sessions that spawn parallel subagents).
func (s *SessionService) StartAutonomousDriverWithTimeout(inst *session.Instance, startupTimeout time.Duration) {
    if s.headlessPool == nil {
        log.Warn("[SessionService] StartAutonomousDriverWithTimeout: headlessPool is nil", "session", inst.Title)
        return
    }
    driver := session.NewAutonomousDriver(inst, s.headlessPool, inst.Prompt, 0, session.WithStartupTimeout(startupTimeout))
    driver.RegisterCompletionCallback(s.onAutonomousDriverComplete)
    driver.RegisterTurnCallback(func(turn, maxTurns int, prompt string) {
        // same body as StartAutonomousDriverForInstance
        if liveInst := s.FindLiveInstance(inst.Title); liveInst != nil {
            liveInst.AutonomousTurn = int32(turn)
            liveInst.AutonomousMaxTurns = int32(maxTurns)
            s.eventBus.Publish(events.NewSessionUpdatedEvent(liveInst, []string{"autonomous_turn"}))
        }
        truncated := prompt
        if len(truncated) > 120 {
            truncated = truncated[:120] + "…"
        }
        s.eventBus.Publish(events.NewNotificationEvent(
            inst.UUID, inst.Title, fmt.Sprintf("autonomous-turn-%s-%d", inst.UUID, turn),
            int32(10), int32(1),
            fmt.Sprintf("Autonomous turn %d/%d", turn, maxTurns),
            fmt.Sprintf("%s: %s", inst.Title, truncated),
            nil,
        ))
    })
    if err := driver.Start(s.driverCtx()); err != nil {
        log.Warn("[SessionService] failed to start autonomous driver", "session", inst.Title, "err", err)
        return
    }
    s.registerDriver(inst.Title, driver)
}
```

##### Task 2.2b: Extend AutonomousDriverStarter interface in BacklogService (~3 min)
File: `server/services/backlog_service.go`, lines 29–33.

Add the new method to the interface:
```go
type AutonomousDriverStarter interface {
    StartAutonomousDriverForInstance(inst *session.Instance)
    StartAutonomousDriverWithTimeout(inst *session.Instance, startupTimeout time.Duration)
}
```

Update `TriggerTriage` (Task 2.1a) to call `s.autonomousStarter.StartAutonomousDriverWithTimeout(inst, 5*time.Minute)` instead of `StartAutonomousDriverForInstance`.

##### Task 2.2c: Add compile-time assertion for AutonomousDriverStarter interface (~1 min)
File: `server/services/session_service.go` (immediately after the `StartAutonomousDriverWithTimeout` method added in Task 2.2a).

Add:
```go
// Compile-time assertion: SessionService must implement AutonomousDriverStarter.
// This ensures that adding a method to the interface in backlog_service.go produces
// a compile error here rather than a runtime panic in tests or production.
var _ backlogpkg.AutonomousDriverStarter = (*SessionService)(nil)
```

Where `backlogpkg` is the import alias for `server/services`. If the assertion is in the same package, use the unqualified type name: `var _ AutonomousDriverStarter = (*SessionService)(nil)` — but note `AutonomousDriverStarter` is defined in `backlog_service.go` which is in the same package as `session_service.go` (`server/services`). So the assertion goes directly without an import alias.

**Mock note**: Any existing or future test mock that implements `AutonomousDriverStarter` must also add `StartAutonomousDriverWithTimeout`. Search for `AutonomousDriverStarter` implementations in `_test.go` files before closing the PR.

---

### Epic 3: TriggerReReview — same pattern as TriggerTriage
**Goal**: `TriggerReReview` creates a non-oneShot session and starts an `AutonomousDriver` with extended startup timeout. Falls back to oneShot when `autonomousStarter` is nil.

#### Story 3.1: Remove oneShot and start AutonomousDriver in TriggerReReview
**As a** re-review session, **I want** to run with MCP enabled and multi-turn orchestration, **so that** `submit_review_verdict` can be called and the AutonomousDriver can reinject the goal if the session stalls.

**Acceptance Criteria**:
- `TriggerReReview` calls `CreateDirectorySession` with `oneShot=false` when `s.autonomousStarter != nil`.
- Falls back to `oneShot=true` when `autonomousStarter == nil`.
- `AutonomousDriver` is started with `WithStartupTimeout(5 * time.Minute)`.

**Files**: `server/services/backlog_service.go`

##### Task 3.1a: Branch on autonomousStarter in TriggerReReview (~4 min)
File: `server/services/backlog_service.go`, lines 1530–1554.

Replace the single `CreateDirectorySession` call (line 1533) with:
```go
// 10. Spawn re-review session — AutonomousDriver mode if available, oneShot fallback.
slug := slugify(item.Title)
title := "re-review:" + slug
useAutonomous := s.autonomousStarter != nil
inst, spawnErr := s.sessionCreator.CreateDirectorySession(ctx, title, item.RepoPath, reReviewPrompt,
    []string{"backlog:review"}, !useAutonomous /*oneShot*/, true /*hidden*/)
if spawnErr != nil {
    return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to spawn re-review session: %w", spawnErr))
}
if useAutonomous {
    s.autonomousStarter.StartAutonomousDriverWithTimeout(inst, 5*time.Minute)
}
```

---

## Phase 3: Completion signals (depends on Phase 2 Epics 2+3)

### Epic 4: Role-aware completion callback in onAutonomousDriverComplete
**Goal**: `onAutonomousDriverComplete` detects whether the finishing session is a triage session (role=triage) or a work session (role=work) and transitions the backlog item to the correct status.

#### Story 4.1: Check ItemSession.SessionRole before transitioning status
**As a** triage session completing its work, **I want** the backlog item to transition to `ready` (not `review`), **so that** the operator can review the triage output before spawning a work session.

**Acceptance Criteria**:
- When `is.SessionRole == session.SessionRoleTriage` AND `outcome.Done == true`, transition to `BacklogStatusReady`.
- When `is.SessionRole == session.SessionRoleTriage` AND `outcome.Stuck == true`, do NOT transition (triage did not complete; item stays at current status so the operator can re-trigger).
- When `is.SessionRole == session.SessionRoleWork`, transition to `BacklogStatusReview` regardless of Done/Stuck (existing behaviour).
- When `is.SessionRole == session.SessionRoleReview`, do NOT transition (re-review completion is handled by `submit_review_verdict` separately).
- Add `ExpectedStatus` precondition on transitions: triage→ready uses `ExpectedStatus=idea`, work→review uses `ExpectedStatus=in_progress` (mirrors `BacklogLifecycleListener` pattern at `backlog_lifecycle.go:199–203`).

**Files**: `server/services/session_service.go`

##### Task 4.1a: Add role-aware status selection in onAutonomousDriverComplete (~5 min)
File: `server/services/session_service.go`, lines 3446–3458.

Replace the hardcoded `toStatus := session.BacklogStatusReview` block:
```go
is, err := concreteStorage.GetItemSessionBySessionUUID(ctx, sessionUUID)
if err == nil && is != nil {
    item, itemErr := is.Edges.BacklogItemOrErr()
    if itemErr == nil && item != nil {
        var toStatus session.BacklogStatus
        var expectedStatus string
        switch is.SessionRole {
        case session.SessionRoleTriage:
            if !outcome.Done {
                // Triage was interrupted/stuck — do not advance the item.
                // The operator can re-trigger triage; advancing to ready with no
                // plan artifacts would be misleading.
                log.Info("[AutonomousDriver] triage stuck, skipping status transition", "item", item.ID, "reason", outcome.Reason)
                return
            }
            toStatus = session.BacklogStatusReady
            expectedStatus = string(session.BacklogStatusIdea)
        case session.SessionRoleWork:
            toStatus = session.BacklogStatusReview
            expectedStatus = string(session.BacklogStatusInProgress)
        default:
            // SessionRoleReview and unknown roles: no transition from AutonomousDriver.
            // Review outcomes are managed by submit_review_verdict.
            log.Info("[AutonomousDriver] skipping status transition for role", "role", is.SessionRole, "item", item.ID)
            return
        }
        precondition := &session.TransitionPrecondition{ExpectedStatus: expectedStatus}
        if _, transErr := concreteStorage.TransitionBacklogItemStatus(ctx, item.ID.String(), toStatus, precondition); transErr != nil {
            log.Warn("[AutonomousDriver] failed to transition backlog item", "item", item.ID, "to", toStatus, "err", transErr)
        } else {
            log.Info("[AutonomousDriver] backlog item transitioned", "item", item.ID, "to", toStatus, "done", outcome.Done)
        }
    }
}
```

Note: Check the exact signature of `TransitionBacklogItemStatus` in `session/storage.go` to confirm the precondition parameter type name. The existing call at line 3453 passes `nil` for the precondition, so the type is nullable. Use the same struct that `BacklogLifecycleListener` uses at `backlog_lifecycle.go:199–203`.

##### Task 4.1b: Write unit test for role-aware completion (~4 min)
File: `server/services/session_service_test.go` (or new `autonomous_completion_test.go`).

Test cases:
- Role=triage, Done=true → item transitions to `BacklogStatusReady` with `ExpectedStatus=idea`.
- Role=triage, Stuck=true → no transition (mock `TransitionBacklogItemStatus` not called).
- Role=work, Done=true → item transitions to `BacklogStatusReview` with `ExpectedStatus=in_progress`.
- Role=work, Stuck=true → item transitions to `BacklogStatusReview` (work stuck still advances to review for human inspection).
- Role=review → no transition regardless of Done/Stuck.

---

### Epic 5: submit_review_verdict stop signal for re-review AutonomousDriver
**Goal**: When `submit_review_verdict` completes successfully, the AutonomousDriver running on the re-review session is stopped so it does not inject further turns after the verdict is already submitted.

**Design note — why no MCP stop for submit_triage_result**: An earlier design proposed stopping the AutonomousDriver from `submit_triage_result`. This was rejected because `AutonomousDriver.Stop()` cancels the context and the driver loop unconditionally calls `fireCompletion(Stuck=true)` after the loop exits (lines 229–232 of `autonomous_driver.go`). Calling `Stop()` from `submit_triage_result` would trigger `onAutonomousDriverComplete(Stuck=true)` — producing a Stuck status transition while the triage was actually successful. For triage, the LLM-based DONE detection is sufficient: after the agent calls `submit_triage_result`, the session becomes idle, the orchestrator LLM reads the successful call in the terminal tail, and emits `DONE:` on its next turn. This fires `onAutonomousDriverComplete(Done=true)` cleanly.

For re-review (`submit_review_verdict`), the same LLM detection applies. However, for belt-and-suspenders we add an explicit post-verdict stop call. Since the verdict is already committed before the stop, a subsequent Stuck transition is harmless for the review role (Epic 4 skips all transitions for `SessionRoleReview`). So the stop call here is safe.

**Why Epic 5 only covers re-review, not triage**: Triage relies on LLM DONE detection (safe by design). Re-review gets the explicit stop as a redundant safety net because review role completions do not trigger status transitions regardless.

#### Story 5.1: Stop re-review AutonomousDriver after submit_review_verdict
**As a** re-review session that has called `submit_review_verdict`, **I want** the AutonomousDriver to stop, **so that** no extra orchestration turns are injected after the verdict is submitted.

**Acceptance Criteria**:
- A new narrow interface `ReviewCompletionSignaler` with `StopDriverForSession(sessionTitle string)` is defined in `server/mcp/tools_backlog.go`.
- `SessionService` implements `StopDriverForSession` by calling `stopAndDeregisterDriver(title)`.
- `backlogHandlers` struct gains an optional `reviewStopper ReviewCompletionSignaler` field.
- `submitReviewVerdict` calls `h.reviewStopper.StopDriverForSession(callerTitle)` after the verdict is persisted, if `h.reviewStopper != nil`.
- When `reviewStopper` is nil, behaviour is unchanged (graceful degradation).

**Files**: `server/mcp/tools_backlog.go`, `server/mcp/server.go`, `server/services/session_service.go`

##### Task 5.1a: Define ReviewCompletionSignaler interface and wire backlogHandlers field (~3 min)
File: `server/mcp/tools_backlog.go`.

Add above `backlogHandlers` struct:
```go
// ReviewCompletionSignaler allows the MCP handler to stop an AutonomousDriver
// after submit_review_verdict completes. The stop call is belt-and-suspenders;
// the LLM orchestrator will also detect completion from the terminal tail.
// Note: Stop() fires fireCompletion(Stuck=true), but the role-aware callback
// in Epic 4 skips all status transitions for SessionRoleReview, so this is safe.
type ReviewCompletionSignaler interface {
    StopDriverForSession(sessionTitle string)
}
```

Add `reviewStopper ReviewCompletionSignaler` field to `backlogHandlers` struct.

##### Task 5.1b: Implement StopDriverForSession on SessionService (~2 min)
File: `server/services/session_service.go`.

Add method after `stopAndDeregisterDriver`:
```go
// StopDriverForSession stops the AutonomousDriver registered under sessionTitle.
// Used by MCP handlers as a belt-and-suspenders stop after task completion.
// Satisfies mcp.ReviewCompletionSignaler.
func (s *SessionService) StopDriverForSession(sessionTitle string) {
    s.stopAndDeregisterDriver(sessionTitle)
}
```

##### Task 5.1c: Wire reviewStopper in MCP server construction (~2 min)
File: `server/mcp/server.go`.

Update `backlogHandlers` construction to pass `svc` as `reviewStopper`:
```go
registerBacklogTools(s, &backlogHandlers{
    storage:       storage,
    store:         store,
    eventBus:      eventBus,
    reviewStopper: svc,
})
```

##### Task 5.1d: Add stop call in submitReviewVerdict (~3 min)
File: `server/mcp/tools_backlog.go`, in the `submitReviewVerdict` handler after the verdict is persisted.

After verdict is committed to DB:
```go
// Stop the AutonomousDriver for this review session.
// The verdict is already persisted; a subsequent Stuck fireCompletion is harmless
// because the role-aware callback (Epic 4) skips transitions for SessionRoleReview.
if h.reviewStopper != nil {
    if title, err := findSessionTitleByUUID(h.store, callerUUID); err == nil {
        h.reviewStopper.StopDriverForSession(title)
    }
}
```

Add helper `findSessionTitleByUUID` in `tools_backlog.go`:
```go
// findSessionTitleByUUID returns the session Title for callerUUID using ListInstanceData.
// InstanceStore.ListInstanceData() returns []InstanceData which carries UUID and Title.
// Returns "" and an error if not found.
func findSessionTitleByUUID(store session.InstanceStore, uuid string) (string, error) {
    instances, err := store.ListInstanceData()
    if err != nil {
        return "", err
    }
    for _, d := range instances {
        if d.UUID == uuid {
            return d.Title, nil
        }
    }
    return "", fmt.Errorf("no session found with UUID %s", uuid)
}
```

Note: `InstanceStore.ListInstanceData()` is confirmed to exist (storage.go:197). `InstanceData` carries `UUID` and `Title` fields. Do NOT use `store.GetAll()` — that method does not exist on `InstanceStore`.

##### Task 5.1e: Write unit test for driver stop in submitReviewVerdict (~4 min)
File: `server/mcp/tools_backlog_test.go`.

Test: when `submitReviewVerdict` succeeds, a mock `ReviewCompletionSignaler.StopDriverForSession` is called with the correct session title.

---

## Phase 3b: Stuck-triage notification (depends on Phase 3 Epic 4)

### Epic 8: Emit NotificationEvent when triage is stuck
**Goal**: When `onAutonomousDriverComplete` detects `Stuck=true` for a triage session, emit a `NotificationEvent` so the operator is informed rather than silently waiting.

**Background**: Epic 4 Task 4.1a already handles the case where triage is stuck by returning early without a status transition. Without this epic, there is no user-visible signal at all — the backlog item stays at `idea` indefinitely with no indication that triage failed.

**Design note**: Only emit the notification for `SessionRoleTriage`. Work sessions (`SessionRoleWork`) that are stuck already advance to `BacklogStatusReview` (where a human can see the stuck state in the session terminal). Review sessions (`SessionRoleReview`) are out of scope (their stuck state is handled by the review workflow itself).

#### Story 8.1: Publish stuck-triage NotificationEvent from onAutonomousDriverComplete
**As an** operator who triggered triage, **I want** a notification when triage gets stuck, **so that** I know to re-trigger it rather than waiting indefinitely.

**Acceptance Criteria**:
- When `is.SessionRole == session.SessionRoleTriage` AND `outcome.Stuck == true` (equivalently, `outcome.Done == false`), publish a `NotificationEvent` to `s.eventBus`.
- Notification: title = `"Triage stuck"`, body = `"<item title>: autonomous triage did not complete"`, severity = warning (priority 20), actionable = true.
- The `NotificationEvent` is published BEFORE the early return (so the operator is notified even though no status transition happens).
- When `headlessPool` is nil or `autonomousStarter` is nil (non-autonomous mode), this code path is unreachable (no driver, no callback). No change needed for that path.

**Files**: `server/services/session_service.go`

##### Task 8.1a: Add stuck-triage notification in onAutonomousDriverComplete (~4 min)
File: `server/services/session_service.go`, inside the `case session.SessionRoleTriage:` block added by Task 4.1a.

Inside the `if !outcome.Done { ... return }` block, add a notification publish before the return:
```go
case session.SessionRoleTriage:
    if !outcome.Done {
        // Notify the operator that triage got stuck.
        // Item stays at current status (idea) — no status transition.
        // The operator can re-trigger triage from the backlog item.
        s.eventBus.Publish(events.NewNotificationEvent(
            item.ID.String(),
            "Triage stuck",
            fmt.Sprintf("stuck-triage-%s", item.ID),
            int32(20), // warning severity
            int32(1),
            "Triage did not complete",
            fmt.Sprintf("%s: autonomous triage session got stuck", item.Title),
            nil,
        ))
        log.Info("[AutonomousDriver] triage stuck, notified operator", "item", item.ID, "reason", outcome.Reason)
        return
    }
```

Check `events.NewNotificationEvent` signature in `session/events/events.go` (or `server/services/session_service.go` existing call site at ~line 283) to confirm parameter order. Match the existing call pattern exactly.

##### Task 8.1b: Write unit test for stuck-triage notification (~3 min)
File: `server/services/session_service_test.go` (or `autonomous_completion_test.go`).

Extend the test from Task 4.1b:
- Role=triage, Stuck=true → `eventBus.Publish` called with a `NotificationEvent` containing title "Triage stuck".
- Role=triage, Done=true → `eventBus.Publish` NOT called with "Triage stuck" (no spurious notification on success).
- Role=work, Stuck=true → `eventBus.Publish` NOT called with "Triage stuck" (work stuck does not trigger triage notification).

---

## Phase 4: Tests and CI validation

### Epic 7: Integration test and make quick-check
**Goal**: All changes pass `make quick-check` (build + test + lint).

#### Story 7.1: Integration validation
**As a** developer, **I want** all changes to pass CI locally, **so that** the PR does not fail.

**Files**: All modified files

##### Task 7.1a: Run make quick-check (~2 min)
```bash
make quick-check
```
Fix any lint or build errors before raising the PR.

##### Task 7.1b: Run targeted Go tests for changed packages (~2 min)
```bash
go test ./server/services/... ./server/mcp/... ./session/...
```

---

## Implementation Order (strict)

1. **Epics 0, 1, 6** (in parallel) — Epic 0 (StartController fix), Epic 1 (WatchSessions filter), Epic 6 (startup timeout) are all independent and can be done simultaneously.
2. **Epics 2 + 3** (TriggerTriage + TriggerReReview) — after Epics 0 AND 6 are complete.
3. **Epics 4 + 5** (role-aware completion + review verdict stop signal) — after Epics 2 and 3.
4. **Epic 8** (stuck-triage notification) — after Epic 4 (requires the role-aware triage stuck branch from 4.1a).
5. **Epic 7** (CI validation) — last.

---

## Key File Inventory

| File | Epic(s) | What Changes |
|---|---|---|
| `server/services/session_service.go` | 0, 1, 2.2a, 2.2c, 4.1a, 5.1b, 8.1a | StartController in CreateDirectorySession; WatchSessions filter; new timeout method; compile-time assertion; role-aware completion; StopDriverForSession; stuck-triage notification |
| `server/services/backlog_service.go` | 2.1a, 2.2b, 3.1a | TriggerTriage + TriggerReReview; extended AutonomousDriverStarter interface |
| `session/autonomous_driver.go` | 6.1a, 6.1b | DriverOption type; WithStartupTimeout; d.startupTimeout in run() |
| `server/mcp/tools_backlog.go` | 5.1a, 5.1d | ReviewCompletionSignaler; reviewStopper field; stop call in submitReviewVerdict; findSessionTitleByUUID helper |
| `server/mcp/server.go` | 5.1c | Wire svc as reviewStopper into backlogHandlers |

---

## Pitfall Reference

1. **isOneShot() is tag-based** (`session/session_driver.go:488`) — reads `backlog:triage` / `backlog:review` tags. Removing `oneShot: true` field does NOT break retry suppression.
2. **nil autonomousStarter panic** — always gate on `s.autonomousStarter != nil` in TriggerTriage/TriggerReReview before calling.
3. **driverRegistry keyed by title, not UUID** — `StopDriverForSession` receives a title. To go from UUID → title, use `store.ListInstanceData()` (not `store.GetAll()` which does not exist on `InstanceStore`).
4. **`CreateDirectorySession` must call `StartController()`** (Epic 0) — without this, `GetController()` returns `nil` and the AutonomousDriver fails silently. This is the top-priority prerequisite for Epics 2 and 3.
5. **`Stop()` fires `fireCompletion(Stuck=true)` unconditionally** — do NOT call `StopDriverForSession` from `submit_triage_result`. The LLM-based DONE detection handles triage completion cleanly. Only use `StopDriverForSession` from `submit_review_verdict` (safe because review role skips all status transitions in Epic 4).
6. **Stuck triage → NO transition** — Epic 4 must gate the `triage→ready` transition on `outcome.Done == true`. A stuck/timed-out triage session leaves the item at `idea` so the operator can re-trigger. Never advance to `ready` when triage was interrupted.
7. **EventsSince has NO filters at all** — the replay path (line 1636) applies zero filters. Add the hidden check there too (Task 1.1c).
8. **AutonomousDriverStarter interface widened** — any test mocks that implement `AutonomousDriverStarter` must also implement `StartAutonomousDriverWithTimeout`. Add a compile-time assertion `var _ AutonomousDriverStarter = (*SessionService)(nil)` next to the interface definition.
9. **`TransitionBacklogItemStatus` precondition parameter** — verify exact type name from `backlog_lifecycle.go:199–203` before implementing Task 4.1a. Use `ExpectedStatus` preconditions to prevent stale driver from corrupting item state.
10. **`StartController()` inside `statusManager != nil` guard** (Task 0.1a) — if `s.statusManager == nil` in a test server, the controller is never started and the autonomous driver still fails silently. Task 0.1b must wire a non-nil `statusManager` to actually exercise the fix. Verify whether `StartController()` is safe to call without a status manager; if so, move it outside the guard.
11. **`maxTurns=0` semantics** — `NewAutonomousDriver` currently treats `maxTurns <= 0` as "use default 20" (verify in `autonomous_driver.go` constructor). The loop `for turnCount := 0; turnCount < d.maxTurns` would execute 0 times if `maxTurns` were 0. Confirm the default-mapping logic exists before relying on it; add a comment in `StartAutonomousDriverWithTimeout`.
12. **`StartAutonomousDriverWithTimeout` callback duplication** — extract a private `buildTurnCallback(inst)` helper shared by both `StartAutonomousDriverForInstance` and `StartAutonomousDriverWithTimeout` to prevent divergence over time.
