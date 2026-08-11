# ADR-002: Cursor-Based Pagination for ListClaudeHistory

**Status**: Accepted
**Date**: 2026-04-12
**Project**: history-page-revamp

## Context

The current `ListClaudeHistoryRequest` accepts a single `limit` integer (currently called with `limit: 500`). This forces the server to serialize and the client to deserialize all 500 entries before any rendering can begin. There is no pagination, no load-more, and no way for the virtualizer to fetch additional pages on scroll.

We need a pagination strategy that:
1. Returns the first page (50 items) fast enough for 200ms render budget
2. Remains stable when new sessions are added while the page is open
3. Composes naturally with `useInfiniteQuery` on the frontend

## Decision

Add cursor-based pagination to `ListClaudeHistoryRequest` / `ListClaudeHistoryResponse` using an opaque `page_token` / `next_page_token` pair (AIP-158 style). Default page size 50, max 200. Retain the existing `limit` field as a wire-compatible deprecated alias.

## Rationale

### Cursor vs. offset

| Criterion | Offset (`LIMIT x OFFSET y`) | Cursor (`page_token`) |
|---|---|---|
| Consistency during inserts | Poor — items shift between pages | Stable — cursor tracks position |
| Performance on large in-memory list | O(N) slice to offset | O(log N) binary search on updatedAt |
| Complexity | Trivial | Slightly more complex |
| Compatible with current sort | Yes | Yes (sorted by `UpdatedAt` desc already) |

The history list is sorted by `UpdatedAt` descending and new sessions are always added at the head. Offset pagination between page fetches would produce duplicate or skipped items. Cursor pagination is correct.

### Unary RPC vs. server streaming

Server streaming was considered to push rows incrementally. Rejected because:
1. First-paint latency is the same — client still waits for the first message
2. HTTP/1.1 fallback complicates local dev
3. ConnectRPC's streaming cancellation adds client state complexity
4. A well-tuned unary call returning 50 rows is faster to first paint than any streaming approach

## Cursor Encoding

```
page_token = base64url( json({ u: updatedAt_unix_ns, i: entry_id }) )
```

The server decodes the cursor to a `(updatedAt, id)` pair and resumes the sorted slice scan with a binary search on `updatedAt`, falling back to linear scan only for ties on the same timestamp.

## Proto Change

```protobuf
message ListClaudeHistoryRequest {
  optional string project      = 1;
  optional string search_query = 2;
  int32           page_size    = 3;  // replaces `limit`; same wire number → backward compat
  string          page_token   = 4;  // empty = first page
}

message ListClaudeHistoryResponse {
  repeated ClaudeHistoryEntry entries        = 1;
  int32                       total_count    = 2;
  string                      next_page_token = 3;  // empty = no more pages
}
```

Field 3 is renamed from `limit` to `page_size` — same wire type (varint), same number → existing callers using the integer value continue to work.

## Consequences

**Positive:**
- First page (50 items) renders in <100ms, well under the 200ms budget
- Frontend `useInfiniteQuery` composes naturally with `getNextPageParam: (last) => last.nextPageToken || undefined`
- Stable ordering under concurrent history writes

**Negative:**
- Server must maintain the in-memory sorted list for binary search (already done in `Reload()`)
- "Jump to page N" is not supported (acceptable — we only need load-more)
- Cursor must be URL-safe (base64url satisfies this)

## Patterns Applied

- **Cursor pagination** (Google AIP-158)
- **Opaque token pattern** — client treats cursor as a black box
- **Backward-compatible proto field rename** — same field number, same wire type

## Related

- `proto/session/v1/session.proto` — ListClaudeHistoryRequest, ListClaudeHistoryResponse
- `server/services/search_service.go` — ListClaudeHistory handler
- research/architecture.md §2 — detailed cursor encoding design
