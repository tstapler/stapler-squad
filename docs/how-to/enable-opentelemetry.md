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

## Compile-time auto-instrumentation (opt-in)

A separate, opt-in build (`make build-otel-auto` → `stapler-squad-otel`) uses `otelc` compile-time weaving to add spans for surfaces the hand-written instrumentation above doesn't cover (e.g. ent's `database/sql` queries), without any source change. See `docs/how-to/enable-otel-auto-instrumentation.md` for the install/build/run recipe and the validated operational findings.
