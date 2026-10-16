# Implementation Plan: RUM Session Diagnostic Bundle

**Feature**: Exportable per-session diagnostic bundle with backend capture and backlog hand-off
**Date**: 2026-10-16
**Status**: Ready for implementation
**ADRs**: ADR-001 (streaming ConnectRPC vs. HTTP handler), ADR-002 (klauspost/compress/zip vs. stdlib)

---

## Creative Pass: High-Level Approaches

Three approaches were evaluated before committing to the architecture below. Rejected options are recorded here and referenced in the Pattern Decisions table.

**Approach A — Streaming ConnectRPC for download + unary for save (Chosen)**
Strength: auth middleware applies automatically via `ConnectOptions`; consistent with `WatchSessions`/`StreamTerminal` server-streaming pattern.
Weakness: more proto machinery; client must reassemble chunks from the stream.

**Approach B — Plain HTTP handler for download + ConnectRPC for save**
Strength: simpler browser path — `fetch(url).then(r => r.blob())` with native `Content-Disposition` filename.
Weakness: auth middleware must be attached manually; inconsistent with the rest of the API surface; no precedent in this codebase for authenticated file downloads via plain HTTP.

**Approach C — Unary ConnectRPC returning `bytes zip_content`**
Strength: simplest client; no chunk reassembly.
Weakness: 4 MB ConnectRPC default message limit is exceeded by large sessions; risks server OOM when serializing the full ZIP into one response frame.

---

## Domain Glossary

| Term | Definition | Notes |
|------|-----------|-------|
| `RUMSessionBundle` | The assembled diagnostic artifact for a single session — a ZIP file containing a `manifest.json` and one file per data source | Acronym: Real User Monitoring |
| `BundleManifest` | The `manifest.json` document at the root of the ZIP; describes format version, session UUID, generated timestamp, server version, and the list of included files with their roles | Versioned at `bundle_format_version: 1` |
| `BundleSection` | A named, independently droppable component of the bundle (e.g., `scrollback`, `analytics`, `context_history`, `artifacts`) | Required vs. omittable per the compaction priority |
| `ScrollbackTail` | The most-recent N terminal output lines from a session's persisted scrollback file, capped at 500 lines in the bundle | Raw PTY bytes; may contain PII |
| `ContextHistorySample` | A per-turn snapshot of context-window occupancy (`tokens_used` / `tokens_max`) from `session/contexthistory/` | 90-day retention |
| `SessionArtifact` | A structured record of session outputs: PR URLs, commit SHAs, external URLs, and command records | Sourced from `session/artifacts/types.go` |
| `BacklogLinkage` | An association between a session and a backlog item, including the item ID, title, status, and role (`diagnose`/`work`/`review`) | Sourced via `Storage.GetItemSessionBySessionUUID` (singular — returns the most-recent item session link only) |
| `BundleDeliveryMode` | The user-chosen output path: `download` (browser Blob + `<a download>`) or `save-on-server` (disk write returning a file path) | Rendered as radio options in `ExportBundleModal` |
| `BundleChunk` | A fixed-size (64 KB) segment of the assembled ZIP bytes streamed to the client via the `ExportSessionBundle` server-streaming RPC | Proto: `BundleChunk.data bytes` |
| `DiagnosticsRetention` | The cleanup policy that prunes saved bundles older than 30 days from `~/.stapler-squad/diagnostics/` at server startup | Analogous to `contexthistory.Store.Prune` |
| `WebVital` | A browser performance metric (LCP, FID, CLS) reported by `WebVitalsReporter` and attached as analytics events with the active session's ID | Requires `sessionId` label added to `analytics.track()` calls |
| `SessionBundleService` | The Go service struct in `server/services/session_bundle_service.go` responsible for assembling and delivering `RUMSessionBundle` artifacts | New, narrow-scoped service; not an extension of `DiagnosticService` |
| `ContentHash` | A SHA-256 digest of the bundle ZIP, returned in `SaveSessionBundleResponse`; used as a machine-independent identifier on the backlog item instead of an absolute file path | Avoids cross-machine path mismatch pitfall |

---

## Pattern Decisions

| Component | Pattern Chosen | Source | Alternative Rejected | Reason |
|-----------|---------------|--------|---------------------|--------|
| `SessionBundleService` struct | Service Layer (PoEAA) | `server/services/diagnostic_service.go` as template | Extend `DiagnosticService` | Wrong domain: DiagnosticService is scoped to AI-nudge eligibility, not user-visible exports |
| `SessionBundleService` struct | Service Layer (PoEAA) | same | Add to `SessionService` | SessionService is already the largest component; adding bundle assembly would create a God object |
| Bundle assembly pipeline | Builder pattern (GoF) | `session/diagnose_bundle.go:BuildDiagnosticBundle` | Template Method on `BuildDiagnosticBundle` | `BuildDiagnosticBundle` is LLM-prompt-focused; the new bundle is user-export-focused with different section priorities and ZIP output |
| Delivery strategies | Strategy pattern (GoF) — two strategies share assembly, differ in delivery | `ExportRules`/`SaveSessionBundle` RPCs | One RPC with a `mode` flag enum | Separate RPCs (`ExportSessionBundle` streaming, `SaveSessionBundle` unary) are cleaner contracts with different response types |
| Bundle download transport | Server-streaming ConnectRPC (Approach A above) | `WatchSessions`, `StreamTerminal` in `proto/session/v1/session.proto` | Plain HTTP handler (Approach B) | Auth middleware applies automatically via `ConnectOptions`; consistent with existing streaming RPCs |
| Bundle download transport | Server-streaming ConnectRPC (Approach A above) | same | Unary RPC returning `bytes` (Approach C) | Unary hits 4 MB message limit; large sessions produce 12–15 MB bundles |
| `ScrollbackTail` newtype | Newtype (`type ScrollbackTail []byte`) | Type-driven design principle | Raw `[]byte` parameter | Prevents passing unscoped byte slices where the semantic is specifically "capped terminal tail" |
| ZIP library | `klauspost/compress/zip` aliased as `kzip` | `server/middleware/gzip.go` (`kgzip` alias convention) | `stdlib archive/zip` | Already in go.mod; SIMD-accelerated deflate; follows the existing `kgzip` alias convention (ADR-002) |
| Per-section compaction | Priority-ordered whole-section drop + hard byte ceiling | `session/diagnose_bundle.go` | Mid-section truncation | Partial sections confuse consumers; the whole-section drop pattern already exists and is tested |
| Backlog hand-off field | Embed content hash + filename in `notes` text field | `proto/session/v1/backlog.proto`:`CreateBacklogItemRequest.notes` | New `diagnostic_bundle_path` proto field | No new proto field needed; avoids machine-local path portability pitfall; `ContentHash` provides identity. **Note (B4)**: requirements AC5 says "link to the saved file path" — the implementation intentionally substitutes SHA-256 + filename for the absolute path; the absolute path is machine-local and meaningless on any other host. |
| RPC naming | `ExportSessionBundle` in proto / plan | `proto/session/v1/rum_bundle.proto` | `ExportSessionDiagnosticBundle` | Both names refer to the same RPC. requirements.md AC8 and design/ux.md use the longer `ExportSessionDiagnosticBundle`; the plan standardizes on the shorter `ExportSessionBundle` (service-qualified: `SessionBundleService.ExportSessionBundle`). These are synonyms — no generated-code breakage. |

