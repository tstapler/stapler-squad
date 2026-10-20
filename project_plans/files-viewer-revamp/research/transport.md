# Research: pluggable range-read and delta-write file transport

**Date**: 2026-10-16
**Feeds**: `decisions/ADR-036-pluggable-range-and-delta-file-transport.md`
**Owner request**: "design a better pluggable web transport for sending parts of files for reading and serializing writes to their smallest parts instead of sending the full content."

Labels: **VERIFIED** (opened in this repo or a cited artifact in this session), **INFERRED** (reasoned, not run), **UNVERIFIED** (needs a spike or benchmark; named in section 9). This research originally edited nothing else; the changes it lists in section 8 were **made on 2026-10-16** (ADR-036 Accepted, plan/ux/validation/requirements updated; see the plan's Review repair log, "ADR-036 integration"). **The CU figures in sections 8.1 and 11.5 were top-down and have been replaced by the integrated plan's bottom-up figures (recomputed by script from the task classes); the original numbers are kept in the "Top-down (original)" column for the record.**

## 0. Baseline (what exists today)

- VERIFIED `proto/session/v1/session.proto:295-310`: the file RPCs are `ListFiles`, `GetFileContent` (whole file; "files over 10MB are rejected; files over 1MB are served truncated with is_truncated=true"), `SearchFiles` (names only). `GetFileContentResponse` (`:2319-2337`) carries `content`, `encoding`, `is_binary`, `size`, `content_type`, `is_truncated`. There is no range parameter, no version token and no write RPC today (the plan adds token, `StatFile` and `WriteFile`: plan Story 0.2.3, Epic 2.1).
- VERIFIED `server/services/file_service.go:28-32,282,707`: constants `maxFileSize = 10 MB`, `truncateSize = 1 MB`; `GetFileContent` at `:282`; `ServeFileRaw` at `:707` streams PDF/video with `http.ServeContent` ("without buffering, no size cap", comment near `:752`), so byte `Range` already works for media on that route (the Range support is a property of `http.ServeContent`, INFERRED from the stdlib; not exercised here).
- VERIFIED planned limits in ADR-031: token `v1:<size>:<mtime_ns>:<sha256hex>` (hash `-` for non-editable reads); editable cap = 1 MB = the non-truncated read ("no relaxed-truncation edit path"); `WriteFile` path-scoped body limit 1.5 MiB; the server always compares sha256 inside a per-path mutex; temp + fsync + rename; the client sends LF-normalized text and the server derives EOL/BOM; `WriteFile` joins `guardedProcedures`.
- VERIFIED `server/server.go:1879-1888`: `guardedProcedures` is a `map[string]middleware.GuardProfile` whose members (`pruneProcedurePath`, `updateFlagProcedurePath`, `replyProcedurePath`, ...) use `middleware.ProfileRebinding`. VERIFIED `server/server.go:1928-1938`: `remoteChain` (the `:8444` chain) wraps `authMW` then `stampRequest(services.ListenerRemote, ...)`, then `Logging`, `CORSWithOrigins`, `Compress`. The local `:8543` chain has no auth (ADR-031 text). So the listener is knowable per request from the stamped request record (INFERRED reuse; the stamp exists, its consumption by a capability handler is new).
- VERIFIED by grep: `server/` has no `connect.WithReadMaxBytes`; body caps are done with `http.MaxBytesReader` in separate HTTP handlers (`server/handlers/telemetry_handler.go:79`, `server/services/session_image_upload_handler.go:89`, others). ADR-031 already plans a path-scoped limiter middleware.
- VERIFIED `web-app/src/lib/transport/`: `websocket-transport.ts` and `watch-ws-transport.ts` (+ tests). `watch-ws-transport.ts` builds a Connect `Transport` over a WebSocket (it imports `runStreamingCall`, `endStreamFlag`, hand-encodes 5-byte Connect envelopes and exposes `fromWebSocket`). VERIFIED `server/server.go:469,520-706`: `wsHandler.HandleWebSocket` plus `StreamingWSBridge` registrations for `Watch*` server-streaming RPCs; ADR-030 decision 4 records that `SearchContent` is deliberately not a `Watch*` RPC and rides plain Connect fetch streaming. INFERRED: the WS bridge carries server-streaming calls only; a request/response or bidirectional multiplex over it would be new protocol work.
- VERIFIED (ADR-030 item 4 cites `server/middleware/gzip.go:155-163`): `middleware.Compress` passes every `/api/` path through uncompressed. Therefore file-window responses get no HTTP-level compression unless Connect-level compression is enabled (UNVERIFIED whether connect-web negotiates it; spike S-T1).
- Already planned and relevant: an Accessible view that pages 500 lines at a time (plan Story 1.5.4), a degrade profile that turns features off for a 600 KB file on coarse pointers (Story 1.5.2), staleness banner via `StatFile` (Story 1.3.4), `SearchContent` with a 2 MB per-file cap (ADR-030 decision 1), bounded drafts of 256 KB (ADR-032), 3-way merge (ADR-034).

**Why this matters (the cost of the current wire).** A one-character save of a 1 MB file ships about 1.34 MiB base64 in Connect JSON (ADR-031's own arithmetic) over a phone's Tailscale link; a 50 MB log cannot be opened at all (10 MB reject) and a 5 MB file is read-only and truncated, so "large files must not freeze the mobile browser" (requirements NFR) is met by refusing them, not by serving them. Pitfall W1 (saving a truncated buffer destroys the tail) is avoided only by forbidding edits of anything truncated.

## 1. Architecture: two ports, many adapters

Hexagonal split so transports are swappable without touching safety rules.

```
UI (CodeEditor, WindowedViewer, AccessiblePager, tab/buffer hooks)
        |  uses only
   FileTransport  (client port; TS interface)          <- router picks adapters per op
        |
   adapters:  FullFileTransport | ConnectUnaryTransport | ConnectStreamTransport
              | WsStreamTransport | HttpRangeTransport
        |  wire (Connect/WS/HTTP)
   server handlers (thin; decode, authenticate, rate-limit, encode)
        |
   FileOps  (application service; ALL policy lives here)
        - resolveInWorktree (os.Root), .git deny, symlink rules (ADR-031)
        - files:editing / files:* flags, hidden-session guard, audit log, analytics port
        - per-path mutex, base-token check, idempotency store, limits
        |
   FileBackend  (server storage port; Go interface)
        osRootBackend (production) | memBackend (tests)
```

Rule: **no safety logic in an adapter**. An adapter may translate bytes and framing only; confinement, flags, token checks and verification are in `FileOps`, exercised once by `FileOpsConformance` and reachable from every wire. This is what makes plugging safe (a new adapter cannot weaken ADR-031).

### 1.1 Client port (TypeScript, shape not final)

```ts
interface FileTransport {
  readonly id: "full" | "connect_unary" | "connect_stream" | "ws_stream" | "http_range";
  capabilities(): FileCapabilities;                       // resolved for this adapter
  stat(ref: FileRef, o?: { wantAnalysis?: boolean }): Promise<FileStat>;
  read(ref: FileRef, req: ReadRequest, sig?: AbortSignal): Promise<ReadResult>;
  readStream?(ref, req, sig): AsyncIterable<ReadResult>;  // optional op
  patch?(ref: FileRef, p: PatchRequest, sig?): Promise<PatchResult>;
  writeFull(ref: FileRef, w: WriteRequest, sig?): Promise<WriteResult>; // always present (ADR-031)
}
type ReadRequest =
  | { kind: "all"; maxBytes: number }                                    // degrade adapter
  | { kind: "lines"; anchorLine: number; dir: "around"|"fwd"|"back"; wantLines: number; maxBytes: number }
  | { kind: "bytes"; offset: number; length: number; dir?: "fwd"|"back" };
```

`FileRef = {sessionId, path}`; results always carry the **validator** (size, mtime_ns, token), the actual served range `{startLine, endLine, startByte, endByte, bof, eof}`, and a per-response `chunkCrc32c` for transport-integrity. A `Result` type with typed `Rejected(reason)` mirrors the ADR-031 `ReadOnlyReason` mapping. Existing hooks (`useOpenBuffers`, `useSaveBuffer`, `useBufferFreshness`) take a `FileTransport`, never the generated Connect client (Wave A, section 8).

### 1.2 Server port (Go, shape not final)

```go
type FileBackend interface {
    Open(ctx, root *os.Root, rel string) (FileHandle, error)         // read-only handle, not a symlink
    Stat(...)
    ReadAt(h FileHandle, off int64, p []byte) (int, error)
    Analyze(ctx, h) (Analysis, error)                                // line index, EOL class, NUL, sha256 in one pass
    Splice(ctx, root, rel string, base Analysis, edits []Edit, v Verifier) (Result, error) // temp+fsync+rename
    ReplaceAll(...)                                                  // ADR-031 write, kept
}
```

`Edit = {StartByte, EndByte int64; Insert []byte}` in **bytes of the raw file**; `FileOps` converts the wire's line/col to bytes using the line index so no adapter ever sees bytes (section 3.3).

## 2. Capability negotiation, versioning, fallback ladder

**One new unary RPC `GetFileTransportCapabilities(session_id)`**, read-only and unguarded like other reads.

```
FileTransportCapabilities {
  uint32 protocol_version = 1;          // major; breaking only
  repeated string features;             // additive: "range.lines","range.bytes","index","patch","stream","analysis"
  repeated Op ops;                      // STAT, READ_LINES, READ_BYTES, READ_STREAM, PATCH, WRITE_FULL
  repeated Wire wires;                  // CONNECT_UNARY, CONNECT_STREAM, WS_STREAM, HTTP_RANGE  (per-listener truth)
  Limits limits;                        // max_range_bytes, max_range_lines, preferred_chunk_bytes,
                                        // max_patch_ops, max_patch_body_bytes, max_insert_bytes, max_write_full_bytes,
                                        // hash_cap_bytes (editable ceiling), max_concurrent_ranges
  uint64 caps_epoch;                    // bumps when a flag or limit changes
}
```

- **Per-listener truth** (INFERRED mechanism, VERIFIED inputs): the handler reads the stamped request record, so `:8444` can omit `PATCH` until Task 0.1.3b decides whether write RPCs are reachable remotely, and `:8543` can omit nothing. The client never guesses tiers. A flag flip (`files:editing`, `files:delta-write`) changes `ops` and bumps `caps_epoch`.
- **Staleness of capabilities**: every `ReadResult`/`PatchResult` carries `caps_epoch`; a mismatch makes the router refetch once (live-settable flags change under a running client).
- **Versioning**: `protocol_version` major mismatch (server major > client max) means the router pins `FullFileTransport`. Minor evolution is by new `features`/`ops`/fields; clients ignore unknown strings. `Unimplemented` from the capability RPC (older server, stale PWA vs newer server or the reverse) is treated as `{ops: [STAT?, WRITE_FULL, READ_ALL]}`, i.e. today's behavior.
- **Router**: `FileTransportRouter` resolves each op independently (`read`, `patch`, `stat`, `writeFull`), so reads can use `connect_stream` while writes stay unary. Ladder per op, highest first: `ws_stream` (reads, only if the WS bridge path passed the Story 0.1.1-style probe) > `connect_stream` > `connect_unary` > `full`. `http_range` is selected only for byte-only consumers (media/binary preview, the optional hex/raw view) and is never chosen for line windows.
- **Demotion**: three consecutive transport-class failures (network error, `Unavailable`, deadline, malformed envelope; never application errors like `Conflict`) demote that op one rung for the page lifetime, emit `files.transport.fallback{from,to,reason}` (ADR-035 extension), and surface a quiet status-bar chip. Promotion is a page reload, not automatic flapping.
- **Degrade-to-full semantics**: `FullFileTransport` implements `read(kind:"all")` over `GetFileContent` and `writeFull` over `WriteFile`; it advertises `limits.hash_cap_bytes = 1 MiB`, so the viewer behaves exactly as the base plan does (truncated reads stay read-only; ADR-031 refusal intact). No feature depends on a rung above it except windowed viewing of files over 1 MiB and delta saves, both of which simply don't light up.
- **Flags** (live-settable, global; no env vars per owner's rule): `files:range-read`, `files:delta-write`, `files:edit-large` (booleans). A forced-adapter override for testing is a boolean kill per adapter in the same flag family, not a string flag (whether the flag RPC supports non-boolean values is UNVERIFIED; booleans are the verified shape in `feature_flag_mutation.go:30-36` per ADR-031).

## 3. Partial reads

### 3.1 Request shapes and the line/byte duality

Two windows over one file, both returning contiguous, **line-aligned** text plus the numbers needed to join them:

- **Line window** `Anchor{line N}, dir, want_lines (default 400, max 4000), max_bytes (default 128 KiB, max 1 MiB)`: serves jump-to-line, search hits, Accessible pager pages (500 lines), scrolling.
- **Byte window** `Anchor{byte B}`: serves "end of file" (`G`), progress-bar seeks, `http_range`, hit `byte_offset` jumps, huge single-line files.

Response always includes `start_line, end_line, start_byte, end_byte, bof, eof, line_count_known, line_count (if known)`, so the client can place the window with no index of its own. Windows are snapped to line boundaries (never mid-line, except the long-line case below), which makes chunk caching trivially joinable.

### 3.2 UTF-8, CRLF, BOM, binary, long lines

- **UTF-8 boundary** (INFERRED, standard): a byte anchor snaps backward over at most 3 continuation bytes (`10xxxxxx`) to a rune start; line windows begin and end at `\n` so cannot split a rune. Invalid UTF-8 in a window sets `non_utf8` for that window; the viewer shows the existing NonUtf8 read-only reason instead of rendering mojibake.
- **BOM** only exists at byte 0: the first window strips it and sets `has_bom`; all byte offsets on the wire stay raw-file offsets (offset base never shifts).
- **CRLF**: windows are returned LF-normalized (client invariant of ADR-031) but byte offsets remain raw, with `eol_class` reported as `LF | CRLF | Mixed | Unknown`. `Unknown` until analysis completes (3.4). Editing requires a completed analysis, because `Mixed` or a lone `\r` is read-only (ADR-031/plan Task 0.2.3b) and cannot be proven from a partial scan.
- **Binary**: the first response sniffs 8 KB for NUL (the same sniff as ADR-030) and sets `is_binary`; the analysis pass also scans the whole file, and a later window containing NUL sets `contains_nul_at`. The viewer then stops text paging and shows the Binary state. (INFERRED: git's heuristic also looks only at the first ~8 KB, so a late-NUL file is a rare but real inconsistency we handle explicitly.)
- **Long lines** (minified JS, single-line JSON): a line over `max_line_bytes` (default 256 KiB, proposed) is flagged `long_line` by the analysis pass; the viewer switches that region to **byte-window mode** (wrapped, read-only) and disables edit for the file. Windows may then end mid-line, marked `partial_last_line`.

### 3.3 Line index (server) versus client discovery

Option A, **server sparse line index** (recommended): one streaming pass (`Analyze`) records the byte offset of every 4,096th line plus totals (lines, `\n`, `\r\n`, lone `\r`, NUL, max line length) and computes sha256 of the whole file. Memory: a 100 MB file with 2 M lines stores about 500 checkpoints (INFERRED, order of kilobytes). Line-to-byte = nearest checkpoint then scan at most 4,096 lines (about 200 KB at 50 B/line, INFERRED), so any line window costs one small read. Cache: bounded LRU (for example 32 files, 64 KB each) keyed by `(path, size, mtime_ns, inode)`; an append-detected rebuild is a full rebuild (incremental extension is an optimization, deferred). The pass runs lazily in the background on first range request, ctx-cancellable, behind a process-wide semaphore of 1-2 builds (same pattern as ADR-030's walk semaphore), at an expected multi-GB/s on cached data (UNVERIFIED; benchmark S-T2). Until it completes, windows by line need the index only for random line access; sequential reads from line 1 and byte anchors work immediately, and `line_count_known=false` shows "about N lines" (size divided by sampled mean line length).

Option B, client discovery (client fetches chunks sequentially and counts): needs the whole file crossing the wire to reach line N, which defeats the goal. Rejected except as the degraded behavior of `http_range` (byte-only), where jump-to-line is unavailable.

Option C, persist the index on disk: rejected (extra state, invalidation, worktrees are agent-churned).

### 3.4 Version token and the "sha256 of a huge file is a full read" problem

Distinguish two jobs the token does:

| Job | Needs | Cost |
|---|---|---|
| **Staleness validator** for read caches (is my chunk still current?) | change detection | stat only |
| **Write identity** (ADR-031's base check) | content equality | a full hash |

Options:

1. **Keep v1 unchanged and make the hash lazy** (recommended). Range reads return `v1:<size>:<mtime_ns>:-` (the format already admits `-`; VERIFIED in ADR-031) plus `analysis: pending|ready`. The `Analyze` pass computes sha256 of the full file in the same pass as the index and caches `validator -> sha256`; once ready, tokens become the normal `v1:...:<sha>`. Edit entry waits for `analysis=ready` ("Preparing file for editing", a progress chip). No token format change; ADR-031's rule "size+mtime equality never accepts without hashing" is preserved because the write path re-hashes while streaming (3.2 of section 4). Staleness for caches uses `(size, mtime_ns)` (the same weakness as today: a same-size same-tick agent rewrite is not detected by a read-cache check, but it is always caught at write time by the hash, and by the focus-time `StatFile`).
2. **Chunk-digest list / Merkle root** (`v2:size:mtime:root`, root over 1 MiB chunk digests). Gives per-chunk verification and cheap partial revalidation of an unchanged prefix. Costs: a first full read anyway (same as option 1), a new format, server cache, and fixed-size chunks all shift after an insert so reuse after edit is poor without content-defined chunking. Rejected for now; revisit only if hash time dominates beyond the 64 MiB tier.
3. **Cheap validator only** (`size:mtime:ctime:ino`, head+tail hash). Weak against same-tick same-size rewrites; fine as a read-cache validator, **not** acceptable as write identity. Optional hardening: a `v2` that adds `ctime_ns`/`ino` to detect rewrites `touch -d` can hide; deferred (open question 6).
4. **Per-response chunk CRC32C** (adopted in addition, not instead): the server computes CRC32C of the exact bytes it sent (free, they are in memory); the client verifies after decoding. This detects transport/codec corruption, not staleness.

Tier table (constants proposed, adjustable; the 64 MiB ceiling is INFERRED from "hash plus rewrite stays near a tenth of a second" and is UNVERIFIED until S-T2):

| Tier | Size | Read | Edit |
|---|---|---|---|
| T0 | up to 1 MiB | one-shot full (today), identical to base plan | full or delta (delta only shrinks the wire) |
| T1 | 1 MiB to 64 MiB (`hash_cap_bytes`) | windows + index + analysis | delta only, one editable window at a time, behind `files:edit-large` |
| T2 | over 64 MiB, up to the read ceiling (proposed 1 GiB) | windows + index, no sha | view-only (`ReadOnlyReason=TooLarge`) |

### 3.5 Chunk size, prefetch, cache

- Unary round trip is the unit; defaults `want_lines=400` / `max_bytes=128 KiB` for scrolling (about 1-2 screens on desktop, several on a phone), pager pages 500 lines, hit-jump window 200 lines either side of the hit. Server hard caps: 4,000 lines and 1 MiB per response.
- **Prefetch**: keep the viewport window plus one window ahead and one behind; when scroll velocity exceeds one window per second prefetch two ahead in the direction of travel; cancel (AbortSignal) on direction change or when the viewport jumps; at most 2 ranges in flight per buffer and 4 per tab. Rationale: HTTP/1.1 gives about 6 connections per origin shared by all tabs (assumed in ADR-030 item 6, UNVERIFIED per browser), and `:8543` is plain HTTP, so ranges must be short unary calls, not long streams, and must stay inside the same per-origin budget as `SearchContent` (the limiter seam in ADR-030 decision 6 extends to range calls with a separate, higher cap).
- **Client chunk cache**: LRU by bytes, budget 8 MiB on `pointer: coarse` and 64 MiB on desktop (proposed), keyed by `(sessionId, path, validator)`; a validator change drops every entry for that file.
- **Staleness**: every response carries the validator; a differing validator in any response drops the cache and shows the existing "changed on disk" banner (Story 1.3.4). `StatFile` on focus/visibility remains the proactive path. The server also fstat's before and after the read and returns `torn=true` if the file changed during the read; the client retries once, then treats the window as stale.

### 3.6 CodeMirror 6 interaction (the real design question)

CM6 `Text` is a rope of fully materialized lines; there is no supported lazy document (INFERRED from the public API: `Text.of(lines)`, no lazy leaf hook). Options:

| Option | Idea | Verdict |
|---|---|---|
| A. Placeholder lines | Full-length doc of blank lines, replace on load | Rejected: per-line overhead at 5 M lines is hundreds of MB (INFERRED); breaks undo, selection and search-in-doc |
| B. Custom lazy `Text` / piece table | Subclass or replace the document type | Rejected: CM6 does not support it; fork risk |
| C. **Windowed document** (recommended for T1/T2) | `EditorState` holds one window of lines only (for example up to 2,000 lines / 1 MiB); a `lineNumbers({formatNumber: n => String(n + windowStartLine)})` gutter; an external virtual scrollbar/spacer sized from line count; windows swap via `view.setState` with a scroll anchor line | Recommended |
| D. Pager only | No CM6 for large files, use the Accessible-view `<pre>` pages | Fallback for T2 and for `Reduce motion`/screen-reader users; reuses Story 1.5.4's pager |

Details for C:
- **Reads** use as many cached chunks as needed to fill a window; the window is replaced when the viewport nears either edge (within 25% of the window), preserving the anchor line so there is no jump. Window swap is `setState`, already proven necessary by ADR-032's one-view-per-buffer approach (spike 0.1.4 covers scroll preservation, a new spike S-T3 covers line-offset gutter and edge swap).
- **Edits**: exactly **one editable window** per buffer ("Edit this window", a range of at most 5,000 lines / 1 MiB around the cursor). Everything outside it is read-only. The window's `EditorState` is the unit of dirty state; navigating away from a dirty window requires Save or Discard (the dirty window is pinned like a dirty buffer in ADR-032). Ops are expressed in absolute lines by adding the window's start line. This sidesteps cross-window undo history and edit spanning windows, at the price of a visible mode; the owner may prefer a single full-doc mode for T0 only (open question 3).
- **Accessible view**: the pager already requests 500-line pages; it becomes the first consumer of `read(lines, around)`, shows "Lines 1-500 of about N" until `line_count_known`, and keeps its own non-virtual `<pre>` (no dependence on CM6 virtualization). Announce "loading" and "failed, retry" politely; focus management stays as Story 1.5.4 defines.
- **Search-hit jump**: `SearchContent` hits already carry a line; add `byte_offset` to the hit (not PII; raw offset) so `open(path, line N)` is one `read(lines, around N)` and works even before the index exists (byte anchor then renumber from `start_line`). For files over 2 MB, `SearchContent` needs a `large_files` mode: ADR-030's 2 MB per-file cap is a policy constant on a streaming scanner, so raise it per request to the hash cap (64 MiB) under a time budget and a lower worker count; hits still stream (INFERRED feasible; UNVERIFIED cost; S-T2 includes a 64 MiB scan). Without this, "huge files searchable" is false.
- **Tab/buffer state (ADR-032)**: `Buffer.Loaded` gains a `source: Full | Windowed{chunkCache, windowStartLine, lineCount?, analysis}` discriminant; persisted tab state stores `(path, anchorLine)` only (never chunk text).

## 4. Delta writes

### 4.1 Wire form: ordered splice ops in base-document line/column coordinates

CM6 gives `ChangeSet.iterChanges(fromA, toA, fromB, toB, inserted)` which is exactly a sorted, non-overlapping list of splices against the base document (VERIFIED from CM6's documented API as known; not re-run here). Wire:

```
Edit { Position start; Position end; string insert; fixed32 deleted_crc32c; }   // Position{ line (1-based), col_utf16 }
ApplyFilePatchRequest {
  FileRef ref; string base_token; bytes client_id (>=16B); uint64 seq;       // idempotency + ordering
  repeated Edit edits;                                                         // ascending, non-overlapping, in BASE coordinates
  uint64 expect_new_size;                                                      // client-computed (utf8 + EOL expansion)
  bytes expect_result_sha256;                                                  // present when the client holds the full doc (T0)
  repeated fixed32 region_crc32c;                                              // crc32c of each edited region AFTER applying (client-computed)
}
```

**Offset-unit decision.** Options: (i) UTF-16 offsets into the whole doc, (ii) byte offsets, (iii) line + UTF-16 column. Chosen: (iii).
- (i) is trivial in T0 but impossible in a windowed doc where the client does not hold the text before the window; (ii) forces the client to do UTF-8 and CRLF arithmetic (each `\n` is two bytes in a CRLF file, BOM shifts everything), which is where silent corruption lives; (iii) is what CM6 knows exactly (`doc.lineAt(pos)`; `col = pos - line.from`, already UTF-16), what LSP uses, and independent of EOL/BOM, which the server owns (ADR-031 "server derives EOL/BOM"). The server converts (line, col) to bytes via the line index and by decoding only the touched line (rejecting a column that splits a surrogate pair).
- Edits are expressed against the **base** (not applied sequentially), like a CM6 ChangeSet, so the server does a single forward pass and offsets never drift mid-batch.

### 4.2 Client sync loop (no OT)

Single human writer. Client keeps `pending: ChangeSet` composed over all transactions since the last acknowledged base (`pending = pending.compose(tr.changes)`), so typing runs, deletions, undo and redo collapse to the net edit (undo of an already-saved edit simply produces a new inverse patch). On Save: `inflight = pending; pending = empty-from-inflight-result`; at most one request in flight (stop-and-wait, which makes `seq` ordering a check rather than a protocol); edits made while in flight accumulate on top and become the next patch, based on the new token returned by the ack. No transformation is needed because no remote ops arrive.

### 4.3 Is OT/CRDT warranted? No (recommended).

One human writer; the other writers are agents and formatters that overwrite whole files with no operation log, so there is no stream of ops to merge, only before/after snapshots. A CRDT would have to *diff* those snapshots, which is exactly the 3-way merge of ADR-034 (INFERRED equivalence, the same information). OT/CRDT adds a server-side document model, tombstones and storage, and would make the file on disk a derived artifact, which breaks "the file on disk is the truth for agents". Revisit only if a second concurrent human editor becomes a requirement (explicitly out of scope in requirements).

### 4.4 Server write strategy

| Strategy | Large-file cost | Correctness | Verdict |
|---|---|---|---|
| In-memory splice then ADR-031 temp+rename | O(file) memory | ADR-031 safe | **T0** (up to 1 MiB) |
| **Streaming splice into a temp file** (read old, write prefix, inserts, suffix; sha256 of old and new computed in the same pass; fsync; rename) | O(file) IO, O(1) memory (64 MiB: about one read plus one write, tens to low hundreds of ms on a local SSD, UNVERIFIED) | Same atomicity as ADR-031; base hash verified before rename | **T1** |
| In-place `pwrite`/`ftruncate` shifting | O(tail) | Torn reads for agents and git; a failure mid-shift corrupts the file | Rejected (ADR-031 already rejects in-place) |
| Append-journal / WAL sidecar with deferred compaction | O(delta) | The file an agent reads is stale until compaction; git and builds see wrong bytes | Rejected |
| Server-held piece table | O(delta) | Divergent source of truth; lost on restart | Rejected |

The streaming pass makes the base check, splice and result hash a single read; the final stat re-check before rename follows ADR-031 (an mtime-only change in that window does not abort, a content change does). Disk preflight: free space at least file size plus slack, else `ResourceExhausted`. Large rewrites are serialized by a process-wide semaphore of 1 so a flurry of saves cannot saturate the disk.

Consequence worth stating: the delta path removes the "saving a truncated buffer destroys the tail" hazard (W1) **by construction**, because no full content is ever sent. That is why the 1 MB edit cap and the truncated-read refusal can be relaxed for T1, but only behind a separate flag (the cost of a bug is silent mangling of a big file).

### 4.5 Idempotency, ordering, partial failure

- **Idempotency**: the server keeps a bounded replay store per `(session, path)`: key `(client_id, seq)` -> `{payload_digest, outcome, new_token}`, 256 entries / 10 minutes. A replay with the same key and digest returns the recorded outcome without reapplying; same key, different digest is `InvalidArgument`. The store dies with the process; after a restart the existing "lost response" rule (plan Story 2.2.3: re-stat first) decides: in T0 the client knows `expect_result_sha256`, so "landed" is "on-disk sha equals it"; in T1 it knows only `expect_new_size`, so an unknown outcome after a server restart is resolved by Reload (documented limit).
- **Ordering**: `seq` must equal `last_acked_seq + 1` for `(client_id, path)`, else `OutOfOrder` (client bug or reordering); stop-and-wait makes it unreachable in practice.
- **Atomic per request**: all edits in one request apply or none do. Oversized logical edits are **not** split across requests (a half-applied logical edit is a corrupt file); limits below, and the client falls back to `writeFull` if the file is T0 or refuses with a clear message otherwise. A two-phase staged commit (`StagePatchChunk` x N then `CommitPatch`) is a possible later extension (open question 5), not in this design's first cut.

### 4.6 Verification (corrupt offsets must not silently mangle files)

Layered, cheapest first, all before `rename` so a failure leaves the original untouched:
1. **Base**: sha256 of the old file equals the token's hash (computed during the streaming read), else `Conflict`.
2. **Per-edit precondition**: `deleted_crc32c` of the text the client believes it is deleting; mismatch is `OffsetMismatch` (no write). CRC32C stdlib hardware path (INFERRED). 32 bits is a screen against off-by-N bugs, not an adversary defense (the actor is a trusted single user; the base sha256 is the identity check).
3. **Size**: resulting length must equal `expect_new_size`, else `ResultMismatch`.
4. **Region checksums**: crc32c of each edited region after applying equals the client's `region_crc32c`.
5. **T0 end-to-end**: when the client holds the whole document it sends `expect_result_sha256` of the final bytes (it applies the same EOL/BOM rules as the server) and the server compares the new-file hash before rename.
6. **Periodic reconcile** (T0): after every 20 patches or on blur, the client calls `StatFile{want_sha256}` and compares to its own hash; a mismatch marks the buffer `Diverged`, stops deltas for it and forces the next save through `writeFull` (the "full-content fallback checksum" the owner asked for).
7. **Response**: `new_token`, `new_size`, `new_line_count`, `applied_edits`, `verified: {size,regions,result_sha}` so the UI and logs show what was actually checked.

### 4.7 Conflicts and rebase

- Base token moved: `Conflict` with the current token. T0: reuse ADR-034 unchanged (`FullText` plus the client's `MergeBase` and `merge3`); the delta is not rebased, the *document* is. T1: `Conflict.theirs = Unavailable{TooLarge}`; options are Reload (keep the pending ChangeSet as a draft) or **Same-position rebase**: `PreflightFilePatch(new_base_token, edits)` verifies that each op's `deleted_crc32c` and a per-edit crc of the full touched lines still hold at the same line numbers in the new file; if all hold, the client adopts the new token and re-sends. If lines moved (an agent inserted above) it fails closed. This is deliberately not a diff-and-guess rebase.
- Rejections reuse the ADR-031 typed `ReadOnlyReason`/Connect-error mapping; new reasons: `OffsetMismatch`, `ResultMismatch`, `OutOfOrder`, `AnalysisPending`, `PatchTooLarge`.

### 4.8 Drafts and undo with deltas (ADR-032)

Store the **composed pending ChangeSet** (CM6 `ChangeSet.toJSON`) plus selection, keyed `(session, path, baseToken)`, instead of a snapshot. Size is proportional to the edit, not the file, so the 256 KB cap now bounds *edit volume*: a 900 KB file with a few edits is fully backed up where a snapshot was "Not backed up". Restore: if disk token equals `baseToken`, load the base and `ChangeSet.fromJSON`; ADR-032's rule that restored drafts have `MergeBase = Available(disk text)` is preserved. If the file changed, show only 2-way options (as ADR-032 does) with "Copy my edits as patch". Undo history is not persisted (same as snapshots today). INFERRED: replaying a ChangeSet needs the exact base length; `ChangeSet.fromJSON` validates lengths, so a mismatched base fails closed.

### 4.9 Save-as-you-type versus explicit Save

Keep explicit Save to disk (requirements; and because half-typed code written continuously breaks agent builds and file watchers). Deltas change the *cost* of a Save, not its trigger. A future optional "autosave on idle (default off)" would be a pure `SyncPolicy` over the same sync loop; not designed here.

### 4.10 Limits (replace 1.5 MiB for patch; keep for `writeFull`)

| Limit | Value (proposed) | Reason |
|---|---|---|
| `ApplyFilePatch` body | 2 MiB (path-scoped `MaxBytesReader`, same mechanism as ADR-031's cap) | bounds memory before decode |
| ops per request | 2,000 | O(ops) validation |
| total inserted text | 1 MiB | oversized paste: refuse or `writeFull` |
| `writeFull` body | 1.5 MiB (unchanged) | ADR-031 |
| editable file | 1 MiB (T0) default; up to 64 MiB under `files:edit-large` | W1 solved by construction for deltas |
| patch rate | 2/s per path, 1 large rewrite at a time process-wide | disk saturation |

## 5. Security and safety

- **Confinement is single-sourced**: `FileOps` calls the shared `resolveInWorktree` (ADR-031) for every wire including `http_range`; `.git` deny, symlink rules, `ActiveDir == ExistingDir` for writes, hidden-session guard, `files:editing` server-side enforcement are unchanged and tested once at `FileOps` plus a "every adapter reaches the same guard" test per wire.
- **Guards**: `ApplyFilePatch` and `PreflightFilePatch` join `guardedProcedures` with `ProfileRebinding` (VERIFIED mechanism). Read ops are not guarded today (INFERRED: the procedures VERIFIED in the map are mutating ones); a new `http_range` GET route would be the first GET that returns file bytes through a path that may not pass the Host/Origin verdict, so it must be registered with the rebinding guard (and `ServeFileRaw`'s existing posture needs the same check; open question 8). Remote tier: `:8444` capabilities omit `PATCH` until Task 0.1.3b.
- **Audit**: one slog line per patch: session, path, edits, bytes inserted/deleted, size before/after, outcome (`saved|conflict|offset_mismatch|result_mismatch|rejected`), adapter, duration; no content (matches the requirements' Observability section).
- **Analytics (ADR-035)**: new closed-enum labels only: `transport` (`full|connect_unary|connect_stream|ws_stream|http_range`), size/payload buckets (`lt1k..ge1m`), ops-count buckets, `fallback_reason`; events `files.transport.select`, `files.transport.fallback`, `files.read.window` (client, sampled), unthrottled server `files.server.patch` mirroring `files.server.write`. No paths, lines, offsets, text or queries.
- **DoS**: caps in 3.5/4.10; index and analysis builds under a semaphore (1-2) with ctx cancel and a CPU niceness equal to search workers; range concurrency per client 4, per IP 8 (extends ADR-030's limiter seam; the same remote-IP keying caveat applies); a response never exceeds 1 MiB; failed `OffsetMismatch` repeats are rate limited like conflicts.
- **Compression**: spike S-T1. If Connect-level gzip is not negotiated by connect-web, add per-RPC compression for range responses over 1 KiB (text typically compresses several-fold, INFERRED) rather than touching `/api/` bypass in `middleware.Compress`; WS adapter uses permessage-deflate if its library supports it (UNVERIFIED).
- **Delta-specific data-loss risks** and controls are 4.6; the principal residual is a client encoder bug that produces a valid but wrong edit with matching client-computed checksums (both sides derived from the same wrong position). Mitigation: the encoder is exercised by the property tests below against the independent full-replace path, and `files:delta-write` is a kill switch that routes saves back to `writeFull` instantly.

## 6. Adapters compared

| Adapter | Reads | Writes | Pros | Cons | Position |
|---|---|---|---|---|---|
| `full` | whole file, 1 MB cap | `WriteFile` | exactly today; always works | no large files; large payloads | mandatory floor (Wave A) |
| `connect_unary` | line/byte windows | `ApplyFilePatch` | same stack, auth tiers, guards, e2e; short-lived calls fit HTTP/1.1 | one RTT per window | **primary** |
| `connect_stream` | server-stream of line-aligned chunks | n/a (Connect browsers lack client streaming) | one request for sequential read-ahead | holds a connection; buffering risk seen in ADR-030's spike | optional, only if prefetch RTT misses target |
| `ws_stream` | server-stream over `watch-ws-transport` | n/a | avoids HTTP connection-pool pressure; reuses an existing, proven client transport | the bridge today carries `Watch*` streams only; remote/auth and proxy behavior untested | optional, gated by a probe |
| `http_range` | raw byte ranges, `206`, ETag = token | n/a | cacheable, works for media/binary | no lines, no verification; GET on the local listener needs the rebinding guard | byte-only consumers |

True bidirectional WS multiplexing (frame types, request ids, flow control) is **not** recommended: Connect browser clients have no client-streaming anyway, and unary patch plus pooled unary/streamed reads covers every operation measured here. Revisit if S-T5 shows connection-pool starvation.

## 7. Testing

- **Shared golden vectors** (`testdata/transport/*.json`, consumed by Go and TS tests): `{base bytes, eol, bom, edits, expected bytes}` including empty file, no trailing newline, CRLF, BOM, astral characters (surrogate pairs), combining marks, edits at 0 and EOF, adjacent edits, insert-only, delete-only, and invalid column (splits a surrogate pair -> rejected).
- **Property tests**: Go `FuzzApplyPatch` and TS fast-check: random base text and random ChangeSet; assert bytes(`applyPatch`) equals bytes(`writeFull(applyChanges(base))`), for LF, CRLF and BOM bases, in T0 (memory) and T1 (streaming splice) paths, and that no-op patches are byte-identical (ADR-031's no-op CRLF+BOM property). Fuzz Position/offset inputs: no panic, no partial write, temp cleaned up.
- **Adapter conformance suite**: `runTransportConformance(makeTransport)` (TS) and `FileOpsConformance(t, newOps)` (Go). Every adapter and backend must pass: capability honesty (advertised op works, unadvertised op is refused), range edges (BOF/EOF, UTF-8 boundary, CRLF/BOM, binary-after-8KB, long line), staleness (validator change drops cache; torn read retried), error mapping to typed reasons, abort/cancel, idempotent replay, out-of-order refusal, confinement/symlink/`.git` corpus (reuse Story 2.1.4's), body-limit refusals, fallback demotion. A new adapter lands only when the suite is green.
- **E2E**: Playwright matrix over `full` and `connect_unary` on desktop and mobile viewports with 50 MB and 5 MB fixtures (scroll, jump, search-hit open, window edit and save, conflict), heap and message-size assertions; `ws_stream` and `connect_stream` join the matrix if shipped.
- **Benchmarks** (run with `&` per repo rule): Go `-benchmem` for sha256 + index pass on 1/16/64/256 MB, `ReadRange` latency, streaming splice at 1/16/64 MiB (the 64 MiB ceiling decision); TS: window swap time, ChangeSet encode time for 2,000 ops, chunk cache memory on a phone-class budget; payload comparison full vs delta for a 1-character edit (expected at least two orders of magnitude smaller, INFERRED).
- **Fault injection**: kill between temp-write and rename; response dropped; duplicate request; 5-second stalls; disk-full; agent writing during the pass (torn read, changed base).

## 8. Plan impact (cost classes; no human time)

Cost classes use the plan's own token midpoints (XS 20k, S 55k, M 130k) and its 1.75x verify factor (plan "Effort estimate"); the dollar-free unit is CU. All INFERRED, recalibrate after Wave A.

### 8.1 Recommended phasing (integrated figures)

Cost classes use the plan's own token midpoints (XS 20k, S 55k, M 130k) and its 1.75x verify factor (plan "Effort estimate"); the dollar-free unit is CU (about 0.93 CU per raw token before verification, 1.6275 after). All INFERRED, recalibrate after Wave A.

| Phase | Content | Gate | Integrated tasks (class) | Raw tokens | CU | Top-down (original) |
|---|---|---|---|---|---|---|
| Spikes S-T1..S-T5 | compression and wire cost, hash/splice throughput, CM6 gap-line and window swap on desktop, WebKit and the owner's phone (extended 0.1.4), encoder prototype, connection-pool contention | Phase 0 (plan Wave 1); the S-T3 owner phone gate selects the engine | 6 (4S + 1M + 1XS): Tasks 0.1.4c/d, 0.1.7a/b, 0.1.8a/b | 370k | 0.60M | 5S, 275k, 0.48M |
| A. Seam | `FileOps`/`FileBackend`, capability RPC, TS `FileTransport` + `FullFileTransport` + router, hooks consume the port, conformance scaffold, flags/analytics wiring, engine ports and engine conformance | behavior-identical to the base plan; plan Wave 3 (and Wave 2 for the engine ports) | 10 (5S + 5M): Stories 0.2.6, 1.2.3, Tasks 0.2.5d, 1.2.1f/g | 925k | 1.51M | 4M + 2S, 630k, 1.10M |
| B. Range reads | line index + `Analyze`, `ReadFileRange`, `connect_unary` adapter + chunk cache + prefetch, `cm6LazyDoc`, windowed viewer, pager integration, large-file search + `byte_offset`, e2e, benchmarks, owner check | before editing (read-only value; plan Wave 6); flag `files:range-read` | 21 (7S + 13M + 1XS): Epic 1.6 | 2,095k | 3.41M | 6M + 4S, 1.0M, 1.75M (+0.8M for `cm6LazyDoc` in section 11.5) |
| C. Delta writes | patch proto, T0 in-memory patch, streaming splice (T1), idempotency + ordering, client encoder + sync loop, verification, ChangeSet drafts, preflight rebase, property/fuzz/golden corpus, limits/audit, UX states, e2e, docs | after Gate G1 (code) and Gate G2 (flag flips); flags `files:delta-write`, then `files:edit-large` (plan Waves 7-9) | 18 (8S + 10M): Stories 2.1.5, 2.2.5, 2.3.3, 2.5.5 | 1,740k | 2.83M | 7M + 3S, 1.075M, 1.88M |
| D. Optional adapters (owner-selected, included, last) | `connect_stream`, `ws_stream`, `http_range` + guard, `ServeFileRaw` audit, pool-contention handling, conformance runs | Gate G-D: S-T1/S-T5/1.6.7b recorded; they decide default enablement, not whether the code is built (plan Wave 10) | 8 (3S + 5M): Epic 2.6 | 815k | 1.33M | 3M + 2S, 0.5M, 0.97M |

**Integrated total: 63 tasks, 5.95M raw tokens, 9.68M CU (verified by script against the plan's effort table)**, taking the plan from 18.04M CU (133 tasks) to **27.71M CU (196 tasks), band XL**. The original top-down estimate was about 3.5M raw tokens and 6.2M CU for sections 8.1 (plus about 1.0M CU for the engine seam and lazy document, section 11.5, about 7.2M CU in all); the bottom-up total is about 2.5M CU higher (about 34%) because the work split into 63 tasks of 3-5 files against about 36 and because the owner included every adapter. The contingent custom engine (plan Epic 1.7, 26 tasks) is about **4.59M CU** at task level, not the "about 6M" of the first draft, and is excluded from every figure above. Wall-clock blockers: the S-T3 owner phone gate, the Task 0.1.3b remote-tier answer, the owner phone check of windowed reading (Task 1.6.7c), the large-edit dogfood (Task 2.5.5b) with Gates G2 and G3, benchmark runs on the real host, CI runs.

### 8.2 Edits needed in other artifacts (made on 2026-10-16; the plan's ids differ from the first draft: Epic 1.6 has Stories 1.6.1-1.6.7, the spikes are Stories 0.1.7/0.1.8 and Tasks 0.1.4c/0.1.4d, and Phase D is Epic 2.6)

- `plan.md`: add Story 0.2.6 (`FileOps` port, `GetFileTransportCapabilities`, flags); Story 1.2.3 (`FileTransport` port + `FullFileTransport` + router; `useOpenBuffers`/`useSaveBuffer`/`useBufferFreshness` take the port); new Epic 1.6 (range reads, windowed viewer); new Stories 2.1.5 (`ApplyFilePatch`), 2.2.5 (delta encoder + sync loop), 2.3.3 (delta conflicts, preflight), 2.5.5 (large-edit dogfood); revise Story 0.2.3 (`StatFile` gains `want_analysis`/`want_sha256`), 1.5.2 (degrade profile: windowing replaces degrade for files over 1 MiB on phones), 1.5.4 (pager sourced from `read(lines)`; "of about N" copy), 1.4.x/1.1.x (hit `byte_offset`, large-file search mode), 2.2.2 (drafts as ChangeSet), 2.2.3 (idempotency key and expected-size reconcile), 2.1.2f (cap applies to `writeFull`; new patch cap), 2.1.4 (delta corpus), 2.5.1 (gates, registry for new RPCs); extend the Wave/dependency graph and the effort table; add spikes S-T1..S-T5 to Epic 0.1.
- `ADR-031`: amend (not rewrite) editable-cap and wire sections to cite ADR-036; token format unchanged. `ADR-030`: per-file cap becomes request-scoped; hit shape gains `byte_offset`; limiter shared. `ADR-032`: `Buffer.Loaded.source`; drafts are ChangeSets; window-pinning rule. `ADR-034`: state that deltas are not rebased, only documents are; T1 same-position rebase. `ADR-035`: add the labels/events in section 5. ADR-033 untouched.
- `ux.md`: new states (Indexing/Preparing-to-edit chip, window-loading skeleton, "about N lines", "Edit this window", fallback chip, delta-diverged recovery, patch-too-large message, T2 view-only reason), mobile prefetch and memory notes, copy for new rejection reasons, a11y announcements for window loads.
- `validation.md`: new rows mapping the golden/property/conformance/benchmark/e2e items in section 7 to requirements (large-file open, jump latency, delta equality, no-tail-loss, fallback), a new pre-mortem entry for "wrong offsets with matching checksums".
- `requirements.md`: resolve the open item "Connect max message size vs full-file delivery for editing" (answered by ranges and patch caps); add a success metric for large files (open 50 MB at a hit line without exceeding a memory budget on the phone viewport).

### 8.3 Existing decisions obsoleted or reconciled

| Decision | Status under ADR-036 |
|---|---|
| ADR-031 "editable cap equals the 1 MB read; no relaxed-truncation edit path" | Kept for `full`/T0; superseded for T1 (delta-only, `files:edit-large`) |
| Plan truncated-read refusal (Truncated token `-`, edit refused) | Kept on the `full` adapter; windowed reads replace truncation for range-capable servers |
| ADR-031 1.5 MiB `WriteFile` cap | Kept for `writeFull`; `ApplyFilePatch` gets its own 2 MiB cap |
| Token `v1:<size>:<mtime_ns>:<sha256>` | Unchanged; lazy hash via analysis pass; optional `v2` hardening deferred |
| ADR-032 draft = bounded snapshot (<=256 KB) | Changed to ChangeSet drafts; 256 KB now bounds edit volume; snapshot used only if ChangeSet unavailable |
| ADR-030 2 MB per-file search cap | Becomes request-scoped (`large_files`) |
| Plan Story 1.5.2 600 KB degrade | Windowing covers files over 1 MiB; degrade still applies to T0 on coarse pointers |
| Requirements NFR "large files must not freeze the mobile browser" | Now met by serving them, not refusing them |

## 9. Spikes and open items

- **S-T1** (VERIFY first): do Connect unary responses on `/api/` get compressed (Connect-level gzip) from connect-web; what do range responses cost over LAN/Tailscale/`:8444`.
- **S-T2**: throughput of index+sha256 pass and streaming splice at 1/16/64/256 MB; decide `hash_cap_bytes`; 64 MiB scan cost for large-file search.
- **S-T3**: CM6 windowed doc (line-offset gutter, edge swap, anchor preservation, undo per window, iOS Safari).
- **S-T4**: ChangeSet-to-ops encoder prototype vs full-replace (property harness) before any server work depends on it.
- **S-T5**: connection-pool contention with prefetch + `SearchContent` + other tabs; decides whether `ws_stream`/`connect_stream` are built.

## 10. Open questions for the owner

*Answered by the owner on 2026-10-16 (ADR-036 Accepted): 1 yes, large-file editing is included (T1 delta-only behind `files:edit-large`); 3 the gap-line document relaxes the single editable window (window swap is the fallback); 7 yes, `ApplyFilePatch` is allowed on `:8444` subject to Task 0.1.3b; 8 yes, `http_range` ships behind the rebinding guard. Still default/open: 2 explicit Save only (kept); 4 the 64 MiB / 1 GiB constants are proposals pending S-T2; 5 oversized pastes are refused (T1) or fall back to `writeFull` (T0), no staged commit; 6 token `v1` kept.*

1. Is large-file **editing** (T1, up to 64 MiB, delta-only) wanted, or is large-file **viewing/search** (T1/T2) the goal with edits staying at 1 MiB? (Largest single scope lever; edits carry the data-loss risk.)
2. Confirm explicit Save only (no autosave to disk), with deltas used to shrink Save.
3. Windowed single-editable-window editing for files over 1 MiB: acceptable UX, or do you want an "open whole file (up to N MB) as a normal document" mode for 1-8 MB on desktop?
4. Edit/read ceilings: 64 MiB editable and 1 GiB readable acceptable (constants, tune after S-T2)?
5. Should oversized pastes be refused/fallback or supported by a two-phase staged commit?
6. Strengthen the read validator with `ctime_ns` + inode (token `v2`) or keep `v1` and rely on write-time hashing?
7. Should `:8444` (remote) accept `ApplyFilePatch`? (Depends on Task 0.1.3b; the capability design handles either answer.)
8. May the new GET `http_range` adapter ship at all, given the local listener's lack of auth? (Recommendation: only with the rebinding guard, otherwise skip it.)

## 11. Editor engine (owner decision 2026-10-16: the engine is not fixed)

Question: can the editor core support lazy (range-fetched) text and delta-shaped edits, or should the engine change? Evidence available in this session: VERIFIED `web-app/package.json` already depends on `@codemirror/{state,view,commands,language,search}` (6.x), `codemirror`, `@lezer/highlight`, `@monaco-editor/react` ^4.7.0 and `monaco-editor` ^0.55.1 (Monaco is imported by `web-app/src/app/config/ConfigPageContent.tsx:8` for the config editor, not the Files tab), and `@xterm/*`. `node_modules` is not installed in this worktree, so no engine claim below was run against installed code; every engine property is INFERRED from public APIs and documentation recalled from memory unless marked otherwise, and Spike S-T3 (extended) is the gate that turns them into facts.

### 11.1 The editor-facing interface (the seam that makes the engine swappable)

Whatever engine ships sits behind one interface, owned by Story 1.2.1's `CodeEditor` extraction (plan), so the Files tab never imports an engine:

```ts
interface EditorEngine {
  mount(el: HTMLElement, o: EngineOptions): EngineHandle;
}
interface EngineHandle {
  // document: a lazily-materialized virtual file
  setSource(src: VirtualDoc): void;            // {lineCount?, loadedRanges, fetch(range)}
  onNeedRange(cb: (r: LineRange) => void): void; // engine asks for text it must draw
  provideRange(r: LineRange, text: string, meta): void; // no history entry, no dirty
  evictRange(r: LineRange): void;              // only clean ranges
  // edits
  onEdits(cb: (e: EditBatch) => void): void;   // ordered splices, ABSOLUTE line/col_utf16, base-doc coords
  takePending(): PendingEdits;                 // composed since last ack (ChangeSet-equivalent)
  ack(newToken: string): void;
  // view
  reveal(line: number, col?: number): void; getViewport(): LineRange; setReadOnly(b: boolean): void;
  // capabilities the UI queries instead of assuming
  readonly features: { vim: boolean; mergeView: boolean; lazyDoc: boolean; irc: boolean /* IME ok */ };
  setMode(extensions: ModeConfig): void;       // wrap, font, highlight on/off, vim, theme
  focus(): void; destroy(): void;
}
```

`EditBatch` is the delta wire's `Edit[]` (absolute line/col_utf16) so `FileTransport.patch` never learns which engine produced it. A second narrow port, `Highlighter` (`tokenize(range, languageId) -> spans`), lets a non-CM engine reuse Lezer for syntax highlighting. `MergeSurface` (`show3Way(base, mine, theirs)`) is a third port so ADR-034 is not welded to `@codemirror/merge`. A shared contract test (`runEngineConformance(makeEngine)`: typing, IME composition does not dispatch, range provide/evict keeps scroll anchor, edits map to the right absolute positions across a gap, read-only, a11y name/role) gates any engine, mirroring the transport conformance suite.

### 11.2 Candidates

**(a) CodeMirror 6 with a lazy document.** CM6's `Text` is an immutable rope that must contain every line the state knows; there is no lazy leaf or custom Text subclass hook (INFERRED from the public API). That kills "true lazy `Text`" but not lazy *content*. Two feasible models:
- **a1. Window swap** (research 3.6, option C): one window per `EditorState`, `view.setState` at the edges, line-offset gutter, one editable window.
- **a2. Gap-line model** (new, recommended inside CM6): the document holds loaded lines plus **one short placeholder line per unloaded gap**, drawn by a block-replace decoration whose widget reports `estimatedHeight = gapLines * lineHeight` (INFERRED: `WidgetType.estimatedHeight` exists for block widgets and CM6 uses it for viewport height maps), so the scrollbar is proportional to the whole file. A `StateField` holds the gap table (placeholder doc position -> absolute line range). The gutter uses `lineNumbers({formatNumber})` reading that table. Loading a gap is a transaction replacing the placeholder with real text, annotated `addToHistory.of(false)` and `Transaction.remote`-style so it is neither undoable nor dirty; clean chunks far from the viewport are evicted back to placeholders the same way (`history` maps over non-history transactions, INFERRED). `EditorState.changeFilter` plus `atomicRanges` forbid user edits that touch a placeholder (INFERRED APIs). This keeps **one document, one undo history and many editable regions**, removing the single-editable-window restriction of option a1.
  - Hard parts (all INFERRED, all in S-T3/S-T4): scroll anchoring when a gap above the viewport loads (CM6 scroll snapshots / `scrollIntoView` effects; jitter risk on iOS); `pending` ChangeSet bookkeeping because chunk loads shift coordinates: handle by mapping `pending.map(loadChange)` and converting `fromA/toA` to absolute lines through the base document's gap table at flush; in-editor find (`@codemirror/search`) sees loaded text only, so project/large-file find goes to the server (already the plan: `SearchContent`); bracket matching and syntax trees see partial text (Lezer incremental parsing over a gap-riddled doc can mis-highlight around a gap; mitigation: highlighting off beyond 1 MiB, as Story 1.5.2 already does on phones).
- Mobile/IME: CM6 uses `contenteditable` with its own DOM-observer; mature on iOS/Android relative to alternatives (INFERRED, widely deployed); the plan's spikes 0.1.4/0.1.5 already target this. No dispatch during `view.composing` (ADR-032) is a rule we keep.
- A11y: virtualized DOM is unreadable by screen readers beyond the viewport; mitigated by the Accessible view (Story 1.5.4), which already pages from the data layer, not the engine (decision: keep).
- Vim: `@replit/codemirror-vim` is the existing choice (ADR-033, plan Story 2.4.1). Merge: `@codemirror/merge` two-way + `node-diff3` (ADR-034). Bundle: already in the bundle; lazy extensions. Maintenance: low-medium (one engine, one gap-table module); risk: medium (scroll/gap interplay, history mapping).

**(b) Monaco.** Already installed (VERIFIED) but only used on the config page. Text model is a piece tree (VS Code's text buffer; INFERRED) which copes with very large in-memory files better than CM6's rope and has "large file optimizations" that turn off tokenization/folding (INFERRED), so a *fully loaded* 50 MB file is plausible on desktop. It has no lazy-fetch model: the model must hold all text, so range fetch would still need the same placeholder/gap trick via view zones or hidden areas (`setHiddenAreas` is not public API, INFERRED) and a custom line-number provider (`lineNumbers` accepts a function, INFERRED). Mobile: Monaco's documentation states it is not supported in mobile browsers (INFERRED from memory of the README; not verifiable offline here); touch selection, IME and soft-keyboard viewport handling are the weak spots, and mobile is a primary requirement (owner's phone). A11y: has a real screen-reader mode (`accessibilitySupport`, renders a limited textarea window; INFERRED) which is a genuine plus but still windowed, so the Accessible view stays. Vim: `monaco-vim` third-party (maintenance unknown). Diff/merge: built-in two-way diff editor; the VS Code 3-way merge editor is workbench code, not in the npm package (INFERRED), so `merge3` UI would be custom. Bundle: several MB with workers (INFERRED; far above CM6) and already a lazy chunk. Maintenance: medium (heavy API, worker configuration); risk: **high on mobile**, medium elsewhere. Does not solve laziness, so it fails the owner's decisive criterion without the same hack as (a2) plus worse mobile.

**(c) Other engines.**
- **Ace**: line-array document, virtual rendering, old but proven; has a built-in vim keybinding and mobile support is mediocre (INFERRED); no lazy doc; 3-way merge absent; ecosystem shrinking. Offers nothing CM6 doesn't, loses vim fidelity and merge.
- **Lexical / ProseMirror-style**: rich-text/tree models (nodes per paragraph); excellent for documents, wrong for code (no code-oriented line model, huge node counts for large files). Lazy loading would still need per-block placeholders. Rejected.
- **Zed/Helix-style rope editors compiled to WASM**: ropes (ropey/crop) are the right data structure, but there is no off-the-shelf embeddable browser editor with IME/soft-keyboard, accessibility and selection handling; a rope in WASM would still need a DOM/canvas view, a text-input layer and an a11y tree, i.e. option (d) with a worse host. Large bundle and build complexity (Rust/WASM toolchain not in the web-app today). Rejected unless used purely as the *document model* inside (d).
- **xterm-style canvas viewers** (xterm.js is VERIFIED in the dependency list, for the terminal): canvas/WebGL rendering handles millions of lines of scrollback by design, a strong fit for a **read-only large-file viewer**; but canvas text has no native selection/IME/accessibility (xterm has an accessibility addon that mirrors the viewport, INFERRED) and no editing model. As a read-only T2 renderer it is possible but the pager already provides the accessible path and CM6 renders T2 windows adequately; not worth a second renderer. Rejected for editing; kept as a note.

**(d) Custom editor: piece-table/rope model, virtualized line renderer, Lezer for highlighting.** Fully controlled laziness: the document is `VirtualDoc` (chunks + piece table of original-vs-added buffers), the view renders only visible lines from fixed-height or measured rows, edits become pieces and map directly to `Edit{line,col}`, and the delta wire falls out of the model with no ChangeSet conversion. Costs: you must build and keep correct the hard 90 percent that CM6 provides: IME composition on iOS/Android (the notorious part; `contenteditable` versus hidden `textarea` input proxy, soft-keyboard viewport and selection handles), bidirectional text and wrapping with variable line heights, selection/caret rendering, accessibility (a11y tree for the viewport, announcements), clipboard, find, undo history, vim mode (`codemirror-vim` is CM-coupled, so vim would be rewritten or lose fidelity), and the 3-way merge view (`@codemirror/merge` is CM-coupled). Syntax highlighting can reuse Lezer parsers stand-alone (`@lezer/highlight` is already a dependency, VERIFIED) over visible ranges, so that part is cheap. Maintenance: **very high** and permanent; risk: very high, concentrated exactly in the mobile/IME area the owner prioritizes. Appropriate only if (a2) fails its spike.

### 11.3 Comparison

| Criterion | (a1) CM6 window | **(a2) CM6 gap-line** | (b) Monaco | (c) Ace/Lexical/WASM rope/xterm | (d) Custom rope + virtual renderer |
|---|---|---|---|---|---|
| Lazy-fetch fit | window only, one editable window | multi-region, one history; needs gap bookkeeping | no native; same hack as a2 | none (xterm read-only only) | native |
| Mobile / IME / soft keyboard | mature (INFERRED) | mature, scroll anchoring risk | weak (INFERRED) | Ace mixed; others unsuited | must be built; highest risk |
| Accessibility | Accessible view | Accessible view | built-in SR mode + still windowed | varies | must be built |
| Vim | existing (ADR-033) | existing | `monaco-vim` third-party | Ace built-in; others none | rewrite or lose |
| 3-way merge / diff | existing plan | existing plan | 2-way built-in; 3-way custom | absent | custom |
| Bundle | already in bundle | already in bundle | several MB (INFERRED) | small to large | small + Lezer |
| Maintenance (cost class) | S-M | **M** | M-L | L | **XL** |
| Risk | medium-low | **medium** | high (mobile) | high | very high |

### 11.4 Recommendation

**Keep CodeMirror 6 as the shipping engine, and make laziness an engine capability behind `EditorEngine`:**
1. **Now (Phase A):** CM6 behind `EditorEngine`/`Highlighter`/`MergeSurface` ports with `lazyDoc:false` (T0 only). The base plan's Story 1.2.1 builds exactly this boundary; the extra work is the interface shape and engine conformance suite.
2. **Phase B:** implement **(a2) gap-line** in a new engine module `cm6LazyDoc` (`StateField` gap table, `loadRange/evictRange` transactions, gutter mapping, `changeFilter`). Gate: extended S-T3 passes (scroll anchoring on desktop Chrome/Firefox and iOS WebKit; history intact across loads/evictions; pending-ChangeSet-to-absolute conversion matches the full-replace oracle; 50 MB virtual file stays within the phone memory budget). If it fails, fall back to **(a1) window swap**, then to pager-only for T2; neither requires UI changes because the interface is the same.
3. **Only if a1 and a2 both fail** their gates on the owner's phone: build **(d)** as an additional `EditorEngine` for T1/T2 (view-first, with Lezer highlighting) while CM6 remains the T0 editor; vim and merge for T1 would be absent until reimplemented, which the capability flags (`features.vim`, `features.mergeView`) express honestly in the UI. Monaco is not recommended for any tier (mobile, no laziness). Ace/Lexical/WASM rope/xterm are rejected.

Rationale: laziness is mostly a *data-layer* property (range fetch, gap table, absolute-coordinate edits), and CM6 already provides the expensive-to-build parts (IME, accessibility hooks, vim, merge) that are exactly the owner's mobile priorities; a custom engine buys native laziness at the price of rebuilding those. The interface makes the decision reversible at the cost of one adapter, not a rewrite.

### 11.5 Plan effects (stories to change; cost classes, same basis as 8.1)

| Story | Change |
|---|---|
| 1.2.1 `CodeEditor` extraction | Becomes the `EditorEngine` port + `CmEngine` adapter + `Highlighter` and `MergeSurface` ports + engine conformance suite (adds 1 M, 1 S); `FileContentViewer`/`FilesTab` import the port only |
| Extensions (`extensions/*` stubs split out of 1.2.1) | Become engine-internal modules of `CmEngine`; `features` flags surfaced to the UI; no UI imports of `@codemirror/*` outside the adapter (lint rule, mirrors `depguard` style) |
| 1.5.2 degrade profile | Expressed as engine `setMode`; windowing/gaps replace degrade for files over 1 MiB |
| 1.5.4 Accessible view | Unchanged in design; its data source moves from engine doc to `FileTransport.read(lines)` so it works for any engine and for unloaded ranges ("Lines 1-500 of about N") |
| 2.2.x editing | Edit batches come from `EditorEngine.onEdits/takePending` (absolute line/col), not from CM6 types, so Story 2.2.5's encoder is engine-neutral |
| 2.3.x merge (ADR-034) | `MergeSurface` port; `CmMergeSurface` wraps `@codemirror/merge` + `merge3`; T1 files use Reload/same-position rebase (no merge UI), so an engine without merge still ships |
| 2.4.1 vim | Capability-gated `features.vim`; `@replit/codemirror-vim` stays in `CmEngine`; a non-CM engine shows "Vim keys unavailable for this engine" |
| 0.1.4 state-swap spike | Extended to S-T3: add gap-line model, scroll anchoring on load above viewport, history mapping, iOS WebKit run; its outcome selects a1 vs a2 vs (d) |
| New | Story 1.6.4 `cm6LazyDoc` (3 M tasks plus conditional S Task 1.6.4d, 0.72M CU incl. verify, as integrated); contingent Epic 1.7 custom engine (26 tasks, about 4.59M CU at task level, only if both CM6 models fail; excluded from the base total) |

Delta to the section 8 total (original, top-down): +about 1.0M CU for the interface/conformance and `cm6LazyDoc`; in the integrated plan this is inside the Phase A and B rows above (Tasks 1.2.1f/g and Story 1.6.4), and the contingent custom engine stays excluded.
