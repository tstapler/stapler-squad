# ADR-033: Terminal streams share a small, tunable pool of multiplexed WebSockets

Status: Proposed — 2026-10-30

## Context

Every visible terminal opens its own WebSocket for `StreamTerminal`; a page with 13 terminals opens 13 sockets at once.
The owner's hypothesis was the "6 connections per domain" browser limit.

Measured on headless Chromium 141 against a throwaway instance (`/tmp/wsprobe/probe.js`, HTTP/1.1 on :62873, also
reproduced against a bare Node WebSocket server so the server's handshake cost is excluded):

| Question | Result |
|---|---|
| Is there a 6-per-host WebSocket cap? | No. 100 of 100 opened; 255 of 300 were open when sampled (the guide's "255 global" figure holds, the "6 per origin" figure does not). |
| Do open sockets starve `fetch`? | No. 12 concurrent fetches with 0/6/13/30/100/300 sockets open: median 2/3/4/3/3/5 ms. |
| Do fetches stall during the handshake burst? | No. 1-5 ms (first fetch against the real server 40-54 ms). |
| Cost of opening N sockets at once | Chromium serializes handshakes per host. Real server: all 6 open by 69 ms, 13 by 769 ms, 30 by ~5 s. Bare Node server: 13 by 561 ms, 30 by 4.6 s. |

So the problem is not connection-pool starvation. It is (a) serialized open latency that grows steeply past ~12 sockets,
(b) the 255 global cap shared by every tab, and (c) per-socket cost on the server (fd, goroutines, handshake).

## Options

| | Option | Verdict |
|---|---|---|
| a | One shared terminal WebSocket with stream ids | Fixes (a)-(c) but one TCP stream carries every terminal; a stalled peer or a single large frame delays all. Needs per-stream flow control either way. |
| b | A pool of N sockets, streams assigned to sockets | a with the head-of-line blocker bounded to 1/N of the streams; N is a tuning knob and N=1 is option a. |
| c | Terminals over the existing unary mux socket | Rejected. The unary bridge is request/response with idempotent replay, an allow-list and a 8 s call deadline; terminal streams are long-lived, bidirectional and ordered. Sharing one socket would couple unary latency to terminal output. |
| d | Status quo plus lazy/visible-only open | Done separately (streamhub agent). Reduces N but does not remove (a)-(c) for many visible terminals. |

## Decision

Option b, with option a as `N=1`, behind the `terminal_ws_pool` feature flag (default off, read by the web client).

1. **Seam, server.** `streamConn` (`ReadMessage`/`WriteMessage`/`SetReadDeadline`/`SetWriteDeadline`) replaces the
   `*websocket.Conn` field in `connectWebSocketStream`. `*websocket.Conn` satisfies it; a `muxStreamConn` is a virtual
   connection carried by a mux socket. The whole `StreamTerminal` path (control mode, hub, `WebSocketTransport.Close`'s
   read-deadline interrupt) runs unchanged over it.
2. **Seam, client.** A `TerminalSocketPool` interface (`openStream(url) -> PooledSocket`). `PooledSocket` is the subset of
   the DOM `WebSocket` that `websocket-transport.ts` uses. Callers do not know how many sockets exist or which carries
   them. With the flag off, `createTerminalSocket` returns a plain `WebSocket` (today's path, unchanged).
3. **Wire format** (new endpoint `/api/ws/terminal-mux`, binary messages, big-endian; modelled on `ws_unary_frame.go`):
   `type u8 | stream id u32 | body`. Types: `HELLO` (server -> client: JSON `{poolSize, window, chunk, maxStreams}`),
   `OPEN`, `DATA` (`kind u8 | fin u8 | payload`), `CREDIT` (`u32` bytes), `CLOSE` (`code u16 | reason`), `PING`/`PONG`,
   `CONFIG` (same JSON as `HELLO`; pushes a new pool size to live sockets). A WebSocket message of any size is split
   into `chunk`-byte `DATA` frames; the receiver reassembles per stream.
4. **No head-of-line blocking inside a socket.** Each stream has its own write queue (byte-capped; `WriteMessage` blocks
   with the caller's write deadline, which is the backpressure into the streamhub subscriber and keeps its existing
   stall-eviction path). One writer goroutine per socket round-robins runnable streams one chunk at a time, like
   `unaryWriter`'s CHUNK interleave. A stream sends only while it has credit; the client grants credit as it consumes
   output, so a slow terminal parks its own queue and never blocks the others. Control frames have priority.
5. **Zombie/slot/deadline lessons from #1265.** The socket writer has a per-frame write deadline (a client that stops
   reading kills the socket, not a goroutine pile); each socket has a stream cap (`maxStreams`, close 1013 beyond it);
   `PING`/`PONG` plus a read deadline reap dead peers; closing a stream flushes its queue before `CLOSE` with a bounded
   wait; socket close drops every stream with a close error so handlers exit. Per-stream context is the socket's.
6. **Assignment.** `AssignmentPolicy` (client) picks the socket for a new stream: `leastLoaded` (fewest in-flight
   unacknowledged output bytes, then stream count) is the default, `hashBySession` the baseline. Assignment is sticky for
   the life of the stream. Sockets open lazily up to N and stay warm.
7. **Pool size N is one tunable.** Precedence: `localStorage ssq:terminal-pool-size` > server `HELLO.poolSize`
   (`STAPLER_SQUAD_TERMINAL_WS_POOL_SIZE`, default 2; `0` unregisters the endpoint so the client falls back) > default 2.
   Growing takes effect on the next assignment; shrinking marks the highest sockets "draining": they accept no new
   streams and close when empty. `CONFIG` frames carry runtime changes to connected clients. Flag off or N=0 is the
   per-stream path.
8. **Failover.** If a mux socket drops, each of its streams is surfaced to the transport as a closed socket exactly like
   a plain WebSocket close, so the existing reconnect path in `useTerminalStream` reopens and the hub's first-paint
   replay repaints; the pool opens the replacement on a surviving or new socket. No new resume protocol.
9. **Compatibility.** Old clients keep using the per-stream endpoint; the mux endpoint is additive. If the mux socket
   cannot be established (404, 4xx, breaker) the pool falls back to a plain `WebSocket` per stream.
10. **Observability.** Server metrics (bounded labels): `terminal_mux_sockets_open`, `terminal_mux_streams_open`,
    `terminal_mux_frames_total{dir,type}`, `terminal_mux_queue_bytes` (gauge across streams), `terminal_mux_hol_wait`
    (ms from enqueue to first frame written), `terminal_mux_credit_stalls_total`, `terminal_mux_socket_closed_total{reason}`.
    Each socket and each stream gets a Tempo span (`terminal_mux.socket`, `terminal_mux.stream`).

## Consequences

- Handshake cost: 13 terminals cost `min(13, N)` handshakes instead of 13; at N=2 that is under the ~70 ms 6-socket figure.
- One more protocol to keep in conformance; the frame codec has shared test vectors like the unary one.
- Output frames of one stream are delivered in order; ordering between streams is not defined.
- Risk: a slow *socket* (TCP) still delays every stream on it; N>1 bounds that and the write deadline reaps dead peers.
- Rollout: flag default off -> internal soak with N=2 -> default on. Rollback is the flag.

## Alternatives rejected

Reusing the unary socket (c); a protobuf wire schema for the frames (the repo's unary mux uses a hand-rolled binary
frame because it must carry opaque bodies and chunk them; the same reasoning applies here and keeps `buf breaking` out
of the loop).
