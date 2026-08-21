# Terminal Escape Code Analytics — Implementation Plan

**Version**: 1.0  
**Date**: 2026-05-14  
**Status**: Ready for implementation  
**References**: requirements.md, research/stack.md, research/features.md, research/architecture.md, research/pitfalls.md

---

## Overview

This plan implements a full-pipeline terminal escape sequence analytics system for stapler-squad. The system instruments two stages of the PTY→browser pipeline, persists events to SQLite via ent ORM, performs in-memory mangle detection, and exposes a ConnectRPC query API with a React web UI.

### Key Architectural Decisions

| Decision | Choice | Rationale |
|---|---|---|
| Parser | Extend `pkg/analytics/escape_code_parser.go` | Already complete FR-1 implementation; returns raw bytes; split-sequence buffering present |
| Zero-overhead off-path | Bool guard on `EscapeCodeParser.Parse()` + `NoopEscapeEventWriter` interface | Consistent with existing `AnalyticsProvider` noop pattern |
| SQLite persistence | Channel+Ticker batch writer using ent `CreateBulk`, flush at 100 rows / 500ms | FR-4 spec match; `CreateBulk` already generated; WAL already configured |
| Mangle detection | In-memory `MangleCorrelator` with bounded pending map, 5s TTL | O(1) lookup; avoids cross-batch SQLite race (pitfall 4.2) |
| DB file | Same `analytics.db` (not a separate file) | Schema migration is additive; WAL handles concurrent read/write |
| `session_seq` source | Add `totalBytesWritten int64` to `CircularBuffer` | **CRITICAL**: current `count` field is not monotonic across overwrites (pitfall 4.1) |
| Stage 2 tap point | `streamViaControlMode` after coalescing loop, before `sendData(buf)` | Captures coalesced frame as actually written to WebSocket |
| OSC redaction | Config flag `EscapeAnalyticsRedactOSCPayloads` defaulting `true` | OSC 52 clipboard can contain PII; security pitfall 5.2 |

### ADR Flags (see "Technology Decisions Requiring ADRs" section at end)

- ADR-TBA-1: Database file strategy (same analytics.db vs. separate escape_events.db)
- ADR-TBA-2: `session_seq` definition and `CircularBuffer` monotonic counter
- ADR-TBA-3: Mangle detection architecture (inline vs. deferred reconciliation)
- ADR-TBA-4: Stage 2 coalescing — set-intersection parse vs. range tagging

---

## Epic 1: Extend EscapeParser + `session_seq` tracking + OSC redaction

**Goal**: Extend the existing `pkg/analytics/escape_code_parser.go` to emit structured events to a new `EscapeEventWriter` interface, track cumulative session byte offset (`session_seq`), and redact OSC payloads that contain PII.

**Existing hook**: `session/response_stream.go` line ~217 already calls `rs.escapeParser.Parse(chunk.Data)`. This epic extends that call without changing the signature of the surrounding code.

---

### Story 1.1: Add `EscapeEventWriter` interface and noop implementation

**Goal**: Define the decoupled interface that all downstream consumers (SQLite batch writer, test spies) implement. This is the dependency-injection boundary.

#### E1-S1-T1 — Define `EscapeEventWriter` interface and `EscapeEventRecord` struct
- **File**: `pkg/analytics/escape_event_writer.go` (new file)
- **Contents**:
  - `type Stage string` with constants `StagePTYRead`, `StageTransport`, `StageBrowser`
  - `type EscapeEventRecord struct` with all FR-2 fields: `SessionID`, `Stage`, `SequenceType`, `SequenceSubtype`, `ByteLen`, `PayloadHash`, `RawBytes`, `Mangled`, `MangleType`, `WallTime`, `SessionSeq`
  - `type EscapeEventWriter interface { WriteEscapeEvent(ctx context.Context, event EscapeEventRecord) }`
  - `type NoopEscapeEventWriter struct{}` with empty `WriteEscapeEvent` body
- **Complexity**: S
- **Notes**: No dependencies on ent or SQLite. Testable in isolation.

#### E1-S1-T2 — Add `EscapeEventWriter` field to `EscapeCodeParser`
- **File**: `pkg/analytics/escape_code_parser.go` (extend, do not rewrite)
- **Change**: Add `writer EscapeEventWriter` and `sessionID string` fields to `EscapeCodeParser` struct. Add `SetEventWriter(w EscapeEventWriter, sessionID string)` setter. When `writer != nil`, call `writer.WriteEscapeEvent(ctx, record)` after each successfully parsed sequence in `extractEscapeSequences`.
- **Complexity**: S
- **Notes**: Existing `EscapeCodeStore.Record()` path is unchanged. The new writer is called alongside, not instead of, the existing store. Use `context.Background()` for now; Epic 4 will thread a real context.

---

### Story 1.2: Add monotonic `totalBytesWritten` to `CircularBuffer`

**Goal**: Provide a stable, never-decreasing `session_seq` counter for correlating Stage 1 vs Stage 2 observations. **CRITICAL PITFALL (4.1)**: the existing `count` field only tracks current buffer occupancy, not total bytes ever written, so it wraps and collides.

