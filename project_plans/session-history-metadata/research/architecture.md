# Architecture Research: session-history-metadata

Feature: extract GitHub PR URLs, commit SHAs, and external URLs from Claude Code JSONL
conversation files associated with sessions. Store in DB, expose via API, feed PR status
poller. Run asynchronously and incrementally.

---

## Current Infrastructure Map

Before diving into options, here is what already exists and can be reused:

| Component | File | Relevant API |
|---|---|---|
| `HistoryLinker` | `session/history_linker.go` | `RegisterFileCallback(func(filePath string))` — fires on every JSONL CREATE/WRITE event and on startup scan |
| `HistoryFileWatcher` | `session/history_watcher.go` | fsnotify on `~/.claude/projects/`; triggers `HistoryLinker.ScanAll()` + all registered callbacks |
| `PRStatusPoller` | `session/pr_status_poller.go` | `SetInstances`, `AddInstance`, `RemoveInstance`; reads `inst.GitHubOwner/Repo/PRNumber` |
| `TokenStore` | `session/tokens/store.go` | **Reference implementation** for "file → worker pool → cached result" pattern; uses `RegisterFileCallback` via `OnHistoryFileChanged` |
| `tokens.Parser` | `session/tokens/parser.go` | `ParseFile(filePath) (*ParseResult, error)` — bufio.Scanner with 10 MB token budget; skips malformed lines |
| `ent/schema/session.go` | DB schema | Has `github_pr_url TEXT`, `github_pr_number INT` already on Session table |
| `pkg/events/types.go` | Event bus | `EventSessionUpdated` with `UpdatedFields []string` propagates to `WatchSessions` stream |
| `server/dependencies.go:788` | Wiring | Shows the `historyLinker.RegisterFileCallback(tokenStore.OnHistoryFileChanged)` pattern |

---

## Question 1: Where should the JSONL scanner hook in?

### Option A: `RegisterFileCallback` on HistoryLinker (recommended)

`HistoryLinker` already provides a `RegisterFileCallback(func(filePath string))` mechanism
used by `TokenStore`. The callback fires:
- On every fsnotify WRITE/CREATE event to any `.jsonl` file under `~/.claude/projects/`
- On startup ScanAll (which re-checks all sessions)
- When `ScanAll()` is force-triggered by a new JSONL create

**Tradeoffs:**
- Pro: Zero new infrastructure. Identical to how `TokenStore` is wired. One watcher serves
  all consumers. Avoids spawning additional fsnotify file descriptors.
- Pro: Fires immediately on append (fsnotify WRITE), enabling near-real-time extraction.
- Pro: The callback receives `filePath` directly — no need to reverse-map session → file.
- Con: Callback is file-path-scoped, not session-scoped. Correlating the file path back to
  a session title requires looking up which session owns that JSONL UUID. This is the same
  problem `TokenStore` faces and solves via `tokens.Associator` (reads from storage).
- Con: Callback fires per-file even when the session is not yet linked (i.e., the file
  may not be associated with any known session yet at the moment of the callback). This
  is acceptable — the extractor should store results keyed by JSONL UUID, then lazily
  associate to session when persisting.

**Verdict: Use `RegisterFileCallback`.** This is the established, tested pattern. Implement
an `ArtifactExtractor` struct with an `OnHistoryFileChanged(filePath string)` method, wired
in `server/dependencies.go` with a single `historyLinker.RegisterFileCallback(extractor.OnHistoryFileChanged)` line.

### Option B: Hook `HistoryLinker.notifyLinked()` directly

There is no `notifyLinked()` method. The closest analog is `correlateSession`, which calls
`inst.SetHistoryInfo()` when a UUID is first found. You would need to add a new callback
slot to HistoryLinker for the "just linked" event.

**Tradeoffs:**
- Pro: Fires exactly once per session link — avoids redundant work on WRITE events.
- Con: Requires modifying `HistoryLinker` internals. The link event fires only when the
  session is first associated; subsequent WRITE appends (new tool calls, commits) would
  not trigger re-scanning. This is insufficient for incremental extraction.
- Con: More invasive change with higher test surface.

**Verdict: Do not use.** It only works for initial linkage, not incremental extraction.

### Option C: New dedicated goroutine per session

Spawn a goroutine per session that `tail -f`s the JSONL file from the last known byte
offset.

**Tradeoffs:**
- Pro: Clean per-session isolation; easy incremental offset tracking.
- Con: N goroutines for N sessions. `PRStatusPoller` already demonstrated the "shared
  ticker" model is better for fleet-wide operations. `HistoryLinker` uses the same pattern.
