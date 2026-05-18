# Terminal Escape Code Analytics — Requirements

## Problem Statement

Terminal escape sequences emitted by programs running in stapler-squad sessions are
suspected to be mangled (truncated, stripped, or corrupted) at one or more points in the
pipeline before reaching the browser's xterm.js renderer. When this happens, terminal UI
breaks in subtle ways (wrong colors, broken cursor positioning, alternate-screen failures,
etc.) and there is no visibility into where or why.

The existing `stripANSI` function in `server/mcp/ansi.go` intentionally strips sequences
for MCP tool output, but the concern is that sequences may also be altered on the
**display path** (PTY → ConnectRPC → browser) in unintended ways. There is no current
mechanism to detect or measure this.

The project also has an existing `pkg/analytics/escape_code_descriptions.go` with semantic
metadata for escape sequences, an `analytics_event` ent schema entity, and a
`sqlite_provider.go` analytics infrastructure — all of which can be leveraged.

---

## Goals

1. **Instrument the full escape sequence pipeline** — capture sequence observations at each
   stage: raw PTY read, Go processing/transport layer, and (via browser reporting) xterm.js
   ingestion.

2. **Persist analytics to SQLite via ent ORM** — same pattern as `analytics_event` and
   `approvalrule` entities; queryable from Go and exposed via ConnectRPC for the web UI.

3. **Detect and record mangling** — when the same logical sequence position can be
   compared across stages, flag sequences that were modified, dropped, or truncated.

4. **Web UI view + query API** — a dashboard page showing escape analytics per session,
   aggregate sequence-type histograms, and detected mangle events; plus a raw query
   ConnectRPC endpoint.

5. **Always-on but configurable** — collection is enabled by default; config controls the
   capture level (`full` = raw bytes, `summary` = metadata + hash, `off` = disabled) and
   optional sampling rate (0.0–1.0, default 1.0).

---

## Non-Goals

- Replacing or changing the existing `stripANSI` logic (analytics observes; it does not
  change behavior).
- Storing analytics for MCP tool output (only the display path matters for this feature).
- Browser-side instrumentation in Phase 1 (xterm.js hooks are a stretch goal for Phase 2).

---

## Processing Pipeline (Stages to Instrument)

```
[PTY raw bytes]
       │
       ▼  Stage 1: PTY Reader (session/scrollback or circular_buffer.go)
[CircularBuffer]
       │
       ▼  Stage 2: Go transport serialization (ConnectRPC streaming)
[WebSocket/ConnectRPC frames]
       │
       ▼  Stage 3 (future): xterm.js in browser
[Rendered terminal]
```

Analytics capture points:
- **Stage 1**: Parse escape sequences from raw PTY bytes. Record sequence type, parameters,
  payload hash, byte length, session ID, wall-clock timestamp.
- **Stage 2**: Record what bytes are actually written into the ConnectRPC response stream.
  Compare against Stage 1 observations to detect drops or mutations.
- **Stage 3**: (stretch / Phase 2) Browser JS reports parsed sequences back via a
  lightweight endpoint.

---

## Functional Requirements

### FR-1: Escape Sequence Parser (Go)
- Parse the standard sequence types from raw byte streams:
  - CSI (ESC [ ... final-byte)
  - OSC (ESC ] ... ST or BEL)
  - DCS (ESC P ... ST)
  - SS2/SS3, bare ESC + single char
- Leverage the existing semantic metadata in `pkg/analytics/escape_code_descriptions.go`.
- Exposed as a standalone `EscapeParser` that other components can wrap around any
  `io.Reader`.

### FR-2: Analytics Ent Schema (new entity: `escape_event`)
Fields:
- `session_id` (string, indexed)
- `stage` (enum: `pty_read` | `transport` | `browser`)
- `sequence_type` (string: CSI, OSC, DCS, etc.)
- `sequence_subtype` (string: e.g., SGR, cursor-up, alternate-screen)
- `byte_length` (int)
- `payload_hash` (string, SHA-256 hex prefix, only if capture_level=full or summary)
- `raw_bytes` (bytes, only if capture_level=full)
- `mangled` (bool: true if this sequence differs from an earlier stage observation)
- `mangle_type` (string: truncated | stripped | mutated | empty)
- `wall_time` (time.Time, indexed)
- `session_seq` (int64: byte offset in session scrollback at time of observation)