---

## Tech Debt Disposition

| Area | Existing Issue | Disposition | Justification |
|------|----------------|------------|----------------|
| `server/services/debug_snapshot.go` | Server-wide snapshot, no per-session scope, no analytics/scrollback/context-history, no browser download path | Extend-as-is conceptually; do NOT touch the file | File is clean and narrowly scoped to server-process diagnostics. The RUM bundle is orthogonal: session-scoped, user-initiated, with entirely different data sources. The non-fatal error-collection pattern (`Errors []string`) should be replicated in the new assembler, not unified. No refactor-first required. |
| `session/scrollback/manager.go` | No sanitization of raw PTY bytes before storage | Mitigate at read time in bundle assembler; do NOT modify scrollback internals | Adding sanitization to the write path risks corrupting existing stored data and breaking display. The bundle assembler can apply best-effort redaction + consent prompt at export time without touching the storage layer. |
| `server/server.go:ConnectOptions` | No `WithReadMaxBytes` on any handler — repo-wide acknowledged gap | Mitigate in bundle service by using server-streaming (no large single frame); do not add a global limit as a side effect of this feature | Adding a global read limit is a separate change that could break existing callers. The streaming approach eliminates the large-frame risk for bundle downloads specifically. |

---

## Migration Plan

No database schema changes. The `session/ent/schema/` files are not modified. New fields go on:
- `session/repository.go` (interface extension — backward compatible)
- `server/dependencies.go` (struct field addition — backward compatible)

No data migration required.

---

## Observability Plan

- **Logs**: `SessionBundleService` uses structured `slog` at `INFO` for assembly start/complete (with session UUID, file count, uncompressed bytes, elapsed ms) and `WARN` for omitted sections (with section name and reason). `DEBUG` for per-section byte counts.
- **Metrics**: No new metrics in v1. The existing OTel RPC interceptor records `rpc.duration` for `ExportSessionBundle` and `SaveSessionBundle` automatically.
- **Alerts**: None required for v1 (diagnostic-only feature; not in a critical path). If `SaveSessionBundle` fails, the user is notified in-app and can retry.

---

## Risk Control

- **Feature flag**: None required. The feature is additive (new RPC + new UI control in an action sheet). It cannot be triggered accidentally. If a rollback is needed, revert the server binary and the web-app build; the feature disappears.
- **Rollback procedure**: Revert commits to `server/services/session_bundle_service.go`, `proto/session/v1/rum_bundle.proto`, and the web-app bundle. No data to clean up (saved bundles in `~/.stapler-squad/diagnostics/` are harmless but can be manually deleted).
- **PII risk**: The export modal must display a consent notice before initiating any download or server-save: "This bundle may contain raw terminal output including typed credentials. Do not share it with untrusted parties." Scrollback is capped at 500 lines (tail only — limits historical exposure). A best-effort secrets scan (entropy heuristic + AWS key prefix pattern) logs a `WARN` to `manifest.json` if triggered patterns are found, but does not block the export; the user is shown a banner.

---

## Unresolved Questions

### Known Open PM Items (non-blocking)

These were surfaced in the Phase 4 triad review (PM leg, round 3). They do not block implementation but should be addressed before public announcement or external user testing:

- **Success metric denominator undefined**: "50% of bug-report items include a bundle within 4 weeks" — the denominator "bug-report items" has no tag, label, or creation-path convention defined. Before launch, agree on the tagging convention (e.g. backlog items created with the "bug" tag) so the metric is countable.
- **Target user under-specified**: "internal users" is the implicit persona. A one-sentence description of their debugging workflow (e.g. "operators and developers who triage agent session failures from the stapler-squad UI") would sharpen the motivation for the two delivery modes.
- **No named behavioral/adoption assumption**: engineering risks are well-documented (P1s in pre-mortem); there is no equivalent behavioral risk assumption (e.g. "users will click Export rather than filing a bug from memory"). Naming this assumption makes the success metric meaningful.

---

- [x] **RESOLVED** — PII consent prompt copy: use the exact text already in the Risk Control section: *"This bundle may contain raw terminal output including typed credentials. Do not share it with untrusted parties."* This is definitive; Story 2.1.2 must render this exact string. No further product/design sign-off required (the adversarial review confirmed: "the text is already written; treat as a design-review checkpoint, not a hard block").
- [x] **RESOLVED** — iOS "Download" path: `useExportBundle.download()` must implement `navigator.canShare({ files: [new File([blob], filename)] })` feature-detect. If supported (iOS Safari, modern Android), call `navigator.share({ files: [file] })`; otherwise fall back to `<a download>` trigger. Task 2.1.1a is the implementation point. This is an engineering-only decision.

---

## Dependency Visualization

```
Phase 1 (Backend Foundation)
  Epic 1.1: Data Layer
    Story 1.1.1: ListAnalyticsBySessionID ──┐
    Story 1.1.2: Promote ctxHistoryStore    │
    Story 1.1.3: GetSessionArtifactsByUUID  │
  Epic 1.2: Proto Contract                  │
    Story 1.2.1: rum_bundle.proto ──────────┤
  Epic 1.3: Bundle Service                  │
    Story 1.3.1: assembler core ────────────┘──► depends on 1.1 (incl. 1.1.3) + 1.2
    Story 1.3.2: ExportSessionBundle ──────────► depends on 1.3.1
    Story 1.3.3: SaveSessionBundle + retention ► depends on 1.3.1

Phase 2 (Frontend) ─────────────────────────────► depends on Phase 1
  Epic 2.1: Export UI
    Story 2.1.1: useExportBundle hook ─────────► depends on 1.2.1b (generated TS)
    Story 2.1.2: ExportBundleModal ────────────► depends on 2.1.1
    Story 2.1.3: Wire into SessionDetailView ──► depends on 2.1.2
  Epic 2.2: Backlog Hand-off
    Story 2.2.1: "Create backlog item" action ─► depends on 2.1.3

Phase 3 (Instrumentation) ─────────────────────► independent of Phase 1+2
  Epic 3.1: Browser OTel + Web Vitals
    Story 3.1.1: session.uuid on OTel spans
    Story 3.1.2: sessionId on WebVitals events

Phase 4 (Verification) ─────────────────────────► depends on Phases 1–3
  Epic 4.1: Tests
    Story 4.1.1: unit tests (bundle assembly)
    Story 4.1.2: E2E test (download flow)
```

---

## Phase 1: Backend Foundation

### Epic 1.1: Data Layer Extensions

**Goal**: Make analytics events and context history accessible per-session for bundle assembly, and promote `ctxHistoryStore` to `RuntimeDeps` so the new service can receive it via dependency injection.

#### Story 1.1.1: Add ListAnalyticsBySessionID to Storage

**As a** bundle assembler, **I want** to query analytics events by session UUID, **so that** analytics data is included in the bundle without a full-table scan.

**Acceptance Criteria**:
- `session.Storage` interface has a `ListAnalyticsBySessionID(ctx context.Context, sessionID string) ([]AnalyticsData, error)` method.
  - *Given* a session with UUID `"abc123"` has 50 analytics events and a second session has 20, *When* `ListAnalyticsBySessionID(ctx, "abc123")` is called, *Then* exactly 50 events are returned ordered by `created_at ASC`, and the second session's events are absent.
