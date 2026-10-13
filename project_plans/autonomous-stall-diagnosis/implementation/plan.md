# Implementation Plan: autonomous-stall-diagnosis

**Feature**: Diagnose why autonomous-mode backlog sessions stop early, record a typed stop reason for every stop, inspect the pane before any restart, re-supervise a still-working session, and prefer a bounded nudge over a respawn when the session is only waiting.
**Date**: 2026-10-13
**Status**: Ready for implementation (repair iteration 3: adversarial round-3 concerns C1-C3, C5-C7 folded in; boot re-adoption gate results are classified reject / skip / unknown; the one owner decision, the re-adoption default, is a single constant, see Deferred / accepted concerns; repair iteration 4: pre-mortem P1 #1-#2 and P2 #3-#11 folded into existing tasks' ACs, no task added, see "Pre-mortem disposition"; repair iteration 5: triad gaps UX G1-G6, engineering G1-G8, product items folded in, three tasks added (3.1.3e, 3.1.3f, 3.1.4d), see "Triad repair disposition")
**ADRs**: [ADR-001](../decisions/ADR-001-deterministic-stall-decision-at-driver-stop.md), [ADR-002](../decisions/ADR-002-stop-reason-exposure-without-wire-change.md)
**Complexity**: 2 (backlog item `4a709ebc-51de-4e6a-8f1d-150cde3925bc`; no schema or proto change; one CSS-only web-app change, Task 3.1.4d)
**Base commit**: `6a395b58f`. Every `file:line` below was re-checked against this commit; implementers must re-check at their own HEAD (lines drift).

---

## Verified starting facts (spot-checked, not relayed)

| Fact | Evidence at `6a395b58f` |
|------|-------------------------|
| Every non-DONE driver exit except startup timeout is reported as "max turns reached" with `Turns = maxTurns` | `session/autonomous_driver.go:313-334, 366, 399-406, 433-439` |
| Startup timeout also fires on ctx cancel (Stop/shutdown) | `session/autonomous_driver.go:296-301` (`waitForIdle` returns false for both) |
| Panic recover logs and fires no completion; `AutonomousMode` stays true | `session/autonomous_driver.go:259-263` |
| `Stop()` is plain `cancel()`; nothing distinguishes external stop from shutdown | `session/autonomous_driver.go:222-230`; shutdown cancels `lifecycleCtx` (`server/server.go:1087`) |
| External `Stop()` callers: StopSession, DeleteSession, autonomous-toggle-off, `submit_review_verdict` | `server/services/session_service_lifecycle.go:47,306`, `session_service_update.go:296`, `server/mcp/tools_backlog.go:1998` (all via `stopAndDeregisterDriver`, `autonomous_orchestration_service.go:148-158`) |
| Service writes `inst.AutonomousMode/Outcome` raw; the actor setter `SetAutonomousComplete` exists but is test-only | `autonomous_orchestration_service.go:282-290`; `session/instance_actor_setters.go:159-180` |
| Work-role `!Done` arm ends the ItemSession, then `RemediationDue`, then respawns in a goroutine | `autonomous_orchestration_service.go:360-442` |
| Pane tail has no sanitizer on this path; existing redaction: `redactErrorDetail` (`server/services/backlog_service_triage.go:2748`), detect-only `RunPreGateSecurityCheck` (`session/backlog_review.go:50`) | read |
| `MarkStuck` on an open row refreshes only `last_checked_at`/`context`; on a resolved row reopens it. Neither resets `diagnose_nudge_count`; `ResetStuckRemediation` does not either | `session/ent_repository_backlog.go:2128-2196, 2403-2425` |
| `MarkStuck` returns `(false, nil)` on a status-precondition miss, and the service discards that bool | `ent_repository_backlog.go:2144-2147`; `autonomous_orchestration_service.go:329` |
| `RecordDiagnoseNudgeAttemptIfBelowCap` is an atomic `UPDATE ... WHERE count < cap`; `Storage.RecordDiagnoseNudgeAttempt` hides the "claimed" bool | `ent_repository_backlog.go:2349-2373`; `session/diagnose_nudge.go:99-119` |
| `diagnose_nudge_enabled` defaults off, is live-read, and is a registered flag | `config/config.go:540-557`; `server/services/feature_flag_service.go:278-281` |
| Nudge cap default 3 attempts, cooldown 1h; stale-work detector needs 2h without progress | `config/config.go:1134-1145` (default documented as 3); `diagnose_nudge.go:28`; `session/backlog_lifecycle_stale.go:62-64` |
| `terminal_write_pins_test.go` (tag `sinkguard`) pins every caller of the write primitives *and* the closed acquirer list, keyed by function name | `server/services/terminal_write_pins_test.go`, `terminal_write_callers_test.go:405-440`; run by `make test-delivery-guards` |
| Only `SessionCard.tsx:877,888` read `autonomousOutcome` in `web-app/src` (excluding `gen/`), both strict `=== "done"` / `=== "stuck"` | grep, 2026-10-13 |
| `AutonomousOutcome` is not persisted (absent from `session/storage.go`/ent schema); `AutonomousMode` **is** persisted (`session/storage.go:85`, `instance_serialization.go:161,358`) | grep, 2026-10-13 |
| Nothing re-adopts a driver at boot: every non-test `StartAutonomousDriver*` caller is spawn (`backlog_service_triage.go:1134`, `:3341`) or the toggle (`session_service_update.go:294`) | `git grep -n StartAutonomousDriver -- '*.go'`, 2026-10-13 |
| The approval path gates LLM auto-approval on `inst.AutonomousMode` | `server/server.go:889-891` |
| `onAutonomousDriverComplete` deregisters the finishing driver at entry (L260), so a later `StopDriverForSession` finds nothing to cancel during a nudge; pause does not call `stopAndDeregisterDriver` (callers: `session_service_lifecycle.go:47,306`, `session_service_update.go:296`, `tools_backlog.go:1998`) | read + `git grep stopAndDeregisterDriver` |
| Stale-work staleness is `LastProgressAt` else `CreatedAt` (`session/backlog_lifecycle_stale.go:100-108`); `RemediateStaleWorkSession` defers only on `IsRetryPending` and then ends the ItemSession, kills the pane and respawns (`backlog_service_triage.go:2182-2250`). A nudge does not touch progress time | read |
| The claim UPDATE is `WHERE count < cap` only; the 1h cooldown is checked by a separate earlier read | `ent_repository_backlog.go:2349-2373`, `diagnose_nudge.go:62-72` |
| `StuckBacklogItem.context` (proto field 9) is returned by `ListStuckBacklogItems` and rendered at `StuckItemDetail.tsx:102` | `proto/session/v1/backlog.proto:2049-2060` |
| Today a graceful shutdown is NOT silent: parent-ctx cancel makes the driver fire a completion (startup wait `autonomous_driver.go:296-301`, loop exit `:433-440`), and the callback then clears `AutonomousMode`/turns in memory, sets outcome `"stuck"`, writes an `autonomous_stuck` row, dispatches a respawn on the cancelled lifecycle ctx and publishes the FAILURE notification | read, `autonomous_orchestration_service.go:282-290,328-336,398-442,601-604` |
| `pause` and user `stop` never reach the driver: `applyStatusTransition` calls `instance.Pause()`/`StopByUser()` only; the three `Stop()` callers are listed above | read, `session_service_update.go:99-110` |
| `StartAutonomousDriverWithTimeout` / `StartAutonomousDriverForInstance` return nothing (pool nil and `driver.Start` error are log-only), and `registerDriver` overwrites `a.drivers[title]` unconditionally | read, `autonomous_orchestration_service.go:141-145,213-244` |
| `VerifyNudgeSafeToWrite` calls `CheckNudgeEligible`, which takes no ctx and `time.Sleep`s in a 3 s settle window (`nudgeIdleSettleWindow`); only the tmux ownership check honours ctx | read, `session/nudge_gate.go:31,42-44,62-83,125-130` |
| `SubmitDriverContent` checks `ctx.Err()` before the first `SendKeys` (so a cancelled ctx cannot start a write) but can fail after the text was typed (`ErrSubmitNotConfirmed`, "cancelled before sending Enter", Enter write error) | read, `session/pane_submit.go:100-142` |
| The stale detector calls `RemediationDue` (which consumes a `stale_work` attempt) in `remediateStaleWorkWithBackoffGate` *before* dispatching `RemediateStaleWorkSession` | read, `session/backlog_lifecycle_stale.go:292-320` |
| `Instance.Status` has `Creating/Active/Paused/Stopped/Hibernated/Restoring/Crashed/PermanentlyFailed/Failed`; `IsPaused()` is `Status == Paused` only; Step 7.5 `StartSessionDriver` runs in a goroutine after instances load and skips Paused/Stopped/Hibernated only | `session/instance.go:60-93`, `instance_state.go:150,165`, `server/dependencies.go:1062-1081` |
| The Review arm's non-Done `switch` is `case Done / case status != review (resolve) / default (UpdateItemSessionEnded)`; the Triage arm is `if !outcome.Done {notify; return}`; the Work arm's `else` is the DONE branch and calls `resolveAutonomousStuck` | read, `autonomous_orchestration_service.go:341-359,361,443-481,483-540` |
| `RemediationDue` records the attempt as it grants, and the detector calls it before the remediator | `session/backlog_remediation.go:277-320`, `session/backlog_lifecycle_stale.go:292-320` |
| `"diagnose_nudge_enabled": true` is set in the owner's workspace config (INFERRED to be the live instance's); the code default is off | `~/.stapler-squad/workspaces/d685c4b1a423cca3/config.json:96`; `config/config.go:547` (const), `:556` (default `false`) |
| A third driver start site exists: the session-creation pipeline builds the driver itself (client `s.autonomousDriverClient()`, completion callback but **no turn callback**), then `Start`, then `registerDriver` afterwards (non-atomic) | `server/services/session_creation_pipeline.go:347-355` |
| `run()`'s `recover` only logs; the completion callback runs synchronously on the driver goroutine, so a panic inside it is swallowed there | `session/autonomous_driver.go:259-263`, `fireCompletion` `:516-523` |
| `SetLifecycleContext` is an unguarded write, `driverCtx()` an unguarded read, and the respawn goroutine reads `a.lifecycleCtx` raw | `autonomous_orchestration_service.go:103-105, 129-134, 434` |
| The work prompt tells the agent to stay idle after `request_review`; `request_review` does not stop the work driver (the only MCP `StopDriverForSession` is the review verdict) | `server/mcp/tools_backlog.go:510, 1996-1998` |
| On `MarkStuck` `(false, err)` the callback today logs a Warn and still ends the ItemSession and respawns ("additive, never a gate"); `(false, nil)` is a status-precondition miss | `autonomous_orchestration_service.go:328-336`; `ent_repository_backlog.go:2128-2196` |
| `DiagnosticBundleInput` has no stuck-row-context field, so the stop-detail tail is not ingested by the diagnose bundle | `session/diagnose_bundle.go:93-105, 224-234` |
| Local `~/.stapler-squad` logs carry no production autonomous-stop evidence: the 21 "timed out waiting for initial idle state", 8 "LLM call failed", and 11 "recovered from panic" lines all come from `test-*` sessions | grep over `staplersquad*.log`, 2026-10-13 |

The last row means **AC1's root cause is code-derived, not log-derived**; the diagnosis document
must say so and label each cause VERIFIED (code path exists) vs INFERRED (frequency in production).

---

## Domain Glossary
*(Names are exact. Implementation, tests and comments must use them verbatim.)*

