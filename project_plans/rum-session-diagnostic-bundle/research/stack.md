# RUM Session Diagnostic Bundle — Technology Stack Research

## 1. ZIP Archive: Go Packages

**stdlib `archive/zip`** is available without any new dependency — part of the Go standard library, correct choice for creating ZIP archives in-memory or to disk.

Additionally, **`github.com/klauspost/compress v1.19.1`** is already in `go.mod`, providing `zstd`, `gzip`, and (via indirect) `pgzip` sub-packages. `pgzip` (`github.com/klauspost/pgzip v1.2.6`) is listed as an indirect dependency but is not directly imported anywhere in the codebase today.

No external ZIP library exists in the module graph; `archive/zip` + `github.com/klauspost/compress/zstd` cover all compression needs.

**Relevant go.mod entries:**
```
github.com/klauspost/compress v1.19.1
github.com/klauspost/pgzip v1.2.6  // indirect
```

## 2. Existing ConnectRPC / Protobuf Setup

**Canonical package:** `connectrpc.com/connect v1.20.0` (the current, non-deprecated path).
Legacy alias `github.com/bufbuild/connect-go v1.10.0` is also present but should not be used for new code.

**Existing ExportRules RPC — canonical pattern** (`proto/session/v1/session.proto` line 246):
```proto
rpc ExportRules(ExportRulesRequest) returns (ExportRulesResponse) {}
```

**Go implementation pattern** (`server/services/rules_service.go`):
```go
func (rs *RulesService) ExportRules(
    ctx context.Context,
    req *connect.Request[sessionv1.ExportRulesRequest],
) (*connect.Response[sessionv1.ExportRulesResponse], error) {
    ...
    return connect.NewResponse(&sessionv1.ExportRulesResponse{YamlContent: string(out)}), nil
}
```

The delegate wires through `session_service_delegates.go`:
```go
func (s *SessionService) ExportRules(...) (*connect.Response[sessionv1.ExportRulesResponse], error) {
    return s.rulesSvc.ExportRules(ctx, req)
}
```

**Proto file location:** `proto/session/v1/session.proto`
**Service name:** `SessionService`
**Implementation:** `server/services/session_service_delegates.go` → `server/services/rules_service.go`

## 3. Large Binary/Bytes Responses in ConnectRPC

No existing RPC returns a raw binary blob for download today. The `ExportRules` RPC returns `string yaml_content` — the entire YAML fits in memory.

The only `bytes` field in session.proto is `bytes raw_bytes = 8` in the `EscapeEvent` message (line 3395), which is a message field, not an RPC response pattern for streaming downloads.

**For the diagnostic bundle**, two approaches are viable based on the existing codebase:
1. **Unary RPC returning `bytes zip_content = 1`** — mirrors ExportRules but with bytes instead of string; appropriate for bundles up to ~50 MB, but ConnectRPC's default 4 MB message size limit applies.
2. **Server-streaming RPC** — `rpc ExportDiagnosticBundle(req) returns (stream BundleChunk)` — WatchSessions (line 45) and StreamTerminal (line 49) establish the server-streaming and bidi-streaming patterns. No existing bytes-streaming RPC for file download exists yet.

A plain HTTP handler (like `AnalyticsHandler`) that writes `Content-Type: application/zip` with `Content-Disposition: attachment` is also viable and avoids ConnectRPC framing overhead for large blobs.

## 4. Go OTel Packages and Initialization

**go.mod entries:**
```
go.opentelemetry.io/otel v1.46.0
go.opentelemetry.io/otel/sdk v1.46.0
go.opentelemetry.io/otel/sdk/metric v1.46.0
go.opentelemetry.io/otel/trace v1.46.0
go.opentelemetry.io/otel/metric v1.46.0
go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc v1.39.0
go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetricgrpc v1.44.0
connectrpc.com/otelconnect v0.8.0
go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp v0.70.0
go.opentelemetry.io/otelc/pkg v0.0.0-20260915134854-341ba5764ca3
```

**Initialization** (`telemetry/telemetry.go`):
- `ServiceName = "stapler-squad"`
- `DefaultOTLPEndpoint = "localhost:4317"` (gRPC)
- `DefaultOTLPHTTPEndpoint = "http://localhost:4318"` (HTTP, for browser relay)
- TracerProvider: `sdktrace.NewTracerProvider(...)` → `otel.SetTracerProvider(tp)`
- MeterProvider: `sdkmetric.NewMeterProvider(...)` → `otel.SetMeterProvider(mp)`
- Propagator: `otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(...))`
- Returns `*Telemetry` struct with `.tracer` (`trace.Tracer`) and `.meter` (`metric.Meter`)

