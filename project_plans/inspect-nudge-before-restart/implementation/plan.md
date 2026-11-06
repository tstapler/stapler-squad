# Implementation Plan: Inspect and Nudge Before Restarting

## Overview

Add snapshot capture + classify/decide/nudge to the two automated restart paths:
1. `RemediateStaleWorkSession` (stale-work detector, `server/services/backlog_service_triage.go`)
2. `handleDeadPane` / `respawnDeadPane` (health checker, `session/health.go`)

The implementation reuses all domain infrastructure shipped in PR #1020 (a895185ca).

---

## Task Breakdown

### T1 — Add new StopReason values (30m)

**File:** `session/domain/autonomous_stop.go`

Add to the `const` block and `AllStopReasons` slice:
```go
StopReasonStaleWork StopReason = "stale_work"
StopReasonDeadPane  StopReason = "dead_pane"
```

Both have `Class() == OutcomeClassStall` by the existing switch default. Verify with `go test ./session/domain/...`.

---

### T2 — Plumb snapshot capture into `RemediateStaleWorkSession` (2–3h)

**File:** `server/services/backlog_service_triage.go`

Precondition: `inst` is already resolved (the function already has the instance).

New flow:
```
1. Check recovery guard: if a.recoveries[inst.Title()] != nil → log + return nil
2. CaptureStopSnapshot(ctx, inst) → StopSnapshot  [new helper or reuse existing]
3. state := domain.ClassifyObserved(snap)
4. action := domain.DecideStallAction(StallDecisionInput{...})
5. if action == domain.StallActionNudge:
     a. UpdateItemSessionEnded(id, now) is NOT called yet (session stays open)
     b. Call nudgeStalledWorkSession path (or StallNudger.NudgeStalledSession directly)
     c. Record stop_reason=stale_work + state + action in stuck row via MarkStuck
     d. return nil (no respawn)
6. else (respawn/none):
     a. UpdateItemSessionEnded(id, now)  ← BUG-064 ordering preserved
     b. KillTmuxPaneOnly(inst)
     c. Record stop_reason=stale_work + state + action in stuck row
     d. AutoRespawnAutonomousWork(ctx, item)
```

**Snapshot capture:** Reuse the snapshot-capture pattern from `onAutonomousDriverComplete`. Need either:
- A shared helper `captureStopSnapshot(ctx, inst) (domain.StopSnapshot, error)` in `server/services/`, or
- Inline construction mirroring `outcome.Snapshot` structure from the driver path

The `AutonomousOrchestrationService` likely has access to the instance's last scrollback/pane tail via `inst.LastPaneOutputSnippet()` or a new `session.CaptureStopSnapshot()` call. Check what `outcome.Snapshot` is built from in the driver path to reuse the same constructor.

**`DecideStallAction` input fields needed:**
- `Snapshot`: the captured snapshot
- `StopReason`: `StopReasonStaleWork`
- `IsArchived`: item is not archived (already checked)
- `IsRetryPending`: already checked earlier in the function
- `ItemInWork`: item status is `in_progress`
- `NudgeEnabled`: feature flag (reuse existing `nudgeStalledWorkSession` flag check)
- `NudgeAllowance`: remaining nudge budget (from `item.BacklogItemData` or stuck row context)
- `ResuperviseAllowance`: from the same source

---

### T3 — Plumb snapshot + stop_reason into dead-pane path (1–2h)

**File:** `session/health.go`

The dead-pane path must never nudge (pane is gone). Add only:

1. Before `respawnDeadPane`: call `instance.BoundedStopTail()` (or `instance.LastPaneOutputSnippet()`) to get raw tail, call `domain.BoundedStopTail(raw)` to produce bounded tail.
2. Build a minimal `domain.StopSnapshot{PaneDead: true, PaneTail: boundedTail, ExitCode: exitCode}`.
3. Call `domain.ClassifyObserved(snap)` — will return `ObservedCrashed` or `ObservedExited`.
4. Structured-log the snapshot (stop_reason, state) at INFO level before respawn.
5. **Do not** call `MarkStuck` from `session/health.go` — that is a service-layer concern. The stuck-row update for `dead_pane` will flow through `AutoRespawnAutonomousWork` (which already runs after `Start(false)` triggers a new session creation and the orchestration service re-picks it up).

