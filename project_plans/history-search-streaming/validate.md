# History Search Streaming — Validation

## AC Checklist

| AC | Verification method | Status |
|----|---------------------|--------|
| 1. `StreamSearchClaudeHistory` streams results before full scan | integration test: mock history with 50 sessions, assert first `Send` happens < 100 ms after request | pending |
| 2. UI renders partial results progressively within 200 ms | manual test on a machine with 200+ sessions; screenshot "Searching…" vs first results | pending |
| 3. MCP `search_claude_history` unary path not broken | existing MCP integration tests pass unchanged | pending |
| 4. `IncrementalSync` not on search hot path | add OTel span to sync; trace shows no sync span on `SearchClaudeHistory` after Story 1 | pending |
| 5. `history_search_streaming` flag gates streaming path | test with flag=false → unary, flag=true → streaming | pending |
| 6. All existing search tests pass; new streaming + sync tests added | `make test ./server/services/...` + `cd web-app && bunx jest --no-coverage useHistoryFullTextSearch` | pending |

## Open Questions for Owner

1. **Sync interval**: 60 s default means a session created 1 minute ago won't
   show in search results. Is that acceptable, or should we trigger `SyncNow`
   from `CreateSession`?

2. **Index lock contention during full rebuild**: The background sync holds
   `e.mu.Lock()` for the duration of `buildIndexLocked`. For users with
   thousands of sessions, this can block concurrent searches for seconds. Should
   we add lock-yield intervals inside the rebuild loop, or is that a separate
   story?

3. **Streaming vs. single-shot**: The initial `StreamSearchClaudeHistory`
   implementation flushes all results in a single streaming message (since BM25
   is fast once the index is warm). Do you want true incremental results
   (chunked by score tier), or is "fast unblock + single batch" sufficient?

4. **`CodeUnavailable` before first sync**: Should the UI show a user-visible
   "Index warming up" message on cold start (first search ever after
   install/restart), or silently fall back to the unary path?

## Pre-implementation Checks

- [ ] `make proto-gen` succeeds after adding the new RPC and message.
- [ ] `make gazelle` clean after adding the new files.
- [ ] `server/services/search_service.go` gocognit/funlen gate: the streaming
      handler is a new method and should not push the existing file over limits.
      Split into `search_service_streaming.go` if needed.
- [ ] `docs/registry/features/history-search.json` updated; `make registry-generate` clean.
- [ ] The streaming handler is registered in `server/server.go` (ConnectRPC
      `mux.Handle`).
- [ ] `noarchivedrevival` and `norawghrequest` analyzers: neither applies to this
      change (no session lifecycle, no GitHub HTTP calls).
