# Requirements: Phantom repeated "1" keystroke on session open/reconnect

Backlog item: `04089969-0f19-499c-be34-2e8bcfc4f13e`

## Problem

On opening/attaching to a session, a single keystroke (`1`) was delivered to
the agent (Claude Code) repeatedly, with no numbered prompt on screen to
justify it. The agent received the input as if typed and could not proceed —
the session became unusable.

Observed on stapler-squad v1.33.x–v1.34.0, macOS, Chrome, session
`mbr-skills`, while the session's terminal stream was flapping
(connect → "stopped" → reconnect):

```
[streamViaControlMode] capture-pane failed, sending stopped notice   err="session not started or paused"
[streamViaControlMode] CM input failed, retrying via subprocess       err="cannot send input to instance that has not been started or is paused"
```

Ruled out:
- Not the `auto_yes` auto-accept feature (flag was `0` in `sessions.db`).
- Not `--dangerously-skip-permissions` (a launch flag, not a keystroke source).
- Input arrived through the normal relay path (`SendInputViaControlMode` /
  frontend `sendInputWithEcho` speculative-echo path,
  `TerminalOutput.tsx:539-545`), i.e. delivered as if typed by a user.

## Confirmed root cause (Phase 0 — see `research/phase0-findings.md`)

The originally-suspected client/server input-relay duplication (`MessageQueue`,
`useTerminalStream` reconnect, `streamViaControlMode`'s subprocess fallback)
was **ruled out** as the source of repeated single-keystroke delivery: a
single push is consumed exactly once by `MessageQueue`'s iterator, and the
server's per-connection read goroutine sends each received input message at
most once (fire-and-forget, no path where a successful send also triggers the
fallback).

The **confirmed primary root cause** is `session/session_driver.go`'s
startup-dialog auto-answer loop: `runSessionDriverWithPrompt` polls
`inst.Preview()` every `driverPollInterval` (2s) and calls
`inst.SendKeys("1\n")` whenever `isStartupDialog(output)` matches — with **no
de-duplication, backoff, or "already answered" latch**. This mechanism is
**unconditional and independent of `auto_yes`** (the ticket only ruled out
`auto_yes`, not this separate auto-answer path). When the session is
flapping/stalled, `Preview()`'s underlying buffer can keep returning the same
stale dialog content long after the live pane has moved on, so the loop
resends `SendKeys("1\n")` every 2 seconds indefinitely — landing as literal,
out-of-context "1" characters once the session recovers. This was confirmed
with a live-executing runtime test (`session/phase0_repro_test.go`), not
source inference alone: the real driver goroutine produced 3 repeated
`SendKeys("1\n")` calls over 3 poll ticks against a stalled preview buffer.

A **confirmed secondary hardening gap**, independently real but not the cause
of this ticket's repeated-single-keystroke symptom, is the client
`MessageQueue`/`useTerminalStream` reconnect path: `MessageQueue.close()`
never clears its buffered `queue` array, and `useTerminalStream.connect()` has
no epoch/generation guard against overlapping reconnects. AC3's text
explicitly calls out the superseded-`MessageQueue` case, so this is fixed as
additive hardening alongside the primary fix, not instead of it.

This may share a *reconnect-instability* root with the infinite-resize loop
bug (#164), but not an *input-duplication* root — features.md found no
evidence tying #164 to input replay.

## Goals

1. Prove the duplication mechanism with direct runtime evidence — not just
   source-reading inference — before changing any code.
2. Fix the confirmed re-emission point so any single physical keystroke
   reaches the agent/tmux pane **at most once**, even while the connection is
   reconnecting/flapping (connect → stopped → reconnect) at the moment it is
   sent.
3. Never replay already-forwarded input after a new connection is
   established. Input typed while disconnected must be dropped, not queued
   for later delivery — including input already sitting in a MessageQueue
   that gets superseded by a reconnect before it's flushed. When input is
   dropped this way, the user must be clearly told (visually and audibly),
   not left to wonder why their keystrokes vanished.
4. Add regression coverage (Go + Jest) that simulates a reconnect
   during/around an input send and asserts the input reaches tmux exactly
   once, and that the drop-on-close and reconnect-epoch behaviors are
   deterministic under overlapping/rapid connects.
5. Manually reproduce the ticket's specific flapping condition
   (not-started/paused, not generic network-offline toggling) and confirm the
   fix eliminates the repeated phantom keystroke.

## Non-Goals

- **Concurrent input from multiple browser tabs/windows attached to the same
  session** is explicitly out of scope. Any future report of duplicate/lost
  input when two tabs are attached to the same session simultaneously is a
  distinct problem and must not be treated as a regression of this fix.
- General reconnect/re-render stability work beyond what's needed to stop
  input replay (e.g. the full infinite-resize-loop bug #164) is out of scope
  except where the two share a fix point discovered during Phase 0.
- Changing the tmux control-mode protocol itself, or the flow-control/SSP
  negotiation protocol, beyond what's needed to make input delivery
  idempotent across reconnects.

## Acceptance Criteria

1. The duplication mechanism is confirmed with direct runtime evidence
   (DevTools WS-frame capture and/or correlated server logs from an actual
   induced-flapping episode) via a Phase 0 go/no-go gate **before** the rest
   of Phase 1/2 implementation proceeds. Source-level research alone does not
   satisfy this.
2. Once confirmed, the identified re-emission/replay point is fixed so a
   single physical keystroke is delivered to the agent/tmux session at most
   once, even when the session is reconnecting or flapping (connect →
   stopped → reconnect) at the time it's sent.
3. Already-forwarded input is never re-emitted after a new connection is
   established; input typed while disconnected is dropped (not queued or
   later flushed) — including messages already sitting in a superseded
   MessageQueue at close time — and the user is visibly and audibly signaled
   (drop-and-signal badge + assertive announcement) when this happens.
4. Regression coverage exists:
   - Go: a bounded read-goroutine exit test.
   - Jest: an overlapping-connect epoch guard test, a
     queued-message-drop-on-close interleaving test, and a triple-rapid-connect
     no-throw test.
   All simulating a reconnect during/around an input send and asserting the
   input is forwarded to tmux exactly once.
5. Manual repro from the ticket — attaching to a session while it
   specifically reproduces the observed not-started/paused flapping condition
   (not just generic network-offline toggling) — no longer produces repeated
   phantom keystrokes in a follow-up verification pass, recorded against the
   backlog item.
6. Concurrent input from multiple browser tabs/windows attached to the same
   session is explicitly documented as out of scope (this document's
   Non-Goals section) so a future multi-tab report is not mistaken for a
   regression of this fix.

## Confirmed code (Phase 0)

- `session/session_driver.go` (`runSessionDriverWithPrompt`, lines ~148-165)
  — **primary fix target**: unbounded `isStartupDialog`/`SendKeys("1\n")`
  retry loop, no de-duplication/backoff/latch.
- `session/instance_terminal.go` (`Preview()`, lines 105-125) and
  `session/claude_controller.go` (`GetRecentOutput`, lines 627-644) — the
  stale-buffer mechanism that lets `Preview()` keep returning matching dialog
  content during a flap.
- `web-app/src/lib/terminal/MessageQueue.ts` (async-iterable outgoing queue;
  `close()` does not clear buffered `queue` array) — **secondary fix
  target**.
- `web-app/src/lib/hooks/useTerminalStream.ts` (`connect()`, lines 156-345 —
  no epoch/generation guard against overlapping reconnects) — **secondary
  fix target**. Follow the existing generation-counter idiom already used in
  this codebase (`web-app/src/lib/hooks/usePathCompletions.ts`).
- `server/services/connectrpc_websocket.go` (`streamViaControlMode`,
  lines 857-924) — read, confirmed NOT a duplication source (single
  fire-and-forget send per received input message); the "session not started
  or paused" / "CM input failed, retrying via subprocess" log lines were
  present during the ticket's episode but are not the repeated-"1" mechanism.
- `session/tmux/control_mode.go` (`SendInputViaControlMode`, lines 659-682)
  — read, confirmed no double-send path.

Ruled out: `session/tmux_process_manager.go`, `session/instance_tmux.go`
session-sharing/refcounting issues are a real architectural concern (see
`research/architecture.md`) contributing to *why* the session flaps, but are
out of scope for this fix per the Non-Goals section (general reconnect
stability beyond input replay).

## Constraints

- Must not regress the existing speculative-echo (SSP) UX for the normal,
  non-flapping case.
- Fix must be testable without requiring a live tmux session for the Jest
  regression tests (mock/simulate the reconnect boundary).
- Manual verification (AC5) requires actually inducing the not-started/paused
  flapping condition described in the ticket, not just toggling network
  online/offline in DevTools.