#### E1-S2-T1 — Add `totalBytesWritten int64` to `CircularBuffer`
- **File**: `session/circular_buffer.go`
- **Change**: Add `totalBytesWritten int64` field (atomic or mutex-protected). Increment by `len(data)` on every `Write()` or `Append()` call, regardless of whether data overwrites old bytes. Expose via `TotalBytesWritten() int64` method.
- **Complexity**: S
- **Notes**: This counter MUST NOT be decremented on circular wrap. Test with a buffer that wraps multiple times and verify monotonic increase.

#### E1-S2-T2 — Thread `session_seq` into `EscapeCodeParser.Parse`
- **File**: `pkg/analytics/escape_code_parser.go`
- **Change**: Change `Parse(data []byte)` signature to `Parse(data []byte, sessionSeq int64)`. Each `EscapeEventRecord` emitted from this parse call uses the provided `sessionSeq` as its `SessionSeq` value. All callers must be updated.
- **Complexity**: S
- **Notes**: The value passed is `circularBuffer.TotalBytesWritten()` captured **before** appending `data`, so `sessionSeq` represents the byte offset at the start of this chunk.

---

### Story 1.3: OSC payload redaction

**Goal**: Prevent PII (clipboard contents, window titles, file paths) from being stored in escape event records. Addresses security pitfalls 5.2 and 5.3.

#### E1-S3-T1 — Add OSC redaction logic in `EscapeCodeParser`
- **File**: `pkg/analytics/escape_code_parser.go`
- **Change**: Add `redactOSCPayloads bool` field (set from config). When `redactOSCPayloads=true`:
  - For OSC 52 (clipboard): record `sequence_type="OSC"`, `sequence_subtype="clipboard"`, `byte_length=N`, but set `raw_bytes=nil` and `payload_hash=""` regardless of capture level.
  - For OSC 0, 1, 2 (window title), OSC 7 (CWD): record type/subtype/byte_length but do not compute hash of payload content; set `payload_hash=""`.
  - All other OSC types: apply normal capture level logic.
- **Complexity**: S

#### E1-S3-T2 — Add tests for OSC redaction
- **File**: `pkg/analytics/escape_code_parser_test.go` (extend existing)
- **Contents**: Table-driven tests verifying that OSC 52, OSC 0, OSC 7 payloads are redacted when `redactOSCPayloads=true`; and that payload hashes are present when `redactOSCPayloads=false`.
- **Complexity**: S

---

### Story 1.4: Parser hardening (pitfall mitigations)

**Goal**: Address correctness pitfalls identified in research/pitfalls.md before analytics data flows into SQLite, to prevent garbage data from poisoning analysis.

#### E1-S4-T1 — Cap `partialBuffer` at 4 KB
- **File**: `pkg/analytics/escape_code_parser.go`
- **Change**: In `findPartialEscapeAtEnd` and any place that appends to `partialBuffer`, add a guard: if `len(partialBuffer) > 4096`, log a warning and reset to nil. Add a `resetPartial()` method called from `Parse` at session-open (or expose a `Reset()` method called from `NewResponseStream`).
- **Complexity**: S
- **Notes**: Pitfall 1.1.

