# Client network telemetry

Browser-reported RPC, WebSocket and page-load metrics, plus the server's unary
WebSocket bridge metrics. Dashboard: `ssq-client-network`
(`docs/observability/grafana/dashboards/client-network.json`).

## Path

```
SPA (rpcTiming interceptor, transports, WebVitalsReporter)
  -> HttpAnalyticsProvider (25 events / 2s batches, 200-event queue, pagehide beacon)
  -> POST /api/analytics            (existing: 1000 req/min, 512 KB body, <=100 events)
  -> analytics.WithQualityMetrics   (server/analytics/client_metrics.go)
  -> OTel instruments -> collector -> VictoriaMetrics
```

No new endpoint or auth surface. Events named `cm.*` exist only to feed metrics
and are not stored in the analytics table; `rpc.*`, `web_vital.*` and
`ws_unary_fallback_total` events are still stored and also feed metrics. Metrics
need `OTEL_ENABLED=true` on the server.

## Label bounds (the sender is untrusted)

| Label | Bound |
|---|---|
| `method` | must match `^[A-Za-z][A-Za-z0-9]{0,63}$`; at most 192 distinct values per process, then `other` |
| `page` | first URL path segment only (`/sessions/<id>` -> `sessions`); `^[a-z0-9-]{1,24}$`; at most 32 distinct, then `other` |
| `transport` | `ws`, `http`, `batch`, `stream` (server-streaming; duration is time-to-headers), else `unknown` |
| `code` | the 17 Connect code names in snake_case, else `unknown` |
| `close_code` | integer 1000-4999, else `none` |
| everything else | fixed enums; unknown values become `unknown` |

Durations clamp to 0-10 min; count-like values (in-flight, pending, batch size) clamp to 0-64.
Never put session ids, URLs, titles or free text in an event label: only the keys above are read.

## Client metrics (`client_*`)

OTel names as registered; VictoriaMetrics appends the unit and `_bucket`/`_count`/`_sum`/`_total`.

| VictoriaMetrics name | Type | Labels | Emitted by |
|---|---|---|---|
| `client_rpc_duration_milliseconds_*` | histogram | `method`, `transport`, `code` | `rpc.<Method>` event from `lib/telemetry/rpcTiming.ts`; one per coalesced flight, including browser queueing |
| `client_rpc_network_queue_delay_milliseconds_*` | histogram | `transport` | same event, Resource Timing `requestStart - fetchStart` |
| `client_rpc_inflight_at_start_*` | histogram | `transport` | same event; unary calls already in flight in the tab |
| `client_rpc_duplicate_inflight_total` | counter | `method` | same event (`dup=1`): identical request already in flight, i.e. coalescing missed it |
| `client_rpc_coalesce_total` | counter | `method`, `outcome` (`started`, `joined`) | `cm.coalesce` from `lib/api/coalescingInterceptor.ts` |
| `client_ws_events_total` | counter | `kind` (`unary`, `watch`), `event` (`open`, `close`, `error`, `reconnect`), `close_code` | `cm.ws` from the unary and watch WebSocket transports |
| `client_ws_pending_at_send_*` | histogram | none | `cm.ws_send`: sent-unanswered plus queued calls when a call was submitted |
| `client_ws_send_queue_delay_milliseconds_*` | histogram | none | `cm.ws_send`: submit until frame written to the socket |
| `client_ws_unary_fallback_total` | counter | `reason` | `ws_unary_fallback_total` event from `wsUnaryPolicy.ts` |
| `client_batch_size_*` | histogram | none | `cm.batch` from `lib/api/batchTransport.ts` |
| `client_batch_fallback_total` | counter | `reason` (`request-failed`, `server-declined`) | `cm.batch_fallback` |
| `client_web_vital_*` | histogram | `vital` (`lcp`, `inp`, `cls`, `fcp`, `ttfb`), `rating`, `page` | `web_vital.*` from `WebVitalsReporter` (ms; CLS unitless) |
| `client_long_task_duration_milliseconds_*` | histogram | `page` | `cm.longtask` from `lib/telemetry/longTasks.ts`; capped at 30 per minute per tab |
| `client_metrics_rejected_total` | counter | `reason` | server: unknown `cm.*` event or malformed web vital |

Round-trip time for a unary WebSocket call is `client_rpc_duration` with
`transport="ws"` (send to reply, includes `client_ws_send_queue_delay`).

## Server WebSocket bridge metrics (`ws_unary_*`)

`server/services/ws_unary_metrics.go`, `ws_unary_bridge.go`. `listener` is a server constant
(`local`/`remote`); `method` is the allowlisted RPC name or `other`.

| VictoriaMetrics name | Type | Labels |
|---|---|---|
| `ws_unary_connections_open` | up/down counter | `listener` |
| `ws_unary_connections_closed_total` | counter | `listener`, `reason` (`peer_close`, `read_error`, `write_error`, `ping_failed`, `protocol`, `shutdown`) |
| `ws_unary_write_errors_total` | counter | `listener` |
| `ws_unary_ping_rtt_milliseconds_*` | histogram | `listener` |
| `ws_unary_call_duration_milliseconds_*` | histogram | `method` |
| `ws_unary_calls_total`, `ws_unary_denied_total`, `ws_unary_upgrade_denied_total`, `ws_unary_inflight` | existing | see `ws_unary_metrics.go` |

## Known gaps

- `rpc.*` events pass through the 200-event client queue, which drops the oldest under a long
  outage, so client counts can under-report during one. Compare with `rpc_server_duration_milliseconds`.
- The terminal stream WebSocket (`connectrpc_websocket.go`) has no per-message metrics yet; only
  the unary bridge does.
- Streaming RPC `client_rpc_duration` (`transport="stream"`) measures time-to-headers, not stream life.
- Batch metrics are only emitted when `batch_unary` is on and two batch-eligible reads land in one 4 ms window.
