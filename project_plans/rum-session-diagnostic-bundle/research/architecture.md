# Architecture Research: RUM Session Diagnostic Bundle

> Builds on `project_plans/analytics-system/research/architecture.md` (EventBus/analytics subscriber pattern). Read that first.

---

## 1. Data Flow: Assembly Pipeline per Source

### 1.1 Session ent data
- **Source**: `session.Storage.GetSession(ctx, sessionID)` returns InstanceData (title, status, program, path, branch, tags, timestamps).
- For live sessions, use the same poller → external-discovery fallback as `DiagnosticService.findInstance()` (lines 57–68, `server/services/diagnostic_service.go`).
- Fields surfaced: `Title`, `Status`, `Program`, `Path`, `Branch`, `SessionType`, `Category`, `Tags`, `CreatedAt`, `UpdatedAt`, `LastTerminalUpdate`, `GitHubPRNumber`.

### 1.2 Analytics events (SQLite / ent)
- **Source**: `session.Storage.ListAnalyticsBySessionID(ctx, sessionUUID)` — **does not exist today**; must be added.
- `AnalyticsData` has `SessionID string`. A new ent-repository method is needed:
  ```go
  // session/ent_repository.go
  func (r *EntRepository) ListAnalyticsBySessionID(ctx context.Context, sessionID string) ([]AnalyticsData, error) {
      entries, err := r.client.AnalyticsEvent.Query().
          Where(analyticsevent.SessionIDEQ(sessionID)).
          Order(ent.Asc(analyticsevent.FieldCreatedAt)).
          Limit(2000). // cap to avoid unbounded reads
          All(ctx)
      ...
  }
  ```
- Web Vitals are tracked via `WebVitalsReporter.tsx` → `analytics.track()` but the `labels` map does NOT include `session_id` yet.

### 1.3 Context history
- **Package**: `session/contexthistory/` — separate ent client (`AnalyticsEntClient *ent.Client` in `ServerDependencies`).
- `contexthistory.Store.ListSamples()` / `ListCompactions()` provide per-turn context-window occupancy.
- `Store` is initialized in `BuildRuntimeDeps()` (dependencies.go:1333–1363) as `ctxHistoryStore *contexthistory.Store` — currently NOT in `ServerDependencies` struct; must be promoted or passed as a dedicated bundle-service dep.
- Nil-guard gracefully when not initialized.

### 1.4 Scrollback
- **Package**: `session/scrollback/`
- Key read interfaces on `*ScrollbackManager`:
  - `GetRecentLines(sessionID string, lines int) ([]byte, error)` — returns last N lines
  - `GetRecentBytes(sessionID string, bytes int64) ([]byte, error)` — returns last N bytes
  - `GetScrollback(sessionID string, fromSeq uint64, limit int) ([]ScrollbackEntry, error)` — paginated
- `ScrollbackManager` is in `ServerDependencies.ScrollbackManager`; already wired into SessionService.
- Max 10,000 lines / 10 MB per session (zstd compressed on disk). Bundle should cap at 5,000 lines.

### 1.5 Artifacts
- **File**: `session/artifacts/types.go`
- `SessionArtifactsBlob` JSON blob stored in `session_artifacts` TEXT column:
  - `PRURLs []string`, `CommitSHAs []string`, `ExternalURLs []string`, `Commands []CommandArtifact`
  - Capped at `maxExternalURLs = 50`, `maxCommands = 30` — safe for bundle inclusion.

### 1.6 Backlog linkage
- `Storage.GetItemSessionsBySessionUUID(ctx, sessionUUID)` finds which backlog items a session is attached to.
- Include: item ID, title, status, role (diagnose / work / review).

### 1.7 Bundle output
```
sessionUUID
    │
    ├─► Storage.GetSession()                 → SessionSnapshot (ent data)
    ├─► Storage.ListAnalyticsBySessionID()   → []AnalyticsEntry (tool decisions)  [NEW METHOD]
    ├─► contexthistory.Store.ListSamples()   → []Sample + []Compaction (ctx window)
    ├─► ScrollbackManager.GetRecentLines()   → []byte (terminal output, capped)
    ├─► Storage.GetSessionArtifacts()        → SessionArtifactsBlob (PRs, commits, URLs)
    └─► Storage.GetItemSessionsBySession()   → []BacklogLinkage (item IDs, roles)
                                                     │
                                                     ▼
                                            RUMSessionBundle (assembled struct)
                                                     │
                            ┌──────────────────────────────────────────────┐
                            ▼                                               ▼
                    bytes → streaming ConnectRPC              file write → path return
                    (ExportSessionBundle)                     (SaveSessionBundle)
```

