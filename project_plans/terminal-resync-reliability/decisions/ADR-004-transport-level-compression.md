# ADR-004: Envelope-Level `CompressedFlag` Instead of Whole-Connection Permessage-Deflate

**Date**: 2026-08-13
**Status**: Accepted (revised)
**Project**: terminal-resync-reliability

## Context

`requirements.md` Scope item 5 asks for batching/compression to reduce resync payload
size, but `requirements.md`'s Out-of-Scope boundary excludes changes to the WebSocket
transport's connection-level behavior for traffic unrelated to resync. Two compression
mechanisms already have partial plumbing in this codebase:

- `server/protocol/envelope.go`'s custom `[1 byte flags][4 bytes length][N bytes data]`
  framing defines a `CompressedFlag` (0x01) bit, but the server never sets it, and the
  client's `parseResponseBody` (`web-app/src/lib/transport/websocket-transport.ts:173-178`)
  **hard-throws** `ConnectError(Code.Internal)` if it ever receives a frame with that flag
  set.
- `gorilla/websocket.Upgrader` (used at `connectrpc_websocket.go:382` via the package-level
  `wsUpgrader`, `:70-74`) natively supports permessage-deflate compression via
  `EnableCompression: true`, transparently negotiated with the browser's own native
  WebSocket compression support — no client code changes needed, but applied to **every**
  frame on the connection, not just resync traffic.

## Original Decision (superseded)

The original version of this ADR chose whole-connection `permessage-deflate`: enabling
`EnableCompression: true` on a local copy of `wsUpgrader` at the `Upgrade()` call site
(`connectrpc_websocket.go:382`) when `terminal:resync-compression` is on, leaving the
shared package-level var untouched. That decision was reversed during architecture
review of `implementation/plan.md`: `EnableCompression: true` compresses **all** frames
on the connection, not just resync-related `CurrentPaneRequest`/`TerminalOutput`
payloads — this exceeds `requirements.md`'s Out-of-Scope boundary, which limits this
project's transport changes to resync traffic specifically. A per-connection compression
toggle also cannot be selectively applied per-message, so there was no way to keep the
mechanism scoped to resync without touching every other message type flowing over the
same socket (session output streaming, keepalives, etc.).

## Decision (revised)

Use the existing envelope-level `CompressedFlag` bit (`server/protocol/envelope.go:19`)
instead. The server gzip-compresses and sets `CompressedFlag` only on `TerminalOutput`
payloads sent in response to a `CurrentPaneRequest` (the resync path) whose marshaled
size exceeds a threshold (e.g. 1024 bytes), when `terminal:resync-compression` is on.
This requires first fixing the client's hard-throw-on-compressed-frame bug in
`parseResponseBody` (`websocket-transport.ts:173-178`) so it decompresses instead of
throwing — see `implementation/plan.md` Task 5.1.1.0. All other message types, and all
resync payloads below the threshold, are unaffected: the shared `wsUpgrader` var and
`ws_stream_bridge.go:65`'s unrelated use of it remain untouched.

## Alternatives Considered

| Alternative | Rejected because |
|---|---|
| Whole-connection `permessage-deflate` via `wsUpgrader.EnableCompression` (original decision) | Compresses every frame on the connection, not just resync traffic — exceeds `requirements.md`'s Out-of-Scope boundary; no mechanism to scope a connection-level toggle to a subset of message types |
| Do nothing (leave `CompressedFlag` unused and unresolved) | Leaves the client's hard-throw-on-compressed-frame bug in place indefinitely and forgoes any payload-size mitigation, contrary to `requirements.md` Scope item 5 |

## Consequences

- Requires a client-side fix (Task 5.1.1.0) to `parseResponseBody`'s error path before any
  server-side `CompressedFlag` use is safe — a small, contained change scoped to the
  compressed-frame branch only.
- Compression is scoped precisely to resync traffic (`CurrentPaneRequest`-triggered
  `TerminalOutput` above the size threshold); all other message types on the connection
  are unaffected, keeping this change within `requirements.md`'s Out-of-Scope boundary.
- CPU/latency cost of gzip-compressing resync payloads at production concurrency is
  unmeasured before this project (Unresolved Question #3 in `implementation/plan.md`);
  ships default-off with a benchmark task (Task 5.1.1.3) gating any future default-on
  recommendation.
- A round-trip test (Task 5.1.1.4) covers the full compress-on-server /
  decompress-on-client path to guard against silent corruption of resync payloads.
