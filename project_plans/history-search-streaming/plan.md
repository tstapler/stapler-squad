# History Search Streaming — Implementation Plan

## Summary

Decouple `IncrementalSync` from the search hot path by running it in a background
goroutine. Add a server-streaming RPC `StreamSearchClaudeHistory` that queries the
warm BM25 index immediately and flushes result batches to the browser. Front the
streaming call behind a feature flag and update `useHistoryFullTextSearch` to consume
the stream.

---

## Story 1 — Background incremental sync

### 1.1  Add sync loop to `SearchService`

**File**: `server/services/search_service.go`

Add a `startSyncLoop(ctx context.Context)` method and a `syncLoopInterval` constant
(default 60s, configurable via feature-flag override). The loop:
1. Calls `getOrRefreshHistoryCache` to get the latest history state.
2. Calls `ss.searchEngine.IncrementalSync(hist)` and logs the result.
3. Sleeps for `syncLoopInterval`; respects context cancellation.

Expose `SyncNow() error` for testing.

**Wire**: `server/dependencies_llm.go` starts the loop via
`go ss.startSyncLoop(ctx)` after construction, before returning the service.

### 1.2  Remove `IncrementalSync` from `SearchClaudeHistory`

In `SearchClaudeHistory` (lines 545–562 of `search_service.go`), remove the
synchronous `IncrementalSync` call. Replace it with a non-blocking check:
if `ss.searchEngine.SyncMetadata()` is nil (never synced yet), return
`connect.CodeUnavailable` with the message "index is warming up, retry shortly".
This guards against a race where the loop hasn't run once yet.

### 1.3  Add `SyncMetadata()` accessor to `SearchEngine`

**File**: `session/search/engine.go`

Already has `GetSyncMetadata() *IndexSyncMetadata` (line 753). No change needed —
use it in the guard above.

---

## Story 2 — `StreamSearchClaudeHistory` proto + handler

### 2.1  Add streaming RPC to proto

**File**: `proto/session/v1/session.proto`

```proto
// StreamSearchClaudeHistory streams Claude conversation search results as they
// are scored, using the same request message as SearchClaudeHistory.
rpc StreamSearchClaudeHistory(SearchClaudeHistoryRequest)
    returns (stream SearchClaudeHistoryStreamResponse) {}
```

Add `SearchClaudeHistoryStreamResponse`:

```proto
message SearchClaudeHistoryStreamResponse {
  repeated SearchResult results = 1;
  int32 total_matches = 2;
  int64 query_time_ms = 3;
  bool has_more = 4;
  // is_final = true on the last message so the client knows the stream is done.
  bool is_final = 5;
}
```

Run `make proto-gen`. Run `make gazelle` and add to `docs/registry/features/`.

### 2.2  Implement `StreamSearchClaudeHistory` handler

**File**: `server/services/search_service.go` (or a new
`search_service_streaming.go` if the method would exceed `funlen`).

```go
func (ss *SearchService) StreamSearchClaudeHistory(
    ctx context.Context,
    req *connect.Request[sessionv1.SearchClaudeHistoryRequest],
    stream *connect.ServerStream[sessionv1.SearchClaudeHistoryStreamResponse],
) error
```

Implementation:
1. Validate the request (same logic as `SearchClaudeHistory`).
2. Guard: if `ss.searchEngine.GetSyncMetadata() == nil`, return
   `connect.CodeUnavailable`.
3. Load history cache (fast path — no sync).
4. Call `ss.searchEngine.Search(query, opts)` — in-memory BM25, typically < 10 ms.
5. Enrich results (snippets, metadata) in the same loop as `SearchClaudeHistory`.
6. Stream one message containing all results with `is_final = true`.
   (Future: for very large result sets, chunk into batches of 10–20 and flush
   with `is_final = false` on all but the last. Leave the chunking TODO for now;
   the first version just eliminates the sync latency.)

Register in `server/server.go` alongside `SearchClaudeHistory`.
Add to `docs/registry/features/history-search.json` and run `make registry-generate`.

---

## Story 3 — Feature flag

### 3.1  Add `history_search_streaming` flag

