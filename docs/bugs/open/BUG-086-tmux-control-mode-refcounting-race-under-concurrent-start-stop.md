# BUG-086: `TmuxSession` control-mode refcounting has a data race under concurrent Start/Stop [SEVERITY: Medium]

**Status**: 🐛 Open
**Discovered**: 2026-08-21
**Impact**: `session/tmux/control_mode.go`'s `StartControlMode`/`StopControlMode` refcounting has an unsynchronized read/write when many Start/Stop pairs interleave concurrently against one tmux session. Not yet observed in production (this is a pre-existing race, not a new regression), but confirmed real and reproducible under `-race` at realistic-but-high concurrency (100 goroutines).

## Problem Description

While adding `session/instance_control_mode_ownership_test.go` (Story 3.1.2 of `project_plans/terminal-multi-connection-streaming/`) — an integration test racing 100 goroutines' `Instance.StartControlMode()` calls each immediately followed by `StopControlMode()` against one real tmux session — `go test -race` reported:

```
WARNING: DATA RACE
Write at 0x00c0003243c8 by goroutine N:
  session/tmux.(*TmuxSession).processControlModeLine()
      session/tmux/control_mode.go:487
  session/tmux.(*TmuxSession).readControlModeOutput()
      session/tmux/control_mode.go:276
  session/tmux.(*TmuxSession).StartControlMode.gowrap3()
      session/tmux/control_mode.go:135

Previous read at 0x00c0003243c8 by goroutine M:
  session/tmux.(*TmuxSession).StopControlMode()
      session/tmux/control_mode.go:210
```

The reader goroutine spawned by `StartControlMode` (`control_mode.go:135`) writes some refcount/state field in `processControlModeLine` (`control_mode.go:487`) unsynchronized against a concurrent `StopControlMode` read of the same field (`control_mode.go:210`).

## Reproduction Steps

1. In `session/instance_control_mode_ownership_test.go`, have each of 100 goroutines call `inst.StartControlMode()` immediately followed by `inst.StopControlMode()` against one real tmux session shared across all 100 goroutines.
2. Run `go test ./session/... -run TestInstanceStartControlMode -race -v`.
3. Expected: no data race.
4. Actual: `WARNING: DATA RACE` fires reliably (observed on the first run).

This bug's own test was subsequently rewritten to no longer interleave concurrent Start/Stop pairs (calling `StopControlMode` once via `t.Cleanup` after all `Start` calls complete instead), so it no longer exercises this race — that rewrite is a workaround for *this* bug, not a fix.

## Root Cause

Not fully investigated. `session/tmux/control_mode.go:487`'s write (inside `processControlModeLine`, running on the background reader goroutine `readControlModeOutput` spawns) and `control_mode.go:210`'s read (inside `StopControlMode`, called synchronously by whatever goroutine calls it) touch the same field with no synchronization between them. Likely candidates: a refcount or "control mode active" boolean/counter field on `TmuxSession` that both the reader goroutine and `StopControlMode` touch without a mutex or atomic.

## Files Likely Affected

- `session/tmux/control_mode.go` — `StartControlMode` (~line 135), `StopControlMode` (~line 210), `processControlModeLine` (~line 487), `readControlModeOutput` (~line 276).

## Fix Approach

Not yet investigated. Likely needs the racing field converted to an atomic type, or guarded by `TmuxSession`'s existing mutex (if one already covers adjacent state) across both the reader goroutine's write and `StopControlMode`'s read.

## Verification

`go test ./session/... -run <a test that interleaves concurrent Start/Stop against one shared real tmux session> -race -count=5` passes clean, no `WARNING: DATA RACE`.

## Related Tasks

Discovered while adding `session/instance_control_mode_ownership_test.go` for `project_plans/terminal-multi-connection-streaming/`'s Story 3.1.2 (wiring `streamhub.AcquireOwnershipLock` into `Instance.StartControlMode` itself, not just the RPC-handler entry points). Confirmed unrelated to that fix or to the streamhub package — the race is entirely within `session/tmux/control_mode.go`'s pre-existing refcounting, with no `streamhub` code on either side of the race trace. Filed rather than silently worked around, per `.claude/rules/fix-flaky-tests-dont-defer.md`.
