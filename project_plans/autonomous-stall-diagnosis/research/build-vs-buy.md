# Build vs. adopt: stop reason + nudge-vs-respawn

Source: read-only research subagent; citations to be re-verified at implementation.

## Reusable vs. missing, per acceptance criterion

| AC | Reusable | Missing |
|---|---|---|
| 1 root-cause doc | Stop paths enumerable in `session/autonomous_driver.go` `run()` | The documentation; core finding: every non-DONE exit except startup timeout is labelled "max turns reached" with `Turns = maxTurns` |
| 2 stop reason observable | `AutonomousDriverOutcome.Reason` (free text); `inst.AutonomousOutcome` ("done"/"stuck", already on wire via `server/adapters/instance_adapter.go:89`); `MarkStuck(autonomous_stuck, detail)` + notification | Typed stop-reason enum; structured log per exit; sub-reason in stuck-row detail and `AutonomousOutcome` |
| 3 inspect then nudge | `GetCurrentStatus`, `IsSafeSteerStatus`, `CheckNudgeEligible`, `VerifyPaneOwnershipBeforeWrite`, `EvaluateDiagnoseNudgeAllowance`/`DiagnoseNudgeAllowed`/`RecordDiagnoseNudgeAttempt`, `previewPane()`, lease + `SubmitDriverContent` | Pure decision function; point-of-stop capture before respawn; an in-process nudge callable (today only the MCP `diagnose_nudge_session` handler) |
| 4 tests | Table-driven precedent (`EvaluateDiagnoseNudgeAllowance`, `nextLastNudge`) | Classifier + decision tests |

Notes: "exited" has no `DetectedStatus` value — derive from controller/instance liveness. The
nudge cap is keyed by (item, StuckReason) on the stuck row, so `MarkStuck(autonomous_stuck)`
must run before the cap is consulted/recorded or the gate is ungated.

## Options

- **OSS state-machine library** — Not recommended. A one-shot label plus a 3-way pure
  function over small enums has no transition graph; pane-state detection is already
  `session/detection`.
- **A. Route stalls through `DispatchDiagnose` (LLM agent)** — Not recommended on the hot
  path: slow, non-deterministic, needs a hidden session, races the respawn teardown, cannot be
  tested deterministically per state. Keep as escalation once the nudge cap is exhausted.
- **B. Deterministic in-process decision reusing existing primitives** — **Recommended.**
  Synchronous (pane inspected before the session is ended), unit-testable, no proto change.
- **C. Add per-reason values to `domain.StuckReason`** — Not recommended: item-level enum,
  proto/ent-mapped, fragments the nudge budget, conflates driver exit with item blockage.

## Recommended shape

1. `StopReason` typed enum; `run()` sets it at each exit; `AutonomousDriverOutcome.StopReason`.
   External `Stop()` maps to `cancelled` and must not MarkStuck/respawn.
2. `ClassifySessionState(status, ctx, alive)` -> awaiting_input | idle | error | exited (pure).
3. `DecideRecovery(reason, state, nudgeAllowed)` -> nudge | respawn | none (pure).
4. Extract the MCP nudge write path into a shared session-level helper; MCP handler calls it too.
5. Call it from `onAutonomousDriverComplete` after MarkStuck, before UpdateItemSessionEnded/respawn.
6. Log structurally; put state, last activity and pane tail in the MarkStuck detail.