---

## 2. Where Should the New Service Live?

### Options evaluated

| Option | Assessment |
|---|---|
| Extend `DiagnosticService` | Wrong domain. DiagnosticService is scoped to backlog-item AI dispatch (nudge eligibility). RUM bundles are session-scoped user exports. Would violate single-responsibility. |
| Add to `SessionService` | SessionService is already the largest component (~200 files). Adding bundle assembly creates a God object. |
| New `SessionBundleService` | Correct pattern. Matches how DiagnosticService was created as a focused, narrowly scoped service. |

### Recommendation: New `server/services/session_bundle_service.go`

```go
type SessionBundleService struct {
    storage      *session.Storage
    ctxHistory   *contexthistory.Store  // nil-safe
    scrollback   *scrollback.ScrollbackManager
    poller       *session.ReviewQueuePoller
    extDiscovery *session.ExternalSessionDiscovery
}

func NewSessionBundleService(
    storage *session.Storage,
    ctxHistory *contexthistory.Store,
    scrollback *scrollback.ScrollbackManager,
) *SessionBundleService { ... }

// SetPoller / SetExternalDiscovery — same setter-injection pattern as DiagnosticService
```

New proto file: `proto/session/v1/rum_bundle.proto` (mirrors `diagnose.proto` shape).

---

## 3. Integration Points and Dependencies

### Dependencies to inject (constructor + setters)

| Dependency | Type | Already in ServerDependencies? | Injection Pattern |
|---|---|---|---|
| `session.Storage` | `*session.Storage` | Yes (`deps.Storage`) | Constructor |
| `contexthistory.Store` | `*contexthistory.Store` | No — local var in BuildRuntimeDeps:1333 | Must add to `RuntimeDeps` or pass via setter |
| `scrollback.ScrollbackManager` | `*scrollback.ScrollbackManager` | Yes (`deps.ScrollbackManager`) | Constructor |
| `session.ReviewQueuePoller` | `*session.ReviewQueuePoller` | Yes (`deps.ReviewQueuePoller`) | Setter: `SetPoller()` |
| `session.ExternalSessionDiscovery` | `*session.ExternalSessionDiscovery` | Yes (`deps.ExternalDiscovery`) | Setter: `SetExternalDiscovery()` |

### New storage query needed
`Storage.ListAnalyticsBySessionID(ctx, sessionUUID string) ([]AnalyticsData, error)` — add to `session/repository.go` interface and `session/ent_repository.go`.

### Wiring in `server.go` (after DiagnosticService block ~line 578)
```go
bundleSvc := services.NewSessionBundleService(deps.Storage, deps.CtxHistoryStore, deps.ScrollbackManager)
bundleSvc.SetPoller(deps.ReviewQueuePoller)
bundleSvc.SetExternalDiscovery(deps.ExternalDiscovery)
bundlePath, bundleHandler := sessionv1connect.NewSessionBundleServiceHandler(bundleSvc, ConnectOptions(deps.ErrorRegistry)...)
mux.Handle(bundlePath, bundleHandler)
```

---

## 4. Event-Command-Policy Table

| Type | Name | Description |
|---|---|---|
| Command | `ExportSessionBundle` | User triggers download; backend assembles and returns bytes |
| Command | `SaveSessionBundle` | User triggers save-to-disk; backend writes file, returns path |
| Policy | `OnBundleSaved` | After SaveSessionBundle, frontend creates a backlog item with bundle path attached |
| Domain Event | `SessionBundleExported` | Published on EventBus for audit/observability (optional v2) |
| Command | `RecordWebVital` (existing) | Frontend sends metric via analytics.track() — needs `session_id` added to labels |
| Command | `AddSessionUUIDToSpan` | OTel init adds `session.uuid` as span attribute when session context is active |

---

## 5. Tech Debt Disposition: `debug_snapshot.go`

**Verdict: Extend-as-is conceptually; do NOT touch the file.**

`debug_snapshot.go` (~410 lines) is a clean, pure utility:
- No service struct, no state, no circular dependencies
- `CollectSnapshot()` is a free function with non-fatal subsystem collection (errors appended, not returned)
- Focused exclusively on *server-process* diagnostics (PID, tmux, approvals, log tail)

