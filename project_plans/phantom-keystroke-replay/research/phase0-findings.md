# Phase 0 — Runtime Evidence Gate (AC1)

Status: **GO — confirmed with direct runtime evidence.**

## Summary

The 6 research-phase agents (stack/features/architecture/pitfalls/ux/build-vs-buy)
were scoped around the ticket's own hypothesis: client-side `MessageQueue`/
`useTerminalStream` reconnect replay, and/or a server-side control-mode
goroutine duplicating a WebSocket-relayed input message. That research found two
real, independently-confirmed *latent* bugs on that path (see
`architecture.md`, `pitfalls.md`, `build-vs-buy.md`):

1. `MessageQueue.close()` never clears its internal `queue` array, so any
   message buffered before `close()` still drains onto the closed connection's
   stream.
2. `useTerminalStream.connect()` has no epoch/generation guard, so an
   overlapping reconnect can create two live `MessageQueue`+stream pairs for
   the same session.

However, the **architecture** agent, reading `connectrpc_websocket.go` and
`control_mode.go` directly, concluded neither of these can actually cause **one
physical keystroke to be delivered to tmux more than once**: a single push
goes into exactly one `MessageQueue`, `Array.shift()` consumes each queued
item exactly once, and the server's per-connection read goroutine
(`streamViaControlMode` Goroutine 2, `connectrpc_websocket.go:857-924`) does a
single try-then-fallback per received message with no internal duplication
path. Two research agents disagreed on whether the "CM input failed, retrying
via subprocess" fallback (`connectrpc_websocket.go:915-923` →
`session/tmux/control_mode.go:659-682`) could itself double-send; direct
reading of `SendInputViaControlMode`'s `select` semantics shows it's
fire-and-forget with no path where a successful enqueue *also* triggers the
subprocess fallback — that theory is ruled out.

None of this explained the ticket's actual symptom: the *same single
character, "1", delivered repeatedly, over and over*, specifically during a
"session not started or paused" flapping episode — not a one-time duplicate.

## The confirmed mechanism: `session/session_driver.go`

Reading `session_driver.go` (not in the original "suspected code" list — this
file was found by direct code reading, not by the scoped research prompts)
turned up an **unconditional, unbounded, auto-answering loop** completely
independent of the `auto_yes` flag the ticket already ruled out:

```go
// session_driver.go:148-165 (runSessionDriverWithPrompt, ticks every driverPollInterval = 2s)
output, previewErr := inst.Preview()
if previewErr == nil && output != "" {
    if isStartupDialog(output) {
        if err := inst.SendKeys("1\n"); err != nil {
            ...
        }
        continue // no de-duplication, no backoff, no "already answered" latch
    }
}
```

`StartSessionDriver` runs for every managed session regardless of `auto_yes`
(per the file's own header comment: "AutoYes (-y) ... covers everything else
that requires interactive input" — `isStartupDialog`/trust-folder answering is
a *separate*, unconditional mechanism). **This is exactly the gap in the
ticket's own "ruled out" reasoning**: the reporter checked `auto_yes = 0` and
concluded stapler-squad wasn't auto-answering a numbered prompt, without
knowing this second, unrelated auto-answer path exists and has no flag gate.

`Preview()` (`instance_terminal.go:105-125`) reads from `ClaudeController`'s
in-memory PTY buffer (`ptyAccess.GetBuffer()`, `claude_controller.go:628-644`)
when a controller is attached. That buffer is only updated by whatever reads
new PTY output; if the underlying tmux/control-mode connection is mid-flap
("session not started or paused"), the buffer keeps returning the **last
content written before the flap** — which, if that was the trust-folder
dialog, means every 2-second poll sees the identical dialog text indefinitely,
even after the live pane has moved on. Each such poll trips
`isStartupDialog()` again and calls `SendKeys("1\n")` again, with **no
de-duplication, no backoff, and no cap**. Whichever `SendKeys` writes actually
land once the session recovers arrive as literal, out-of-context "1"
characters in whatever is now on-screen — matching the agent's own reported
confusion ("I'm seeing repeated '1' messages, but I don't have a menu... that
these would select").

## Runtime evidence (not source inference)

`session/phase0_repro_test.go` (`TestPhase0_StuckDialogCausesUnboundedRepeatedSendKeys`)
runs the real, unmodified `runSessionDriverWithPrompt` goroutine against a fake
`ProcessManager` whose `CapturePaneContent()` always returns the same
trust-folder dialog text (simulating a stalled buffer during flapping), and
counts real `SendKeys` calls via an atomic counter.

Actual test output:

```
=== RUN   TestPhase0_StuckDialogCausesUnboundedRepeatedSendKeys
{"time":"...","level":"INFO","msg":"SessionDriver: answered startup dialog","session":"phase0-repro"}
{"time":"...","level":"INFO","msg":"SessionDriver: answered startup dialog","session":"phase0-repro"}
{"time":"...","level":"INFO","msg":"SessionDriver: answered startup dialog","session":"phase0-repro"}
    phase0_repro_test.go:130: Phase 0 evidence: real runSessionDriverWithPrompt goroutine
        called SendKeys("1\n") 3 times over ~6.5s while Preview() returned unchanging
        trust-dialog content
--- PASS: TestPhase0_StuckDialogCausesUnboundedRepeatedSendKeys (8.00s)
```

This is the real, unmodified production driver goroutine executing against a
fake environment engineered to reproduce the flapping precondition — not
source-level inference. It directly reproduces "the keystroke 1 sent to the
agent over and over" with no cap, correlated to the exact log line
(`"SessionDriver: answered startup dialog"`) that a real server would emit
during a real episode.

## Verdict and scope for Phase 1/2

**Confirmed primary root cause**: `session_driver.go`'s dialog-answer loop has
no de-duplication/backoff/latch, and will resend `SendKeys("1\n")` every poll
tick for as long as `Preview()` keeps returning dialog-matching content —
which a stalled/flapping session can do indefinitely. **This is the fix
target for AC2 and the manual repro in AC5.**

**Confirmed secondary hardening gap** (independently real, per architecture.md
/ pitfalls.md / build-vs-buy.md, and explicitly named in AC3's literal text —
"including messages already sitting in a superseded MessageQueue at close
time"): the client `MessageQueue`/`useTerminalStream` reconnect path has no
epoch guard and does not clear its backlog on close. This does not, by itself,
explain the reported ticket's repeated-single-keystroke symptom (ruled out by
the architecture agent's read of the consume-once queue semantics and the
server's single-goroutine-per-connection input handling), but it is a real
latent duplication/staleness risk under reconnect churn and is explicitly
in-scope per AC3/AC4's text. **This is the fix target for AC3's MessageQueue
clause and the Jest regression tests in AC4.**

Both fixes proceed in Phase 1/2. The plan phase should treat them as two
additive, independent changes (per `architecture.md`'s conclusion), not
alternatives.
