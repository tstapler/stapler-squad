# ADR-001: Path Completion API Design — Unary RPC vs Server-Streaming

Status: Accepted
Date: 2026-04-08

## Context

The Omnibar needs a backend API to list filesystem directory contents for path completion. Two patterns are viable: a unary RPC that returns a batch of entries, or a server-streaming RPC that streams entries as they are discovered.

The server reads local directories via `os.ReadDir`, which returns all entries at once (not incremental). Network latency to localhost is negligible (~0.1ms). The client needs the full result set to render the dropdown and compute the longest common prefix for Tab completion.

## Decision

Use a **unary RPC** `ListPathCompletions` that returns up to `max_results` entries in a single response.

```protobuf
rpc ListPathCompletions(ListPathCompletionsRequest) returns (ListPathCompletionsResponse) {}
```

Request fields: `path_prefix` (string), `max_results` (int32, default 50), `directories_only` (bool).
Response fields: `entries[]` (path, name, is_directory), `base_dir` (string), `truncated` (bool).

## Consequences

- Simpler client logic: single request/response, no stream lifecycle management.
- Caching is straightforward: cache the response by `path_prefix` with TTL.
- Server implementation is a thin wrapper around `os.ReadDir` + filter.
- Truncation at 500 entries server-side prevents unbounded responses.
- Does NOT support progressive rendering (acceptable: localhost latency is negligible and directory reads complete in <5ms for typical directories).

## Alternatives Rejected

1. **Server-streaming RPC**: Adds complexity (stream lifecycle, partial results, cancellation) for no observable latency benefit on localhost.
2. **Reuse ListSessions for path suggestions**: Only provides previously-used paths, not filesystem browsing. Kept as a complementary feature, not replaced.
3. **WebSocket-based custom protocol**: Bypasses the ConnectRPC stack used everywhere else; unnecessary protocol divergence.
