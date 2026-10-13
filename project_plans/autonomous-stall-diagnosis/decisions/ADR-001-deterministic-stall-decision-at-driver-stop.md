# ADR-001: Deterministic nudge/respawn/none decision, fed by a snapshot captured inside the driver

**Status**: Accepted (plan phase)
**Date**: 2026-10-13
**Feature**: autonomous-stall-diagnosis
**Deciders**: planning phase (SDD Phase 3)

## Context

Every non-DONE exit of `AutonomousDriver.run` reaches `onAutonomousDriverComplete` as the same
`Stuck:true` outcome. For a backlog work session the service then ends the `ItemSession` and
dispatches `AutoRespawnAutonomousWork` without looking at what the pane was doing. The pane is
untouched by a driver stop (it keeps its tmux session), so the recovery step is what turns "the
driver gave up" into "the session was killed" — including for a session that is merely idle at a
prompt, or still working.

We need (a) the pane's last activity/output captured before anything is ended or killed, and (b)
a recovery choice that can prefer a nudge over a respawn when the session is only waiting.

## Options considered

### A. Deterministic in-process decision at driver stop (CHOSEN)
Driver captures a `StopSnapshot` in a single `finish()`; pure `ClassifyObserved` +
`DecideStallAction` map (reason, state, allowance, flags) to {Nudge, Respawn, None}; the service
acts on the result.
- **Strength**: synchronous (pane inspected before `UpdateItemSessionEnded`/respawn), unit-testable
  per state, no new proto/ent surface, reuses the existing nudge cap and write guards.
- **Weakness**: a fixed nudge text cannot adapt to *why* the agent is idle; richer recovery is
  deferred to the existing escalation (diagnose dispatch) once the cap is exhausted.

### B. Route every stall through `DispatchDiagnose` (LLM agent)
- **Strength**: flexible, can read the transcript and choose a bespoke action.
- **Weakness**: slow and non-deterministic on the hot path, needs a hidden session, races the
  respawn teardown, and cannot be tested deterministically per state. Kept as the escalation
  when the nudge cap is exhausted (already how `diagnose_nudge_session` is reached).

### C. Add per-reason values to `domain.StuckReason`
- **Strength**: durable, filterable per-reason rows in the Unfinished tab.
- **Weakness**: `StuckReason` is an item-level enum mapped through ent, proto and TS (churn in
  all three); it fragments the per-(item, reason) nudge budget and conflates a driver exit with
  item blockage. Rejected; the typed reason lives on the single `autonomous_stuck` row's context.

### D. Capture the snapshot in the service callback instead of the driver
- The completion callback already runs synchronously on the driver goroutine
  (`fireCompletion`, `session/autonomous_driver.go:516`), so a service-side capture would observe
  the same instant as a `finish()` capture; the race is not a reason. Rejected because the driver
  owns the exit-site knowledge (which `break` fired, `stopRequested`) and a `stopSource` fake makes
  the capture unit-testable without the service. Capture stays in the driver.

## Decision

Option A. Capture in the driver; decide in a pure domain function; act in the service behind a
seam (`StallNudger`). Four actions: `None`, `Nudge`, `Resupervise`, `Respawn`.

`DecideStallAction` (first match wins):

