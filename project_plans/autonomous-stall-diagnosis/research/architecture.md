# Architecture research: autonomous stall diagnosis

Source: read-only research subagent (line numbers as reported; re-verify at implementation).
Builds on `project_plans/backlog-stuck-item-visibility/research/architecture.md` (durable
stuck-row design: `BacklogStuckState`, `MarkStuck`/`ResolveStuck`) and
`project_plans/backlog-triage-autonomous/research/architecture.md` (MaxTurnsReached flow; it
did not enumerate non-max-turns exits — this document does).

## 1. Stop-path inventory (AC1)

### 1a. Inside the driver (`session/autonomous_driver.go`)

| # | Trigger | Site | Shown today | Real class |
|---|---|---|---|---|
| D1 | startup idle wait fails | L296-301 | "startup timeout", Turns=0 | Startup stall; ALSO fires when ctx cancelled (Stop/shutdown during startup) |
| D2 | `ctx.Err()` at loop top | L314 | "max turns reached", Turns=maxTurns | External Stop / server shutdown (mislabelled) |
| D3 | `waitForRateLimitClear` error | L319, L487-491 (cap 4h L480) | "max turns reached" | ctx cancel or rate limit exhausted |
| D4 | `CallBlocking` error | L331-334 | "max turns reached" | Orchestrator/LLM error (or ctx cancel mid-call) |
| D5 | suppressed-turn ctx check | L366 | "max turns reached" | ctx cancel |
| D6 | `AcquireTerminalWriteLease` error | L399-402 | "max turns reached" | Lease contention timeout |
| D7 | `SubmitDriverContent` error | L403-406 | "max turns reached" | Submit failure, typically dead pane |
| D8 | loop exhausted | L433-440 | "max turns reached" | The only true max-turns (WAIT/duplicate/malformed turns burn budget) |
| D9 | panic | L259-263 | nothing — no `fireCompletion` | Silent stall: `AutonomousMode` stays true |
| D10 | DONE directive | L344-353 | "complete" | Orchestrator can hallucinate; service ignores for work role; item stays in_progress, no driver |
| D11 | idle not reached after turn | L418-421 | log only | Signal: driver never checks `TmuxAlive`/`PaneProcessDead` |

Correction to coordinator: the external `StopDriverForSession` caller is
`submit_review_verdict` (`server/mcp/tools_backlog.go:1998`), not `request_review`.
A sixth ctx-cancel `break` exists at L366.

### 1b. External `Stop()` callers (all end as Stuck via D1/D2)

`stopAndDeregisterDriver` (`server/services/autonomous_orchestration_service.go:148-158`):
`session_service_lifecycle.go:47` (StopSession), `:306` (DeleteSession),
`session_service_update.go:296` (autonomous toggled off), `session_service_wiring.go:364` ->
`tools_backlog.go:1998` (submit_review_verdict). Server shutdown cancels `lifecycleCtx`
(`server.go:1087`) => D2 on every driver. Today all take MarkStuck -> UpdateItemSessionEnded ->
RemediationDue -> AutoRespawnAutonomousWork (L329, L391, L412, L434), contrary to the
requirements non-goal.

### 1c. Outside the driver (none notify the driver)

| # | Path | Site | Effect |
|---|---|---|---|
| O1 | health checker pane exit 0 | `session/health.go:408-432` | Instance Stopped; EventStopped |
| O2 | health checker crash | `health.go:468-492`, `lastPaneOutputSnippet` L377 | crash reason only in `Instance.ExitReason` |
| O3 | tmux session gone | `health.go:305/340` | restart or fail (archival-guarded L227) |
| O4 | lifecycle event | `session/backlog_lifecycle.go:872/907` `onSessionExited` | in_progress -> review while driver alive |
| O5 | stale-work detector | `session/backlog_lifecycle_stale.go:62-64,292`; `backlog_service_triage.go:2182` | Targets exactly "idle at an interactive prompt" (doc L2139-2150) yet kills + respawns with no nudge |
| O6 | non-LLM SessionDriver | `session/session_driver.go:810-972` | Parallel nudge mechanism; precedent for fixed nudge text via `TryTerminalWriteLease(LeaseWriterNudge)` |
| O7 | `classifyFailureReason` | `session/retry_state.go:197` | Small existing taxonomy to align with |
| O9 | respawn gate | `backlog_service_triage.go:2081` | silent no-op / RESPAWN_BLOCKED_ACTIVE / rework cap |

### 1d. Service-side mislabelling (`onAutonomousDriverComplete`, L257-625)

- S1: `inst.AutonomousOutcome` written as raw fields (L285-289), bypassing the actor setter;
  `SetAutonomousComplete` (`session/instance_actor_setters.go:159-180`) exists, test-only.
- S2: stuck detail hard-coded "stopped after N turns without a DONE signal" (L329-331).
- S3: Done on work role in_progress only logs (L~455-480).
- S4: notification body says "stopped after N turns" even for startup timeout/cancel (L~600).

## 2. Types and flow

