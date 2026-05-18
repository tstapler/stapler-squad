# Architecture Research: Terminal Escape Code Analytics

## Status

Research complete. All five design questions answered based on direct codebase exploration.

---

## Finding 1: Stage 1 Instrumentation is Already Partially Wired

The existing `pkg/analytics/EscapeCodeParser` is already integrated into
`session/response_stream.go` — but into the **in-memory-only** `EscapeCodeStore`, not SQLite.

**Exact hook point** (`response_stream.go`, lines 217–219):

```go
// Parse escape codes for analytics (passthrough - doesn't modify data)
if rs.escapeParser != nil {
    rs.escapeParser.Parse(chunk.Data)
}
```

This executes after `pty.Read(readBuf)` returns `n > 0` bytes, before `broadcast(chunk)`.
The `scrollback/CircularBuffer.Append` returns a `ScrollbackEntry` that has the
monotonically increasing `Sequence uint64` field — this is the natural `session_seq` to use
for Stage 1 vs Stage 2 correlation.

**Recommended approach**: Extend `EscapeCodeParser.Parse` to also emit to a new
`EscapeEventWriter` interface (see Question 3 below). The parser already handles split-sequence
buffering via `partialBuffer []byte`, satisfying NFR-2.

The `EscapeCodeParser` is constructed per-session in `NewResponseStream`/`NewResponseStreamWithBuffer`
using `analytics.GetGlobalStore()` and `analytics.NewEscapeCodeParser(store, sessionName)`.
The `sessionName` passed is the tmux session name string; for ent schema writes it should be
the stable UUID (`instance.GetStableID()`), obtained from the caller before `NewResponseStream`.

---

## Finding 2: Stage 2 Instrumentation Point

There are **two distinct streaming paths** for terminal output. Both must be instrumented:

### Path A: Control Mode (primary path for managed sessions)

`server/services/connectrpc_websocket.go`, `streamViaControlMode` function.
The critical send closure is at line ~688:

```go
sendData := func(data []byte) error {
    // ... marshal to proto
    return stream.WriteMessage(websocket.BinaryMessage, protocol.CreateEnvelope(0, dataBytes))
}
```

The coalescing loop (lines ~728–743) batches multiple `updateChan` frames into a single `buf`
before calling `sendData(buf)`. Stage 2 instrumentation should tap into `buf` immediately
**before** `sendData(buf)` is called, after coalescing — this records what was actually
written into the WebSocket frame, not raw individual reads.

### Path B: tmux Capture (legacy/external sessions)

`streamViaTmuxCapture` function, around line 1140. The data originates from
`streamer.AddConsumer` callbacks and is written at line ~1156. This path sends full
terminal snapshots (with clear-screen prefix), not raw PTY bytes, so Stage 2 records here
would observe structurally different data from Stage 1. Mangle detection across paths A
and B requires separate correlation buckets.

### Path C: SessionService.StreamSession (ConnectRPC bidirectional stream)

`session_service.go` around line 1225. This reads directly from `ptyFile.Read(buf)` and
sends via `stream.Send(stateMsg)` (MOSH-style terminal state). This path bypasses
`ResponseStream` entirely and thus bypasses the existing Stage 1 `EscapeCodeParser`.
Stage 1 instrumentation here requires a parallel tap in this goroutine.

**Recommended**: Focus Phase 1 Stage 2 instrumentation on Path A (control mode) only, as
it carries the highest-fidelity PTY bytes and is used by all managed claude sessions.

---

## Finding 3: EscapeParser Threading — Decorator/Writer Interface Pattern

The existing design uses a global singleton store (`analytics.GetGlobalStore()`) which is
anti-pattern for multi-database or testable injection. The requirements call for SQLite
persistence via ent ORM.

**Recommended pattern: `EscapeEventWriter` interface injected into `EscapeCodeParser`**

```go
// pkg/analytics/escape_event_writer.go
type EscapeEventWriter interface {
    WriteEscapeEvent(ctx context.Context, event EscapeEventRecord) error
}

type EscapeEventRecord struct {
    SessionID    string
    Stage        Stage  // "pty_read" | "transport" | "browser"
    SequenceType string
    Subtype      string
    ByteLen      int
    PayloadHash  string  // SHA-256 prefix, if capture_level != "off"
    RawBytes     []byte  // only if capture_level == "full"
    SessionSeq   uint64  // from scrollback.CircularBuffer.Sequence
    WallTime     time.Time
}
```

