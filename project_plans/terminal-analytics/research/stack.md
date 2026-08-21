# Stack Research — Terminal Escape Code Analytics

## Q1: Go Libraries for Parsing ANSI/VT100/VT220 Escape Sequences

### Candidate Libraries

#### `github.com/Azure/go-ansiterm` (already in go.mod as indirect dep)
- **What it is**: A state-machine-based ANSI/VT parser used internally by Docker/Moby for PTY handling. Already present as an indirect dep via `github.com/moby/term`.
- **Parser model**: Push-style event handler interface (`AnsiEventHandler`) with named methods per sequence type (CUU, CUD, SGR, ED, EL, OSC, DCS, etc.). Stateful — persists partial sequence state across `Parse()` calls automatically.
- **States**: Ground, Escape, EscapeIntermediate, CsiEntry, CsiParam, DcsEntry, OscString, Error.
- **Completeness**: Covers CSI, OSC, DCS (partial), and simple ESC sequences. Does not expose raw bytes to the handler — only decoded semantic events. Missing: APC, PM, SOS, C1 single-byte sequences, SS2/SS3.
- **Maintenance**: Actively maintained by Microsoft/Azure for Docker use. Last commit January 2025.
- **Fit**: Strong fit for the semantic decode path (it gives you structured events), but the handler interface cannot easily give you raw bytes + offsets needed for `payload_hash` and `session_seq`. Would require wrapping to capture raw bytes alongside events.