New `session/domain/autonomous_stop.go` (dependency-free; plain strings):

- `StopReason`: done, max_turns, startup_timeout, orchestrator_error, submit_failed,
  lease_timeout, rate_limit_exhausted, cancelled_external, cancelled_shutdown, panic;
  `IsStall()` false for done and cancelled_external.
- `ObservedState`: awaiting_input (idle_prompt nudge-safe / needs_approval), idle, working,
  error, exited, crashed, unknown (satisfies AC2's idle/awaiting-input, error, completed-or-exited).
- `StopSnapshot`: CapturedAt, LastActivityAt, PaneTail (bounded ~40 lines/4KB), DetectedStatus,
  DetectedContext, TmuxAlive, PaneDead, ExitCode.
- Pure `ClassifyObserved(StopSnapshot)` and `DecideStallAction(in) {Nudge, Respawn, None}`.

Capture in the driver, via a single `d.finish(reason, detail)` that replaces the `break`s and
is the sole `fireCompletion` caller; distinguish external Stop from shutdown with an atomic
`stopRequested` set in `Stop()`; fire from the panic recover. Capturing in the callback would
race Stop-kill sequences.

Carry: `AutonomousDriverOutcome` gains `StopReason` + `Snapshot` (keep `Done/Reason/Turns/Stuck`;
derive `Stuck` from `IsStall()`). `onAutonomousDriverComplete`: structured log once, detail
= `stop_reason= state= turns= last_activity= tail=<=1KB>`, keep the single
`StuckReasonAutonomousStuck` (new enum values would churn ent/proto/TS). Add
`AutonomousStopReason` to `AutonomousModeState` (`session/instance_snapshot.go:67-73`) with a
setter beside `SetAutonomousComplete`; replace raw writes at L282-288 with it.

Proto: phase 1 none — `autonomous_outcome` must stay `""|done|stuck` because
`web-app/src/components/sessions/SessionCard.tsx:877/888` compares strictly. Phase 2 optional:
one `string autonomous_stop_reason` (tag after current max; field 67 is `autonomous_outcome`,
`types.proto:265`). `exit_reason = 72` is crash-only; do not overload.

## 3. Nudge-vs-respawn integration

Key row: `(item.ID, StuckReasonAutonomousStuck)`; MarkStuck (L329) runs first so the row exists.

1. Fail closed if MarkStuck failed (`DiagnoseNudgeAllowed` is "ungated" with no row).
2. Nudge only if: kill switch on (`config.EffectiveDiagnoseNudgeEnabled`), not archived
   (`inst.Snapshot()`), `IsStall()`, observed state is idle prompt (`IsSafeSteerStatus`),
   `DiagnoseNudgeAllowed`.
3. needs_approval / question / error are NOT nudge-safe (typing would answer a permission
   prompt) -> human notification.
4. exited/crashed/PaneDead -> Respawn (existing flow); cancelled_external -> None (no MarkStuck,
   no UpdateItemSessionEnded, no respawn).
5. Run in the existing goroutine (VerifyNudgeSafeToWrite blocks >=3s); `TryTerminalWriteLease(LeaseWriterNudge)`;
   fixed text; `RecordDiagnoseNudgeAttempt` only after a successful write.
6. Races: atomic `WHERE count < cap` UPDATE; add `Storage.ClaimDiagnoseNudgeSlot` exposing the
   bool (current `RecordDiagnoseNudgeAttempt` returns only `justCapped`); in-process in-flight
   map per item; skip when `IsRetryPending`; no `RemediationDue` and no `UpdateItemSessionEnded`
   on the nudge path.
7. The driver is dead after nudging: restart supervision via `StartAutonomousDriverForInstance`
   (idempotent), restore `AutonomousMode=true` through the setter, notify "nudged".
8. Cap persists per open row (no reset by MarkStuck found) — record as a design decision.

## 4. Rule constraints

- `instance-lock-free-reads`: reads via `Snapshot()`; add fields to `AutonomousModeState` first.
- `noarchivedrevival`: orchestration service is out of the file set, but guard the nudge/restart
  path with the archival check anyway; keep `//nolint:silenttransition` on `UpdateItemSessionEnded`.
- Pane tail into DB detail: truncate ~1KB, redact (see pitfalls.md P8).

## 5. Disposition for `onAutonomousDriverComplete`

**Isolate via seam**, preceded by one behaviour-preserving extraction: pull the
`case SessionRoleWork: if !outcome.Done {…}` arm (L360-~442) into `handleWorkStall(...)`, then
hang a single `DecideStallAction` switch off it. Function is ~370 lines at nesting 6+; adding
inline would blow the gocognit gate. Do not restructure Review/Triage arms.

## 6. Test plan

Table-driven classify/decide across {reason × state × allowed × enabled × archived}; driver
tests per `break` site + panic + startup-cancel (previewer seam exists); service test that the
nudge path skips `UpdateItemSessionEnded`/`RemediationDue` and restarts a driver; concurrency
test that two completions yield at most cap nudges.
