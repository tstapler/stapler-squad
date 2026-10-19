# RUM Session Diagnostic Bundle — Feature Landscape Research

## 1. Existing Export / Diagnostic Features

### server/services/debug_snapshot.go — CreateDebugSnapshot
- `DebugSnapshot` struct (Version=1) captures: `ServerInfo`, `[]SessionSnapshot`, `TmuxSnapshot`, `ApprovalSnapshot`, `RecentLogsSnapshot`, `Errors`.
- `CollectSnapshot` writes server-wide state; each `SessionSnapshot` includes status, program, branch, pane content (capped at `maxPaneContentBytes = 10000` bytes), timestamps, GitHub PR number.
- `CreateDebugSnapshot` RPC writes JSON to the server log directory. Always server-side only — no browser download path exists.
- **Gap for RUM feature**: scope is server-wide (all sessions), not per-session. No analytics events, context history, or backlog linkage. No version field evolution path.

### session/diagnose_bundle.go — BuildDiagnosticBundle
- Assembles an LLM-prompt bundle for the Diagnose & Nudge agent — not a user-visible artifact.
- Sections: `item` (description, ACs, status), `review_verdicts`, `linked_sessions`, `recent_logs`, `git_diff`, `git_log`.
- Compaction: whole omittable sections dropped (never mid-section truncation) when over `DiagnosticBundleTokenBudget`. Hard ceiling `maxDiagnosticBundleBytes = 4 * budget * bytesPerToken` as last resort.
- Input type `DiagnosticBundleInput` is pre-fetched by caller; `BuildDiagnosticBundle` is a pure function — easily unit-testable.
- **Relevance**: the section-compaction pattern (priority-ordered drop, hard ceiling safety net) is directly reusable for large-session bundle size management.

### web-app/src/components/logs/ExportButton.tsx
- Exports log entries (JSON / CSV / plain text) as browser downloads via `Blob` + `URL.createObjectURL` + `<a download>` click pattern.
- State: `showDropdown`, `exporting`. Dropdown with keyboard-accessible `role="menu"` / `role="menuitem"`.
- Also offers "Copy All" to clipboard via `navigator.clipboard.writeText`.
- **Reuse**: the Blob-download pattern is the exact client-side delivery path for the new .zip bundle download mode.

### web-app/src/lib/hooks/useExportRules.ts
- Hook pattern: calls a backend RPC (`exportRules`), receives YAML content, creates `Blob`, triggers download.
- Clear template for a `useExportDiagnosticBundle` hook that calls a new backend RPC and either downloads a zip or posts to a backend-save endpoint.

---

## 2. Large Session Edge Cases

### session/scrollback — CircularBuffer
- `session/scrollback/buffer.go`: `CircularBuffer` with configurable `maxSize` (entry count). Oldest entries evicted when full — no byte-level cap in the scrollback package itself.
- In `session/claude_controller.go`: `NewCircularBuffer(256 * 1024)` — the "output" circular buffer is 256 KB.
- The scrollback `Manager` (`session/scrollback/manager.go`) persists to disk; `Storage` handles durable writes.
- **Edge case**: for long-running sessions the scrollback file can be large. The bundle should apply a tail cap (byte count) before zipping, similar to `debug_snapshot.go`'s `maxPaneContentBytes`.

### session/contexthistory — Context History Samples
- `DefaultRetention = 90 * 24 * time.Hour` (90 days). Pruned every 6 hours.
- `Sample` struct: `SessionUUID`, `TurnIndex`, `SampledAt`, `Model`, `ContextTokens`, `ContextMax`.
- `SessionSummary` aggregates `FractionWarn` (≥75%) and `FractionCritical` (≥90%) fractions.
- **Edge case**: a 90-day session with many turns could produce thousands of samples. Bundle should export a summary (`SessionSummary`) rather than the raw sample array, or apply a row limit.

### diagnose_bundle.go compaction
- Priority drop order: `recent_logs` → `git_log` → `git_diff`. Non-omittable sections (item core, verdicts) survive even when budget is exceeded.
- Hard ceiling fires only after all omittable sections are gone.
- **Pattern to borrow**: define omittable vs. required bundle sections; drop omittable ones (scrollback tail, full context history, artifacts list) before touching required ones (session metadata, server info).

---

## 3. Backlog Item Creation Patterns

### proto/session/v1/backlog.proto — CreateBacklogItemRequest
Key fields available for bundle hand-off:
- `title` ← session title + timestamp
- `description` ← summary from bundle metadata
- `notes` ← bundle file path or URL
- `repo_path` ← session's repo path
- `external_url` ← optional bundle share URL
- `category` ← "bugfix" or "chore" depending on trigger

