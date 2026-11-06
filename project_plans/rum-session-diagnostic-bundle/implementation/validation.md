# Validation Plan: RUM Session Diagnostic Bundle

**Date**: 2026-10-16
**Status**: Ready

---

## Coverage Matrix

| Req # | Requirement | Test Cases | Type |
|-------|-------------|------------|------|
| 1 | UI control in session detail view triggers export | TC-032, TC-033 | e2e |
| 2 | Bundle includes session metadata, analytics, ctx history, scrollback, server info, backlog linkage | TC-001, TC-003, TC-013, TC-020 | unit, integration |
| 3 | "Download" initiates browser file download of `.zip` | TC-030 | e2e |
| 4 | "Save on server" writes to `~/.stapler-squad/diagnostics/<uuid>-<timestamp>.zip` | TC-007, TC-022, TC-031 | unit, integration, e2e |
| 5 | "Create backlog item" available after server-save; pre-populated correctly | TC-031, TC-037 | e2e |
| 6 | Browser OTel spans include `session.uuid` attribute | TC-039 | unit (Jest) |
| 7 | Web Vitals events carry active session's ID | TC-040 | unit (Jest) |
| 8 | `ExportSessionBundle` and `SaveSessionBundle` protected by auth middleware | TC-023, TC-024 | integration |
| 9 | Assembly completes within 5 seconds for 10k scrollback + 1k analytics | TC-009 | unit |
| 10 | Saved bundle ≤ 50 MB (size cap enforced) | TC-002, TC-011 | unit |
| 11 | Graceful degradation when OTel disabled | TC-012, TC-035 | unit, e2e |
| 12 | At least one unit test (assembly) and one E2E test (download flow) | TC-001, TC-030 | unit, e2e |
| UX AC1 | Entry points discoverable from both action sheet and kebab menu | TC-032, TC-033 | e2e |
| UX AC3 | Download file has `.zip` extension and correct filename pattern | TC-030 | e2e |
| UX AC4 | Server-save shows file path; mobile defaults to "Save on server" | TC-031, TC-036 | e2e |
| UX AC5 | "Create backlog item →" available after save, not after download | TC-031, TC-037 | e2e |
| UX AC9 | In-progress feedback within 200 ms; Cancel available after 5 s | TC-038 | e2e |
| UX AC11 | OTel disabled notice is non-blocking informational banner | TC-035 | e2e |
| UX AC-A | ARIA roles, keyboard nav, color contrast | TC-034 | e2e |
| Story 1.1.1 | `ListAnalyticsBySessionID` ordering (ASC by created_at) and 2000-row cap | TC-005, TC-006 | unit |
| Story 1.1.3 | `GetSessionArtifactsByUUID` scoped to correct session | TC-013 | unit |
| Story 1.3.1 | `assembleBundle` calls `FlushSession` before reading scrollback | TC-010 | unit |
| Story 1.3.3 | `RunDiagnosticsRetention` prunes files older than 30 days | TC-008 | unit |
| Story 1.3.3 | `SaveSessionBundle` returns `CodeResourceExhausted` when disk < 100 MB | TC-025 | integration |
| Edge case | `CtxHistoryStore` nil → bundle succeeds, `context_history` in `omitted_sections` | TC-004 | unit |
| Edge case | Session not found → `CodeNotFound` | TC-021 | integration |
| Edge case | Bundle exactly at 50 MB boundary | TC-011 | unit |

---

## Test Cases

### Unit Tests

#### TC-001: assembleBundle returns valid ZIP containing manifest.json
- **Covers**: Requirement #2, #12; Story 4.1.1 AC1
- **Precondition**: In-memory fake `Storage` returning a minimal session record (UUID, title, branch, status); zero analytics events; fake `ScrollbackReader` returning empty bytes; `ctxHistory` non-nil returning empty series.
- **Steps**:
  1. Construct `SessionBundleService` with fakes.
  2. Call `assembleBundle(ctx, "test-uuid-001", 500)`.
  3. Parse the returned `[]byte` with `archive/zip.NewReader`.
  4. Locate `manifest.json` in the ZIP entries.
  5. Unmarshal the JSON body into `BundleManifest`.
