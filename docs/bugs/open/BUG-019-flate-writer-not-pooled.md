# BUG-019: compress/flate Writers Not Pooled [SEVERITY: Low]

**Status**: 🐛 Open
**Discovered**: 2026-04-24
**Impact**: HTTP response compression allocates a new flate writer per request. At 12MB resident and being the top active CPU consumer (18% of CPU samples), this wastes memory and adds latency on every compressed response.

## Problem Description

CPU profiling shows `compress/flate.(*compressor).findMatch` and `compress/flate.(*compressor).deflate` together consume ~18% of sampled CPU time. Heap profiling shows `compress/flate.NewWriter` holds 12MB (7% of heap). This is consistent with flate writers being allocated fresh on every compressed HTTP response rather than pooled and reset.

`compress/flate.NewWriter` is expensive (~32KB internal buffer per writer). Pooling via `sync.Pool` with `Reset()` is the standard mitigation and is already used elsewhere in the Go stdlib (e.g., `compress/gzip` in some frameworks).

Note: BUG-016 (fixed) addressed a similar issue for WebSocket per-message-deflate. This bug covers the general HTTP response compression path.

## Reproduction Steps

1. Run stapler-squad with `--profile`
2. Capture CPU profile: `curl -s --output cpu.prof "http://localhost:6060/debug/pprof/profile?seconds=15"`
3. Inspect: `go tool pprof -top cpu.prof`
4. Expected: compress/flate is not a top CPU consumer for a mostly-idle server
5. Actual: flate findMatch + deflate = ~18% of sampled CPU

## Root Cause

HTTP middleware (likely the `Compress` middleware in `server/server.go`) creates a new `flate.Writer` per response. Without pooling, each compressed response pays the full allocation and initialization cost.

## Files Likely Affected

- `server/server.go` — where the `Compress` middleware is wired
- HTTP compression middleware (whichever library provides it) — check if it exposes a writer pool option

## Fix Approach

1. Check if the compression middleware (e.g., `chi/middleware.Compress` or similar) has a built-in pool option.
2. If not, wrap with a `sync.Pool` that calls `w.Reset(dst)` before reuse.
3. If the middleware doesn't support injection, replace with a custom handler that manages the pool.

## Verification

After fix: `compress/flate.NewWriter` drops out of the heap top-20. CPU samples for `flate.findMatch`/`flate.deflate` should be proportional to actual data volume, not request count.

## Related Tasks

- BUG-016 (fixed): websocket per-message flate writer allocation — same root cause, different code path
- BUG-018: gob session persistence memory hotspot