- Con: Goroutine lifetime management (create on link, kill on delete/archive) adds
  significant complexity and risk of goroutine leaks.
- Con: Duplicates the fsnotify watcher infrastructure that `HistoryFileWatcher` already
  manages.

**Verdict: Do not use.**

### Option D: Shared background scanner (like PRStatusPoller)

A time-based ticker polls all known JSONL files every N seconds.

**Tradeoffs:**
- Pro: Simple, no fsnotify dependency.
- Con: Latency: up to N seconds before new URLs are extracted after being written.
- Con: Unnecessary work on idle sessions. `PRStatusPoller` shows this model works but
  requires rate limiting, backoff, and concurrency control — all of which `RegisterFileCallback`
  avoids by being event-driven.

**Verdict: Use as a fallback startup scan only.** The extractor should run a full scan on
startup (like `TokenStore.walkAndEnqueue`) to backfill existing files, then rely on
`RegisterFileCallback` for incremental updates.

---

## Question 2: Where should extracted artifacts be stored?

### Option A: New ent fields on Session table (one per artifact type)

Add columns: `extracted_pr_url TEXT`, `extracted_commit_shas TEXT` (JSON array),
`extracted_external_urls TEXT` (JSON array), `artifact_scan_offset INT64`.

**Tradeoffs:**
- Pro: Typed, queryable. You can `WHERE extracted_pr_url != ''` in storage queries.
- Pro: Adding `extracted_pr_url` alongside the existing `github_pr_url` is natural —
  `github_pr_url` is set at session creation from a GitHub URL input; `extracted_pr_url`
  is set asynchronously from JSONL scanning.
- Con: Schema migration required for each new artifact type. Commit SHAs and external
  URLs cannot be added as TEXT columns without either JSON encoding (losing queryability)
  or introducing separate junction tables.
- Con: The `session.go` ent schema is already 119 lines; adding 3-4 more fields is not
  disqualifying but is worth noting.

### Option B: Single JSON blob field `session_artifacts TEXT` (recommended)

Add one field to the Session ent schema:

```go
field.String("session_artifacts").
    Optional().
    Comment("JSON-encoded SessionArtifacts blob: pr_urls, commit_shas, external_urls, scan_offset_bytes.")
```

Deserializes to a typed Go struct in the application layer:

```go
type SessionArtifacts struct {
    PRURLs           []string `json:"pr_urls"`
    CommitSHAs       []string `json:"commit_shas"`
    ExternalURLs     []string `json:"external_urls"`
    ScanOffsetBytes  int64    `json:"scan_offset_bytes"`
    LastScannedAt    time.Time `json:"last_scanned_at"`
}
```

**Tradeoffs:**
- Pro: Single migration. Forward-compatible: add new artifact types to the Go struct
  without a schema migration.
- Pro: Follows the precedent already set by `ClaudeSessionData.Metadata map[string]string`
  for flexible extension.
- Pro: For the immediate use case (feed PR URL to `PRStatusPoller`), only `pr_urls[0]`
  is needed — easily accessed after deserialization.
- Con: Cannot query individual artifact fields in SQL without JSON functions. However,
  this is acceptable because: (a) artifacts are per-session, not cross-session aggregates;
  (b) the primary consumer is the frontend panel (read by session) and `PRStatusPoller`
  (reads from the in-memory `Instance`).
- Con: Requires care to avoid unbounded growth of `external_urls`. Cap at 50 entries or
  deduplicate by domain.

### Option C: Separate `session_artifact` ent entity

A junction table: `session_id TEXT, artifact_type TEXT, artifact_value TEXT, created_at`.

**Tradeoffs:**
- Pro: Fully normalized. Can query all sessions that ever mentioned a given PR URL.
- Pro: Supports pagination and filtering in SQL.
- Con: Highest migration complexity. Requires a new ent schema file, edge definition on
  Session, and generated ent code.
- Con: Overkill for the stated use case. The feature's primary goal is "show in frontend
  panel" and "feed PR status poller" — both of which are per-session reads.
- Con: Write amplification: every JSONL append that yields new artifacts requires an
  upsert into a separate table.

**Verdict: Option B (single JSON blob) for V1.** If future requirements need cross-session
artifact queries (e.g., "all sessions that pushed PR #123"), migrate to Option C then.

The scan offset (`ScanOffsetBytes`) stored inside the blob is critical for incremental
scanning and belongs in the same blob.

---

## Question 3: ConnectRPC API surface

### Current session state in `GetSession` / `WatchSessions`

