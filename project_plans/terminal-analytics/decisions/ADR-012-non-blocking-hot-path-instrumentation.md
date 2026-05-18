# ADR-012: Non-Blocking Hot-Path Instrumentation for Escape Sequence Analytics

## Status

Accepted

## Context

The terminal analytics feature requires instrumenting the PTY read path (Stage 1) to capture
escape sequence observations for every session. The PTY read loop in `session/response_stream.go`
runs in a tight goroutine: it calls `pty.Read(readBuf)`, passes the chunk to
`escapeParser.Parse(chunk.Data)`, and then broadcasts the chunk to all connected WebSocket
clients. This path has a hard latency budget of **< 50 µs overhead per 4 KB chunk** (NFR-1).

Three performance problems make synchronous or blocking instrumentation unsafe on this path:

1. **`EscapeCodeStore.Record` write-lock contention**: The existing in-memory store acquires a
   full `sync.Mutex` write-lock on every call and performs an O(N) eviction scan under that lock.
   At typical vim-redraw rates (hundreds of sequences per frame, 60 fps), all PTY goroutines
   serialize through this lock. Extending this pattern to a SQLite write would completely consume
   the 50 µs budget.

2. **Synchronous SQLite write**: A direct `INSERT` per sequence via the ent ORM involves a
   syscall, a SQLite B-tree write, and potentially a WAL flush. Even with WAL mode the median
   insert latency is 10–100 µs — 2–4× the entire per-chunk budget.

3. **Channel backpressure propagation**: If the analytics writer goroutine falls behind (disk
   I/O spike, WAL auto-checkpoint), a blocking channel send on the PTY read path propagates
   backpressure all the way to the PTY `read()` syscall, causing the terminal to appear frozen
   to the user.

The `EscapeCodeStore.Record` O(N) eviction pitfall is the most critical risk: the pitfalls
research identified that the existing eviction path does a full scan + sort under the write
lock, which is O(N) in the number of stored entries. Any new SQLite-backed writer must not
replicate this pattern.

## Decision

**Use a non-blocking buffered channel send from the PTY read path; drop events under
backpressure rather than stalling.**

The implementation has four components:

### 1. Non-blocking channel send on hot path

The `EscapeEventWriter` implementation enqueues observations via a non-blocking select:

```go
select {
case w.ch <- event:
default:
    atomic.AddInt64(&w.dropCount, 1)  // observable; logged at session close
}
```

The channel is buffered with capacity 1000. At typical escape sequence rates (~500
sequences/second during active vim sessions), this provides ~2 seconds of backpressure
absorption before any drops occur. The drop counter is included in the per-session close
summary log line (NFR-4).

### 2. Background batch writer goroutine

A single background goroutine owned by the `EscapeEventBatchWriter` drains the channel and
flushes to SQLite using ent `CreateBulk`:

- Flush trigger: 100 rows accumulated, **or** 500 ms ticker, whichever comes first (FR-4).
- Single ent client / single SQLite connection: all writes serialized through one goroutine,
  eliminating lock contention entirely.
- WAL auto-checkpoint disabled (`_wal_autocheckpoint=0`); explicit `PRAGMA wal_checkpoint(PASSIVE)`
  issued at session close to avoid mid-burst stalls.

### 3. Graceful shutdown with drain

On context cancellation the goroutine drains remaining channel contents before returning:

```go
case <-ctx.Done():
    for len(w.ch) > 0 {
        // drain remaining into final batch
    }
    w.flush(ctx, batch)
    return
```

This prevents in-flight observations from being silently lost at application shutdown.

### 4. Per-session row cap enforced in memory

`EscapeAnalyticsMaxRowsPerSession` (default 10,000) is maintained as an in-memory counter
per session inside the batch writer, not via a per-write `SELECT COUNT(*)` query. When the
counter reaches the cap, the `select { default: drop }` path is taken regardless of channel
fullness, avoiding any DB query on the hot path.

## Alternatives Considered

### Blocking channel send

Simplifies backpressure handling (no drop counter needed), but propagates I/O stalls directly
to the PTY reader goroutine, causing visible terminal freezes. Unacceptable given the
interactive nature of the terminal sessions being instrumented.

### Synchronous SQLite write per sequence

Achieves the lowest latency for individual observations (no channel hop) but breaks NFR-1
immediately: even a 10 µs SQLite insert exceeds the per-sequence budget when hundreds of
sequences arrive per chunk. Also replicates the `EscapeCodeStore` write-lock anti-pattern.

### In-process ring buffer only (no SQLite persistence)

The existing `EscapeCodeStore` already provides this. An in-process ring buffer satisfies
NFR-1 trivially but does not satisfy FR-2 (persist to SQLite), FR-5 (ConnectRPC query), or
FR-6 (web UI analytics page). The ring buffer remains useful as a fast in-memory staging area
but cannot replace the persistent store.

## Consequences

**Positive:**
- PTY read path overhead is bounded to a single non-blocking channel send (~50–100 ns) plus
  the parser's O(chunk_length) scan — well within the 50 µs/4 KB budget.
- SQLite write pressure is decoupled from terminal throughput; a WAL checkpoint or disk stall
  affects only analytics latency, never terminal responsiveness.
- The single-writer goroutine pattern eliminates all mutex contention on the write path.

**Negative / Trade-offs:**
- Events may be dropped under sustained high throughput or prolonged writer stalls. The drop
  counter mitigates observability loss, but analytics completeness is best-effort, not
  guaranteed.
- The 500 ms flush interval means the most recent observations may not be queryable for up to
  500 ms after they are captured. Acceptable for a diagnostic/analytics feature; not
  acceptable for real-time alerting (which is not a requirement).
- Graceful shutdown must drain the channel; a hard kill (SIGKILL) loses in-flight
  observations. This is documented behavior.

## References

- NFR-1: < 50 µs overhead per 4 KB chunk on Stage 1 instrumentation path
- FR-4: Batch writes, flush every 500 ms or 100 rows
- Pitfalls §2.1: `EscapeCodeStore.Record` write-lock + O(N) sort on every call
- Pitfalls §2.2: Channel backpressure / goroutine leak
- Pitfalls §2.3: WAL auto-checkpoint contention
- Stack research §Q3: Channel + Ticker batch flusher pattern with ent `CreateBulk`
- Architecture research Finding 3: `EscapeEventWriter` interface injected into `EscapeCodeParser`