The RUM bundle is orthogonal: it is *session-scoped*, user-initiated, and includes sources `debug_snapshot.go` never touches (scrollback, analytics events, context history, artifacts). The non-fatal collection pattern (errors list, never abort) should be replicated in the new bundle assembler, not unified with the existing code.

**No refactor-first needed.** The seam is already clean by virtue of `debug_snapshot.go` having no service-level coupling.

---

## 6. Proto Design for Bundle RPCs

### Problem: ConnectRPC default 4 MB message size limit
- Scrollback: up to 10 MB uncompressed; at cap of 5,000 lines = ~500 KB typical.
- Analytics events: ~1,000 events × ~500 bytes/event JSON = ~500 KB.
- Context history samples: ~100 turns × ~100 bytes = ~10 KB.
- **Worst-case total: ~12–15 MB** — exceeds default 4 MB gRPC limit without configuration.

### Recommendation: Server-streaming for download, unary for save

```protobuf
// proto/session/v1/rum_bundle.proto
service SessionBundleService {
  // ExportSessionBundle streams the assembled bundle as chunks for browser download.
  rpc ExportSessionBundle(ExportSessionBundleRequest) returns (stream BundleChunk) {}

  // SaveSessionBundle assembles the bundle, writes it to disk, returns path.
  rpc SaveSessionBundle(SaveSessionBundleRequest) returns (SaveSessionBundleResponse) {}
}

message ExportSessionBundleRequest {
  string session_uuid = 1;
  int32 max_scrollback_lines = 2;  // optional, default 5000
}

message BundleChunk {
  bytes data = 1;
  int32 chunk_index = 2;
  bool final = 3;
}

message SaveSessionBundleRequest {
  string session_uuid = 1;
  string output_dir = 2;   // optional; defaults to configured data dir
}

message SaveSessionBundleResponse {
  string file_path = 1;
  int64 file_size_bytes = 2;
  string content_hash = 3;  // SHA-256 for cross-machine identity
}
```

Server-streaming matches the existing `WatchSessions` / `StreamTerminal` pattern and avoids hitting message size limits. The Go server sends 64 KB chunks; the TypeScript client reassembles and calls `URL.createObjectURL(new Blob(chunks))` for the download.

For `SaveSessionBundle`, a single unary response is fine (just returns a path string, no bytes on the wire).

---

## 7. Browser OTel and Web Vitals Gaps

### otel-init.ts gaps
Current `otel-init.ts` creates a `WebTracerProvider` with `service.name = "stapler-squad-web"` and registers `DocumentLoadInstrumentation` + `FetchInstrumentation`. **It does not set `session.uuid` as a span attribute.**

To add it:
```typescript
// otel-init.ts — export a setter so pages can inject session UUID
export function setActiveSessionUUID(uuid: string) {
  // add to custom span attributes on the active tracer
  tracer.startActiveSpan("session-scoped-op", { attributes: { "session.uuid": uuid } }, ...)
}
```

The simpler approach: add `session.uuid` as a label in `WebVitalsReporter.tsx` when the session context is available — since the vitals are already going through `analytics.track()` which ultimately writes to the SQLite `ClassificationAnalytics` table with a `SessionID` field.

### WebVitalsReporter gap
`analytics.track()` call in `WebVitalsReporter.tsx` does not include `session_id`. The `AnalyticsProvider.track()` interface must be extended to accept optional `sessionId` or the reporter must be rendered within a session page context that injects it.

---

## 8. Summary of Open Work Items for Plan Phase

1. Add `Storage.ListAnalyticsBySessionID()` to `session/repository.go` + `session/ent_repository.go`
2. Promote `ctxHistoryStore` to `RuntimeDeps` struct in `server/dependencies.go`
3. Create `proto/session/v1/rum_bundle.proto` with streaming `ExportSessionBundle` + unary `SaveSessionBundle`
4. Implement `server/services/session_bundle_service.go` (new file, ~200 lines)
5. Register `SessionBundleService` in `server/server.go` (4-line block after DiagnosticService)
6. Add `session.uuid` span attribute to `otel-init.ts` (or inject via WebVitalsReporter labels)
7. Extend `WebVitalsReporter.tsx` analytics.track() calls with `sessionId` label
8. Frontend: `ExportBundleButton` component → ConnectRPC client → blob download or save+backlog flow
