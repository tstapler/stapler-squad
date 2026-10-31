# Terminal WebSocket pool (`terminal_ws_pool`)

Carries `StreamTerminal` over a few shared WebSockets (`/api/ws/terminal-mux`) instead of one per terminal. Design and measurements: [ADR-033](../adr/ADR-033-terminal-websocket-pool.md). Default off.

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
- A stream with queued output and no credit for 30 s is closed with 1013. Streams per socket: 64 (`OPEN` beyond it gets `CLOSE 1013`). Socket idle timeout 60 s (client pings every 20 s); socket write deadline 10 s.
- Admission matches `/api/ws/unary`: remote listener requires authentication and a strict origin match; local requires a loopback origin.

## Failure behaviour

A dropped mux socket closes each of its streams with code 1006, which `useTerminalStream`'s reconnect path treats like any WebSocket loss; the reopened stream gets a first-paint replay from the hub. A mux failure before any `HELLO` (404, 403, unreachable) fails the streams that were waiting on it once; they reconnect on plain WebSockets, and new streams use plain WebSockets for 60 s before the pool tries the mux again.

## Metrics

Server: `terminal_mux_sockets_open`, `terminal_mux_streams_open`, `terminal_mux_frames_total{dir,type}`, `terminal_mux_queue_bytes` (histogram, per-stream queued bytes at enqueue), `terminal_mux_hol_wait` (ms from enqueue to first frame written), `terminal_mux_credit_stalls_total`, `terminal_mux_write_errors_total`, `terminal_mux_socket_closed_total{reason}`. Spans: `terminal_mux.socket`, `terminal_mux.stream`. Dashboard: "Terminal socket pool (ADR-033)" row in `docs/observability/grafana/dashboards/client-network.json`.
