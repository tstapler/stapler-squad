# ADR-001: Wire framing for unary RPCs multiplexed over one WebSocket

**Status**: Accepted 2026-10-26 by the Story 5.1.1 spike (`research/ws-unary-seams.md`, `server/ws_unary_spike_test.go`; no amendment). Prior history: Proposed (Phase 3 plan, amended in repairs 2026-10-26 pass 1 and pass 2; confirm in spike Story 5.1.1. Build proceeds on Proposed; Accepted is recorded by the spike)
**Date**: 2026-10-26
**Decider**: tstapler (owner decision recorded: build unary-over-WebSocket; not deferred; default-on is gated by Story 5.5.1)

## Context
Unary Connect calls use `fetch` over HTTP/1.1 on `localhost:8543`; the browser caps 6 connections per origin, and the HAR shows 161 of 234 requests stalled ~30 s (`requirements.md` Evidence 1). The existing WS code is streaming-only: `web-app/src/lib/transport/watch-ws-transport.ts:92-97` delegates `unary` to HTTP, and `server/services/ws_stream_bridge.go` (`StreamingWSBridge`) serves one request envelope per socket per watch path with `Content-Type: application/connect+proto` (`ws_stream_bridge.go:94-98`), which is correct for **streaming** handlers.

Unary Connect handlers are invoked with `Content-Type: application/proto` and a **raw** body (no 5-byte envelope); errors come back as an HTTP status plus a JSON body. The earlier draft of this ADR put a Connect envelope in unary REQUEST/RESPONSE frames and cited `encodeEnvelope` at `connectrpc_websocket.go:24`; that function does not exist (line 24 is an import; the real helpers are `envelopeBufPool` and `marshalProtoEnvelope`, `connectrpc_websocket.go:49-63`) and the envelope would not match what a unary handler expects.

## Options
1. **Raw-proto mux frames** (chosen): one WS binary message = 1 type byte + 4-byte request id + type-specific body. Unary bodies carry raw protobuf, matching `application/proto`. The bridge sets `Content-Type: application/proto` and passes the raw bytes to the real handler chain.
2. **Connect-envelope mux frames** (previous draft): reuses streaming envelope code, but the bridge would have to unwrap the envelope and set `application/proto` anyway, so the envelope adds bytes and code for no benefit on unary.
3. **JSON-RPC-style text frames** (`{id, method, params}`): easy to debug, but base64/JSON-encodes protobuf bytes and doubles work for `ListFiles`/`GetFileContent`.
4. **Per-call WebSocket**: no new framing, but the HAR already shows 20 open sockets; defeats the purpose.

## Decision
Option 1. Frame layout (all integers big-endian):

| Offset | Size | Field |
|---|---|---|
| 0 | 1 | type: 1 REQUEST, 2 RESPONSE, 3 ERROR, 4 CANCEL, 5 PING, 6 PONG, 7 CHUNK |
| 1 | 4 | request id (client-chosen uint32; wraps with in-flight collision check; even ids are reserved for a future server-initiated push and are never sent today) |
| 5 | n | type-specific body |