#### `github.com/charmbracelet/x/ansi` (NOT in go.mod; would be a new dep)
- **What it is**: Charmbracelet's modern terminal utility library. Provides both a streaming parser and sequence tokenizer.
- **Parser model**: Iterator/tokenizer style — returns typed tokens with raw bytes accessible. Handles split sequences (buffers partial).
- **Completeness**: CSI, OSC, DCS, APC, PM, SOS, SS2, SS3, C1, PM. Very complete VT220+ coverage.
- **Maintenance**: Actively maintained (part of Charm's core toolchain, updated regularly in 2024–2025).
- **Fit**: Excellent API fit for this project — returns raw bytes per token which is exactly needed for `payload_hash`. However, adding a new direct dep just for parsing when a custom parser already exists is a cost.

#### `github.com/aoldershaw/ansi` (NOT in go.mod)
- **What it is**: A small ANSI sequence parser focused on terminal emulation (cursor state, color state).
- **Completeness**: Targets common CSI/SGR sequences for rendering. Not as complete as the above two for the full VT220 spectrum.
- **Maintenance**: Low activity (personal project). Not recommended for production.

#### Custom parser in `pkg/analytics/escape_code_parser.go` (already exists)
- **What it is**: A fully hand-written parser implementing all sequence types required by FR-1: CSI, OSC, DCS, PM, APC, SOS, C1, Simple, Charset.
- **Features**: Returns `ParsedEscapeCode` with `RawBytes`, `HexEncoded`, `Category`, `Description`, `StartOffset`, `EndOffset`. Handles split sequences via `partialBuffer`. Already wired to `EscapeCodeStore`.
- **Completeness**: Covers everything in FR-1. The `categorizeCSI` logic delegates to `escape_code_descriptions.go` for semantic naming.
- **Gap vs. requirements**: Missing `session_seq` (byte offset tracking) — currently only tracks `StartOffset` within a single chunk, not the cumulative scrollback offset. Also missing `payload_hash` computation.

### Recommendation: Extend the Custom Parser

The existing `pkg/analytics/escape_code_parser.go` already implements the full FR-1 spec. It is the best fit:

1. **Zero new dependencies** — `github.com/Azure/go-ansiterm` is already in the module graph (no `go get` needed for direct use), but it doesn't expose raw bytes, making it harder to implement `payload_hash`. The custom parser already returns `RawBytes`.
2. **Exact API match** — The `ParsedEscapeCode` struct already has `StartOffset`/`EndOffset`. Adding a cumulative `sessionSeq int64` to the `EscapeCodeParser` struct and threading it through is a small change.
3. **Split-sequence handling** — Already implemented via `partialBuffer`.
4. **`charmbracelet/x/ansi`** is architecturally superior but adds a new module dep for functionality already present. Consider it only if the custom parser proves insufficient in benchmarks or edge cases.

If `github.com/Azure/go-ansiterm` is eventually used directly (e.g., for full terminal emulation in a framebuffer), it can complement the custom parser — use `go-ansiterm` for semantic dispatch and the custom parser for byte-level observation.

---

## Q2: Zero-Overhead Instrumentation Patterns in Go

### Options Evaluated

#### Interface-based noop (current pattern in this codebase)
`server/analytics/provider.go` already uses this: `AnalyticsProvider` interface with `LogAnalyticsProvider` as a no-op logging fallback. The existing `EscapeCodeStore.Record()` guards with `if !s.enabled { return }`.

- **Overhead when disabled**: One boolean field check + return. In Go, a simple struct field bool check after inlining is essentially free (single branch prediction hit). The function call itself is not inlined if it goes through an interface — this is the key cost.
- **Cost**: Interface dispatch is ~2–5ns per call (virtual dispatch + pointer indirection). For a path parsing 4KB chunks at <50µs budget, this is negligible (~1–2% of budget even at 1000 calls/chunk).

#### `sync/atomic` flag check
```go
var enabled int32 // 0=off, 1=on
if atomic.LoadInt32(&enabled) == 0 { return }
```
- Adds memory-fence semantics, useful for hot paths shared across goroutines where the flag may change at runtime. Overhead is ~1–2ns. Unnecessary here since the capture level is set at startup and rarely changes.

#### Build tags (`//go:build noinstrumentation`)
- Compile-time elimination: zero overhead at runtime, but requires a separate build target. Not appropriate here since the requirement (NFR-3) is *runtime* configurability, not compile-time.

#### Concrete type check (recommended for hot path)
The most efficient pattern for this codebase: store a `noopEscapeObserver` concrete type when `capture_level=off`:

```go
type EscapeObserver interface {
    Observe(data []byte, stage Stage)
}

type noopEscapeObserver struct{}
func (noopEscapeObserver) Observe([]byte, Stage) {}

type escapeObserver struct { ... }
func (o *escapeObserver) Observe(data []byte, stage Stage) { ... }
```

When `capture_level=off`, inject `noopEscapeObserver{}`. The Go compiler will devirtualize calls to a concrete type stored in an interface if the call site can determine the concrete type. Even without devirtualization, an empty method body with no side effects is branch-predicted to near-zero cost.

The `EscapeCodeParser.enabled` bool guard already achieves this — keep it. For Stage 2 (transport), the channel send is the hot path: use a nil-channel check:

```go
if p.ch == nil { return } // channel is nil when disabled — send to nil blocks forever, so gate it
```

### Recommendation

Use the **interface noop pattern** (consistent with the existing `AnalyticsProvider` / `LogAnalyticsProvider` approach). For the hot path in Stage 1 (every PTY read):
- Keep the `if !p.enabled { return data }` guard at the top of `Parse()` — this is a struct field bool, not interface dispatch, so it is optimally fast.
- For Stage 2 async channel send: use a buffered channel with a **non-blocking send** (`select { case ch <- ev: default: }`) — never blocks the transport path.

---

## Q3: Batched Async SQLite Writes with ent ORM

### Current Pattern in This Codebase

`server/analytics/sqlite_provider.go` uses a single-connection ent client (`db.SetMaxOpenConns(1)`), WAL mode (`_journal_mode=WAL`), and synchronous normal (`_synchronous=NORMAL`). Each `Record()` call issues one `INSERT`. This is fine for low-frequency analytics events but will be a bottleneck for high-frequency escape sequence events (potentially thousands per second during active terminal sessions).

### ent `CreateBulk` — Already Available

`AnalyticsEventCreateBulk` is already generated in `session/ent/analyticsevent_create.go`. The ent ORM supports:

```go
client.AnalyticsEvent.CreateBulk(builders...).Save(ctx)
```

This issues a single `INSERT INTO ... VALUES (...), (...), (...)` statement, which is dramatically faster than N individual INSERTs due to reduced lock acquisition overhead.

### Recommended Pattern: Channel + Ticker Batch Flusher

This is the standard Go pattern for high-frequency write batching:

```go
type EscapeEventWriter struct {
    ch     chan escapeEventRow   // buffered, e.g. cap=1000
    client *ent.Client
}

func (w *EscapeEventWriter) Start(ctx context.Context) {
    ticker := time.NewTicker(500 * time.Millisecond)
    defer ticker.Stop()
    var batch []escapeEventRow
    for {
        select {
        case row := <-w.ch:
            batch = append(batch, row)
            if len(batch) >= 100 {
                w.flush(ctx, batch)
                batch = batch[:0]
            }
        case <-ticker.C:
            if len(batch) > 0 {
                w.flush(ctx, batch)
                batch = batch[:0]
            }
        case <-ctx.Done():
            // Drain remaining
            w.flush(ctx, batch)
            return
        }
    }
}

func (w *EscapeEventWriter) flush(ctx context.Context, rows []escapeEventRow) {
    builders := make([]*ent.EscapeEventCreate, len(rows))
    for i, r := range rows {
        builders[i] = w.client.EscapeEvent.Create().
            SetSessionID(r.sessionID).
            // ... set all fields
    }
    _ = w.client.EscapeEvent.CreateBulk(builders...).Exec(ctx)
}
```

**Key design choices**:
- **Buffered channel cap ~1000**: Absorbs bursts without blocking the hot path.
- **Flush at 100 rows OR 500ms** (matching FR-4's requirement exactly).
- **Non-blocking send on hot path**: `select { case w.ch <- row: default: /* drop if full */ }` — drop on buffer full rather than blocking the PTY reader.
- **WAL mode** (already configured): Allows concurrent reads while writing, critical since the ConnectRPC query path reads from the same DB.
- **Single ent client / single connection** (already configured): SQLite's single-writer constraint is already handled; all writes go through the one goroutine flusher, so there is no contention.

### Alternative: Separate `escape_events.db`

The open question in the requirements asks whether `escape_event` should be a separate DB file. Given the high write volume, a **separate `escape_events.db`** is recommended to avoid WAL checkpoint pressure on the main DB. The `OpenAnalyticsDB` pattern in `server/analytics/db.go` can be replicated trivially with a different filename.

---

## Q4: Go Libraries for Terminal Output Diffing / Sequence Comparison

### Available Libraries

No Go library purpose-built for "terminal sequence diffing" (comparing two byte streams at the escape-sequence level) was found in the module cache or major Go package indices. The closest candidates:

#### `github.com/Azure/go-ansiterm` (already in module graph)
- **Diffing use**: Could be used to replay two byte streams through its state machine and compare resulting terminal state (cursor position, color state, etc.) rather than raw bytes. This is a semantic diff.
- **Limitation**: Does not track which sequences were present in stream A but absent in stream B at the byte level.

#### `bytes.Equal` / SHA-256 hash comparison (stdlib)
- For the mangle detection requirement (FR-7), the spec calls for **byte-for-byte comparison** keyed on `(session_id, session_seq)`. This is straightforward with stdlib:
  - At Stage 1: compute `sha256(rawBytes)[:8]` as `payload_hash`, store with `session_seq`.
  - At Stage 2: compute hash of same bytes as they pass through transport; look up Stage 1 record by `(session_id, session_seq)`; compare hashes.
- No external library needed for this approach.

#### `github.com/sergi/go-diff` (not in module graph)
- Implements Myers diff algorithm (text diffing). Could be used to diff the textual representation of two byte streams' parsed sequences. Overkill for the byte-exact mangle detection in FR-7, but potentially useful for a richer "what changed" display in the web UI.

#### Custom sequence-level diff
- Given the `ParsedEscapeCode` struct with `StartOffset`/`EndOffset`, a simple O(n) scan comparing Stage 1 and Stage 2 parsed sequences by position and hash is sufficient and needs no external library. The mangle detection described in FR-7 (look up by `session_seq`) is essentially a hash-map lookup, not a diff.

### Recommendation

No new library needed for diffing. Implement mangle detection as:
1. In-memory ring buffer (keyed by `session_seq`) storing Stage 1 `payload_hash` values, capped per session.
2. At Stage 2, hash the bytes at the same offset, do a map lookup, and set `mangled=true` on mismatch.

This is O(1) per sequence and requires no new dependencies. The `EscapeCodeStore` in `pkg/analytics/escape_code_store.go` already demonstrates the pattern for in-memory keyed storage.

---

## Summary of Decisions

| Question | Decision | Rationale |
|---|---|---|
| ANSI parser library | Extend existing `pkg/analytics/escape_code_parser.go` | Already complete, returns raw bytes, handles split sequences; zero new deps |
| Zero-overhead when disabled | Bool guard (`if !p.enabled`) on hot path + noop interface at integration boundary | Consistent with existing codebase pattern; struct-field bool avoids interface dispatch on the critical path |
| Batched SQLite writes | Channel + Ticker goroutine using ent `CreateBulk`, flush at 100 rows / 500ms | Matches FR-4 spec exactly; `CreateBulk` is already generated; WAL mode already configured |
| Sequence diffing | No external library; hash-map lookup by `session_seq` | FR-7 requires byte-exact comparison, not fuzzy diff; stdlib SHA-256 + map is sufficient |
| DB file placement | Separate `escape_events.db` | Isolates high-write-volume escape events from main analytics DB to avoid WAL pressure |
