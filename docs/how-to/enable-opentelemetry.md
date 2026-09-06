# OpenTelemetry Observability

Stapler Squad supports OpenTelemetry instrumentation for APM integration (Datadog, etc.). Disabled by default.

## Environment Variables

```bash
OTEL_ENABLED=true ./stapler-squad
DD_TRACE_ENABLED=true ./stapler-squad

# Configure OTLP endpoint (default: localhost:4317)
OTEL_EXPORTER_OTLP_ENDPOINT=localhost:4317 OTEL_ENABLED=true ./stapler-squad

# Set environment and version for trace metadata
OTEL_SERVICE_ENVIRONMENT=production OTEL_SERVICE_VERSION=1.0.0 OTEL_ENABLED=true ./stapler-squad
```

## Datadog Agent Configuration (for OTLP ingestion)

```yaml
# /etc/datadog-agent/datadog.yaml
otlp_config:
  receiver:
    protocols:
      grpc:
        endpoint: 0.0.0.0:4317
```

Both OTLP gRPC exporters (`otlptracegrpc.WithCompressor("gzip")` /
`otlpmetricgrpc.WithCompressor("gzip")` in `telemetry/telemetry.go`) send
gzip-compressed payloads. This needs no Agent-side config change — gRPC
compression is negotiated by the client and decoded transparently by the
receiver.

## Instrumented Operations

- All HTTP requests (via otelhttp middleware)
- All ConnectRPC endpoints (via otelconnect interceptor)
- History cache operations (cache hit/miss, load duration)
- Search engine operations (sync, search duration, result count)
- `safeexec.sigkill_escalations` (counter) — increments once per confirmed SIGKILL escalation in `CommandContextPG` (i.e. the process group ignored SIGTERM within `sigkillGrace`); not incremented on ESRCH/already-exited
- `cgroup_memory_*` (gauges/counters, Linux only, `telemetry/cgroup_linux.go`) — reads the running process's own cgroup v2 memory files fresh every collection: `cgroup_memory_current_bytes`/`_high_bytes`/`_max_bytes` (`memory.current`/`.high`/`.max`), `cgroup_memory_events_{high,max,oom,oom_kill}_total` (`memory.events`, kernel-cumulative), `cgroup_memory_pressure_{some,full}_avg10` (`memory.pressure` PSI, % stalled over the last 10s). Added 2026-08-25 to get dashboard visibility into `MemoryHigh`/`MemoryMax` throttling (see `scripts/install-service.sh`'s cgroup cap comment) while investigating live-instance terminal type-ahead reports — correlate these against symptom reports before deciding whether to change the caps.
- `http.server.connections_open` / `http.server.connections_hijacked_total` and `rpc.server_streams.open` — see "Connection concurrency metrics" below.

## Connection concurrency metrics

Added 2026-09 after root-causing a `ConnectError: [deadline_exceeded]` class of
bug: the browser reported unary RPCs (`GetSessionDiff`, `ListWorkflows`,
`LogUserInteraction`, ...) timing out, but Tempo showed **zero** server-side
spans for them — the request never reached the server in time. Root cause:
this server speaks plain HTTP/1.1 (`curl -v --http2 http://localhost:8543/health`
shows "using HTTP/1.x"), so a browser's 6-connections-per-origin cap can be
exhausted by the server-streaming `Watch*` RPCs (`WatchSessions`,
`WatchReviewQueue`, `WatchBacklogItems`, `WatchInsights`,
`WatchUnfinishedWork`) plus the terminal WebSocket, silently queuing any new
unary call *inside the browser* until it blows its client-side deadline. The
fix (extending `StreamingWSBridge` — see `server/services/ws_stream_bridge.go`
— to cover the remaining `Watch*` RPCs, so they share one WebSocket
connection instead of each holding its own HTTP/1.1 connection open) reduces
raw connection pressure; these metrics make that visible and catch a
regression early instead of requiring another round of manual Tempo queries:

| Metric | Type | Labels | Source |
|---|---|---|---|
| `http.server.connections_open` | Observable gauge | — | `server/http_connection_metrics.go`, fed by the main listener's `(*http.Server).ConnState` hook (wired in `server/server.go`'s `newServerBase`). Current count of connections net/http still tracks as plain HTTP/1.1 (new/active/idle) — excludes hijacked ones. |
| `http.server.connections_hijacked_total` | Observable counter | — | Same file. Cumulative count of connections hijacked off net/http's tracking — in this server, exclusively WebSocket upgrades (`StateHijacked` is `net/http`'s terminal state for a hijacked connection, so this is the only place to count them). |
| `rpc.server_streams.open` | Observable gauge | `method` (`WatchSessions`, `WatchReviewQueue`, `WatchBacklogItems`, `WatchInsights`, `WatchUnfinishedWork`, `StreamTerminal`) | `server/services/watch_stream_metrics.go`'s `TrackOpenStream`, called at the top of each Watch* handler's body and around the terminal stream's `HandleWebSocket` call, with a `defer`/paired call decrementing on stream close. |