- The ent implementation applies a `LIMIT 2000` cap so long-lived sessions do not cause unbounded reads.
  - *Given* a session has 3,000 analytics events, *When* `ListAnalyticsBySessionID` is called, *Then* at most 2,000 events are returned (the most-recent 2,000 by `created_at`).

**Files**:
- `session/repository.go`
- `session/ent_repository.go`

##### Task 1.1.1a: Add interface method to Storage (XS)
- In `session/repository.go`, add `ListAnalyticsBySessionID(ctx context.Context, sessionID string) ([]AnalyticsData, error)` to the `Storage` interface.
- Update any mock/fake `Storage` implementations in the test packages (search: `grep -rn "Storage" session/ --include="*_test.go" -l` or `grep -rn "MockStorage\|FakeStorage\|storageStub" session/ -l`); add a stub returning `nil, nil` so the package still compiles.
- Files: `session/repository.go`

##### Task 1.1.1b: Implement in EntRepository (S)
- In `session/ent_repository.go`, implement `(r *EntRepository) ListAnalyticsBySessionID`:
  - Query: `r.client.AnalyticsEvent.Query().Where(analyticsevent.SessionIDEQ(sessionID)).Order(ent.Desc(analyticsevent.ByCreatedAt())).Limit(2000).All(ctx)`
  - Map ent results to `[]AnalyticsData` using the same mapping helper as the existing analytics query in `analytics_store.go`.
- Files: `session/ent_repository.go`

---

#### Story 1.1.2: Promote ctxHistoryStore to RuntimeDeps

**As a** `SessionBundleService`, **I want** to receive `*contexthistory.Store` via constructor injection, **so that** context history samples are included in bundles without reaching into `BuildRuntimeDeps` internals.

**Acceptance Criteria**:
- `server.RuntimeDeps` struct has a `CtxHistoryStore *contexthistory.Store` field.
  - *Given* the server starts with `STAPLER_SQUAD_ANALYTICS_ENABLED=true`, *When* `BuildRuntimeDeps` completes, *Then* `deps.CtxHistoryStore` is non-nil and `NewSessionBundleService` can be called with it.
- When analytics is disabled or `contexthistory.Store` initialization fails, `CtxHistoryStore` is `nil`; `SessionBundleService` must nil-guard all calls to it and omit the `context_history` section from the bundle without returning an error.
  - *Given* `CtxHistoryStore` is `nil`, *When* `assembleBundle` runs, *Then* the bundle manifest lists `context_history` in `omitted_sections` with reason `"context_history_unavailable"` and assembly completes successfully.

**Files**:
- `server/dependencies.go`

##### Task 1.1.2a: Add CtxHistoryStore to RuntimeDeps (S)
- In `server/dependencies.go`, add `CtxHistoryStore *contexthistory.Store` to the `RuntimeDeps` struct.
- In `BuildRuntimeDeps`, assign `deps.CtxHistoryStore = ctxHistoryStore` immediately after the existing `ctxHistoryStore :=` local variable assignment (around line 1333–1363).
- Files: `server/dependencies.go`

---

#### Story 1.1.3: Add GetSessionArtifactsByUUID to Storage

**As a** bundle assembler, **I want** to look up session artifacts by session UUID, **so that** the bundle can include PR URLs, commit SHAs, and external URLs for the correct session without a title-based lookup.

**Acceptance Criteria**:
- `session.Storage` interface has a `GetSessionArtifactsByUUID(ctx context.Context, sessionUUID string) (*artifacts.SessionArtifactsBlob, error)` method.
  - *Given* session UUID `"abc123"` has two PR URLs, *When* `GetSessionArtifactsByUUID(ctx, "abc123")` is called, *Then* the returned blob contains those two PR URLs and no records from other sessions.

**Files**:
- `session/repository.go`
- `session/ent_repository.go`

##### Task 1.1.3a: Add interface method and ent implementation (S)
- In `session/repository.go`, add `GetSessionArtifactsByUUID(ctx context.Context, sessionUUID string) (*artifacts.SessionArtifactsBlob, error)` to the `Storage` interface.
- In `session/ent_repository.go`, implement it: query `r.client.Session.Query().Where(session.UUIDEQ(sessionUUID)).Select(session.FieldSessionArtifacts).Only(ctx)`, then unmarshal the JSON blob from the `session_artifacts` TEXT column.
- Update the same mock/fake `Storage` implementations touched in Task 1.1.1a; add a stub returning `nil, nil`.
- Files: `session/repository.go`, `session/ent_repository.go`, and any mock/fake files found above

---

### Epic 1.2: Proto Contract

**Goal**: Define the RPC surface for bundle export and server-save as a new standalone proto file, generate Go and TypeScript stubs, and register the service in `server.go`.

#### Story 1.2.1: Define rum_bundle.proto and generate stubs

**As a** frontend hook, **I want** typed Go and TypeScript clients for `ExportSessionBundle` and `SaveSessionBundle`, **so that** the frontend can call the RPCs with compile-time type safety.

**Acceptance Criteria**:
- `proto/session/v1/rum_bundle.proto` exists and compiles without errors via `make proto-gen`.
  - *Given* the proto file is correctly authored, *When* `make proto-gen` runs, *Then* `gen/session/v1/rum_bundle_pb.go` and `web-app/src/gen/session/v1/rum_bundle_pb.ts` are regenerated without error.
- `server/server.go` registers `SessionBundleService` as a ConnectRPC handler on the shared mux, protected by the same `ConnectOptions` interceptors (OTel, error recorder) as all other endpoints.
  - *Given* the server starts, *When* a `POST /session.v1.SessionBundleService/SaveSessionBundle` request arrives without a valid auth token, *Then* the response is `401 Unauthorized` — same as other protected endpoints.
- `make registry-generate` runs after proto-gen and updates `docs/registry/features/rum-bundle.json` to record the new RPCs.
  - *Given* `rum_bundle.proto` is committed, *When* `make registry-generate` runs, *Then* `docs/registry/features/rum-bundle.json` includes entries for `ExportSessionBundle` and `SaveSessionBundle`.

**Files**:
- `proto/session/v1/rum_bundle.proto` (new)
- `server/server.go`
- `docs/registry/features/rum-bundle.json` (new, generated by `make registry-generate`)

##### Task 1.2.1a: Author rum_bundle.proto (S)
- Create `proto/session/v1/rum_bundle.proto` with the following messages and service:
  ```protobuf
  service SessionBundleService {
    rpc ExportSessionBundle(ExportSessionBundleRequest)
        returns (stream BundleChunk) {}
    rpc SaveSessionBundle(SaveSessionBundleRequest)
        returns (SaveSessionBundleResponse) {}
  }

  message ExportSessionBundleRequest {
    string session_uuid = 1;
    int32 max_scrollback_lines = 2; // optional, default 500
  }

  message BundleChunk {
    bytes data = 1;
    int32 chunk_index = 2;
    bool final = 3;
  }

  message SaveSessionBundleRequest {
    string session_uuid = 1;
    string output_dir = 2; // optional; defaults to configured diagnostics dir
  }

  message SaveSessionBundleResponse {
    string file_path = 1;
    int64 file_size_bytes = 2;
    string content_hash = 3; // SHA-256 hex, for cross-machine identity
  }
  ```
