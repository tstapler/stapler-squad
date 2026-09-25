# BUG-086: `TmuxSession` control-mode refcounting has a data race under concurrent Start/Stop [SEVERITY: Medium]

**Status**: ✅ FIXED (2026-09-22)
**Resolution**: The race was not in the refcount/subscriber state (already guarded by `controlModeSubMu` everywhere) but in `readControlModeOutput` and `monitorControlModeErrors` each reading `t.controlModeDone`/`t.controlModeStdout` directly from inside their own goroutine instead of capturing them once at spawn time. `StartControlMode`/`startRemoteControlMode` now pass `doneCh`/`stdout` (and `monitorControlModeErrors`'s `doneCh`) as explicit parameters, captured under `controlModeSubMu` at the same moment the fields are assigned — mirroring the fix `runCMSender` already had for its own channel parameters (PR #704). Verified: `go test ./session/tmux/... -race -count=5` and the new interleaved-Start/Stop repro test both pass clean, no `WARNING: DATA RACE`, no panics.
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

**Restored repro**: `session/tmux/control_mode_interleaved_start_stop_test.go`'s `TestControlMode_InterleavedConcurrentStartStop_NoDataRace` races 100 goroutines, each doing a real `StartControlMode()` immediately followed by `StopControlMode()`, against one shared real tmux session (isolated `-L` socket). Before the fix below, this reliably crashed under `-race` with `panic: close of nil channel` at `control_mode.go`'s scanner-EOF cleanup (not the exact `WARNING: DATA RACE` line/field originally reported — line numbers had drifted across intervening commits, including PR #704's unrelated fix to `runCMSender`'s channel-capture race — but the same missing-synchronization root cause: a reader-goroutine field read racing a concurrent `StopControlMode` write).

## Root Cause

`readControlModeOutput` and `monitorControlModeErrors` each captured `doneCh` from `t.controlModeDone` *inside their own goroutine body*, under `controlModeSubMu.RLock()`, rather than being handed the value by the goroutine that spawned them (`StartControlMode`/`startRemoteControlMode`). Because `go t.readControlModeOutput()` returning does not mean the spawned goroutine has actually run yet, there is a window — between `StartControlMode` unlocking `controlModeStartMu` and the new reader goroutine reaching its own `RLock()`/capture — during which a full concurrent `StopControlMode` (refcount to 0) can already tear the generation down: close `doneCh`, nil `t.controlModeDone`, and (unguarded by any lock) close and nil `t.controlModeStdout`. The reader goroutine then captures `doneCh == nil` and builds its scanner over an already-closed `stdout`, so `scanner.Scan()` fails immediately with `"file already closed"`, and the post-loop cleanup's `if t.controlModeDone == doneCh { close(doneCh); ... }` compares `nil == nil`, evaluates true, and panics on `close(nil)`.

This is the same bug class PR #704 (`6c3f3b69d`) already fixed for `runCMSender`'s `highPriSendCh`/`normPriSendCh`/`cmSenderExited` parameters (see that commit's message: "a unilateral %exit ... resets controlModeCmd to let a fresh StartControlMode() proceed ... so a new StartControlMode call can reassign those same t.* fields to new channels while this goroutine is still running against the old ones") — it just hadn't been applied to `readControlModeOutput`'s own `doneCh`/`stdout` capture or to `monitorControlModeErrors`'s `doneCh` capture, both of which retained the pre-#704 self-capture pattern.

The refcount (`controlModeRefCount`), `controlModeCmd`/`controlModeRemoteProc`, and subscriber map were *not* the racing fields — every read/write of those is already correctly guarded by `controlModeSubMu` on both the `StartControlMode`/`StopControlMode` side and the `processControlModeLine`/`handleBeginNotification`/`handleExitNotification` side.

## Fix

- `readControlModeOutput(doneCh chan struct{}, stdout io.ReadCloser)` and `monitorControlModeErrors(stderr io.ReadCloser, doneCh <-chan struct{})` now take `doneCh`/`stdout` as explicit parameters instead of reading `t.controlModeDone`/`t.controlModeStdout` from inside the goroutine.
- `StartControlMode` and `startRemoteControlMode` capture `doneCh`/`stdout` as local variables at the same point they already captured `highPriSendCh`/`normPriSendCh`/`cmSenderExited` for `runCMSender` (under `controlModeSubMu.Lock()`, before spawning any goroutine), and pass them explicitly: `go t.readControlModeOutput(doneCh, stdout)`, `go t.monitorControlModeErrors(stderr, doneCh)`.
- `session/tmux/control_mode_test.go`'s `startReader` test helper updated to read `sess.controlModeDone`/`sess.controlModeStdout` once (single-threaded test setup, no concurrent Start/Stop) and pass them the same way.
- New regression test: `session/tmux/control_mode_interleaved_start_stop_test.go`.

Verified:
- `go test ./session/tmux/... -run TestControlMode_InterleavedConcurrentStartStop_NoDataRace -race -count=5` — clean, no panics, no races.
- `go test ./session/tmux/... -race -count=5` (full package) — clean, ~253s, no failures.
- `go test ./session -run TestInstanceStartControlMode_should_NeverProduceTwoOwners -race -count=2` — still passes (this test's own `t.Cleanup`-based Stop workaround is no longer necessary for correctness, but was left in place since it targets a different invariant — see its updated doc comment).
- `go build ./...` and `golangci-lint run ./session/tmux/... ./session/...` — clean.

## Files Likely Affected

- `session/tmux/control_mode.go` — `StartControlMode` (~line 135), `StopControlMode` (~line 210), `processControlModeLine` (~line 487), `readControlModeOutput` (~line 276).

## Verification

`go test ./session/... -run <a test that interleaves concurrent Start/Stop against one shared real tmux session> -race -count=5` passes clean, no `WARNING: DATA RACE`.

## Related Tasks

Discovered while adding `session/instance_control_mode_ownership_test.go` for `project_plans/terminal-multi-connection-streaming/`'s Story 3.1.2 (wiring `streamhub.AcquireOwnershipLock` into `Instance.StartControlMode` itself, not just the RPC-handler entry points). Confirmed unrelated to that fix or to the streamhub package — the race is entirely within `session/tmux/control_mode.go`'s pre-existing refcounting, with no `streamhub` code on either side of the race trace. Filed rather than silently worked around, per `.claude/rules/fix-flaky-tests-dont-defer.md`.
