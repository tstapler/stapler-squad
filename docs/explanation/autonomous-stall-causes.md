# Why autonomous-mode sessions stop early

This explains every way an autonomous-mode backlog session can stop before finishing, which
of those are real stalls, and what the recovery path does with each. It is the diagnosis
deliverable for backlog item `4a709ebc` (AC1). It describes the code as it was before the stop-reason
work in `project_plans/autonomous-stall-diagnosis/`; the **After this change** section
records what changed.

## Short version

- The orchestrator loop (`AutonomousDriver.run`) can exit in ten distinct ways, but it reports only
  two outcomes: `Done` and `Stuck`. Every non-DONE exit except the startup timeout is
  reported as `Reason: "max turns reached"` with `Turns = maxTurns`, whatever actually happened.
  **VERIFIED** (code path, below).
- The completion callback treats every `Stuck` the same way: it records an `autonomous_stuck` row,
  ends the work session, and respawns, without looking at what the pane was doing.
  **VERIFIED** (code path, below).
- An externally requested stop (user stop/delete/toggle-off, `submit_review_verdict`, server shutdown)
  also reaches that callback as `Stuck`. **VERIFIED**.
- A panic inside the loop is swallowed and fires no completion at all, so the session keeps
  `AutonomousMode == true` with no driver. **VERIFIED**.
- How often each cause happens in production is **unknown**: the local logs hold only
  `test-*` session evidence (census below). This document does not rank causes by frequency.

## Evidence base

All citations are pinned to the last commit that touched each file. The branch HEAD when written
was `e0734b146`. Labels:

- **VERIFIED**: the code path exists and was read at the cited line.
- **INFERRED**: plausible from the code, no production evidence.

Log census (run on this workstation; logs rotate and test runs pollute them):

```
$ cat $(find ~/.stapler-squad -name 'staplersquad*.log') | grep -c 'AutonomousDriver: timed out waiting'
20
$ ... | grep 'AutonomousDriver: timed out waiting' | grep -oE '"session":"[^"]*"' | sort | uniq -c | sort -rn
     10 "session":"test-batched-summary-startup"
      6 "session":"test-wait-blocks"
      4 "session":"test-idempotent"
$ ... | grep -c 'AutonomousDriver: LLM call failed'
6
$ ... | grep -c 'recovered from panic'
10
```

Every matching line comes from a `test-*` session. There is no production evidence for or
against any cause below, and the counts differ from earlier research runs (21/8/11) only because the
log files rotate. Treat counts as test noise.

## The stop paths

Verdicts: **real stall** (the session was not finished and something stopped it),
**mislabelled** (a different cause reported as "max turns reached"), **not a stall** (the stop is
expected or deliberate).

### Inside the driver (`session/autonomous_driver.go`)

