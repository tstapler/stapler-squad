# Requirements: Inspect and Nudge Before Restarting from Stale-Work Detector and Health Checker

**Item ID:** aea335bb-bdc5-4aae-ad64-a6907dd648a6
**Source:** autonomous-stall-diagnosis (AC3 scope decision), Deferred Adv C1 — filed after PR #1020 (a895185ca) merged

---

## Problem Statement

Two automated restart paths kill sessions without inspecting what the session was doing:

1. **`RemediateStaleWorkSession`** (`server/services/backlog_service_triage.go:2182`) — triggered by the 2-hour stale-work detector. Calls `UpdateItemSessionEnded` then `KillTmuxPaneOnly` then `AutoRespawnAutonomousWork` with no snapshot, no classification, no nudge, and no `stop_reason`.

2. **`handleDeadPane` / `respawnDeadPane`** (`session/health.go:408, 511`) — triggered by the session health checker when a tmux pane exits unexpectedly. Calls `KillSession` then `Start(false)` with no snapshot, no classification, and no `stop_reason`. `handleCrashedPane` does capture `lastPaneOutputSnippet` (a local string) but does not produce a `StopSnapshot` or record a typed stop reason.

The consequence: an idle-at-prompt session (agent finished its turn and is waiting) is killed and cold-respawned after 2 hours without any opportunity for the agent to voluntarily complete. Operators see no record of why the session was restarted.

---

## Goal

Both restart paths should:

1. Capture a `StopSnapshot` (`BoundedStopTail` + `sanitizeStopTail`) **before** ending the ItemSession or killing the pane, so the tail reflects what the session was doing at the moment of restart.
2. Pass the snapshot through `domain.ClassifyObserved` to get an `ObservedState`.
3. Feed `ObservedState` through `domain.DecideStallAction` to decide whether to nudge, respawn, or resupervise — instead of unconditionally respawning.
4. For the stale-work path: if `StallAction == nudge`, send one nudge via `StallNudger.NudgeStalledSession` and let the driver continue without respawning.
5. Record a typed `stop_reason` (`stale_work` or `dead_pane`) with a plain-language lead sentence in the stuck-row context, matching the format of `formatStopDetail`.

---

## Scope

### In scope

- Add `StopReasonStaleWork = "stale_work"` and `StopReasonDeadPane = "dead_pane"` to `AllStopReasons` in `session/domain/autonomous_stop.go`.
- Plumb `BoundedStopTail` + `sanitizeStopTail` capture into `RemediateStaleWorkSession`.
- Plumb `BoundedStopTail` capture into `handleDeadPane` / `handleCrashedPane` (dead-pane path uses `StopReasonDeadPane`; sanitize is optional given pane is already dead, but capture for the log).
- Wire `domain.ClassifyObserved` → `domain.DecideStallAction` in `RemediateStaleWorkSession`, branching on result: nudge (via existing `StallNudger`) vs respawn.
- Record `stop_reason` + snapshot tail in the stuck row context on both paths.
- Keep the BUG-064 ordering invariant: `UpdateItemSessionEnded` must still precede `KillTmuxPaneOnly`.
- Table tests (`ObservedState × path` at minimum) covering: idle-prompt + nudge allowed → nudge, idle-prompt + nudge exhausted → respawn, working → respawn, dead-pane → never nudge.
- Ensure `cancelRecovery` (the `beginRecovery`/`recoveries`-map guard) defers `RemediateStaleWorkSession` when a driver-stop recovery is already live for that session.

### Out of scope

- Changing the 2-hour stale threshold.
- Pause/stop driving the driver (Deferred Adv C1 repair-2).
- Changing `handleCrashedPane`'s restart semantics (crash restarts are not stall-type events).
- Adding `stallRecoveryGuard` as a named interface (the `cancelRecovery` pattern is sufficient).

---

## Acceptance Criteria

| # | Criterion | Status |
|---|-----------|--------|
| AC1 | A stale item whose pane is at an idle prompt **and** has nudge allowance left receives exactly one nudge (via `StallNudger`) and is **not** respawned; the stuck-row context is updated with `stop_reason=stale_work state=idle_prompt action=nudge`. | pending |
| AC2 | A dead pane is **never** nudged regardless of its last observed state; it is always respawned (or cold-restored via `Start(false)`) with `stop_reason=dead_pane` in the stuck row. | pending |
| AC3 | A `StopSnapshot` (containing `BoundedStopTail`-processed pane tail) is captured and logged via `logStop` / structured log before every restart or nudge on both the stale-work path and the dead-pane path. | pending |
| AC4 | Table-driven unit tests cover all `ObservedState` variants × both entry points (`RemediateStaleWorkSession` and `handleDeadPane`), verifying nudge vs. respawn outcome per row. | pending |
| AC5 | When `cancelRecovery` (the `recoveries`-map guard) is live for a session, `RemediateStaleWorkSession` defers its restart/nudge for that session (consistent with the existing stall-recovery guard behavior in `AutonomousOrchestrationService`). | pending |

---

## Dependencies

- PR #1020 (a895185ca) merged — provides `domain.StopSnapshot`, `domain.ClassifyObserved`, `domain.DecideStallAction`, `BoundedStopTail`, `StallNudger`, `NudgeStalledAutonomousSession`, and `sanitizeStopTail`.
- The nudge-resolution rate soak from the autonomous-stall-diagnosis rollout (gating signal, not a code dependency).

---

## Size Estimate

M–L: two entry points with distinct orchestration contexts, one shared effect (snapshot + classify + action). Estimated 3–5 days of focused implementation and test authoring.
