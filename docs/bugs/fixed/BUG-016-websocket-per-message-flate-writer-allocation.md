# BUG-016: WebSocket Per-Message flate.Writer Allocation Causes GC Pressure [SEVERITY: Low]

**Status**: ✅ FIXED (2026-04-23)
**Discovered**: 2026-04-23 — via pprof heap profile
**Fixed**: 2026-04-23 — `server/services/connectrpc_websocket.go`, `server/services/terminal_websocket.go`
**Impact**: Every outgoing WebSocket message allocates a new `flate.Writer` (deflate compressor) rather than reusing one from a pool. Cumulative allocation: **24.9 MB** in a short session. This generates GC pressure on active terminal streaming connections.

## Problem Description

The gorilla/websocket library used for terminal streaming is configured with `NoContextTakeover` compression. In this mode, each message is compressed independently (no shared LZ77 sliding window between messages), which requires allocating a fresh `flate.Writer` per message rather than reusing one.

Pprof heap profile shows:
```
1.0 MB in-use / 24.9 MB total allocated
services.(*ConnectRPCWebSocketHandler).streamViaControlMode.func4
  → websocket.compressNoContextTakeover
    → flate.(*compressor).init
```

24.9 MB allocated for compression objects in a single relatively short session. Under sustained terminal streaming (multiple active sessions, high output rate), this translates to continuous GC work and allocation spikes.

## Reproduction Steps

1. Run stapler-squad with `--profile`
2. Open several active terminal streaming connections in the web UI
3. `curl -s 'localhost:6060/debug/pprof/heap?debug=1' > heap.txt`
4. Look for `websocket.compressNoContextTakeover` in the allocators
5. Expected: minimal allocations for compression (pooled writers)
6. Actual: large cumulative allocations from per-message compressor init

## Root Cause

`websocket.NoContextTakeover` is configured (likely via `websocket.Upgrader` or `websocket.Conn.SetCompressionLevel`). This RFC-compliant mode ensures each message can be decompressed without context from prior messages, at the cost of preventing compressor reuse.

Alternatives:
- **`contextTakeover` mode**: Reuses the same compressor across messages (shared LZ77 window). Better compression ratio AND eliminates per-message allocation. Client must support `permessage-deflate` with context takeover — all modern browsers do.
- **`sync.Pool` for compressors**: Pool `flate.Writer` instances even in NoContextTakeover mode. Requires resetting state between uses.

## Files Likely Affected

- `server/services/connectrpc_websocket.go` — WebSocket upgrade and compression configuration
- Possibly `server/server.go` or middleware — where `websocket.Upgrader` is configured

## Fix Approach

**Option A (Preferred)**: Switch to `contextTakeover` if client compatibility allows. Eliminates per-message allocation entirely and improves compression ratio.

**Option B**: Pool `flate.Writer` instances via `sync.Pool`. Writers would be reset via `flate.Writer.Reset(w)` before reuse. Requires wrapping gorilla/websocket's compression internals.

**Option C**: Accept as-is if GC pauses are not measurable in practice. NoContextTakeover is simpler and more compatible.

Verify with before/after heap profiles under load before committing.

## Verification

After fix, `curl localhost:6060/debug/pprof/heap?debug=1` should show near-zero cumulative allocations for `websocket.compressNoContextTakeover` under equivalent streaming load.

## Related Tasks

- Discovered alongside BUG-017 (SQLite startup allocation) during pprof analysis session 2026-04-23