- **Expected**: No error; ZIP is parseable; `manifest.json` is present; `BundleManifest.BundleFormatVersion == 1`; `BundleManifest.SessionUUID == "test-uuid-001"`; `BundleManifest.ServerVersion` is non-empty.
- **File**: `server/services/session_bundle_service_test.go` — `TestAssembleBundle_ValidZIPWithManifest`

---

#### TC-002: assembleBundle drops analytics section when total uncompressed bytes exceed 45 MB
- **Covers**: Requirement #10; Story 4.1.1 AC2; UX AC10 size cap
- **Precondition**: Fake `Storage` returning 3,000 analytics events each with a 16 KB payload (total ≈ 48 MB uncompressed). Scrollback returns 2 MB of lines.
- **Steps**:
  1. Construct service with oversized-analytics fake.
  2. Call `assembleBundle`.
  3. Parse ZIP, extract `manifest.json`, unmarshal.
- **Expected**: `BundleManifest.OmittedSections` contains `"analytics"`; ZIP is still valid and parseable; other required sections (session metadata, server info) are present.
- **File**: `server/services/session_bundle_service_test.go` — `TestAssembleBundle_SizeCapDropsAnalytics`

---

#### TC-003: assembleBundle captures scrollback tail (most-recent N lines, not oldest N)
- **Covers**: Requirement #2; Story 1.3.1 plan note on `GetRecentLines` vs `GetScrollback`
- **Precondition**: Fake `ScrollbackReader.GetRecentLines("test-uuid", 500)` returns a deterministic 500-line byte slice whose last line is `"line-10000"`. Fake `GetScrollback(0, 500)` would return `"line-1"` through `"line-500"` if called.
- **Steps**:
  1. Call `assembleBundle` with `maxScrollbackLines = 500`.
  2. Parse ZIP, extract `scrollback.txt` (or equivalent section file).
  3. Read its contents.
- **Expected**: The scrollback section byte content matches the `GetRecentLines` response (contains `"line-10000"`); `GetScrollback` is never called (verified via call count on the fake).
- **File**: `server/services/session_bundle_service_test.go` — `TestAssembleBundle_ScrollbackTailUsesGetRecentLines`

---

#### TC-004: assembleBundle succeeds with omitted context_history section when CtxHistoryStore is nil
- **Covers**: Requirement #11; Story 1.1.2 AC nil-guard
- **Precondition**: Construct `SessionBundleService` with `ctxHistory = nil`. Fake `Storage` returns a valid session.
- **Steps**:
  1. Call `assembleBundle`.
  2. Parse ZIP, extract `manifest.json`.
- **Expected**: No error returned; ZIP is valid; `BundleManifest.OmittedSections` contains `"context_history"` with reason `"context_history_unavailable"`.
- **File**: `server/services/session_bundle_service_test.go` — `TestAssembleBundle_NilCtxHistory_OmitsSection`

---

#### TC-005: ListAnalyticsBySessionID returns events for the correct session ordered ASC by created_at
- **Covers**: Story 1.1.1 AC1
- **Precondition**: In-memory SQLite (or ent test client). Insert 50 analytics events for session `"abc123"` with ascending timestamps; insert 20 events for session `"other-session"`.
- **Steps**:
  1. Call `storage.ListAnalyticsBySessionID(ctx, "abc123")`.
  2. Inspect result count and timestamp ordering.