- Add `// +api: bundle:export` and `// +api: bundle:save` Go marker comments to the service registration block in `server.go` for the feature registry.
- Files: `proto/session/v1/rum_bundle.proto`

##### Task 1.2.1b: Run proto-gen, register in server.go, run registry-generate (S)
- Run `make proto-gen` to generate `gen/session/v1/rum_bundle_pb.go` and `web-app/src/gen/session/v1/rum_bundle_pb.ts` (the generated TS file is also gitignored; do not commit it, same policy as other gen output).
- In `server/server.go`, after the `DiagnosticService` registration block (~line 578), add:
  ```go
  bundleSvc := services.NewSessionBundleService(deps.Storage, deps.CtxHistoryStore, deps.ScrollbackManager) // Storage and CtxHistoryStore satisfy BundleStorage and ContextHistoryReader respectively
  bundleSvc.SetPoller(deps.ReviewQueuePoller)
  bundleSvc.SetExternalDiscovery(deps.ExternalDiscovery)
  bundlePath, bundleHandler := sessionv1connect.NewSessionBundleServiceHandler(bundleSvc, ConnectOptions(deps.ErrorRegistry)...)
  mux.Handle(bundlePath, bundleHandler)
  ```
- Run `make registry-generate` and commit the resulting `docs/registry/features/rum-bundle.json`.
- Files: `server/server.go`, `docs/registry/features/rum-bundle.json`

---

### Epic 1.3: Bundle Assembly Service

**Goal**: Implement `SessionBundleService` with priority-ordered section assembly, streaming export, unary server-save, and startup diagnostics retention.

#### Story 1.3.1: Bundle Assembly Core

**As a** bundle delivery handler, **I want** a pure assembly function that collects all data sources into a `RUMSessionBundle` ZIP in memory with per-section size budgets, **so that** both delivery modes share a single assembly path.

**Acceptance Criteria**:
- `assembleBundle(ctx, req)` returns a `[]byte` ZIP and a `BundleManifest` for a session with up to 10,000 scrollback lines and 1,000 analytics events within 5 seconds.
  - *Given* a session with 10,000 scrollback lines and 1,000 analytics events, *When* `assembleBundle` is called, *Then* it returns in under 5 seconds and the ZIP is valid (parseable by `archive/zip.NewReader`).
- The ZIP always contains `manifest.json` at the root; all other files are optional.
  - *Given* `CtxHistoryStore` is nil and scrollback returns an error, *When* `assembleBundle` runs, *Then* the ZIP contains at least `manifest.json` listing the omitted sections with reasons.
- The uncompressed content of all sections combined does not exceed 50 MB (enforced by the per-section budget guard).
  - *Given* analytics events total 45 MB, *When* `assembleBundle` runs, *Then* analytics is truncated to its section budget and `manifest.json` records the truncation.
- Bundle assembly calls `scrollback.FlushSession(sessionUUID)` before reading scrollback to drain the in-memory circular buffer to disk, eliminating the TOCTOU gap.
  - *Given* the session has 100 unflushed in-memory scrollback lines, *When* `assembleBundle` runs, *Then* `FlushSession` is called before the scrollback read; those lines appear in the bundle.

**Files**:
- `server/services/session_bundle_service.go` (new)

##### Task 1.3.1a: Create session_bundle_service.go with assembler (M)
- Create `server/services/session_bundle_service.go` with:
  - `SessionBundleService` struct fields: `storage BundleStorage` (narrow interface, defined below), `ctxHistory ContextHistoryReader` (nil-safe interface, defined below), `scrollback ScrollbackReader` (interface defined in same file), `poller`, `extDiscovery`, `exportSem chan struct{}` (buffered channel of capacity 3, initialized in constructor). *(pre-mortem P1 fix: concurrent OOM — assembleBundle materializes full ZIP in memory; cap concurrent exports at 3)*
  - Define a `BundleStorage` interface in `session_bundle_service.go`: `type BundleStorage interface { GetSession(ctx context.Context, uuid string) (*session.Session, error); ListAnalyticsBySessionID(ctx context.Context, uuid string, limit int) ([]AnalyticsData, error); GetItemSessionBySessionUUID(ctx context.Context, uuid string) (*ItemSession, error); GetSessionArtifactsByUUID(ctx context.Context, uuid string) (*artifacts.SessionArtifactsBlob, error) }` — `*session.EntRepository` satisfies this interface; tests inject a fake.
  - Define a `ContextHistoryReader` interface in `session_bundle_service.go`: `type ContextHistoryReader interface { SummarizeSession(ctx context.Context, uuid string) (SessionSummary, error); Series(ctx context.Context, uuid string) ([]Sample, error); Compactions(ctx context.Context, uuid string) ([]Compaction, error) }` — `*contexthistory.Store` satisfies this interface (method verified: `store.go:230`); nil-safe in production (`ctxHistory` may be nil when analytics is disabled, add nil-guard before any call); tests inject a fake.
  - Define a `ScrollbackReader` interface in `session_bundle_service.go`: `type ScrollbackReader interface { FlushSession(sessionUUID string) error; GetRecentLines(sessionID string, n int) ([]byte, error) }` — `*scrollback.ScrollbackManager` satisfies this interface; tests inject a fake.
  - Constructor: `NewSessionBundleService(storage BundleStorage, ctxHistory ContextHistoryReader, scrollback ScrollbackReader) *SessionBundleService` — initialize `exportSem: make(chan struct{}, 3)`
  - `SetPoller` / `SetExternalDiscovery` setter-injection methods (matching `DiagnosticService` pattern).
  - `BundleManifest` struct (versioned, timestamped, `SessionUUID`, `ServerVersion`, list of included/omitted sections).
  - `assembleBundle(ctx, sessionUUID string, maxScrollbackLines int) (zipBytes []byte, manifest BundleManifest, err error)` implementing the section pipeline:
    1. Flush scrollback (call `scrollbackMgr.FlushSession(sessionUUID)`)
    2. Collect session ent metadata (required section); after step 2: `if ctx.Err() != nil { return nil, ctx.Err() }`
    3. Collect server info from `runtime.GOOS`, `runtime.Version()`, `os.Getpid()` (required section); after step 3: `if ctx.Err() != nil { return nil, ctx.Err() }`
    4. Collect backlog linkage via `storage.GetItemSessionBySessionUUID(ctx, sessionUUID)` (singular — returns most-recent link only; omittable — nil-guard, add to omitted_sections on error); after step 4: `if ctx.Err() != nil { return nil, ctx.Err() }`
    5. Collect scrollback tail via `scrollbackMgr.GetRecentLines(sessionUUID, min(maxScrollbackLines, 500))` — this is the correct tail API; do NOT use `GetScrollback(sessionID, 0, N)` which returns the *oldest* N lines (ascending from seq=0), not the tail (omittable — add to omitted_sections on error); after step 5: `if ctx.Err() != nil { return nil, ctx.Err() }`
    6. Collect analytics events via `ListAnalyticsBySessionID`, capped at 2,000 (omittable); after step 6: `if ctx.Err() != nil { return nil, ctx.Err() }`
    7. Collect context history via `ctxHistory.SummarizeSession(ctx, sessionUUID)` for aggregate stats, `ctxHistory.Series(ctx, sessionUUID)` for raw per-turn token samples, and `ctxHistory.Compactions(ctx, sessionUUID)` for compaction events (all three needed — requirements AC2 explicitly requires "context history samples and compaction events"; omittable — nil-guard `ctxHistory`, add to omitted_sections when nil or on error); after step 7: `if ctx.Err() != nil { return nil, ctx.Err() }`
    8. Collect session artifacts (omittable); after step 8: `if ctx.Err() != nil { return nil, ctx.Err() }`
    9. Drop omittable sections in reverse priority order if uncompressed bytes exceed 45 MB
    10. Write ZIP using `kzip "github.com/klauspost/compress/zip"` with `manifest.json` + per-section files
    11. Append `errors` list to manifest (non-fatal collection pattern from `debug_snapshot.go`)
