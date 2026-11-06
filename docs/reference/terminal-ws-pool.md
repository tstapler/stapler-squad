# Terminal WebSocket pool (`terminal_ws_pool`)

Carries `StreamTerminal` over a few shared WebSockets (`/api/ws/terminal-mux`) instead of one per terminal. Design and measurements (13 terminals: first output ~0.45 s per-stream vs under 50 ms pooled, echo latency unchanged): [ADR-033](../adr/ADR-033-terminal-websocket-pool.md#measured-latency-pool-vs-per-stream). Default off.

## Knobs

| Knob | Where | Effect |
|---|---|---|
| `terminal_ws_pool` | Feature flag (Settings -> Features), read by the web client | Off: one WebSocket per terminal (unchanged). On: pooled. Evaluated per new stream; follows the 30 s flag refresh, so already-open terminals keep their socket until they reconnect. |
| `STAPLER_SQUAD_TERMINAL_WS_POOL_SIZE` | Server env, read at startup (default `2`, max `64`) | Socket count advertised to clients in `HELLO`. `0` makes `/api/ws/terminal-mux` answer 404; the first streams fail once, then clients use one WebSocket per terminal. |
| `localStorage["ssq:terminal-pool-size"]` | Per browser | Overrides the advertised size. `0` disables the pool for this browser. |
| `POST /actuator/terminal-mux?poolSize=N` | Runtime, local listener only (403 on remote access) | Changes the advertised size and pushes `CONFIG` to live sockets; clients that have not pinned a size resize without a reload. Not persisted: a restart returns to the env value. `GET /actuator/terminal-mux` reports `{poolSize, sockets, streams}`. |

Client size is clamped to 1-16. Growing takes effect on the next stream; shrinking marks the emptiest sockets draining (no new streams, closed when their last stream ends). Live streams are never moved or dropped.

## Wire format

Binary WebSocket messages, big-endian: `type u8 | stream id u32 | body`. Codecs: `server/services/ws_terminal_mux_frame.go` and `web-app/src/lib/transport/terminal-mux-frame.ts`, both tested against `web-app/src/lib/transport/__fixtures__/terminal-mux-vectors.json`.

| Type | Value | Direction | Body |
|---|---|---|---|
| `HELLO` | 1 | server -> client | JSON `{poolSize, window, chunk, maxStreams}` |
| `OPEN` | 2 | client -> server | none; the stream's first two DATA messages are the Connect header text and request envelope, exactly as on a plain socket |
| `DATA` | 3 | both | `kind u8 (1 text, 2 binary) \| fin u8 \| payload` (messages are split into `chunk`-byte frames) |
| `CREDIT` | 4 | client -> server | `u32` bytes of receive window returned |
| `CLOSE` | 5 | both | `code u16 \| reason` |
| `PING` / `PONG` | 6 / 7 | both | none |
| `CONFIG` | 8 | server -> client | same JSON as `HELLO` |

## Flow control and limits

- Each stream starts with 256 KiB of send credit. The server stops sending on a stream at zero credit and keeps serving other streams; the client returns credit as frames arrive (transport-level, since the browser WebSocket API gives no application backpressure).
- Per-stream queue cap 1 MiB; `WriteMessage` blocks with the caller's write deadline when full.
- A stream with queued output and no credit for 30 s is closed with 1013. Streams per socket: 64 (`OPEN` beyond it gets `CLOSE 1013`). Socket idle timeout 60 s (client and server each ping every 20 s; the peer answers with `PONG`); socket write deadline 10 s.
- Admission matches `/api/ws/unary`: remote listener requires authentication and a strict origin match; local requires a loopback origin.

## Authorization

`/api/ws/terminal-mux` is registered on both listeners whenever the pool size is above 0, regardless of the `terminal_ws_pool` flag (the flag is read only by the web client), so this re-check protects the endpoint today, not only after the flag is enabled.

The socket is authenticated at upgrade, and **every `OPEN` re-runs the listener's auth chain** with the upgrade request's identity (cookie, host, peer), the same chain a plain `StreamTerminal` upgrade passes. A session that expires or is revoked after the socket was admitted therefore cannot open new streams on it. A denied `OPEN` gets `CLOSE 1008` with the fixed text `unauthorized` (no session id, credential or chain detail), the handler never runs, and the socket is closed with 1008 after 3 denied `OPEN`s. Only `unauthenticated` and `forbidden` results count; a transient chain failure (`error`) is refused but never ends the socket. The strike counter never resets on a successful `OPEN`, by design: a reset would let a client alternate allowed and denied `OPEN`s forever, and a legitimate client has no reason to see even one denial. Beyond the 3-strike close, the check is bounded only by the 64-stream cap and the cost of one cookie lookup. On the web client, a `CLOSE 1008` marks that socket draining (the next stream dials a fresh one with the current cookie) and the stream fails with `Code.Unauthenticated`, which `isNonRetriableConnectError` treats as non-retriable. A bridge with no dispatcher wired refuses every `OPEN`. Streams already open when a session is revoked keep running until they end, as a plain WebSocket does.

There is no per-session ACL to check: authorization in stapler-squad is a single passkey session, and `StreamTerminal` resolves any known session id for any authenticated caller on both the pooled and plain paths (including archived sessions, which the handler does not filter). If per-session scoping is ever added it must be enforced in `streamTerminal` so both paths get it.

## Failure behaviour

A dropped mux socket closes each of its streams with code 1006, which `useTerminalStream` treats like any WebSocket loss. Its reconnect is unconditional (no env flag) and never gives up on connection-level failures: full-jitter exponential backoff (2 s first window, 30 s cap) retries indefinitely, so streams sharing the dropped socket retry at spread-out times. After 5 consecutive failures the pane shows "Server unreachable, retrying" (each retry waits up to 30 s, about 15 s on average once the backoff is at its cap) and a tab-visible or `online` event retries at once. A stream that stayed up 10 s restarts the backoff and clears that state. Retrying stops, and the Retry banner shows, for definitive errors (4001/`Unauthenticated`, 4004/`NotFound`, a missing worktree, `HUB_START_FAILED`) and when the server itself keeps ending the stream (an error frame or a clean EndStream) for 10 minutes without a stable attach. That second case has its own backoff and clock, because squad answers that way while still loading sessions after a restart; it never shows "unreachable". Close 1013 (the server refused a stream at its per-socket cap) retries on its own schedule without counting toward "unreachable". A process-wide gate allows 4 connects in flight (until the first message); the foreground pane goes first. The connect timeout is 8 s (initial, background); only a foreground/visibility/online resume uses 1.2 s for its first 2 attempts, and a timeout abort retries after 0.5 s doubling to 5 s, at most 5 times in a row before the normal backoff takes over. While reconnecting the pane shows `Reconnecting terminal…`, and a Reconnect button appears after 5 s without a connection. The reopened stream gets a first-paint replay from the hub. A mux failure before any `HELLO` (404, 403, unreachable) fails the streams that were waiting on it once; they reconnect on plain WebSockets, and new streams use plain WebSockets for 60 s before the pool tries the mux again.

### Reading a 1006 in the logs

The 1006 above is synthetic: the server gives every stream of a dropped socket `errMuxSocketClosed` (a 1006 `CloseError`, text `mux socket closed`). Per-pane lines for it (`WebSocket read ended: mux socket closed`, `StreamTerminal ended: mux socket closed`, `EndStreamError not sent`) log at Debug. The cause is one line per socket, `terminal mux socket closed`, with `reason`, `err`, `close_code`/`close_text` (when the error is a WebSocket close), `streams` (panes dropped), `age`, `since_last_frame`, `listener`, `remote_addr` and `user_agent`. The client sends no connection id, so correlate by `remote_addr` and time.

| `reason` (same value as `terminal_mux_socket_closed_total{reason}`) | Level | Meaning |
|---|---|---|
| `peer_closed` | Info | Client sent a normal or going-away close frame. |
| `shutdown` | Info | Server context cancelled. |
| `read_error` | Warn | Transport failed with no close frame (abrupt TCP close, proxy or tailscale drop, tab killed). |
| `idle_timeout` | Warn | No frame for 60 s (`muxReadIdle`); `since_last_frame` confirms. |
| `malformed` | Warn | Non-binary message or undecodable frame. |
| `unauthorized` | Warn | Three refused `OPEN`s. |
| `write_error` | Warn | A server write failed and the server tore the socket down. |
| `control_overflow` | Warn | The client stopped reading control frames (queue over 1024). |

Before this change `idle_timeout`, `write_error` and `control_overflow` were all counted as `read_error`, so an alert selecting `reason="read_error"` will under-count after deploy. A write failure caused by the peer going away (reset, broken pipe, closed connection) is not `write_error`; it is classified from the read side (`read_error` or `peer_closed`).

## Metrics

Server: `terminal_mux_sockets_open`, `terminal_mux_streams_open`, `terminal_mux_frames_total{dir,type}`, `terminal_mux_queue_bytes` (histogram, per-stream queued bytes at enqueue), `terminal_mux_hol_wait` (ms from enqueue to first frame written), `terminal_mux_credit_stalls_total`, `terminal_mux_write_errors_total`, `terminal_mux_socket_closed_total{reason}`, `terminal_mux_open_denied_total{listener,reason}` (listener `local`/`remote`; reason `unauthenticated`, `forbidden`, `unavailable`, `error`). A denied `OPEN` also logs `[TerminalMux] OPEN denied` with the same two fields. Spans: `terminal_mux.socket`, `terminal_mux.stream`. Dashboard: "Terminal socket pool (ADR-033)" row in `docs/observability/grafana/dashboards/client-network.json`.