- **Expected**: Returns exactly 50 events; all events have `SessionID == "abc123"`; events are ordered `created_at ASC` (each event's timestamp ≥ previous).
- **File**: `server/services/session_bundle_service_test.go` (or `session/ent_repository_test.go`) — `TestListAnalyticsBySessionID_OrderingAndFiltering`

---

#### TC-006: ListAnalyticsBySessionID caps results at 2000 events for long-lived sessions
- **Covers**: Story 1.1.1 AC2
- **Precondition**: Insert 3,000 analytics events for a single session UUID into the test database.
- **Steps**:
  1. Call `storage.ListAnalyticsBySessionID(ctx, sessionUUID)`.
  2. Count returned rows.
- **Expected**: Returns exactly 2,000 events (not 3,000); the 2,000 returned are the most recent by `created_at`.
- **File**: `session/ent_repository_test.go` — `TestListAnalyticsBySessionID_LimitCap`

---

#### TC-007: SaveSessionBundle writes file and response content_hash matches file on disk
- **Covers**: Requirement #4; Story 1.3.3 AC1
- **Precondition**: Temporary directory as diagnostics dir. Fake `Storage` returns a valid session. `SessionBundleService` constructed with that dir as output.
- **Steps**:
  1. Call `SaveSessionBundle` via the service method directly (not over RPC) with `sessionUUID = "abc123"`.
  2. List files in the temp dir matching `abc123-*.zip`.
  3. Read the file bytes, compute SHA-256 hex.
  4. Compare to `SaveSessionBundleResponse.ContentHash`.
- **Expected**: Exactly one matching file is created; the SHA-256 of the file bytes equals `response.ContentHash`; `response.FileSizeBytes` matches the file size on disk; the file is a valid ZIP.
- **File**: `server/services/session_bundle_service_test.go` — `TestSaveSessionBundle_ContentHashMatchesFile`

---

#### TC-008: RunDiagnosticsRetention prunes files older than 30 days and retains newer ones
- **Covers**: Story 1.3.3 AC2 (30-day retention)
- **Precondition**: Temporary directory with three `.zip` files: one with `mtime` 31 days ago, one 29 days ago, one 1 day ago.
- **Steps**:
  1. Call `RunDiagnosticsRetention(dir, 30*24*time.Hour)`.
  2. List remaining `.zip` files.
- **Expected**: The 31-day-old file is deleted; the 29-day and 1-day files are retained; no error is returned; an `INFO` log line is emitted for the deleted file.
- **File**: `server/services/session_bundle_service_test.go` — `TestRunDiagnosticsRetention_PrunesOldFiles`

---

#### TC-009: assembleBundle completes within 5-second context deadline for large session
- **Covers**: Requirement #9; Story 4.1.1 AC3
- **Precondition**: Fake `ScrollbackReader.GetRecentLines` returns a 10,000-line byte slice (synchronously, no artificial delay). Fake `Storage.ListAnalyticsBySessionID` returns 1,000 analytics events (each with a 1 KB payload). `context.WithTimeout(5 * time.Second)` applied.
- **Steps**:
  1. Call `assembleBundle` under the 5-second context deadline.
  2. Check whether the context was cancelled before the call returned.
- **Expected**: `assembleBundle` returns before the context deadline; `ctx.Err()` is nil; no `context.DeadlineExceeded` error.
- **File**: `server/services/session_bundle_service_test.go` — `TestAssembleBundle_CompletesWithinDeadline`

---

#### TC-010: assembleBundle calls FlushSession before reading scrollback
- **Covers**: Story 1.3.1 AC4 (FlushSession TOCTOU guard)
- **Precondition**: Fake `ScrollbackReader` with call-order tracking. `FlushSession` marks a flag; `GetRecentLines` asserts the flag is already set.
- **Steps**:
  1. Call `assembleBundle`.
  2. Inspect call-order log on the fake.
- **Expected**: `FlushSession` is called before `GetRecentLines`; both are called exactly once; the fake does not trigger an ordering-violation error.
- **File**: `server/services/session_bundle_service_test.go` — `TestAssembleBundle_FlushBeforeRead`

---

#### TC-011: assembleBundle at exactly 50 MB boundary — ZIP is valid, no section dropped
- **Covers**: Requirement #10 boundary condition
- **Precondition**: Fake analytics payload sized to fill exactly 45 MB (the per-section budget ceiling, below the global 50 MB cap). Scrollback returns 4 MB. Total uncompressed ≈ 49.9 MB (under the 50 MB limit).
- **Steps**:
  1. Call `assembleBundle`.
  2. Parse ZIP; check `manifest.json` for `omitted_sections`.
- **Expected**: No sections are dropped (all fit under the 50 MB ceiling); `BundleManifest.OmittedSections` is empty or absent; ZIP is valid.
- **File**: `server/services/session_bundle_service_test.go` — `TestAssembleBundle_ExactSizeBoundary`

---

#### TC-012: assembleBundle succeeds and omits trace data when OTel is disabled
- **Covers**: Requirement #11; Story 1.1.2 AC nil-guard pattern (OTel path)
- **Precondition**: `SessionBundleService` constructed with `ctxHistory` non-nil but with a server config indicating OTel is disabled (or the OTel exporter returning a "disabled" sentinel). Any trace-related collection step is skipped.
- **Steps**:
  1. Call `assembleBundle`.
  2. Parse ZIP, check `manifest.json`.
- **Expected**: No error; ZIP is valid and parseable; `BundleManifest` does not include a `trace_data` section; `OmittedSections` notes `"otel_trace"` with reason `"otel_disabled"` (if the section is defined) or the section simply absent.
- **File**: `server/services/session_bundle_service_test.go` — `TestAssembleBundle_OTelDisabled_NoTraceData`

---

#### TC-013: GetSessionArtifactsByUUID returns artifacts scoped to the requested session
- **Covers**: Requirement #2 (artifacts in bundle); Story 1.1.3 AC
- **Precondition**: Test database with two sessions. Session `"abc123"` has two PR URL artifacts; session `"other-xyz"` has one commit SHA artifact.
- **Steps**:
  1. Call `storage.GetSessionArtifactsByUUID(ctx, "abc123")`.
  2. Inspect returned blob.
- **Expected**: Returned blob contains exactly 2 PR URL records; no artifacts from `"other-xyz"` are present; no error.
- **File**: `session/ent_repository_test.go` — `TestGetSessionArtifactsByUUID_ScopedToSession`

---

### Integration Tests

#### TC-020: ExportSessionBundle streams BundleChunks that reassemble to a valid ZIP
- **Covers**: Requirement #3; Story 1.3.2 AC1
- **Precondition**: Test ConnectRPC server with `SessionBundleService` wired up. One active session in the test database. No auth bypass needed (use a test auth token).
- **Steps**:
  1. Open a server-streaming call to `ExportSessionBundle` with a valid `session_uuid`.
  2. Collect all `BundleChunk` messages until `final = true`.
  3. Concatenate `chunk.data` in order of `chunk_index`.
  4. Parse the concatenated bytes with `archive/zip.NewReader`.
- **Expected**: All chunks received without error; final chunk has `final = true`; reassembled bytes form a valid ZIP containing `manifest.json`; chunk indices are sequential from 0.
- **File**: `server/services/session_bundle_service_test.go` — `TestExportSessionBundle_StreamsValidZIP`

---

#### TC-021: ExportSessionBundle returns CodeNotFound for an unknown session UUID
- **Covers**: Story 1.3.2 AC2; edge case "session not found"
- **Precondition**: Test ConnectRPC server. No session with UUID `"nonexistent-uuid-9999"` in the database.
- **Steps**:
  1. Call `ExportSessionBundle` with `session_uuid = "nonexistent-uuid-9999"`.
  2. Capture the stream error.
- **Expected**: Stream terminates immediately with a ConnectRPC error; error code is `connect.CodeNotFound` (HTTP 404); no `BundleChunk` messages are sent.
- **File**: `server/services/session_bundle_service_test.go` — `TestExportSessionBundle_NotFound`

---

#### TC-022: SaveSessionBundle writes file and returns correct content hash
- **Covers**: Requirement #4; Story 1.3.3 AC1
- **Precondition**: Test ConnectRPC server with a writable temp `diagnostics/` directory. One active session in the test database.
- **Steps**:
  1. Call `SaveSessionBundle` RPC with a valid `session_uuid`.
  2. Read `response.FilePath`, `response.FileSizeBytes`, `response.ContentHash`.
  3. Open the file at `response.FilePath`, compute SHA-256.
- **Expected**: RPC succeeds; file exists at `response.FilePath`; SHA-256 of file bytes equals `response.ContentHash`; `os.Stat(filePath).Size()` equals `response.FileSizeBytes`; file is a valid ZIP.
- **File**: `server/services/session_bundle_service_test.go` — `TestSaveSessionBundle_Integration_FileAndHash`

---

#### TC-023: Unauthenticated call to ExportSessionBundle is rejected
- **Covers**: Requirement #8; Story 1.2.1 AC2
- **Precondition**: Test ConnectRPC server with the same `ConnectOptions` auth interceptors as production. No auth token in the request.
- **Steps**:
  1. Send `ExportSessionBundle` request with no `Authorization` header.
  2. Capture the response error.
- **Expected**: Response status is `401 Unauthorized` (or ConnectRPC `connect.CodeUnauthenticated`); no bundle data is streamed.
- **File**: `server/services/session_bundle_service_test.go` — `TestExportSessionBundle_Unauthenticated`

---

#### TC-024: Unauthenticated call to SaveSessionBundle is rejected
- **Covers**: Requirement #8; Story 1.2.1 AC2
- **Precondition**: Same as TC-023.
- **Steps**:
  1. Send `SaveSessionBundle` request with no auth token.
  2. Capture the response error.
- **Expected**: Response is `401 Unauthorized`; no file is written to the diagnostics directory.
- **File**: `server/services/session_bundle_service_test.go` — `TestSaveSessionBundle_Unauthenticated`

---

#### TC-025: SaveSessionBundle returns CodeResourceExhausted when available disk < 100 MB
- **Covers**: Story 1.3.3 AC3; edge case "disk full"
- **Precondition**: `SessionBundleService` configured with a fake `diskSpaceChecker` that reports 50 MB free. Valid session in test database.
- **Steps**:
  1. Call `SaveSessionBundle` RPC.
  2. Capture the error code and message.
- **Expected**: RPC returns `connect.CodeResourceExhausted`; error message contains `"less than 100MB free"` (exact string per plan); no file is written.
- **File**: `server/services/session_bundle_service_test.go` — `TestSaveSessionBundle_DiskFull_ResourceExhausted`

---

#### TC-026: DiagnosticsRetention runs at startup and does not hard-fail when directory is absent
- **Covers**: Story 1.3.3b (startup integration); edge case missing directory
- **Precondition**: `BuildRuntimeDeps` call with a config directory pointing to a temp path where `diagnostics/` does not yet exist.
- **Steps**:
  1. Run `RunDiagnosticsRetention` against a non-existent directory.
  2. Check returned error.
- **Expected**: No error returned (or error is logged but swallowed); the server continues to start normally; subsequent `SaveSessionBundle` creates the directory on demand.
- **File**: `server/services/session_bundle_service_test.go` — `TestRunDiagnosticsRetention_MissingDir_NoHardFail`

---

### Frontend Unit Tests (Jest)

#### TC-027: useExportBundle download() assembles chunks and triggers <a download> click
- **Covers**: Story 2.1.1 AC1
- **Precondition**: Mock `connectrpc` client where `exportSessionBundle` returns three `BundleChunk` messages. Mock `document.createElement` + click spy.
- **Steps**:
  1. Render a test component using `useExportBundle`.
  2. Call `download("test-uuid")`.
  3. Assert the `<a>` element was created with `download` attribute and `.click()` was called.
- **Expected**: `document.createElement("a")` called once; `href` is a `blob:` URL; `download` attribute matches `/^[a-z0-9-]+-\d{8}-\d{6}\.zip$/` (title-slug + YYYYMMDD-HHMMSS); `URL.revokeObjectURL` called after click.
- **File**: `web-app/src/lib/hooks/useExportBundle.test.ts`

---

#### TC-028: useExportBundle surfaces error state when RPC returns CodeNotFound
- **Covers**: Story 2.1.1 AC3
- **Precondition**: Mock client where `exportSessionBundle` rejects with a ConnectRPC `CodeNotFound` error.
- **Steps**:
  1. Call `download("bad-uuid")`.
  2. Wait for hook state update.
- **Expected**: `loading` becomes `false`; `error` is the string `"Session not found"`; no `<a>` element click occurs.
- **File**: `web-app/src/lib/hooks/useExportBundle.test.ts` — `test('surfaces CodeNotFound as error string')`

---

#### TC-039: setActiveSessionUUID attaches session.uuid to subsequently created spans
- **Covers**: Requirement #6; Story 3.1.1 AC1
- **Precondition**: JSDOM test environment. Import `setActiveSessionUUID` from `otel-init.ts`. Mock OTel `SpanProcessor` to capture `span.attributes`.
- **Steps**:
  1. Call `setActiveSessionUUID("abc123")`.
  2. Start a new span via the tracer.
  3. Inspect the span's attributes via the mock processor's `onStart` capture.
- **Expected**: Span has attribute `"session.uuid"` with value `"abc123"`.
- **File**: `web-app/src/lib/telemetry/otel-init.test.ts` — `test('setActiveSessionUUID tags subsequent spans')`

---

#### TC-040: WebVitalsReporter passes sessionId to analytics.track when session active
- **Covers**: Requirement #7; Story 3.1.2 AC1
- **Precondition**: Call `setActiveSessionUUID("sess-xyz")`. Mock `analytics.track` spy.
- **Steps**:
  1. Trigger a synthetic LCP event via `WebVitalsReporter`.
  2. Inspect `analytics.track` call arguments.
- **Expected**: `analytics.track` called with `sessionId: "sess-xyz"` in the labels/properties object for the LCP event.
- **File**: `web-app/src/lib/analytics/WebVitalsReporter.test.tsx` — `test('attaches sessionId when session is active')`

---

### UX Acceptance Tests (Playwright E2E)

#### TC-030: Download flow — modal opens, consent checked, download event fires with *.zip filename
- **Covers**: Requirements #3, #12; UX AC3.1, AC3.2, AC3.4
- **Precondition**: Test server with one active session. Playwright download interception enabled.
- **Steps**:
  1. Navigate to the session detail view.
  2. Click `[data-testid="more-actions-button"]` to open the action sheet.
  3. Click `[data-testid="export-diagnostics-action"]`.
  4. Assert `ExportBundleModal` is visible (role `dialog` with name matching "Export Session Diagnostics").
  5. Click the "Download .zip" radio option.
  6. Check the consent checkbox.
  7. `const [download] = await Promise.all([page.waitForEvent("download"), page.click("[data-testid='export-confirm-button']")])`.
  8. Assert `download.suggestedFilename()`.
- **Expected**: `download.suggestedFilename()` matches `/^[a-z0-9-]+-\d{8}-\d{6}\.zip$/` (title-slug + YYYYMMDD-HHMMSS pattern); download event fires within 10 seconds; modal closes after download.
- **File**: `tests/e2e/diagnostics-bundle.spec.ts` — `test('download flow fires download event with .zip filename')`

---

#### TC-031: Save-on-server flow — file path shown, "Create backlog item →" button appears
- **Covers**: Requirements #4, #5; UX AC4.1, AC4.2, AC5.1, AC5.3
- **Precondition**: Test server with one active session.
- **Steps**:
  1. Open `ExportBundleModal` via the action sheet.
  2. Click the "Save on server" radio option.
  3. Check the consent checkbox.
  4. Click `[data-testid="export-confirm-button"]`.
  5. Wait for the success state to appear.
  6. Read the displayed file path text.
  7. Assert `[data-testid="create-backlog-item-from-bundle"]` is visible.
- **Expected**: Modal shows success state with a file path containing `diagnostics/`; `[data-testid="create-backlog-item-from-bundle"]` is visible and focusable; the "Create backlog item →" button is not visible in the download flow (AC5.3).
- **File**: `tests/e2e/diagnostics-bundle.spec.ts` — `test('save-on-server flow shows file path and backlog button')`

---

#### TC-032: "Export Diagnostics" appears in SessionDetailView action sheet before the destructive divider
- **Covers**: Requirement #1; UX AC1.1, AC1.3
- **Precondition**: Test server with one active session. Navigate to session detail view.
- **Steps**:
  1. Click `[data-testid="more-actions-button"]`.
  2. Locate `[data-testid="export-diagnostics-action"]`.
  3. Check its DOM position relative to the `<hr>` divider.
- **Expected**: `[data-testid="export-diagnostics-action"]` is present in the action sheet; it appears before the destructive `<hr>` divider element in DOM order; clicking it opens `ExportBundleModal` with the correct session UUID prop (verifiable via the modal's heading or a hidden `data-session-uuid` attribute).
- **File**: `tests/e2e/diagnostics-bundle.spec.ts` — `test('Export Diagnostics item appears before destructive divider')`

---

#### TC-033: "Export Diagnostics" appears in SessionActionsOverflow Workflow section
- **Covers**: Requirement #1; UX AC1.2, AC1.4
- **Precondition**: Test server with one active session. Session list view.
- **Steps**:
  1. Locate a session row and click its kebab overflow button.
  2. Expand (or verify auto-expanded) the "Workflow" accordion section.
  3. Locate the "Export Diagnostics" menu item.
  4. Click it and assert modal opens.
- **Expected**: "Export Diagnostics" `[role="menuitem"]` is present under the "Workflow" section header; it appears after "Create Checkpoint" in DOM order; clicking it opens `ExportBundleModal`.
- **File**: `tests/e2e/diagnostics-bundle.spec.ts` — `test('kebab menu Workflow section has Export Diagnostics item')`

---

#### TC-034: Accessibility — ARIA roles, keyboard navigation, Export button aria-disabled state
- **Covers**: UX AC-A1, AC-A4, AC-A6
- **Precondition**: `ExportBundleModal` opened via action sheet.
- **Steps**:
  1. Inspect the trigger button `[data-testid="more-actions-button"]` for `aria-haspopup`.
  2. Open modal; do NOT check the consent checkbox.
  3. Inspect `[data-testid="export-confirm-button"]` for `aria-disabled="true"`.
  4. Trigger an export error by disconnecting the mock server; assert `role="alert"` on the error banner.
  5. Tab through the modal and verify focus order: radio group → consent checkbox → Cancel → Export button.
- **Expected**: Trigger button has `aria-haspopup="dialog"` (or `="true"`); Export button is `aria-disabled="true"` when consent unchecked; error banner carries `role="alert"`; Tab sequence follows documented order.
- **File**: `tests/e2e/diagnostics-bundle.spec.ts` — `test('modal ARIA roles and keyboard nav are correct')`

---

#### TC-035: OTel-disabled informational notice is non-blocking and not shown when OTel enabled
- **Covers**: Requirement #11; UX AC11.1, AC11.2, AC11.4
- **Precondition**: Two test server configurations — one with OTel disabled (env var or config flag), one with OTel enabled.
- **Steps** (OTel disabled):
  1. Open `ExportBundleModal`.
  2. Assert the informational notice is visible (text contains "OpenTelemetry is disabled").
  3. Assert the notice does NOT have `role="alert"`.
  4. Check the consent checkbox and assert Export button becomes enabled.
- **Steps** (OTel enabled):
  1. Open modal; assert the OTel notice is absent.
- **Expected** (OTel disabled): Notice present, not an alert, Export not blocked; **Expected** (OTel enabled): Notice absent.
- **File**: `tests/e2e/diagnostics-bundle.spec.ts` — `test('OTel disabled shows non-blocking info notice')`

---

#### TC-036: Mobile viewport defaults "Save on server" delivery mode
- **Covers**: UX AC4.3; Story 2.1.2 AC1
- **Precondition**: Playwright viewport set to `375 × 812` (iPhone SE) before navigating.
- **Steps**:
  1. Navigate to session detail view.
  2. Open `ExportBundleModal` via action sheet.
  3. Inspect the "Save on server" radio's `checked` state.
  4. Inspect the "Download .zip" radio's `checked` state.
- **Expected**: "Save on server" radio is pre-selected (`checked = true`); "Download .zip" radio is not checked.
- **File**: `tests/e2e/diagnostics-bundle.spec.ts` — `test('mobile viewport pre-selects Save on server')`

---

#### TC-037: "Create backlog item →" creates BacklogItem with correct title and notes
- **Covers**: Requirement #5; UX AC5.2; Story 2.2.1 AC1
- **Precondition**: Complete a save-on-server export (TC-031 setup). Test server allows `CreateBacklogItem` RPC.
- **Steps**:
  1. After server-save succeeds, click `[data-testid="create-backlog-item-from-bundle"]`.
  2. Wait for the success toast "Backlog item created." to appear.
  3. Retrieve the last `CreateBacklogItem` RPC call from the test server's request log.
  4. Inspect `request.title` and `request.notes`.
- **Expected**: `request.title` matches `"Diagnostic bundle: <session title> (<YYYYMMDD>)"`; `request.notes` contains `"Bundle: <filename> (SHA-256: <contenthash>)"`; no absolute `~/.stapler-squad/...` path appears in `notes`.
- **File**: `tests/e2e/diagnostics-bundle.spec.ts` — `test('Create backlog item populates title and notes correctly')`

---

#### TC-038: In-progress feedback appears within 200 ms and Cancel shown after 5 s
- **Covers**: UX AC9.1, AC9.3
- **Precondition**: Mock server that artificially delays the `ExportSessionBundle` stream for 7 seconds.
- **Steps**:
  1. Record timestamp T0 immediately before clicking Export.
  2. Click `[data-testid="export-confirm-button"]`.
  3. Assert "Assembling bundle…" label appears; record timestamp T1.
  4. Wait 5.5 seconds.
  5. Assert "Taking longer than expected…" label appears and a Cancel button is enabled.
- **Expected**: T1 − T0 < 200 ms; after 5+ seconds without completion, label changes and Cancel is clickable; clicking Cancel closes modal without error.
- **File**: `tests/e2e/diagnostics-bundle.spec.ts` — `test('in-progress feedback within 200ms; Cancel after 5s')`

---

## Pre-flight Checklist

- [ ] `make build` passes (compiles proto stubs + ent generated code)
- [ ] `make test` passes (Go unit + integration — covers TC-001 through TC-026)
- [ ] `cd web-app && pnpm run lint` passes (NOT npm/yarn)
- [ ] `cd web-app && npx jest --no-coverage` passes (covers TC-027, TC-028, TC-039, TC-040)
- [ ] `cd tests/e2e && npm test` passes (Playwright E2E — covers TC-030 through TC-038)
- [ ] `make registry-generate` run and `docs/registry/features/rum-bundle.json` committed (verifies TC of Story 1.2.1 AC3)
- [ ] `make lint` passes (golangci-lint, includes `dupl` new-code gate and `noarchivedrevival`/`norawghrequest` custom linters)
- [ ] `cd web-app && pnpm run lint:duplicates` passes (jscpd threshold not exceeded by new test files)
