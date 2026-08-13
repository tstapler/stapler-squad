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

## Trace Attributes

| Attribute | Description |
|---|---|
| `session.id`, `session.title`, `session.status` | Session context |
| `history.entry_count` | History loading metrics |
| `search.query`, `search.result_count`, `search.duration_ms` | Search metrics |
| `cache.hit`, `cache.refresh_duration_ms` | Cache performance |
| `sync.sessions_added`, `sync.sessions_updated` | Index sync metrics |