| id | where | symbol | what the code does | reported today | verdict |
|----|-------|--------|--------------------|----------------|---------|
| D1 | [L296-301](https://github.com/tstapler/stapler-squad/blob/a9b19d863a44a42f6966b3e1616693d210ac7ef2/session/autonomous_driver.go#L296-L301) | `waitForIdle(startupCtx, ...)` then `fireCompletion` | The first-idle wait fails after the startup timeout (default 60s) **or** when the run ctx is cancelled (`waitForIdle` returns false on `ctx.Done`, [L548-587](https://github.com/tstapler/stapler-squad/blob/a9b19d863a44a42f6966b3e1616693d210ac7ef2/session/autonomous_driver.go#L548-L587)) | `Stuck`, "startup timeout", `Turns: 0` | real stall when only the deadline elapsed; **mislabelled** when ctx was cancelled (external stop or shutdown during startup) |
| D2 | [L313-315](https://github.com/tstapler/stapler-squad/blob/a9b19d863a44a42f6966b3e1616693d210ac7ef2/session/autonomous_driver.go#L313-L315) | loop top `ctx.Err() != nil` | `break` | `Stuck`, "max turns reached", `Turns = maxTurns` | **mislabelled**: external `Stop()` or server shutdown |
| D3 | [L318-320](https://github.com/tstapler/stapler-squad/blob/a9b19d863a44a42f6966b3e1616693d210ac7ef2/session/autonomous_driver.go#L318-L320), [L484-513](https://github.com/tstapler/stapler-squad/blob/a9b19d863a44a42f6966b3e1616693d210ac7ef2/session/autonomous_driver.go#L484-L513) | `waitForRateLimitClear` | Error when ctx is cancelled or the 4h `maxRateLimitWait` ([L480](https://github.com/tstapler/stapler-squad/blob/a9b19d863a44a42f6966b3e1616693d210ac7ef2/session/autonomous_driver.go#L480)) is exceeded; `break` | "max turns reached" | real stall (rate limit exhausted) or **mislabelled** (ctx cancel) |
| D4 | [L330-334](https://github.com/tstapler/stapler-squad/blob/a9b19d863a44a42f6966b3e1616693d210ac7ef2/session/autonomous_driver.go#L330-L334) | `headlessPool.CallBlocking` error | `break` (no retry) | "max turns reached" | real stall (orchestrator/LLM error) or **mislabelled** (ctx cancel mid-call) |
| D5 | [L365-367](https://github.com/tstapler/stapler-squad/blob/a9b19d863a44a42f6966b3e1616693d210ac7ef2/session/autonomous_driver.go#L365-L367) | suppressed-turn `ctx.Err()` check | `break` | "max turns reached" | **mislabelled** (ctx cancel) |
| D6 | [L398-402](https://github.com/tstapler/stapler-squad/blob/a9b19d863a44a42f6966b3e1616693d210ac7ef2/session/autonomous_driver.go#L398-L402) | `AcquireTerminalWriteLease` error | `break` | "max turns reached" | real stall (lease contention for `AutonomousTurnLeaseWait`) |
| D7 | [L403-406](https://github.com/tstapler/stapler-squad/blob/a9b19d863a44a42f6966b3e1616693d210ac7ef2/session/autonomous_driver.go#L403-L406) | `SubmitDriverContent` error | `break` | "max turns reached" | real stall (typically a dead or gone pane) |
| D8 | [L433-440](https://github.com/tstapler/stapler-squad/blob/a9b19d863a44a42f6966b3e1616693d210ac7ef2/session/autonomous_driver.go#L433-L440) | loop exhausted / post-loop `!outcome.Done` | Funnel for D2-D7, plus the genuine turn cap | "max turns reached" (adds "(N malformed ...)") | the only true max-turns; WAIT, duplicate-nudge and malformed turns all burn the same budget ([L336-342](https://github.com/tstapler/stapler-squad/blob/a9b19d863a44a42f6966b3e1616693d210ac7ef2/session/autonomous_driver.go#L336-L342), [L356-381](https://github.com/tstapler/stapler-squad/blob/a9b19d863a44a42f6966b3e1616693d210ac7ef2/session/autonomous_driver.go#L356-L381)) |
| D9 | [L259-263](https://github.com/tstapler/stapler-squad/blob/a9b19d863a44a42f6966b3e1616693d210ac7ef2/session/autonomous_driver.go#L259-L263) | `recover()` in `run` | Logs and returns; **never calls `fireCompletion`** | nothing | real stall, and silent: no outcome, `AutonomousMode` stays true |
| D10 | [L343-354](https://github.com/tstapler/stapler-squad/blob/a9b19d863a44a42f6966b3e1616693d210ac7ef2/session/autonomous_driver.go#L343-L354) | `directive == directiveDone` | `Done: true` | "Autonomous fix complete" | the orchestrator can hallucinate DONE; see S3 |
| D11 | [L424-430](https://github.com/tstapler/stapler-squad/blob/a9b19d863a44a42f6966b3e1616693d210ac7ef2/session/autonomous_driver.go#L424-L430) | post-turn `waitForIdle` | Warns "proceeding anyway" and continues | log only | not a stop, but the driver never checks `TmuxAlive`/`PaneProcessDead`, so a dead pane keeps burning turns until D7/D8 |

`fireCompletion(` call sites: lines [299](https://github.com/tstapler/stapler-squad/blob/a9b19d863a44a42f6966b3e1616693d210ac7ef2/session/autonomous_driver.go#L299),
[352](https://github.com/tstapler/stapler-squad/blob/a9b19d863a44a42f6966b3e1616693d210ac7ef2/session/autonomous_driver.go#L352) and
[440](https://github.com/tstapler/stapler-squad/blob/a9b19d863a44a42f6966b3e1616693d210ac7ef2/session/autonomous_driver.go#L440), plus the definition at
[L516](https://github.com/tstapler/stapler-squad/blob/a9b19d863a44a42f6966b3e1616693d210ac7ef2/session/autonomous_driver.go#L516). Every exit except D9 goes through one of the three.

### How the service reacts (`server/services/autonomous_orchestration_service.go`)

| id | where | symbol | what happens | verdict |
|----|-------|--------|--------------|---------|
| S1 | [L282-290](https://github.com/tstapler/stapler-squad/blob/0b3ad53879b82891071cd5a83373c54d4415df17/server/services/autonomous_orchestration_service.go#L282-L290) | `onAutonomousDriverComplete` | Writes `AutonomousMode`, `AutonomousOutcome` as raw fields (`"done"` or `"stuck"`), bypassing the actor setter and snapshot republish; `Instance.SetAutonomousComplete` ([instance_actor_setters.go:176](https://github.com/tstapler/stapler-squad/blob/79cd52cd8391bc1aa2374e9079ab13d102b0c245/session/instance_actor_setters.go#L176)) exists but is only used by tests | defect: the badge can lag until another mutation rebuilds the snapshot |
| S2 | [L329-331](https://github.com/tstapler/stapler-squad/blob/0b3ad53879b82891071cd5a83373c54d4415df17/server/services/autonomous_orchestration_service.go#L329-L331) | `MarkStuck(autonomous_stuck, ...)` | Detail is the fixed string "autonomous driver stopped after %d turns without a DONE signal" regardless of cause | **mislabelled** for every cause but D8 |
| S3 | [L455-485](https://github.com/tstapler/stapler-squad/blob/0b3ad53879b82891071cd5a83373c54d4415df17/server/services/autonomous_orchestration_service.go#L455-L485) | work role, `outcome.Done` | Item stays `in_progress`; nothing nudges the now driverless session | by design after a live hallucinated-DONE incident (comment at L444-471) |
| S4 | [L601-604](https://github.com/tstapler/stapler-squad/blob/0b3ad53879b82891071cd5a83373c54d4415df17/server/services/autonomous_orchestration_service.go#L601-L604) | generic notification | Body says "stopped after N turns" even for a startup timeout (N = 0) or a cancel | **mislabelled** |
| R1 | [L360-442](https://github.com/tstapler/stapler-squad/blob/0b3ad53879b82891071cd5a83373c54d4415df17/server/services/autonomous_orchestration_service.go#L360-L442) | work role `!Done` arm | `UpdateItemSessionEnded` ([L391](https://github.com/tstapler/stapler-squad/blob/0b3ad53879b82891071cd5a83373c54d4415df17/server/services/autonomous_orchestration_service.go#L391)), `RemediationDue` ([L415](https://github.com/tstapler/stapler-squad/blob/0b3ad53879b82891071cd5a83373c54d4415df17/server/services/autonomous_orchestration_service.go#L415)), then `AutoRespawnAutonomousWork` ([L434](https://github.com/tstapler/stapler-squad/blob/0b3ad53879b82891071cd5a83373c54d4415df17/server/services/autonomous_orchestration_service.go#L434)) | the recovery: always a respawn; the pane's state is never read first (the root of AC3) |

`AutoRespawnAutonomousWork` itself ([backlog_service_triage.go:2081](https://github.com/tstapler/stapler-squad/blob/e96a1ebafb9acbe9c5d50cc2bb29e5906c54608b/server/services/backlog_service_triage.go#L2081))
has only liveness guards (`findActiveWorkSession`, the rework cap); none of them inspect output.

### Externally requested stops (all reach the callback as `Stuck`)

`stopAndDeregisterDriver` ([L148](https://github.com/tstapler/stapler-squad/blob/0b3ad53879b82891071cd5a83373c54d4415df17/server/services/autonomous_orchestration_service.go#L148))
calls `d.Stop()`, which cancels the run ctx; the loop breaks (D2) and still calls
`fireCompletion`. Callers:

| id | caller | verdict |
|----|--------|---------|
| X1 | StopSession: [session_service_lifecycle.go:47](https://github.com/tstapler/stapler-squad/blob/a2aade9b1aab7a10b346d92b11dbec9ef3a4d171/server/services/session_service_lifecycle.go#L47) | not a stall, treated as one |
| X2 | DeleteSession: [session_service_lifecycle.go:306](https://github.com/tstapler/stapler-squad/blob/a2aade9b1aab7a10b346d92b11dbec9ef3a4d171/server/services/session_service_lifecycle.go#L306) | not a stall, treated as one |
| X3 | autonomous toggled off: [session_service_update.go:296](https://github.com/tstapler/stapler-squad/blob/474b089b1328034def9ff1e0346fe5d65fe00c74/server/services/session_service_update.go#L296) | not a stall, treated as one |
| X4 | `submit_review_verdict`: [tools_backlog.go:1998](https://github.com/tstapler/stapler-squad/blob/01385626926d6fdb19e23416eea1b2a78f07c6f8/server/mcp/tools_backlog.go#L1998) | not a stall; the review-role arm is protected only by role (comment at L1993) |
| X5 | server shutdown: lifecycle ctx cancel, [server.go:1087](https://github.com/tstapler/stapler-squad/blob/5fd2a25df2718b3241ba0bd66e3993e4bb8a3df7/server/server.go#L1087) | not a stall, treated as D2 on every active driver |

The work-role arm has no such protection, so a user stop on a work session with a linked item
runs MarkStuck and the respawn path, subject only to the item still being `in_progress`
(`AutoRespawnAutonomousWork` returns early otherwise). Pause and user-stop
([session_service_update.go:96-110](https://github.com/tstapler/stapler-squad/blob/474b089b1328034def9ff1e0346fe5d65fe00c74/server/services/session_service_update.go#L96-L110)) never call
`stopAndDeregisterDriver` at all, so the driver outlives the paused or stopped session and later
fires `Stuck` for it (D7).

### Paths outside the driver (none notify the driver)

| id | where | what happens | verdict |
|----|-------|--------------|---------|
| O1 | [health.go:408-432](https://github.com/tstapler/stapler-squad/blob/5f9b634627a59d32cc1573912233af77102c5a23/session/health.go#L408-L432) `handleDeadPane` | pane process exited with code 0: `MarkExitedNormally`, instance Stopped | not a stall (completed or exited) |
| O2 | [health.go:468](https://github.com/tstapler/stapler-squad/blob/5f9b634627a59d32cc1573912233af77102c5a23/session/health.go#L468) `handleCrashedPane` / [L511](https://github.com/tstapler/stapler-squad/blob/5f9b634627a59d32cc1573912233af77102c5a23/session/health.go#L511) `respawnDeadPane` | crash: respawn or `MarkCrashed`; the output snippet lands only in `Instance.ExitReason` | real stall (error) |
| O3 | [health.go:305](https://github.com/tstapler/stapler-squad/blob/5f9b634627a59d32cc1573912233af77102c5a23/session/health.go#L305) `checkTmuxHealth` / [L340](https://github.com/tstapler/stapler-squad/blob/5f9b634627a59d32cc1573912233af77102c5a23/session/health.go#L340) `recoverMissingSession` | tmux session gone: restart or fail | real stall |
| O4 | [backlog_lifecycle.go:872](https://github.com/tstapler/stapler-squad/blob/a2aade9b1aab7a10b346d92b11dbec9ef3a4d171/session/backlog_lifecycle.go#L872) `EventExited/EventStopped` -> [L907](https://github.com/tstapler/stapler-squad/blob/a2aade9b1aab7a10b346d92b11dbec9ef3a4d171/session/backlog_lifecycle.go#L907) `onSessionExited` | ends the ItemSession, moves a work item `in_progress -> review` while a driver may still be live | not a stall; races the driver completion |
| O5 | [backlog_lifecycle_stale.go:62-64](https://github.com/tstapler/stapler-squad/blob/b74780da6f08d089e26ff46e06555c78c0c7e920/session/backlog_lifecycle_stale.go#L62-L64) + [RemediateStaleWorkSession](https://github.com/tstapler/stapler-squad/blob/e96a1ebafb9acbe9c5d50cc2bb29e5906c54608b/server/services/backlog_service_triage.go#L2182) | 2h with no `report_progress`: end the ItemSession, kill the pane, respawn. The doc comment names this exact case: the agent "idle at an interactive prompt" | real stall, and the clearest instance of "respawn where a nudge would do" |
| O6 | [session_driver.go:933](https://github.com/tstapler/stapler-squad/blob/a9b19d863a44a42f6966b3e1616693d210ac7ef2/session/session_driver.go#L933) `attemptBacklogNudge` | the non-LLM SessionDriver already sends one fixed "You appear to have paused" nudge, then falls to `handleDriverFailure` | a separate, older nudger (see Known gaps) |
| O7 | [retry_state.go:197](https://github.com/tstapler/stapler-squad/blob/74f2c3a4b45913ee6b933c0d6e8b82270e19b170/session/retry_state.go#L197) `classifyFailureReason` | the only existing failure taxonomy: `tmux_exited` / `crashed` | context for naming |
| O8 | `ratelimit.StateNone` gate in `waitForRateLimitClear` (D3) | the only rate-limit handling is inside the driver loop; a pane-level rate limit is not surfaced in the driver outcome | context for D3 |
| O9 | [backlog_service_triage.go:2081](https://github.com/tstapler/stapler-squad/blob/e96a1ebafb9acbe9c5d50cc2bb29e5906c54608b/server/services/backlog_service_triage.go#L2081) `AutoRespawnAutonomousWork` | silent no-op when the item is no longer `in_progress`; `RESPAWN_BLOCKED_ACTIVE` when a live session exists; rework-cap notification otherwise | silent failure modes after a stall: the operator sees "stuck" and nothing else |
| O10 | [session_creation_pipeline.go:347-355](https://github.com/tstapler/stapler-squad/blob/0a2b96f5349785fa10a5adb7ddd8822ab25e31f7/server/services/session_creation_pipeline.go#L347-L355) | a third driver start site (spawn and toggle are the others) with no turn callback | not a stop; relevant to any change of the start path |

## Root cause

Ranked by confidence in the mechanism, **not** by frequency (unknown):

1. **The stop reason is thrown away at the source.** VERIFIED. D2-D7 all `break` into one
   post-loop block (D8) that labels them "max turns reached" with `Turns = maxTurns`. Nothing
   downstream (the stuck row, the notification, the instance outcome) can recover the real cause,
   so "the session stopped early" is undiagnosable from any surface. This is why the
   problem is visible only as "autonomous items keep stopping".
2. **Recovery restarts without looking.** VERIFIED. `onAutonomousDriverComplete` ends the
   session and respawns for every `Stuck`, so a session that was merely idle at a prompt, or
   still working, is killed and rebuilt. The stale-work path (O5) does the same.
3. **External stops are treated as stalls.** VERIFIED. X1-X5 run the full stall path; only the
   review role is protected, by role rather than by cause.
4. **A panic leaves a ghost.** VERIFIED. D9 fires no completion; `AutonomousMode` stays true with no
   driver, which also keeps the approval gate live for a session nothing is supervising.
5. **The 60s startup idle wait versus a long first turn** (D1). INFERRED. A session whose first
   turn runs longer than the startup timeout is reported "startup timeout" and respawned
   while it is still working. No production log shows this.
6. **Orchestrator DONE hallucination** (D10/S3). VERIFIED as a handled incident (comment at
   `autonomous_orchestration_service.go` L444-471), not a current cause: the service refuses to act on
   it for the work role.

## Known gaps (not changed by this item)

Resolved by this change: S1, S2, X1-X5 (see "After this change"). The gaps below remain.

- The stale-work detector (O5) and health-checker restarts (O1-O3) still kill and respawn with no
  pane snapshot and no nudge. AC3 is scoped to driver-stop paths; these are a recorded follow-up
  (see the plan's "Follow-up backlog item").
- Pause and user stop never stop the driver (X-section above).
- `buildTurnCallback` writes `AutonomousTurn` as raw fields (same pattern as S1).
- The in-process nudge and the older `attemptBacklogNudge` (O6) can both be active on one
  session. The write lease serialises the writes but not the intent; each nudger has its own cap.
- The nudge cap is a lifetime counter on the `(item, autonomous_stuck)` row; neither a stuck-row
  reopen nor Reset clears it.
- `diagnose_nudge_enabled` is already `true` in the owner's workspace config, which is why the
  in-process nudge gets its own `autonomous_stall_nudge_enabled` flag (default off) rather than
  reusing it. The code defaults, live state and rollback for these flags are recorded in
  [ADR-030](../adr/ADR-030-autonomy-flag-defaults.md).

## After this change

Every exit now carries a typed `domain.StopReason` (`done`, `max_turns`, `startup_timeout`,
`orchestrator_error`, `submit_failed`, `lease_timeout`, `rate_limit_exhausted`,
`cancelled_external`, `cancelled_shutdown`, `panic`) plus a pane snapshot taken before anything is
ended. A panic now fires exactly one completion.

**Resolved.**

- **S1**: the stop is written through `Instance.SetAutonomousStopped` / `SetAutonomousStopObserved`
  (actor setter, snapshot republished) by `applyInstanceStop`.
- **S2**: the stuck row's context is built by `formatStopDetail` from the reason, observed state and
  recovery taken, not a fixed string.
- **X1-X5**: cancelled-class reasons (`cancelled_external`, `cancelled_shutdown`) are never handled
  as stalls: no stuck row, no ItemSession end, no respawn, no nudge. A user stop also cancels an
  in-flight nudge (`stopAndDeregisterDriver` -> `cancelRecovery`).

**Nudge versus respawn** (work role only, `session/domain/autonomous_stop.go` `DecideStallAction`,
first match wins):

| State and conditions | Action |
|---|---|
| not a stall class, archived, or retry pending | none |
| `max_turns` or `panic` | respawn |
| idle at a prompt, item `in_progress`, both nudge flags on, slot available | nudge, then re-attach the driver |
| anything else | respawn |

A nudge types one fixed reminder into the idle pane under the terminal write lease and consumes one
slot of the item's cap (one slot stays reserved for the diagnose agent). Delivery failures that are
certain fall back to respawn; an uncertain delivery, a cancel, or a session that is no longer
restartable leaves the pane alone. Flags are re-read at the claim, so turning one off stops
in-flight plans. A superseded driver's late completion callback is ignored (matched by run id).