- Files: `server/services/session_bundle_service.go`

---

#### Story 1.3.2: ExportSessionBundle Streaming RPC

**As a** browser user, **I want** to download the bundle as a browser file, **so that** I can attach it to a bug report without granting server access.

**Acceptance Criteria**:
- `ExportSessionBundle` streams the assembled ZIP in 64 KB `BundleChunk` messages; the final chunk has `final = true`.
  - *Given* a bundle of 200 KB, *When* `ExportSessionBundle` is called, *Then* 4 `BundleChunk` messages are sent (3 full + 1 final with `final=true`); the client reassembles them as the exact ZIP bytes.
- Calling `ExportSessionBundle` with an invalid session UUID returns a `not_found` ConnectRPC error (HTTP 404).
  - *Given* session UUID `"nonexistent-uuid"`, *When* `ExportSessionBundle` is called, *Then* the stream ends immediately with `connect.CodeNotFound`.

**Files**:
- `server/services/session_bundle_service.go`

##### Task 1.3.2a: Implement ExportSessionBundle handler (M)
- Add `ExportSessionBundle(ctx, req *connect.Request[sessionv1.ExportSessionBundleRequest], stream *connect.ServerStream[sessionv1.BundleChunk]) error` method to `SessionBundleService`.
- Acquire semaphore slot at start: `select { case s.exportSem <- struct{}{}: defer func() { <-s.exportSem }() default: return connect.NewError(connect.CodeResourceExhausted, errors.New("too many concurrent exports")) }`.
- Resolve session via `findInstance` (copy the private helper verbatim from `server/services/diagnostic_service.go` — search for `func (s *DiagnosticService) findInstance` there; it takes `(ctx, sessionUUID string) (*session.Instance, error)` and checks both the in-memory map and the poller).
- Call `assembleBundle` with `req.Msg.SessionUuid` and `req.Msg.MaxScrollbackLines` (default 500 when 0).
- Loop over ZIP bytes in 64 KB chunks, sending each via `stream.Send(&sessionv1.BundleChunk{Data: chunk, ChunkIndex: int32(i), Final: i == lastIdx})`.
- Return `connect.NewError(connect.CodeNotFound, ...)` when the session is not found.
- Files: `server/services/session_bundle_service.go`

---

#### Story 1.3.3: SaveSessionBundle Unary RPC and Diagnostics Retention

**As a** user on mobile (or any user who wants a tracked artifact), **I want** to save the bundle to the server and get back a stable identifier, **so that** the bundle survives beyond the browser session.

**Acceptance Criteria**:
- `SaveSessionBundle` writes the ZIP to `~/.stapler-squad/diagnostics/<session-uuid>-<timestamp>.zip` and returns the file path, byte count, and SHA-256 content hash.
  - *Given* session UUID `"abc123"` and the diagnostics directory is writable, *When* `SaveSessionBundle` is called, *Then* a file `abc123-20261016T120000Z.zip` exists in the diagnostics directory and the response's `content_hash` is the SHA-256 hex of that file.
- On server startup, the diagnostics retention cleaner deletes saved bundles older than 30 days.
  - *Given* a bundle file with `mtime` of 31 days ago, *When* the server starts, *Then* that file is deleted; a bundle 29 days old is retained.
- `SaveSessionBundle` returns `connect.CodeResourceExhausted` (HTTP 429) if available disk space in the diagnostics directory falls below 100 MB.
  - *Given* the disk has 50 MB free, *When* `SaveSessionBundle` is called, *Then* the RPC returns `connect.CodeResourceExhausted` with message `"diagnostics directory has less than 100MB free"`.

**Files**:
- `server/services/session_bundle_service.go`
- `server/dependencies.go` (register startup retention cleaner)

##### Task 1.3.3a: Implement SaveSessionBundle handler (S)
- Add `SaveSessionBundle(ctx, req *connect.Request[sessionv1.SaveSessionBundleRequest]) (*connect.Response[sessionv1.SaveSessionBundleResponse], error)` to `SessionBundleService`.
- Resolve the diagnostics output directory from `config.GetConfigDir()` + `"diagnostics/"` (default). When `req.Msg.OutputDir` is non-empty: call `filepath.Clean(filepath.Abs(req.Msg.OutputDir))` and assert the result has `config.GetConfigDir()` as a prefix; return `connect.CodeInvalidArgument` if the check fails. *(P1 pre-mortem fix: path traversal prevention)*
- Call `assembleBundle`, then:
  - Check available disk space via package-level `diskFreeBytes(path string) (uint64, error)`. Implement this in two build-tagged files in `server/services/`: `disk_space_unix.go` (`//go:build !windows`, uses `syscall.Statfs`) and `disk_space_windows.go` (`//go:build windows`, returns `^uint64(0), nil` as always-sufficient stub). Pattern: `server/services/exec_unix.go` / `server/services/exec_windows.go` if present, else any existing `*_unix.go` / `*_windows.go` pair in the package.; return `CodeResourceExhausted` if under 100 MB. *(pre-mortem P1 fix: Windows/WSL2 build compatibility)*
  - Write ZIP to `<outputDir>/<slugifiedTitle>-<YYYYMMDD-HHMMSS>.zip` atomically (write to a `.tmp` file, then rename).
  - Compute SHA-256 of written bytes.
  - Return `SaveSessionBundleResponse{FilePath, FileSizeBytes, ContentHash}`.
- Files: `server/services/session_bundle_service.go`

##### Task 1.3.3b: Implement DiagnosticsRetention cleanup at startup (S)
- Add `RunDiagnosticsRetention(dir string, maxAge time.Duration) error` free function in `server/services/session_bundle_service.go`.
- Deletes files in `dir` matching `*.zip` whose `mtime` is older than `maxAge` (default 30 days); logs a `slog.INFO` per deleted file.
- In `server/dependencies.go`, wire retention in two places: (a) wrap the startup call asynchronously: `go func() { if err := services.RunDiagnosticsRetention(ctx, diagnosticsDir, 30*24*time.Hour); err != nil { slog.Warn("diagnostics retention failed", "error", err) } }()` — non-blocking, so a slow or absent diagnostics directory never delays `BuildRuntimeDeps`; (b) launch a goroutine with a `time.NewTicker(24 * time.Hour)` that calls `RunDiagnosticsRetention` on each tick — matching `analytics/retention.go`'s `EnforceRetention` pattern. Log but do not hard-fail if the directory does not yet exist. *(pre-mortem P1 fix: startup-only retention leaves long-lived daemons without cleanup)*
- Files: `server/services/session_bundle_service.go`, `server/dependencies.go`

