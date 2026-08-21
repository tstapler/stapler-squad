# BUG-015: EventBus Goroutine Leak from Nil context.Background() [SEVERITY: Low]

**Status**: ✅ FIXED (2026-04-23)
**Discovered**: 2026-04-23 — identified via pprof goroutine profile (`localhost:6060/debug/pprof/goroutine?debug=2`)
**Fixed**: 2026-04-23 — `server/server.go`
**Impact**: 2 goroutines permanently blocked for the lifetime of the process. Fixed count — not a growing leak — but the `StartSubscriber` and `StartPushSubscriber` subscriptions were never cleaned up on server shutdown.

## Resolution Summary

**Fix Applied**: Changed `serverCtx := context.Background()` → `serverCtx := connCtx` at `server/server.go:100`. `connCtx` is already cancelled during `Shutdown()` via `connCtxCancel`.

**Changes Made**:
1. `server/server.go:100` — one-line change from `context.Background()` to `connCtx`

## Problem Description

`events.EventBus.Subscribe(ctx)` spawns a cleanup goroutine:

```go
go func() {
    <-ctx.Done()   // ← blocks forever if ctx is context.Background()
    eb.Unsubscribe(id)
}()
```

`context.Background().Done()` returns a **nil channel**. Receiving on a nil channel blocks forever in Go. Two callers in `server.go` passed `context.Background()` as the context:

- `notifications.StartSubscriber(serverCtx, ...)`
- `push.StartPushSubscriber(serverCtx, ...)`

Both spawned cleanup goroutines that would never unblock, meaning:
1. The goroutines leaked for the process lifetime
2. On server shutdown, `Unsubscribe` was never called for these two subscriptions

## Reproduction Steps

1. Run stapler-squad with `--profile`
2. `curl -s 'localhost:6060/debug/pprof/goroutine?debug=2' | grep -A5 "nil chan"`
3. Expected: no goroutines blocked on nil channel
4. Actual: 2 goroutines in state `[chan receive (nil chan), N minutes]` at `server/events/bus.go:41`

## Root Cause

`serverCtx` was set to `context.Background()` which has no cancellation signal. The `connCtx` variable was already available in the same scope and is the correct lifetime context for server background workers — it is cancelled during `Shutdown()`.

## Files Affected

- `server/server.go` — `NewServer()`, line 100
- `server/events/bus.go` — `Subscribe()`, line 40-43 (correct implementation, wrong context passed by caller)

## Verification

After restart:
```bash
curl -s 'localhost:6060/debug/pprof/goroutine?debug=2' | grep "nil chan"
# Should return empty
```

## Related Tasks

- Discovered during pprof profiling analysis session (2026-04-23)
