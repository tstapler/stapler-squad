# Pitfalls: stop-reason classification + nudge-vs-respawn

Source: read-only research subagent; line numbers approximate, re-verify at implementation.

Three facts shape every guard: (1) every non-DONE driver exit is the same `Stuck:true`
outcome, and a panic fires no completion at all; (2) a driver stop does not kill the pane, so
the `ItemSession` stays `EndedAt == nil`; (3) `AutoRespawnAutonomousWork` races
`onSessionExited`, `RemediateStaleWorkSession` and reconcilers.

| # | Pitfall | Evidence | Design guard |
|---|---|---|---|
| P1 | Every non-DONE exit is labelled "max turns reached", `Turns = maxTurns`; panic recover never fires completion so `AutonomousMode` stays true | `session/autonomous_driver.go` ~L313-336, L398-406, L433-439, L259-263; notification body uses `outcome.Turns` (`autonomous_orchestration_service.go` ~L603) | Typed `StopReason` set at each `break`; fire completion from panic recover; record real `turnCount`; never infer reason from text |
| P2 | External `Stop()` (request_review, pause, delete) reaches `fireCompletion` as Stuck; work role then `MarkStuck` + respawn (review role only protected by role) | `stopAndDeregisterDriver`; `tools_backlog.go` ~L1993 comment | `StopReasonCancelled` (cancel cause / `stoppedExternally` flag); `onAutonomousDriverComplete` early-returns: no MarkStuck, no respawn, no nudge |
| P3 | Nudge loop with no cap = the 78-bounce shape. `RecordDiagnoseNudgeAttempt` no-ops with no open stuck row; `DiagnoseNudgeAllowed` fails open with no row | `session/diagnose_nudge.go`; `docs/tasks/backlog-feature-improvement.md` ~L968 | Decision takes (count, nextEligibleAt, now); cap/cooldown => respawn, never "nudge anyway"; MarkStuck first; use atomic record-if-below-cap; verify MarkStuck refresh does not reset the count |
| P4 | Nudging a mid-work session (false idle; background subagent; hallucinated DONE) | `autonomous_driver.go` ~L526-545; `nudge_gate.go` (StatusReady dead); `autonomous_orchestration_service.go` ~L454 | "Awaiting" only via `IsSafeSteerStatus` + settle window + output recency; active states => none; do not use `isIdleStatus`; request_review stays the sole completion path |
| P5 | Nudge into dead/foreign pane | `nudge_gate.go` `VerifyPaneOwnershipBeforeWrite`; `docs/explanation/oneshot-session-respawn-loop.md` | Reuse `VerifyNudgeSafeToWrite` + `LeaseWriterNudge`; classify exited before idle; dead => respawn/none; failed gate must not consume nudge budget; never nudge one-shot sessions |
| P6 | Driver-complete callback races `onSessionExited` | BUG-063/064, `session/backlog_lifecycle.go` ~L920-975; RESPAWN_BLOCKED_ACTIVE (#764) | Decide before any `EndedAt` write; nudge branch does not end/kill/respawn; respawn branch keeps existing synchronous end-then-dispatch order; capture state BEFORE end/kill |
| P7 | Double remediation with `RemediateStaleWorkSession` (independent backoffs per reason, check-then-act) | `backlog_service_triage.go` ~L2182-2250 | One per-item in-flight gate shared; after a nudge mark "nudged at T" so the stale sweep defers a cooldown; fallback goes through `RemediationDue` |
| P8 | Pane tail leaks secrets into logs, stuck-row detail, notification (pushed + persisted) | no sanitizer on the autonomous path; `session/inhibition.go` patterns, `backlog_review.go` `secretPatterns` unused here | Strip ANSI, redact via shared patterns before ANY sink; raw tail in memory only; canary-secret tests |
| P9 | Unbounded captured output; byte-slice truncation splits UTF-8 | `buildOrchestrationPrompt` ~L641; row-growth precedent in `AutoReopenForPRFix` | `captureTail(maxLines≈40, maxBytes≈4KB)` rune-safe; overwrite latest only; capture once per stop |
| P10 | Flaky tests (real tmux/sleep/3s settle; bare `go func()` respawn dispatch; unguarded field writes) | `CheckNudgeEligibleWithSettleWindow` parameterized; driver option seams; `previewer` fake | Pure `Decide(...)`; observer interface not `*Instance`; fake respawner signalling a channel; injectable dispatch; `t.Parallel`, no `t.Setenv` |

## Open question for the planner

`inst.AutonomousOutcome` is on the wire as "done"|"stuck". Extending to `stuck:idle` etc. could
break UI string equality checks — grep `web-app` for `autonomousOutcome` before changing, or
keep `AutonomousOutcome` as-is and carry the sub-reason in a separate field/detail.

Also: a DONE stop reason must not reset nudge count or remediation backoff
(`docs/tasks/backlog-feature-improvement.md` ~L1410 oscillation).
