# BUG-014: Reconnect Backoff Always Resets on Unstable tmux Connection [SEVERITY: Medium]

**Status**: ✅ FIXED (2026-04-23)
**Discovered**: 2026-04-23 — observed via log spam during tmux server instability
**Fixed**: 2026-04-23 — `session/tmux/server_registry.go`
**Impact**: During tmux server instability, stapler-squad reconnect loop cycled at 100ms indefinitely, forking tmux subprocesses at ~10/sec and generating unnecessary syscall/CPU overhead. Under sustained instability this contributed to process table pressure.

## Resolution Summary

**Fix Applied**: Track connection start time; only reset `backoff` to `backoffBase` (100ms) if the connection was stable for at least 5 seconds. If the connection dies immediately, backoff is allowed to grow exponentially toward the 30s cap.

**Changes Made**:
1. `session/tmux/server_registry.go` — Added `connectTime := time.Now()` after `readLines()` setup; moved backoff reset inside a `minStableConnection` guard (5s).

**Before**:
```go
backoff = backoffBase // reset backoff on successful connect  ← unconditional
r.readLines(scanner)
```

**After**:
```go
connectTime := time.Now()
r.readLines(scanner)
const minStableConnection = 5 * time.Second
if time.Since(connectTime) >= minStableConnection {
    backoff = backoffBase
}
```

## Problem Description

`reconnectLoop` in `TmuxServerRegistry` resets `backoff = backoffBase` (100ms) immediately after `cmd.Start()` succeeds, before determining whether the connection is actually stable. When the tmux server is unhealthy:

1. `startControlMode()` calls `cmd.Start()` — succeeds (just fork+exec of tmux binary)
2. `syncSessions()` fails with `list-sessions: exit status 1` — warning logged, execution continues
3. `backoff = backoffBase` — resets to 100ms
4. `readLines()` returns immediately (control-mode exits at once; keepalive session missing)
5. Loop waits 100ms, doubles backoff… then next `cmd.Start()` succeeds again → step 3 fires again
6. Backoff never grows beyond 100ms; loop cycles indefinitely at ~10/sec

## Reproduction Steps

1. Start stapler-squad with a healthy tmux server
2. Kill the tmux server: `tmux kill-server`
3. Observe logs: `[registry] control-mode exited; reconnecting in 100ms` repeats at high frequency
4. Expected: backoff grows from 100ms → 200ms → 400ms → … → 30s
5. Actual: backoff stays at 100ms every cycle

## Root Cause

`cmd.Start()` succeeds even when the tmux server is dead (it just execs the tmux binary). This caused the code to treat every iteration as a "successful connect" and unconditionally reset backoff, defeating exponential backoff entirely during outages.

## Files Affected

- `session/tmux/server_registry.go` — `reconnectLoop()` function

## Verification

During tmux server instability, log messages should show increasing reconnect intervals:
```
[registry] control-mode exited; reconnecting in 100ms
[registry] control-mode exited; reconnecting in 200ms
[registry] control-mode exited; reconnecting in 400ms
...
[registry] control-mode exited; reconnecting in 30s
```

## Related Tasks

- Discovered while investigating zombie process accumulation (`fork/exec ... resource temporarily unavailable`)