---

## Phase 2: Frontend

### Epic 2.1: Export UI

**Goal**: Deliver the `useExportBundle` hook and `ExportBundleModal` component, wire them into the existing session action surfaces, and implement the delivery-mode selection UX described in the UX research.

#### Story 2.1.1: Create useExportBundle Hook

**As a** React component, **I want** a hook that calls `ExportSessionBundle` or `SaveSessionBundle` and returns loading/error state, **so that** the delivery-mode UI is decoupled from the RPC calls.

**Acceptance Criteria**:
- `useExportBundle` returns `{ download, saveOnServer, loading, error }` where `download(sessionUuid)` triggers the streaming RPC → Blob → `<a download>` path and `saveOnServer(sessionUuid)` triggers the unary RPC → returns `SaveSessionBundleResponse`.
  - *Given* a valid session UUID, *When* `download(uuid)` is called, *Then* a `<a download="<filename>.zip">` element is clicked programmatically, initiating a browser file save.
- While the RPC is in-flight, `loading` is `true`; it becomes `false` on completion or error.
  - *Given* the server takes 3 seconds, *When* `download(uuid)` is called, *Then* `loading` is `true` for 3 seconds, then `false`.
- Network or RPC errors surface in `error` as a human-readable message string (not thrown).
  - *Given* the server returns `CodeNotFound`, *When* `download(uuid)` is called, *Then* `error` is set to `"Session not found"` and `loading` is `false`.

**Files**:
- `web-app/src/lib/hooks/useExportBundle.ts` (new)

##### Task 2.1.1a: Create useExportBundle.ts (S)
- Create `web-app/src/lib/hooks/useExportBundle.ts` following the `useExportRules.ts` pattern:
  - Import `createClient` from `@connectrpc/connect` and the generated `SessionBundleService` from `@/gen/session/v1/rum_bundle_pb`.
  - `download(sessionUuid: string, sessionTitle: string)`: call the server-streaming `exportSessionBundle` RPC, collect all `BundleChunk.data` arrays, create `new Blob([...chunks], { type: "application/zip" })`, trigger `<a download="<session-title>-YYYYMMDD-HHMMSS.zip">` click (slugify the session title: lowercase, spaces→hyphens, non-alphanumeric stripped; example: `"Fix login flow-20261016-143022.zip"`), revoke the object URL. **Note (B2 fix)**: requirements and UX align on a title-based human-readable filename, not a UUID-based one.
  - `saveOnServer(sessionUuid: string)`: call the unary `saveSessionBundle` RPC, return the `SaveSessionBundleResponse`.
  - Export `{ download, saveOnServer, loading, error }` state.
- Files: `web-app/src/lib/hooks/useExportBundle.ts`

---

#### Story 2.1.2: Create ExportBundleModal Component

**As a** user triggering bundle export, **I want** a modal that shows delivery-mode options, a consent notice, and progress feedback, **so that** I understand what is being captured and can choose how to receive it.

**Acceptance Criteria**:
- The modal renders two radio options: "Download .zip" (default on desktop) and "Save on server" (default on mobile, detected via `window.innerWidth < 640`).
  - *Given* the modal is opened on a device with `window.innerWidth = 375`, *When* the modal renders, *Then* "Save on server" radio is pre-selected.
- A consent notice reading exactly: **"This bundle may contain raw terminal output including typed credentials. Do not share it with untrusted parties."** is visible before the Export button can be clicked. This copy is final — do not substitute with a draft or placeholder.
  - *Given* the modal just opened, *When* no radio is selected yet, *Then* the "Export" button is disabled.
  - *Given* a delivery mode is selected, *When* the user has not dismissed the consent notice, *Then* the "Export" button is disabled.
- While export is in flight, the button label changes to "Assembling bundle…" with an inline spinner and the button is disabled.
  - *Given* export takes 3 seconds, *When* the user clicks "Export", *Then* the button reads "Assembling bundle…" for those 3 seconds.
- On successful download, `showActionToast("Bundle downloaded.", "success", "bundle-export-<sessionId>")` is called.
  - *Given* the streaming RPC completes successfully, *When* the Blob triggers a download, *Then* the action toast appears.
- On successful server-save, the toast shows "Bundle saved to server." and a "Create backlog item →" button appears inline in the modal.
  - *Given* `saveOnServer` resolves with a file path, *When* the modal re-renders, *Then* "Create backlog item →" button is visible.
- Errors display both an action toast and an inline `role="alert"` banner with a Retry button.
  - *Given* the RPC returns an error, *When* the modal re-renders, *Then* both `showActionToast(..., "error", ...)` fires and the `role="alert"` banner appears with a Retry button.
- All interactive elements meet WCAG AA: trigger button has `aria-haspopup="dialog"`, in-progress status has `role="status"` + `aria-live="polite"`, error banner has `role="alert"`.
  - *Given* export fails, *When* the error banner renders, *Then* `role="alert"` is present on the banner element.

**Files**:
- `web-app/src/components/sessions/ExportBundleModal.tsx` (new)
- `web-app/src/components/sessions/ExportBundleModal.css.ts` (new, vanilla-extract)

##### Task 2.1.2a: Create ExportBundleModal.tsx and styles (M)
- Create `web-app/src/components/sessions/ExportBundleModal.tsx`:
  - Use the existing `Modal`/`ModalContent` (Radix `Dialog.Root`) pattern, mirroring the `actionSheetModal` in `SessionDetailView.tsx`.
  - Delivery-mode radio group (`"download"` | `"save"`), defaulting to `"save"` when `window.innerWidth < 640`.
  - Consent notice with a required checkbox (`aria-required="true"`); "Export" button disabled until checked and delivery mode selected.
  - Calls `useExportBundle().download` or `.saveOnServer` per mode; shows spinner + "Assembling bundle…" label during `loading`.
  - Post-save inline "Create backlog item →" button (Story 2.2.1 wires this up).
  - Error handling: `showActionToast` via `useNotifications()` + inline `role="alert"` banner.
- Create `web-app/src/components/sessions/ExportBundleModal.css.ts` with vanilla-extract styles for the modal content, consent notice, delivery-mode radio group, and error banner.
- Files: `web-app/src/components/sessions/ExportBundleModal.tsx`, `web-app/src/components/sessions/ExportBundleModal.css.ts`

---

#### Story 2.1.3: Wire Export Control into Session UI

**As a** user viewing a session, **I want** to trigger bundle export from the action sheet (detail view) or the kebab menu (list view), **so that** the feature is discoverable at both entry points.

**Acceptance Criteria**:
- `SessionDetailView` action sheet contains an "Export Diagnostics" item inserted before the destructive `<hr>` divider, with `data-testid="export-diagnostics-action"`.
  - *Given* the session detail action sheet is opened, *When* the sheet renders, *Then* "Export Diagnostics" appears in the list before the divider and has `data-testid="export-diagnostics-action"`.
