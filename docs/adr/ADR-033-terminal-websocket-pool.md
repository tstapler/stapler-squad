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
5. **Admission.** Same gates as `/api/ws/unary`: a known listener, `AuthModeRequired` on the remote listener, and a per-listener origin policy (strict origin match on remote, loopback on local). Stream ids are client-chosen; the server refuses id 0 and duplicates, and a late `forget` by an old handler never evicts a reused id. Reassembly is capped at the inbound byte bound, `CREDIT` is clamped to the window, the control queue is capped, and a stream with queued output and no credit for 30 s is closed with 1013.
6. **Zombie/slot/deadline lessons from #1265.** The socket writer has a per-frame write deadline (a client that stops
   reading kills the socket, not a goroutine pile); each socket has a stream cap (`maxStreams`, close 1013 beyond it);
   `PING`/`PONG` plus a read deadline reap dead peers (both sides ping: the server every 20 s because a hidden tab's timers are throttled to about once a minute); closing a stream flushes its queue before `CLOSE` with a bounded
   wait; socket close drops every stream with a close error so handlers exit. The socket's request context parents the socket and stream spans; handlers do not receive it (they end when the stream or socket ends).
7. **Assignment.** `AssignmentPolicy` (client) picks the socket for a new stream: `leastLoaded` (fewest in-flight
   unacknowledged output bytes, then stream count) is the default, `hashBySession` the baseline. Assignment is sticky for
   the life of the stream. Sockets open lazily up to N and stay warm.
8. **Pool size N is one tunable.** Precedence: `localStorage ssq:terminal-pool-size` > server `HELLO.poolSize`
   (`STAPLER_SQUAD_TERMINAL_WS_POOL_SIZE`, default 2; `0` makes the endpoint answer 404 so the client falls back; the env var is read at startup, `TerminalMuxBridge.SetPoolSize` is the programmatic seam that pushes `CONFIG` to live sockets) > default 2.
   Growing takes effect on the next assignment; shrinking marks the highest sockets "draining": they accept no new
   streams and close when empty. `CONFIG` frames carry runtime changes to connected clients. Flag off or N=0 is the
   per-stream path.
9. **Failover.** If a mux socket drops, each of its streams is surfaced to the transport as a closed socket exactly like
   a plain WebSocket close, so the existing reconnect path in `useTerminalStream` reopens and the hub's first-paint
   replay repaints; the pool opens the replacement on a surviving or new socket. No new resume protocol.
10. **Compatibility.** Old clients keep using the per-stream endpoint; the mux endpoint is additive. If the mux socket
   cannot be established (404, 4xx, breaker) the pool falls back to a plain `WebSocket` per stream.
11. **Observability.** Server metrics (bounded labels): `terminal_mux_sockets_open`, `terminal_mux_streams_open`,
    `terminal_mux_frames_total{dir,type}`, `terminal_mux_queue_bytes` (per-stream unsent bytes, a histogram sampled at each enqueue), `terminal_mux_hol_wait`
    (ms from enqueue to first frame written), `terminal_mux_credit_stalls_total`, `terminal_mux_socket_closed_total{reason}`.
    Each socket and each stream gets a Tempo span (`terminal_mux.socket`, `terminal_mux.stream`).

## Measured latency: pool vs per-stream

Headless Chromium 141, throwaway instance on loopback, 13 `bash` sessions, host load average 100-148 (noisy; each config run three times, interleaved, median shown with the min-max range). Harness: `bench.ts` (bundled client pool, one browser per row). Time to first output (TTFO) is the slowest of the 13 streams from the first open. Echo is one keystroke on stream 0 until it is echoed back (30 samples); "loaded" repeats it while three other terminals flood output with `yes | head -c 10000000`.

| Config | Sockets | TTFO max (ms) | Echo idle p95 (ms) | Echo loaded p95 (ms) |
|---|---|---|---|---|
| per-stream (flag off) | 13 | 458 (423-602) | 1 (1-4) | 4 (3-19) |
| pool N=1 | 1 | 6 (5-7) | 1 (1-2) | 4 (3-21) |
| pool N=2 | 2 | 18 (17-18) | 1 (1-3) | 6 (5-18) |
| pool N=4 | 4 | 41 (41-60) | 2 (1-3) | 8 (4-11) |

Reading it: TTFO falls from ~0.45 s to under 50 ms because 13 serialized handshakes become 1-4. Idle echo is unchanged. Loaded echo p95 is within run-to-run noise of per-stream (the spread across rounds, 3-21 ms, is larger than the gap between configs), and the flood volume per run varied from 0.9 to 7.7 MB, so the loaded column is indicative, not a throughput comparison. No tuning of chunk size, credit window or writer scheduling was needed. Not measured: a congested or high-RTT network, where one slow TCP stream carries all terminals on its socket (N>1 bounds that).

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
