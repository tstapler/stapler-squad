# ADR-005: AskUserQuestion answer routing is a ladder; the keystroke path is the default and the hook-mediated answer is conditional on spikes S1 and S2

**Status**: Proposed (2026-10-17); amends ADR-010 of `notification-tray-and-hidden-session-gate` (Reply no longer hidden-only when `askuserquestion_structured` is on); triad round 2: this ADR becomes Accepted when the owner-defaults list in plan `Unresolved Questions` is confirmed at the Phase 5 start gate (one explicit accept step for all eight ADRs; until then the stated defaults proceed)
**Project**: notification-actions-and-agent-messaging
**Flag**: `askuserquestion_structured` (scopes `stage:render`, `stage:answer`, `stage:hook_reply`)

## Context

Claude Code documents a programmatic answer: a `PreToolUse` hook on `AskUserQuestion` returning `permissionDecision:"allow"` plus `updatedInput` containing `questions` and `answers` ("allow" alone is insufficient). The text is framed for non-interactive callers; whether it skips the dialog in an **interactive tmux TUI** is unverified (`research/stack.md` section 2). `defer` is ignored in interactive sessions. The hook may also block the TUI before the dialog renders, so a long-blocking hook would remove the terminal as an answer path. `ssq-hooks` currently exits empty for `AskUserQuestion` (`cmd/ssq-hooks/main.go:102-107`) so the native dialog stays, and the only verified live-answer path is the ADR-010 single-digit keystroke (claim, `DialogMatch`, write lease, audit, `NOT_SENT` vs `SEND_INDETERMINATE`). Non-Claude harnesses have no verified native answer channel.

## Decision

Route by session state and capability, highest rung first; each rung falls through on a typed failure:

1. **`hook_reply`** (only if S1 PASS, and S2 shows an acceptable blocking limit): used for **pre-answered** questions (a durable answer already recorded when the hook fires, e.g. from a resume or a peer/agent answer) and, if S1/S2 allow, a bounded wait. The hook never blocks longer than `hook_block_ms` (default 0 = non-blocking; any non-zero value is capped below the Spike S2 ceiling and the hook exits empty at the cap so a hung or unreachable server shows the native dialog, pre-mortem #19) so the terminal stays an answer path. If S1 FAILS, this rung is deleted from scope and the plan's conditional tasks close with "not applicable", no other design change.
2. **`live_keystroke`**: existing path generalized to visible sessions (flag `askuserquestion_structured`, scope `stage:answer`) through the extracted `PaneReplyCore`; single-select, 1-7 options, one digit, no Enter. Multi-select and multi-question use a stepwise sequence **only if S1c measured the dialog navigation**; otherwise read-only with Open session.
3. **`resume`** (stopped, non-archived, durable row): answer persisted; nothing is pushed. After an explicit operator resume the agent reads it through the existing `get_guidance_request` tool (`server/mcp/tools_guidance.go`, whose description already tells the agent to re-check after a respawn), tested in Task 3.4d. `noarchivedrevival` is never bypassed by an answer. **Decided (triad round 2): the `resume` route never calls `Start`, `RecoverFromStopped`, `restartForRetry` or `transitionToLocked(..., Active)`.** It only stores the answer (`saved_for_later`); the operator resumes the session through the normal Open session path, and a spy test in Story 3.4 asserts zero calls. `server/services/question_answer_router.go` is still added to the analyzer's `lifecycleFiles` as a ratchet so a future revival in that file must carry an archival guard.
4. **`read_only`**: archived sessions, unmeasured harnesses, unknown shapes. Question and options render; no pane write. (For an archived session the answer is still stored on the durable row with `delivery_state=saved_for_later`, and nothing is revived; Story 3.4.)

Non-Claude harnesses: a `QuestionDialogProfile` per harness (strategy). A harness is replyable only when it has a measured capture-pane fixture and a bottom-anchored matcher (as ADR-010 2d); otherwise `read_only`.

Delivery state machine: `pending -> sent -> acknowledged | delivery_failed | saved_for_later | stale`.

**Acknowledgement signal (triad round 1, replaces "moves to Recent activity after 60 s").** `acknowledged` is set by the first of: (a) a `PostToolUse` receipt for `AskUserQuestion` from that session after `sent_at` (whether the injected `post_tool_logging` hook's matcher covers `AskUserQuestion` is UNVERIFIED and is Spike S7 item (6); if it does not, Task 3.4e adds the matcher to the injected hook); (b) two consecutive fresh captures 3 s apart with neither `DialogMatch` for the row's question nor `session.PaneShowsDialog` (Task 2.4c; the fallback, and the only signal while (a) is unavailable). At 60 s without an acknowledgement the server takes one more fresh capture under the write lease: the row's own dialog still showing means the keystroke was probably lost, so the state becomes `delivery_failed(reason=still_showing)` (a digit **was** written, so its copy is `design/ux.md` 3.1 row 16, never "Nothing was typed"; Retry sits behind a "Retry anyway?" confirm, re-verifies `DialogMatch` and writes one digit); the dialog gone is acknowledgement by (b); a capture error or indeterminate layout leaves the state `sent` with `unacknowledged=true`, shown as the **Unconfirmed** chip. An Unconfirmed row stays in Needs you and in the bell badge until it is acknowledged, the operator marks it handled (browser-local), or the session ends. It is never demoted to Recent activity by a timer, because that would hide a possibly stalled agent. `saved_for_later` applies only to agent-created (`agent_mcp`) guidance; a hook-mirrored question dies with its session.

## Consequences

- Behavior with S1 failing is identical to a keystroke-only design, so the project does not depend on an unverified hook behavior.
- Visible-session replies add a collision case with the human typing in the terminal; mitigated because `DialogMatch` is re-verified under the write lease and mismatch yields `STALE` (the `ActionOutcome`; `STALE_PROMPT` is the legacy hidden-reply enum value of ADR-010) with zero writes.
- Per-harness profiles are added one at a time, each gated by a fixture.

## Alternatives rejected

- **Hook-blocking as the default**: removes the terminal answer path while waiting; unverified timeout ceiling.
- **SDK `canUseTool`**: requires driving Claude through the Agent SDK rather than the interactive tmux CLI; out of scope (no tmux replacement).
- **Blind keystrokes for non-Claude harnesses**: the wrong-dialog risk that drove ADR-010.