- `SessionActionsOverflow` "Workflow" accordion section contains an "Export Diagnostics" menu item.
  - *Given* the kebab menu Workflow section is expanded, *When* the section renders, *Then* an "Export Diagnostics" item appears.
- Clicking either entry point opens `ExportBundleModal` with the correct `sessionUuid` prop.
  - *Given* session UUID is `"abc123"`, *When* "Export Diagnostics" is clicked, *Then* `ExportBundleModal` opens with `sessionUuid="abc123"`.

**Files**:
- `web-app/src/components/sessions/SessionDetailView.tsx`
- `web-app/src/components/sessions/SessionActionsOverflow.tsx`

##### Task 2.1.3a: Add action sheet entry in SessionDetailView (S)
- In `web-app/src/components/sessions/SessionDetailView.tsx`, add `exportBundleOpen` state (boolean), an `<ExportBundleModal>` rendered conditionally, and an `actionSheetItem` button labeled "Export Diagnostics" with `data-testid="export-diagnostics-action"` that sets `exportBundleOpen = true` and closes the action sheet. Insert before the destructive divider.
- Files: `web-app/src/components/sessions/SessionDetailView.tsx`

##### Task 2.1.3b: Add Workflow section entry in SessionActionsOverflow (XS)
- In `web-app/src/components/sessions/SessionActionsOverflow.tsx`, add an "Export Diagnostics" `<MenuItem>` in the "Workflow" `<MenuSection>` alongside "Create PR" and "Checkpoint"; clicking it triggers the same `ExportBundleModal` pattern.
- Files: `web-app/src/components/sessions/SessionActionsOverflow.tsx`

---

### Epic 2.2: Backlog Hand-off

**Goal**: After a server-save succeeds, let the user create a pre-populated backlog item linking to the bundle's content hash and filename so the investigation becomes tracked work.

#### Story 2.2.1: "Create backlog item from bundle" Action

**As a** user who saved a bundle on the server, **I want** to create a pre-populated backlog item in one click, **so that** the investigation is tracked in the backlog without manual copy-pasting.

**Acceptance Criteria**:
- Clicking "Create backlog item →" calls `CreateBacklogItem` with `title = "Diagnostic bundle: <session title> (<YYYYMMDD>)"`, `notes` including the content hash and filename (not the absolute path), and `repo_path` from the session. **Note (B3 fix)**: standardized on UX spec title format.
  - *Given* session UUID `"abc12345-..."` with title `"Fix login flow"` saved on 2026-10-16, *When* "Create backlog item →" is clicked, *Then* the `CreateBacklogItem` RPC is called with `title = "Diagnostic bundle: Fix login flow (20261016)"` and `notes` containing `"Bundle: Fix-login-flow-20261016-143022.zip (SHA-256: a3b4c5d6...)"`.
- A success toast `"Backlog item created."` appears and a link to the new item is shown in the modal.
  - *Given* `CreateBacklogItem` returns successfully, *When* the handler resolves, *Then* `showActionToast("Backlog item created.", "success", ...)` fires.

**Files**:
- `web-app/src/components/sessions/ExportBundleModal.tsx`

##### Task 2.2.1a: Wire backlog hand-off in ExportBundleModal (S)
- In `web-app/src/components/sessions/ExportBundleModal.tsx`, add a `handleCreateBacklogItem` callback that:
  - Uses the existing `CreateBacklogItem` ConnectRPC call (same client as `useExportBundle`).
  - Builds the request from the `SaveSessionBundleResponse` and session metadata (`uuid`, `title`, `repoPath`) passed as props.
  - Sets `title = "Diagnostic bundle: <session title> (<YYYYMMDD>)"` (date from SaveSessionBundleResponse or current date).
  - Populates `notes` as: `"Bundle: <filename> (SHA-256: <contentHash>)"` — no absolute path (B4: intentional deviation from requirements AC5 which says "link to file path"; SHA-256 + filename is portable across machines).
  - On success: fires the toast, shows a "View backlog item →" link in the modal.
- Files: `web-app/src/components/sessions/ExportBundleModal.tsx`

---

## Phase 3: Instrumentation

### Epic 3.1: Browser OTel and Web Vitals Session Linkage

**Goal**: Attach `session.uuid` to browser OTel spans and `sessionId` to Web Vitals analytics events so session-scoped performance data is queryable from the bundle's analytics section.

#### Story 3.1.1: Add session.uuid to Browser OTel Spans

**As a** developer correlating a performance trace with a session, **I want** OTel spans emitted when a session is in focus to carry `session.uuid`, **so that** I can filter the trace for all spans related to that session.

**Acceptance Criteria**:
- `otel-init.ts` exports a `setActiveSessionUUID(uuid: string | null)` function that stores the UUID and attaches it as a span attribute to subsequently created spans via a custom `SpanProcessor`.
  - *Given* `setActiveSessionUUID("abc123")` is called, *When* a new fetch span is created, *Then* the span has attribute `session.uuid = "abc123"`.
- `session.uuid` is only attached to spans created after `setActiveSessionUUID` is called; spans created before (e.g., document load) do not carry it.
  - *Given* `setActiveSessionUUID` is called at t=5s, *When* the document-load span (created at t=0) is exported, *Then* it does not have `session.uuid`.
- `SessionDetailView` calls `setActiveSessionUUID(session.uuid)` when the session UUID changes and `setActiveSessionUUID(null)` on unmount.
  - *Given* the user navigates from session A to session B, *When* `SessionDetailView` re-renders with session B's data, *Then* `setActiveSessionUUID` is called with session B's UUID.

**Files**:
- `web-app/src/lib/telemetry/otel-init.ts`
- `web-app/src/components/sessions/SessionDetailView.tsx`

##### Task 3.1.1a: Add setActiveSessionUUID to otel-init.ts + call from SessionDetailView (S)
- In `web-app/src/lib/telemetry/otel-init.ts`:
  - Add a module-level `let activeSessionUUID: string | null = null`.
  - Export `setActiveSessionUUID(uuid: string | null): void` setter.
  - Register a custom `SpanProcessor` that calls `span.setAttribute("session.uuid", activeSessionUUID)` in `onStart` when `activeSessionUUID` is non-null. Add it after the existing `BatchSpanProcessor`.
- In `web-app/src/components/sessions/SessionDetailView.tsx`, add a `useEffect` that calls `setActiveSessionUUID(session?.uuid ?? null)` on mount/update and `setActiveSessionUUID(null)` in the cleanup function.
- Files: `web-app/src/lib/telemetry/otel-init.ts`, `web-app/src/components/sessions/SessionDetailView.tsx`

---

#### Story 3.1.2: Attach sessionId to Web Vitals Analytics Events

**As a** bundle assembler, **I want** Web Vitals (LCP, FID, CLS) analytics events to carry the active session's ID, **so that** the bundle's analytics section includes session-scoped performance metrics.

**Acceptance Criteria**:
- `WebVitalsReporter` passes `sessionId` to `analytics.track()` when a session UUID is available in context.
  - *Given* session UUID `"abc123"` is active and LCP fires, *When* `WebVitalsReporter` records the LCP event, *Then* the analytics event stored in SQLite has `session_id = "abc123"`.