New RPC handlers should call `otel.Tracer("stapler-squad")` or receive the telemetry instance via DI.

## 5. Frontend Export Patterns

**`web-app/src/lib/hooks/useExportRules.ts`** — canonical hook pattern for ConnectRPC-triggered file download:
```typescript
import { createClient } from "@connectrpc/connect";
import { SessionService, ExportRulesRequestSchema } from "@/gen/session/v1/session_pb";
import { getConnectTransport } from "@/lib/api/transport";

const client = createClient(SessionService, getConnectTransport());
const resp = await client.exportRules(req);
const blob = new Blob([resp.yamlContent], { type: "text/yaml" });
const url = URL.createObjectURL(blob);
const a = document.createElement("a");
a.href = url; a.download = "rules.yaml"; a.click();
URL.revokeObjectURL(url);
```

**`web-app/src/components/logs/ExportButton.tsx`** — generic multi-format export button (JSON/CSV/TXT) using the same `Blob` + `<a download>` pattern. This component works entirely client-side from in-memory data.

**Transport:** `getConnectTransport()` returns a `createConnectTransport` singleton pointing to `getApiBaseUrl()`. For a binary download (`bytes zip_content`), the frontend would need to pass `{type: "application/zip"}` to `Blob`. Alternatively, a plain `fetch()` to an HTTP endpoint and `response.blob()` avoids ConnectRPC proto framing for large binaries.

**Generated types location:** `web-app/src/gen/session/v1/session_pb.ts`

## 6. Zstd/Compression Library for Scrollback

**Package:** `github.com/klauspost/compress/zstd` (from `github.com/klauspost/compress v1.19.1`)

**Usage** (`session/scrollback/storage.go` and `session/scrollback/fork.go`):
```go
import "github.com/klauspost/compress/zstd"

// Encoder (write path)
encoder, _ := zstd.NewWriter(nil, zstd.WithEncoderLevel(level))

// Decoder (read path)
dec, err := zstd.NewReader(srcFile)
```

**Default config** (`session/scrollback/manager.go`):
```go
CompressionType:  "zstd",
CompressionLevel: 3,   // SpeedFastest tier
```

Supported compression types: `"none"`, `"zstd"`, `"gzip"`. The scrollback storage file (`FileScrollbackStorage`) reads compressed data with a pooled decoder and writes with a pooled encoder.

**For the diagnostic bundle**, scrollback can be included as-is (raw `.zstd` file) or decompressed on the fly using `zstd.NewReader` and re-packed into the ZIP. Including the raw compressed bytes saves bandwidth.

## 7. Analytics Event Query Patterns (session_id)

**Provider:** `server/analytics/sqlite_provider.go` — `SQLiteAnalyticsProvider` backed by `entgo.io/ent` ORM.

**Schema** (`session/ent/schema/analytics_event.go`):
- `session_id string` — optional, has a dedicated B-tree index
- `event_name`, `event_category`, `created_at`, `duration_ms`, `page`, `component`, `labels`

**Ent predicate package:** `github.com/tstapler/stapler-squad/session/ent/analyticsevent`

**Query-by-session_id pattern** (derived from existing code in `analytics_handler.go` and `retention.go`):
```go
events, err := client.AnalyticsEvent.Query().
    Where(analyticsevent.SessionIDEQ(sessionID)).
    Order(analyticsevent.ByCreatedAt()).
    All(ctx)
```

The `analyticsevent.SessionIDEQ(sessionID)` predicate is generated by ent and available at `github.com/tstapler/stapler-squad/session/ent/analyticsevent`. The `Record()` method in `sqlite_provider.go` shows the write pattern; the read pattern using `client.AnalyticsEvent.Query().Where(...)` follows the same ent DSL.

**EscapeEvent** (terminal analytics, `session/ent/schema/escape_event.go`) also has a `session_id` field and index — query pattern is identical.

## Summary

- **ZIP**: Use stdlib `archive/zip`; `github.com/klauspost/compress/zstd` already in-tree for scrollback decompression if needed.
- **ConnectRPC**: Unary RPC pattern established by `ExportRules` (`connectrpc.com/connect v1.20.0`); for large binary payloads, a plain HTTP handler writing `application/zip` is simpler and avoids proto framing.
- **Frontend**: `useExportRules.ts` hook provides the exact pattern to replicate — `createClient` + ConnectRPC call + `Blob` + `<a download>` trigger; or `fetch()` + `response.blob()` for an HTTP endpoint.