If the stuck-row context needs `stop_reason=dead_pane` explicitly before `AutoRespawnAutonomousWork`, that can be written by a callback or by `RemediateStaleWorkSession` on its next invocation detecting the respawned session. For now, the logging alone satisfies AC3; the stuck-row annotation for dead-pane can be revisited if needed.

---

### T4 — `formatStopDetail` extension for new stop reasons (30m)

**File:** `server/services/autonomous_stop_detail.go`

`stopReasonCode` (used in `formatStopDetail`) maps `StopReason` to its string form. Since `StopReason` is already a string type, no change to `stopReasonCode` itself — just ensure `stale_work` and `dead_pane` are handled correctly (they will be, since the function passes the code through directly).

Verify `formatStopDetail` output for a `stopRecord{reason: StopReasonStaleWork, ...}` produces the expected lead sentence. The lead sentence comes from `stopSummary(rec)` — check that it handles the two new values gracefully (likely falls through to the generic stall summary).

---

### T5 — Table-driven tests (3–4h)

**New test file:** `server/services/backlog_service_triage_nudge_test.go`

Table columns: `name`, `observedState`, `nudgeAllowance`, `recoveryLive`, `expectedAction`, `expectNudgeCalled`, `expectRespawnCalled`, `expectedStopReason`.

Rows to cover (minimum):
1. `idle_prompt + nudge_allowed → nudge, no respawn`
2. `idle_prompt + nudge_exhausted → respawn`
3. `idle_prompt + recovery_live → skip (no action)`
4. `working → respawn`
5. `error → respawn`
6. `unknown → respawn`

**New test additions to health check tests:** `session/health_test.go` or `session/health_deadpane_test.go`

Table rows:
1. `dead pane, exit code 1 → respawn, never nudge, stop_reason=dead_pane logged`
2. `dead pane, exit code 0 + no signal → MarkExitedNormally (existing behavior, no change)`

Use mock `StallNudger` that records call count. Inject via `SetStallNudger` (already exists on `AutonomousOrchestrationService`).

---

### T6 — Integration: wire recovery guard check into `RemediateStaleWorkSession` (30m)

**File:** `server/services/backlog_service_triage.go`

Add check at top of `RemediateStaleWorkSession`:
```go
if _, live := a.recoveries[instTitle]; live {
    log.Info("RemediateStaleWorkSession: skipping — stall recovery live", "session", instTitle)
    return nil
}
```

This mirrors the `canRestart` check in `nudgeStalledWorkSession`. Requires access to `a.recoveries` (already accessible since the method is on `AutonomousOrchestrationService` or its equivalent struct — verify the receiver type).

---

## Sequence Diagram (stale-work path after change)

```
2h tick → RemediateStaleWorkSession(ctx, itemID)
  → check recovery guard → live? skip
  → CaptureStopSnapshot(ctx, inst)
  → ClassifyObserved(snap) → state
  → DecideStallAction(...) → action
  → action == nudge?
      YES → NudgeStalledSession(ctx, inst, claimFn)
            → MarkStuck (stop_reason=stale_work, action=nudge)
            → return nil
      NO  → UpdateItemSessionEnded(id, now)  [BUG-064 gate]
            → KillTmuxPaneOnly(inst)
            → MarkStuck (stop_reason=stale_work, action=respawn)
            → AutoRespawnAutonomousWork(ctx, item)
```

---

## Risk Flags

1. **Snapshot capture availability:** The stale-work path needs to call something to get the current pane tail. Verify `instance.LastPaneOutputSnippet()` or equivalent exists and returns a usable string. If the session's pane is already quiescent (idle), the tail will be valid. If the pane capture fails (tmux not responding), the snapshot should set `CaptureErr` and continue — don't block the respawn.

2. **NudgeAllowance source:** `DecideStallAction` needs the current nudge allowance count. The stale-work path does not currently read this from the item — it needs to be fetched from the stuck row or from `item.BacklogItemData` (wherever the nudge counter is stored post-PR #1020). Confirm the field name and location.

3. **`RemediateStaleWorkSession` receiver:** Confirm the function is a method on `AutonomousOrchestrationService` (which has `a.recoveries`). If it's on a separate `BacklogService` struct, the recovery guard is not directly accessible and needs injection.

4. **Stuck row for dead-pane via health checker:** The health checker is in `session/`, far from `storage.MarkStuck`. For MVP, log the stop_reason via slog and let `AutoRespawnAutonomousWork` handle the stuck row update. A follow-up can wire a callback for explicit `dead_pane` stuck-row annotation if operators need it.
