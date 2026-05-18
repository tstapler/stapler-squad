# Terminal Analytics — Pitfalls & Failure Modes

## 1. Parser Correctness Pitfalls

### 1.1 Split-sequence handling has an unbounded growth risk

The existing `EscapeCodeParser.partialBuffer` correctly prepends unfinished sequences to the next chunk. However, `findPartialEscapeAtEnd` only scans the **last 50 bytes** for a lone ESC. A pathological input — a program that emits `ESC` then waits indefinitely, or a very long DCS/OSC payload — will leave `partialBuffer` growing without bound. Worse, the partial-buffer content is **never aged out**: if the PTY session resets and the matching close never arrives, the stale bytes from the previous "sequence" will be prepended to the first chunk of fresh data, producing a spurious parse hit.

**Mitigation**: cap `partialBuffer` at a maximum size (e.g. 4 KB) and discard with a log warning when exceeded. Add a `resetPartial()` call at session-open time.

### 1.2 8-bit C1 codes are only partially handled

The parser handles `ESC+secondByte` for second bytes in `0x40–0x5F` (the 7-bit equivalents of C1 codes). True **8-bit C1 codes** (`0x80–0x9F`) are present in legacy or badly-configured terminals and arrive as single bytes. For example:
- `0x9B` is the 8-bit CSI introducer (equivalent to `ESC [`)
- `0x9D` is the 8-bit OSC introducer
- `0x9C` is the 8-bit ST terminator

The current `extractEscapeSequences` loop only looks for `data[i] == 0x1b`. Any 8-bit C1 byte is silently passed through as raw data. This means:
- Sequences like `0x9B 1 m` (SGR via 8-bit CSI) are never parsed.
- `parseStringSequence` already handles `0x9C` as a terminator, so a DCS that opens with 7-bit `ESC P` and closes with 8-bit `0x9C` is handled — but a DCS that opens with 8-bit `0x90` is not.

**Mitigation**: add an 8-bit C1 dispatch branch alongside the `0x1b` branch.

### 1.3 OSC parameter overflow is unchecked