#### E1-S4-T2 — Add OSC payload length cap
- **File**: `pkg/analytics/escape_code_parser.go`
- **Change**: In `parseOSC`, add a maximum scan length of 65536 bytes. If the scan exceeds this limit without finding BEL or `ESC \`, record the sequence as truncated (`mangle_type="truncated"`) and advance past the cap.
- **Complexity**: S
- **Notes**: Pitfall 1.3. A 50 KB clipboard OSC at `capture_level=full` would otherwise be stored verbatim in SQLite.

#### E1-S4-T3 — Add tmux DCS passthrough detection
- **File**: `pkg/analytics/escape_code_parser.go`
- **Change**: In `parseStringSequence`, detect the `ESC P tmux ;` prefix. When found, set `sequence_subtype="tmux-passthrough"`. Do not attempt to unwrap the inner sequence in Phase 1; document this as a known limitation. This prevents false-positive mangle detection for tmux passthrough sequences (pitfall 1.4).
- **Complexity**: S

---

## Epic 2: New `escape_event` ent schema entity + SQLite batch writer

**Goal**: Persist escape event observations to SQLite using the ent ORM, with a high-throughput batch writer that never blocks the PTY hot path.

---

### Story 2.1: `escape_event` ent schema entity

#### E2-S1-T1 — Define `EscapeEvent` ent schema
- **File**: `session/ent/schema/escape_event.go` (new file)
- **Contents** (based on architecture.md Finding 4):
  ```go
  field.String("id").Unique().NotEmpty().Immutable()
  field.String("session_id").NotEmpty()
  field.String("stage").NotEmpty()
  field.String("sequence_type").NotEmpty()
  field.String("sequence_subtype").Optional()
  field.Int("byte_length")
  field.String("payload_hash").Optional()
  field.Bytes("raw_bytes").Optional()
  field.Bool("mangled").Default(false)
  field.String("mangle_type").Optional()
  field.Time("wall_time").Immutable()
  field.Int64("session_seq")
  ```
  Indexes: `(session_id)`, `(session_id, stage)`, `(session_id, session_seq)`, `(wall_time)`, `(mangled)`, `(sequence_type)`
- **Complexity**: S
- **Notes**: Use `INTEGER` PK to minimize index overhead per pitfall 3.3. The composite `(session_id, session_seq)` index is critical for mangle correlation lookups.

#### E2-S1-T2 — Run ent code generation
- **Command**: `go run -mod=mod entgo.io/ent/cmd/ent generate --feature sql/upsert ./session/ent/schema` (MUST use `--feature sql/upsert` per CLAUDE.md)
- **Files modified**: All `session/ent/` generated files for `EscapeEvent` entity
- **Complexity**: S
- **Notes**: The existing `OpenAnalyticsDB` in `server/analytics/db.go` calls `client.Schema.Create(ctx)` on startup, so the new table is auto-migrated.

---

### Story 2.2: Batch writer goroutine

#### E2-S2-T1 — Implement `EscapeEventBatchWriter` struct
- **File**: `server/analytics/escape_event_batch_writer.go` (new file)
- **Contents**:
  - `EscapeEventBatchWriter` struct with: `ch chan EscapeEventRecord` (buffered, cap 1000), `client *ent.Client`, per-session row counters `map[string]int`, `maxRowsPerSession int`
  - `Start(ctx context.Context)` goroutine: ticker at 500ms, flush at 100 rows or ticker, drain on ctx.Done()
  - `Write(record EscapeEventRecord)` — **non-blocking send**: `select { case w.ch <- record: default: atomic.AddInt64(&w.dropped, 1) }`. Never blocks. Drop with counter.
  - `flush(ctx, batch []EscapeEventRecord)` using `client.EscapeEvent.CreateBulk(...).Exec(ctx)`
  - Per-session row cap enforced in `Write()` using in-memory counter (pitfall 3.1)
  - `Implements EscapeEventWriter interface`
- **Complexity**: M

#### E2-S2-T2 — Set `_wal_autocheckpoint=0` on analytics DB connection
- **File**: `server/analytics/db.go`
- **Change**: Add `_wal_autocheckpoint=0` to the SQLite DSN when opening the analytics DB. Add a `CheckpointPassive(ctx context.Context)` helper that runs `PRAGMA wal_checkpoint(PASSIVE)`, to be called at session close.
- **Complexity**: S
- **Notes**: Pitfall 2.3. Prevents mid-burst checkpoint stalls.

#### E2-S2-T3 — Add retention hook for `escape_event` table
- **File**: `server/analytics/retention.go` (extend existing)
- **Change**: Add a case to the retention loop that deletes `escape_event` rows older than the configured retention window (or a separate `EscapeAnalyticsRetentionDays` config field defaulting to 7 days).
- **Complexity**: S
- **Notes**: Pitfall 3.4. Without retention, high-capture sessions accumulate unbounded data.

---

### Story 2.3: Batch writer tests

#### E2-S3-T1 — Unit tests for `EscapeEventBatchWriter`
- **File**: `server/analytics/escape_event_batch_writer_test.go` (new file)
- **Contents**:
  - Test: records are batched and flushed at 100 rows
  - Test: records are flushed by ticker at <100 rows
  - Test: `Write()` drops (no block) when channel is full
  - Test: per-session row cap stops accepting new records after `maxRowsPerSession`
  - Test: `Start()` goroutine drains remaining batch on ctx cancellation
- **Complexity**: M

---

## Epic 3: Config fields + zero-overhead noop path

**Goal**: Add the three config fields from FR-3 and wire the noop path so that `capture_level=off` adds zero overhead to the PTY read loop.

---

### Story 3.1: Config struct additions

#### E3-S1-T1 — Add escape analytics config fields to `Config` struct
- **File**: `config/config.go`
- **Change**: Add to the `Config` struct (JSON-tagged for `config.json` persistence):
  ```go
  EscapeAnalyticsCaptureLevel      string  `json:"escapeAnalyticsCaptureLevel,omitempty"`      // "full"|"summary"|"off", default "summary"
  EscapeAnalyticsSamplingRate      float64 `json:"escapeAnalyticsSamplingRate,omitempty"`      // 0.0–1.0, default 1.0
  EscapeAnalyticsMaxRowsPerSession int     `json:"escapeAnalyticsMaxRowsPerSession,omitempty"` // default 10000
  EscapeAnalyticsRedactOSCPayloads bool    `json:"escapeAnalyticsRedactOSCPayloads,omitempty"` // default true
  EscapeAnalyticsRetentionDays     int     `json:"escapeAnalyticsRetentionDays,omitempty"`     // default 7
  ```
- **Complexity**: S

#### E3-S1-T2 — Add config defaults and validation
- **File**: `config/config.go`
- **Change**: In the config loading/default-setting code, add defaults:
  - `EscapeAnalyticsCaptureLevel = "summary"` if empty
  - `EscapeAnalyticsSamplingRate = 1.0` if zero
  - `EscapeAnalyticsMaxRowsPerSession = 10000` if zero
  - `EscapeAnalyticsRedactOSCPayloads = true` (bool default)
  - `EscapeAnalyticsRetentionDays = 7` if zero
  - Validation: reject `CaptureLevel` values outside `{"full","summary","off"}`; reject `SamplingRate` outside `[0.0, 1.0]`
- **Complexity**: S

---

### Story 3.2: Zero-overhead noop path

#### E3-S2-T1 — Wire noop `EscapeEventWriter` when `CaptureLevel = "off"`
- **File**: `server/server.go` or the analytics initialization code
- **Change**: In server startup, when `cfg.EscapeAnalyticsCaptureLevel == "off"`, inject `analytics.NoopEscapeEventWriter{}` as the writer for all `EscapeCodeParser` instances. No `EscapeEventBatchWriter` goroutine is started. The bool guard `if !p.enabled` in `EscapeCodeParser.Parse()` short-circuits before any writer is called.
- **Complexity**: S
- **Notes**: Acceptance criterion AC-6: zero rows written at `capture_level=off`.

#### E3-S2-T2 — Deterministic sampling logic
- **File**: `pkg/analytics/escape_code_parser.go`
- **Change**: Add sampling check using deterministic hash: `hash(sessionID + "|" + strconv.FormatInt(chunkSeqNumber, 10)) % 1000 < uint64(samplingRate * 1000)`. Store `chunkSeqNumber int64` on the parser (incremented per `Parse` call). This ensures Stage 1 and Stage 2 make identical sampling decisions for the same chunk (pitfall 4.4).
- **Complexity**: S

---

## Epic 4: Stage 1 and Stage 2 instrumentation hook-up

**Goal**: Wire the `EscapeEventWriter` into the actual data flow: Stage 1 in `session/response_stream.go` and Stage 2 in `server/services/connectrpc_websocket.go`.

---

### Story 4.1: Stage 1 — `ResponseStream` wiring

#### E4-S1-T1 — Update `NewResponseStream` to accept `EscapeEventWriter`
- **File**: `session/response_stream.go`
- **Change**: Add `writer analytics.EscapeEventWriter` parameter to `NewResponseStream` (or add `SetEventWriter` setter if function signature changes are disruptive). Pass `sessionID` (stable UUID, not tmux session name — use `instance.GetStableID()` before calling `NewResponseStream`).
- **Complexity**: S

#### E4-S1-T2 — Thread `totalBytesWritten` into Stage 1 parse call
- **File**: `session/response_stream.go`
- **Change**: At the existing parse hook (line ~217), change `rs.escapeParser.Parse(chunk.Data)` to `rs.escapeParser.Parse(chunk.Data, rs.ptyAccess.buffer.TotalBytesWritten())`. The `TotalBytesWritten()` value is captured **before** the `CircularBuffer.Write()` call that appends this chunk — this represents the byte offset at the start of the chunk.
- **Complexity**: S

#### E4-S1-T3 — Wire `EscapeCodeParser` fields from config
- **File**: `session/response_stream.go` or session construction site
- **Change**: When constructing `EscapeCodeParser`, pass `cfg.EscapeAnalyticsCaptureLevel`, `cfg.EscapeAnalyticsSamplingRate`, and `cfg.EscapeAnalyticsRedactOSCPayloads` into the parser fields. Inject the `EscapeEventBatchWriter` (or noop) based on `CaptureLevel == "off"`.
- **Complexity**: S

---

### Story 4.2: Stage 2 — `connectrpc_websocket.go` tap

#### E4-S2-T1 — Add Stage 2 observer tap in `streamViaControlMode`
- **File**: `server/services/connectrpc_websocket.go`
- **Change**: In `streamViaControlMode`, after the coalescing loop assembles `buf` and **before** `sendData(buf)` is called, invoke a Stage 2 observer. The observer calls `escapeParser.ParseStage2(buf, sessionID)` which parses the buffer with a lightweight second pass and calls `MangleCorrelator.CheckStage2(...)` for each parsed sequence.
- **Complexity**: M
- **Notes**: Architecture research Finding 2 / Open Concern 2: coalescing means `buf` may span multiple Stage 1 `session_seq` values. The Stage 2 parser runs a fresh parse pass on `buf` to identify individual sequences, then each sequence's byte position within `buf` plus the cumulative Stage 2 byte offset gives a `session_seq` range for lookup.

#### E4-S2-T2 — Stage 2 cumulative byte counter
- **File**: `server/services/connectrpc_websocket.go`
- **Change**: Add a per-stream `stage2BytesWritten int64` counter in `streamViaControlMode`. Increment by `len(buf)` after each successful `sendData`. Use `stage2BytesWritten` (before increment) as the base offset for correlating Stage 2 sequences with Stage 1 records.
- **Complexity**: S

#### E4-S2-T3 — Session close log summary (NFR-4)
- **File**: `session/response_stream.go` or `streamViaControlMode` cleanup path
- **Change**: On session stream close, log: `"escape analytics: session %s closed: total_sequences=%d mangled=%d dropped=%d bytes_overhead=%d"`. Values come from counters maintained in `EscapeCodeParser` and `EscapeEventBatchWriter`.
- **Complexity**: S

---

## Epic 5: Mangle detection (`MangleCorrelator` with TTL)

**Goal**: Implement the in-memory bounded correlation map that bridges Stage 1 and Stage 2 observations to detect mangled sequences.

---

### Story 5.1: `MangleCorrelator` implementation

#### E5-S1-T1 — Implement `MangleCorrelator` struct
- **File**: `pkg/analytics/mangle_correlator.go` (new file)
- **Contents**:
  - `Stage1Observation struct { PayloadHash string; ByteLen int; WallTime time.Time }`
  - `MangleCorrelator struct { mu sync.Mutex; pending map[string]Stage1Observation; maxAge time.Duration; maxSize int }`
  - `RecordStage1(sessionID string, sessionSeq int64, hash string, byteLen int)` — inserts into pending map with key `sessionID+":"+seqStr`. Drops oldest entry when `len(pending) >= maxSize` (bounded growth).
  - `CheckStage2(sessionID string, sessionSeq int64, hash string, byteLen int) (mangled bool, mangleType string)` — looks up by key; if found and hash matches: evict, return `(false, "")`. If found and hash differs: evict, return `(true, "mutated")` or `(true, "truncated")` based on byte length comparison.
  - If not found: return `(false, "")` — absence at Stage 2 lookup time is not necessarily a mangle (may be in a different batch); the eviction pass handles "stripped" detection.
  - `EvictExpired(writer EscapeEventWriter)` — called periodically; evicts entries older than `maxAge` (5s default), writing them as `mangled=true, mangle_type="stripped"` escape events.
- **Complexity**: M

#### E5-S1-T2 — Eviction goroutine
- **File**: `pkg/analytics/mangle_correlator.go`
- **Change**: Add `StartEviction(ctx context.Context, writer EscapeEventWriter)` method that runs a ticker at `maxAge/2` interval and calls `EvictExpired(writer)`. Tied to the context for clean shutdown.
- **Complexity**: S

#### E5-S1-T3 — Integrate `MangleCorrelator` into Stage 1 and Stage 2 paths
- **Files**: `pkg/analytics/escape_code_parser.go`, `server/services/connectrpc_websocket.go`
- **Change**: Stage 1 (`EscapeCodeParser.Parse`): after parsing each sequence, call `correlator.RecordStage1(sessionID, sessionSeq+seqStartOffset, hash, byteLen)`. Stage 2 (Epic 4 tap): call `correlator.CheckStage2(...)` and set `Mangled`/`MangleType` on the `EscapeEventRecord` before passing to the writer.
- **Complexity**: S

---

### Story 5.2: `MangleCorrelator` tests

#### E5-S2-T1 — Unit tests for mangle detection
- **File**: `pkg/analytics/mangle_correlator_test.go` (new file)
- **Contents**:
  - Test: hash match → `mangled=false`
  - Test: byte-length difference → `mangled=true, mangle_type="truncated"`
  - Test: same length, different hash → `mangled=true, mangle_type="mutated"`
  - Test: Stage 1 recorded, Stage 2 never arrives → eviction emits `mangle_type="stripped"` after TTL
  - Test: bounded size — inserting beyond `maxSize` drops oldest entry
  - Integration test: feed a known-stripped OSC sequence through the Stage 1+2 flow and verify `mangled=true` row in SQLite (AC-3)
- **Complexity**: M

---

## Epic 6: ConnectRPC `QueryEscapeAnalytics` + `GetEscapeAnalyticsSummary` RPCs

**Goal**: Expose the `escape_event` data via two ConnectRPC endpoints, enabling the web UI and external tooling to query and aggregate escape analytics.

---

### Story 6.1: Proto definitions

#### E6-S1-T1 — Add `EscapeEvent` proto message and enums
- **File**: `proto/session/v1/analytics.proto` (new file, or extend `session.proto`)
- **Contents**:
  ```protobuf
  message EscapeEvent {
    string id = 1;
    string session_id = 2;
    string stage = 3;
    string sequence_type = 4;
    string sequence_subtype = 5;
    int32 byte_length = 6;
    string payload_hash = 7;
    bytes raw_bytes = 8;
    bool mangled = 9;
    string mangle_type = 10;
    google.protobuf.Timestamp wall_time = 11;
    int64 session_seq = 12;
  }
  
  message QueryEscapeAnalyticsRequest {
    string session_id = 1;             // required
    string stage = 2;                  // optional filter
    string sequence_type = 3;          // optional filter
    bool mangled_only = 4;             // optional filter
    google.protobuf.Timestamp start_time = 5;
    google.protobuf.Timestamp end_time = 6;
    int32 page_size = 7;               // default 100, max 1000
    string page_token = 8;             // cursor-based pagination
  }
  
  message QueryEscapeAnalyticsResponse {
    repeated EscapeEvent events = 1;
    string next_page_token = 2;
    int32 total_count = 3;
  }
  
  message GetEscapeAnalyticsSummaryRequest {
    string session_id = 1;
    google.protobuf.Timestamp start_time = 2;
    google.protobuf.Timestamp end_time = 3;
  }
  
  message EscapeSequenceCount {
    string sequence_type = 1;
    int64 count = 2;
    int64 mangled_count = 3;
  }
  
  message GetEscapeAnalyticsSummaryResponse {
    repeated EscapeSequenceCount histogram = 1;
    int64 total_sequences = 2;
    int64 total_mangled = 3;
    double mangle_rate = 4;
  }
  ```
- **Complexity**: S

#### E6-S1-T2 — Add RPCs to `SessionService` proto definition
- **File**: `proto/session/v1/session.proto`
- **Change**: Add to `SessionService`:
  ```protobuf
  rpc QueryEscapeAnalytics(QueryEscapeAnalyticsRequest) returns (QueryEscapeAnalyticsResponse);
  rpc GetEscapeAnalyticsSummary(GetEscapeAnalyticsSummaryRequest) returns (GetEscapeAnalyticsSummaryResponse);
  ```
- **Complexity**: S

#### E6-S1-T3 — Run proto generation
- **Command**: `make generate-proto`
- **Files modified**: `session/gen/session/v1/*.go`, `web-app/src/gen/session/v1/*_pb.ts`
- **Complexity**: S

---

### Story 6.2: Go handler implementation

#### E6-S2-T1 — Implement `QueryEscapeAnalytics` handler
- **File**: `server/services/analytics_service.go` (new file, or add to `session_service.go`)
- **Contents**:
  - Query `escape_event` via ent: filter by `session_id` (required), optionally `stage`, `sequence_type`, `mangled`, and time range.
  - Cursor-based pagination using `session_seq` as the cursor (stable, monotonically increasing).
  - Map ent `EscapeEvent` rows to proto `EscapeEvent` messages.
  - `raw_bytes` field: returned as-is from DB; the UI is responsible for rendering as hex/base64 (security pitfall 5.1).
  - Add `// +api: escape:query` marker per feature registry rules.
- **Complexity**: M

#### E6-S2-T2 — Implement `GetEscapeAnalyticsSummary` handler
- **File**: `server/services/analytics_service.go`
- **Contents**:
  - Aggregate query: `SELECT sequence_type, COUNT(*), SUM(CASE WHEN mangled THEN 1 ELSE 0 END) FROM escape_event WHERE session_id=? AND wall_time BETWEEN ? AND ? GROUP BY sequence_type`
  - Use ent's `GroupBy` + `Count` + `Aggregate` to build the query without string interpolation (security pitfall 5.4).
  - Calculate `mangle_rate = total_mangled / total_sequences`.
  - Add `// +api: escape:summary` marker.
- **Complexity**: M

#### E6-S2-T3 — Register handlers in server
- **File**: `server/server.go`
- **Change**: Register the new analytics service routes with ConnectRPC router.
- **Complexity**: S

---

### Story 6.3: Handler tests

#### E6-S3-T1 — Go tests for `QueryEscapeAnalytics`
- **File**: `server/services/analytics_service_test.go` (new file)
- **Contents**:
  - Test: returns paginated results filtered by `session_id` (AC-4)
  - Test: `mangled_only=true` filter
  - Test: time range filter
  - Test: empty result set for unknown session
  - Test: pagination cursor advances correctly across pages
- **Complexity**: M

#### E6-S3-T2 — Go tests for `GetEscapeAnalyticsSummary`
- **File**: `server/services/analytics_service_test.go`
- **Contents**:
  - Test: histogram correctly counts sequence types
  - Test: mangle_rate calculated correctly
  - Test: returns zero values for session with no events
- **Complexity**: S

---

## Epic 7: Web UI — Escape analytics page

**Goal**: Build the React analytics page per FR-6, using existing component patterns, ConnectRPC hooks, and vanilla-extract CSS.

---

### Story 7.1: Data access hook

#### E7-S1-T1 — Add `useEscapeAnalytics` hook
- **File**: `web-app/src/lib/hooks/useEscapeAnalytics.ts` (new file)
- **Contents**:
  - `useEscapeAnalyticsSummary(sessionId: string)` — calls `GetEscapeAnalyticsSummary` via ConnectRPC client; returns `{ histogram, totalSequences, totalMangled, mangleRate, loading, error }`.
  - `useEscapeEvents(params: QueryEscapeAnalyticsRequest)` — calls `QueryEscapeAnalytics`; returns `{ events, nextPageToken, loading, error, fetchNextPage }`.
  - Both hooks use existing `useConnectRpcClient` pattern.
- **Complexity**: S
- **Notes**: Add `// +feature: escape-analytics` marker in first 10 lines per feature registry rules.

---

### Story 7.2: Analytics page components

#### E7-S2-T1 — `EscapeAnalyticsPage` container component
- **File**: `web-app/src/components/analytics/EscapeAnalyticsPage.tsx` (new file)
- **Contents**:
  - Session selector dropdown (reuse existing `SessionSelector` component or build inline)
  - Renders `SequenceHistogram`, `MangleRateIndicator`, `EscapeEventTable` as children
  - URL path: `/analytics/escape` or as a tab in the existing analytics section
  - Add `// +feature: escape-analytics` marker
- **Complexity**: S

#### E7-S2-T2 — `SequenceHistogram` bar chart component
- **File**: `web-app/src/components/analytics/SequenceHistogram.tsx` (new file)
- **Contents**:
  - Bar chart showing sequence type counts from `GetEscapeAnalyticsSummary` response histogram
  - Horizontal bars with sequence type label on left, count on right
  - Highlight mangled counts in a contrasting color
  - Uses vanilla-extract CSS (`.css.ts` colocated file)
  - No external chart library — implement with CSS flex bars to avoid new dependencies
- **Complexity**: M
- **Notes**: Acceptance criterion AC-5.

#### E7-S2-T3 — `MangleRateIndicator` component
- **File**: `web-app/src/components/analytics/MangleRateIndicator.tsx` (new file)
- **Contents**:
  - Displays mangle rate percentage with color coding: green < 1%, yellow 1–5%, red > 5%
  - Shows absolute counts: `N mangled of M total sequences`
  - Vanilla-extract CSS
- **Complexity**: S

#### E7-S2-T4 — `EscapeEventTable` filterable table component
- **File**: `web-app/src/components/analytics/EscapeEventTable.tsx` (new file)
- **Contents**:
  - Columns: wall_time, stage, sequence_type, sequence_subtype, byte_length, mangled, mangle_type
  - Filter controls: stage select, sequence_type text input, mangled-only checkbox, time range pickers
  - Pagination with "Load more" button using `fetchNextPage` from `useEscapeEvents`
  - `raw_bytes` column: rendered as hex string via `Array.from(bytes).map(b => b.toString(16).padStart(2, '0')).join(' ')` — NEVER passed to a terminal renderer (security pitfall 5.1)
  - Vanilla-extract CSS
- **Complexity**: M

#### E7-S2-T5 — Vanilla-extract styles for all analytics components
- **Files**: 
  - `web-app/src/components/analytics/EscapeAnalyticsPage.css.ts`
  - `web-app/src/components/analytics/SequenceHistogram.css.ts`
  - `web-app/src/components/analytics/MangleRateIndicator.css.ts`
  - `web-app/src/components/analytics/EscapeEventTable.css.ts`
- **Contents**: Style definitions using `vars` from `../../styles/theme.css`. No hardcoded hex colors. No `.module.css` files per CSS architecture rules (ADR-009).
- **Complexity**: S

---

### Story 7.3: Route registration and navigation

#### E7-S3-T1 — Register `/analytics/escape` route
- **File**: `web-app/src/app/` routing configuration (React Router or Next.js routing)
- **Change**: Add route entry for `EscapeAnalyticsPage`. Add navigation link in the analytics section of the sidebar/nav.
- **Complexity**: S

---

### Story 7.4: Feature registry updates

#### E7-S4-T1 — Update feature registry for new RPCs and UI
- **Files**: `docs/registry/features/escape-analytics.json` (new file per registry rules)
- **Contents**:
  ```json
  {
    "id": "escape:query",
    "type": "backend",
    "rpc": "QueryEscapeAnalytics",
    "markerFound": true,
    "tested": false,
    "testIds": [],
    "lastModified": "2026-05-14T00:00:00Z"
  }
  ```
  Plus entries for `escape:summary` and the `escape-analytics` frontend feature.
- **Complexity**: S
- **Notes**: Set `tested: true` and populate `testIds` once e2e tests are written (E7-S5-T1).

---

### Story 7.5: Tests

#### E7-S5-T1 — Jest unit tests for analytics components
- **File**: `web-app/src/components/analytics/EscapeAnalyticsPage.test.tsx` (new file)
- **Contents**:
  - `SequenceHistogram` renders bar for each sequence type in mock histogram data
  - `MangleRateIndicator` shows correct color tier for mock mangle rates
  - `EscapeEventTable` renders event rows; filter controls update visible rows; raw_bytes rendered as hex
  - Mocked ConnectRPC client via Jest mock
- **Complexity**: M

#### E7-S5-T2 — E2E Playwright test for escape analytics page
- **File**: `tests/e2e/escape-analytics.spec.ts` (new file)
- **Contents**:
  ```ts
  // @feature escape:query, escape:summary, escape-analytics
  ```
  - Navigates to `/analytics/escape`
  - Selects a session that has escape events
  - Verifies histogram renders (at least one bar visible)
  - Verifies event table shows rows
  - Verifies `mangled_only` filter reduces visible rows
- **Complexity**: M
- **Notes**: Acceptance criterion AC-5. Tests run against `http://localhost:8544`.

---

## Critical Pitfalls Cross-Reference

The following pitfalls from `research/pitfalls.md` are addressed by specific tasks:

| Pitfall | Severity | Addressed By |
|---|---|---|
| Unbounded `partialBuffer` growth | High | E1-S4-T1 |
| OSC payload uncapped (50 KB blobs) | High | E1-S4-T2 |
| tmux DCS passthrough = false mangle | Medium | E1-S4-T3 |
| No monotonic `session_seq` counter in `CircularBuffer` | **Critical** | E1-S2-T1 |
| Hot-path lock contention → `select { default: drop }` | **Critical** | E2-S2-T1 |
| Channel backpressure stalls PTY reader | High | E2-S2-T1 |
| WAL auto-checkpoint mid-burst stall | Medium | E2-S2-T2 |
| Per-session row cap via in-memory counter | Medium | E2-S2-T1 |
| No retention policy for `escape_event` | Medium | E2-S2-T3 |
| Stage 1/Stage 2 batch window race → false negative mangle | High | E5-S1-T1 (in-memory correlator bypasses this) |
| Sampling decision inconsistent across stages | Medium | E3-S2-T2 |
| OSC 52 clipboard PII at `full` level | **Security High** | E1-S3-T1 + E1-S3-T2 |
| `raw_bytes` rendered as terminal output in UI | **Security High** | E7-S2-T4 |
| OSC 0/2/7 title PII at `summary` level | Security Medium | E1-S3-T1 |

---

## Technology Decisions Requiring ADRs

### ADR-TBA-1: Database file strategy for `escape_event`

**Question**: Should `escape_event` use the same `analytics.db` file or a separate `escape_events.db`?

**Tension**: 
- Same file: simpler wiring; existing `OpenAnalyticsDB` + `Schema.Create` migration is additive.
- Separate file: isolates high-volume escape writes from lower-volume analytics events; avoids WAL pressure cross-contamination; allows independent retention/deletion.

**Current plan recommendation**: Same `analytics.db` (architecture.md Finding 4), with `_wal_autocheckpoint=0` to control checkpoint timing (E2-S2-T2). Revisit if benchmarks show WAL contention.

**ADR needed**: Yes — this affects the `OpenAnalyticsDB` wiring and the retention strategy.

---

### ADR-TBA-2: `session_seq` definition and `CircularBuffer` monotonic counter

**Question**: What is the canonical definition of `session_seq`? Specifically: is it the cumulative PTY byte offset (requires `totalBytesWritten` on `CircularBuffer`) or a per-parse-call sequence number?

**Tension**:
- Cumulative byte offset: stable across buffer wraps; correlates directly with what Stage 2 receives; requires modifying `CircularBuffer`.
- Per-call sequence number: simpler to add; doesn't require `CircularBuffer` changes; but may be less meaningful for cross-stage correlation since Stage 2 works with coalesced frames not individual reads.

**Current plan recommendation**: `totalBytesWritten int64` on `CircularBuffer` (E1-S2-T1) to get a stable monotonic offset that means the same thing at both stages.

**ADR needed**: Yes — this is a data model decision that affects the correlation query and the `escape_event` schema.

---

### ADR-TBA-3: Mangle detection architecture — inline vs. deferred reconciliation

**Question**: Should mangle detection run inline at Stage 2 time (using the in-memory `MangleCorrelator`) or as a deferred reconciliation pass after session close?

**Tension**:
- Inline `MangleCorrelator`: immediate results; more complex; susceptible to Stage 1 observations not yet flushed to DB when Stage 2 arrives (pitfall 4.2 — mitigated by keeping Stage 1 records in memory, not DB).
- Deferred reconciliation at session close: simpler; immune to batch window races; but no live mangle data during session.

**Current plan recommendation**: In-memory `MangleCorrelator` (Epic 5) — avoids the cross-batch SQLite race by never relying on DB round-trips for correlation. The `maxAge` eviction handles the "Stage 2 never arrived" case.

**ADR needed**: Yes — this affects the architecture of Epic 5.

---

### ADR-TBA-4: Stage 2 coalescing — set-intersection vs. range tagging

**Question**: How to correlate Stage 2 (coalesced buffer) observations back to Stage 1 (per-PTY-read) `session_seq` values? The coalescing loop merges multiple Stage 1 chunks into one `buf`.

**Options**:
1. **Set-intersection parse**: Run a second `EscapeCodeParser` pass on `buf` at Stage 2; each sequence's byte position within `buf` plus `stage2BytesWritten` base gives a stable offset to look up in `MangleCorrelator`. This requires the two parsers to produce identical sequence boundaries for the same bytes.
2. **Range tagging**: At Stage 1, tag each `updateChan` frame with its `sessionSeq` range; Stage 2 reads these tags from the channel payload (requires modifying the channel type from `[]byte` to a tagged struct).

**Current plan recommendation**: Set-intersection parse (E4-S2-T1) — no channel type changes required; more self-contained.

**ADR needed**: Yes — option 2 requires modifying `SubscribeControlModeUpdates` channel semantics.

---

## Summary

| Dimension | Count |
|---|---|
| Epics | 7 |
| Stories | 21 |
| Tasks | 43 |
| ADRs needed | 4 |

### Task complexity breakdown
- S (Small, < 2h): 26 tasks
- M (Medium, 2–8h): 14 tasks
- L (Large, > 8h): 3 tasks (E5-S2-T1 integration test, E7-S5-T2 e2e test, E4-S2-T1 Stage 2 coalescing tap)

### Acceptance Criteria coverage

| AC | Covered By |
|---|---|
| AC-1: Parser identifies all CSI/OSC/DCS from test corpus | E1-S4-T1/T2/T3 + existing parser tests |
| AC-2: `escape_event` rows appear in SQLite after vim session | Epic 4 wiring |
| AC-3: Mangle detection flags stripped OSC in integration test | E5-S2-T1 |
| AC-4: `QueryEscapeAnalytics` returns filtered paginated results | E6-S3-T1 |
| AC-5: Web UI histogram renders for test session | E7-S5-T1, E7-S5-T2 |
| AC-6: `capture_level=off` → zero rows written | E3-S2-T1 |
| AC-7: Stage 1 overhead < 50µs on 4KB chunks | E2-S2-T1 (non-blocking channel) |
| AC-8: Parser handles split sequences | E1-S4-T1 (cap guards existing partialBuffer logic) |

### Implementation order (suggested)

Phase 1 (foundation): Epic 1 → Epic 2 → Epic 3  
Phase 2 (wiring): Epic 4 → Epic 5  
Phase 3 (API + UI): Epic 6 → Epic 7  

Each phase can proceed after the previous phase's Go code is merged and `make ci` passes.