### FR-3: Config Fields
New fields on `Config` struct:
- `EscapeAnalyticsCaptureLevel` (string): `"full"` | `"summary"` | `"off"`, default `"summary"`
- `EscapeAnalyticsSamplingRate` (float64): 0.0–1.0, default 1.0
- `EscapeAnalyticsMaxRowsPerSession` (int): max rows stored per session, default 10,000

### FR-4: SQLite Writer
- Reuse `server/analytics/sqlite_provider.go` pattern.
- Batch writes (flush every 500ms or 100 rows, whichever comes first) to avoid lock
  contention.
- Background goroutine with graceful shutdown.

### FR-5: ConnectRPC Query Endpoint
New RPC: `QueryEscapeAnalytics`
- Filter by: session_id, stage, sequence_type, mangled=true, time range
- Returns: paginated list of `EscapeEvent` protos
- Aggregate endpoint: `GetEscapeAnalyticsSummary` — returns histogram of sequence_type
  counts and mangle rate per session

### FR-6: Web UI — Analytics Page
- New page: `/analytics/escape` (or tab within existing Analytics section)
- Components:
  - Session selector dropdown
  - Histogram: sequence types (bar chart by frequency)
  - Mangle rate indicator (% of sequences flagged as mangled per session)
  - Filterable event table (stage, type, mangled, time range)
- Built with existing React + vanilla-extract patterns

### FR-7: Mangle Detection
- At Stage 2 capture: look up matching Stage 1 observation by (session_id, session_seq)
- If bytes present in Stage 1 but absent/different in Stage 2 → set `mangled=true`,
  `mangle_type` accordingly
- Tolerance: byte-for-byte comparison (no fuzzy matching)

---

## Non-Functional Requirements

### NFR-1: Performance
- Stage 1 instrumentation: < 50µs overhead per 4KB chunk (parsing only, no DB write on
  hot path)
- Stage 2: async channel write, never blocks the ConnectRPC response path
- Full capture level: expected ~100 bytes/row overhead in SQLite

### NFR-2: Correctness
- Parser must handle split sequences (sequence split across two PTY read() calls)
- Parser must handle nested/interrupted sequences gracefully (treat as unparsed bytes)

### NFR-3: Configurability
- When `EscapeAnalyticsCaptureLevel = "off"`, zero overhead (compile-time noop path via
  interface)
- Sampling applies per-session-chunk, not per-sequence, to preserve sequence integrity

### NFR-4: Observability
- Log a summary line per session at session close: total sequences captured, mangle count,
  byte overhead

---

## Acceptance Criteria

| ID | Criterion |
|----|-----------|
| AC-1 | `EscapeParser` correctly identifies all CSI, OSC, DCS sequences from a known test corpus |
| AC-2 | `escape_event` rows appear in SQLite after a vim session with `capture_level=summary` |
| AC-3 | Mangle detection correctly flags a stripped OSC sequence in an integration test |
| AC-4 | `QueryEscapeAnalytics` RPC returns correct paginated results filtered by session_id |
| AC-5 | Web UI escape analytics page renders sequence histogram for a test session |
| AC-6 | With `capture_level=off`, no `escape_event` rows are written (verified by row count) |
| AC-7 | Stage 1 overhead is < 50µs on 4KB chunks (benchmark) |
| AC-8 | Parser handles sequences split across two read() calls without dropping or duplicating |

---

## Open Questions

1. Should the `escape_event` entity be a separate SQLite database file (like the existing
   analytics DB) or part of the main ent database?
2. For the browser reporting endpoint (Phase 2), should it be a fire-and-forget HTTP POST
   or a ConnectRPC streaming call?
3. What is the expected volume of escape sequences per session? (Needed to validate the
   10,000-row per session cap is reasonable.)
