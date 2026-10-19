# Research: Inspect and Nudge Before Restarting

## Entry Points to Modify

### 1. `RemediateStaleWorkSession` — `server/services/backlog_service_triage.go:2182`

Current flow (simplified):
```
LoadItem → ListItemSessions → find open work session
  → if IsRetryPending: return nil (skip)
  → UpdateItemSessionEnded(id, now)  ← BUG-064 ordering: must precede kill
  → KillTmuxPaneOnly(inst)
  → AutoRespawnAutonomousWork(ctx, item)
```

Missing: no snapshot, no classify, no nudge, no stop_reason.

The function has access to `inst *session.Instance` via the session summary's UUID. `AutonomousOrchestrationService` (which contains `RemediateStaleWorkSession`) also owns `a.nudger()` (the `StallNudger`), `a.recoveries` map (for the guard), and `a.beginRecovery`.

### 2. `handleDeadPane` / `respawnDeadPane` — `session/health.go:408, 511`

Current flow:
```
handleDeadPane(instance, exitCode, exitSignal, result):
  → recoveryDebounced (threshold guard)
  → if exitCode==0 && signal=="" → MarkExitedNormally, return
  → if within restartGracePeriod → respawnDeadPane
  → else → handleCrashedPane → respawnDeadPane

respawnDeadPane(instance):
  → KillSession
  → Start(false)
```

`handleCrashedPane` captures `lastPaneOutputSnippet` (a plain string from `instance.LastPaneOutputSnippet()`) but does not produce a `StopSnapshot` or write to a stuck row. The health checker (`SessionHealthChecker`) is in `session/`, not `server/services/`, so it does not have direct access to `AutonomousOrchestrationService`'s nudger — it would need the nudger injected or would need to call a method on a shared interface.

**Key constraint:** Dead panes must never be nudged — the pane is gone, so there is nothing to receive a nudge. The classify/decide machinery still runs (to produce an `ObservedState` for the log), but `DecideStallAction` on a snapshot with `PaneDead=true` will always return `respawn` (row 1/6 of the decision table), so the logic is safe either way — but the dead-pane path should short-circuit to `stop_reason=dead_pane` + snapshot capture + respawn without going through `StallNudger`.

---

## Existing Infrastructure (from PR #1020)

### `domain.StopSnapshot` (`session/domain/autonomous_stop.go:134`)
```go
type StopSnapshot struct {
    CapturedAt, LastActivityAt time.Time
    PaneTail         string   // BoundedStopTail output
    Bucket           StatusBucket
    DetectedStatus   string
    DetectedContext  string
    ControllerActive bool
    TmuxAlive        bool
    PaneDead         bool
    ExitCode         int
    CaptureErr       string
}
```

### `domain.ClassifyObserved(snapshot StopSnapshot) ObservedState`
Pure function; returns one of: `ObservedIdlePrompt`, `ObservedNeedsApproval`, `ObservedIdle`, `ObservedWorking`, `ObservedError`, `ObservedExited`, `ObservedCrashed`, `ObservedUnknown`.

### `domain.DecideStallAction(input StallDecisionInput) StallAction`
8-row decision table (first match wins):
1. not a stall → `none`
2. archived → `none`
3. retry pending → `none`
4. `max_turns` or `panic` → `respawn`
5. working + item in_work + resupervise allowed → `resupervise`
6. working → `respawn`
7. idle_prompt + item in_work + nudge enabled + nudge allowed → `nudge`
8. default → `respawn`

### `BoundedStopTail(raw string) string` (`session/autonomous_stop_snapshot.go:25`)
Strips ANSI, normalizes UTF-8, removes `\r`, trims trailing blanks, keeps last 40 lines and at most 4096 bytes.

### `sanitizeStopTail(tail string) string` (`server/services/autonomous_stop_detail.go:37`)
Service-side: withholds tail if secret patterns fire; otherwise redacts error detail, folds to one line, caps at 1024 bytes.

### `StallNudger` (`server/services/autonomous_stall_recovery.go:24`)
```go
type StallNudger interface {
    NudgeStalledSession(ctx context.Context, inst *session.Instance, claim func() (bool, error)) error
}
```
Concrete: calls `session.NudgeStalledAutonomousSession`.

