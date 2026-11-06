# Real User Monitoring: Exportable Per-Session Diagnostic Bundle

## Item
Real user monitoring (RUM): exportable per-session diagnostic bundle with backend capture and backlog hand-off

## Problem Statement

Users experiencing bugs or performance regressions in the browser app currently have no way to capture and package the relevant session activity for investigation. Server-side OTel traces flow only to an external Datadog collector (not queryable by the app itself), analytics events are stored in SQLite but not exportable per-session, and the existing `CreateDebugSnapshot` RPC writes a local file path that cannot be downloaded via HTTP. There is no path from a captured performance event to a trackable backlog item.

## Goals

1. Allow a user to trigger an in-app export of everything relevant to a session in one compressed bundle.
2. Support two delivery modes: direct browser download and backend-saved file.
3. Create a path from a saved bundle to a backlog item so the investigation becomes tracked work.

## Success Metrics

1. Within 4 weeks of launch, at least 50% of bug reports from internal users include a diagnostic bundle (measured by backlog items tagged `diagnostic-bundle` divided by total bug-report items created in the same period).
2. P95 bundle assembly time ≤ 5 seconds for sessions with ≤ 10,000 scrollback lines and ≤ 1,000 analytics events (measured from the `ExportSessionBundle` / `SaveSessionBundle` RPC start to first byte sent, logged in structured `slog` at INFO level per the Observability Plan).
3. Zero OOM or server-crash incidents attributable to bundle assembly within the first 30 days, verified by the absence of `bundle-assembly` in crash reports and by the semaphore cap (concurrent export limit of 3) firing no more than 1% of export attempts under normal load.

## Non-Goals

- Real-time streaming or live dashboards (out of scope; see `analytics-system` plan for that direction).
- Exporting OTel spans from an external Datadog collector (not queryable by the app without a Datadog API integration; the bundle should capture locally-available data only).
- Changing the existing analytics ingestion pipeline.

## User Stories

1. **As a user**, when I encounter a bug or slowness, I want to click one button in the app and download a `.zip` bundle containing the session's diagnostics, so I can share it with a developer.
2. **As a user**, I want to trigger the server to save the bundle to disk (as an alternative to download), so I can reference it later without sending a file.
3. **As a developer or operator**, I want to create a backlog item from a saved bundle (pre-populated with relevant context), so the investigation becomes tracked work.
4. **As the system**, I want browser-side OTel spans and Web Vitals to be tagged with the current session UUID, so bundle data is correlatable across sources.

## Acceptance Criteria

1. A UI control (button or menu item) in the session detail view triggers a session diagnostic bundle export.
2. The bundle includes: session metadata (UUID, title, branch, status, program, timestamps), analytics events for the session, context history samples and compaction events, session artifacts (PR URLs, commits, commands), the most recent scrollback tail (up to 500 lines), server info (PID, Go version, OS/arch), and backlog linkage if any.
3. Clicking "Download" in the UI initiates a browser file download of a `.zip` bundle containing a structured JSON manifest.
4. Clicking "Save on server" invokes a backend RPC that writes the bundle to `~/.stapler-squad/diagnostics/<session-uuid>-<timestamp>.zip` and returns the file path in the response.
5. A "Create backlog item from bundle" action is available after a bundle is saved on the server; it creates a `BacklogItem` pre-populated with the bundle's session title, a summary, and the bundle's filename plus SHA-256 content hash. Absolute file paths are excluded — they are machine-local and non-portable.
6. Browser OTel spans emitted by the web app include a `session.uuid` attribute set to the active session's UUID (when a session is in focus), enabling span→session correlation.
7. Web Vitals (LCP, FID, CLS) reported by `WebVitalsReporter` are attached as analytics events with the current session's ID for session-scoped performance analysis.
8. The new `ExportSessionDiagnosticBundle` RPC is protected by the same auth middleware as all other ConnectRPC endpoints.
9. Bundle assembly completes within 5 seconds for a session with up to 10,000 scrollback lines and 1,000 analytics events.
10. The saved bundle file is no larger than 50 MB (before compression).
11. The feature works and degrades gracefully when OTel is disabled (bundle omits trace data rather than failing).
12. At least one unit test covers bundle assembly (valid JSON, required fields present, size cap enforced) and at least one E2E test validates the download flow triggers a file download response.

## Data Sources (Locally Available)

| Source | Location | Notes |
|--------|----------|-------|
| Session metadata | `session/ent/schema/session.go` | uuid, title, branch, status, program, timestamps |
| Analytics events | `server/analytics/sqlite_provider.go` | Filter by session_id |
| Context history | `session/contexthistory/` | Samples + compaction events |
| Artifacts | `session/artifacts/types.go` | PR URLs, commits, commands |
| Scrollback | `session/scrollback/manager.go` | Zstd-compressed circular buffer |
| Backlog linkage | `BacklogService.GetSessionBacklogIndex` | Maps session UUIDs to backlog items |
| Server info | `server/services/debug_snapshot.go` (`ServerInfo`) | PID, Go version, OS, arch |

## Existing Patterns to Follow

- **Export RPC pattern**: `ExportRules` in `session.proto` → `useExportRules.ts` hook → `<a download>` trigger
- **Diagnostic bundle assembly**: `session/diagnose_bundle.go` (`BuildDiagnosticBundle`) — section-based assembly with size budget
- **Debug snapshot**: `server/services/debug_snapshot.go` — collects server state; adapt for per-session scoping
- **Log export**: `web-app/src/components/logs/ExportButton.tsx` — `Blob` + `URL.createObjectURL` pattern
- **OTel attribute tagging**: `telemetry/attributes.go` — define `session.uuid` attribute key; set in `otel-init.ts`

## Constraints

- OTel spans forwarded to Datadog are not retrievable from within the app. Bundle will NOT include raw trace data.
- Scrollback is capped at 500 lines in the bundle (scrollback manager already enforces a 10,000-line per-session cap, but the bundle should be smaller).
- `session/ent/*.go` is gitignored (generated); schema changes go in `session/ent/schema/` only.
- New proto RPCs require running `make proto-gen` and must be registered in `server/server.go`.
- `web-app/` uses `pnpm`, never `npm`/`yarn`.
- New proto types need entries in the Feature Registry (`make registry-generate`).
