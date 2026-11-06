# Unary WebSocket limits and metrics (`/api/ws/unary`)

The unary multiplexer (`unary_over_ws` flag, default off) runs read RPCs over one WebSocket per tab. These are the limits that keep a stuck handler or a hostile peer from exhausting the server. Source: `server/services/ws_unary_bridge.go`.

## How a call ends

- **CANCEL or deadline:** the call's slot is released at once. The client deadline (`timeoutMs`) becomes the handler context deadline, clamped to 5 minutes (also when the client sends none). On a deadline the server queues `deadline_exceeded` before releasing the id.
- **Zombies:** a handler that ignores its context keeps running after its slot is gone. It counts as a zombie only if still running 250 ms after CANCEL/deadline, so a burst of cancels with cooperative handlers is never counted.
- **Zombie cap:** more than 32 zombies on one socket closes it (close code 1001). The client reconnects and replays replayable reads over HTTP. This counts toward the client's 3-strike breaker.

## Limits

| Limit | Value | Override | On exceed |
|---|---|---|---|
| In-flight calls per socket | 64 | `STAPLER_SQUAD_WS_UNARY_MAX_INFLIGHT` (clamped to 256) | `resource_exhausted`; client replays over HTTP |
| Zombies per socket | 32 | none | socket closed |
| Unary sockets per process | 64 | none | upgrade refused, 503 |
| Running zombies per process | 128 | none | new upgrades refused, 503 (existing sockets and HTTP are unaffected) |
| Buffered response bytes per process | 256 MiB, held until the last CHUNK is written or the socket closes | none | `resource_exhausted` for that call; client replays over HTTP |
| One buffered response | 32 MiB | none | `resource_exhausted` |
| Call deadline | 5 min max | none | `deadline_exceeded` |

A slow-reading peer can hold budget for up to the 10 s write deadline; other calls then fall back to HTTP. A peer that can open a socket can park zombies by sending REQUEST and CANCEL for a context-ignoring method (`GetSessionDiff`, `GetVCSStatus`) and lock other peers out of new sockets until the handlers return. The real fix is threading `ctx` into those handlers.

## Metrics

Exposed through OpenTelemetry and `GET /actuator/ws-unary`:

- `ws_unary_zombies` (gauge): abandoned handlers still running past the grace.
- `ws_unary_abandoned_total{reason}`: `cancel`, `deadline`, `shutdown` (counted only if still running after the grace) and `zombie_cap` (once per closed socket).
- `ws_unary_upgrade_denied_total{listener,reason}`: includes `socket_limit` and `zombie_limit`.
- `ws_unary_inflight`, `ws_unary_calls_total`, `ws_unary_denied_total`: as before.

Client counter `ws_unary_fallback_total{reason}` gains `ws-resource-exhausted`, `ws-unavailable` and `ws-send-queue-full`.