### `nudgeStalledWorkSession` (`server/services/autonomous_stall_recovery.go:186`)
Existing method on `AutonomousOrchestrationService`. Checks `canRestart`, calls `nudger.NudgeStalledSession(rctx, inst, claim)`, on success calls `reattachDriver` with 5-minute timeout, clears `AutonomousStopReason`. On failure, signals `leave` or `respawn`.

### `stopRecord` + `formatStopDetail` (`server/services/autonomous_stop_detail.go`)
Stuck-row format:
```
<plain summary> | stop_reason=<code> state=<observed_state> action=<stall_action> [nudge=N/cap] turns=N resupervise=N/cap last_activity=<rfc3339> nudge_outcome=<outcome> tail=<sanitized tail>
```
Written via `storage.MarkStuck(ctx, itemID, domain.StuckReasonAutonomousStuck, status, formatStopDetail(rec))`.

### Recovery guard pattern (`autonomous_stall_recovery.go:72–100`)
`beginRecovery(title)` inserts a `*stallRecovery` into `a.recoveries[title]` and returns a child context + cleanup func. `cancelRecovery(title)` cancels any live recovery. `RemediateStaleWorkSession` should check `a.recoveries[title] != nil` (recovery live) and skip/defer if so, just as `nudgeStalledWorkSession`'s `canRestart` path does.

---

## New StopReason Values Needed

The `AllStopReasons` slice in `session/domain/autonomous_stop.go` currently has 10 entries. Two new ones are needed:

```go
StopReasonStaleWork  StopReason = "stale_work"   // 2h idle with no progress
StopReasonDeadPane   StopReason = "dead_pane"    // tmux pane exited unexpectedly
```

Both will have `Class() == OutcomeClassStall` (neither is `done` nor `cancelled_*`), so they will flow correctly through any stall-detection logic that checks `Class() != OutcomeClassDone`.

---

## Architectural Challenge: health.go is in `session/` Package

`RemediateStaleWorkSession` is a method on `AutonomousOrchestrationService` in `server/services/` — it has full access to `StallNudger`, `sanitizeStopTail`, `stopRecord`/`formatStopDetail`, and `storage.MarkStuck`. 

`handleDeadPane` is in `session/health.go` — it does **not** have access to any of these service-layer types. Options:

**Option A (Preferred):** Don't nudge from the health checker. Dead-pane events should never nudge (pane is gone). For non-crash exits (e.g. `exitCode=0`) that are actually stall candidates, the stale-work detector will catch them on its next 2h tick. `handleDeadPane` only needs to capture a `BoundedStopTail` snapshot (available in `session/` package) and emit a structured log with `stop_reason=dead_pane`. The service-layer stuck-row update happens when `AutoRespawnAutonomousWork` runs after `respawnDeadPane` calls `Start(false)`.

**Option B:** Inject a callback/interface into `SessionHealthChecker` so it can call back into `AutonomousOrchestrationService` for nudge decisions. Higher coupling, more testing surface.

Option A is consistent with the out-of-scope boundary ("dead pane is never nudged") and avoids cross-layer coupling.

---

## Interaction with BUG-064 Ordering

`RemediateStaleWorkSession` has an explicit comment at line 2163:
> BUG-064: UpdateItemSessionEnded must be called BEFORE KillTmuxPaneOnly

A snapshot must be captured **before** `UpdateItemSessionEnded` (i.e., before we mark the session ended), so the tail reflects the live session state. The ordering must be:

1. Capture `StopSnapshot` (pane still alive)
2. `UpdateItemSessionEnded` (mark ended — BUG-064 gate passes)
3. `KillTmuxPaneOnly`
4. Nudge OR `AutoRespawnAutonomousWork` depending on `DecideStallAction`

---

## Test Surface

Key test scenarios (table-driven, `ObservedState × path`):

| Scenario | Path | Expected action |
|---|---|---|
| pane idle-prompt, nudge allowed | stale-work | nudge, no respawn |
| pane idle-prompt, nudge exhausted | stale-work | respawn |
| pane working | stale-work | respawn |
| pane dead | health | respawn (stop_reason=dead_pane) |
| pane idle-prompt, live recovery in recoveries map | stale-work | skip (guard fires) |
| pane exited normally (exitCode=0, no signal) | health | MarkExitedNormally (existing behavior, no change) |

These should use the existing `stopRecord`/`nudgeStalledWorkSession` test patterns from `server/services/autonomous_stop_test.go`.
