# BUG-101: Terminal resize instability and control-mode teardown timing have each been patched narrowly, repeatedly, instead of consolidated [SEVERITY: Medium]

**Status**: ✅ Fixed
**Discovered**: 2026-09-07, investigating a live "shows Connected, nothing happens when typing" report on session `stelekit`, confirmed happening on sessions regardless of stream-hub-forced status.
**Fixed**: 2026-09-22

## Problem Description

Two files have each accumulated multiple independent fixes to the same conceptual problem, each narrowing a specific symptom without addressing the shared root cause (Fowler's "Divergent Change"):

**`web-app/src/lib/hooks/useTerminalFlowControl.ts`'s resize oscillation handling** — fixed at least 5 times:
`20da32505`, `417d370dc`/`e5a42e730`, `9651edd5e`, `e8060fb1b`/`b5dba385e` (#728), and this investigation's fix (generalizing the #728 bounce check from an exact 2-back match to a small history window, since a real oscillation observed live on `staplersquad_stelekit` wandered across 3+ distinct values — `10x6 -> 67x38 -> 67x22 -> 67x38` — not a clean A-B-A flip-flop).

**`session/tmux/control_mode.go`'s subscriber-close/teardown timing** — fixed at least 4 times:
`6e6a9f676` (close slow subscribers instead of dropping bytes), `b0416e224` (add a grace period before closing), `c6bc8585c` (synchronize `controlModeDone` reads), BUG-090 (fixed: streamhub teardown races ahead of `StopControlMode`), BUG-086 (fixed: `readControlModeOutput`/`monitorControlModeErrors` read `controlModeDone`/`controlModeStdout` from the struct instead of capturing them at spawn time), plus this investigation's fix (moving the slow-subscriber grace-period wait off the tmux stdout read loop, since blocking it could stall tmux long enough that it stops noticing its own stdin close, forcing `StopControlMode`'s hard-kill path on every single teardown of a busy session).

## Root Cause

Neither file has a single place that owns "decide the final, settled value and apply it once":

- The resize pipeline reacts to every raw signal independently at each layer (`XtermTerminal`'s `ResizeObserver`, `useTerminalFlowControl`'s `resize()`, the server's control-mode resize+capture-pane round trip) — any upstream instability (a bouncing viewport) propagates through to real, expensive backend work every time, and each fix has added another special case to the send-path's gate rather than introducing one settling authority upstream of it.
- The control-mode subscriber/teardown lifecycle has similarly accreted independent protections (grace period, refcounting, generation tracking, deferred-close-on-in-flight-send) at the point each specific race was found, rather than being modeled as one explicit state machine.

## Fix

Both recommended consolidations were implemented.

**Resize settling** — extracted `web-app/src/lib/hooks/useResizeSettling.ts`, a dedicated hook owning value-dedup, the trailing-edge throttle, bounce/oscillation detection against a history window, and the escalating hold, in one place. `useTerminalFlowControl.ts`'s `resize()` now only supplies an `onSettled(cols, rows) -> bool` send callback (returning whether the send succeeded, so a failed send isn't recorded as "last sent" and falsely deduped) and delegates all timing/oscillation judgment to `useResizeSettling`. No behavior changed: all 16 existing `resize` tests in `web-app/src/lib/hooks/__tests__/useTerminalFlowControl.test.ts` pass unmodified against the new implementation, and `web-app/src/lib/hooks/__tests__/useResizeSettling.test.ts` adds 12 tests exercising the boundary directly (dedup, throttle, direct and 3-value-history bounces, escalating/capped hold, streak reset, failed-send non-dedup, pending-timer cancellation on bounce-back).

**Control-mode subscriber lifecycle** — replaced the three independently-guarded fields (`controlModeSubscribers` as a plain `map[string]chan []byte]`, `slowSendInFlight map[string]bool`, `pendingCloseAfterDrain map[string]chan []byte`) with one state machine in `session/tmux/control_mode.go`:

- `controlModeSubscriber{ch, state}` where `state` is `subscriberAttached` or `subscriberDraining`.
- `transitionSubscriberLocked(subscriberID, event)` is the single function every subscriber-lifecycle mutation flows through now (`broadcastControlModeUpdate`, `closeSubscriberLocked`, `drainSlowSubscriber`, `SubscribeToControlModeUpdates`), replacing what used to be direct map pokes scattered across ~6 functions. Events: `evSendFull`, `evCloseRequested`, `evDrainSucceeded`, `evDrainTimedOut`; actions: `actionNone`, `actionSpawnDrain`, `actionCloseNow` (silent — explicit close or a deferred close honored post-drain), `actionCloseTimedOut` (logs the "grace period elapsed" warning — a genuinely stalled consumer nobody asked to close).
- A second, non-iterated map (`controlModeClosingSubscribers map[string]chan []byte`) remains as bookkeeping the state machine owns, not a fourth independent field: `controlModeSubscribers` is what `broadcastControlModeUpdate` iterates on every tmux output line (the hot path), so a subscriber whose close is already decided is removed from it immediately rather than being revisited on every subsequent frame; `controlModeClosingSubscribers` holds just the channel for a subscriber closed while a `drainSlowSubscriber` goroutine still held a blocking send on it, so the close can be deferred until that goroutine resolves without a send-on-closed-channel panic. Both are read/written exclusively inside `transitionSubscriberLocked`.
- Refcounting (`controlModeRefCount`) was deliberately left untouched — that's BUG-086's scope, not this one's.

`session/tmux/control_mode_refcount_test.go`'s whitebox tests (which previously poked `slowSendInFlight`/`pendingCloseAfterDrain` directly to simulate races) were migrated to drive/assert against the new state (`controlModeSubscribers[id].state`, `controlModeClosingSubscribers[id]`) instead — same regression coverage (frame-reordering guard, deferred-close-on-unsubscribe-during-drain, no-panic-on-concurrent-close, slow-subscriber-closed-on-full-channel, bursty-subscriber-kept-open-within-grace-period, caller-never-blocks-on-a-stuck-subscriber), same assertions on observable behavior, now against the consolidated representation.

### Verification

- `go test ./session/tmux/... -race -count=3` — clean, no races, no flakes (151.7s).
- `cd web-app && npx jest src/lib/hooks --no-coverage` — 64 suites / 608 tests pass.
- `make build` — web UI + Go binary build clean.
- `make lint` — clean.

## Files Affected

- `web-app/src/lib/hooks/useTerminalFlowControl.ts`
- `web-app/src/lib/hooks/useResizeSettling.ts` (new)
- `web-app/src/lib/hooks/__tests__/useResizeSettling.test.ts` (new)
- `session/tmux/control_mode.go`
- `session/tmux/tmux.go`
- `session/tmux/control_mode_refcount_test.go`, `control_mode_test.go`, `control_mode_dispatch_test.go`

## Related

- BUG-086 (fixed): control-mode refcounting/generation-capture race under concurrent Start/Stop — explicitly out of scope for this fix, left untouched.
- BUG-090 (fixed): streamhub teardown state races ahead of `StopControlMode` call