`parseOSC` scans forward byte-by-byte until it finds BEL or `ESC \`. A long OSC payload — for example `OSC 52` (clipboard access) with base64-encoded data can be tens of kilobytes — means the parser will scan the entire payload without any length guard before returning the full raw bytes. At `capture_level=full` these bytes are stored in SQLite verbatim.

**Mitigation**: add a hard cap on OSC payload scan (e.g. 64 KB); record the sequence as truncated if exceeded.

### 1.4 DCS passthrough / tmux passthrough sequences

tmux uses `ESC P tmux ; <inner-sequence> ESC \` (DCS with `tmux` as a sub-command) to tunnel sequences through the multiplexer layer. The inner sequence is escaped (`ESC` → `ESC ESC`). The current `parseStringSequence` treats the entire DCS payload as opaque and records it as a flat blob. This means:
- The inner sequence (which may be an SGR, cursor move, etc.) is not parsed.
- The byte counts at Stage 1 and Stage 2 will differ by exactly the tmux framing bytes, which will **always appear as a mangle** unless the Stage 2 comparator understands passthrough unwrapping.

**Mitigation**: detect `ESC P tmux ;` prefix and un-escape the inner payload before recording; add a `mangle_type` value of `passthrough_unwrapped` to distinguish this from true mangling.

### 1.5 Private-use / unknown final bytes

CSI sequences can use final bytes outside the standard `@–Z a–z` range (e.g. kitty terminal's `ESC [ N u` keyboard protocol uses lowercase `u`; the sixel `ESC [ ? PI` uses `q`). The current `parseCSI` scans for final bytes in `0x40–0x5A` and `0x61–0x7A` which covers `A–Z` and `a–z` — this is correct per ECMA-48. However it silently returns `nil, 0` if the final byte is in the intermediate byte range `0x20–0x2F` and the parser already consumed those as intermediate bytes. The case where an intermediate byte appears but no valid final byte follows will cause the parser to return `nil, 0` and skip the `ESC`, then re-encounter `[` on the next iteration and try to parse a broken sequence.

---

## 2. Performance Pitfalls

### 2.1 Lock contention on the hot PTY read path

`EscapeCodeStore.Record` acquires a full write-lock (`sync.Mutex`) on every call, and the eviction path (`evictOldEntries`) does a full scan + sort under the lock. At high throughput (e.g. vim redrawing at 60fps with hundreds of escape sequences per frame), all PTY-goroutines will serialize through this lock. This is the most critical performance risk.

The existing `EscapeCodeStore` is an in-memory aggregation store. For the new SQLite-backed `escape_event` entity the same pattern applies: if `Record` calls are synchronous and per-sequence they will completely dominate the 50 µs/4 KB budget specified by NFR-1.

**Mitigation** (already partly in requirements): the FR-4 batch writer (flush every 500 ms or 100 rows) must be implemented with a **non-blocking channel send** from the hot path. If the channel is full the sequence observation must be dropped (with a counter), not block. Channel capacity of ~1000 rows gives ~10 seconds of headroom at typical rates before dropping.

### 2.2 Channel backpressure / goroutine leak

If the SQLite batch writer goroutine falls behind (e.g. disk I/O spike, WAL checkpoint), the channel will fill. A blocking channel send on the hot PTY path will cause the PTY reader to stall, which propagates backpressure all the way to the PTY `read()` syscall. This will cause the terminal to appear frozen.

In the existing `subscriber.go` pattern the EventBus channel send is also blocking (`ch, _ := bus.Subscribe(ctx)` — the `events.EventBus` implementation must be checked for buffer size). The new escape event writer must explicitly use a buffered channel with a `select { default: drop }` pattern.

**Goroutine leak risk**: the batch writer goroutine must be tied to a `context.Context` that is cancelled on session close or application shutdown. If the goroutine exits on ctx cancellation but the channel is never drained, in-flight rows will be silently lost. The shutdown path must flush the channel before returning.

### 2.3 WAL mode checkpoint contention

`db.go` correctly sets `_journal_mode=WAL`. However WAL mode has a checkpoint mechanism: when the WAL file grows beyond ~1000 pages, SQLite auto-checkpoints during the next write. A checkpoint holds a shared lock and can stall a burst of incoming write batches for tens to hundreds of milliseconds.

With a high-throughput session (vim/tmux startup produces thousands of sequences), the analytics writer may trigger a checkpoint mid-burst. The checkpoint occurs synchronously inside the writer goroutine, blocking the batch flush for the checkpoint duration.

**Mitigation**: set `_wal_autocheckpoint=0` to disable automatic checkpointing and schedule explicit checkpoints (e.g. `PRAGMA wal_checkpoint(PASSIVE)`) during idle periods or session close.

### 2.4 The `partialBuffer` allocation pattern causes per-chunk heap allocation

`Parse` creates a new `[]byte` with `make([]byte, len(p.partialBuffer)+len(data))` and copies both buffers into it on every chunk where a partial sequence exists. In practice almost every chunk that ends mid-sequence will trigger this. The allocation is proportional to chunk size (4 KB typical) and will generate GC pressure.

**Mitigation**: use a pre-allocated ring buffer or `bytes.Buffer` for the partial state.

---

## 3. SQLite Pitfalls

### 3.1 Per-session row cap interaction with batch writes

FR-3 specifies `EscapeAnalyticsMaxRowsPerSession` (default 10,000). Enforcing this cap requires a `SELECT COUNT(*)` or `DELETE ... WHERE` query per session before or during writes. If this check is done synchronously in the batch writer it adds a query round-trip per batch. If done lazily (only at session close) the actual row count can significantly overshoot the cap during a burst.

**Recommended pattern**: maintain an in-memory per-session counter in the batch writer, increment it on every enqueue, and stop enqueuing when the cap is reached. Avoid per-write DB queries on the hot path.

### 3.2 Storage scale: 10K rows × N sessions

At `capture_level=full` with ~100 bytes/row (as stated in NFR-1):
- 10,000 rows/session × 100 bytes = ~1 MB/session
- A user running 100 concurrent sessions = ~100 MB analytics DB

However, at `capture_level=full` the `raw_bytes` blob for a long OSC (e.g. clipboard) can be 10–50 KB per row, completely invalidating the 100-byte estimate. A single paste operation via `OSC 52` with a 50 KB clipboard payload produces one row of 50 KB; 10,000 such rows = 500 MB.

**Mitigation**: apply a hard cap on `raw_bytes` storage per row (e.g. 1 KB, truncated with a flag). At `capture_level=summary` store only the SHA-256 hash and byte_length, never the blob.

### 3.3 SQLite row limits and INTEGER PRIMARY KEY

SQLite's maximum row count per table is `2^63 - 1` (effectively unlimited). However, the ent auto-migration for `escape_event` will use either a `TEXT` or `INTEGER` primary key. If using an auto-increment integer PK, ent will use `INTEGER PRIMARY KEY AUTOINCREMENT`; after `~9.2 × 10^18` inserts the row ID wraps (extremely unlikely). If using a UUID string PK (same pattern as `analytics_event`), each row stores an extra 36 bytes in the index — multiplied by 10K rows × N sessions this is not negligible.

**Recommended**: use `INTEGER PRIMARY KEY` (not `AUTOINCREMENT`) for `escape_event` to minimize index overhead, with a composite index on `(session_id, session_seq)` for the mangle detection query.

### 3.4 Retention / old session accumulation

There is an existing `retention.go` in `server/analytics/` for the `analytics_event` table. The `escape_event` table will need its own retention policy since at full capture level it will grow much faster. Without a retention hook, the analytics DB can grow unboundedly across sessions.

---

## 4. Data Correlation / Mangle Detection Pitfalls

### 4.1 `session_seq` is a byte offset, not a sequence number

FR-2 defines `session_seq` as "byte offset in session scrollback at time of observation." The `CircularBuffer` in `session/circular_buffer.go` overwrites old data when full (10 MB default). This means:

- The absolute byte offset into the PTY stream increases monotonically (not the circular buffer's internal index).
- If Stage 1 observes sequence at byte offset `N` and Stage 2 writes a chunk that covers bytes `M..M+K`, the offset comparison requires the Stage 2 capture to know which byte range of the session stream it corresponds to.
- The circular buffer does **not** track total bytes written (only `count` = bytes currently in buffer). There is no monotonically increasing global byte counter.

**Failure mode**: without a monotonic session byte counter, `session_seq` cannot be reliably computed at Stage 2. Two sequences at the same circular buffer position in different wraps of the buffer will collide on `(session_id, session_seq)`.

**Mitigation**: add a `totalBytesWritten int64` field to `CircularBuffer` that is never decremented on overwrites. Use this as the stable `session_seq` value at both stages.

### 4.2 Out-of-order writes to SQLite

The batch writer flushes every 500 ms. Stage 1 and Stage 2 observations may be batched in different flush cycles. A Stage 2 observation may be written to SQLite **before** the corresponding Stage 1 observation (if the Stage 1 batch hasn't flushed yet when the Stage 2 lookup occurs).

The mangle detection query at Stage 2 (FR-7: "look up matching Stage 1 observation by (session_id, session_seq)") will return no row, causing a false negative (missed mangle detection) rather than a false positive.

**Mitigation**: mangle detection should be deferred — either:
a) Run as a reconciliation pass after session close, not inline at Stage 2 write time.
b) Keep Stage 1 observations in a short-lived in-memory LRU keyed by `(session_id, session_seq)` for the duration of the batch window, and do the lookup against memory rather than SQLite.

### 4.3 Clock skew between Stage 1 and Stage 2

The `wall_time` field is captured independently at each stage. Stage 1 records time at PTY read; Stage 2 records time at ConnectRPC frame serialization. The delta between these timestamps is not meaningful for mangle detection (the same sequence can take 0–100+ ms to transit the pipeline under load). Using `wall_time` for correlation will produce false matches (two different sequences happening to share a timestamp).

**Mitigation**: `session_seq` (byte offset) is the only reliable correlation key. `wall_time` should be treated as an audit timestamp only, not used in the mangle detection lookup.

### 4.4 Buffer position drift under high-throughput redraws

vim and similar programs emit alternating-screen-enter sequences, bulk redraws (thousands of SGR + cursor sequences), and alternating-screen-exit in a single burst. Between Stage 1 parse and Stage 2 capture:
- The circular buffer `head` pointer may have advanced significantly.
- If sampling rate < 1.0 and sampling applies per-chunk, a chunk may be sampled at Stage 1 but the corresponding Stage 2 chunk (covering the same byte range) may be from a different sample decision.

The requirement (NFR-3) says "Sampling applies per-session-chunk, not per-sequence." This means if a chunk is sampled at Stage 1 it must also be sampled at Stage 2. This requires the sampling decision to be stable per `session_seq` range, not re-rolled independently at each stage.

**Mitigation**: derive the sampling decision from a deterministic function of `(session_id, chunk_seq_number)` (e.g. `hash(session_id + chunk_number) < threshold`) so both stages make the same decision without needing to share state.

---

## 5. Security Pitfalls

### 5.1 Terminal escape injection via stored raw bytes

At `capture_level=full`, `raw_bytes` contains verbatim PTY output including executable terminal sequences. If these bytes are displayed in the web UI (e.g. in a "raw bytes" column in the event table), a malicious or buggy program could inject sequences that:
- Reset terminal state in the browser's developer tools console.
- Manipulate the xterm.js instance rendering the UI (if the bytes are written to a terminal rather than a DOM element).
- Embed OSC hyperlinks (`ESC ] 8 ;; url ESC \`) that display as clickable links to attacker-controlled URLs.

The specific risk depends on whether the web UI renders `raw_bytes` as text in a terminal element or as escaped HTML. In the React SPA the `EscapeEvent` proto's `raw_bytes` field should be treated as opaque binary data and rendered only as hex or base64, never written to a terminal.

**Mitigation**: the `raw_bytes` field must never be passed to xterm.js or any terminal renderer. Display it only via `hex.EncodeToString` or base64 in DOM text nodes.

### 5.2 PII in OSC window titles and clipboard data

OSC sequences commonly carry user-visible strings:
- `OSC 0 ; <title> BEL` / `OSC 2 ; <title> BEL` — sets window/tab title. Titles often contain file paths, hostnames, usernames, or even passwords if the user types in a visible prompt.
- `OSC 52 ; c ; <base64-data> BEL` — clipboard access. The base64 payload is clipboard content, which may include API keys, passwords, or PII.
- `OSC 7 ; <uri> BEL` — current working directory as a `file://` URI, revealing the full filesystem path.

At `capture_level=full`, all of this is stored in `raw_bytes`. At `capture_level=summary`, the `sequence_subtype` will identify these as "OSC window title" or "OSC clipboard" but the payload hash is still computed from the raw bytes (SHA-256 of a short title is reversible by brute force).

**Mitigation**:
- At `capture_level=full`, OSC 52 payloads should be **never stored** (the clipboard content itself provides no mangle-detection value).
- At `capture_level=summary`, do not hash OSC 0/2 payloads; store only the sequence type and byte length.
- Add a config flag `EscapeAnalyticsRedactOSCPayloads bool` defaulting to `true`.

### 5.3 Filename/path exposure via DCS and OSC sequences

tmux passthrough DCS sequences (`ESC P tmux ; ...`) can carry inner sequences that include file paths (e.g. from editors announcing their open file via OSC 7). Similarly, iTerm2-style `ESC ] 1337 ; File=name=<filename> BEL` sequences embed filenames in the analytics record.

At `capture_level=full`, storing these verbatim means session analytics records contain the user's filesystem layout. This is not a concern for local single-user deployments but becomes significant if analytics data is ever exported or shared (e.g. for debugging mangle issues).

### 5.4 SQL injection risk is non-existent but query injection via labels is possible

The ent ORM uses parameterized queries exclusively; there is no SQL injection risk from `raw_bytes` or `sequence_subtype` fields stored in the database. However, the `GetEscapeAnalyticsSummary` RPC aggregates by `sequence_type` string (which comes from untrusted PTY data). If this value is ever interpolated into a dynamic ent query string (rather than used as a parameterized filter value), injection is possible. Verify that all ent queries use typed field selectors, not string interpolation.

---

## Summary Table

| Area | Severity | Pitfall | Recommended Fix |
|---|---|---|---|
| Parser | High | Unbounded `partialBuffer` growth for never-terminated sequences | Cap at 4 KB, discard with log |
| Parser | Medium | 8-bit C1 codes (`0x80–0x9F`) silently ignored | Add C1 dispatch branch |
| Parser | Medium | OSC payloads uncapped; multi-KB blobs stored at `full` level | Hard cap OSC scan at 64 KB |
| Parser | Medium | tmux DCS passthrough always appears as mangle | Add passthrough unwrap logic |
| Performance | Critical | `EscapeCodeStore` write-lock + sort on every sequence at PTY read rate | Non-blocking channel; batch writer with `select { default: drop }` |
| Performance | High | Channel backpressure stalls PTY reader if writer falls behind | Buffered channel (≥1000), explicit drop counter |
| Performance | Medium | WAL auto-checkpoint mid-burst stalls batch writer | `_wal_autocheckpoint=0`, explicit checkpoint at session close |
| SQLite | High | `raw_bytes` OSC 52 clipboard: single row can be 50 KB | Cap `raw_bytes` per row; skip OSC 52 payload at `summary` level |
| SQLite | Medium | No retention policy for `escape_event` table | Extend `retention.go` to cover new table |
| Correlation | High | No monotonic byte counter in `CircularBuffer` → `session_seq` not reliably computable | Add `totalBytesWritten int64` to `CircularBuffer` |
| Correlation | High | Stage 1/Stage 2 batch windows differ → mangle lookup hits empty DB | Deferred reconciliation pass at session close, or in-memory Stage 1 LRU |
| Correlation | Medium | Sampling decision made independently at each stage → false negatives | Deterministic sampling: `hash(session_id + chunk_seq) < threshold` |
| Security | High | OSC 52 clipboard content stored verbatim at `full` level | Never store OSC 52 payload; add `EscapeAnalyticsRedactOSCPayloads` config |
| Security | High | `raw_bytes` rendered in UI could reach terminal renderer | Render only as hex/base64 in DOM text nodes |
| Security | Medium | OSC window titles contain PII; SHA-256 of short strings is reversible | Do not hash OSC 0/2/7 payloads at `summary` level |