**Gap**: no `source_session_uuid` or `bundle_path` field exists. The feature must either embed the bundle path in `notes` (text) or add a new proto field `optional string diagnostic_bundle_path = 20` so the backlog item has a machine-readable link to the bundle.

---

## 4. Session Detail UI — Where to Host the Export Button

### web-app/src/components/sessions/SessionDetail.tsx
- Supported tabs: `"terminal" | "diff" | "vcs" | "logs" | "info" | "files" | "browser" | "artifacts" | "summary"`.
- `SessionDetailView.tsx` renders these tabs. The **"info" tab** already shows Instance ID (monospace with copy button), Status, Session Type, restart lineage.
- Best placement: add an "Export Diagnostic Bundle" button in the Info tab's action row, or as a secondary action in the tab toolbar. Could mirror the existing copy-button pattern.
- **Alternative**: a dedicated "diagnostics" tab — consistent with how `artifacts` and `summary` are separate tabs. Allows richer UI (bundle contents preview, backlog hand-off form) without cluttering Info.

---

## 5. Browser OTel / Analytics Infrastructure

### web-app/src/lib/telemetry/otel-init.ts
- OTel SDK already present: `@opentelemetry/sdk-trace-web`, `@opentelemetry/instrumentation-fetch`, `@opentelemetry/instrumentation-document-load`.
- Service name: `"stapler-squad-web"`. Traces exported to `/api/otel/v1/traces` (proxied by Go server).
- **Current gap**: no `session.uuid` attribute on spans. Adding it requires calling `span.setAttribute("session.uuid", sessionId)` where a session-scoped tracer/span is active.

### web-app/src/lib/analytics/types.ts
- `AnalyticsEvent` has optional `sessionId?: string` field — infrastructure is ready for session-scoped events.
- No Web Vitals integration exists yet. Adding `web-vitals` npm package + reporting via the existing `AnalyticsProvider.track()` interface would attach Web Vitals to session analytics events naturally.

---

## 6. Unstated Needs

### Bundle Versioning
- `debug_snapshot.go` already sets `Version: 1` on its JSON. The new bundle zip should include a `manifest.json` with a `bundle_format_version` field so future consumers can detect schema changes without crashing.
- Recommend starting at `v1`; bump on any breaking structural change to the zip layout.

### Bundle Sharing (URL)
- No bundle-sharing infrastructure exists. Options: (a) static download only (simplest, no server state); (b) server-side: save bundle, return a short-lived signed URL; (c) server-side: save bundle, return a stable path that maps to a `/api/bundles/{id}` endpoint.
- The `external_url` field on `BacklogItem` is already used for cross-host claim dedup — a bundle URL could populate this field, giving the item a stable identity.

### Bundle Format Discoverability
- Without a `manifest.json`, downstream tooling (other agents, CI) cannot know what a zip contains. Recommend a top-level `manifest.json` in the zip with: `bundle_format_version`, `session_uuid`, `generated_at`, `server_version`, list of included files and their roles.

### Multi-Session Backlog Items
- `BacklogItem.item_sessions` is a repeated `ItemSession` field (each with `session_uuid`) — items already map to multiple sessions.
- The RUM bundle should default to a single-session scope. When a backlog item has multiple linked sessions, the UI could offer "export bundle for this session" vs. "export bundle covering all linked sessions". Multi-session bundles need a modified manifest listing all session UUIDs.
- The `ItemSession.claimant_host_id` field complicates multi-session bundles when sessions ran on different hosts — cross-host scrollback may be unavailable.

---

## 3-Bullet Summary

- **Two solid patterns already exist**: `CreateDebugSnapshot` (server-writes JSON to disk, RPC-triggered) and `ExportButton`/`useExportRules` (client Blob-download); the new feature needs to compose both — a new RPC that either writes the zip to disk OR streams the bytes back for a browser Blob download, using the mode flag in the request.
- **Size-cap discipline is critical**: scrollback uses a 256 KB circular buffer (evicts oldest), context history retains 90 days of per-turn samples (could be thousands of rows for long sessions), and the diagnose_bundle compaction pattern (priority-ordered whole-section dropping + hard byte ceiling) is the established codebase idiom for keeping diagnostic outputs bounded — the RUM bundle must apply the same discipline per section.
- **Backlog hand-off has a proto gap**: `CreateBacklogItemRequest` has no `source_session_uuid` or `bundle_path` field; the feature must either embed the bundle path in `notes` (text, lower friction) or add a new proto field for machine-readable linkage, and the `session.uuid` OTel span attribute and Web Vitals `sessionId` analytics field both need to be wired up — the SDK is present but these attributes are absent from current instrumentation.