**File**: `server/features/history.go` (or a new file following the pattern in
that package).

```go
var HistorySearchStreaming = FeatureFlag{
    Name:        "history_search_streaming",
    Description: "Use StreamSearchClaudeHistory server-streaming RPC instead of unary SearchClaudeHistory",
    Default:     false,
}
```

The flag is live-settable via the existing feature-flag RPC; no env-var path.

---

## Story 4 — Frontend streaming consumer

### 4.1  Update `useHistoryFullTextSearch`

**File**: `web-app/src/lib/hooks/useHistoryFullTextSearch.ts`

When `history_search_streaming` is enabled (read from `useFeatureFlags`):

```ts
// In search():
for await (const msg of clientRef.current.streamSearchClaudeHistory({...})) {
  setState(prev => ({
    ...prev,
    results: [...prev.results, ...msg.results.map(convertResult)],
    totalMatches: msg.totalMatches,
    hasMore: msg.hasMore,
    loading: !msg.isFinal,
  }));
  if (msg.isFinal) break;
}
```

Abort on query change (same `AbortController` pattern already in the hook).
Fall back to the unary path when the flag is off.

### 4.2  Update the loading indicator in `HistorySearchResults`

**File**: `web-app/src/components/history/HistorySearchResults.tsx`

Change the "Searching conversations…" spinner: show results as they arrive
(even a partial batch) and change the spinner to a subtle progress indicator
while `loading && results.length > 0`.

---

## Story 5 — MCP compatibility

The MCP `search_claude_history` tool (`server/mcp/tools_history.go`) calls
`SearchClaudeHistory` (unary). It is **not** changed — it stays on the unary
path. The unary handler now skips the synchronous `IncrementalSync` too (after
Story 1.2), so it also benefits from the background sync.

---

## Story 6 — Tests

### 6.1  Background sync loop unit test
`server/services/search_service_test.go` — verify `SyncNow` indexes new sessions;
verify `SearchClaudeHistory` returns `CodeUnavailable` before the first sync.

### 6.2  Streaming handler test
`server/services/search_service_streaming_test.go` — verify the stream emits
at least one message and the last message has `is_final = true`.

### 6.3  Frontend hook test
`web-app/src/lib/hooks/useHistoryFullTextSearch.test.ts` — mock the streaming
client and assert that `results` updates progressively as messages arrive.

---

## Adversarial Review Pass

### Risk 1 — Race: index not warm on first search

Mitigated by Story 1.2's `CodeUnavailable` guard. The UI should catch this code
and display "Index warming up, please retry in a moment" rather than a generic
error spinner. Add this to the frontend error handler in Story 4.1.

### Risk 2 — Background sync holds the engine write lock too long

`IncrementalSync` holds `e.mu.Lock()` for the entire sync, which blocks
concurrent `Search` calls. For large history sets (1000+ sessions), a full
rebuild can take seconds and block the search path. Mitigation: add a per-session
lock-release-and-reacquire in `buildIndexLocked` every N sessions (a "yield"
interval), or run the file reads outside the lock and only lock for the
`AddDocument` calls. File this as a follow-up TODO rather than blocking this
story — the background loop removes sync from the hot path, so the lock contention
is no longer user-visible.

### Risk 3 — Stale results after a new session is created

Background sync runs every 60 s by default. A just-created session won't appear
in search for up to 60 s. Mitigation: trigger `SyncNow` from the `CreateSession`
completion path (a port call, not a direct dependency). Or simply document the
eventual-consistency window. Acceptable for V1.

### Risk 4 — Proto change registries

New RPC must appear in `docs/registry/features/history-search.json` and
`web-app/src/app/route-meta.ts` (not applicable here — no new page). Missing
registry entry will fail `make registry-generate`'s CI check.

### Risk 5 — Streaming + offset pagination incompatible

`SearchClaudeHistory` already rejects `offset > 0` with `group_by_session` etc.
The streaming handler inherits the same validation. The initial implementation
does not support cursor-based "load more" for streaming results (the client just
calls the stream again with a new query or higher offset). Document this limit.
