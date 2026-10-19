# RUM Session Diagnostic Bundle — Build vs. Buy Research

## 1. ZIP Bundle Assembly: stdlib `archive/zip` vs. `klauspost/compress/zip`

**Verdict: use `github.com/klauspost/compress/zip` (build, zero extra cost)**

`github.com/klauspost/compress v1.19.1` is already a **direct** dependency in the root `go.mod`. The codebase uses it today for HTTP response compression:

- `server/middleware/gzip.go` — imports `github.com/klauspost/compress/gzip` and `github.com/klauspost/compress/zstd` via alias `kgzip`
- `server/protocol/compression.go` — same pattern
- `session/scrollback/storage.go` — uses `github.com/klauspost/compress/zstd`

`klauspost/compress/zip` is a drop-in replacement for `archive/zip` with the same API, backed by klauspost's SIMD-accelerated deflate (~20–40% faster compression, especially for the large scrollback payloads that dominate bundle size). Since the package is already vendored/downloaded, there is **no new module to vet or pin**.

Recommendation: import as `kzip "github.com/klauspost/compress/zip"` to match the existing `kgzip` alias convention.

## 2. Browser RUM Libraries: OTel Web SDK (existing) vs. Dedicated RUM Products

**Verdict: extend existing OTel setup (build); no third-party RUM product needed**

`web-app/package.json` already carries a complete OTel Web stack:

| Package | Version |
|---|---|
| `@opentelemetry/api` | 1.9.1 |
| `@opentelemetry/sdk-trace-web` | 2.11.0 |
| `@opentelemetry/exporter-trace-otlp-http` | 0.222.0 |
| `@opentelemetry/instrumentation` | 0.222.0 |
| `@opentelemetry/instrumentation-document-load` | 0.67.0 |
| `@opentelemetry/instrumentation-fetch` | 0.222.0 |
| `@opentelemetry/context-zone` | 2.11.0 |
| `@opentelemetry/resources` | 2.11.0 |
| `@opentelemetry/semantic-conventions` | 1.43.0 |

`web-app/src/lib/telemetry/otel-init.ts` wires these into a `WebTracerProvider` with a `BatchSpanProcessor` pointing at `/api/otel/v1/traces` (the in-process Go OTLP proxy at `server/handlers/otel_proxy_handler.go`).

**No DataDog, Sentry, LogRocket, or FullStory packages are present.**

Dedicated RUM products add session replay, rage-click detection, and SaaS dashboards — none of which are required for the diagnostic bundle. The `session.uuid` span attribute needed for backlog hand-off is a one-line addition to the existing OTel resource or a custom span attribute set at session start.

When OTel is disabled the provider is simply not registered; all `tracer.startSpan()` calls no-op via the OTel API's default no-op implementation — graceful degradation is already built into the OTel API contract.

## 3. Session Diagnostics Bundle Format: Custom JSON vs. HAR vs. OTLP JSON

**Verdict: custom JSON manifest inside ZIP (build), modelled on `DebugSnapshot`**

`server/services/debug_snapshot.go` already defines:

```go
type DebugSnapshot struct {
    Version    int                `json:"version"`
    Timestamp  time.Time          `json:"timestamp"`
    Note       string             `json:"note,omitempty"`
    Server     ServerInfo         `json:"server"`
    Sessions   []SessionSnapshot  `json:"sessions"`
    // ...
}
```

This is the natural template. The ZIP bundle manifest (`manifest.json`) should follow the same versioned, timestamped envelope, adding a `session_uuid` field and sections for analytics SQLite export, context history, and artifact metadata.

**Why not HAR?**
HAR is a browser DevTools export format for HTTP request/response pairs. It captures network traffic well but has no concept of session metadata, scrollback, or ent DB records. It would cover only a slice of what the bundle needs.

**Why not OTLP JSON?**
OTLP JSON (`spans` arrays with `resourceSpans` / `scopeSpans`) is the right wire format for sending traces to a collector, but it is verbose and has no concept of scrollback, SQL snapshots, or structured session state. It would be a poor container for the non-trace parts of the bundle.

Custom JSON is well-understood by the existing codebase, is human-readable, and can reference the raw files inside the same ZIP by relative path.

## 4. Backlog Item Creation: Existing `CreateBacklogItem` RPC vs. New Specialized RPC

**Verdict: use existing `CreateBacklogItemRequest` (build, no new proto)**

`proto/session/v1/backlog.proto` defines `CreateBacklogItemRequest` with all fields needed for a diagnostic hand-off:

| Field | Use for bundle hand-off |
|---|---|
| `title` | Auto-generated: "Session diagnostic: &lt;session_uuid first 8 chars&gt; - &lt;session_title&gt;" |
| `description` | Rendered summary from manifest |
| `acceptance_criteria` | Pre-populated checklist from bundle contents |
| `notes` | Path or download URL to the ZIP |
| `repo_path` | Session's worktree path |
| `category` | "bugfix" or "chore" depending on trigger |

A new specialized RPC (e.g. `CreateBacklogItemFromDiagnosticBundle`) would add proto schema, gRPC stub regeneration, and service-layer wiring for minimal gain. The existing RPC is expressive enough.

## 5. Client-Side vs. Server-Side Bundle Assembly

**Verdict: server-side primary assembly with optional client-side OTel span addendum**

The data sources split cleanly:

| Data source | Location | Access |
|---|---|---|
| Session ent record (status, worktree, timestamps) | Server (SQLite via ent) | Server only |
| Analytics SQLite | Server (local file) | Server only |
| Scrollback (already gzip-compressed .gz files) | Server (`session/scrollback/`) | Server only |
| Context history | Server (local files) | Server only |
| Artifact metadata | Server (artifact store) | Server only |
| OTel trace spans (page load, fetch timing) | Browser (in-memory `BatchSpanProcessor`) | Client only |
| Web Vitals (LCP, FID, CLS) — via Performance API | Browser | Client only |

**Recommended hybrid flow:**
1. Frontend triggers export via a new `SessionBundleService.ExportSessionBundle` streaming RPC.
2. Server assembles `manifest.json` + all server-side files into a ZIP in memory.
3. Frontend serializes in-memory OTel spans (force-flush `BatchSpanProcessor`) into a `client_spans.json` file and POSTs it alongside for inclusion (or sends as a separate request field).
4. Server closes the ZIP and streams it back for download or saves to a configured path.

Pure client-side assembly is not viable: scrollback, analytics SQLite, and context history are not accessible from the browser.

Pure server-side assembly misses the client's real-time OTel spans (which may not have been flushed to the collector yet at the moment of export).

## Dependency Summary

| Decision | Recommendation | New dependency? |
|---|---|---|
| ZIP assembly | `klauspost/compress/zip` | No — already in go.mod |
| Browser RUM | Extend existing OTel setup | No |
| Bundle format | Custom JSON manifest in ZIP | No |
| Backlog hand-off | Existing `CreateBacklogItem` RPC | No |
| Assembly location | Server-primary + client OTel addendum | No |

All recommendations are "build on existing" — no new libraries, SDKs, or SaaS products need to be introduced.