- REQUEST body: `u16 methodLen | method ("/pkg.Service/Method") | u16 metaLen | meta JSON {headers, timeoutMs} | raw proto bytes`. The bridge maps `timeoutMs` to the `Connect-Timeout-Ms` header and lets connect-go apply the deadline. **`meta.headers` is an explicit client-header allowlist** (the `clientHeaders` list in the shared `ws-unary-allowlist.json` fixture; initially only `Connect-Protocol-Version`). Any other key, and always `Host`, `Origin`, `Cookie`, `Authorization`, `Forwarded`, `X-Forwarded-*`, `Content-*`, `Connect-Timeout-Ms`, `Accept-Encoding`, answers ERROR `invalid_argument` (fail closed). Guard-relevant headers come only from the upgrade request (ADR-002 item 1), so page JS cannot change a HostGuard/`LocalWriteGuard`/auth verdict per call.
- RESPONSE body: `u32 totalLen | u8 fin | u16 metaLen | meta JSON {headers, trailers} | raw proto bytes (first slice)`. `fin = 1` means the payload is complete in this frame. `meta` appears only in the RESPONSE frame (never in CHUNK). `headers` are the recorder's response headers except `Set-Cookie`, `Content-*` and hop-by-hop headers; `trailers` are the recorder's `Trailer-`-prefixed headers with the prefix stripped (connect-go sends unary trailers that way). The client maps them to `ConnectError`/response `header` and `trailer`.
- CHUNK body (type 7): `u8 fin | raw proto bytes`. Sent after a RESPONSE with `fin = 0`; the client concatenates until `fin = 1` and verifies the sum equals `totalLen`. Interleaving of CHUNK frames across request ids is allowed (head-of-line mitigation, ADR-003). Both `fin` and `totalLen` are part of the shared conformance vectors (Story 5.1.1b).
- ERROR body: JSON `{code, message, details?, headers?, trailers?}` where `code` is the Connect code string, built from the dispatch recorder's HTTP status and JSON body; mapped 1:1 to `ConnectError`/`connect.Code`. A method not in the server's `wsUnaryAllowed` yields `permission_denied` with `details.marker = "ws_method_not_allowed"`.
- CANCEL: no body; server cancels the handler context for that id. After the RESPONSE head has been sent, CANCEL also makes the server drop that id's queued CHUNK frames.
- Max frame size 256 KiB (`maxFrameBytes = 262144`, measured on the **whole encoded frame**: type + id + method + meta + body; the client sends proto bytes, so no JSON/base64 inflation applies). **Enforcement is two-layer.** (1) Application layer: after each `ReadMessage`, the bridge checks `len(msg) > maxFrameBytes`; if so it parses the request id from bytes 1-4 (when at least 5 bytes were read) and answers ERROR `resource_exhausted` for that id, socket stays open. (2) Hard backstop: `conn.SetReadLimit(4 * maxFrameBytes)` (1 MiB). In the vendored gorilla/websocket (`v1.5.4-0.20250319132907-e064f32e3674`, `conn.go:933-935`) exceeding the read limit sends close 1009 and returns `ErrReadLimit`, the connection is dead afterwards, so the backstop is a protocol-violation guard only (a conforming client never sends more than `maxFrameBytes`, so it is never reached). **Requests** whose encoded frame would exceed `maxFrameBytes` are HTTP-only by client policy (for example `WriteFile` up to 1.5 MiB under `withFileBodyLimits`). **Responses** larger than 256 KiB are sent as RESPONSE (`fin=0`) plus CHUNK frames.
- The bridge buffers the whole response (recorder style) before chunking; there are no streaming semantics, and chunking exists to bound head-of-line blocking, not to stream.
- Unary is a REQUEST then exactly one RESPONSE(+CHUNKs) or ERROR. Streaming RPCs stay on the existing per-path WS bridges (unchanged).
- Route: `/api/ws/unary` (see ADR-002 and ADR-003).

## Consequences
- No dependency on `marshalProtoEnvelope`/`envelopeBufPool` for unary (they are streaming-envelope helpers); the codec is new (`server/services/ws_unary_frame.go`, `lib/transport/unary-mux-frame.ts`) and tested against shared vectors.
- DevTools loses per-call network rows for WS-routed calls, so the dev in-flight gauge (Story 1.1.2) and `rpc:<Method>` timing marks must also cover the WS path.
- Server bridge must dispatch by re-entering a per-listener dispatcher (stamp, HostGuard, auth or `LocalWriteGuard`, mux; ADR-002), not re-implement handlers, so interceptors, metrics, `/actuator` counters and rpcTiming stay uniform (Story 5.2.1).
- Wire-parity claim is about the **decoded message** plus metadata, not bytes: the shared HTTP transport is JSON by default (`web-app/src/lib/api/transport.ts:32-39`, no `useBinaryFormat`) while the bridge carries proto, so tests compare decoded messages.
- Not wire-compatible with generic Connect clients; the endpoint is a private web-app channel, HTTP remains the public protocol.