The `EscapeCodeParser` gains an optional `EscapeEventWriter` field alongside its existing
`*EscapeCodeStore`. The `Parse` method calls both the in-memory store (existing behavior)
and the new writer (new SQLite path) when non-nil.

**Wiring**: `ResponseStream.NewResponseStream` should accept an `EscapeEventWriter` as an
optional parameter (or via a setter `SetEventWriter`). The `ResponseStream` passes the
current `scrollback.CircularBuffer.Sequence` counter into each `Parse` call so the writer
can stamp `session_seq`.

**Zero-overhead "off" path**: Implement a `NoopEscapeEventWriter` that satisfies the
interface with empty methods. When `EscapeAnalyticsCaptureLevel == "off"`, inject the noop.
No allocation on hot path; the `if writer != nil` guard is branch-predictor-friendly.

---

## Finding 4: Ent Schema — New `escape_event` Entity (Do Not Reuse `analytics_event`)

The existing `analytics_event` entity (see `session/ent/schema/analytics_event.go`) is
designed for coarse UI/session lifecycle events with a generic `labels map[string]string`
bag. Reusing it for escape sequences would:

- Force all typed fields (`stage`, `sequence_type`, `byte_length`, `session_seq`,
  `payload_hash`, `raw_bytes`, `mangled`, `mangle_type`) into the untyped labels blob,
  breaking queryability and index efficiency.
- Mix high-volume escape data (~thousands/session) with low-volume lifecycle events
  (~dozens/session), polluting queries and retention logic.

**Decision: New `escape_event` ent schema entity.**

```go
// session/ent/schema/escape_event.go
type EscapeEvent struct{ ent.Schema }

func (EscapeEvent) Fields() []ent.Field {
    return []ent.Field{
        field.String("id").Unique().NotEmpty().Immutable(),
        field.String("session_id").NotEmpty(),
        field.String("stage").NotEmpty(),           // "pty_read" | "transport" | "browser"
        field.String("sequence_type").NotEmpty(),   // CSI, OSC, DCS, Simple, etc.
        field.String("sequence_subtype").Optional(), // SGR, Cursor-Up, Alternate-Screen, etc.
        field.Int("byte_length"),
        field.String("payload_hash").Optional(),    // SHA-256 hex prefix (summary/full)
        field.Bytes("raw_bytes").Optional(),        // only if capture_level=full
        field.Bool("mangled").Default(false),
        field.String("mangle_type").Optional(),     // truncated | stripped | mutated | empty
        field.Time("wall_time").Immutable(),
        field.Int64("session_seq"),                 // scrollback sequence at observation
    }
}

func (EscapeEvent) Indexes() []ent.Index {
    return []ent.Index{
        index.Fields("session_id"),
        index.Fields("session_id", "stage"),
        index.Fields("session_id", "session_seq"),  // for mangle correlation lookup
        index.Fields("wall_time"),
        index.Fields("mangled"),
        index.Fields("sequence_type"),
    }
}
```

The composite index on `(session_id, session_seq)` is the critical one for Stage 2 mangle
lookups (FR-7). The `stage` field as a string (not Go iota enum) avoids proto/ent impedance
mismatch when values cross boundaries.

**Storage location**: Same `analytics.db` SQLite file as `analytics_event`. The
`server/analytics/db.go` `OpenAnalyticsDB` already calls `client.Schema.Create(ctx)` which
is additive — adding `EscapeEvent` to the ent schema will migrate the table on next start.
No separate database file is needed; SQLite WAL mode handles concurrent reads from the
analytics and mangle-detection goroutines without contention.

---

## Finding 5: Mangle Detection — In-Memory Correlation Map

The requirements specify byte-for-byte comparison between Stage 1 and Stage 2 observations
keyed on `(session_id, session_seq)` (FR-7).

**Recommended approach: `MangleCorrelator` — a bounded in-memory map**