Graph `rpc.server_streams.open` summed by `method` to answer "how many
long-lived streams does one browser tab hold open right now" directly. A
sustained `http.server.connections_open` at or near 6 alongside high
`rpc.server_streams.open` is the exact signature of this bug class — a new
unary RPC has nowhere left to go.

## Trace Attributes

| Attribute | Description |
|---|---|
| `session.id`, `session.title`, `session.status` | Session context |
| `history.entry_count` | History loading metrics |
| `search.query`, `search.result_count`, `search.duration_ms` | Search metrics |
| `cache.hit`, `cache.refresh_duration_ms` | Cache performance |
| `sync.sessions_added`, `sync.sessions_updated` | Index sync metrics |
| `network.queue_time_ms` (browser spans only) | See "Browser network queueing delay" below |

## Browser tracing (web-app)

`web-app/next.config.ts` sets `output: "export"` — the Next.js app is a fully
static bundle embedded into the Go binary (`server/web/dist`), so there is no
Next.js/Node server to run [Next.js's own `instrumentation.ts`
guide](https://nextjs.org/docs/app/guides/open-telemetry) in. Instead, the
browser runs the OpenTelemetry **Web** SDK directly
(`web-app/src/lib/telemetry/otel-init.ts`), instrumenting page loads and
`fetch` calls (including the app's own ConnectRPC traffic), and exports spans
to the Go server rather than straight to a collector — avoiding CORS and
keeping the collector off the public network.

Disabled by default, gated the same way as the server:

```bash
# Enable browser tracing. NEXT_PUBLIC_* vars are baked in at `next build`
# time (static export, no server-side env reads), so this must be set before
# `make web-build` / `pnpm run build`, not at runtime like the server's vars.
NEXT_PUBLIC_OTEL_ENABLED=true make web-build

# Override the export endpoint (default: same-origin "/api/otel/v1/traces",
# proxied by the Go server — see below). Only needed for local dev, where
# `next dev` runs on a different origin (:3001) than the Go server (:8543).
NEXT_PUBLIC_OTEL_EXPORTER_OTLP_TRACES_ENDPOINT=http://localhost:8543/api/otel/v1/traces NEXT_PUBLIC_OTEL_ENABLED=true make web-build
```

The Go server relays browser export requests to the same collector it
exports to itself (`server/handlers/otel_proxy_handler.go`), registered at
`POST /api/otel/v1/traces` and `POST /api/otel/v1/metrics` only when
`OTEL_ENABLED=true` server-side too. Point it at an OTLP/**HTTP** endpoint —
distinct from `OTEL_EXPORTER_OTLP_ENDPOINT` above, which is gRPC:

```bash
# Configure the HTTP collector endpoint the proxy relays to (default: http://localhost:4318)
OTEL_EXPORTER_OTLP_HTTP_ENDPOINT=http://localhost:4318 OTEL_ENABLED=true ./stapler-squad
```

This requires the collector to also expose an HTTP receiver alongside the
gRPC one already shown above — e.g. for the Datadog Agent:

```yaml
# /etc/datadog-agent/datadog.yaml
otlp_config:
  receiver:
    protocols:
      grpc:
        endpoint: 0.0.0.0:4317
      http:
        endpoint: 0.0.0.0:4318
```

Because the browser always talks to the Go server (same-origin), the
collector itself never needs CORS configuration — only the Go↔collector hop
is cross-network, and that's a plain server-to-server HTTP call.

### Browser network queueing delay

`web-app/src/lib/telemetry/rpcTiming.ts`'s ConnectRPC interceptor (see
"Connection concurrency metrics" above for the bug class this exists to
catch) computes a proxy for how long each unary RPC's underlying fetch spent
queued or negotiating a connection before the browser put bytes on the wire,
using the Resource Timing API: `requestStart - fetchStart` on the
`PerformanceResourceTiming` entry matching the call's URL. A request stuck
behind the browser's 6-connections-per-origin cap shows a large gap here even
though the server never saw it start late — which is exactly what
distinguishes this from genuine server-side latency (already visible as a
long span duration).

The value is set as the `network.queue_time_ms` attribute on the currently
active OTel span (normally the `fetch` span `otel-init.ts`'s
`FetchInstrumentation` created for that exact call — active through the
fetch's promise chain via `ZoneContextManager`'s context propagation), so a
queued request is visibly distinguishable from a slow server-side one
directly in a Tempo/Grafana trace view instead of requiring a manual
Resource Timing correlation by hand. It's also included in the
`performance.measure(...)` detail (`queueDelayMs`, visible in Chrome
DevTools' Performance panel) and, when an analytics provider is configured,
as the `networkQueueDelayMs` label on the `rpc.<Method>` analytics event —
omitted entirely (not zero) when no matching resource-timing entry is found,
e.g. in non-browser test environments.

## Compile-time auto-instrumentation (opt-in)

A separate, opt-in build (`make build-otel-auto` → `stapler-squad-otel`) uses `otelc` compile-time weaving to add spans for surfaces the hand-written instrumentation above doesn't cover (e.g. ent's `database/sql` queries), without any source change. See `docs/how-to/enable-otel-auto-instrumentation.md` for the install/build/run recipe and the validated operational findings.
