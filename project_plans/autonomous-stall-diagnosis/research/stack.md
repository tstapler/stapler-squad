# Stack research: autonomous stall diagnosis

No new dependencies. Everything below already exists in-repo. Source: read-only research
subagent; line numbers are as reported at research time and must be re-checked at
implementation time.

## Pane-state classification

- `detection.DetectedStatus` enum: `session/detection/detector.go:87-103`.
  `StatusReady` is dead for Claude (catch-all returns `StatusUnknown`;
  `session/nudge_gate.go:52-56`, `session/detection/binaries/claude.go:38-43`) — do not key
  classification on it.
- Read current status: `ClaudeController.GetCurrentStatus()` (`session/claude_controller.go:733`),
  `Instance.GetDetectedStatusInfo()` (`session/instance_state.go:345`; second return value
  distinguishes "no controller" = exited/external from a genuine `StatusUnknown`).
- Safe-to-write idle is `IsSafeSteerStatus` (`session/idle_status_gate.go:29`), not raw
  `StatusIdle` (ambiguous with shell/vim prompts).
- Driver's existing buckets: `isIdleStatus`, `isImmediateStatus` (`session/autonomous_driver.go:598-612`).
- Proposed mapping: awaiting-input = NeedsApproval/InputRequired/safe-Idle; error =
  Error/TestsFailing; idle = unsafe Idle/Success/Unknown-with-controller; exited = no active
  controller; active = Executing/Processing/WaitingForAgent/Compacting.

## Nudge delivery and guards

- MCP `writeNudge` is unexported on an MCP-only struct (`server/mcp/tools_diagnose.go:319`) and
  pinned by `server/services/terminal_write_pins_test.go`; the autonomous path must not add a
  second raw write path — call session-level functions:
  `Instance.AcquireTerminalWriteLease(ctx, LeaseWriterNudge, wait)` (`session/instance_write_lease.go:214`),
  `SubmitContentWithEnter` (`session/pane_submit.go:156`),
  `VerifyNudgeSafeToWrite` (`session/nudge_gate.go:132`; blocks for the settle window — call
  with a bounded ctx, off the hot path).
- Cap: pure `EvaluateDiagnoseNudgeAllowance` (`session/diagnose_nudge.go:42`), storage-backed
  `DiagnoseNudgeAllowed` / `RecordDiagnoseNudgeAttempt` (`:60`, `:99`), keyed on `domain.StuckReason`.

## Activity and output inputs

- `Instance.Preview()` / `PreviewContext(ctx)` (`session/instance_terminal.go:163/214`);
  the driver wraps it as `previewPane()` (test-substitutable).
- `Instance.LastMeaningfulOutputTime()` (`session/instance_state.go:278`),
  `GetTimeSinceLastMeaningfulOutput()` (`session/instance_approval.go:126`).
- Use `Snapshot()` for field reads (`.claude/rules/instance-lock-free-reads.md`).

## Logging

`github.com/tstapler/stapler-squad/log`, message + bracketed component tag + key/value pairs;
keys `session`, `item`, `itemSession`, `turn`, `err`, `reason`, `role`.

## Placement

New pure functions in package `session` (`session/autonomous_stall.go`) beside
`diagnose_nudge.go`: `StopReason` enum, `ClassifyPaneState`, `DecideRecovery`. Extend
`AutonomousDriverOutcome` with a typed `StopReason`; keep `autonomous_outcome` a string
(no proto regeneration).
