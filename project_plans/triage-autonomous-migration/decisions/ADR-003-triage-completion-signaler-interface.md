# ADR-003: TriageCompletionSignaler narrow interface for MCP→SessionService stop signal

**Date**: 2026-06-15
**Status**: Accepted

## Context

Once triage sessions use AutonomousDriver, there is a timing problem: the agent calls `submit_triage_result`, which persists the result and fires a notification. However, the AutonomousDriver does not know the MCP tool was called — it will continue its orchestration loop, potentially injecting additional turns after triage is complete.

Three options were considered:

**Option A**: AutonomousDriver detects completion from the terminal tail (`DONE:` prefix). This already exists but fires asynchronously after the LLM writes `DONE:` to the terminal.

**Option B**: `submit_triage_result` MCP handler directly calls a method on `SessionService` via a narrow interface to stop the driver immediately. Belt-and-suspenders alongside Option A.

**Option C**: `ItemSession` close event triggers driver stop. Indirect; harder to test.

## Decision

Both Option A and Option B are used (belt-and-suspenders):
- Option A remains: AutonomousDriver LLM detects `DONE:` from terminal tail as before.
- Option B is added: a new narrow interface `TriageCompletionSignaler` with `StopDriverForSession(sessionTitle string)` is defined in `server/mcp/`. `SessionService` implements it by delegating to the existing `stopAndDeregisterDriver`. `backlogHandlers` gains an optional `driverStopper` field. `submitTriageResult` calls `h.driverStopper.StopDriverForSession(inst.Title)` after successful save.

Whichever signal fires first stops the driver (both are idempotent: `AutonomousDriver.Stop()` uses `cancel()` which is safe to call multiple times).

### Role-aware status transition

`onAutonomousDriverComplete` previously always transitioned to `BacklogStatusReview`. With triage sessions now driving the AutonomousDriver, a triage completion must transition to `BacklogStatusReady` (not `review`) so the operator sees the triage output before a work session is spawned.

- `SessionRoleTriage` → `BacklogStatusReady`
- `SessionRoleWork` → `BacklogStatusReview`
- `SessionRoleReview` → no transition (review outcomes are managed by `submit_review_verdict`)

## Consequences

- `AutonomousDriverStarter` interface in `backlog_service.go` is widened to add `StartAutonomousDriverWithTimeout`. All mock implementations must be updated.
- `server/mcp.NewCore` must pass `svc *services.SessionService` to `backlogHandlers.driverStopper`. `svc` is already a parameter of `NewCore`.
- The `TriageCompletionSignaler` interface is minimal (one method) to avoid coupling the MCP layer to `SessionService` directly. Tests can substitute a mock.
- If the `InstanceStore` does not expose a `FindByUUID` method, add it or use an alternative lookup strategy before coding Task 5.1d.