| Term | Definition | Notes |
|------|-----------|-------|
| `StopReason` | Why an `AutonomousDriver` run ended: `done`, `max_turns`, `startup_timeout`, `orchestrator_error`, `submit_failed`, `lease_timeout`, `rate_limit_exhausted`, `cancelled_external`, `cancelled_shutdown`, `panic`; `""` = unspecified/legacy | String newtype in `session/domain`; exhaustive-switch tests; `""` is treated as a stall |
| `StopReason.IsStall()` | True when the stop means the work did not finish and nobody asked for it | False for `done`, `cancelled_external` and `cancelled_shutdown` (the last two are `OutcomeClass` Cancelled). `StopReason("")` is a stall |
| `ObservedState` | What the *pane* was doing at stop time, independent of why the driver stopped: `idle_prompt`, `needs_approval`, `idle`, `working`, `error`, `exited`, `crashed`, `unknown` | Satisfies AC2's idle/awaiting-input, error, completed-or-exited split |
| `StatusBucket` | Detector status folded to `safe_idle`, `idle`, `needs_input`, `error`, `active`, `unknown` | Computed in `session` (owns `detection` and `IsSafeSteerStatus`); keeps `session/domain` dependency-free |
| `StopSnapshot` | Value captured once, in the driver, before anything is ended or killed: `CapturedAt`, `LastActivityAt`, `PaneTail`, `Bucket`, `DetectedStatus`, `DetectedContext`, `ControllerActive`, `TmuxAlive`, `PaneDead`, `ExitCode`, `CaptureErr` | `PaneTail` is bounded + ANSI-stripped but **not** redacted; memory only |
| `ClassifyObserved` | Pure `StopSnapshot -> ObservedState` | `session/domain` |
| `StallAction` | `none` / `nudge` / `resupervise` / `respawn` | Sum type |
| `StallDecisionInput` | `{Reason, State, NudgeEnabled, NudgeAllowed, ResuperviseAllowed, ItemInWorkStatus, Archived, RetryPending}` | Bundled struct (primitive-obsession checklist: six bools). `NudgeEnabled` is the already-combined value `diagnose_nudge_enabled AND autonomous_stall_nudge_enabled`, computed by `planStall`; `ItemInWorkStatus` is "the item is still in the status the work role works in (`in_progress`)", from the item read the callback already did (pre-mortem #3; after `request_review` the pane idles by design). No `Role` field by design: `handleWorkStall` is the only caller and is reached only for a backlog-linked `SessionRoleWork` stall, so the nudge text ("call request_review") is role-correct and review/triage/unrecognized/unlinked sessions cannot receive a nudge or re-supervise. A service test pins this (Task 3.2.3d). Instance status is deliberately not an input: `canRestart` and the nudge writer refuse a non-`Active` instance instead (Deferred, Adv C1) |
| `DecideStallAction` | Pure `StallDecisionInput -> StallAction`; the table in ADR-001 | First match wins; work-role only by construction (see above) |
| `OutcomeClass` | `Done` / `Stall` / `Cancelled`, derived from `StopReason` by `outcome.Class()`; `Cancelled` = `cancelled_external` or `cancelled_shutdown` | The callback branches on this at the top of every arm, never on `outcome.Done` alone; a `Cancelled` outcome has `Done == false`, which is exactly what made the old arms misread it (adversarial B6, Task 3.1.3b) |
| `AutonomousDriverOutcome` | Existing result struct; gains `StopReason`, `Snapshot` and `RunID` | `StopReason` is the single source: methods `IsDone()` / `IsStall()` derive from it; legacy `Done`/`Stuck`/`Reason` fields are read only by `normalizeStopReason` at the callback boundary (parse-at-boundary), never by decision code |
| `finish` | The driver's single exit: captures the snapshot, builds the outcome, is the sole `fireCompletion` caller, idempotent | Replaces the bare `break`s |
| `stopRequested` | Atomic flag set in `Stop()` before `cancel()`; separates `cancelled_external` from `cancelled_shutdown` | |
| `stopSource` | Narrow driver-side interface for the observations `finish` needs (status, last activity, liveness, pane exit, tail) | Test seam, mirrors `panePreviewer`; nil means "use the instance" |
| `BoundedStopTail` | `string -> string`: ANSI-stripped, last <=40 lines, <=4KB, rune-safe | `session/autonomous_stop_snapshot.go` |
| `sanitizeStopTail` | Service-side sink sanitizer: withhold on `RunPreGateSecurityCheck`, else `redactErrorDetail`, <=1KB, one line | Reuses existing patterns; no new regex |
| `StopDetail` | The `autonomous_stuck` row's `context` string: `<plain summary> \| stop_reason=… state=… action=… nudge=n/cap turns=… resupervise=n/3 last_activity=… [nudge_outcome=…] tail=…`. The plain summary is one to three sentences from the `stopCopy` table (Operator-facing copy, below); the `key=value` part is for greppability and tests | Only sink for the tail |
| `stopCopy` | Typed copy table owned by `server/services/autonomous_stop_detail.go`: `reasonSentence(StopReason)`, `stateSentence(ObservedState)`, `actionSentence(action, nudgeFailureClass, n, cap)` and `nudgeFailurePhrase(class)`. One function, `plainSummary(...)`, composes them and feeds both the notification body and the stuck-row lead sentence, so the two cannot drift | Codes (`orchestrator_error`, `lease_busy`, `idle_prompt`) never appear in a sentence; they stay in the `key=value` part and the log line. Completeness and golden-string tests in Task 3.1.4c |
| `action` (row/log value) | What the callback finally did: `nudged`, `respawned`, `resupervised`, `none`, plus the transient `nudging` that exists only between plan time and the end of the nudge effect (seconds). `left` (nudge delivery uncertain, pane left alone) is written as `none` with `nudge_outcome=uncertain` | `resupervised` never reaches a row (no row is written for it); it is in the log line |
| `AutonomousStopReason` | `domain.StopReason` (typed) on `AutonomousModeState` / `InstanceSnapshot` | In-memory, actor setter; not on the wire |
| `SetAutonomousStopped` | Instance actor setter (returns `error`: the `sendSyncErr` result, non-nil only when the instance actor's ctx is done, i.e. the instance is going away; the older `_ = i.sendSyncErr` setters are untouched): clears mode/turns, sets outcome (`""|done|stuck`) and `AutonomousStopReason` atomically | Supersedes test-only `SetAutonomousComplete`; used whenever the session ends autonomy: Respawn, None, done, `cancelled_external` (outcome `""`) and the flag-OFF `cancelled_shutdown` callback branch (outcome `"stuck"`, byte-for-byte what the callback writes today) |
| `SetAutonomousStoppedQuiet` | Variant of `SetAutonomousStopped` for the boot pass: clears mode/turns, records `AutonomousStopReason`, outcome is always `""` | Boot-cleared candidates must not show the `SessionCard.tsx:888` "stuck" badge (outcome is not persisted, no row or notification explains it; adversarial C2) |
| `SetAutonomousStopObserved` | Instance actor setter: records `AutonomousStopReason` and zeroes turn counters, leaves `AutonomousMode` and outcome untouched | Used while a Nudge or Resupervise is in flight, so the approval gate (`server.go:889-891`) keeps working, and for `cancelled_shutdown` when `autonomous_driver_readoption` is ON; a recovery must end in a registered driver or `SetAutonomousStopped`, and the boot pass clears the mode of every candidate it definitively rejects (never mode true with no driver, except candidates logged `unknown` or `skip`; see `readoptGate`) |
| `RunID` | Per-driver unique id (process-wide atomic counter) carried on the outcome and the registry entry | Lets the callback tell "my own run" from a successor registered under the same title |
| `a.mu` rule | `a.mu` guards **map membership only** (`drivers`, `starting`, `recoveries`). While it is held: no call to `driverCtx()` (an atomic load, never under the lock), `NewAutonomousDriver`, `Start`, `Stop`, `withCostSink`/`storageGetter`, `SessionService`, `instanceFinder`, the event bus, or any `Instance` method that takes `i.mu` | Pre-mortem P1 #1. Enforced by a lock-probing test (REG-12) and watchdog tests (REG-13..REG-15) |
| `lifecycleCtx` (changed) | `atomic.Pointer[context.Context]`; `SetLifecycleContext` stores, `driverCtx()` loads (falls back to `Background()` when unset); every reader, including the respawn goroutine at `:434`, goes through `driverCtx()`. Task 3.1.3e **moves** `lifecycleCtx`, `SetLifecycleContext` and `driverCtx()` out of `autonomous_orchestration_service.go` into `autonomous_driver_registry.go`, so the boot-pass trigger (Task 3.1.3f) edits the registry file and never the service file | Never read or written under `a.mu`, so locking the registry never nests with it |
| `completion` | Value returned by `claimCompletion(title, runID) (completion, claimResult)`: `{ctx, release}`; `ctx` is the recovery ctx derived from `a.driverCtx()`, cancelled by `stopAndDeregisterDriver`. `claimResult` is `claimed`, `superseded` (a different non-zero RunID is registered) or `duplicate` (same RunID already has a recovery entry) | Runs under one `a.mu` hold that does map mutation only: supersession check, driver deregistration and `recoveries[title]` registration (the ctx is derived from the atomic-loaded `driverCtx()` *before* the lock is taken), so there is no window in which a stop finds neither (architecture C1). A completion from a *newer* run cancels the older run's recovery and takes the entry. `release` is `defer`red by the callback and is a no-op if the entry was already replaced. **`completion.ctx` bounds only the nudge settle/tmux/submit calls and the restart check**; every storage write in `handleWorkStall` (`MarkStuck`, `UpdateItemSessionEnded`, `RemediationDue`, the claim) uses the outer `context.Background()` ctx, as today (pre-mortem #6) |
| `recovery` | The registry entry `a.recoveries[title] = {cancel, runID}` created by `claimCompletion` for every completion (before the item lookup, `planStall` or `MarkStuck`) and removed by `release` or by `restartAfterRecovery` | Cancelled by `stopAndDeregisterDriver`; replaces the plain `nudgeInFlight` set |
| `startDriverIfAbsent` | `(inst, timeout) (*AutonomousDriver, error)` on the service, **reserve-then-start**: (1) under `a.mu` check `drivers[title]` and `starting[title]`; a registered driver whose `Finished()` is true is treated as absent (replaced, with a Warn), any other registered driver or a `starting` reservation returns `ErrDriverPresent`; otherwise insert a `starting` sentinel and unlock; (2) **outside the lock** build the driver (`NewAutonomousDriver`, `withCostSink`, callbacks) and `Start` it; (3) re-lock, and if the sentinel is still the entry (no stop arrived) swap it for the driver, else `Stop()` the new driver outside the lock and return `ErrDriverPresent`; on a build or `Start` error delete the sentinel and return `ErrNoDriverPool` / the `Start` error. `StartAutonomousDriverWithTimeout`/`ForInstance` become void wrappers over the same inner `startDriver` (public signatures and the interface assertion unchanged); every recovery and the boot pass call the error-returning form | Closes the check-then-register gap (architecture C8, adversarial C4) and the silent-failure gap (architecture C7, adversarial C3) without calling anything that locks while `a.mu` is held (pre-mortem P1 #1: `NewAutonomousDriver` -> `inst.GetController()`, `withCostSink` -> `storageGetter()` -> `SessionService.GetStorage()`). A `Stop` during step 2 deletes the sentinel, so the finished build is torn down instead of registered. The liveness check keeps a dead registered driver from returning `ErrDriverPresent` forever (pre-mortem #5) |
| `registerDriver` (changed) | Never overwrites silently: if a different driver is registered under the title it is `Stop()`ped (outside `a.mu`; its completion is then superseded by RunID) and a Warn is logged | Covers the remaining void starters (spawn, toggle) |
| `restartAfterRecovery` | `(rec, inst, timeout) error`: under one `a.mu` hold verifies `recoveries[title] == rec`, `rec.ctx.Err() == nil` and `canRestartLocked`, inserts the `starting` sentinel and deletes the recovery entry; then builds and `Start`s the driver **outside** the lock and swaps the sentinel for it exactly as `startDriverIfAbsent` does | The only restart path for Nudge/Resupervise; returns an error so the caller falls back to Respawn |
| `canRestart` / `canRestartLocked` | Same pure check of `{ctx live, same runID, !archived, !paused, Status == Active, controller present, drivers[title] == nil and no `starting` sentinel, snapshot AutonomousMode still true}`. `canRestart(rec, inst)` takes `a.mu` itself (called before the nudger); `canRestartLocked` assumes the caller holds it (called only inside `restartAfterRecovery`). Instance reads go through `Snapshot()`, never `i.mu` | Run immediately before the nudger is invoked and again immediately before the restart (architecture C1); two variants so no path re-takes the non-reentrant mutex |
| `stallRecoveryGuard` | Shared, mutex-guarded `itemID -> deferUntil` marker injected into both `AutonomousOrchestrationService` and `BacklogService` | Stamped in `handleWorkStall` the moment `planStall` returns Nudge/Resupervise (before `MarkStuck` and its DB round trips), refreshed to the 30 min window on success, and **replaced by a 2 min stamp on every exit that does not end with a registered driver** (leave-the-pane, Respawn fallback, cancel), so it is held only while a recovery is live (pre-mortem #12); consulted by `RemediateStaleWorkSession` (defer window 30 min) |
| `resuperviseBudget` | In-memory `sessionUUID -> count`, cap 3 | Lost on restart (accepted); keyed by ItemSession UUID so a new session starts fresh |
| `stallResult` | `handleWorkStall` return value `{Action domain.StallAction; Notify notifyOverride}`; `notifyOverride{Suppress bool; Title, Body string}` | The outer function consumes it so the generic FAILURE notification can be suppressed or reworded |
| `handleWorkStall` | Extracted work-role `!Done` arm of `onAutonomousDriverComplete`. **It owns the whole `switch plan`** (Respawn, None, Nudge, Resupervise) and the `planStall` call; the outer function only passes `completion`, `is`, `item`, `outcome` and consumes the `stallResult` | Extraction is behaviour-preserving in 3.1.1a (always returns `{Respawn}`); 3.2.3 fills it. Review, triage, unrecognized-role and unlinked sessions never reach it and keep today's handling. Two extraction rules (pre-mortem #6, #7): the outer ctx stays `context.Background()` for all storage writes (`completion.ctx` goes only to the nudger and the restart check), and the instance write plus its `SessionUpdatedEvent` are one `applyInstanceStop` call made once at plan time, never from a function-exit defer |
| `StallNudger` | Service-side port: `NudgeStalledSession(ctx, inst, claim) error`; default adapter `defaultStallNudger` is a plain struct with a plain method (no closures) that calls `session.NudgeStalledAutonomousSession` | Fake in tests; the adapter's `defaultStallNudger.NudgeStalledSession` is the unit pinned `kindChain` |
| `ErrNudgeSubmitUncertain` | Every error from `SubmitDriverContent` after a successful claim, including `ErrSubmitNotConfirmed` and cancel-after-send (`pane_submit.go:100-142`) | The text may have landed or may sit unsubmitted in the input box: the slot stays consumed (no refund), the pane is **not** killed, not respawned and **not re-supervised** (a re-supervised driver's first turn would append to the pending input); always leave-the-pane (failure table). If the recovery ctx is also cancelled, the ctx-cancel row wins (adversarial C5) |
| `NudgeStalledAutonomousSession` | Session-level writer: gates, lease, claim, fixed-text submit. The only new raw-write caller | One new pin row |
| `autonomousStallNudgeText` | Fixed nudge string; no interpolation of item or pane content | Exact text: `The autonomous driver noticed this session stopped. If the task is finished, call request_review now. If it is not, continue working on it and call report_progress when you reach a milestone. Do not ask for confirmation.` Test asserts equality with the constant and that the submitted bytes equal it |
| `ClaimDiagnoseNudgeSlot` | `Storage` method exposing the atomic claimed/not-claimed result as a `ClaimResult{Claimed, JustCapped bool}` struct; one UPDATE with `count < cap - reserve AND (next_eligible IS NULL OR next_eligible <= now)` | `RecordDiagnoseNudgeAttempt` (MCP path, unchanged semantics) keeps its own UPDATE. There is deliberately no refund (Deferred / accepted concerns, Adv C5) |
| `nudge slot` | One unit of `(item, autonomous_stuck)` nudge headroom under the cap | Lifetime per row (see Unresolved Questions) |
| `re-supervision` | Starting a fresh driver on the nudged or still-working session so completion tracking continues | Via `restartAfterRecovery` (5 min warm-up); work-role backlog-linked sessions only. Each run gets a fresh `AutonomousMaxTurnsOrDefault`, so a session can use up to (1 + cap) x the turn budget; the chain is traceable through `resupervise=n/cap` in every `StopDetail` and log line |
| `ReadoptAutonomousDrivers` | Boot pass: evaluates the `readoptGate` list for every instance whose persisted `AutonomousMode` is true, in a stable order (`CreatedAt` ascending, ties by instance UUID), starts one driver per passing instance via `startDriverIfAbsent` (5 min warm-up) up to the per-boot cap, clears the ghost mode (via `SetAutonomousStoppedQuiet`) only on a **definitive reject**, and defers the cap excess and every `unknown` candidate to bounded follow-up passes | See Story 3.1.3 and Task 3.1.3c. Runs once after both the lifecycle ctx is set and the instances are loaded, wired and settled (Step 7.6) |
| `readoptGate` | Ordered list of named predicates. Each evaluation returns one of three classes: `pass`, or a failure classified **reject** (definitive: clear mode), **skip** (leave mode untouched, log) or **unknown** (transient probe error: leave mode untouched, log at Warn, re-evaluate in a follow-up pass). Adoption requires ALL gates `pass`; the first non-pass gate and its class are logged (dry-run evidence) | Names: `status_active`, `not_archived`, `not_paused`, `mode_persisted`, `controller_present`, `no_driver`, `not_retry_pending`, `item_session_open`, `item_status_matches_role`, `pane_alive`, `pane_owned`, `no_step75_driver`. Classification table in Story 3.1.3 |
| `autonomous_stall_nudge_enabled` | Live-settable feature flag; default is the single constant `autonomousStallNudgeDefault` in `config/config.go` (**false**, same pattern as `autonomousDriverReadoptionDefault`; no env var). Gates **only** the in-process `Nudge` branch of `planStall`; effective value is `diagnose_nudge_enabled AND autonomous_stall_nudge_enabled` | Pre-mortem P1 #2: `diagnose_nudge_enabled` is already `true` in the owner's workspace config, so reusing it alone would make the in-process nudge live on first deploy and spend the item's shared 2-slot budget before the diagnose agent saw the item. Requiring both keeps `diagnose_nudge_enabled` the single kill switch for every autonomous pane write, so turning it off still restores today's respawn |
| `autonomous_driver_readoption` | Live-settable feature flag; default is the single constant `autonomousDriverReadoptionDefault` in `config/config.go` (owner decision: **OFF**, see Deferred). OFF = dry-run: gates are still evaluated and logged, nothing is started | Flag OFF restores today's in-memory shutdown mode/outcome only, not the stuck row, respawn or notification (see Risk Control) |

---

## Pattern Decisions

**Step 0.5 alternatives (CREATIVE pass).** (1) *Deterministic decision at driver stop* — strength: synchronous, testable, no new surface; weakness: fixed nudge text. (2) *Route every stall to `DispatchDiagnose`* — strength: adaptive; weakness: slow, non-deterministic, races teardown. (3) *Per-reason `StuckReason` values* — strength: durable per-reason UI; weakness: ent/proto/TS churn, fragments the nudge budget. Chosen: (1); (2) stays the escalation after the cap; (3) rejected. See ADR-001.

| Component | Pattern Chosen | Source | Alternative Rejected | Reason |
|-----------|---------------|--------|---------------------|--------|
| `StopReason`, `ObservedState`, `StatusBucket`, `StallAction` | Sum types (typed string consts + exhaustive tests) | type-driven-design | Free-text `Reason`, raw strings | Free text is how every exit became "max turns reached" |
| `StallDecisionInput` | Parameter object | Fowler (Refactoring) / primitive-obsession checklist | 6 positional args incl. 4 bools | Same-typed argument pile |
| `ClassifyObserved` / `DecideStallAction` | Pure functions, functional core | Hexagonal "pure core" | Method on service reading live state | Needs table-driven tests per state; no clock/tmux |
| Driver exit | Single exit point (`finish`) + idempotence guard | Guard clause / Template Method | Callback fired at each `break` | Four sites already drifted (all say "max turns"); panic fired none |
| Snapshot capture | Observer seam `stopSource` | GoF Adapter | Capture in service callback | Races pause/delete teardown (ADR-001 option D) |
| Nudge write | Port + Adapter (`StallNudger`) | Hexagonal / GoF Adapter | Service calls `session` writers directly | Service tests need no tmux |
| Nudge cap | Atomic conditional UPDATE claim (`ClaimDiagnoseNudgeSlot`) | PoEAA optimistic check-and-set | Read `DiagnoseNudgeAllowed` then `Record…` | Check-then-reserve race (memory: `instinct_check_then_reserve_needs_process_wide_lock`) |
| In-flight recovery guard | Keyed map of `{cancel, runID}` under `a.mu`, created inside the same critical section that deregisters the finished driver (`claimCompletion`) and cancelled by the stop path; the critical section mutates maps only (`a.mu` rule, Glossary) | Concurrency primitive selection | Plain keyed set; register later in the nudge branch | DB claim bounds count, not concurrent writes to one pane; a plain set cannot be cancelled by Stop (adversarial B3); a later registration leaves a window where a stop finds nothing (architecture C1) |
| Driver start (recoveries and boot pass) | `startDriverIfAbsent`: reserve a `starting` sentinel under `a.mu`, build and `Start` the driver outside the lock, swap the sentinel for the driver (or tear it down if a stop deleted the sentinel), returning an `error`; a registered driver with `Finished() == true` counts as absent; `registerDriver` stops a displaced driver instead of overwriting | Reserve-then-commit (two-phase check-and-set) | Check, build, `Start` and register under one `a.mu` hold (the iteration-3 design) | The single-hold design called `a.driverCtx()`, `inst.GetController()` and `storageGetter()` with the non-reentrant `a.mu` held, so a locked `driverCtx()` self-deadlocks and a request path holding a storage lock can invert against it (pre-mortem P1 #1). A void starter hides a failed start (architecture C7/C8, adversarial C3/C4) |
| Boot re-adoption | Strict conjunction of named gates, each result classed reject / skip / unknown and logged, dry-run when the flag is off, ghost mode cleared only on a definitive reject | Fail-closed allow-list | Adopt every persisted `AutonomousMode == true` | Persisted mode is not proof of a live, wanted driver (adversarial B5) |
| Cross-service remediation deferral | Shared `stallRecoveryGuard` (marker + window) consulted by the stale detector's gate (`remediateStaleWorkWithBackoffGate`) *before* `RemediationDue`, with the in-remediator check as defence in depth | Pitfalls P7 | Service-private map; check only inside `RemediateStaleWorkSession` | Staleness is progress-time based, so the 2h threshold does not prevent a collision (adversarial B4); a check after `RemediationDue` burns a `stale_work` attempt (architecture C6) |
| `onAutonomousDriverComplete` | Extract Function then Seam | Fowler (Refactoring) | Inline new branches | Function is ~370 lines at nesting 6+; gocognit 40 / funlen 150 |
| Stop reason exposure | Existing sinks only (snapshot field, row context, log) | YAGNI | New proto field | `SessionCard.tsx` string-compares outcome (ADR-002) |
| Pane-tail redaction | Reuse existing patterns, fail closed | Defence in depth | New regex set | Three pattern sets already exist; add none |

---

## Tech Debt Disposition

| Area | Existing Issue | Disposition | Justification |
|------|----------------|--------------|----------------|
| `server/services/autonomous_orchestration_service.go` `onAutonomousDriverComplete` (L257-625) | ~370 lines, nesting 6+, three role arms, bookkeeping interleaved with notification | **Isolate via seam** (Story 3.1.1) | One behaviour-preserving extraction of the work `!Done` arm into `handleWorkStall`; new nudge/decision code lives in a new file and is called from that seam. Review/Triage arms untouched. |
| `session/autonomous_driver.go` `run()` (L258-441) | Bare `break`s collapse 8 causes into one label; panic exits silently | **Refactor-first** (Story 2.1.1) | The typed reason cannot be added without replacing the exits; a single `finish` is the fix. If `make ready-complexity-gate` flags `run()`, extract startup-wait and submit-turn methods as the story's first, behaviour-preserving commit. |
| Raw `inst.AutonomousMode/Outcome/Turn` writes in the service (L282-289) | Bypasses the instance actor (rule `instance-lock-free-reads.md`) | **Refactor-first** (Story 3.1.2) | Replaced by one setter; stays within the same function being touched. |
| `server/mcp/tools_diagnose.go` `writeNudge` | A second nudge write path | **Extend as-is** | Untouched; this plan adds exactly one new pinned row instead of a second MCP-shaped path. Converging the two on one session-level function is a follow-up, not scope. |
| Stale-work detector `remediateStaleWorkWithBackoffGate` (`session/backlog_lifecycle_stale.go:292`) / `RemediateStaleWorkSession` (`backlog_service_triage.go:2182`) | Kills + respawns an idle-at-prompt session with no nudge; measures progress-report time, so it can fire seconds after a nudge | **Extend (one guard, two sites)** (Story 3.2.5) | The gate type-asserts the remediator for an optional `StaleWorkDeferrer` and returns before `RemediationDue` (no `stale_work` attempt consumed); `BacklogService` implements it over `stallRecoveryGuard` and re-checks inside `RemediateStaleWorkSession` after the `active == nil` branch. The kill-and-respawn path itself stays snapshot-less and nudge-less (AC3 scope, see Deferred C1). |
| `buildTurnCallback` raw `AutonomousTurn` writes (L175-177) | Same unguarded-write class | **Extend as-is** | Not touched by this change; noted, not fixed (do what was asked). |

---

## Migration Plan

None. No schema, proto, ent or new persisted field. `AutonomousStopReason` is in-memory; the
stuck row's `context` is an existing free-text column.

## Observability Plan

- **Logs**: exactly one structured line per stop from `onAutonomousDriverComplete`, message
  `[AutonomousDriver] stopped` (Info for done/cancelled, Warn otherwise), keys `session`, `item`,
  `role`, `run_id`, `stop_reason`, `state`, `turns`, `max_turns`, `last_activity_age`, `action`,
  `tail_bytes`, `resupervise` (`n/cap` on every stop for a backlog work session, so a chain of re-supervised runs is reconstructible) and, for `action=resupervise`, `prior_stop_reason`. For a work-role stall the line is emitted once from the exit of
  `handleWorkStall` so `action` is the **final** action (a Nudge that fell back to Respawn logs
  `respawn`), and carries `nudge_outcome=n/a|success|disabled|not_idle|lease_busy|ownership|slot_unavailable|uncertain|ctx_cancelled|restart_failed`
  (`disabled` = the plan would have been Nudge had both flags been on, which is the pre-enablement
  evidence of how many stalls the nudge would have caught) and, when the re-supervise cap is what
  sent the stop to Respawn, `resupervise_cap_hit=true`. The tail is never logged. Existing driver
  warnings stay; they gain `stop_reason` where cheap.
- **Durable record**: `autonomous_stuck` row `context` = `StopDetail`: a plain-language summary
  sentence first (what stopped, what the session was doing, what the system did and whether the
  operator must act; copy table below), then `stop_reason`, `state`, `action`
  (`nudged|respawned|none|nudging`), `nudge=n/cap`, turns, `resupervise=n/cap`, last activity,
  sanitized tail <=1KB, newlines shown as ` ⏎ ` because `StuckItemDetail.tsx:203` renders the string
  in a `<span>` that collapses whitespace. The row is written at plan time and, **for a Nudge
  plan only**, refreshed once more when the final action is known (best-effort: a refresh error is
  a Warn, never a gate): `MarkStuck` on an open row refreshes `context` and `last_checked_at`
  (fact table, `ent_repository_backlog.go:2128-2196`), so `action=nudging` becomes `nudged`,
  `respawned` (nudge failed) or `none` (delivery uncertain). **After a successful nudge the row is
  annotated, not resolved**: it holds the lifetime nudge counter (`diagnose_nudge_count`), it is
  what `ClaimDiagnoseNudgeSlot` and the diagnose agent key on, and it is closed by the paths that
  close any open `autonomous_stuck` row today (the DONE branch's `resolveAutonomousStuck`; other
  closers not verified). The operator sees `Nudged automatically ... You do not need to do
  anything` as the lead sentence instead of a bare stuck row (UX G1).
- **Notification**: title and body from the `stopCopy` table (Operator-facing copy, below), one
  plain sentence per reason, state and action; no codes, no "stopped after N turns" (a turn count,
  when shown, reads "during turn N"); no tail; nudge success publishes an INFO "nudged" instead of
  the FAILURE "stuck" notification; Resupervise, `cancelled_external` and `cancelled_shutdown`
  publish nothing (log line only; accepted, ADR-002). A `Cancelled` outcome never publishes the
  "complete" notification either. A failed nudge that falls back to Respawn names the failure in an
  operator phrase ("the session was busy again"); the class code (`nudge_outcome=lease_busy`) is
  log-only.
- **Operator read path (AC2)**: the stuck-row `context` is returned by the existing
  `ListStuckBacklogItems` RPC (`StuckBacklogItem.context`) and shown in `StuckItemDetail.tsx:102`.
  Stops with no stuck row (Resupervise, external cancel, shutdown, non-backlog, review/triage
  roles that keep today's handling) are visible in the log line only; the in-memory
  `AutonomousStopReason` is a test seam, not an operator surface. A Resupervise that later hits the
  cap writes its stuck row with `resupervise_cap_reached runs=N last_stop_reason=<reason>` so the
  chain is traceable without a log search. A Resupervise in flight is operator-invisible by design
  (bounded to about 16 minutes by the cap; its end is always visible, either a later stop or the
  cap-reached row): accepted, with reasons, in ADR-002 (UX G3).
- **Metrics**: none new (the log line is greppable; the stuck row is the durable record). A counter
  endpoint next to `/actuator/git-backend` was considered and deferred: it is a new surface outside
  this plan's no-new-surface constraint (C1) and the log line carries every dimension.
- **Soak exit criteria (pre-mortem #9/#10).** Review date: 14 days after
  `autonomous_stall_nudge_enabled` is first turned on (or 14 days after deploy for the
  re-supervise and shutdown-retention evidence, whichever the owner is reading). Source is the
  `[AutonomousDriver] stopped` records in `~/.stapler-squad/logs/staplersquad.log` (JSON lines; key
  names per the Logs bullet, verify against the first real line):
  `jq -r 'select(.msg=="[AutonomousDriver] stopped") | [.stop_reason,.state,.action,.nudge_outcome] | @tsv' staplersquad.log | sort | uniq -c | sort -rn`.
  Report: count per `stop_reason` x `action`; nudge outcomes (`nudge_outcome`); re-supervise cap
  hits (`resupervise_cap_hit=true`); and any burst of `action=respawn` with
  `prior_stop_reason in (orchestrator_error, rate_limit_exhausted, lease_timeout)` and
  `state=working` (the fleet-wide pool-outage signature). Decision rules, agreed now so the soak is
  not "met" by default: (a) if `max_turns` is more than 50% of stalls, open a follow-up to revisit
  ADR-001 row 4 (what `max_turns` + `idle_prompt` + remaining budget should do) and the lifetime
  nudge counter; (b) if `nudge_outcome=disabled` is non-zero and `success` stays zero after the
  flag is on, the gate (not the feature) is suspect; (c) if the working + exhausted -> Respawn
  burst appears, add the pool-outage guard recorded in Deferred.
- **Success target and delete thresholds (product; the chosen numbers are INFERRED, not derived
  from data, because there is no pre-change baseline: local logs hold only `test-*` evidence).**
  Hypothesis and target are stated in `requirements.md` ("Falsifiable hypothesis"); this is the
  measurement. Two ratios, from the same `stopped` log lines (join on the `item` key):
  - **Nudge resolution rate** = work stalls with `state=idle_prompt`, `action=nudged`,
    `nudge_outcome=success`, **and no `action=respawned` line for the same `item` within 24 h**,
    divided by all work stalls with `state=idle_prompt` where the nudge was eligible
    (`nudge_outcome` in `success|not_idle|lease_busy|slot_unavailable|uncertain|restart_failed`;
    `disabled` and `n/a` excluded). Target >= 50%.
  - **Re-supervise completion rate** = re-supervised stops that are not followed by a
    `resupervise_cap_hit=true` line for the same `item`, divided by all `action=resupervised`
    stops. Target >= 50%.
  - **Delete thresholds.** Nudge: with at least 10 eligible stalls in the window, a resolution
    rate below 30% means the assumption is false: turn `autonomous_stall_nudge_enabled` off and
    delete the Nudge branch (Tasks 3.2.2, 3.2.3c; the typed reasons, snapshot and re-supervise
    stay). Fewer than 10 eligible stalls in two consecutive 14-day windows means the problem the
    nudge addresses is rare: delete the branch the same way (it is dead weight at that
    frequency). Re-supervise: a completion rate below 30% over at least 10 stops means row 5 should
    return `respawn` (one row of `DecideStallAction`, one test table).
- **Review dates (so the soak cannot silently never start).** The PR description states three
  dates computed at merge: **flip decision** = merge date + 7 days (read the `readopt candidate`
  dry-run lines and the `nudge_outcome=disabled` count, then flip or explicitly defer each of the
  two flags); **soak review** = the flip date + 14 days; **sunset** = merge date + 30 days: if
  `autonomous_stall_nudge_enabled` is still off with no recorded decision, the nudge is treated as
  unvalidated and a delete-or-enable decision item is opened. The owner (or the shipping agent, as
  a one-shot `/schedule` routine or a Todoist reminder) holds these; the plan cannot enforce them,
  so the ship step (Task 4.1.1b) lists them as a checklist item.
- **Alerts**: no new alerts.

## Operator-facing copy (owner review)

Every sentence an operator can read comes from the `stopCopy` table (Task 3.1.4b), composed as
`<reason sentence> <state sentence> <action sentence>`. The same string leads the stuck row's
`context` (Unfinished tab "Why:") and is the notification body (prefixed `Session '<name>':`). The
wire values (`stop_reason=…`, `idle_prompt`, `lease_busy`) appear only in the `key=value` part of the
row and in the log line. Task 3.1.4c asserts each string byte for byte (golden) and asserts that no
sentence contains any `StopReason`, `ObservedState` or `nudge_outcome` code. Wording below is the
proposal for the owner to edit; each cell is one constant.

**Reason sentence (per `StopReason`)**

| `StopReason` | Sentence | Notification |
|---|---|---|
| `max_turns` | The autonomous run used up its turn allowance before finishing. | yes |
| `startup_timeout` | The autonomous run gave up waiting for the session to become ready. | yes |
| `orchestrator_error` | The autonomous run hit an internal error and stopped. | yes |
| `submit_failed` | The autonomous run could not type its next instruction into the session. | yes |
| `lease_timeout` | The autonomous run could not get access to the session because something else was using it. | yes |
| `rate_limit_exhausted` | The autonomous run gave up waiting for the AI service's rate limit to clear. | yes |
| `panic` | The autonomous run crashed unexpectedly. | yes |
| `""` (unspecified) | The autonomous run stopped without recording a reason. | yes |
| `done` | (unchanged "Autonomous fix complete") | existing |
| `cancelled_external`, `cancelled_shutdown` | none | none |

**State sentence (per `ObservedState`)**

| `ObservedState` | Sentence |
|---|---|
| `idle_prompt` | The session was waiting at its prompt. |
| `needs_approval` | The session was waiting for an approval. |
| `idle` | The session was sitting at a shell prompt. |
| `working` | The session was still working. |
| `error` | The session was showing an error. |
| `exited` | The session had exited. |
| `crashed` | The session had crashed. |
| `unknown` | The session's state could not be read. |

**Action sentence and title (per final action); "You act?" is the plain answer to "do I have to do something"**

| Final action | Title (type) | Action sentence | You act? |
|---|---|---|---|
| nudging (plan-time row annotation, before the nudge outcome is known) | none (row only; the notification fires after the outcome) | A prompt is being sent to continue it automatically. You do not need to do anything. | No |
| nudged | Autonomous session nudged (INFO) | It was prompted to continue automatically (nudge `n` of `cap`). You do not need to do anything. | No |
| respawned (no nudge tried) | Autonomous fix stuck (FAILURE, unchanged) | A fresh session is being started automatically. You only need to act if a "needs manual attention" notification follows. | Only if a follow-up says so |
| respawned after a failed nudge | Autonomous fix stuck (FAILURE, unchanged) | A prompt could not be sent (`<failure phrase>`), so a fresh session is being started instead. You only need to act if a "needs manual attention" notification follows. | Only if a follow-up says so |
| respawned after a nudge that was sent but could not be re-attached (`restart_failed`) | Autonomous fix stuck (FAILURE, unchanged) | A prompt was sent, but the autonomous run could not be re-attached, so a fresh session is being started instead. You only need to act if a "needs manual attention" notification follows. | Only if a follow-up says so |
| none: delivery uncertain (`uncertain`) | Autonomous fix stuck (FAILURE, unchanged) | A prompt may or may not have reached the session, so nothing was restarted. Open the session and check it. | **Yes** |
| none: archived | Autonomous fix stuck (FAILURE, unchanged) | Nothing was restarted because the session is archived. You do not need to do anything. | No |
| none: retry pending | Autonomous fix stuck (FAILURE, unchanged) | Nothing was restarted because an automatic retry is already scheduled. You do not need to do anything. | No |
| respawned, re-attach cap reached | Autonomous fix stuck (FAILURE, unchanged) | The autonomous run was re-attached `n` times while the session kept working, so a fresh session is being started. You only need to act if a "needs manual attention" notification follows. | Only if a follow-up says so |
| unlinked / non-work role (today's handling) | Autonomous fix stuck (FAILURE, unchanged) | Open the session to review what was accomplished and give the next instruction. | **Yes** |
| resupervised | none (log line only) | n/a | No |

**Failure phrases (per `nudge_outcome`, plain text; code stays in the log)**

| Class | Phrase |
|---|---|
| `not_idle` | the session was busy again |
| `lease_busy` | another process was using the session |
| `ownership` | the session's terminal was not the one this run expected |
| `slot_unavailable` | no automatic prompt was available for this item (limit reached or one was sent recently) |
| `ctx_cancelled`, `uncertain`, `disabled`, `n/a` | never in a sentence (cancelled: no notification; uncertain: own row above; disabled: today's respawn wording) |

Example Why line for a work stall that was nudged: *The autonomous run hit an internal error and
stopped. The session was waiting at its prompt. It was prompted to continue automatically (nudge 1
of 2). You do not need to do anything. | stop_reason=orchestrator_error state=idle_prompt
action=nudged nudge=1/2 turns=2 resupervise=0/3 last_activity=… tail=…*

**Owner decisions in this table** (one constant each): (1) whether a plain "restarting
automatically" respawn should stay FAILURE-class (today's behaviour, kept) or become INFO; (2) the
word "autonomous run" (matches `SessionCard.tsx`'s "Autonomous run stopped"); (3) whether
`needs_approval` + respawn should say "waiting for an approval" (it is a restart, not an approval).

## Risk Control

- **Feature flags** (all live-settable via the feature-flag RPC/panel, never an env var):
  - `diagnose_nudge_enabled` (existing, default off in code, live-read at decision time) stays the
    **kill switch for every autonomous pane write**: the in-process nudge requires it as an AND.
    Its registry description (`feature_flag_service.go:278-281`) is updated to mention the
    in-process autonomous nudge. **It is `true` today in the owner's workspace config**
    (`config.json:96`), so on its own it would turn the in-process nudge on at first deploy; that
    is why the nudge has its own flag below. With it off the decision table can still return
    `None` (rows 1-3), `Respawn` (rows 4, 6, 8) and `Resupervise` (row 5); `Nudge` (row 7) is
    unreachable.
  - `autonomous_stall_nudge_enabled` (new, live-read at decision time; default is the single
    constant `autonomousStallNudgeDefault` in `config/config.go`, **false**) gates only the
    in-process `Nudge` branch, AND-ed with `diagnose_nudge_enabled` (decision: AND, because the
    existing flag is documented as the autonomous-write kill switch; with OR or an independent
    flag, switching it off would no longer restore today's behaviour). It does not touch the LLM
    diagnose agent's path (`tools_diagnose.go:338`), so the owner can enable the agent's nudge
    and the in-process nudge independently, and the in-process nudge cannot spend the item's
    shared slots before the agent has seen it. Acceptance: with `diagnose_nudge_enabled = true`
    and this flag off, a stall at `idle_prompt` is Respawn (today's behaviour), STL-7.
  - `autonomous_driver_readoption` (new, live-read; default is the single constant
    `autonomousDriverReadoptionDefault` in `config/config.go`, **owner-decided: OFF** for the
    first release) gates the boot pass and the in-memory shutdown mode/outcome. OFF makes the
    first boot a dry-run whose per-candidate gate log is the gate-validation evidence; the owner
    flips the flag live after reading it. Every boot logs every candidate with its gate result
    either way.
- **Flag-off semantics for `autonomous_driver_readoption` (adversarial C2, round-3 C7)**: OFF
  restores today's *in-memory* graceful-shutdown mode/outcome (mode cleared, outcome `stuck`),
  and nothing else. Today's shutdown also writes an `autonomous_stuck` row, dispatches a respawn
  on the cancelled lifecycle ctx and publishes the FAILURE notification
  (`autonomous_orchestration_service.go:282-290,328-336,398-442,601-604`); OFF does **not**
  restore those, they are ungated corrections (below) because a shutdown is not a stall. A
  rollback decision must not assume OFF means "shutdown behaves exactly as before". Concretely:
  (a) the `cancelled_shutdown` branch reads the flag at callback time and, when OFF, calls
  `SetAutonomousStopped` (mode cleared, outcome `stuck`, no stuck row, no respawn, no
  notification); (b) the boot pass still runs in dry-run, logs each candidate with its gate
  result or `would_adopt`, starts nothing, and clears the stale mode (via
  `SetAutonomousStoppedQuiet`, outcome `""`) of every candidate classified reject **or**
  `would_adopt` (none is adopted); `skip` and `unknown` candidates are left untouched and logged,
  in both flag states. Invariant asserted by test after the pass in both flag states: no instance
  has `AutonomousMode == true` without a registered driver, **except** candidates logged `skip`
  (a driver is registered, so the invariant holds trivially) or `unknown` (mode left for a
  follow-up pass or a human; logged at Warn).
- **What an operator sees on a graceful restart, flag OFF (UX G2; accepted, not a regression).**
  Today a shutdown writes an `autonomous_stuck` row, publishes the FAILURE notification and
  respawns on the cancelled ctx (fact table, `autonomous_orchestration_service.go:282-290,328-336,
  398-442,601-604`). After this change, for `cancelled_shutdown` **regardless of the flag**: no
  stuck row, no notification, no respawn, no `UpdateItemSessionEnded` (Task 3.1.3b; SVC-4, SVC-8).
  What remains with the flag OFF is only the in-memory `autonomous_outcome = "stuck"` badge on the
  `SessionCard`, set by `SetAutonomousStopped`. It is a net improvement over today (today it comes
  with a row and a FAILURE notification), and it is not durable: `serverCtx` is cancelled by
  `Shutdown()` (`server/server.go:194`, `:1087`), so the process is exiting, and
  `AutonomousOutcome` is not persisted (`session/storage.go` has no such field), so the badge is
  gone at the next boot. A connected UI may show it for the length of the teardown. The alternative
  (set outcome `""` for the flag-OFF shutdown) would stop being "today's in-memory outcome" and
  breaks nothing, but it would also make the flag change an outcome the owner did not ask to
  change; kept as decided, with the cost stated here.
- **Ungated behaviour changes**: typed reasons/logging, `cancelled_external` taking no
  respawn/stuck row/notification, `cancelled_shutdown` taking no respawn/stuck row/notification
  (regardless of the readoption flag), and `working -> Resupervise` for work-role backlog-linked
  sessions (bounded, then Respawn = today's behaviour). These are corrections, not features;
  rollback is revert of the PR (the table is one function). **Revert-only means a redeploy, and
  `make install-service` kills the tmux server and every live session with it unless the deployed
  unit passes `--tmux-keep-server`** (`docs/explanation/tmux-keep-server-on-restart.md`; the
  `CLAUDE.md` WARNING). So a revert is a last resort taken between backlog bursts, and the
  deployed unit must be checked for `--tmux-keep-server` before any rollback; a live-settable
  Resupervise kill switch was considered and deferred (see Deferred, pre-mortem #11).
- **Rollback procedure**: in-process nudge off = flag `autonomous_stall_nudge_enabled` off (or
  `diagnose_nudge_enabled` off, which also stops the diagnose agent's nudges): today's respawn for
  every idle-prompt stall; readoption off = flag `autonomous_driver_readoption` off (today's
  in-memory shutdown mode/outcome only, see above); everything else = revert commit (see the
  tmux warning above). No data to unwind.
- **Staged rollout**: full rollout on merge with **both new flags OFF**: the nudge is dark (the
  stopped log line's `nudge_outcome=disabled` counts what it would have caught) and re-adoption is
  a dry-run. Order: (1) deploy; (2) read the `readopt candidate` dry-run lines once and the
  `nudge_outcome=disabled` count, flip `autonomous_driver_readoption` and/or
  `autonomous_stall_nudge_enabled` live per the owner's reading; (3) review at the soak date
  (Observability Plan, exit criteria).

- **Risky assumptions (named so they can be falsified, thresholds in the Observability Plan).**
  (i) A fixed "call request_review" nudge unsticks an idle-at-prompt work session often enough to
  matter; with a 2-slot lifetime budget the upside is bounded. *Falsified by*: nudge resolution
  rate below 30% over >= 10 eligible stalls, or fewer than 10 eligible stalls in two 14-day
  windows. *Action*: flag off, delete the Nudge branch. (ii) The pane classifier (`working` vs
  `idle_prompt`) is reliable from a bounded tail. *Falsified by*: `state=working` stalls whose next
  stop is `idle_prompt` within one warm-up, or a nudge landing in a working pane (`not_idle`
  share above 30% of eligible stalls). *Action*: tighten `CheckNudgeEligible`'s settle window or
  drop row 5. (iii) Re-attaching a driver to a working pane beats today's immediate respawn.
  *Falsified by*: re-supervise completion rate below 30% over >= 10 stops. *Action*: row 5 returns
  `respawn`. The typed reasons, snapshot, redaction and the log line have value independent of all
  three assumptions and are never part of a delete.

## Unresolved Questions

Defaults are decided in this plan so implementation is not blocked; each is a one-row or
one-constant change if the owner disagrees.

- [ ] `working -> Resupervise` (cap 3, then Respawn) for stall reasons other than `max_turns`, in the
      work arm of backlog-linked sessions only (ADR-001 rows 5-6). Today a working pane is
      respawned. Default: never kill an actively working pane on the first stops, never leave it
      driverless. `max_turns` stays on today's Respawn path so the rework cap and `RemediationDue`
      backoff keep gating turn-budget exhaustion. Review, triage, unrecognized-role and unlinked
      sessions keep today's handling (no nudge, no re-supervise). — Story 1.1.1 table, 3.2.4 — owner: tstapler
- [ ] `needs_approval` / `error` / `idle` (shell prompt) / `unknown` stalls keep today's Respawn
      rather than None. Human notification still fires with the typed state. — none blocks — owner: tstapler
- [ ] Nudge cap is a lifetime counter on the `(item, autonomous_stuck)` row: neither `MarkStuck`
      reopen nor `ResetStuckRemediation` resets `diagnose_nudge_count` (verified above). With
      cap 3 and one slot reserved for the diagnose agent, an item gets 2 deterministic nudges for
      life, then respawn-only. Whether "Reset" should also clear it is out of scope. — none blocks — owner: tstapler
- [ ] Re-supervision startup timeout: default 5 minutes, the value triage already uses
      (`backlog_service_triage.go:3341`). A re-supervised session still busy past it ends again as
      `startup_timeout`/`working` and is re-supervised until the cap (3), then Respawn. — Story 3.2.4 — owner: implementer
- [ ] Resupervise counter and `stallRecoveryGuard` are in memory; a restart resets both. Bounded
      by process lifetime. — none blocks — owner: tstapler
- [ ] Boot re-adoption only adopts **backlog-linked** instances whose ItemSession is open
      (the owner's gate list). A user-toggled autonomous session with no backlog item is not
      re-adopted; its stale mode is cleared at boot, which is today's outcome. Widening to
      unlinked sessions is one gate (`item_session_open` treating `ErrNotFound` as pass) plus a
      role-less path. — Task 3.1.3c — owner: tstapler
- [ ] Re-adoption per-boot cap defaults to 8 drivers per pass (a start-storm bound). The excess is
      **deferred**, not cleared: candidates are ordered by `CreatedAt` ascending (ties by instance
      UUID) and the rest are re-evaluated by up to 3 follow-up passes 60 s apart; only candidates
      still over the cap after the last pass are cleared (quietly) with a Warn that names the count.
      — Task 3.1.3c — owner: implementer
- [ ] `max_turns` stays on today's Respawn path (row 4) even when the pane is idle or working, so
      a nudge or re-supervise never hands a fresh turn budget around the rework cap. `panic` joins
      row 4 (Respawn): a deterministic panic must not be re-supervised three times. — ADR-001 row 4 — owner: tstapler
- [ ] The in-process nudge has its own flag, `autonomous_stall_nudge_enabled`, default off, AND-ed
      with `diagnose_nudge_enabled` (decided above; one constant and one registry entry if the
      owner prefers an independent flag). — Risk Control — owner: tstapler
- [ ] Nudge / Re-supervise require the item to still be `in_progress`; any other status falls to
      the Respawn row, whose `AutoRespawnAutonomousWork` re-checks `in_progress` itself, i.e.
      today's handling. — ADR-001 rows 5, 7 — owner: implementer

## Deferred / accepted concerns

Concerns from the adversarial and architecture reviews that are not fully fixed in this plan, each
with the reason. Nothing here removes an acceptance criterion.

**Decided: `autonomous_driver_readoption` ships default OFF (owner decision, adversarial C4).**
Boot re-adoption (Story 3.1.3) stays in scope and is gated by the full list in Task 3.1.3c. It is
the largest remaining risk surface, so the first boot is a dry-run: the `would_adopt` /
rejecting-gate log is gate-validation evidence read *before* anything is adopted, and the owner
flips the flag live afterwards. OFF is safe: it clears stale mode at boot, which is today's
outcome. The default lives in one constant, `autonomousDriverReadoptionDefault` in
`config/config.go` (value `false`; read by the flag registry entry and
`EffectiveAutonomousDriverReadoption`); no other task, test or doc depends on the value (tests
set the flag explicitly).

**AC3 scope (decided against the AC text).** The backlog item's AC3 reads: "Before a session is
restarted, its prior state (last activity and output) is inspected and captured, and the restart
path can choose a nudge over a full respawn when the session is only waiting." It does **not**
name the stale-work detector or the health checker, and the item description scopes the work to
"the current recovery path" that "restarts sessions without first inspecting what the session was
doing", which is the driver-stop recovery in `onAutonomousDriverComplete`. AC3 is therefore met
for **driver-stop paths**. The stale-work detector (`RemediateStaleWorkSession`) and health-checker
restarts still kill and respawn with no pane snapshot and no nudge. This is recorded as a
**follow-up backlog item** (row Adv C1 below), not as a silent omission: this plan adds only the
deferral guard (Story 3.2.5) so the stale detector cannot kill a session this plan is nudging or
re-supervising, and the diagnosis document lists those restart paths under "known gaps".

| Concern | Disposition | Reason |
|---------|-------------|--------|
| Adv C1 (round 1): AC3 only covers the driver-completion path; `RemediateStaleWorkSession` and health-checker restarts still kill/respawn with no snapshot or nudge | **Deferred as a follow-up item (AC text does not name these paths)** | AC3's text names neither the stale detector nor the health checker (re-read from the backlog item), and the description scopes the work to the driver-stop recovery path. Those paths have no driver snapshot and are a second restart surface; routing them through `DecideStallAction` needs pane capture in `BacklogService`. Follow-up spec: reuse `ClassifyObserved`/`DecideStallAction` and the nudger from `RemediateStaleWorkSession` and `handleDeadPane`/`respawnDeadPane` (`session/health.go`), capturing a `StopSnapshot` before `UpdateItemSessionEnded`/`KillTmuxPaneOnly`. The plan only adds the deferral guard (3.2.5); the diagnosis doc lists the others as known gaps. |
| Adv C5 (round 1) / architecture: nudge on `orchestrator_error` with a failing LLM pool is wasted | **Accepted** | Bounded by nudge cap 2 + re-supervise cap 3; recorded in ADR-001 Consequences. Gating on pool health needs a health signal that does not exist. |
| Adv C8 (round 1): nudge runs synchronously inside the driver goroutine's callback | **Accepted, with tests** | The run is cancellable through the `recovery` entry (created in `claimCompletion`, so no registration gap) and the settle wait is ctx-aware (Task 3.2.2a). |
| Adv C10a: 21 existing tests may not cover `RemediationDue`/`justParked` branches | **Folded in**: Task 3.1.1a adds characterization tests first | |
| Adv minor: lifetime nudge cap (time-windowed cap) | **Deferred** | Needs a schema/semantic change to the stuck row; listed under Unresolved Questions. |
| Architecture: `writeNudge` (MCP) and the autonomous nudge are two implementations of verify/lease/cap/write | **Accepted, recorded** | Extracting a shared `NudgeGuarded` would touch the MCP handler and its orderings. Both are pinned; the autonomous path differs on purpose (atomic claim, no refund). Revisit after the soak. |
| Architecture nit: `StopDetail` is an unparsed string | **Accepted** | Nothing parses it; the tests assert prefix keys. |
| Architecture nit: `SessionDriver.attemptBacklogNudge` is a second automated nudger | **Folded in** as one sentence in Task 1.2.1b | |
| Adv minor: shutdown stop capture adds up to 2s per stop | **Folded in**: capture is skipped for `cancelled_shutdown` and `cancelled_external` (Task 2.1.1a) | |
| Repair-2 B5 (boot re-adoption under-gated) | **Fixed in scope** (Task 3.1.3c, 3.1.3f, 3.1.3d) | Twelve-gate conjunction, per-gate result log, dry-run when the flag is off, ghost-mode clear on definitive rejection, per-boot cap, table test over every gate row and a persisted round-trip test. |
| Repair-2 Adv C2 (flag off leaves mode true with no driver) | **Fixed** | Defined in Risk Control: flag off restores today's in-memory mode/outcome in the shutdown branch and clears stale mode at boot. |
| Round-3 Adv C1 (clear-on-any-rejection conflated reject / skip / unknown; contradicted the run-twice AC) | **Fixed** (Story 3.1.3 classification table, Task 3.1.3c/d) | Three result classes; only a definitive reject clears mode; `no_driver` / `ErrDriverPresent` is a skip, a probe timeout or non-`ErrNotFound` storage error is unknown (left untouched, re-evaluated). AC4 gains the skip-without-clear and transient-probe-error rows. |
| Round-3 Adv C2 (boot-cleared candidates showed a false "stuck" badge) | **Fixed** (Task 3.1.2a, 3.1.3c) | Boot pass uses `SetAutonomousStoppedQuiet` (outcome `""`); `"stuck"` stays only on the flag-OFF shutdown callback branch. |
| Round-3 Adv C3 (pass trigger preceded the controller settle; no fallback) | **Fixed** (Task 3.1.3f) | Signal (b) fires after Step 7.6's 500 ms settle and scan (`server/dependencies.go:1084-1088`, same goroutine as Steps 7 and 7.5), via `defer` so every exit signals, plus a bounded fallback timer. |
| Round-3 Adv C5 (submit-uncertain -> Resupervise writes into a pane with unsubmitted text) | **Fixed** (failure table, Story 3.2.3) | Always leave-the-pane; ctx-cancel row takes precedence. |
| Round-3 Adv C6 (per-boot cap cleared the excess in unspecified order) | **Fixed** (Unresolved Questions, Task 3.1.3c, scheduling in 3.1.3f) | Stable order, excess deferred to bounded follow-up passes. |
| Round-3 Adv C7 (Risk Control overstated flag-OFF equivalence) | **Fixed** (Risk Control) | States exactly what OFF restores and what stays an ungated correction. |
| Round-3 Adv C4 (default ON removes gate-validation evidence) | **Resolved: default OFF** (owner decision) | See "Decided" paragraph above; single constant `autonomousDriverReadoptionDefault = false`. |
| Repair-2 B6 (`cancelled_*` falls into Done-reading arms) | **Fixed** (Task 3.1.3b) | Per-arm guard at the top of every arm, tests for every role including unlinked. |
| Repair-2 Adv C1 (pause and user stop never stop the driver, `session_service_update.go:99-110`) | **Accepted limitation, claim removed** | Story 3.1.3 does not claim pause is covered. A paused/stopped session's pane dies, the driver exits `exited`/`crashed` and takes today's Respawn row; the respawner's own guards decide. `canRestart` and the nudge writer refuse any instance whose status is not `Active`, so a pause during a recovery cannot be undone by it. The boot pass refuses `Paused`/`Stopped` too (gate `status_active`). Stopping the driver on pause is a one-line `stopAndDeregisterDriver` call in `applyStatusTransition` that belongs in its own item. |
| Repair-2 Adv C5 (`ReleaseDiagnoseNudgeSlot` refund erases a concurrent MCP nudge's cooldown) | **Refund dropped, justified** | The claim runs after the gate and the lease, immediately before the write, so only a failing `SubmitDriverContent` can burn a slot, and that failure may have delivered text (`ErrSubmitNotConfirmed`), so a refund would also under-count a real write. Cost: at most one slot of a 2-slot lifetime budget lost on a rare submit failure; the item then leaves the pane per the failure table. No second UPDATE, no owner-aware predicate. |
| Repair-2 Arch C2 (cancelled ctx does not return promptly) | **Fixed** (Task 3.2.2a) | `CheckNudgeEligibleWithSettleWindowCtx` selects on `ctx.Done()` and the poll timer; `CheckNudgeEligible` (MCP path) delegates with `context.Background()`, unchanged. Fake-nudger service tests cannot prove promptness or no-write; the session-level test in 3.2.2a does. Residual: a stop in the microseconds between the last ctx check and the first `SendKeys` writes one fixed-text nudge to an idle pane. |
| Resupervise multiplies the turn budget (Adv C6) | **Accepted, made traceable** | Up to (1 + 3) x `AutonomousMaxTurnsOrDefault` per ItemSession before Respawn; `max_turns` itself never re-supervises (row 4). Every stop carries `resupervise=n/cap` and the cap-reached row carries `last_stop_reason`. |
| `resuperviseBudget` not refreshed by progress (arch nit) | **Accepted** | A long legitimately busy session is respawned after about 16 minutes of continuous `working` stops (60 s + 3 x 5 min); today it is respawned on the first. Entries are deleted on `done` and on ItemSession end (adversarial M2). |
| UX G2: flag-OFF `cancelled_shutdown` keeps the in-memory `stuck` badge | **Accepted, with evidence** | Row, notification and respawn are already suppressed regardless of the flag; the badge is in-memory, in an exiting process, and `AutonomousOutcome` is not persisted. Full statement in Risk Control, "What an operator sees on a graceful restart". |
| UX G3: Resupervise and `cancelled_*` are log-only | **Accepted** (ADR-002 table) | A Resupervise is bounded (about 16 minutes) and its end is always visible (next stop or the cap-reached row); a cancel is the operator's own action, so a notification about it is noise. No extra cue is added; the cost is that a mid-chain Resupervise needs the log line to see. |
| Engineering G5: redaction residuals | **Mitigated fail-closed, residual stated** (Task 1.1.2b/c) | `redactErrorDetail`'s key/value alternative requires `\b` after `token\|secret\|password\|authorization` and `api[_-]?key`, so `AWS_SECRET_ACCESS_KEY=...` and `client_secret_key=...` are not matched (checked by running the pattern in Python `re`, which shares the `\b` and alternation semantics: 0 matches on both; Task 1.1.2c re-proves it with Go's `regexp`), and a 40-line tail can begin inside a key body (no `BEGIN` header). Added as string scans, not regexes: withhold the whole tail on an env-style `NAME=` whose name has a `SECRET`/`TOKEN`/`PASSWORD`/`KEY`/`CREDENTIAL` segment, or on a line of >= 40 base64-alphabet characters with no spaces. **Accepted residual:** a short secret in free prose with no marker (`the password is hunter2`), or a credential split across the 40-line cut. The tail stays opt-out-able by the owner as one function. |
| Engineering G3: BUG-079 (`TestAutonomousDriver_Stop_CancelsLoop_DuringNudgeSuppression` full-suite race) | **Declared known flake with evidence; baseline run added to Task 2.1.1c** | `docs/bugs/open/BUG-079-autonomousdriver-idle-timer-race-under-full-suite.md` is `Open`, unreproduced, cause UNVERIFIED. Its suspect (a package-level idle-settle timer variable shared across tests) is not in the code at this HEAD: the driver's timers are per-driver struct fields (`autonomous_driver.go:101,145`) and the only package-level timer var, `nudgeCooldown` (`session/nudge_dedup.go:13`), is read but never written by any test (`grep -rn nudgeCooldown session/`). A second, unrelated BUG-079 file exists for Jest (`...jest-full-suite-only-failures...`); cite the driver one. Task 2.1.1c: baseline `-race -count` run before adding tests, join every new driver in `t.Cleanup`, and if the race reproduces, fix it as its own commit (standing fix-collateral-debt policy) with the race trace pasted into the bug doc. |
| Engineering G4: BUG-087 (`captureLogs` global slog swap) | **Mitigated in the new assertions** (Task 3.1.4c) | At HEAD `captureLogs` holds `slogDefaultMu` (`autonomous_orchestration_service_test.go:445-455`), which serializes capturers but not emitters: a parallel test that logs without capturing can still write into the swapped default, which is the exact repro in BUG-087 (an unrelated go-git WARN). New assertions therefore filter records by a unique `session=<title>` and never count unfiltered lines. Not a fix of BUG-087 itself. |
| Engineering G7: instance setters discard `sendSyncErr` | **Fixed for the new setters only** (Task 3.1.2a/b) | The new setters return the error and `applyInstanceStop` logs it and skips the event; the existing `_ = i.sendSyncErr` setters are out of scope (what was asked) and listed here so the pattern is visible. |

### Follow-up backlog item (spec, ready to file; not filed by this plan)

AC3 is met for driver-stop paths only (above). The stale-work detector and health-checker
restarts are the remaining restart surfaces. This block is the item body; the shipping agent files
it at ship time and pastes the item ID into the PR description (Task 4.1.1b checklist). Until an ID
is recorded here, this block is the only record, so it is part of the plan on purpose.

```
Title:   Inspect and nudge before restarting from the stale-work detector and health checker
Source:  autonomous-stall-diagnosis (AC3 scope decision), Deferred Adv C1
Problem: RemediateStaleWorkSession (server/services/backlog_service_triage.go) and
         handleDeadPane/respawnDeadPane (session/health.go) end the ItemSession, kill the pane and
         respawn with no StopSnapshot and no nudge, so an idle-at-prompt session is killed after 2h
         without anyone looking at what it was doing.
Scope:   Reuse domain.ClassifyObserved / DecideStallAction and the StallNudger from both paths;
         capture a StopSnapshot (BoundedStopTail + sanitizeStopTail) before UpdateItemSessionEnded /
         KillTmuxPaneOnly; record stop_reason (new values stale_work, dead_pane) in the stuck row
         context with the same plain-language lead sentence.
Out:     Changing the 2h stale threshold; pause/stop driving the driver (Deferred Adv C1 repair-2).
AC:      (1) A stale item whose pane is at an idle prompt, nudge allowance left: one nudge, no
         respawn, row annotated; (2) a dead pane is never nudged; (3) a snapshot is captured and
         logged before every restart on both paths; (4) table tests per ObservedState x path;
         (5) stallRecoveryGuard still defers the detector while a driver-stop recovery is live.
Deps:    autonomous-stall-diagnosis merged (domain types, StallNudger, claim, guard, copy table).
Size:    M-L (two entry points, one shared effect); gated by the nudge resolution rate from the soak.
```

## Pre-mortem disposition

Source: `implementation/pre-mortem.md` (2 P1, 9 P2, 5 P3), each claim re-checked against the code
at HEAD before patching (corrections are recorded in the pre-mortem). Iteration 4 added no task;
every fix was folded into an existing task's ACs and each new test has an ID in `validation.md`.
Task ids in this table keep the iteration-4 names; the later 3.1.3 split is in "Triad repair
disposition" below.

| # | Sev | Disposition | Where |
|---|-----|-------------|-------|
| 1 | P1 | **Fixed.** `lifecycleCtx` is an atomic pointer never read under `a.mu`; reserve-then-start with a `starting` sentinel, driver built and started outside the lock; `canRestart`/`canRestartLocked`; the `a.mu` rule (no seam, storage, bus or `Instance` call under the lock); watchdog tests that fail instead of hang | Glossary, Pattern Decisions, 3.1.3a, 3.1.3e (atomic ctx), 3.1.3c, 3.1.3f, 3.1.3d (REG-10, REG-12..REG-15) |
| 2 | P1 | **Fixed.** Dedicated flag `autonomous_stall_nudge_enabled`, default off, AND-ed with `diagnose_nudge_enabled` (kept as the autonomous-write kill switch) | Glossary, Risk Control, Unresolved Questions, Wall-clock blockers, 3.2.3a/b/d (STL-7, STL-11, STL-19) |
| 3 | P2 | **Fixed.** `ItemInWorkStatus` input; non-`in_progress` item never Nudges or Resupervises | ADR-001 rows 5-7, 1.1.1, 3.2.3b (DOM-4/5, STL-18) |
| 4 | P2 | **Fixed.** `MarkStuck` `(false, err)` fails open to Respawn; `(false, nil)` maps to `None` | 3.2.3b (STL-17) |
| 5 | P2 | **Fixed in part.** Third start site inventoried and routed; panic-recover wrapper around the callback; `Finished()` liveness check. Steady-state ghost-mode sweeper **deferred** (below) | 3.1.3a (liveness), 3.1.3e (wrapper, third start site), 3.1.3d, 2.1.1a (STL-15, REG-16) |
| 6 | P2 | **Fixed.** Recovery ctx reaches only the nudger and the restart check; storage writes stay on `Background()` | Glossary `completion`, 3.2.3b (STL-16) |
| 7 | P2 | **Fixed.** One `applyInstanceStop` (write + event) at plan time, not a function-exit defer; characterization rows for event payload and `MarkStuck` count | 3.1.1a, 3.1.2b, 3.2.3b (SVC-11, SVC-12, STL-12) |
| 8 | P2 | **Decided + partly folded.** Default OFF (owner decision). Adoption stamps `stallRecoveryGuard`. Reduced adopted turn budget and a `stale` gate **deferred** (below) | 3.1.3c (RDP-16) |
| 9 | P2 | **Deferred with reason** (below); signature is a soak exit criterion | Observability Plan |
| 10 | P2 | **Fixed.** `nudge_outcome` / `resupervise_cap_hit` log keys, pinned query, review date, decision rules | Observability Plan, 3.1.4a, 3.2.3b (OBS-2) |
| 11 | P2 | **Noted.** Revert-only and the `--tmux-keep-server` hazard stated; Resupervise kill switch **deferred** (below) | Risk Control |
| 12 | P3 | **Folded.** Stamp placed at plan time and shortened on every non-driver exit; startup wiring assertion | Glossary, 3.2.3c, 3.2.5a (DEF-5) |
| 13 | P3 | **Folded.** `panic` joins row 4 (Respawn); never Nudge/Resupervise | ADR-001 row 4, 1.1.1 (DOM-4/5, RSP-6) |
| 14 | P3 | **Deferred** (below) | |
| 15 | P3 | **Deferred** (below) | |
| 16 | P3 | **Pre-mortem claim corrected**: `DiagnosticBundleInput` has no stuck-context field (`session/diagnose_bundle.go:93-105`), so nothing ingests the tail; one sentence in ADR-002 | ADR-002 |

Deferred from the pre-mortem, each with its reason:

| Concern | Disposition | Reason |
|---------|-------------|--------|
| #5 steady-state ghost-mode sweeper (periodic `readoptGate` pass) | **Deferred** | A periodic sweep over live instances is a new subsystem with its own clock, cap and failure modes. The known sources of mode-true-without-driver are closed individually (panic wrapper, `Finished()` liveness, third start site, boot pass); a failure shows as a `readopt`-style log gap. The follow-up reuses `readoptGates`. |
| #8 adopted drivers inherit a reduced turn budget; a `stale` gate (`LastProgressAt` older than the stale threshold -> reject) | **Deferred** | The flag ships OFF, so no production session is adopted until the owner has read the dry-run log; both changes alter the 12-gate list and RDP-1's 28 rows for a path that is not live. Decide with the first dry-run in hand (the log already carries last-activity age). |
| #9 pool-outage guard: working + exhausted Resupervise -> `None` + one notification instead of Respawn | **Deferred** | `None` on a working pane leaves it driverless with approvals unhandled, the exact outcome adversarial B1 removed from row 5. Respawn is today's behaviour on the **first** stop; the plan only delays it by about 16 minutes, so it is strictly no worse. The burst signature is a soak exit criterion, and the guard is added if it appears. |
| #11 live-settable Resupervise kill switch | **Deferred** | Resupervise is bounded (3) and falls to today's Respawn; a flag is another registry entry, test row and rollout state for a behaviour that is already the conservative one. Revisit if the soak shows a regression. |
| #14 superseded `done` dropped wholesale | **Deferred**, Warn strengthened | A displaced run completing `done` requires the new driver to start while the old one is still reading DONE; the displacing path cancels the old run, which then reports `cancelled_*`. `claimCompletion` logs a Warn naming `done` and the dropped item (REG-1), so it is findable. |
| #15 double nudge: successor driver has no `lastSentNudge` | **Deferred** | Needs a new driver option and run-local state plumbing in `autonomous_driver.go` for a 3-message edge; the diagnosis document states the "both active?" answer (Task 1.2.1b). |

## Triad repair disposition

Source: the three `pm:triad-review` analyses (UX, engineering, product) of the plan at
`db5083502`. Each claim was re-verified against the code before patching; corrections to the
reviews are marked. Three tasks were added (35 -> 38); no scope was removed.

| Gap | Disposition | Where |
|-----|-------------|-------|
| UX G1 chosen action not recorded; row state after a nudge unspecified | **Fixed.** `action=` and `nudge=n/cap` in the row; Nudge plans refresh the row once; row **annotated, not resolved** (it holds the nudge counter) | Observability Plan, 3.1.4a, 3.2.3b/c, STL-20 |
| UX G2 flag-OFF shutdown shows stuck | **Review corrected + accepted.** Row, notification and respawn are already suppressed for shutdown regardless of the flag (3.1.3b); what remains is an in-memory badge in an exiting process, not persisted | Risk Control, Deferred |
| UX G3 Resupervise/cancel log-only | **Accepted** with reasons | ADR-002, Deferred |
| UX G4 internal codes in notification | **Fixed.** Class to phrase map in `stopCopy`; code stays in the log | Operator-facing copy, 3.1.4b, 3.2.3c, STL-21 |
| UX G5 no copy table | **Fixed.** Full table for owner review plus golden tests | Operator-facing copy, 3.1.4c, OBS-10 |
| UX G6 "Why" readability/overflow | **Fixed.** Plain lead sentence (server) + `overflow-wrap: anywhere` on `.value` (CSS only); verified `.value` has only `color` | 3.1.4d |
| Eng G1 split 3.1.3a / 3.1.3c | **Fixed.** 3.1.3a + 3.1.3e, 3.1.3c + 3.1.3f; waves recomputed (15) | Dependency Visualization |
| Eng G2 W5 file collision | **Fixed.** `SetLifecycleContext` moves to the registry file in 3.1.3e; 3.1.3c/f never edit the service file | 3.1.3e, 3.1.3f |
| Eng G3 BUG-079 | **Handled.** Baseline run, driver join hygiene, own-commit fix if reproduced, else declared known flake. Review corrected: the bug's suspected package-level timer is not in the code at HEAD | 2.1.1c, Deferred |
| Eng G4 BUG-087 | **Handled.** Session-key filtered assertions; runtime growth called out. Review corrected: `captureLogs` now serializes capturers via `slogDefaultMu` but not emitters | 3.1.4c, 4.1.1b |
| Eng G5 redaction residuals | **Fixed fail-closed + residual stated** (string scans, no new regex) | 1.1.2b/c, Deferred |
| Eng G6 stale 3.2.4a wording | **Fixed** | 3.2.4a |
| Eng G7 setter errors discarded | **Fixed for the new setters** + test INS-6 | 3.1.2a/b |
| Eng G8 Story 3.2.3 Files line | **Fixed** (per-task file list) | Story 3.2.3 |
| Product: risky assumption, threshold, soak target, dates, follow-up spec, hypothesis in requirements | **Fixed** | Risk Control, Observability Plan, Deferred, requirements.md |

## Dependency Visualization

Edges are "must land before". Every task in a wave has all of its predecessors in earlier waves;
no two tasks in one wave edit the same file (checked against each task's Files list).

```
1.1.1a ─> 1.1.1b ─> 1.1.1c
1.1.1a ─┬─> 2.1.1a ─> 2.1.1b ─> 2.1.1c
1.1.2a ─┘
1.1.2a ─> 1.1.2b ─> 1.1.2c
1.2.1a ─> 1.2.1b ─────────────────────────────────────────────> 4.1.1a ─> 4.1.1b
3.1.1a ─┐
1.1.1a ─> 3.1.2a ─┤
2.1.1a ───────────┴─> 3.1.2b ─> 3.1.3a ─> 3.1.3e ─> 3.1.3b ─┬─> 3.1.3c ─> 3.1.3f ─> 3.1.3d
                                                            └─> 3.1.4a ─> 3.1.4b ─> 3.1.4c
3.1.4d (CSS-only UI hardening; no predecessor, no successor)
3.2.1a ─┐
3.2.2a ─> 3.2.2b ─┤
3.2.5a ─> 3.2.5b ─┤                      3.1.4b + 3.1.3a ─> 3.2.3a ─> 3.2.3b ─> 3.2.3c ─> 3.2.3d
                  └─────────────────────────────────────────────┘ (3.2.3b needs 3.2.1a, 3.2.5a;
                                                                    3.2.3c needs 3.2.2a, 3.2.5a)
3.2.3c ─> 3.2.4a ─> 3.2.4b
3.2.5b + 3.2.3c + 3.2.4a ─> 3.2.5c
Shared-file serial spine (all edit autonomous_orchestration_service.go):
  3.1.1a -> 3.1.2b -> 3.1.3a -> 3.1.3e -> 3.1.3b -> 3.1.4a -> 3.1.4b -> 3.2.3a -> 3.2.3b
  (3.2.3c and 3.2.4a also edit it but are later; 3.1.3c and 3.1.3f do NOT edit it: SetLifecycleContext
  moves to the registry file in 3.1.3e, which is what removed the former W5 collision.)
New source files per lane (no cross-lane edits): readoption = autonomous_readoption.go (gates, 3.1.3c)
  and autonomous_readoption_trigger.go (trigger, 3.1.3f);
  nudge effect = autonomous_stall_nudge_effect.go; re-supervise = autonomous_stall_resupervise.go;
  guard = stall_recovery_guard.go. Test files are likewise one per task (see each task).
```

Waves (agent waves, 15): W0 `1.1.1a` `1.1.2a` `1.2.1a` `3.1.1a` `3.1.4d` `3.2.1a` `3.2.2a` `3.2.5a` ->
W1 `1.1.1b` `1.1.2b` `1.2.1b` `2.1.1a` `3.1.2a` `3.2.2b` `3.2.5b` -> W2 `1.1.1c` `1.1.2c`
`2.1.1b` `3.1.2b` -> W3 `2.1.1c` `3.1.3a` -> W4 `3.1.3e` -> W5 `3.1.3b` -> W6 `3.1.3c` `3.1.4a` ->
W7 `3.1.3f` `3.1.4b` -> W8 `3.1.3d` `3.1.4c` `3.2.3a` -> W9 `3.2.3b` -> W10 `3.2.3c` -> W11
`3.2.3d` `3.2.4a` -> W12 `3.2.4b` `3.2.5c` -> W13 `4.1.1a` -> W14 `4.1.1b`.
Same-wave file check, re-derived for the changed waves: W6 `3.1.3c` (autonomous_readoption.go and
its test) vs `3.1.4a` (autonomous_stop_detail.go, service.go); W7 `3.1.3f` (readoption_trigger.go,
registry.go, session_service_wiring.go, dependencies.go, server.go) vs `3.1.4b` (stop_detail.go, service.go); W8 `3.1.3d`
(three `*_test.go` files), `3.1.4c` (stop_detail_test.go, service_stop_test.go), `3.2.3a`
(stall_recovery.go, service.go, config.go, pins_test.go): no shared file. `3.1.4d` in W0 touches
only web-app and e2e files.

---

## Phase 1: Foundations (no behaviour change)

### Epic 1.1: Pure stop classification and decision
**Goal**: Typed, dependency-free vocabulary and the nudge/respawn/none decision, fully table-tested before any wiring exists.

#### Story 1.1.1: Typed stop reasons, observed state and stall decision
**As a** maintainer, **I want** the stop reason, the pane's state and the recovery choice to be pure typed values, **so that** every state's decision is unit-tested and no free text decides recovery.
**Acceptance Criteria** (AC2, AC3, AC4):
- `StopReason` covers done, max-turns, startup-timeout, orchestrator error, submit failure, lease timeout, rate-limit exhausted, cancelled (external), cancelled (shutdown), panic; `Class()` is `Done` for `done`, `Cancelled` for `cancelled_external`/`cancelled_shutdown` and `Stall` for everything else; `IsStall()` is `Class() == Stall`. `AutonomousDriverOutcome.IsDone()/IsStall()/Class()` are the only accessors decision code uses.
  - *Given* `StopReasonCancelledExternal` or `StopReasonCancelledShutdown`, *When* `IsStall()` is called, *Then* it returns `false` and `Class()` is `Cancelled`; *Given* `StopReason("")` (a legacy outcome from an old test), *Then* `IsStall()` returns `true`.
  - *Given* a legacy literal `AutonomousDriverOutcome{Done: true, StopReason: ""}`, *When* normalised, *Then* `StopReason == done`; *Given* `{Done: false, Stuck: false, StopReason: ""}`, *Then* a stall (legacy tests relied on `Done == false` meaning stuck). No `StopReason` value can yield `IsDone() && IsStall()` (invariant test over every value).
- `ClassifyObserved` separates awaiting-input, error and exited.
  - *Given* a `StopSnapshot{Bucket: safe_idle, TmuxAlive: true, PaneDead: false}`, *When* `ClassifyObserved` runs, *Then* `idle_prompt`; *Given* `{Bucket: active, TmuxAlive: true, PaneDead: true, ExitCode: 1}`, *Then* `crashed` (liveness wins over bucket).
- `DecideStallAction` implements the ADR-001 table.
  - *Given* `{Reason: orchestrator_error, State: idle_prompt, NudgeEnabled: true, NudgeAllowed: true, ItemInWorkStatus: true}`, *When* decided, *Then* `nudge`; *Given* the same with `State: needs_approval`, *Then* `respawn`; *Given* the same with `Archived: true`, *Then* `none`; *Given* `Reason: cancelled_external` or `cancelled_shutdown`, *Then* `none` for every state; *Given* `Reason: max_turns` with `State: idle_prompt` and nudge enabled and allowed, *Then* `respawn` (never `nudge`/`resupervise`); *Given* `Reason: panic` with `State: idle_prompt` or `working`, headroom and flags on, *Then* `respawn` (a deterministic panic is never re-supervised or nudged).
  - *Given* `{Reason: startup_timeout, State: working, ResuperviseAllowed: true, ItemInWorkStatus: true}`, *Then* `resupervise`; *Given* the same with `ResuperviseAllowed: false`, *Then* `respawn` (never `none`).
  - *Given* `ItemInWorkStatus: false` (item already in `review`), `State: idle_prompt` or `working`, flags on and headroom, *Then* `respawn` (never `nudge`/`resupervise`: after `request_review` the pane idles by design).
**Files**: `session/domain/autonomous_stop.go`, `session/domain/autonomous_stop_test.go`

##### Task 1.1.1a: Types and constants (XS) — AC2, AC3
- Add `StopReason` (+ `IsStall`, `IsValid`, `String`), `ObservedState`, `StatusBucket`, `StopSnapshot`, `StallAction`, `StallDecisionInput` (six bools incl. `ItemInWorkStatus`). Plain strings only; no imports beyond `time`. `AutonomousDriverOutcome` derives `Stuck`/`Done` from `StopReason` through methods; the struct fields stay only as legacy input (Task 2.1.1a).
- Files: `session/domain/autonomous_stop.go`

##### Task 1.1.1b: `ClassifyObserved` and `DecideStallAction` (S) — AC3
- Classification precedence: not-alive/`PaneDead` (exit 0 -> `exited`, else `crashed`) > no controller -> `exited` > bucket mapping (`safe_idle`->`idle_prompt`, `needs_input`->`needs_approval`, `error`->`error`, `active`->`working`, `idle`->`idle`, else `unknown`).
- Decision: ADR-001 rows 1-8 in order, first match wins (row 1 = `Class() != Stall`; row 4 = `max_turns` or `panic` -> Respawn; rows 5 and 7 additionally require `ItemInWorkStatus`, and row 6 covers "working and (headroom exhausted or item not in work status)").
- Files: `session/domain/autonomous_stop.go`

##### Task 1.1.1c: Table-driven tests (S) — AC4
- `TestClassifyObserved_*`: bucket x alive x paneDead x exitCode x controller table.
- `TestDecideStallAction_*`: named rule cases per ADR row, plus an exhaustive loop over every `StopReason` x every `ObservedState` x 2^6 booleans (the count is derived from the enum slices, not hard-coded) asserting the invariants: *`nudge` iff `IsStall` ∧ reason∉{`max_turns`,`panic`} ∧ ¬archived ∧ ¬retryPending ∧ itemInWorkStatus ∧ state=`idle_prompt` ∧ enabled ∧ allowed*; *`resupervise` only when reason∉{`max_turns`,`panic`} ∧ itemInWorkStatus ∧ state=`working` ∧ allowed*; no `ObservedState` outside `idle_prompt` ever yields `nudge`; no `Class() != Stall` reason ever yields anything but `none`; no stall that is not archived/retry-pending yields `none`.
- `t.Parallel()`, no sleeps, no env.
- Files: `session/domain/autonomous_stop_test.go`

#### Story 1.1.2: Bounded, redacted stop tail
**As an** operator, **I want** the captured pane tail to be size-bounded and secret-scrubbed before it reaches any durable sink, **so that** diagnosing a stall cannot leak a credential into the DB.
**Acceptance Criteria** (AC3 / pitfalls P8, P9):
- `BoundedStopTail` returns at most 40 lines / 4KB, ANSI-free, never splitting a UTF-8 rune.
  - *Given* a 10,000-byte tail ending in `"…é"` where `é` straddles the 4096-byte cut, *When* bounded, *Then* the result is valid UTF-8, <=4096 bytes, and keeps the last lines.
- `sanitizeStopTail` redacts or withholds credentials and caps at 1KB, one line.
  - *Given* a tail containing `ghp_` + 36 alphanumerics, `Authorization: Bearer abc…` (20+ chars), `postgres://u:pw@host/db`, and a `-----BEGIN RSA PRIVATE KEY-----` block, *When* sanitized, *Then* the output contains none of those canaries (private-key block => whole tail replaced by `[tail withheld: secret pattern detected]`).
  - *Given* a tail containing `AWS_SECRET_ACCESS_KEY=wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY` or `export client_secret_key=abcd1234efgh` (names the existing key/value pattern does not match), *Then* the whole tail is withheld (fail closed on an env-style `NAME=` whose upper-case name has a `SECRET`, `TOKEN`, `PASSWORD`, `PASSWD`, `KEY` or `CREDENTIAL(S)` segment).
  - *Given* a tail whose first lines are base64 key-body lines with no `BEGIN` header (a 64-character `MIIEvQIBADANBgkqhkiG9w0BAQEFAASCBKcwggSjAgEAAoIBAQC7VJTUt9Us8cKj` line), *Then* the whole tail is withheld (fail closed on any line of >= 40 base64-alphabet characters with no spaces).
  - *Given* a plain idle-prompt tail, a normal `git status`/test-output tail and a 40+ character path or URL line, *Then* the first two pass through and the long URL/path line is not mistaken for key material (it contains `/` segments and `.`/`:`; the rule requires the whole line to be base64-alphabet only). Accepted residual: a short secret in free prose with no marker, or a credential split across the 40-line cut (Deferred, engineering G5).
**Files**: `session/autonomous_stop_snapshot.go`, `server/services/autonomous_stop_detail.go`, `server/services/autonomous_stop_detail_test.go`

##### Task 1.1.2a: `BoundedStopTail` (S) — AC3
- Reuse `stripANSI` (`session/review_transcript.go:41`); trim trailing blank lines; last 40 lines; rune-safe byte cap (cut forward to a rune boundary).
- Files: `session/autonomous_stop_snapshot.go` (new; also holds `stopSource` in 2.1.1a), `session/autonomous_stop_snapshot_test.go`

##### Task 1.1.2b: `sanitizeStopTail` (S) — AC3
- Order: `session.RunPreGateSecurityCheck(raw)` non-nil -> withhold entire tail; then two **string-scan** fail-closed rules (no new regex, which keeps the plan's "no new regex set" decision): `hasSecretEnvAssignment(line)` (a token before `=` made only of `A-Z0-9_`, split on `_`, any segment in `{SECRET, TOKEN, PASSWORD, PASSWD, KEY, CREDENTIAL, CREDENTIALS}`) and `looksLikeKeyBody(line)` (length >= 40, every rune in `A-Za-z0-9+/=`, no space, and not a path: no `//`); either one -> withhold entire tail. Else `redactErrorDetail`; collapse whitespace runs/newlines to ` ⏎ `; cap 1024 bytes rune-safe. The rules withhold more tails than strictly needed (an `API_KEY=` line of a harmless config dump); that is the intended cost of failing closed on a durable sink, and the row's `state`/`action` still say what happened.
- Files: `server/services/autonomous_stop_detail.go`

##### Task 1.1.2c: Canary-secret tests (S) — AC3, AC4
- One canary per family: `sk-`, `ghp_`, `AKIA`, `xox*-`, JWT, bearer, URL userinfo, `password=`, PEM. Also: empty input, invalid UTF-8 input, ANSI-only input, exactly-at-cap input.
- Residual canaries (engineering G5): `AWS_SECRET_ACCESS_KEY=`, `client_secret_key=` (first asserted to pass `redactErrorDetail` unredacted, so the test documents *why* the new rule exists, then asserted withheld by `sanitizeStopTail`), a headerless base64 key-body tail (the mid-key case), and the false-positive guards: a 60-character URL line, a 50-character file path and an idle-prompt tail are **not** withheld. Add the accepted-residual case as a named subtest with a comment (`the password is hunter2` passes through) so the limit is pinned, not hidden.
- Files: `server/services/autonomous_stop_detail_test.go`

### Epic 1.2: Diagnosis document
**Goal**: AC1 — every stop path documented with a citation, real stalls separated from mislabels.

#### Story 1.2.1: Evidence-backed stop-path diagnosis
**As a** maintainer, **I want** a single explanation of why autonomous sessions stop early, **so that** the next incident starts from facts and not from re-reading 700 lines of driver.
**Acceptance Criteria** (AC1):
- The document lists every driver exit and every non-driver path with a permalink citation and a verdict (real stall / mislabelled / not a stall).
  - *Given* the driver at `6a395b58f`, *When* `grep -c 'fireCompletion(' session/autonomous_driver.go` is run, *Then* the document has a row for each of the 3 call sites it finds (lines 299, 352, 440 at base; the 4th match is the definition) plus the panic path that has none, and a row for each of `break` at the loop top, rate-limit wait, LLM call, suppressed-turn ctx check, lease acquire and submit (D1-D9 of `research/architecture.md` section 1a).
- The document states which causes are VERIFIED (code path exists; test or run output cited) and which frequencies are INFERRED, and states that local logs contain only test-session evidence.
  - *Given* the log census command `cat $(find ~/.stapler-squad -name 'staplersquad*.log') | grep -c 'AutonomousDriver: timed out waiting'` returns 21 with all session names `test-*`, *When* the doc is read, *Then* it says production frequency is unknown rather than ranking causes by count.
**Files**: `docs/explanation/autonomous-stall-causes.md`, `CLAUDE.md`

##### Task 1.2.1a: Re-verify citations and census (S) — AC1
- At the then-current HEAD re-open each site in `research/architecture.md` section 1 (D1-D11, S1-S4, O1-O7, O9, external callers) and record `path:line` + `git rev-parse HEAD`. Build permalinks as `https://github.com/tstapler/stapler-squad/blob/<sha>/<path>#L<n>` where `<sha>` comes from `gh api "repos/tstapler/stapler-squad/commits?path=<path>&per_page=1" --jq '.[0].sha'` (not the contents-endpoint blob SHA).
- Run the log census (command + output pasted into the doc), noting rotation/test-pollution.
- Files: none (feeds 1.2.1b)

##### Task 1.2.1b: Write the explanation (M) — AC1
- Diataxis *explanation*. Sections: what "stuck" meant before; stop-path table (driver, service mislabels, external callers, outside-driver paths) with columns `id`, `path:line` (SHA-pinned permalink), **`symbol`** (the function or statement the permalink lands on, so the citation check can compare it), `verdict`; verdict column; root-cause statement ranked by confidence (mislabel hides cause: VERIFIED; recovery kills a resumable or working pane: VERIFIED by reading `onAutonomousDriverComplete`; external stop treated as stall: VERIFIED; 60s startup idle wait vs a long first turn: INFERRED, no production evidence; panic leaves `AutonomousMode` true: VERIFIED); the post-change `StopReason` taxonomy and ADR-001 decision table; known gaps (stale-work detector kills with no nudge or snapshot, AC3 scope; lifetime nudge cap; pause and user stop never stop the driver (`session_service_update.go:99-110`, Deferred Adv C1); `buildTurnCallback` raw writes; whether `SessionDriver.attemptBacklogNudge` (`session_driver.go:925-953`) and the autonomous nudge can both be active on one session: state the answer with a citation, the lease serialises writes but not intent); the stuck row stays open after a successful nudge (cosmetic, architecture nit); the third driver start site (`session_creation_pipeline.go:347-355`) and the stale-detector / health-checker restarts list; `diagnose_nudge_enabled` already true in the live config and the new `autonomous_stall_nudge_enabled` flag that gates the in-process nudge.
- Add one row to the `CLAUDE.md` Reference Documents Index (shared file: single-line table row only).
- Files: `docs/explanation/autonomous-stall-causes.md`, `CLAUDE.md`

---

## Phase 2: Driver instrumentation

### Epic 2.1: Driver emits typed stops with a pre-teardown snapshot
**Goal**: Every exit path, including panic, fires exactly one completion carrying a typed reason, the real turn count and a pane snapshot taken before anything is ended.

#### Story 2.1.1: `finish()` as the single exit
**As an** operator, **I want** every driver stop to say why and what the pane was doing, **so that** "stuck" is no longer one undifferentiated outcome.
**Acceptance Criteria** (AC2, AC3):
- Each exit site produces its own `StopReason`; `Turns` is the real count; `Stuck` is derived.
  - *Given* a driver with `maxTurns: 5` whose pool returns an error on turn 2 while the run ctx is live, *When* it stops, *Then* the outcome is `{StopReason: orchestrator_error, Turns: 2, Stuck: true}` (today: `Reason: "max turns reached"`, `Turns: 5`).
- External stop, shutdown and startup timeout are distinguished.
  - *Given* a driver blocked in the startup idle wait, *When* `Stop()` is called, *Then* `StopReason == cancelled_external`; *When* instead the parent ctx is cancelled, *Then* `cancelled_shutdown`; *When* only the 60s startup deadline elapses, *Then* `startup_timeout` with `Turns: 0`.
- A panic fires a completion exactly once.
  - *Given* a pool that panics on the first call, *When* the driver stops, *Then* the callback receives `StopReason: panic` once and `driver.Wait` returns true.
- The snapshot is captured before the callback and never blocks completion.
  - *Given* a `stopSource` whose `Tail` panics, *When* the driver stops, *Then* the callback still fires with `Snapshot.CaptureErr != ""`.
**Files**: `session/autonomous_driver.go`, `session/autonomous_stop_snapshot.go`, `session/autonomous_driver_test.go`, `session/autonomous_stop_snapshot_test.go`

##### Task 2.1.1a: Outcome fields, `stopRequested`, `stopSource`, capture (M) — AC2, AC3
- `AutonomousDriverOutcome` gains `StopReason domain.StopReason`, `Snapshot domain.StopSnapshot` and `RunID uint64`; keep `Done/Reason/PRUrl/Turns/Stuck` as legacy inputs (existing struct-literal tests construct it) and add `IsDone()`/`IsStall()` methods derived from `StopReason` via `normalizeStopReason`. `AutonomousDriver.RunID()` returns the same id; the service registry stores the driver so identity is comparable.
- `AutonomousDriver` gains `stopRequested atomic.Bool` (set first in `Stop()`), `completed atomic.Bool`, `source stopSource`, `turnLeaseWait time.Duration` (zero -> `AutonomousTurnLeaseWait`, so a lease-timeout test need not wait 15s). All new fields must have zero-value fallbacks: existing tests build `&AutonomousDriver{...}` literals.
- `stopSource` + default `instanceStopSource{inst, controller}` in `session/autonomous_stop_snapshot.go`: `GetCurrentStatus()` -> `StatusBucket` via `IsSafeSteerStatus`; `LastMeaningfulOutputTime()`; `TmuxAlive()`; `PaneExitInfo()`; tail via `Instance.PreviewContext(ctx)` (`instance_terminal.go:214`; `panePreviewer.Preview()` takes no ctx, so `previewPane()` cannot honour a timeout) with `ctx` = `context.WithTimeout(context.Background(), 2s)` (the run ctx may already be cancelled), then `BoundedStopTail` (which adapts `stripANSI(raw []byte) string`: pass `[]byte(tail)`). Fakes: `stopSource` is the test seam, so the `previewer` fake is not involved. Capture runs under its own `defer recover()`; a failure sets `CaptureErr`, never blocks or panics. Capture is skipped (zero snapshot, `CaptureErr = "skipped"`) for `cancelled_external` and `cancelled_shutdown`: no decision consumes it and shutdown must not wait on tmux.
- Test seams (validation S2): `captureTimeout time.Duration` on `AutonomousDriver` (zero -> 2 s) so "a hung tail never blocks completion" costs no real seconds. `Finished() bool` accessor: non-blocking read of the `done` channel (`done == nil` -> false, i.e. not started), used by `startDriverIfAbsent`'s liveness check (Task 3.1.3a); `Wait(ctx)` is not usable for this because with an already-cancelled ctx and a closed `done` its `select` picks at random.
- Files: `session/autonomous_driver.go`, `session/autonomous_stop_snapshot.go`

##### Task 2.1.1b: Replace the exits in `run()` (M) — AC2
- Introduce a `stopReason` local; each former `break` assigns its own reason; after the loop `finish(reason, detail)` is the sole `fireCompletion` caller (idempotent via `completed.CompareAndSwap`).
- Cancel classification helper: `ctx.Err() != nil` -> `stopRequested.Load() ? cancelled_external : cancelled_shutdown`; otherwise by site: LLM error -> `orchestrator_error`; `AcquireTerminalWriteLease` `ErrLeaseBusy` -> `lease_timeout`; `SubmitDriverContent` error -> `submit_failed`; `waitForRateLimitClear` non-cancel error -> `rate_limit_exhausted`; startup wait: `startupCtx` deadline with `ctx.Err()==nil` -> `startup_timeout`; loop exhausted -> `max_turns`; DONE -> `done` (unchanged outcome fields).
- Panic recover calls `finish(StopReasonPanic, fmt.Sprint(r))`, guarded so a panic inside the callback cannot re-enter.
- `Turns` = the 1-based number of the turn in progress at exit (`turnCount+1`, the existing DONE convention), 0 for startup exits, and `maxTurns` only for loop exhaustion; never `maxTurns` for the other exits.
- If `make ready-complexity-gate` flags `run()`, first extract `waitStartupIdle` and `injectTurn` methods as a separate behaviour-preserving commit. The pinned unit `session:AutonomousDriver.run` (`terminal_write_pins_test.go:49`) calls `AcquireTerminalWriteLease` and `SubmitDriverContent`; if that block moves, rename the pin row to `session:AutonomousDriver.injectTurn` (kind unchanged) in the same commit, keep `run` unpinned only if it no longer calls either primitive, and run `make test-delivery-guards` before pushing (it is not part of `go test`). If the block does not move, the row is untouched.
- Files: `session/autonomous_driver.go`

##### Task 2.1.1c: Driver tests per exit (M) — AC2, AC4
- One table-driven test over the sites using `fakeHeadlessPool`, `fakePanePreviewer`, `fakeSendKeysProcessManager{failOn}` and a fake `stopSource`: startup timeout (`WithStartupTimeout` tiny), startup cancel via `Stop()`, startup cancel via parent ctx, LLM error, LLM error caused by cancel, lease busy (hold the lease, tiny `turnLeaseWait`), submit failure (`failOn`), rate-limit exhausted (needs a small `maxRateLimitWait` seam if the const is not injectable — add one field, zero -> const), max turns, DONE, panic, suppressed-turn cancel.
- Assert exactly one callback per run, snapshot captured before the callback, `Turns` values, and that `Stop()` after completion is a no-op.
- Update `TestAutonomousDriver_PanicRecovery` to also assert the single completion. The hung-`Tail` case sets `captureTimeout` tiny (S2, no 2 s wait). `Finished()` is false before `Start`, false while running, true after every exit incl. panic (DRV-10).
- No `time.Sleep`, `t.Parallel()`, no `t.Setenv`; reuse `pumpIdleSignals`.
- **BUG-079 handling** (engineering G3; `docs/bugs/open/BUG-079-autonomousdriver-idle-timer-race-under-full-suite.md`, an intermittent `-race` failure of `TestAutonomousDriver_Stop_CancelsLoop_DuringNudgeSuppression` that appears only in the full `session` run, in the same file this task grows by about a dozen driver tests). In order: (1) **baseline before editing**: `go test ./session/ -race -count=10 -run TestAutonomousDriver` and one full `go test ./session/... -race -count=1`, pasted into the commit message; (2) every new test starts the driver with `ctx` and joins it in `t.Cleanup` (`driver.Stop(); driver.Wait(...)`), and stops its `pumpIdleSignals` goroutine through the same ctx, because the leading remaining suspect is a leaked `run()` or pump goroutine from a neighbour (the bug doc's package-level-timer suspect does not match this HEAD: timers are per-driver fields, and `nudgeCooldown` is never written by tests); (3) if the race reproduces in (1) or later, the race trace names the variable: fix it as its **own commit** (standing fix-collateral-debt policy), and paste the trace into the bug doc; (4) if it does not reproduce, the PR description says "BUG-079 known flake, not caused by this diff, baseline run attached" and a red full-suite gate with exactly that test name is re-run once, not treated as a regression. Do not mark the bug fixed without a captured trace.
- Files: `session/autonomous_driver_test.go`, `session/autonomous_stop_snapshot_test.go`

---

## Phase 3: Service integration

### Epic 3.1: Seam and typed recording
**Goal**: The service records and distinguishes every stop, and external/shutdown stops stop being treated as stalls.

#### Story 3.1.1: Behaviour-preserving extraction of the work-stall arm
**As a** maintainer, **I want** the work-role `!Done` arm in its own function, **so that** the decision seam exists before any behaviour changes and the diff of the behavioural story stays reviewable.
**Acceptance Criteria** (enabler for AC3):
- No behaviour change.
  - *Given* the 21 existing `TestAutonomousOrchestrationService_*` / `TestOnAutonomousDriverComplete_*` tests (counted with `grep -cE '^func Test(AutonomousOrchestrationService_|OnAutonomousDriverComplete_)'`) plus the new characterization tests, *When* `go test ./server/services -run 'Autonomous|OnAutonomousDriverComplete'` runs before and after, *Then* all pass unchanged and the diff is a pure move plus one call that consumes a `stallResult` of `{Respawn, zero}`.
**Files**: `server/services/autonomous_orchestration_service.go`, `server/services/autonomous_orchestration_service_characterization_test.go`

##### Task 3.1.1a: Extract `handleWorkStall` (S) — AC3 (enabler)
- Move the `case session.SessionRoleWork: if !outcome.Done {…}` body (L361-442 at base) into `func (a *AutonomousOrchestrationService) handleWorkStall(ctx, storage, item, is, outcome)`; keep comments, ordering (`UpdateItemSessionEnded` -> `RemediationDue` -> goroutine) and the `//nolint:silenttransition` annotation verbatim. `toStatus` stays unset.
- `handleWorkStall` returns `stallResult{Action domain.StallAction; Notify notifyOverride}` (zero `notifyOverride` = publish the generic notification unchanged). In this commit it always returns `{Action: Respawn}`; the outer function consumes the value at the existing notification site so later stories only populate it. This is the contract the nudge/resupervise branches use to suppress or reword the generic FAILURE "Autonomous fix stuck" notification built at the end of the outer function.
- Before extracting, add characterization tests for the branches the 21 existing tests may miss: `RemediationDue` not-due (backoff skip), `justParked`, `UpdateItemSessionEnded` failure (`notifyStuckBookkeepingFailed`), and nil `autonomousStuckRespawner`. They must pass on the pre-extraction code. Also pin the two behaviours the later moves would silently change (pre-mortem #7): (a) the `SessionUpdatedEvent` published at `:290` carries a payload equal to the instance snapshot after the write (SVC-11), and (b) `MarkStuck` is called exactly once per stall for each of work, triage, review and unrecognized roles (SVC-12). The extraction keeps the outer `ctx := context.Background()` for every storage call (do not thread any later recovery ctx through this function).
- Signature here is `(ctx, storage, item, is, outcome)`; Task 3.2.3b adds the `completion` parameter and moves `planStall`, the work-role `MarkStuck` and the `switch plan` into it, so the glossary's ownership statement holds from 3.2.3b on and this commit stays a pure move.
- Run the existing tests and `make ready-complexity-gate`; commit alone.
- Files: `server/services/autonomous_orchestration_service.go`, `server/services/autonomous_orchestration_service_characterization_test.go`

#### Story 3.1.2: Instance records the stop reason through the actor
**As a** maintainer, **I want** the autonomous fields written through typed setters, **so that** snapshot readers never race the callback, the stop reason is visible on the instance, and a recovered-in-place stop never clears `AutonomousMode`.
**Acceptance Criteria** (AC2):
- Wire outcome stays in `{"", "done", "stuck"}` for every `StopReason`.
  - *Given* `SetAutonomousStopped(StopReasonCancelledExternal)`, *When* `inst.Snapshot().Autonomous` is read, *Then* `AutonomousMode == false`, `AutonomousOutcome == ""`, `AutonomousStopReason == domain.StopReasonCancelledExternal`; *Given* `StopReasonOrchestratorError`, *Then* outcome `"stuck"`; *Given* `StopReasonDone`, *Then* `"done"`.
- Observed-only stops keep intent.
  - *Given* `AutonomousMode == true`, *When* `SetAutonomousStopObserved(StopReasonStartupTimeout)` runs, *Then* `AutonomousMode` is still `true`, outcome unchanged, `AutonomousTurn == 0`, reason recorded; *When* `SetAutonomousMode(true, ...)` runs next, *Then* the reason is cleared.
- A write that did not apply is observable (engineering G7). The three new setters return `sendSyncErr`'s error, which is non-nil only when the instance actor's ctx is done (`session/actor.go:34-56`).
  - *Given* an instance with a live actor (`NewLiveInstance`, `session/live_instance.go:36-51`) whose ctx has been cancelled (`li.cancel()`), *When* `SetAutonomousStopped(StopReasonOrchestratorError)` runs, *Then* it returns a non-nil error wrapping `context.Canceled`, the snapshot is unchanged, and `applyInstanceStop` logs one Warn naming the session and reason, publishes no `SessionUpdatedEvent` (the payload would be the stale pre-write snapshot) and still lets the callback continue (the instance is going away; bookkeeping and the respawn decision are unaffected).
**Files**: `session/instance.go`, `session/instance_snapshot.go`, `session/instance_actor_setters.go`, `session/instance_actor_setters_test.go`, `server/services/autonomous_orchestration_service.go`, `server/services/autonomous_apply_stop_test.go`

##### Task 3.1.2a: Field, snapshot, setters (S) — AC2
- `Instance.AutonomousStopReason domain.StopReason` (typed, not string; `session` already imports `domain`); add to `AutonomousModeState` and `buildSnapshot` (the header comment in `instance_snapshot.go` is the single field list). `SetAutonomousStopped(reason domain.StopReason)` sets mode false, turns 0, outcome by a typed exhaustive switch, reason. `SetAutonomousStopObserved(reason)` sets reason and zeroes turn counters only. `SetAutonomousStoppedQuiet(reason)` (boot pass, adversarial C2) is `SetAutonomousStopped` with outcome forced to `""` for every reason; the table test asserts it for all `StopReason` values. `SetAutonomousMode(true, …)` clears the reason (new run).
- Keep `SetAutonomousComplete(done)` as a thin wrapper if tests still use it; otherwise remove with its test.
- Error contract (engineering G7): `SetAutonomousStopped`, `SetAutonomousStopObserved` and `SetAutonomousStoppedQuiet` return `error` (the `sendSyncErr` result) instead of the `_ =` discard the older setters use. This matters because the plan's invariant "mode is never left true with no driver" depends on these writes applying; the `nil`-`liveInstance` direct path (what the invariant tests use) cannot fail, so without a test the failure branch is unobserved. New test INS-6 (validation) uses a cancelled live instance. The older setters are not changed.
- Files: `session/instance.go`, `session/instance_snapshot.go`, `session/instance_actor_setters.go`, `session/instance_actor_setters_test.go`

##### Task 3.1.2b: Replace raw writes (S) — AC2
- Replace L282-289 with one `applyInstanceStop(inst, action, outcome)` helper that calls `SetAutonomousStopObserved` for `Nudge`, `Resupervise` and (flag-dependent, Task 3.1.3b) `cancelled_shutdown`, and `SetAutonomousStopped(reason)` otherwise (`cancelled_external` -> outcome `""`); `reason` is `normalizeStopReason(outcome)` (empty + `Done` -> `done`; empty + `!Done` -> unspecified stall, outcome `stuck`, so legacy test outcomes behave as today). The `NewSessionUpdatedEvent` publish (fields `autonomous_mode`, `autonomous_outcome`) happens **inside** `applyInstanceStop`, immediately after the write, so the write and its event are one unit and the event payload is the post-write snapshot (pre-mortem #7; SVC-11). The helper is called once at plan time, never from a function-exit defer. Until Task 3.2.3b computes an action, the helper is called with `Respawn`/`None` equivalents so behaviour is unchanged.
- Table test over all `StopReason` values asserting the wire outcome set.
- Setter error (engineering G7): `applyInstanceStop` checks the setter's error; on non-nil it logs one Warn (`session`, `stop_reason`, `err`), skips the `SessionUpdatedEvent` publish and returns the error to its caller only as a value the Nudge/Resupervise paths read as "instance gone: do not restart" (they already refuse via `canRestart`'s status check; this is the same refusal from the write side). Test: SVC-1 gains a "write failed" row; INS-6 covers the setter.
- Files: `server/services/autonomous_orchestration_service.go`, `server/services/autonomous_apply_stop_test.go`

#### Story 3.1.3: Cancelled stops never look like stalls; shutdown is re-adopted under a strict gate
**As an** operator, **I want** toggling autonomous off, deleting or handing off to review not to mark the item stuck or respawn it, and a service restart not to turn every autonomous session into a manual one (nor to revive sessions that must stay dead), **so that** the system stops fighting my own actions.
**Acceptance Criteria** (AC2, AC4, requirements non-goal):
- Class guards, every role (adversarial B6). A `Cancelled` outcome has `Done == false`; each arm must test `outcome.Class()` first.
  - *Given* a backlog work `ItemSession` (in_progress item, `EndedAt == nil`) and a driver stopped through `StopDriverForSession`, *When* the callback runs with `StopReasonCancelledExternal`, *Then* no `autonomous_stuck` row is written, `ItemSession.EndedAt` is still nil, the respawner and the nudger are called 0 times, no notification is published, and the instance shows outcome `""` with reason `cancelled_external`.
  - *Given* the same work case with a **pre-existing open** `autonomous_stuck` row, *Then* the row stays open (the Done branch's `resolveAutonomousStuck` is not reached) and `remediation_attempts` is unchanged.
  - *Given* a triage-role `ItemSession` for an item in `idea`, *When* the callback runs with `cancelled_external`, *Then* the item is still `idea` (no `TransitionBacklogItemStatus`, no `idea -> ready`), no "Triage stuck" notification and no triage result is recorded.
  - *Given* a review-role session whose item is still in `review`, *When* `cancelled_external` runs, *Then* `UpdateItemSessionEnded` is not called (the `default` arm is unreachable) and the open row, if any, is untouched; *Given* the item has left review, *Then* the row is resolved exactly as the existing `status != review` case does.
  - *Given* an unrecognized role, *When* `cancelled_external` runs, *Then* no notification and no transition.
  - *Given* an unlinked (non-backlog) instance, *When* `cancelled_external` runs, *Then* the instance shows outcome `""`, and no "Autonomous fix stuck" or "complete" notification is published (the final `if outcome.Done {...} else {...}` is a three-way switch on `Class()`).
  - The same five cases are run with `cancelled_shutdown` and assert the same silence (no row, no `UpdateItemSessionEnded`, no notification).
- Shutdown with the flag ON (decision: persisted `AutonomousMode` is the re-adoption signal).
  - *Given* `StopReasonCancelledShutdown` for a work item and `autonomous_driver_readoption` ON, *Then* `AutonomousMode` is still `true` in the snapshot, no `autonomous_stuck` row, no notification, `UpdateItemSessionEnded`/`RemediationDue`/respawn/nudger not called, and the instance data round-trips through `ToInstanceData`/`FromInstanceData` with `AutonomousMode == true`.
- Shutdown with the flag OFF (adversarial C2).
  - *Given* the same with the flag OFF, *Then* `AutonomousMode == false`, outcome `"stuck"` and the stop reason `cancelled_shutdown` are set via `SetAutonomousStopped` (today's in-memory outcome), with still no row, notification, respawn or nudge.
- Boot pass, strict gates (adversarial B5) with a three-class result (round-3 C1). A candidate is any instance whose persisted `AutonomousMode == true`. It is adopted only if **every** gate passes. A failing gate is classified; only a definitive **reject** clears the mode. Table-driven, one row per gate with the other gates passing:
  | Gate | Failure | Class |
  |------|---------|-------|
  | `status_active` | `Status` is `Hibernated`, `Stopped`, `Crashed`, `PermanentlyFailed`, `Failed` or `Paused` (one subcase each) | reject |
  | `status_active` | `Status` is `Creating` or `Restoring` (a backend mid-restore) | unknown |
  | `not_archived` | `IsArchived()` | reject |
  | `not_paused` | `IsPaused()` | reject |
  | `mode_persisted` | `AutonomousMode == false` (not a candidate; listed so the gate is table-covered) | reject (no-op) |
  | `controller_present` | no controller | reject; **unknown** on the bounded fallback pass, where Step 7 may not have run |
  | `no_driver` | a driver already registered under the title, or `startDriverIfAbsent` returns `ErrDriverPresent` | **skip** (mode untouched, Info log; this is how a second pass run coexists with the first run's adoption) |
  | `not_retry_pending` | `IsRetryPending()` | reject |
  | `item_session_open` | no linked ItemSession (`ErrNotFound`), or its `EndedAt != nil` (rework/stale paths end it and kill the pane without clearing mode) | reject |
  | `item_session_open` / `item_status_matches_role` | storage lookup returns a non-`ErrNotFound` error | unknown |
  | `item_status_matches_role` | item status is not the status the role works in (work: `in_progress`, review: `review`, triage: `idea`) | reject |
  | `pane_alive` | `PaneExitInfo()` reports the pane dead / `TmuxAlive() == false` for a non-restoring status | reject |
  | `pane_owned` | `VerifyPaneOwnershipBeforeWrite` returns an ownership mismatch | reject |
  | `pane_owned` | the `tmux show-environment` probe times out or errors | unknown |
  | `no_step75_driver` | `InitialPrompt != ""` and `InitialPromptSentAt` is zero (the Step 7.5 `StartSessionDriver` owns the pane first; not adopted, mode cleared as today) | reject |
  - *Given* all gates pass, *Then* exactly one driver is started (5 min warm-up) via `startDriverIfAbsent`; *Given* the pass runs twice (test only; production is `sync.Once`), *Then* the second run starts none **and leaves the adopted session's `AutonomousMode == true`** (its `no_driver` gate is `skip`, not `reject`; no clear, one Info line).
  - *Given* a candidate with a reject-class failure, *Then* its in-memory mode is cleared via `SetAutonomousStoppedQuiet(cancelled_shutdown)` (outcome `""`, so `SessionCard.tsx:888` shows no badge; adversarial C2), one Info line names the instance, the failing gate and class `reject`, and no driver is started.
  - *Given* a `skip` candidate (driver already registered, or `ErrDriverPresent` from the start), *Then* the mode is untouched, the registry is unchanged, and the result is logged `skip gate=no_driver`.
  - *Given* a transient probe error (fake `paneProbe` returns a timeout for `pane_owned`, or the fake item storage returns a non-`ErrNotFound` error), *Then* the mode is untouched, no driver is started, the result is logged at Warn as `unknown`, and a follow-up pass re-evaluates it: a second evaluation that now passes adopts it, one that still errors leaves it untouched after the last follow-up pass.
  - *Given* a start that returns `ErrNoDriverPool` or a `Start` failure, *Then* the mode is cleared the same way as a reject and the failure logged.
  - *Given* more than the per-boot cap (8) passing candidates, *Then* the first 8 in `CreatedAt`-ascending order (ties by instance UUID) are started in the first pass, the rest are logged `deferred` with their mode **untouched**, and they are re-evaluated by follow-up passes (60 s apart, at most 3, injected clock); only candidates still over the cap after the last follow-up pass are cleared quietly, with one Warn naming the count.
  - *Given* the flag OFF, *Then* gates are evaluated and logged (`would_adopt` or the failing gate and class), nothing is started, reject and `would_adopt` candidates are cleared quietly, and `skip` and `unknown` candidates are left untouched.
  - Invariant after the final pass, for flag ON and OFF: no instance has `AutonomousMode == true` with no registered driver, **except** candidates logged `unknown` (and, transiently, `deferred` between passes).
- Persisted round trip (adversarial B5 counter-evidence that persisted mode alone is not proof).
  - *Given* an instance saved with `AutonomousMode == true` and `Status == Hibernated`, *When* stored, loaded through `FromInstanceData` and the pass runs, *Then* it is a `reject` by `status_active` and its mode is cleared (outcome `""`).
- Trigger ordering (round-3 C3).
  - *Given* the lifecycle ctx has been set and signal (b) has not arrived, *Then* the pass does not run until the fallback timer (90 s, injected clock) elapses, then runs once with `controller_present` failures classified `unknown`; *Given* (b) arrives first, *Then* the fallback never runs a second pass. *Given* the Step 7 / 7.5 / 7.6 goroutine returns on any path, *Then* (b) is signalled (`defer`).
- Superseded completion.
  - *Given* driver D1 completing with `RunID` 1 while D2 (`RunID` 2) is registered under the same title, *When* the callback runs, *Then* the registry still holds D2, the instance's mode/outcome are unchanged, and nothing is recovered.
- Registration (architecture C1/C8, adversarial C4).
  - *Given* a completion that has been claimed and not yet released, *When* `stopAndDeregisterDriver(title)` runs at any point after `claimCompletion` returns (including before the item lookup), *Then* the recovery ctx is cancelled.
  - *Given* a driver already registered under a title, *When* `startDriverIfAbsent` is called, *Then* it returns `ErrDriverPresent` and the registry is unchanged; *When* `registerDriver` is called with a different driver, *Then* the previous driver's `Stop()` is called once.
- Lock safety (pre-mortem P1 #1). Every test below runs under a watchdog helper that **fails** the test (not hangs) if the body has not returned after 5 s; a passing run never waits on it.
  - *Given* fake seams (`instanceFinder`, `storageGetter`, bus, pool, `driverStarter`) that call `a.mu.TryLock()` and fail if it returns false, *When* `claimCompletion`, `startDriverIfAbsent`, `restartAfterRecovery`, `registerDriver` and `stopAndDeregisterDriver` run, *Then* no seam is ever invoked with `a.mu` held (REG-12).
  - *Given* a completion callback, *When* it calls `claimCompletion` then `stopAndDeregisterDriver`, *Then* neither re-takes the mutex (REG-13).
  - *Given* a `Stop` arriving while `startDriverIfAbsent` is between sentinel and commit (blocked in a fake `NewAutonomousDriver`), *Then* it returns without deadlock, the new driver is torn down and not registered (REG-14).
  - *Given* a real `SessionService.DeleteSession` running concurrently with a completion callback and a `startDriverIfAbsent` under `-race`, *Then* all three return (REG-15).
- Liveness (pre-mortem #5). *Given* a registered driver whose `Finished()` is true, *When* `startDriverIfAbsent` runs, *Then* it replaces the entry and starts a new driver rather than returning `ErrDriverPresent` (REG-16). *Given* a panic inside the completion callback, *Then* `AutonomousMode` is cleared, one FAILURE notification is published and the Respawn body is not replayed (STL-15).
**Files**: `server/services/autonomous_orchestration_service.go` (3.1.3a, 3.1.3e, 3.1.3b), `server/services/autonomous_driver_registry.go` (3.1.3a, 3.1.3e, 3.1.3f), `server/services/session_creation_pipeline.go` (3.1.3e), `server/services/autonomous_readoption.go` (3.1.3c), `server/services/autonomous_readoption_trigger.go` (3.1.3f), `server/dependencies.go`, `server/server.go` and `server/services/session_service_wiring.go` (3.1.3f), `server/services/feature_flag_service.go` and `config/config.go` (3.1.3b), plus the three test files of 3.1.3d. `SessionService.SetLifecycleContext` (`session_service_wiring.go:269-275`) already delegates to the autonomous service by name and is not changed; 3.1.3f adds only the signal-(b) delegate beside it

##### Task 3.1.3a: Registry hardening, completion identity (M) — AC2
- New `server/services/autonomous_driver_registry.go` holds `recovery`, `claimCompletion`, `startDriver`/`startDriverIfAbsent`, `canRestart`/`canRestartLocked`, and the changed `registerDriver` (map mutation under `a.mu` and nothing else, per the Glossary `a.mu` rule; types per the Glossary). `a.recoveries map[string]*recovery`, `a.starting` reservations (separate map or typed entry, whichever keeps existing tests that read `a.drivers` compiling).
- Replace the callback's first line (`stopAndDeregisterDriver(instanceName)`, `autonomous_orchestration_service.go:260`) with `c, res := a.claimCompletion(instanceName, outcome.RunID)` and `defer c.release()`. Under one `a.mu` hold: registered driver with non-zero `RunID` different from `outcome.RunID` -> `superseded` (log Info, return without touching registry/instance/item); registered and matching -> delete without `Stop()` (goroutine already exited); unregistered -> proceed; then register the recovery entry (ctx derived from `a.driverCtx()` loaded **before** the lock is taken, never `Background()`), cancelling and replacing an entry of an older runID, or returning `duplicate` for the same runID (callback returns; the winner is acting). `RunID == 0` matches anything (legacy test literals). Registration therefore happens before the item lookup, `planStall` and `MarkStuck`, so a stop in the DB round-trip window finds the entry (architecture C1).
- `stopAndDeregisterDriver(title)` additionally cancels and deletes `recoveries[title]`; every external stopper (`session_service_lifecycle.go:47,306`, `session_service_update.go:296`, `server/mcp/tools_backlog.go:1998` via `StopDriverForSession`) reaches an in-flight nudge.
- `startDriver(inst, timeout, opts) (*AutonomousDriver, error)` is the former body of `StartAutonomousDriverWithTimeout`/`ForInstance` returning `ErrNoDriverPool`/`Start` errors; the two public methods become void wrappers that log the error (signatures and the compile-time interface assertion unchanged). `startDriverIfAbsent` is the reserve-then-start sequence in the Glossary: sentinel under the lock, `NewAutonomousDriver` / `withCostSink` / `Start` outside it, commit under the lock; `Stop()` on a displaced or torn-down driver always runs outside `a.mu`. A registered driver with `Finished() == true` is replaced (Warn), so a dead entry cannot return `ErrDriverPresent` forever.
- `canRestart(rec, inst) error` and `canRestartLocked(rec, inst) error` per the Glossary row (include `GetLifecycleStatus() == session.Active`, no `starting` sentinel); callers: `canRestart` before the nudger and in the nudge/resupervise effects, `canRestartLocked` only inside `restartAfterRecovery`. No `a.*` method that locks is called with `a.mu` held.
- Files: `server/services/autonomous_driver_registry.go`, `server/services/autonomous_orchestration_service.go`

##### Task 3.1.3e: Lifecycle ctx atomic, callback panic wrapper, third start site (S) — AC2
- Split from 3.1.3a for sequencing (engineering G1): 3.1.3a is the registry and its identity/ownership rules; this task is the three changes that touch other call sites. Full scope kept.
- `lifecycleCtx` becomes an `atomic.Pointer[context.Context]`: `SetLifecycleContext` stores, `driverCtx()` loads (fallback `Background()`), and the respawn goroutine's raw `a.lifecycleCtx` read at `:434` is changed to `a.driverCtx()`. **`lifecycleCtx`, `SetLifecycleContext` and `driverCtx()` move from `autonomous_orchestration_service.go:103-134` into `autonomous_driver_registry.go`** (engineering G2): the boot-pass trigger (Task 3.1.3f) then edits only the registry and trigger files, so it no longer shares `autonomous_orchestration_service.go` with Task 3.1.4a in one wave. The method keeps its name and signature, so `session_service_wiring.go` and `server/server.go:1087` do not change. Nothing reads or writes it under `a.mu`, so a locked registry never nests with it (pre-mortem P1 #1; this replaces Task 3.1.3c's earlier "write under `a.mu`").
- Completion-callback panic wrapper (pre-mortem #5): after `claimCompletion` succeeds the callback runs under `defer recoverCompletionPanic(...)`: on a panic it logs Error with the stack, calls `SetAutonomousStopped(<stop reason>)` (mode cleared, so no ghost), publishes one FAILURE notification ("autonomous recovery panicked"), and does **not** replay the Respawn body (non-idempotent steps such as `UpdateItemSessionEnded` may already have run); `release` and the short guard stamp still happen via defers. The stale detector is the backstop (STL-15).
- Third start site (pre-mortem #5): `session_creation_pipeline.go:347-355` is routed through `startDriver`/`startDriverIfAbsent` instead of building and registering the driver inline. Before routing, read `s.autonomousDriverClient()` and `a.pool`/`SetClient`: if they are the same client use it directly, otherwise pass the pipeline's client as a `startDriver` option. Routing adds the turn callback the inline site never had (`AutonomousTurn` updates for UI-created autonomous sessions, same as the toggle/spawn starters) and removes the non-atomic start-then-register gap; note it in the PR as a behaviour difference.
- Files: `server/services/autonomous_driver_registry.go`, `server/services/autonomous_orchestration_service.go`, `server/services/session_creation_pipeline.go`

##### Task 3.1.3b: Per-arm class guards, shutdown handling, readoption flag (M) — AC2
- `applyInstanceStop` (Task 3.1.2b) maps: `Cancelled`/external -> `SetAutonomousStopped(cancelled_external)` (outcome `""`); `cancelled_shutdown` -> `SetAutonomousStopObserved` when `autonomous_driver_readoption` is ON, `SetAutonomousStopped(cancelled_shutdown)` when OFF (flag read at this instant).
- The top-level `MarkStuck` block (`autonomous_orchestration_service.go:326-336`) is guarded by `outcome.Class() == Stall`, and for `is.Role == SessionRoleWork` it moves into `handleWorkStall` after `planStall` (Task 3.2.3b); until then it stays at the top, guarded.
- Per-arm guard at the **top of each arm** (the structural change B6 asks for), each a single `if outcome.Class() == domain.OutcomeCancelled { ... }`:
  - Triage arm: `return` (log Info) before `if !outcome.Done {...}`; `toStatus` is never set, so the post-switch `TransitionBacklogItemStatus` cannot fire.
  - Work arm: `return` (log Info) before `if !outcome.Done` and before the DONE `else`, so `UpdateItemSessionEnded`, `RemediationDue`, respawn and `resolveAutonomousStuck` are all unreachable.
  - Review arm: run only the existing `status != review` resolve case; the `default` (UpdateItemSessionEnded) is skipped for `Cancelled`; then `return`.
  - Unrecognized-role arm: `return` before the generic notification.
  - Unlinked / lookup-failed / item-load-failed path (no arm runs): the final notification is a `switch outcome.Class()` (`Done` -> complete, `Stall` -> stuck, `Cancelled` -> none); update event and one Info log still fire.
- Register `autonomous_driver_readoption` (live-read via `config`, same pattern as `diagnose_nudge_enabled`, `feature_flag_service.go:278-281`); no env var. Its default is the single constant `autonomousDriverReadoptionDefault` in `config/config.go` (owner decision: **OFF**, value `false`; see Deferred); no other code or test hard-codes the default.
- Files: `server/services/autonomous_orchestration_service.go`, `server/services/feature_flag_service.go`, `config/config.go`

##### Task 3.1.3c: Boot re-adoption pass and gates (M) — AC2
- `server/services/autonomous_readoption.go`: `readoptGates` (ordered `[]struct{name string; check func(ctx, inst, deps) gateResult}`, where `gateResult{Class pass|reject|skip|unknown; Gate string; Err error}`), `func (a *AutonomousOrchestrationService) ReadoptAutonomousDrivers(ctx, instances []*session.Instance) readoptReport`. Dependencies are narrow seams for tests: `driverStarter`, `itemSessionLookup` (wraps `GetItemSessionBySessionUUID` + `GetBacklogItem`; maps `ErrNotFound` to reject and any other error to unknown), `paneProbe` (wraps `PaneExitInfo`, `VerifyPaneOwnershipBeforeWrite`, and a liveness read through `GetLifecycleStatus()` rather than the raw-`Status` `Instance.TmuxAlive()`; a probe timeout or error is unknown, an ownership mismatch or dead pane is reject).
- Clearing policy: only a `reject` (or a start failure: `ErrNoDriverPool`/`Start` error) clears mode, via `SetAutonomousStoppedQuiet(cancelled_shutdown)` (outcome `""`) + `NewSessionUpdatedEvent`. `skip` and `unknown` never write. `ErrDriverPresent` from `startDriverIfAbsent` maps to `skip`.
- Order and cap: candidates are sorted `CreatedAt` ascending, ties by instance UUID; the first 8 passing candidates are started, the rest are `deferred` (mode untouched). `ReadoptAutonomousDrivers(ctx, instances, pass)` takes the pass number: pass 1 evaluates every candidate; follow-up passes (scheduled by Task 3.1.3f) re-evaluate only `deferred` and `unknown` candidates, and on the last follow-up (pass 4) still-over-cap candidates are cleared quietly with one Warn carrying the count and still-`unknown` candidates are left untouched with one Warn. The scheduling (60 s apart, at most 3, injected clock) is Task 3.1.3f; this task is pure and synchronous, so its tests call it directly.
- Per-instance logging: one structured line per candidate (`readopt candidate session=<title> result=adopted|would_adopt|reject|skip|unknown|deferred|start_failed gate=<name>`); Info for adopted/would_adopt/reject/skip/deferred, Warn for unknown and start_failed. Each line also carries `last_activity_age` (from the persisted `LastProgressAt`) so the dry-run shows which candidates the stale detector would kill next tick (the deferred `stale` gate, below, is decided from this).
- Adoption resets the turn budget (`AutonomousMaxTurnsOrDefault`) per restart; log it. Does not touch `AutonomousStopReason` or the resupervise budget. Adoption **stamps `stallRecoveryGuard`** for the candidate's item (30 min, item known from the `item_session_open` lookup) so the stale detector cannot kill a just-adopted session on its next tick (pre-mortem #8; RDP-16).
- Files: `server/services/autonomous_readoption.go`

##### Task 3.1.3f: Boot-pass trigger, fallback timer, follow-up passes, Step 7.6 seam (M) — AC2
- Split from 3.1.3c for sequencing (engineering G1): the gates and pass logic are testable without the dependency graph; the trigger needs the lifecycle ctx and the `server/dependencies.go` goroutine. Full scope kept.
- Step 7 / 7.5 / 7.6 goroutine seam (validation S5): the goroutine body in `server/dependencies.go` is extracted to a named function taking a `signalSettled func()` and returning on every path (`defer signalSettled()` as its first statement), so "every exit signals" is testable without running the dependency graph (RDP-14).
- Trigger: `maybeReadopt()` runs the first pass exactly once (`sync.Once`) when **both** (a) `SetLifecycleContext` has been called and (b) the instance list is loaded, wired **and settled**, whichever happens second. Verified ordering at base `6a395b58f`: instances are wired at `server/dependencies.go:881`; one goroutine (from `:932`) runs Step 7 controllers (`:1047`), Step 7.5 `StartSessionDriver` loop (`:1063-1080`), then Step 7.6 which sleeps 500 ms "to allow controllers to initialize their terminal readers" and runs `scanner.Scan` + `syncOrphanedApprovalsToQueue` (`:1082-1088`). Signal (b) is sent **after `:1088`** (after the settle sleep and scan, not after the Step 7.5 loop), so a re-adopted driver's first `waitForIdle` (settle 0) never reads a not-yet-initialised idle detector; signal it with a `defer` at the top of that goroutine so every exit path signals. Signal (a) comes from `SetLifecycleContext` (`server/server.go:1087`).
- Bounded fallback: when (a) arrives, `maybeReadopt` also arms a one-shot timer (const 90 s, injected clock). If (b) has not arrived when it fires, the first pass runs anyway, with `controller_present` failures classified `unknown` (the controllers may not have started) so a late dependency goroutine cannot cause a mass clear; the follow-up passes then re-evaluate. A pass never runs twice (`sync.Once`). The pass refuses to start any driver while `a.lifecycleCtx == nil` (otherwise `driverCtx()` would fall back to `Background()` and the driver would not stop on shutdown).
- Follow-up scheduler: after the first pass, if any candidate is `deferred` or `unknown`, schedule up to 3 follow-up passes 60 s apart through the `clock` seam (`time.AfterFunc` equivalent); each calls `ReadoptAutonomousDrivers` with the pass number and stops early when nothing is left to re-evaluate. No pass runs before `SetLifecycleContext` (it refuses to start any driver while the atomic ctx pointer is unset).
- `SetLifecycleContext` lives in `autonomous_driver_registry.go` after Task 3.1.3e; this task adds the single line that calls `a.maybeReadopt()` after the ctx is stored (never under `a.mu`). `SessionService.SetLifecycleContext` (`session_service_wiring.go:269-275`) already delegates by method name and is not changed; this task adds a sibling delegate `SessionService.NotifyInstancesSettled(instances)` there for signal (b), called from the extracted Step 7.6 function's `defer`.
- Files: `server/services/autonomous_readoption_trigger.go` (new), `server/services/autonomous_driver_registry.go` (one call), `server/services/session_service_wiring.go` (one delegate), `server/dependencies.go`, `server/server.go`

##### Task 3.1.3d: Tests (M) — AC2, AC4
- Files, one per concern so waves never share a test file: `server/services/autonomous_orchestration_service_class_test.go` (class-guard table: roles {triage, work, review, unrecognized, unlinked} x reasons {cancelled_external, cancelled_shutdown} x flag {ON, OFF}; pre-existing-open-row case; serialization round trip), `server/services/autonomous_driver_registry_test.go` (supersession, duplicate, stop-after-claim cancels, `startDriverIfAbsent` error table, `registerDriver` displacement, atomic `SetLifecycleContext` vs `driverCtx()` under `-race`, and the lock-safety / liveness cases REG-12..REG-16 behind a 5 s fail-not-hang watchdog helper), `server/services/autonomous_readoption_test.go` (gate table with the reject / skip / unknown class column, skip-without-clear on twice-run and on `ErrDriverPresent`, transient-probe-error leaves mode untouched and is re-evaluated, quiet clear has outcome `""`, cap with stable order and deferral to follow-up passes, flag OFF dry-run, start failure, invariant with its `unknown` exception, persisted `Hibernated` round trip, trigger-once-after-both, fallback timer runs once when (b) never arrives and never double-runs when (b) arrives first, every exit of the Step 7/7.5/7.6 goroutine signals (b)).
- Uses fakes (`fakeStarter`, in-memory `Storage`, `paneProbe` fake); no sleeps.
- Files: the three test files above

#### Story 3.1.4: Every stop is observable
**As an** operator, **I want** the stop reason, the pane's state, the turn count and a scrubbed tail in the stuck row and one log line, **so that** I can tell idle from errored from exited without opening the session.
**Acceptance Criteria** (AC2, AC3):
- *Given* `{StopReason: orchestrator_error, Turns: 2, Snapshot{Bucket: safe_idle, LastActivityAt: 12:03:10, PaneTail: "…password=hunter2…"}}`, *When* the callback runs for a linked work item, *Then* `MarkStuck` receives a context that **starts with the plain summary** (`The autonomous run hit an internal error and stopped. The session was waiting at its prompt. …`) followed by ` | stop_reason=orchestrator_error state=idle_prompt action=… turns=2 …`, containing `[REDACTED]` and not `hunter2`; exactly one `[AutonomousDriver] stopped` log line (filtered by this test's unique `session=` key) is emitted and contains no tail text.
- *Given* `startup_timeout` with `Turns: 0`, *Then* the notification body does not contain "stopped after 0 turns".
- *Given* each `StopReason` x `ObservedState` x action in the Operator-facing copy table, *Then* the summary equals the table's strings byte for byte (golden) and contains none of the wire codes (`StopReason` values, `ObservedState` values, `nudge_outcome` classes), so a developer-speak edit fails a test (UX G4, G5).
- *Given* a Nudge plan, *Then* the row's context shows `action=nudging` at plan time and, after the effect, `action=nudged nudge=1/2` (success) or `action=respawned` plus the plain failure phrase (fallback) or `action=none` plus the uncertain sentence; for every non-Nudge plan the row is written once with its final action (UX G1).
- *Given* a long unbroken token (a 300-character URL) in the context at a 360 px viewport, *Then* the "Why" value wraps and the page has no horizontal scroll (Task 3.1.4d; UX G6).
**Files**: `server/services/autonomous_orchestration_service.go`, `server/services/autonomous_stop_detail.go`, `server/services/autonomous_stop_detail_test.go`, `server/services/autonomous_orchestration_service_stop_test.go`, `web-app/src/components/backlog-stuck/StuckItemDetail.css.ts`, `web-app/src/components/backlog-stuck/StuckItemDetail.test.tsx`, `tests/e2e/backlog-stuck-items.spec.ts`

##### Task 3.1.4a: Log line and detail formatter (S) — AC2
- `formatStopDetail(outcome, state, action, nudge)` returns `<plainSummary(...)> | stop_reason=… state=… action=… nudge=n/cap turns=… resupervise=n/cap last_activity=… [nudge_outcome=…] tail=…` (the summary comes from Task 3.1.4b's `stopCopy`; the `key=value` part keeps the keys the tests and the `jq` query use) and `logStop(...)`; log exactly once per stop, after the **final** action is known (`action=none`/`respawn` equivalents until Story 3.2.3b lands, when the work arm's call moves to the exit of `handleWorkStall` so a Nudge that fell back to Respawn logs `respawn`; keys include `run_id`, `resupervise=<used>/<cap>`, `0/3` until Task 3.2.3a supplies the budget, `nudge_outcome` (`n/a` until 3.2.3b) and `resupervise_cap_hit`). Replace the hard-coded detail at L329-331; `MarkStuck`'s `(applied, err)` result is now captured and passed on (used by 3.2.3b: an error fails open to Respawn, a clean `applied == false` maps to `None`).
- Files: `server/services/autonomous_stop_detail.go`, `server/services/autonomous_orchestration_service.go`

##### Task 3.1.4b: Typed notification body (S) — AC2
- The `stopCopy` table from "Operator-facing copy (owner review)" above, as typed constants: `reasonSentence`, `stateSentence`, `actionSentence`, `nudgeFailurePhrase`, composed by `plainSummary`. Every enum value has a row (completeness test over the DOM-2 slice); replaces the L602 text; tail never included; titles per the copy table (`Autonomous session nudged` INFO, `Autonomous fix stuck` FAILURE unchanged for every other case). No turn count in the sentences (a count, when a caller adds one, reads "during turn N"; never "stopped after N turns" for a turn that did not complete). Nudge-failure classes map to operator phrases here and **only** here, so the class code never reaches a notification (UX G4); Task 3.2.3c passes the class, not a string.
- Files: `server/services/autonomous_stop_detail.go`, `server/services/autonomous_orchestration_service.go`

##### Task 3.1.4c: Tests (S) — AC2, AC4
- Detail contains the plain summary then the key=value part; canary absent from stuck row, notification and captured logs (`captureLogs` helper); real turn count shown; one log line per stop (with `nudge_outcome` and `resupervise_cap_hit` keys, OBS-2); completeness **and golden-string** tests for the copy table (OBS-4, OBS-10); **OBS-7**: with a real `Storage`, `MarkStuck` with a `StopDetail` then the `ListStuckBacklogItems` handler returns `StuckBacklogItem.context` equal to the detail (the AC2 operator read path ADR-002 names was previously untested).
- **Log-capture isolation (engineering G4, BUG-087).** `captureLogs` holds `slogDefaultMu` (`autonomous_orchestration_service_test.go:445-455`) so capturers do not overlap, but emitters in other parallel tests are not serialized and can write into the swapped default. Every assertion on captured logs here therefore (a) gives the test a unique session title (`"stall-" + t.Name()`), (b) filters records through a helper `recordsFor(buf, title)` that keeps only lines whose `session=` equals that title, and (c) asserts "exactly one `[AutonomousDriver] stopped`" and canary absence **on the filtered set**. The tests are not `t.Parallel()`. If a non-global logger seam lands first (BUG-087's option 2) the helper is replaced, not the assertions.
- Files: `server/services/autonomous_stop_detail_test.go`, `server/services/autonomous_orchestration_service_stop_test.go`

##### Task 3.1.4d: Operator "Why" value wraps long tokens (XS) — AC2 (UX G6)
- `web-app/src/components/backlog-stuck/StuckItemDetail.css.ts`: `value` (`:31-33`) has only `color`; the row is `display: flex; flex-wrap: wrap` with a `minWidth: 90px` label, and the value is a bare `<span>`. A pane tail or URL with no spaces is up to 1 KB, so add `overflowWrap: "anywhere"` and `minWidth: 0` to `value`. The ` ⏎ ` separators and spaces already allow normal wrapping; this only covers unbroken tokens. CSS only: no TSX change, so the `// +feature: backlog-diagnose-nudge` marker and the feature registry are untouched (`make registry-diff` stays empty).
- Tests: (1) `StuckItemDetail.test.tsx`: a case with a plain-sentence lead followed by a 300-character unbroken token renders both in `data-testid="stuck-item-why"` (jsdom cannot measure layout; this pins the data path, not the wrap). (2) `tests/e2e/backlog-stuck-items.spec.ts` (already `// @feature backlog:list-stuck`, uses `seedStuckItem({context})`): one test seeds a 300-character unbroken `context`, sets the viewport to 360 x 740, opens the card, and asserts `document.documentElement.scrollWidth <= window.innerWidth` through `page.evaluate`. Conventions kept: `data-testid` locators, no `waitForTimeout`, page helper reused.
- The UI still renders the context string verbatim; the lead sentence is the server's (Task 3.1.4a/b), so the "Why" line reads in plain language without a component change (UX G6).
- Files: `web-app/src/components/backlog-stuck/StuckItemDetail.css.ts`, `web-app/src/components/backlog-stuck/StuckItemDetail.test.tsx`, `tests/e2e/backlog-stuck-items.spec.ts`

### Epic 3.2: Nudge, re-supervise, or respawn
**Goal**: A work session that is only waiting at an idle prompt gets a bounded nudge and a re-attached driver; one that is still working keeps a driver and `AutonomousMode`; neither races a user's stop or the stale-work remediator; a respawn happens only when neither applies, and no path leaves `AutonomousMode == true` with no driver.

#### Story 3.2.1: Atomic claim of a nudge slot
**As a** maintainer, **I want** the cap check, the cooldown check and the reservation to be one atomic step, **so that** two stalls cannot both pass the cap.
**Acceptance Criteria** (AC3):
- *Given* an open `(item, autonomous_stuck)` row with `diagnose_nudge_count = 0`, cap 3, reserve 1 (so 2 claimable slots) and 8 goroutines calling `ClaimDiagnoseNudgeSlot` concurrently, *When* all return, *Then* exactly 2 returned `Claimed == true` and the row's count is 2.
- *Given* the row has `diagnose_next_eligible_at` in the future, *Then* `Claimed == false` even with count 0 (the cooldown is part of the same UPDATE, not a prior read); *Given* it is in the past or nil, *Then* claimable.
- *Given* no open row, *Then* `Claimed == false` with a nil error (fail closed).
- *Given* the MCP path `RecordDiagnoseNudgeAttempt`, *Then* its behaviour is unchanged (existing tests pass).
**Files**: `session/diagnose_nudge.go`, `session/diagnose_nudge_test.go`, `session/ent_repository_backlog.go`

##### Task 3.2.1a: `ClaimDiagnoseNudgeSlot` (S) — AC3, AC4
- `func (s *Storage) ClaimDiagnoseNudgeSlot(ctx, itemID, reason, reserve int) (ClaimResult, error)` with `ClaimResult{Claimed, JustCapped bool}` (struct, not two bools). Backed by a new `EntRepository.ClaimDiagnoseNudgeSlotIfEligible` whose UPDATE adds `AND (diagnose_next_eligible_at IS NULL OR diagnose_next_eligible_at <= now)` to the existing `count < maxAttempts-reserve` predicate. `RecordDiagnoseNudgeAttempt` and `RecordDiagnoseNudgeAttemptIfBelowCap` (MCP path) are untouched. No release/refund method exists (Deferred, Adv C5).
- `DiagnoseNudgeAllowed` gains a `reserve` parameter variant (`DiagnoseNudgeAllowedWithReserve`) so `planStall`'s read applies the same reserve the claim does.
- Files: `session/diagnose_nudge.go`, `session/ent_repository_backlog.go`, `session/diagnose_nudge_test.go`

#### Story 3.2.2: One pinned, guarded, cancellable nudge writer
**As a** security-minded maintainer, **I want** exactly one new raw-write caller with the same guards the MCP nudge has, that returns promptly when its ctx is cancelled, **so that** the autonomous path adds no unreviewed write surface and a stop wins.
**Acceptance Criteria** (AC3):
- Order is verify -> lease -> claim -> write, and a failed gate never consumes budget.
  - *Given* a status that is not a safe idle prompt (`StatusExecuting`), *When* `NudgeStalledAutonomousSession` runs, *Then* it returns `ErrNudgeTargetNotIdle`, the claim callback is invoked 0 times and no lease is left held (`AssertLeaseFree`).
  - *Given* the lease already held by another writer, *Then* it returns `ErrLeaseBusy` and the claim callback is invoked 0 times.
  - *Given* the claim callback returns `false`, *Then* it returns `ErrNudgeSlotUnavailable`, the lease is released and nothing is written.
  - *Given* an instance whose status is not `Active`, or that is archived or paused, *Then* it returns an error before the gate, with 0 claim calls and 0 writes.
- Cancellation (architecture C2).
  - *Given* a fake controller that reports a safe idle prompt continuously and a ctx cancelled 100 ms into the settle window, *When* the writer runs, *Then* it returns `context.Canceled` within the settle window (asserted by racing the call against a 1 s timer), zero `SendKeys` calls were made, the lease was never taken and the claim callback was invoked 0 times.
  - *Given* a ctx cancelled after the gate passes and before the lease, *Then* the same zero-write, zero-claim result.
- Submit uncertainty (architecture C3).
  - *Given* a claim that succeeded and a `SubmitDriverContent` that returns `ErrSubmitNotConfirmed`, or fails after the text was sent, *Then* the error wraps `ErrNudgeSubmitUncertain`, the lease is released and **no** release/refund of the slot occurs (the claim count stays incremented).
- The bytes written equal `autonomousStallNudgeText` exactly (no interpolation).
- `make test-delivery-guards` passes with the new pin rows and fails without them.
**Files**: `session/autonomous_stall_nudge.go`, `session/autonomous_stall_nudge_test.go`, `session/nudge_gate.go`, `session/nudge_gate_test.go`, `server/mcp/tools_diagnose_test.go`, `server/services/terminal_write_pins_test.go`, `server/services/terminal_write_callers_test.go`

##### Task 3.2.2a: `NudgeStalledAutonomousSession` and ctx-aware settle (M) — AC3
- `session/nudge_gate.go`: add `CheckNudgeEligibleWithSettleWindowCtx(ctx, inst, settleWindow, pollInterval)` that replaces `time.Sleep` with a `select` on `ctx.Done()` and a timer; `CheckNudgeEligibleWithSettleWindow` and `CheckNudgeEligible` delegate with `context.Background()` (MCP behaviour unchanged); `VerifyNudgeSafeToWrite(ctx, inst)` passes its ctx through (it already takes one, and only the tmux ownership check honoured it before: `nudge_gate.go:132-140`).
- `session/autonomous_stall_nudge.go`: signature `NudgeStalledAutonomousSession(ctx, inst *Instance, claim func() (bool, error)) error`. Steps: `GetLifecycleStatus() == Active` and not archived/paused, else error; `VerifyNudgeSafeToWrite(ctx, inst)`; `ctx.Err()` re-check; `inst.TryTerminalWriteLease(LeaseWriterNudge)`; `claim()`; `ctx.Err()` re-check before submit is `SubmitDriverContent`'s own first statement; `SubmitDriverContent(ctx, inst, lease, autonomousStallNudgeText, DefaultPaneSettlePollInterval, DefaultPaneSettleMaxWait)`; any submit error is wrapped `ErrNudgeSubmitUncertain`. Keep the lease and write calls inside this one function (no closures that call the primitives; the pin scanner keys units by function name).
- Test seam (validation S4): `NudgeStalledAutonomousSession` is a thin wrapper over an unexported `nudgeStalledWith(ctx, inst, claim, settleWindow, poll time.Duration, submit submitFn)`; the wrapper passes the 3 s `nudgeIdleSettleWindow`, `DefaultPaneSettlePollInterval` and `SubmitDriverContent`. The cancel-mid-settle test passes an hour-long window and a 1 ms poll so a regression to `time.Sleep` hangs the test (caught by the watchdog) instead of passing by luck. The lease and write calls stay inside the one pinned function the guard keys on (the wrapper or the inner function, whichever the pin row names; keep exactly one).
- `server/mcp` `writeNudge` is the other caller of the now-ctx-aware gate: add NDG-12, a test that a cancelled request ctx yields `context.Canceled` from the gate and `writeNudge` maps it to a defined error, not a panic or a "not idle" mislabel (adversarial minor M3).
- A fake-nudger service test cannot prove promptness or no-write; the tests above (session package, fake controller) do. State this in the test file header.
- Files: `session/nudge_gate.go`, `session/autonomous_stall_nudge.go`, `session/nudge_gate_test.go`, `session/autonomous_stall_nudge_test.go`, `server/mcp/tools_diagnose_test.go`

##### Task 3.2.2b: Pin rows (XS) — AC3
- Add `"session:NudgeStalledAutonomousSession": {kind: kindAcquirer, reason: "autonomous stall nudge: gates, one Try lease, atomic slot claim, fixed text; reached only from the service stall path behind diagnose_nudge_enabled"}` to `pinnedCallers`. It calls both a surface primitive (`SubmitDriverContent`) and a lease constructor (`TryTerminalWriteLease`), so it must be `kindAcquirer` and is picked up by the closed-acquirer test (T-RO-45).
- Add `"NudgeStalledAutonomousSession"` to `realCallerConfig().surface` (`terminal_write_callers_test.go`). Its single service-side caller (the adapter method) is pinned `kindChain` as `server/services:defaultStallNudger.NudgeStalledSession` **in Task 3.2.3a, the same commit that introduces the adapter** (a surface entry with no caller yet is fine; an unpinned caller would fail the guard).
- The `claim` callback built in the nudge effect calls only `Storage` methods, so no extra unit enters the guard.
- Run `make test-delivery-guards` (sinkguard tag; not part of plain `go test`).
- Files: `server/services/terminal_write_pins_test.go`, `server/services/terminal_write_callers_test.go`

#### Story 3.2.3: Service nudge branch
**As an** operator, **I want** an idle-at-prompt work stall nudged (bounded) and the driver re-attached, **so that** the session continues instead of being killed.
**Acceptance Criteria** (AC3, AC4):
- Nudge path.
  - *Given* a work `ItemSession` for item "Fix flaky retry test" (in_progress), an open `autonomous_stuck` row with `diagnose_nudge_count = 0`, `diagnose_nudge_enabled` **and** `autonomous_stall_nudge_enabled` on, a pane at `"Claude Code idle prompt showing ? for shortcuts"`, *When* the driver exits with `orchestrator_error`, *Then* the fake nudger is called once, `diagnose_nudge_count == 1`, `ItemSession.EndedAt == nil`, `remediation_attempts == 0`, the respawner is called 0 times, a driver is registered again, the instance has `AutonomousMode == true` throughout (never observed false), and an INFO "nudged" notification (not "Autonomous fix stuck") is published exactly once.
- Failure mapping (explicit, table-tested via `fallbackAfterNudge`). Rows are evaluated top to bottom, first match wins: in particular a cancelled recovery ctx (stop, delete, toggle-off, shutdown) takes precedence over every nudger error, so a cancel-after-send never reaches the submit-uncertain row and never restarts a driver.

  | Nudger outcome | Action | Why |
  |---|---|---|
  | success | stay nudged; `restartAfterRecovery` | |
  | recovery ctx cancelled (stop, delete, toggle-off, shutdown) or `canRestart` fails because a stop was requested | None: leave the pane; if the snapshot still has `AutonomousMode == true` and no driver is registered, `SetAutonomousStopped(cancelled_external)` (shutdown: per the readoption flag, as in Task 3.1.3b) | The stopper owns the outcome; mode is never left true with no driver (adversarial C4b) |
  | instance not `Active`, archived or paused at the nudger | None, same mode rule | A pause during the window cannot be undone |
  | `ErrNudgeTargetNotIdle` (pane changed state) | Resupervise if headroom, else Respawn | Pane is now working or exited |
  | `ErrLeaseBusy` (another writer holds the pane) | Resupervise if headroom, else Respawn | |
  | ownership mismatch from `VerifyNudgeSafeToWrite`, or dead pane / no controller | Respawn | The pane is not ours or gone |
  | `ErrNudgeSlotUnavailable` (claim lost: cap, cooldown or row gone) | Respawn | Nothing was written |
  | `ErrNudgeSubmitUncertain` (includes `ErrSubmitNotConfirmed`; cancel-after-send only when the recovery ctx is still live, otherwise the ctx-cancelled row above wins) | **Leave the pane**: `SetAutonomousStopped(<stall reason>)`, stuck row stays, notification "nudge delivery uncertain"; never Resupervise (the pane may hold unsubmitted typed text and a re-supervised driver's first turn would append to it), never Respawn, slot stays consumed | The text may have landed or sit unsubmitted; killing a pane that may now be working on it, writing into it again, or refunding a slot for a real write is wrong (architecture C3, adversarial C5) |
  | successful nudge but `restartAfterRecovery` returns an error (`ErrNoDriverPool`, `Start` failure, successor registered) | successor registered: None (the successor owns the pane); otherwise Respawn with mode cleared | Mode is never left true with no driver (architecture C7) |
  | second completion for a title with a recovery already registered (same RunID) | None | `claimCompletion` returned `duplicate`; the winner is acting |

  - *Given* a fake nudger returning each outcome above, *Then* the action matches the table, `diagnose_nudge_count` is never decremented, and a Respawn executes the original body (ItemSession ended, `RemediationDue` consulted, respawn dispatched) with `AutonomousMode` cleared via `SetAutonomousStopped`.
- Scope by role (architecture C4).
  - *Given* a review-role, triage-role, unrecognized-role or unlinked stall at `idle_prompt` with `diagnose_nudge_enabled` on and headroom, *Then* the nudger and the driver restarter are called 0 times and the outcome is today's handling for that arm.
- Fallbacks.
  - *Given* the same work case with `diagnose_nudge_enabled` off, *Then* behaviour equals today's (ItemSession ended, `RemediationDue` consulted, respawn dispatched). *Given* `diagnose_nudge_enabled` **on** and `autonomous_stall_nudge_enabled` off (the live config's current state), *Then* the same today's-respawn result, the nudger is never called, and the stopped log line says `nudge_outcome=disabled`. *Given* the stall flag on and `diagnose_nudge_enabled` off, *Then* the same (the kill switch wins). *Given* `diagnose_nudge_count == 2` (cap 3, reserve 1), *Then* respawn. *Given* `needs_approval`, *Then* the nudger is never called. *Given* the item is no longer `in_progress` (e.g. `review`, the pane idle after `request_review`) with either state `idle_prompt` or `working`, *Then* neither the nudger nor the restarter is called and today's Respawn body runs (STL-18). *Given* `MarkStuck` returned `(applied == false, err == nil)`, *Then* the nudger is never called (`None`, Warn naming the item status). *Given* `MarkStuck` returned a non-nil error (e.g. `SQLITE_BUSY`), *Then* the handling fails open to the original Respawn body exactly as today (log Warn, still end the ItemSession and respawn), never `None` (STL-17). *Given* `IsRetryPending() == true`, *Then* neither nudge nor respawn. *Given* the row was reopened by `MarkStuck` with count already at the cap, *Then* the post-`MarkStuck` re-read downgrades to Respawn before any settle wait (nudger called 0 times).
- Concurrency.
  - *Given* 8 concurrent completions for the same item with cap 3 (distinct titles share the item), *Then* the nudger's write count is <= 2 and `diagnose_nudge_count <= 2`.
- Notification contract.
  - *Given* a Nudge or Resupervise result, *Then* `handleWorkStall` returns `stallResult{Notify: {Suppress: true}}` (or an INFO override for a successful nudge) and the outer function publishes no FAILURE notification; *Given* Respawn after a nudge failure, *Then* the generic notification body carries the operator phrase for the failure class from the copy table (`... A prompt could not be sent (the session was busy again), so a fresh session is being started instead. ...`) via `notifyOverride`, and contains no class code (`lease_busy`, `slot_unavailable`, ...); the code is only in the log line's `nudge_outcome` (UX G4).
- Row annotation (UX G1).
  - *Given* a Nudge plan that succeeds, *Then* the open row is written at plan time with `action=nudging` and refreshed once with `action=nudged nudge=n/cap`, and is **not** resolved (it holds `diagnose_nudge_count`); *Given* a Nudge that falls back to Respawn, *Then* the refresh carries `action=respawned` and the plain failure phrase; *Given* delivery uncertain, *Then* `action=none` and the "check it" sentence; *Given* the refresh write errors, *Then* a Warn is logged and the recovery outcome is unchanged (annotation is best-effort, never a gate).
- Recovery ctx vs bookkeeping (pre-mortem #6).
  - *Given* a Nudge whose recovery ctx is cancelled (stop arrives) and whose fallback is Respawn, *Then* `UpdateItemSessionEnded`, `MarkStuck` and `RemediationDue` still run and the fake storage records `ctx.Err() == nil` on each of them (STL-16).
- Instance write and event (pre-mortem #7).
  - *Given* each action, *Then* the instance write and its `SessionUpdatedEvent` happen once at plan time and the event payload equals the post-write snapshot; a Nudge that falls back to Respawn adds exactly one second write (`SetAutonomousStopped`) and one second event; a late fallback write never zeroes a successor driver's turn counters because it is skipped when a driver is registered (STL-12).
**Files** (per task, engineering G8; no two tasks of one wave share a file): 3.2.3a `autonomous_stall_recovery.go`, `autonomous_orchestration_service.go`, `config/config.go`, `terminal_write_pins_test.go` (the `kindChain` row; 3.2.2b edited the same test file in an earlier wave); 3.2.3b `autonomous_orchestration_service.go`, `autonomous_stall_recovery.go`, `autonomous_stall_resupervise.go` (stub, filled in 3.2.4a), plus a call to `stall_recovery_guard.go` (created in 3.2.5a); 3.2.3c `autonomous_stall_nudge_effect.go`; 3.2.3d `autonomous_stall_recovery_nudge_test.go`, `autonomous_stop_detail_test.go` (golden strings consumed here), `feature_flag_service.go`. All under `server/services/` except `config/config.go`

##### Task 3.2.3a: Seams, budget type, adapter (S) — AC3
- `StallNudger` interface + `SetStallNudger`; `defaultStallNudger` struct (plain method `NudgeStalledSession`, no closures) calls `session.NudgeStalledAutonomousSession`, plus its `kindChain` pin row `server/services/defaultStallNudger.NudgeStalledSession` in the same commit (Task 3.2.2b). The restart seam is a `driverStarter` interface over `startDriverIfAbsent` (error-returning), not a void func, so tests observe re-supervision and failures.
- `resuperviseBudget` type (map under its own mutex, cap const 3, `Used(key)`, `Allowed(key)`, `Increment(key)`, `Delete(key)`) is created here so `planStall` can read it in 3.2.3b; its effect is wired in 3.2.4a. Key: `ItemSession.ID`. Entries are deleted on `done` and wherever `UpdateItemSessionEnded` is called for the work session (adversarial M2).
- Flag seam (validation S1). `AutonomousOrchestrationService` holds three injectable `func() bool` fields, defaulting to the live config readers: `diagnoseNudgeEnabled` (`config.EffectiveDiagnoseNudgeEnabled(config.LoadConfig())`), `stallNudgeEnabled` (`config.EffectiveAutonomousStallNudgeEnabled(config.LoadConfig())`) and `readoptionEnabled` (`config.EffectiveAutonomousDriverReadoption(...)`, introduced in Task 3.1.3b). `config.LoadConfig()` resolves the config dir from the environment and the only other way to flip a flag in a test is `t.Setenv`, which forbids `t.Parallel()`; no existing test sets `diagnose_nudge_enabled`, so there is no precedent to copy. `planStall` reads the first two at call time and ANDs them.
- Config (pre-mortem P1 #2): `AutonomousStallNudgeFeatureFlag = "autonomous_stall_nudge_enabled"`, the constant `autonomousStallNudgeDefault = false` and `EffectiveAutonomousStallNudgeEnabled(cfg *Config) bool` (via `GetFeatureFlagWithDefault`, same shape as `EffectiveDiagnoseNudgeEnabled`, `config/config.go:555`). The registry entry is added in Task 3.2.3d; no env var anywhere.
- Files: `server/services/autonomous_stall_recovery.go`, `server/services/autonomous_orchestration_service.go`, `config/config.go`, `server/services/terminal_write_pins_test.go`

##### Task 3.2.3b: `handleWorkStall` owns plan and dispatch (M) — AC3
- `handleWorkStall(c completion, storage, item, is, outcome) stallResult` is called only from the Work arm of a backlog-linked session for `Class() == Stall`; it owns `planStall`, the work-role `MarkStuck`, and the entire `switch plan` (Respawn, None, Nudge, Resupervise). The outer function passes `completion`, `is`, `item`, `outcome` and consumes the `stallResult`. Glossary, 3.1.1a and this task agree on this.
- `planStall(inst, item, is, outcome) domain.StallAction` runs **before** `MarkStuck` (a Resupervise must not write a stuck row). Input: `State = domain.ClassifyObserved(outcome.Snapshot)`; `NudgeEnabled = a.diagnoseNudgeEnabled() && a.stallNudgeEnabled()` read at this instant (Task 3.2.3a seams); `NudgeAllowed` = `DiagnoseNudgeAllowedWithReserve(...,1)` (error => false, fail closed; the atomic claim is authoritative); `ResuperviseAllowed = budget.Allowed(is.ID)`; `ItemInWorkStatus = session.BacklogStatus(item.Status) == in_progress` (the same predicate as the boot gate `item_status_matches_role`, one shared function); `Archived = inst.IsArchived()`; `RetryPending = inst.IsRetryPending()`. `planStall` also records whether the plan would have been `Nudge` with both flags on (a second pure `DecideStallAction` call), which feeds `nudge_outcome=disabled`.
- Row `action=` (UX G1): the plan-time `MarkStuck` context uses `action=nudging` for a Nudge plan and the final value (`respawned`, `none`) for every other plan. After the nudge effect (Task 3.2.3c) finishes, a Nudge plan refreshes the same open row once more through `MarkStuck` with the final `action=` (`nudged`, `respawned`, `none`); the refresh is best-effort (Warn on error) and uses the outer `Background()` ctx. SVC-12's "exactly once per stall" therefore holds for every plan except a Nudge, which is exactly twice.
- `MarkStuck` runs here for every plan except `Resupervise` and `None`-because-archived/retry-pending (adversarial M4). Its result is captured as `(applied, err)`: **`err != nil` fails open to the original Respawn body** (log Warn, as today's "additive, never a gate" comment at `:328-336`; never `None`, which would leave an open ItemSession, no driver and no row until the 2 h stale detector); `applied == false` with `err == nil` (the item status moved on between the read and the write) downgrades a `Nudge` plan to `None` with a Warn naming the item status, because `MarkStuck`'s precondition is evaluated against the status read a moment earlier. For a `Nudge` plan the nudge allowance is re-read after `MarkStuck` and a now-false value downgrades to `Respawn` before the settle wait (adversarial M3). Review, triage, unrecognized-role and unlinked sessions keep today's top-level `MarkStuck` (guarded by `Class() == Stall`, Task 3.1.3b) and never reach `handleWorkStall`.
- The instance write (`applyInstanceStop`, which also publishes the `SessionUpdatedEvent`, Task 3.1.2b) is called **once, at the moment the plan is known and before any effect**, not from a function-exit defer (a late defer would run after the nudge/restart effects, publish a stale event, and could zero a successor driver's turn counters): `Nudge`/`Resupervise` -> `SetAutonomousStopObserved`; `Respawn`/`None` -> `SetAutonomousStopped`. A later fallback from Nudge to Respawn performs one explicit second `SetAutonomousStopped` (and event), skipped when a driver is already registered under the title. Between the callback's start and the plan the mode simply stays as the driver left it, and `canRestart` covers the window.
- Ctx rule (pre-mortem #6): `handleWorkStall` receives the outer `ctx` (`Background()`) and the `completion`; `completion.ctx` is passed **only** to the nudger, the settle wait and `canRestart`/`restartAfterRecovery`; `MarkStuck`, `MarkStuckNotified`, `ClaimDiagnoseNudgeSlot`, `UpdateItemSessionEnded`, `RemediationDue` and the item re-read use the outer ctx.
- Guard stamp: immediately after `planStall` returns `Nudge`/`Resupervise` (before `MarkStuck`), `stallRecoveryGuard.Mark(item.ID, 30m)`; every exit that does not end with a registered driver (leave-the-pane, Respawn fallback, cancel, `None`) re-marks with 2 min so the stale detector is deferred only while a recovery is live.
- The single `[AutonomousDriver] stopped` line (Task 3.1.4a) is emitted from the exit of `handleWorkStall` with the final `action`, `nudge_outcome` and `resupervise_cap_hit`.
- `switch plan`: `Respawn` -> the extracted original body unchanged; `None` -> log reason, fall through (stuck row stays, generic notification fires only for a real stall); `Resupervise` -> `a.resupervise(...)` (a stub returning `{Respawn}` here, replaced in 3.2.4a); `Nudge` -> 3.2.3c.
- Files: `server/services/autonomous_orchestration_service.go`, `server/services/autonomous_stall_recovery.go`, `server/services/autonomous_stall_resupervise.go`

##### Task 3.2.3c: Nudge effect, failure mapping, notification (M) — AC3
- `autonomous_stall_nudge_effect.go`: on a `Nudge` plan, in order: `canRestart(rec, inst)` once more immediately before invoking the nudger (architecture C1: not only before restart; the guard was already stamped at plan time, Task 3.2.3b); run the nudger under the completion ctx with `claim = func() (bool, error) { r, err := storage.ClaimDiagnoseNudgeSlot(ctx, item.ID, reason, 1); return r.Claimed, err }` (Storage call only, no refund); map the error through `fallbackAfterNudge` (table above). The nudge path must not call `UpdateItemSessionEnded` or `RemediationDue`.
- On success: `canRestart`, `restartAfterRecovery(rec, inst, 5*time.Minute)`; on error fall through the table; on success refresh the guard stamp, publish INFO "Autonomous session nudged" with the copy table's body (`n/cap`, not the tail), and refresh the stuck row to `action=nudged nudge=n/cap` (annotate, do not resolve: the row holds the nudge counter); `SetAutonomousMode(true, "")` is not called (mode never went false).
- Fallbacks pass the nudge-failure **class** (not a string) to `stopCopy`; the notification phrase, the row's `action=respawned` text and the `nudge_outcome=<class>` log key are derived from that one value, and only the log key shows the code (UX G4).
- Files: `server/services/autonomous_stall_nudge_effect.go`

##### Task 3.2.3d: Tests and flag description (M) — AC3, AC4
- Table-driven service tests for every AC above using a real in-memory `Storage` (`createTestStorage`), a fake `StallNudger`, a fake respawner (channel-signalled, as in the existing respawn tests), a fake `driverStarter`; the 8-goroutine concurrency test uses a start barrier, not sleeps. The role-scope test (review/triage/unrecognized/unlinked at `idle_prompt`) lives here.
- Update the `diagnose_nudge_enabled` description in `feature_flag_service.go` to cover the in-process autonomous nudge (the kill switch for every autonomous pane write); register `autonomous_stall_nudge_enabled` next to it reading the default constant exactly as the `autonomous_driver_readoption` entry reads `autonomousDriverReadoptionDefault`, with a description that states default off and that it is AND-ed with `diagnose_nudge_enabled`; run the feature-flag tests (STL-19: registered, default is `false` asserted as a literal so flipping the shipped default means editing a test, and setting it live changes the next `planStall`).
- The added cases from the pre-mortem and validation: STL-8 (archived), STL-12 (write+event once per plan, fallback adds one), STL-14 (resupervise writes no row), STL-15 (callback panic), STL-16 (recovery ctx cancelled, bookkeeping persists), STL-17 (`MarkStuck` error fails open), STL-18 (item not `in_progress`), STL-7 extended with the flag-combination rows, STL-11 with the combined-flag and `ItemInWorkStatus` assembly.
- Copy and row cases (triad UX G1, G4, G5): STL-20 (row annotation per final action; refresh error is a Warn, outcome unchanged), STL-21 (no nudge-class code in any notification body or row summary; failure phrases exact), STL-10 extended with the golden title/body for nudged, respawned, respawned-after-failed-nudge, uncertain.
- Files: `server/services/autonomous_stall_recovery_nudge_test.go`, `server/services/feature_flag_service.go`

#### Story 3.2.4: Re-supervise a still-working session (bounded)
**As an** operator, **I want** a work session whose driver stopped while the agent is still working to keep a driver and its autonomous mode, **so that** approvals keep being handled and nothing waits two hours for the stale detector.
**Acceptance Criteria** (AC3):
- *Given* a work `ItemSession` and a driver that exits `startup_timeout` with snapshot state `working`, *When* the callback runs, *Then* a driver is registered again (5 min warm-up), `AutonomousMode == true` and the LLM approval gate (`SetAutonomousChecker`) still returns true, no `autonomous_stuck` row is written, `ItemSession.EndedAt == nil`, the nudger and respawner are called 0 times, no notification is published, and one log line carries `action=resupervise`, `resupervise=1/3` and `prior_stop_reason=startup_timeout`.
- *Given* the same stop happens 4 times for one `ItemSession` (cap 3), *Then* the 4th is `Respawn` (ItemSession ended, respawner called, stuck row written with `resupervise_cap_reached runs=3 last_stop_reason=startup_timeout` in its context) so the item is never left driverless.
- *Given* `startDriverIfAbsent` returns an error (pool nil or `Start` failure) during a Resupervise, *Then* the budget is not incremented and the Respawn body runs (ItemSession ended, stuck row, respawner called, `AutonomousMode` cleared); mode is never left true with no driver (architecture C7, adversarial C3).
- *Given* a toggle-off then on registered a successor D2 during the window, *Then* `startDriverIfAbsent` returns `ErrDriverPresent`, no second driver starts and D2 stays registered.
- *Given* a non-backlog (unlinked) autonomous session with the same stop, *Then* handling is today's (mode cleared, outcome `stuck`, generic notification); it is **not** re-supervised and not nudged (defined, architecture C4c).
- *Given* a nudge that succeeded and whose re-supervised driver ends `startup_timeout`/`working`, *Then* it is re-supervised (the "nudge, then a long turn" case), consuming re-supervise headroom, not nudge slots.
- *Given* a `max_turns` or `panic` stop with state `working` or `idle_prompt`, *Then* Respawn (row 4): neither re-supervise nor nudge.
- *Given* a `working` stop for an item that is no longer `in_progress`, *Then* it is not re-supervised and today's Respawn body runs (its `AutoRespawnAutonomousWork` re-checks `in_progress`).
**Files**: `server/services/autonomous_stall_resupervise.go`, `server/services/autonomous_stall_recovery_resupervise_test.go`

##### Task 3.2.4a: Effect (M) — AC3
- `a.resupervise(c, storage, inst, item, is, outcome) stallResult` in `autonomous_stall_resupervise.go` (the guard was stamped at plan time in Task 3.2.3b); `canRestart`; `SetAutonomousStopObserved(reason)` (already applied once at plan time by `applyInstanceStop`, Task 3.2.3b; there is no function-exit defer, pre-mortem #7); `restartAfterRecovery(rec, inst, 5*time.Minute)`; on success increment `budget` and refresh the stamp, return `{Resupervise, Notify: Suppress}`; on error return `{Respawn}` with the `last_stop_reason` and run count carried into the stuck row context (`resupervise_cap_reached runs=N last_stop_reason=<reason>` when the cap is what sent it to Respawn). Log `resupervise=n/cap` on every run so the chain is reconstructible (adversarial C6).
- Files: `server/services/autonomous_stall_resupervise.go`

##### Task 3.2.4b: Tests (S) — AC3, AC4
- Table test over (reason x working) for budget exhaustion, start failure, successor-registered, unlinked, `max_turns` and `panic` cases (neither re-supervises; both Respawn); the exhausted case asserts the stuck row's context contains `resupervise_cap_reached` and the log line `resupervise_cap_hit=true`; an item not `in_progress` is not re-supervised (RSP-6).
- Files: `server/services/autonomous_stall_recovery_resupervise_test.go`

#### Story 3.2.5: Recoveries are cancellable and defer the stale remediator
**As an** operator, **I want** stop/delete/toggle-off to win over an in-flight nudge, and the stale-work remediator not to kill (or burn an attempt on) a pane that was just nudged, **so that** automation never fights my action or itself.
**Acceptance Criteria** (AC3, requirements non-goal):
- Stop during nudge.
  - *Given* a fake nudger blocked inside `NudgeStalledSession`, *When* `StopDriverForSession(title)` (or `stopAndDeregisterDriver`) is called, *Then* the nudger's ctx is cancelled, it returns, no driver is registered afterwards, the restarter is called 0 times and `AutonomousMode` is not set back to true.
  - *Given* a stop arriving after `claimCompletion` returned but before the item lookup finished (a fake storage that blocks in `GetItemSessionBySessionUUID`), *Then* the nudger is never invoked or is invoked with an already-cancelled ctx and writes nothing (architecture C1).
  - *Given* the instance becomes paused, archived or non-`Active` while the nudger is blocked (pause does not call `stopAndDeregisterDriver`), *Then* `canRestart` fails and no driver is started.
  - *Given* a toggle off then on during the window registered a successor driver D2, *Then* the recovery aborts without registering another and D2 stays registered.
- Stale remediation (architecture C6).
  - *Given* an in_progress item whose active work session has `LastProgressAt` three hours old (stale), a recovery in flight or stamped within the 30 min window, and a real `Storage`, *When* the detector's `remediateStaleWorkWithBackoffGate` runs, *Then* `RemediationDue` is not called (the open `stale_work` row's `remediation_attempts` is unchanged), and `RemediateStaleWorkSession` is not dispatched; *Given* the window has expired (fake clock), *Then* it proceeds as today (existing tests unchanged).
  - *Given* the guard is stamped and the remediator is called directly, *Then* it returns nil without `UpdateItemSessionEnded`, `KillTmuxPaneOnly` or `AutoRespawnAutonomousWork`, but a stale item with no active session (`active == nil`) is still respawned (adversarial M1).
**Files**: `server/services/stall_recovery_guard.go`, `server/services/backlog_service.go`, `server/services/backlog_service_triage.go`, `server/services/session_service_wiring.go`, `session/backlog_lifecycle_stale.go`, tests as listed per task

##### Task 3.2.5a: `stallRecoveryGuard` and wiring (S) — AC3
- `stallRecoveryGuard{mu, until map[string]time.Time, now func() time.Time}` with `Mark(itemID, d)` / `Deferred(itemID) bool`; window const 30 min. Injected into `AutonomousOrchestrationService` (field + `SetStallRecoveryGuard`) and `BacklogService` at the wiring site that already links the two services. `BacklogService` implements `session.StaleWorkDeferrer` (`StallRecoveryDeferred(itemID) bool`) over it. The recovery marks at plan time (Task 3.2.3b, before `MarkStuck`), refreshes on success (Tasks 3.2.3c, 3.2.4a) and re-marks with 2 min on every exit that ends with no registered driver; entries lapse by time, never by a missed unmark. Startup wiring assertion (pre-mortem #12): at the `SetStaleWorkRemediator` wiring site, if the wired remediator does not satisfy `session.StaleWorkDeferrer` log Error ("stale-work deferral inert") instead of failing silently; DEF-5 pins it in the wiring test.
- Files: `server/services/stall_recovery_guard.go`, `server/services/backlog_service.go`, `server/services/session_service_wiring.go`, `server/services/stall_recovery_guard_test.go`

##### Task 3.2.5b: Gate the stale detector before `RemediationDue` (S) — AC3
- `session/backlog_lifecycle_stale.go`: add the optional `StaleWorkDeferrer` interface; in `remediateStaleWorkWithBackoffGate`, right after the `remediator == nil` check and **before** `l.storage.RemediationDue`, `if d, ok := remediator.(StaleWorkDeferrer); ok && d.StallRecoveryDeferred(itemID) { log Info; return }`. Rationale: `RemediationDue` records the attempt as it grants (`session/backlog_remediation.go:277-320`), so a check after it burns one of the `stale_work` attempts and can park the item.
- `backlog_service_triage.go` `RemediateStaleWorkSession`: add a defence-in-depth `Deferred(itemID)` check after the `active == nil` -> `AutoRespawnAutonomousWork` branch and before `UpdateItemSessionEnded` (so the safety-net respawn for an item with no active session is never deferred).
- Files: `session/backlog_lifecycle_stale.go`, `session/backlog_lifecycle_stale_deferral_test.go`, `server/services/backlog_service_triage.go`, `server/services/backlog_service_stall_deferral_test.go`

##### Task 3.2.5c: Stop-during-recovery tests (M) — AC3, AC4
- Deterministic stop-during-nudge test: the fake nudger blocks on a channel until its ctx is cancelled; the test calls `StopDriverForSession`, then asserts the outcomes above. Pause/archive/non-Active/successor cases use the same fake with the instance mutated while blocked. The pre-lookup stop case uses a storage fake that blocks. Guard tests use the injected clock; no sleeps.
- Files: `server/services/autonomous_stall_recovery_stop_test.go`

---

## Phase 4: Verify

### Epic 4.1: Gates and reconciliation
**Goal**: Everything the repo requires before pushing passes, and the diagnosis document matches what was built.

#### Story 4.1.1: Reconcile docs and run the gates
**As a** maintainer, **I want** the document and the code to agree and every gate green, **so that** the PR is mergeable without a second pass.
**Acceptance Criteria** (AC1, AC4):
- *Given* the finished branch, *When* the gates below run, *Then* each exits 0 and its output is pasted into the PR description (not claimed).
**Files**: `docs/explanation/autonomous-stall-causes.md`

##### Task 4.1.1a: Reconcile the diagnosis document (S) — AC1
- Replace pre-change line references with permalinks at the branch SHA; map each `StopReason` to its driver site by symbol name; confirm the decision table matches `DecideStallAction`.
- Files: `docs/explanation/autonomous-stall-causes.md`

##### Task 4.1.1b: Run the gates (S) — AC4
- `make build` (generated proto/ent code first), then `go build ./...`; `go test ./session/domain/... ./session/... ./server/services/... -race` (targeted by package; `-timeout=20m` for `server/services`); `make lint`; `make ready-complexity-gate`; `make test-delivery-guards`; `make actor-field-guard`; `make registry-diff` (expect no change: no RPC, proto or component change); `make lint-custom` (includes `noarchivedrevival`, `silenttransition`); feature-flag registry tests for `autonomous_driver_readoption`.
- web-app (Task 3.1.4d is the only web-app change, CSS only): `cd web-app && pnpm exec jest --testPathPatterns StuckItemDetail --no-coverage` (pnpm, never npm: `docs/how-to/use-pnpm-in-web-app.md`), `pnpm run lint:duplicates` (jscpd margin; the new test adds no `jest.mock` block), and the one e2e spec `cd tests/e2e && npx playwright test backlog-stuck-items.spec.ts`. `make registry-diff` stays empty (no RPC, component or marker change).
- **Suite runtime (engineering G4).** The plan adds roughly 90 service-package tests (REG 16, RDP 18, OBS 10, STL 21, RSP 8, STP 5, DEF 6, SVC 12 in `validation.md`) plus ~15 driver/domain rows; `server/services -race` already needs `-timeout=20m` locally and CI runs it under `-timeout=1200s` (`.github/workflows/mcp-integration.yml`; BUG-067 is stale). Record the before/after wall time of `go test ./server/services -race` in the PR; the two watchdog helpers (`requireReturns`, 5 s) never wait on a passing run, and tests use fakes and an injected clock, so growth should be CPU-bound only. If the package exceeds the CI soft wrapper, split by build tag in a follow-up, not by deleting rows.
- Known flakes on a red gate: `TestAutonomousDriver_Stop_CancelsLoop_DuringNudgeSuppression` (BUG-079, driver file) and any `captureLogs`-based assertion in a test **not** added by this plan (BUG-087); rerun once and name the bug in the PR. A failure in a test this plan added is never attributed to them.
- Ship checklist (product): (1) the follow-up backlog item for the stale-work and health-checker restart paths is filed from the spec block in Deferred and its ID is pasted into the PR description; (2) the three review dates (flip decision = merge + 7 d, soak review = flip + 14 d, sunset = merge + 30 d) are written into the PR description and a reminder is set; (3) `requirements.md` "Falsifiable hypothesis" is unchanged or its edit explained.
- Files: none

---

## Acceptance-criteria traceability

| AC | Requirement | Tasks |
|----|-------------|-------|
| AC1 | Root cause identified and documented, each stop path cited | 1.2.1a, 1.2.1b, 4.1.1a |
| AC2 | Each stop's reason recorded and observable (stuck-row context via `ListStuckBacklogItems`, log line, in-memory field); distinguishes idle/awaiting-input, error, completed/exited; the operator-facing text is plain language and states whether the operator must act | 1.1.1a, 1.1.1b, 2.1.1a, 2.1.1b, 3.1.2a, 3.1.2b, 3.1.3a, 3.1.3b, 3.1.3c, 3.1.3d, 3.1.3e, 3.1.3f, 3.1.4a, 3.1.4b, 3.1.4d |
| AC3 | State inspected and captured before restart; nudge chosen over respawn when only waiting; working session re-supervised; stop/stale races closed. **Scope: driver-stop paths**, which is what the AC text names; the stale-detector and health-restart paths are a recorded follow-up item (Deferred, "AC3 scope") | 1.1.1b, 1.1.2a, 1.1.2b, 2.1.1a, 3.1.1a, 3.2.1a, 3.2.2a, 3.2.2b, 3.2.3a, 3.2.3b, 3.2.3c, 3.2.4a, 3.2.5a, 3.2.5b |
| AC4 | Tests cover classification and nudge-vs-respawn-vs-resupervise per state | 1.1.1c, 1.1.2c, 2.1.1c, 3.1.3d, 3.1.4c, 3.1.4d, 3.2.1a, 3.2.2a, 3.2.3d, 3.2.4b, 3.2.5c, 4.1.1b |

Requirement mapping: R1 -> AC1 tasks; R2 -> 1.1.1a, 2.1.1b, 3.1.2, 3.1.4; R3 -> 1.1.1b, 2.1.1a; R4 -> 1.1.2, 3.1.4a; R5 -> 1.1.1b, 3.2.x; R6 -> 1.1.1c, 3.2.3d, 3.2.4b, 3.2.5c. Constraint "external stop is not a stall" -> 3.1.3a, 3.1.3b, 3.1.3d, 3.2.5c. Review findings: round-1 B1 -> 3.2.4a/b; B2 -> 3.1.3a/b/c/d/e/f; B3 -> 3.1.3a, 3.2.5c; B4 -> 3.2.5a/b/c; round-2 B5 -> 3.1.3c/d; B6 -> 3.1.3b/d; architecture C1 -> 3.1.3a, 3.2.3c, 3.2.5c; C2 -> 3.2.2a; C3 -> 3.2.2a, 3.2.3c; C4 -> 3.2.3b, 3.2.3d, 3.2.4b; C6 -> 3.2.5b; C7/C8 -> 3.1.3a, 3.2.4a; C9 -> Dependency Visualization. Triad repair (iteration 5): UX G1 -> 3.1.4a, 3.2.3b, 3.2.3c; UX G2 -> Risk Control + Deferred (no task, SVC-4/SVC-8 already assert it); UX G3 -> ADR-002 + Deferred; UX G4 -> 3.1.4b, 3.2.3c, 3.2.3d; UX G5 -> Operator-facing copy, 3.1.4b, 3.1.4c; UX G6 -> 3.1.4a/b, 3.1.4d; engineering G1/G2 -> 3.1.3a/e/c/f; G3 -> 2.1.1c; G4 -> 3.1.4c, 4.1.1b; G5 -> 1.1.2b/c; G6 -> 3.2.4a; G7 -> 3.1.2a/b; G8 -> Story 3.2.3 Files; product -> requirements.md, Observability Plan (success target, delete thresholds, review dates), Risk Control (risky assumptions), Deferred (follow-up spec), 4.1.1b (ship checklist).

---

## Effort estimate

Cost classes per `~/.claude/skills/sdd/skills/ESTIMATION.md`; bands and weights are INFERRED, recalibrate after the first implementer run. CU = raw tokens x (1 + 4 x output share), caching ignored (conservative).

| Story | Tasks | Classes | Raw tokens | Output share | CU (k) | Wall-clock blocker |
|-------|-------|---------|-----------|--------------|--------|--------------------|
| 1.1.1 domain types/decide | 3 | XS, S, S | 120k | 15% | 192 | |
| 1.1.2 tail + redaction (+ fail-closed rules and residual canaries) | 3 | S, S, S | 150k | 12% | 222 | |
| 1.2.1 diagnosis doc | 2 | S, M | 175k | 5% / 15% | 255 | |
| 2.1.1 driver finish() (+ BUG-079 baseline and hygiene) | 3 | M, M, M | 365k | 12% | 540 | |
| 3.1.1 extraction + characterization | 1 | S | 70k | 8% | 92 | |
| 3.1.2 setters (+ error return, INS-6) | 2 | S, S | 125k | 12% | 185 | |
| 3.1.3 registry / lifecycle ctx / class guards / re-adoption gates / trigger / tests (3.1.3a and 3.1.3c split into 3.1.3a+e and 3.1.3c+f) | 6 | M, S, M, M, M, M | 480k | 12% | 710 | |
| 3.1.4 observability (+ copy table, golden tests, log-capture filter, CSS/e2e) | 4 | S, S, S, XS | 220k | 12% | 326 | |
| 3.2.1 claim slot (no refund) | 1 | S | 50k | 12% | 74 | |
| 3.2.2 pinned, cancellable writer | 2 | M, XS | 145k | 12% | 215 | `make test-delivery-guards` CI queue |
| 3.2.3 nudge branch (+ row annotation, copy consumers) | 4 | S, M, M, M | 480k (incl. flag + registry) | 12% | 710 | |
| 3.2.4 re-supervise | 2 | M, S | 135k | 12% | 200 | |
| 3.2.5 guard + stale gate + stop tests | 3 | S, S, M | 215k | 12% | 318 | |
| 4.1.1 reconcile + gates (+ web-app/e2e gates, ship checklist) | 2 | S, S | 120k | 8% | 158 | CI queue |

- Task count: 3+3+2+3+1+2+6+4+1+2+4+2+3+2 = **38** (35 before repair iteration 5: +3.1.3e and +3.1.3f from splitting 3.1.3a and 3.1.3c for sequencing, +3.1.4d for the CSS/e2e change; no scope removed). Raw tokens sum 2,850k; base sum about 4.20M CU (4,197k). With the x1.75 verification multiplier (per-story spec + quality review, `6-verify` layers, CI reruns): about **7.3M CU** (7,345k), band **Large** (mid-band; iteration 5 added +170k raw / +251k base CU, about +0.44M after the multiplier, against 6.9M after iteration 4: the splits, copy table and its golden tests, residual canaries, setter-error test, BUG-079 baseline, the CSS/e2e task; earlier, iteration 4 folded the pre-mortem into existing tasks' ACs, no task added: +100k raw / +145k base CU, about +0.25M after the multiplier, against 6.65M after iteration 3: registry reserve-then-start and lock tests in 3.1.3, the nudge flag, item-status guard, ctx and event rules and extra tests in 3.2.3, seams S1-S5 spread over 2.1.1, 3.1.3, 3.2.2). One line: the tests are the bulk of the size, not the production code.
- **Critical path (agent waves)**: 15 waves, listed under Dependency Visualization; the serial spine is the shared service file `autonomous_orchestration_service.go` (3.1.1a -> 3.1.2b -> 3.1.3a -> 3.1.3e -> 3.1.3b -> 3.1.4a -> 3.1.4b -> 3.2.3a -> 3.2.3b). The split added one wave (W4 holds 3.1.3e alone) and removed the former W5 same-file collision.
- **Riskiest first**: 3.1.1 (extraction) and 2.1.1 (driver exits) lead their lanes; 3.2.2b's pin test is run as soon as the writer exists, not at the end.
- **Overrun checkpoint**: if cumulative actual CU on completed stories exceeds 1.25x their table value (x1.75), pause and report the delta and its cause.
- **Wall-clock blockers (owners)**: (1) flipping `autonomous_stall_nudge_enabled` live (it ships OFF; `diagnose_nudge_enabled` is already true in the live config, so this flag is the real switch) after reading the `nudge_outcome=disabled` count in the `stopped` log line, then the 14-day soak review against the Observability Plan's success target and delete thresholds; the flip decision is due merge + 7 d and the sunset merge + 30 d (Observability Plan, "Review dates"): tstapler. (2) Owner veto/confirmation of the defaults under Unresolved Questions (working -> Resupervise, `max_turns` -> Respawn, boot re-adoption linked-only scope; the re-adoption default is decided: OFF): tstapler. (3) First post-deploy boot: read the `readopt candidate` log lines once (the default is OFF, so this is a dry-run: read it before flipping the flag on); also note the re-adopted driver's first turn can inject into a quiet-but-mid-tool-call pane (review M4): tstapler. (4) CI queue for `test-delivery-guards` and the complexity gate: none.
