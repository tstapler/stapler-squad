# ADR-001: Use Streaming ConnectRPC for Bundle Download, Not a Plain HTTP Handler

**Date**: 2026-10-16
**Status**: Accepted
**Feature**: RUM Session Diagnostic Bundle

---

## Context

The assembled bundle ZIP is up to ~15 MB uncompressed (worst-case: scrollback 500 lines + 2,000 analytics events + context history + artifacts). It must be delivered to the browser for download. Two patterns exist in this codebase:

1. **Streaming ConnectRPC** — used by `WatchSessions` and `StreamTerminal`; chunks flow via `stream.Send()`.
2. **Plain HTTP handler** — used by `AnalyticsHandler`; writes `Content-Type: application/json` with standard `http.ResponseWriter`.

A third option (unary ConnectRPC returning `bytes zip_content`) was rejected early: ConnectRPC has no configured `WithReadMaxBytes` guard (confirmed: zero occurrences of `WithReadMaxBytes` in production server code), and a 15 MB response in a single frame risks OOM on both server and proxy.

## Decision

Use **server-streaming ConnectRPC** (`rpc ExportSessionBundle(...) returns (stream BundleChunk)`) for the download path and a **unary ConnectRPC** (`rpc SaveSessionBundle(...)`) for the server-save path. Both are defined in `proto/session/v1/rum_bundle.proto` and registered via `ConnectOptions`.

## Rationale

| Factor | Streaming ConnectRPC | Plain HTTP Handler |
|--------|---------------------|-------------------|
| Auth middleware | Automatic via `ConnectOptions` interceptors | Must be attached manually to `mux.Handle` |
| Pattern consistency | Matches `WatchSessions`, `StreamTerminal` | Matches `AnalyticsHandler`, `OtelProxyHandler` (unauthenticated endpoints) |
| Client code | Chunk reassembly into `Blob` (~10 lines) | `fetch(url).then(r => r.blob())` (simpler) |
| Large payload safety | Chunks eliminate single-frame OOM risk | Same risk as unary RPC if writing full body in one call |
| New proto machinery | Yes: new `.proto` file, `make proto-gen` step | No: handler registered directly in `server.go` |

The plain HTTP handler pattern in this codebase is used exclusively for unauthenticated or system-internal endpoints (`/api/otel/v1/traces`, `/analytics`). Adding a user-triggered file download via a plain HTTP handler would require manually threading auth middleware and would be inconsistent with the rest of the session API surface. The extra proto machinery is the correct trade-off.

## Consequences

- `proto/session/v1/rum_bundle.proto` must be authored and `make proto-gen` run before any frontend work.
- The TypeScript client uses the generated `SessionBundleService` client and reassembles `BundleChunk.data` arrays before calling `new Blob([...chunks])`.
- The 64 KB chunk size is a constant in `session_bundle_service.go`; it can be tuned without a proto change.