```go
// pkg/analytics/mangle_correlator.go
type Stage1Observation struct {
    PayloadHash string
    ByteLen     int
    WallTime    time.Time
}

type MangleCorrelator struct {
    mu      sync.Mutex
    // key: sessionID + ":" + strconv.FormatUint(sessionSeq, 10)
    pending map[string]Stage1Observation
    maxAge  time.Duration  // evict if Stage 2 doesn't arrive within maxAge (default 5s)
}
```

**Flow**:
1. Stage 1 (`ResponseStream.streamLoop`): after parsing, call
   `correlator.RecordStage1(sessionID, sessionSeq, hash, byteLen)`. This inserts into
   `pending` map.
2. Stage 2 (`streamViaControlMode` sendData closure): before writing to WebSocket, call
   `correlator.CheckStage2(sessionID, sessionSeq, hash, byteLen)`. This looks up the
   pending entry:
   - If found and hash matches: no mangle, evict from map, write ent row with `mangled=false`.
   - If found and hash differs: mangle detected, write ent row with `mangled=true`,
     `mangle_type` set appropriately.
   - If not found after maxAge: write Stage 1 row with `mangled=true`, `mangle_type="stripped"`
     (Stage 2 never saw it).

**Bounded growth**: After `maxAge` (5s), an eviction goroutine flushes entries that have
no Stage 2 counterpart, writing them as "stripped" escape events. The bounded map prevents
unbounded memory growth if sessions are high-throughput or Stage 2 is slow/disconnected.

**Session_seq resolution**: The `scrollback.CircularBuffer` already has a monotonically
increasing `sequence uint64` per `Append`. The `ResponseStream` currently writes to
`rs.ptyAccess.buffer.Write(chunk.Data)` (the raw PTY `CircularBuffer`, not the
`scrollback.CircularBuffer`). The `session_seq` for correlation should use a separate
per-session atomic counter incremented on each PTY read chunk in `streamLoop`, or tap the
`scrollback.Manager.CurrentSequence(sessionID)` after each `AppendOutput`. The latter is
preferable since it's already persisted and stable across restarts.

---

## Summary of Key Architectural Decisions

| Question | Decision |
|---|---|
| Stage 1 tap | `ResponseStream.streamLoop` after `pty.Read`, immediately after the existing `escapeParser.Parse` call. Already wired; extend `EscapeCodeParser` to call an injected `EscapeEventWriter`. |
| Stage 2 tap | `streamViaControlMode.sendData` closure, **after coalescing loop**, before `stream.WriteMessage`. Tap `buf` slice directly. |
| Parser threading | `EscapeEventWriter` interface injected into `EscapeCodeParser`; `NoopEscapeEventWriter` for `capture_level=off`. No global state changes needed. |
| Ent schema | New `escape_event` entity in same `analytics.db`. Do not reuse `analytics_event` — fields are incompatible; volume is orders of magnitude higher. |
| Mangle detection | In-memory `MangleCorrelator` with bounded pending map keyed on `(session_id, session_seq)`, eviction after 5s TTL, flushed as "stripped" events on timeout. |

---

## Open Architecture Concerns

1. **`session_seq` source for Stage 2**: The control-mode path (`streamViaControlMode`)
   consumes from `updateChan` (a `chan []byte` from `SubscribeControlModeUpdates`), not
   directly from the PTY. There is no scrollback sequence number attached to these frames.
   A lightweight sequence counter injected by Stage 1 into the channel payload (or a
   side-channel `sync.Map`) is needed to bridge the two stages.

2. **Coalescing complicates per-sequence correlation**: The Stage 2 coalescing loop batches
   multiple Stage 1 chunks into one `buf`. After coalescing, `buf` may contain sequences
   spanning multiple Stage 1 `session_seq` values. Options:
   - Tag each Stage 1 observation with a range `(seqStart, seqEnd)` instead of a single
     sequence number.
   - Parse Stage 2 `buf` independently with a second `EscapeCodeParser` pass and do
     set-intersection comparison against Stage 1's recorded sequences.
   The second option is more robust and aligns with FR-3 "tolerant parser" design.

3. **`SessionService.StreamSession` path (Path C)** bypasses `ResponseStream` entirely.
   Phase 1 can defer instrumentation of this path; it uses MOSH-style terminal state
   synthesis which transforms raw PTY bytes before sending, making byte-for-byte mangle
   detection semantically different from the control-mode path.