The `Session` proto message (`proto/session/v1/types.proto`) already carries `github_pr_url`
(field 26) and `github_pr_number` (field 25). These are set at session creation time from
GitHub URL input, not from JSONL scanning.

The `WatchSessions` RPC streams `SessionEvent` messages with `updatedFields` — whenever
`EventSessionUpdated` is published with `UpdatedFields: []string{"github_pr_url"}`, all
connected `WatchSessions` clients receive the update.

### Recommendation: Extend `GetSession` response + `WatchSessions` event; no new RPC

**Option A: Include artifacts in `GetSession` response (recommended)**

Add a new proto message:

```protobuf
message SessionArtifacts {
  repeated string pr_urls = 1;
  repeated string commit_shas = 2;
  repeated string external_urls = 3;
  google.protobuf.Timestamp last_scanned_at = 4;
}
```

Add it to the `Session` message as `optional SessionArtifacts artifacts = <next_field>`.

**Tradeoffs:**
- Pro: Zero new RPC. Frontend panel uses `GetSession` on load, `WatchSessions` for live
  updates. No polling needed.
- Pro: Consistent with how `DiffStats`, `GitWorktree`, and `ClaudeSession` are embedded
  in the `Session` proto — all are sub-messages that are null when not populated.
- Pro: When the extractor runs and persists new artifacts, it publishes
  `EventSessionUpdated` with `UpdatedFields: []string{"artifacts"}`. The `WatchSessions`
  handler already translates `EventSessionUpdated` into a `SessionEvent` streamed to all
  clients — no new plumbing.
- Con: Adds bytes to every `GetSession` response. Acceptable — most fields will be empty
  (nil proto) for sessions with no JSONL file.

**Option B: Separate `GetSessionArtifacts` RPC**

A dedicated `GetSessionArtifacts(session_id)` → `SessionArtifacts` RPC.

**Tradeoffs:**
- Pro: Smaller `GetSession` payload; artifact data fetched on demand.
- Con: Frontend needs a second RPC call to populate the artifacts panel. This adds RTT
  and requires coordinated loading states.
- Con: Artifacts are not pushed via `WatchSessions` — frontend must poll or re-fetch.

**Option C: Stream via existing `WatchSessions` event bus only**

No `GetSession` change. Frontend subscribes to `WatchSessions` and receives the initial
state when the stream is established.

**Tradeoffs:**
- Con: `WatchSessions` currently sends the full session snapshot as the first event only
  on stream establishment; adding artifact data to `SessionEvent` would require the
  frontend to have an active `WatchSessions` subscription before artifacts arrive.
  This is fragile for cold loads.

**Verdict: Option A (embed in `Session` proto + `WatchSessions` push).** Consistent with
existing sub-message patterns (DiffStats, ClaudeSession). The frontend artifact panel
fetches from `GetSession` on load and receives live updates via `WatchSessions`.

---

## Question 4: Incremental scanning

**Recommended approach: byte offset tracking stored in `SessionArtifacts.ScanOffsetBytes`**

When `OnHistoryFileChanged(filePath)` fires:
1. Read `ScanOffsetBytes` from the cached `SessionArtifacts` for this file (keyed by
   conversation UUID, derived from the filename as `tokens.Parser` does).
2. `os.Open` → `f.Seek(ScanOffsetBytes, io.SeekStart)` → scan only new lines.
3. After scanning, update `ScanOffsetBytes` to the new file size (via `f.Stat().Size()`
   after the seek to avoid TOCTOU, or record the actual bytes read).
4. Merge newly found artifacts into existing deduped sets and persist the updated blob.

**Why not mtime / file-size check?**

mtime checks tell you the file changed but not where. File-size comparison alone tells you
it grew by N bytes, but you still need an offset to seek to. Byte offsets are the minimal
sufficient state.

**Why not line count delta?**

JSONL lines vary enormously in size (tool_use blocks with base64 can be several MB per
line). Counting lines requires reading them. Offsets are O(1) seek.

**Worker pool architecture (follow TokenStore pattern):**

```
OnHistoryFileChanged(filePath)
    → enqueue(filePath) onto buffered channel (size 256)
    → worker pool (4 goroutines)
        → extractFromFile(filePath, cachedOffset)
        → merge artifacts
        → persist to DB
        → publish EventSessionUpdated("artifacts")
```

The in-flight deduplication map (`sync.Map` keyed by filePath) from `TokenStore` should
be copied — it prevents the same file from being queued multiple times when multiple WRITE
events fire in rapid succession (common during active Claude sessions).

**Startup backfill:**

