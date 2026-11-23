# History Search Streaming — Requirements

## Problem Statement

The Claude History Browser's full-text search mode (shown in the screenshot as
"Full-Text Search" with query "Light") blocks the entire UI while the server scans
all history. The user sees "Searching conversations…" with a spinner and no results
until the unary RPC completes.

Two root causes:

1. **`SearchClaudeHistory` is a blocking unary RPC.** The server must run
   `IncrementalSync` (which reads all JSONL files for newly indexed sessions) and
   the BM25 query to completion before the first byte reaches the browser.

2. **`IncrementalSync` scans every project JSONL file on the first call after a cold
   start (or after the history cache expires).** On a machine with many Claude
   projects (the screenshot shows 47+ open tabs suggesting heavy usage), this can
   take seconds.

Result: a user with large history (hundreds of sessions, many JSONL files) waits
multiple seconds before seeing any results, even though the BM25 scorer can rank
top results almost instantly once the index is built.

## Goals

- Results begin appearing in the UI immediately as the server finds them, without
  waiting for the full scan to complete.
- The first meaningful batch (top 5–10 results) arrives within 200 ms of the user
  finishing typing (after debounce).
- The existing BM25 in-memory index (`search.SearchEngine`) is preserved; this
  feature improves how results are delivered, not how they are ranked.
- No regression in result quality or relevance order.

## Non-Goals

- Replacing the BM25 engine with a different search technology.
- Changing how the history JSONL cache TTL works.
- Pagination of `ListClaudeHistory` (already paginated with cursor tokens).
- Full-text search of message *bodies* (the JSONL files under
  `~/.claude/projects/`); that is `SearchClaudeHistory`'s domain and is in scope.

## Acceptance Criteria

1. `SearchClaudeHistory` returns a **server-streaming response**: the client
   receives at least one batch of results before the full scan completes when the
   index is not fully warm.
2. The UI renders each arriving batch progressively — results appear on screen
   within 200 ms of the first streaming message, without clearing or re-rendering
   the whole list.
3. The existing unary RPC path is preserved (or gracefully deprecated) so MCP
   callers (`search_claude_history` tool) are not broken.
4. `IncrementalSync` is not called on every search request when the index is already
   current; a background warm-up or lazy sync decouples index freshness from search
   latency.
5. A new feature flag (`history_search_streaming`) gates the streaming path so it
   can be rolled out and rolled back independently.
6. All existing `SearchClaudeHistory` tests pass; new streaming-path tests cover
   progressive delivery and cancellation.
