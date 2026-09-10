# BUG-101: Terminal resize instability and control-mode teardown timing have each been patched narrowly, repeatedly, instead of consolidated [SEVERITY: Medium]

**Status**: 🐛 Open (architectural finding, not a single bug — filed per `quality:reflect-and-fix`'s Phase 1.5 correlation check)
**Discovered**: 2026-09-07, investigating a live "shows Connected, nothing happens when typing" report on session `stelekit`, confirmed happening on sessions regardless of stream-hub-forced status.

## Problem Description

Two files have each accumulated multiple independent fixes to the same conceptual problem, each narrowing a specific symptom without addressing the shared root cause (Fowler's "Divergent Change"):

**`web-app/src/lib/hooks/useTerminalFlowControl.ts`'s resize oscillation handling** — fixed at least 5 times:
`20da32505`, `417d370dc`/`e5a42e730`, `9651edd5e`, `e8060fb1b`/`b5dba385e` (#728), and this investigation's fix (generalizing the #728 bounce check from an exact 2-back match to a small history window, since a real oscillation observed live on `staplersquad_stelekit` wandered across 3+ distinct values — `10x6 -> 67x38 -> 67x22 -> 67x38` — not a clean A-B-A flip-flop).

**`session/tmux/control_mode.go`'s subscriber-close/teardown timing** — fixed at least 4 times:
`6e6a9f676` (close slow subscribers instead of dropping bytes), `b0416e224` (add a grace period before closing), `c6bc8585c` (synchronize `controlModeDone` reads), BUG-090 (fixed: streamhub teardown races ahead of `StopControlMode`), plus BUG-086 (open: a refcounting data race under concurrent Start/Stop), plus this investigation's fix (moving the slow-subscriber grace-period wait off the tmux stdout read loop, since blocking it could stall tmux long enough that it stops noticing its own stdin close, forcing `StopControlMode`'s hard-kill path on every single teardown of a busy session).

## Root Cause

Neither file has a single place that owns "decide the final, settled value and apply it once":

- The resize pipeline reacts to every raw signal independently at each layer (`XtermTerminal`'s `ResizeObserver`, `useTerminalFlowControl`'s `resize()`, the server's control-mode resize+capture-pane round trip) — any upstream instability (a bouncing viewport) propagates through to real, expensive backend work every time, and each fix has added another special case to the send-path's gate rather than introducing one settling authority upstream of it.
- The control-mode subscriber/teardown lifecycle has similarly accreted independent protections (grace period, refcounting, generation tracking, deferred-close-on-in-flight-send) at the point each specific race was found, rather than being modeled as one explicit state machine.

## Why This Matters Beyond the Individual Fixes

The user's own observation during this investigation: the "shows Connected, nothing happens when typing" symptom was happening on sessions regardless of whether they were forced onto the stream-hub path — consistent with the root cause living in `control_mode.go`, which both the legacy and stream-hub transport paths share. A narrow per-symptom patch to either file is likely to need another narrow patch again the next time a new oscillation shape or teardown race is discovered, exactly as the git history above shows already happening.

## Recommended Fix Approach (not implemented here — scope decision, see below)

- **Resize**: introduce one client-side "resize settling" boundary (e.g. a dedicated hook) that owns debounce + oscillation detection + escalation once, and exposes only "call resize() with a value guaranteed settled" to the rest of the app — closing off the ability for a 6th fix to reimplement the judgment call ad hoc at the send-path layer again.
- **Control-mode teardown**: model the subscriber lifecycle (attached / draining / torn down, plus the in-flight-slow-send state this investigation added) as one explicit state machine with a single transition function, rather than the current pattern of independently-guarded fields (`slowSendInFlight`, `pendingCloseAfterDrain`, `controlModeExited`, refcounting) each added by a separate historical fix.

## Scope Decision

This investigation's fixes (generalized bounce detection with escalating hold; moving the slow-subscriber wait off the read loop) address the specific symptom chain that produced the live report, with full test coverage (see the accompanying PR). The deeper consolidations above are a larger, separate refactor — deliberately not attempted in the same change that fixes a live production issue. Filed here so the recurrence pattern is visible before the next narrow patch, per `quality:reflect-and-fix`'s Level 0 consolidation gate.

## Files Affected

- `web-app/src/lib/hooks/useTerminalFlowControl.ts`
- `session/tmux/control_mode.go`

## Related

- BUG-086 (open): control-mode refcounting race under concurrent Start/Stop
- BUG-090 (fixed): streamhub teardown state races ahead of `StopControlMode` call
