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

Ruled out: the `auto_yes` auto-accept feature (flag was `0`) and
`--dangerously-skip-permissions` (a launch flag, not a keystroke source).
Input arrived through the normal relay path, i.e. delivered as if typed.

## Root cause — already confirmed and already fixed on `main`

This exact bug was independently root-caused and fixed via GitHub issue #165
before this backlog item's SDD work began, in two merged commits still
present on `main`:

- `3546c2b12` — *"fix(session): stop repeated '1' keystroke on
  directory-approval prompts (#146)"*. `session_driver.go`'s
  `NeedsApproval`-driven auto-answer path had no guard against resending: the
  driver polls every 2s, `NeedsApproval` can remain the detected status for
  several ticks after `"1\r"` is sent while the PTY redraws, and each tick
  re-matched the same dialog text and resent `"1"`. Commit message
  explicitly: *"reported in #165 as 'repeated 1 keystroke sent to agent on
  session open/reconnect', worse during reconnect churn since the driver
  keeps polling through it."* Fix: `shouldApprovePromptOnce`, an
  edge-triggered latch keyed on an `awaitingClear` flag (not a time-based
  cooldown), mirroring the pre-existing `shouldAnswerStartupDialog` latch.
- `c0e6c4ce6` — *"fix: stop repeated key sends on trust/approval dialogs and
  stale preview reads"*. `Instance.Preview()` called `GetRecentOutput(0)`,
  which the PTY buffer treats as "entire session-lifetime buffer" rather than
  a bounded recent window — so an already-answered dialog could still appear
  "visible" long after it scrolled out of view, which is exactly what makes
  the resend loop worse *during* reconnect/flap churn (stale buffer content
  outlives the live pane). `Preview()` now prefers tmux `capture-pane` (real
  terminal emulation) for tmux-backed instances. Also generalized the
  shared edge-triggered latch (`shouldSendOnce`, keyed on `awaitingClear`) to
  both the startup dialog and the approval-dialog auto-answer paths. Per the
  code's own doc comment (`session_driver.go:145`): "A fixed time-based
  cooldown is not sufficient here" — `shouldSendOnce`/`awaitingClear` is
  deliberately not a cooldown/timestamp mechanism.

Verified present on this branch: `session/session_driver.go` (`shouldSendOnce`,
`shouldAnswerStartupDialog`, `shouldApprovePromptOnce`, the `awaitingClear`
edge-triggered latch state) and `session/instance_terminal_test.go`. No
`withinCooldown`, `lastApprovalAnsweredAt`, or `shouldApprovePromptWithCooldown`
identifiers exist anywhere in this file — an earlier draft of this document
misdescribed the mechanism using those names; corrected here.
**AC1 and AC2 are therefore satisfied by code already on `main`** — this
effort's job is to verify that with a regression test tied to this backlog
item (not just re-derive the fix) and close the remaining gap below.

## Why finish this

This backlog item's AC3/AC4/AC5/AC6 are explicitly still open and tracked in
the company's backlog system regardless of the upstream symptom fix landing
via #146/`c0e6c4ce6` — closing the ticket's original symptom did not close
the ticket. The remaining scope is now small and well-bounded (client-side
reconnect hardening in `MessageQueue`/`useTerminalStream` plus regression
tests, not a re-fix of the original bug) specifically *because* that upstream
discovery narrowed it — two prior SDD attempts failed to ship at a broader
scope, before this narrowing existed. Completing it closes a real, if
lower-severity, latent gap: input loss/duplication during reconnect churn is
still possible today via the `MessageQueue`/epoch bugs this plan fixes,
independent of whether it manifests as the original ticket's exact "phantom
`1` keystroke" symptom.

## Remaining confirmed gap — client-side reconnect hardening

Independently real, not the cause of the original ticket's symptom, but
explicitly in scope per AC3/AC4/AC6: the frontend input-relay path has no
protection against re-emitting or losing buffered input across a reconnect
boundary.

- `web-app/src/lib/terminal/MessageQueue.ts` — `close()` sets `closed = true`
  but never clears the buffered `queue` array. `[Symbol.asyncIterator]`'s
  loop condition is `while (!this.closed || this.queue.length > 0)` — so any
  message still sitting in `queue` when `close()` is called **is still
  yielded** (and therefore still sent) after close, instead of being dropped.
  Confirmed by reading the current implementation on this branch.
- `web-app/src/lib/hooks/useTerminalStream.ts` — `connect()` has no
  epoch/generation guard against overlapping reconnects (rapid/triple
  reconnect can start a second stream loop before the first one's cleanup
  finishes). No existing generation-counter idiom is applied here, unlike
  `usePathCompletions.ts` which already uses one for a similar race.
- No drop-and-signal UI exists yet: `web-app/src/components/sessions/` has no
  `InputDropBadge` (or equivalent) component, and no assertive live-region
  announcement fires when disconnected input is dropped.
- No `session/services/connectrpc_websocket.go` read-goroutine test bounds
  goroutine exit on reconnect/close (AC4's "Go: bounded read-goroutine exit
  test").

## Goals

1. Confirm AC1/AC2 against the already-merged fix with a regression test
   scoped to this backlog item (not re-implement a fix that already exists).
2. Hardened the client reconnect path so:
   - already-forwarded input is never re-emitted after a new connection is
     established,
   - input still queued when the connection is superseded/closed is dropped,
     not held for later delivery,
   - the user is visibly (badge) and audibly (assertive announcement)
     signaled when this drop happens,
   - connection-state/decoder corruption caused by a `disconnect()`-vs-
     `connect()` interleaving race is prevented — a stale `disconnect()`
     continuation that resolves after a newer `connect()` has already
     established a connection must not reset or corrupt that newer
     connection's state or decoder.
3. Add regression coverage (Go + Jest) simulating a reconnect during/around
   an input send, asserting input reaches tmux at most once and that
   drop-on-close / overlapping-reconnect behavior is deterministic.
4. Manually reproduce the ticket's specific flapping condition
   (not-started/paused, not generic network-offline toggling) and confirm no
   repeated phantom keystrokes, recorded against the backlog item.

## Non-Goals

- **Concurrent input from multiple browser tabs/windows attached to the same
  session** is explicitly out of scope. A future report of duplicate/lost
  input with two tabs attached simultaneously is a distinct problem and must
  not be treated as a regression of this fix.
- General reconnect/re-render stability work beyond what's needed to stop
  input replay/loss (e.g. the unrelated infinite-resize-loop bug) is out of
  scope except where it shares a fix point discovered during this work.
- Changing the tmux control-mode protocol or the flow-control/SSP negotiation
  protocol itself, beyond what's needed to make input delivery idempotent
  across reconnects.
- Re-implementing or second-guessing the already-merged `session_driver.go`
  `shouldSendOnce`/`awaitingClear` edge-triggered latch fix (#146 /
  `c0e6c4ce6`); this work builds on it.

## Acceptance Criteria

(Verbatim from the backlog item; ✓ = already satisfied before this session
per stored backlog state.)

1. [✓] The duplication mechanism is confirmed with direct runtime evidence
   via a Phase 0 go/no-go gate before Phase 1/2 implementation.
2. [ ] The identified re-emission/replay point is fixed so a single physical
   keystroke is delivered at most once, even while reconnecting/flapping —
   **satisfied by existing merged code (#146, `c0e6c4ce6`)**; this session
   adds a regression test tying it to this backlog item.
3. [✓] Already-forwarded input is never re-emitted after a new connection;
   input typed while disconnected is dropped, not queued/flushed —
   **not actually true on `main` today** (see Remaining confirmed gap above)
   despite being marked done; this session must make it true in the diff
   that ships, including the drop-and-signal badge + assertive announcement,
   and including an epoch guard preventing connection-state/decoder
   corruption from a `disconnect()`-vs-`connect()` interleaving race (a
   stale `disconnect()` continuation must not clobber a newer `connect()`'s
   established state).
4. [ ] Regression coverage: Go bounded read-goroutine exit test; Jest
   overlapping-connect epoch guard, queued-message-drop-on-close
   interleaving, and triple-rapid-connect no-throw tests.
5. [ ] Manual repro from the ticket no longer reproduces phantom keystrokes,
   recorded against the backlog item.
6. [✓] Multi-tab concurrent input documented as out of scope (this
   document's Non-Goals section).

## Constraints

- Must not regress the existing speculative-echo (SSP) UX for the normal,
  non-flapping case.
- Fix must be testable without a live tmux session for the Jest regression
  tests (mock/simulate the reconnect boundary).
- Manual verification (AC5) requires actually inducing the not-started/paused
  flapping condition described in the ticket, not just toggling network
  online/offline in DevTools.