On `Start(ctx)`, walk `~/.claude/projects/` and enqueue all `.jsonl` files with
`ScanOffsetBytes = 0`. For files already in the DB with a non-zero offset, enqueue with
the stored offset (skipping already-scanned content). This matches `TokenStore.walkAndEnqueue`.

---

## Question 5: Integration with PRStatusPoller

**Recommended: Store in `Instance` fields + update via `Storage`; `PRStatusPoller` reads
from `Instance` as it already does.**

When the extractor finds a GitHub PR URL (e.g., `https://github.com/owner/repo/pull/123`):

1. Parse the URL to extract `owner`, `repo`, `prNumber`.
2. Call `storage.UpdateExtractedPRURL(sessionTitle, prURL, prNumber)` — a new storage
   method that:
   a. Persists the artifact blob.
   b. Sets `inst.GitHubOwner`, `inst.GitHubRepo`, `inst.GitHubPRNumber`, `inst.GitHubPRURL`
      on the in-memory `Instance` (same fields that `PRStatusPoller` reads from).
3. `PRStatusPoller` picks up the change on its next tick (≤60s) because it reads
   `inst.GitHubOwner/Repo/PRNumber` at poll time.

**Why not a direct call to `PRStatusPoller`?**

`PRStatusPoller` could be injected into the extractor and `TriggerImmediate(inst)` called
when a PR URL is found. However:
- This creates a dependency cycle risk (extractor → poller, poller → storage).
- The 60s latency before the poller's next tick is acceptable for PR status display.
- If immediate polling is desired later, an `onPRURLExtracted` callback on `PRStatusPoller`
  (similar to `onUpdated`) can be added incrementally.

**Why not via EventBus?**

An `EventPRURLExtracted` event type could be published and `PRStatusPoller` could subscribe.
This adds indirection without benefit — the poller already polls all sessions on a timer.
The simpler path (write to Instance fields → poller reads them) is correct here.

---

## Summary Architectural Decisions

### 1. Scanner hook: `RegisterFileCallback` on `HistoryLinker`

Implement `ArtifactExtractor` with an `OnHistoryFileChanged(filePath string)` method.
Wire it in `server/dependencies.go` with
`historyLinker.RegisterFileCallback(artifactExtractor.OnHistoryFileChanged)`.
Follow `TokenStore`'s worker pool pattern (buffered channel + 4 workers + in-flight dedup).
Run a startup backfill walk. This reuses the existing fsnotify infrastructure with zero
new watchers.

### 2. Storage: single JSON blob field `session_artifacts TEXT` on the Session ent schema

One ent migration. `SessionArtifacts` Go struct holds `PRURLs`, `CommitSHAs`,
`ExternalURLs`, `ScanOffsetBytes`, `LastScannedAt`. Incremental scanning uses
`ScanOffsetBytes` as a seek position (byte offset into the JSONL file). When a PR URL is
extracted, also backfill `Instance.GitHubOwner/Repo/PRNumber` for `PRStatusPoller`.
Cap `ExternalURLs` at 50 deduplicated entries to bound blob size.

### 3. API: embed `SessionArtifacts` sub-message in `Session` proto + push via `WatchSessions`

No new RPC. Add `SessionArtifacts` as a proto sub-message embedded in `Session` (next
available field number after 35+). Extractor publishes `EventSessionUpdated` with
`UpdatedFields: []string{"artifacts"}` after each successful scan. Frontend artifact panel
reads from `GetSession` on load and receives live updates via the existing `WatchSessions`
stream — consistent with `DiffStats`, `ClaudeSession`, and PR status fields.

---

## Implementation File List

| File | Change |
|---|---|
| `session/ent/schema/session.go` | Add `session_artifacts TEXT` field |
| `session/artifact_extractor.go` | New: `ArtifactExtractor` struct, worker pool, JSONL scanning |
| `session/artifact_types.go` | New: `SessionArtifacts` Go struct, URL regex patterns |
| `session/storage.go` | Add `UpdateSessionArtifacts(title, artifacts)` method |
| `session/ent_repository.go` | Implement `UpdateSessionArtifacts` against ent |
| `proto/session/v1/types.proto` | Add `SessionArtifacts` message, embed in `Session` |
| `server/adapters/` | Map `SessionArtifacts` → proto sub-message in `ToProto` |
| `server/dependencies.go` | Wire `historyLinker.RegisterFileCallback(extractor.OnHistoryFileChanged)` |
| `server/services/session_service.go` | Populate `artifacts` field in `GetSession` response |
| `web-app/src/components/sessions/SessionArtifactsPanel.tsx` | New frontend panel |