| # | Condition | Action |
|---|-----------|--------|
| 1 | outcome class is not `Stall` (`done`, `cancelled_external`, `cancelled_shutdown`) | None (no stuck row, no notification; for shutdown see "Shutdown" below) |
| 2 | session archived | None |
| 3 | an automated retry is pending/in flight for the session | None |
| 4 | reason is `max_turns` | Respawn (today's behaviour: the rework cap and `RemediationDue` backoff keep gating turn-budget exhaustion; a nudge or re-supervise would hand out a fresh budget around them) |
| 5 | observed state is `working` AND re-supervise headroom left | Resupervise |
| 6 | observed state is `working` AND re-supervise headroom exhausted | Respawn (terminal fallthrough = today's behaviour; the item is never left driverless) |
| 7 | state is `idle_prompt` AND nudge kill switch on AND allowance ok | Nudge |
| 8 | otherwise (needs_approval, error, exited, crashed, idle, unknown) | Respawn |

The decision is consulted only for a backlog-linked `SessionRoleWork` stall (`handleWorkStall`).
Review, triage, unrecognized-role and unlinked sessions keep today's handling, so the fixed nudge
text ("call request_review") is always role-correct. Instance status is not a table input:
`canRestart` and the nudge writer refuse a non-`Active` instance, and pause/user-stop never stop the
driver (accepted limitation, plan Deferred Adv C1).

Recovery effects:

- **Resupervise**: start a fresh driver (`startDriverIfAbsent`, an error-returning check-and-register
  under one lock, 5 min warm-up) on the same live session; a start error falls back to Respawn so
  mode is never left true with no driver. `AutonomousMode` stays true throughout (the approval gate at
  `server/server.go:889-891` keys on it), no `MarkStuck`, no `UpdateItemSessionEnded`, no nudge
  slot consumed, no FAILURE notification (Warn log line only). Bounded by an in-memory per-
  `ItemSession` counter (default 3); when exhausted row 6 sends the session to Respawn, so a
  looping stop cannot run forever and cannot strand the item.
- **Nudge**: as before, plus: failures map explicitly (see plan Story 3.2.3 failure table); there is
  no slot refund (a failed `SubmitDriverContent`, including `ErrSubmitNotConfirmed`, may have
  delivered the text or left it unsubmitted in the input box, so it always maps to leave-the-pane:
  never Respawn, and never Resupervise, whose first turn would append to the pending input; a
  cancelled recovery ctx takes precedence over this row);
  the settle wait honours the recovery ctx; `AutonomousMode` stays true through the nudge window.
- **Shutdown**: with flag `autonomous_driver_readoption` ON, `cancelled_shutdown` does not clear
  `AutonomousMode`, writes no `autonomous_stuck` row and publishes no FAILURE notification;
  persisted `AutonomousMode` is the re-adoption *candidate* signal, never proof. Nothing re-adopts
  drivers at boot today (VERIFIED: every non-test `StartAutonomousDriver*` caller is spawn
  `backlog_service_triage.go:1134,3341` or the toggle `session_service_update.go:294`), so the plan
  adds a boot pass that adopts a candidate only when a twelve-gate conjunction holds (status
  `Active`, not archived/paused/retry-pending, controller present, no registered driver, open
  ItemSession whose item status matches the role, live and owned pane, no Step 7.5 prompt driver)
  and clears the ghost mode (outcome `""`, no badge) only of a *definitively* rejected candidate
  (plan Story 3.1.3: gate results are reject / skip / unknown; a registered driver is a skip and a
  transient probe or storage error is unknown, both leave the mode untouched). The pass is
  triggered after Step 7.6's controller settle, with a bounded fallback, and defers (not clears)
  candidates over the per-boot cap. With the flag OFF the shutdown branch calls
  `SetAutonomousStopped` as the callback does today for the in-memory mode/outcome (it does not
  restore today's stuck row, respawn or FAILURE notification, which are ungated corrections) and
  the boot pass is a dry-run that clears stale mode. The flag default is one constant, an open
  owner decision.
- **Stop during recovery**: the recovery is registered in a cancellable map keyed by session title
  inside the same critical section that deregisters the finished driver (`claimCompletion`), before
  the item lookup or any DB write, so a stop at any later point finds it; every
  `stopAndDeregisterDriver` call cancels it, and `canRestart` re-checks
  ctx/runID/archived/paused/`Status == Active`/controller/no-successor immediately before the nudger
  and again before the restart.

Rows 4-6 are the conservative reading of precedent (`RemediateStaleWorkSession` defers on
`IsRetryPending`; pitfalls P4 forbids acting on active states). Row 5 replaces the earlier draft's
"working -> None": that mapping left a driverless session whose approvals stopped being handled
(adversarial review B1).

**Budget split.** The in-process nudge and the LLM diagnose agent share the `(item,
autonomous_stuck)` counter and 1h cooldown. The in-process nudge may use only `cap - 1` slots
(`reserve = 1`), so after the deterministic nudges the escalation agent still has one nudge in its
action space. With the default cap 3 that is 2 deterministic nudges. A configured cap of 1 means
no deterministic nudges.

## Consequences

- Respawn remains the default for every state the nudge cannot safely handle; the only recoveries
  removed are rows 1-3 (and the working-pane respawn, which becomes Resupervise-then-Respawn).
- The nudge path never calls `UpdateItemSessionEnded` or `RemediationDue`; it restarts a driver.
- `StopSnapshot` carries raw pane text in memory only; redaction happens at the one sink (ADR-002).
- Known weakness, accepted: with `orchestrator_error` caused by a failing LLM pool, a nudge is
  wasted and the re-supervised driver fails again; bounded by the nudge cap (2) and the
  re-supervise cap (3), worst case about 5 extra driver runs per `ItemSession` before Respawn.
  Each re-supervised run gets a fresh turn budget, so a working session can use up to (1 + 3) x
  `AutonomousMaxTurnsOrDefault`; `max_turns` itself never nudges or re-supervises (row 4), and every
  stop logs `resupervise=n/cap` so the chain is traceable.
