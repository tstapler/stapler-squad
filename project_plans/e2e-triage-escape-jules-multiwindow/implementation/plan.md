# Plan: e2e triage (Complexity 1)

Migration / Observability / Risk Control: N/A — complexity 1. Domain glossary: none (no new types). Tech debt disposition: Extend as-is (test-only edits).

## Epic 1 — Fix four failing e2e tests (test-side drift; no app changes)

### Story 1.1 — Analytics global view (AC 1, 2)
- Task 1.1a `tests/e2e/escape-analytics-global-view.spec.ts` `buildGlobalSummaryResponse`: also send `correlationOutcomes` (= totalSequences), `correlationCoverage: 1`, `captureHealthy: true`. Root cause: card renders `correlationOutcomes` (`EscapeAnalyticsPage.tsx:296`).
- Task 1.1b `tests/e2e/pages/EscapeAnalyticsGlobalViewPage.ts:43` `getErrorBanner`: exclude `[data-testid="announcer-assertive"]`. Root cause: permanent live region `Announcer.tsx:46`.

### Story 1.2 — Multi-window (AC 6)
- Task 1.2a `tests/e2e/multi-window.spec.ts` `nonToastAlerts`: exclude the announcer; update doc comment.

### Story 1.3 — Jules badge (AC 3, 4, 5)
- Task 1.3a `tests/e2e/jules-status-badge.spec.ts` "no badge renders before the first Jules state arrives": additionally hold the `WatchBacklogItems` WebSocket with `page.routeWebSocket`, buffering client messages until the gate releases, then `connectToServer()` and replay/forward. Root cause: the watch stream is a WebSocket, which `page.route` cannot gate.
- AC 3, 4: no code change — they pass on a freshly built binary (reported HTML-for-JSON came from a binary lacking the seed route, `backlog_debug_seed_handler.go:121`). Verify by running.

### Story 1.4 — Verification (AC 7, 8)
- Task 1.4a Run the three specs on `chromium` and `chromium-dom` against the fresh binary; show output.
- Task 1.4b Keep assertions intact (diff review).

## Test mapping
Each AC is verified by its own Playwright test; see research/pitfalls.md for reproduction evidence.
