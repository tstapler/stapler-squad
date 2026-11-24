# History Search Streaming — Research

## Codebase Findings

### The bottleneck: `IncrementalSync` on every search request

`server/services/search_service.go:SearchClaudeHistory` (line 511) calls
`ss.searchEngine.IncrementalSync(hist)` synchronously on **every search RPC**
before running any BM25 query. `IncrementalSync`
(`session/search/engine.go:458`) does:

- **Cold path** (`ShouldRebuildLocked` or no metadata): calls `buildIndexLocked`,
  which loops over every session in history and calls
  `history.GetMessagesFromConversationFile(entry.ID, 0)` — a filesystem read of
  each session's JSONL file under `~/.claude/projects/<hash>/conversations/`.
  For a user with 100+ sessions this easily takes 2–10 seconds.

- **Warm incremental path**: still reads the JSONL file for each
  *new or updated* session. After a cache TTL miss (default set via
  `historyCacheTTL`), all sessions with a changed `UpdatedAt` are re-indexed.

The in-memory BM25 `Search()` itself is O(posting list length × terms) — typically
< 10 ms once the index is built.

### The existing unary RPC

```proto
// session.proto line 100
rpc SearchClaudeHistory(SearchClaudeHistoryRequest) returns (SearchClaudeHistoryResponse) {}
```

This is an ordinary unary RPC; the browser must wait for the full response.

### Streaming precedent in this codebase

`SearchContent` already uses server-streaming:

```proto
// session.proto line 371
rpc SearchContent(SearchContentRequest) returns (stream SearchContentResponse) {}
```

Its handler (`server/services/content_search_handler.go`) uses
`connect.ServerStream` and flushes partial batches every
`searchBatchInterval = 100 ms`. The frontend consumes it via an async for-loop.
This is the established pattern to follow.

### `EnableLazyLoad` and background warm-up

`SearchEngine.EnableLazyLoad` (`engine.go:95`) defers persisted-index loading to
the first query. There is currently no background goroutine that runs
`IncrementalSync` proactively. The composition root
(`server/dependencies_llm.go` / `server/dependencies.go`) wires the engine but
does not start a background sync loop.

### Frontend hook: `useHistoryFullTextSearch`

`web-app/src/lib/hooks/useHistoryFullTextSearch.ts` calls
`clientRef.current.searchClaudeHistory({...})` — a standard `await` unary call.
ConnectRPC's generated TypeScript client (`@connectrpc/connect`) supports both
unary and server-streaming calls; the streaming variant is a for-await loop.

### Existing streaming consumer example

`web-app/src/components/sessions/SearchResultsList.tsx` (content search) already
consumes `SearchContent` via streaming and appends batches as they arrive.

### Feature flags

`server/features/history.go` and `server/features/*.go` show the established flag
pattern: a `FeatureFlag` struct registered at startup, read per-request.

## Options Considered

### Option A — Background sync + streaming RPC (recommended)

1. Start a background goroutine in `SearchService` that runs `IncrementalSync`
   on startup and whenever the history cache is refreshed.
2. Add `rpc StreamSearchClaudeHistory(SearchClaudeHistoryRequest) returns
   (stream SearchClaudeHistoryStreamResponse) {}` to `session.proto`.
3. The streaming handler skips `IncrementalSync` (the background goroutine owns
   that) and streams BM25 results from the warm index immediately.
4. The frontend switches to the streaming call behind `history_search_streaming`.

**Why**: Completely decouples index freshness from request latency. Streaming is
the right transport even when the backend is fast: it unblocks the browser's
render pipeline earlier and handles cancellation cleanly.

### Option B — Early-result flush inside the existing unary RPC

Change `SearchClaudeHistory` to run BM25 against the *current* index state before
`IncrementalSync` completes, embed a "preliminary" flag in the response, and have
the UI poll for the final result.

**Why not**: Two-phase polling adds client complexity. The "preliminary" vs "final"
result UX is confusing. Does not reduce server-side latency at all.

### Option C — Raise the history cache TTL

A longer TTL means more searches hit the warm cache, so `IncrementalSync` runs
less often. Easy, but it means stale results for new sessions and doesn't help on
cold start or the first search after restart.

**Why not**: Treats the symptom. Combined with Option A as a secondary knob.

## Decision

**Option A.** Background sync decouples the slow I/O from the search hot path.
Streaming RPC follows the established `SearchContent` pattern and reduces
perceived latency even when the backend is already fast.
