# Client network telemetry

Browser-reported RPC, WebSocket and page-load metrics, plus the server's unary
WebSocket bridge metrics. Dashboard: `ssq-client-network`
(`docs/observability/grafana/dashboards/client-network.json`).

## Path

```
SPA (rpcTiming interceptor, transports, WebVitalsReporter)
  -> HttpAnalyticsProvider (25 events / 2s batches, 200-event queue, pagehide beacon)
  -> POST /api/analytics            (existing: 1000 req/min, 512 KB body, <=100 events;
                                      plus a 60000 events/min budget and a 30000 cm.* events/min budget;
                                      excess gets 429)
  -> analytics.WithQualityMetrics   (server/analytics/client_metrics.go)
  -> OTel instruments -> collector -> VictoriaMetrics
```

No new endpoint or auth surface. The limits are global on purpose: local browsers and same-box reverse proxies all arrive from a
loopback address, so a per-peer limit would only be a lower limit shared by every tab. Sizing: a tab
flushes about every 20s (3 requests/min) or at 100 queued events, so 10 tabs send about 30 requests/min
against the 1000/min request bucket; one RPC emits about 2.5 events, so 50 RPC/s across tabs is about
7,500 events/min against the 60,000/min event budget (8x headroom). Refusals increment
`client_metrics_rate_limited_total{scope}` (`global_requests`, `global_events`, `cm_budget`) and log at
most once a minute, because the browser swallows the 429. A flooding client can still exhaust the
shared budget; it cannot add labels or series.

Events named `cm.*` exist only to feed metrics
and are not stored in the analytics table; `rpc.*`, `web_vital.*` and
`ws_unary_fallback_total` events are still stored and also feed metrics. Metrics
need `OTEL_ENABLED=true` on the server.

## Label bounds (the sender is untrusted)

| Label | Bound |
|---|---|
| `method` | must be an RPC method compiled into the binary (read from the proto registry), else `other`; junk names cannot use up the label budget |
| `page` | first URL path segment, only if listed in `server/analytics/client_pages.json`, else `other`. A jest test fails when a route in `route-meta.ts` is missing from that file |
| `transport` | `ws`, `http`, `batch`, `stream` (server-streaming; duration is time-to-headers), else `unknown`. The dashboard's `All` for `$transport` is `ws\|http\|batch`, so `stream` and `unknown` are hidden unless selected |
| `outcome` | class of the Connect code: `ok`, `canceled`, `deadline_exceeded`, `saturated` (`unavailable`, `resource_exhausted`), `error` |
| `code` | the 17 Connect code names in snake_case, else `unknown`; only on `client_rpc_codes_total`, never combined with `method` |
| `close_code` | allow-list of the codes browsers and the bridge produce (1000-1003, 1005, 1006, 1008, 1009, 1011-1013), `none` when absent, else `other`; the browser records the close once per socket, even after an error event |
| everything else | fixed enums; unknown values become `unknown` |

Durations clamp to 0-10 min; count-like values (in-flight, pending, batch size) clamp to 0-64.
Never put session ids, URLs, titles or free text in an event label: only the keys above are read.

## Client metrics (`client_*`)

OTel names as registered; VictoriaMetrics appends the unit and `_bucket`/`_count`/`_sum`/`_total`.

| VictoriaMetrics name | Type | Labels | Emitted by |
|---|---|---|---|
| `client_rpc_duration_milliseconds_*` | histogram | `method`, `transport`, `outcome` | `rpc.<Method>` event from `lib/telemetry/rpcTiming.ts`; one per coalesced flight, including browser queueing |
| `client_rpc_codes_total` | counter | `code` | same event; full Connect code without the method label |
| `client_rpc_network_queue_delay_milliseconds_*` | histogram | `transport` | same event, Resource Timing `requestStart - fetchStart` |
| `client_rpc_inflight_at_start_*` | histogram | `transport` | same event; calls already in flight in the tab, from a module-wide counter (includes stream calls until their headers arrive, and calls on the non-shared transport in `useSessionService`) |
| `client_rpc_duplicate_inflight_total` | counter | `method`, `coalescible` | same event (`dup=1`), shared transport and read-shaped methods only: an identical read was already in flight. `coalescible=true`: coalescing should have merged it (a miss); `false`: the method is not coalescible, so it is a candidate to add |
| `client_rpc_coalesce_total` | counter | `method`, `outcome` (`joined`) | `cm.coalesce` from `lib/api/coalescingInterceptor.ts`. Only joins are beaconed; flights started are the `rpc.*` counts |
| `client_ws_events_total` | counter | `kind` (`unary`, `watch`), `event` (`open`, `close`, `error`, `reconnect`), `close_code` | `cm.ws` from the unary and watch WebSocket transports |
| `client_ws_pending_at_send_*` | histogram | none | `cm.ws_send`: sent-unanswered plus queued calls when a call was submitted |
| `client_ws_send_queue_delay_milliseconds_*` | histogram | none | `cm.ws_send`: submit until frame written to the socket |
| `client_ws_unary_fallback_total` | counter | `reason` | `ws_unary_fallback_total` event from `wsUnaryPolicy.ts` |
| `client_batch_size_*` | histogram | none | `cm.batch` from `lib/api/batchTransport.ts` |
| `client_batch_fallback_total` | counter | `reason` (`request-failed`, `server-declined`) | `cm.batch_fallback` |
| `client_web_vital_*` | histogram | `vital` (`lcp`, `inp`, `cls`, `fcp`, `ttfb`), `rating`, `page` | `web_vital.*` from `WebVitalsReporter` (ms; CLS unitless) |
| `client_long_task_duration_milliseconds_*` | histogram | `page` | `cm.longtask` from `lib/telemetry/longTasks.ts`; capped at 30 per minute per tab |
| `client_metrics_rate_limited_total` | counter | `scope` | `server/handlers/analytics_rate_budget.go`: batches refused with 429 |
| `client_metrics_rejected_total` | counter | `reason` (`unknown_event`, `bad_web_vital`, `missing_duration`) | server: events the allow-list refused. Old cached clients whose `rpc.*` events lack `method`/`transport` land as `other`/`unknown` rather than being rejected |

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

- Events are lost when `POST /api/analytics` fails: `HttpAnalyticsProvider.flush()` takes the
  queue before sending and swallows fetch errors, so a failed POST (or a 429) drops that chunk.
  The 200-event queue cap additionally drops the oldest events if the page produces more between
  flushes. Compare against `rpc_server_duration_milliseconds` when a count looks low.
- The terminal stream WebSocket (`connectrpc_websocket.go`) has no per-message metrics yet; only
  the unary bridge does.
- Streaming RPC `client_rpc_duration` (`transport="stream"`) measures time-to-headers, not stream life.
- Batch metrics are only emitted when `batch_unary` is on and two batch-eligible reads land in one 4 ms window.
- `client_rpc_duplicate_inflight_total` only sees the shared transport; calls through
  `useSessionService`'s own transport (`getSession`, `listSessions`, `listShells`, ...) bypass the
  coalescing and batching interceptors and are not checked for duplicates.
- `cm.*` events are still beaconed when the server runs without `OTEL_ENABLED`; they are accepted
  and discarded. A flag telling the client to stop is not implemented.
- The long-task observer uses `buffered: true`, so a remount of `WebVitalsReporter` replays the
  buffered tasks (visible in dev under React StrictMode; production mounts once).
- Metric truthfulness is unauthenticated: a client that can reach `/api/analytics` can inflate
  counts within the shared budgets. Labels and series counts stay bounded regardless.
