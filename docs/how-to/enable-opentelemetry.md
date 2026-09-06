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

## Trace Attributes

| Attribute | Description |
|---|---|
| `session.id`, `session.title`, `session.status` | Session context |
| `history.entry_count` | History loading metrics |
| `search.query`, `search.result_count`, `search.duration_ms` | Search metrics |
| `cache.hit`, `cache.refresh_duration_ms` | Cache performance |
| `sync.sessions_added`, `sync.sessions_updated` | Index sync metrics |

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

## Compile-time auto-instrumentation (opt-in)

A separate, opt-in build (`make build-otel-auto` → `stapler-squad-otel`) uses `otelc` compile-time weaving to add spans for surfaces the hand-written instrumentation above doesn't cover (e.g. ent's `database/sql` queries), without any source change. See `docs/how-to/enable-otel-auto-instrumentation.md` for the install/build/run recipe and the validated operational findings.