- When no session is active (e.g., on the home/list page), `sessionId` is omitted and the event is stored without a `session_id`.
  - *Given* no session is focused, *When* a CLS event fires, *Then* the analytics event has `session_id = ""` (empty, per existing schema default).

**Files**:
- `web-app/src/lib/analytics/WebVitalsReporter.tsx`

##### Task 3.1.2a: Add sessionId label to WebVitalsReporter (S)
- In `web-app/src/lib/analytics/WebVitalsReporter.tsx`, capture the session UUID in a ref at `WebVitalsReporter` mount time: `const sessionUUIDRef = useRef(getActiveSessionUUID())`. Pass the stable ref value (`sessionUUIDRef.current`) — not a live `getActiveSessionUUID()` call — into each vitals callback's closure. This ensures a CLS/LCP event that fires after navigation still reports the correct session UUID from when it started measuring, rather than the UUID of a different session that became active later.
- Pass `sessionId: sessionUUIDRef.current ?? undefined` to `analytics.track()` for each Web Vitals call (LCP, FID, CLS handlers).
- Files: `web-app/src/lib/analytics/WebVitalsReporter.tsx`

---

## Phase 4: Verification

### Epic 4.1: Tests

**Goal**: Establish a unit test baseline for bundle assembly correctness and a single E2E test verifying that the download flow produces a file download response, satisfying acceptance criteria 12 from the requirements.

#### Story 4.1.1: Unit Tests for Bundle Assembly

**As a** future maintainer, **I want** unit tests that validate bundle structure, required fields, and the size cap, **so that** regressions in assembly logic are caught before they ship.

**Acceptance Criteria**:
- A test verifies that `assembleBundle` returns a valid ZIP with at least `manifest.json` for a minimal session.
  - *Given* an in-memory fake `Storage` returning session metadata but no analytics or scrollback, *When* `assembleBundle` is called, *Then* the result is a valid ZIP parseable by `archive/zip.NewReader` containing a `manifest.json` with a valid JSON body.
- A test verifies that the size cap drops omittable sections when the total uncompressed content exceeds 45 MB.
  - *Given* analytics events total 48 MB and scrollback totals 2 MB, *When* `assembleBundle` is called, *Then* the bundle omits `analytics` (or truncates it) and `manifest.json` lists `analytics` in `omitted_sections`.
- A test verifies that `assembleBundle` completes within 5 seconds for a session with 10,000 scrollback lines (simulated via an in-memory byte slice) and 1,000 analytics events.
  - *Given* a fake Storage returning 1,000 analytics events and a fake ScrollbackManager returning 10,000 lines, *When* `assembleBundle` runs in a test with a 5-second context deadline, *Then* the context is not cancelled before assembly completes.

**Files**:
- `server/services/session_bundle_service_test.go` (new)

##### Task 4.1.1a: Write unit tests for SessionBundleService (M)
- Create `server/services/session_bundle_service_test.go` with:
  - Fake in-memory `Storage` (implementing `session.Storage` interface) returning a minimal session, 0 analytics events, and nil scrollback.
  - Test: `TestAssembleBundle_ValidZIPWithManifest` — asserts valid ZIP, `manifest.json` present, JSON parse succeeds, `bundle_format_version = 1`.
  - Test: `TestAssembleBundle_SizeCapDropsAnalytics` — fakes 3,000 oversized analytics events; asserts `manifest.omitted_sections` contains `"analytics"` after capping.
  - Test: `TestAssembleBundle_CompletesWithinDeadline` — uses `context.WithTimeout(5s)`; asserts no deadline exceeded.
  - Test: `TestSaveSessionBundle_ContentHashMatchesFile` — verifies SHA-256 in response matches file on disk after a save.
- Files: `server/services/session_bundle_service_test.go`

---

#### Story 4.1.2: E2E Test for Bundle Download Flow

**As a** CI pipeline, **I want** an E2E test that verifies clicking "Export Diagnostics → Download" triggers a browser file download, **so that** the delivery path is exercised end-to-end against a real server.

**Acceptance Criteria**:
- The test opens a session, opens the action sheet, clicks "Export Diagnostics", selects "Download .zip", consents, and verifies a download response is triggered.
  - *Given* a test server with one active session, *When* the E2E test clicks "Export Diagnostics" → selects "Download" → checks consent → clicks "Export", *Then* the Playwright `download` event fires with a filename matching `*.zip`.
- The test uses `data-testid` locators only (no CSS class selectors), per E2E conventions.
  - *Given* the spec file is linted by the E2E conventions CI check, *When* the check runs, *Then* zero CSS class locator violations are reported.

**Files**:
- `tests/e2e/diagnostics-bundle.spec.ts` (new)

##### Task 4.1.2a: Write E2E test for bundle download flow (M)
- Create `tests/e2e/diagnostics-bundle.spec.ts`:
  - Annotate: `// @feature bundle:export, bundle:save` (line 1, per E2E convention).
  - Create a session via the test server helper.
  - Navigate to the session detail view.
  - Click `data-testid="more-actions-button"` to open the action sheet.
  - Click `data-testid="export-diagnostics-action"`.
  - Assert `ExportBundleModal` opens.
  - Select the "Download .zip" radio.
  - Check the consent checkbox.
  - `const [download] = await Promise.all([page.waitForEvent("download"), page.click("[data-testid='export-confirm-button']")])`.
  - Assert `download.suggestedFilename()` matches `/^session-.*\.zip$/`.
- No `waitForTimeout` calls; use `expect(locator).toBeVisible()` for element readiness.
- Files: `tests/e2e/diagnostics-bundle.spec.ts`

---

## Appendix: File Index

All new and modified files across all phases:

| File | New / Modified | Phase |
|------|---------------|-------|
| `session/repository.go` | Modified | 1.1 |
| `session/ent_repository.go` | Modified | 1.1 |
| `session/ent_repository_test.go` | Modified (mock/fake update) | 1.1 |
| `server/dependencies.go` | Modified | 1.1 + 1.3 |
| `proto/session/v1/rum_bundle.proto` | New | 1.2 |
| `server/server.go` | Modified | 1.2 |
| `docs/registry/features/rum-bundle.json` | New (generated) | 1.2 |
| `server/services/session_bundle_service.go` | New | 1.3 |
| `server/services/session_bundle_service_test.go` | New | 4.1 |
| `server/services/disk_space_unix.go` | New (`//go:build !windows`) | 1.3 |
| `server/services/disk_space_windows.go` | New (`//go:build windows`) | 1.3 |
| `web-app/src/lib/hooks/useExportBundle.ts` | New | 2.1 |
| `web-app/src/components/sessions/ExportBundleModal.tsx` | New | 2.1 + 2.2 |
| `web-app/src/components/sessions/ExportBundleModal.css.ts` | New | 2.1 |
| `web-app/src/components/sessions/SessionDetailView.tsx` | Modified | 2.1 + 3.1 |
| `web-app/src/components/sessions/SessionActionsOverflow.tsx` | Modified | 2.1 |
| `web-app/src/lib/telemetry/otel-init.ts` | Modified | 3.1 |
| `web-app/src/lib/analytics/WebVitalsReporter.tsx` | Modified | 3.1 |
| `tests/e2e/diagnostics-bundle.spec.ts` | New | 4.1 |
